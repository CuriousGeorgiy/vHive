package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/vhive-serverless/vanilla-firecracker-snapshots/networking"
	"log"
	"net"
)

const (
	containerdAddress      = "/run/firecracker-containerd/containerd.sock"
	containerdTTRPCAddress = containerdAddress + ".ttrpc"
	containerdNamespace    = "vanilla-firecracker-snapshots"
	macAddress             = "AA:FC:00:00:00:01"
	hostDevName            = "tap0"
	snapshotter            = "devmapper"
)

func main() {
	var vmID = flag.String("id", "", "virtual machine identifier")
	var image = flag.String("image", "", "container image name")
	var bootFromSnap = flag.Bool("boot-from-snap", false, "boot from snapshot")
	var containerSnapPath = flag.String("container-snap-path", "", "path to container snapshot")

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	flag.Parse()

	if *vmID == "" {
		log.Fatal("Incorrect usage. 'id' needs to be specified")
	}

	if *image == "" && !*bootFromSnap {
		log.Fatal("Incorrect usage. 'image' needs to be specified when 'boot-snap' is false")
	}

	if *containerSnapPath == "" && *bootFromSnap {
		log.Fatal("Incorrect usage. 'image' needs to be specified when 'boot-snap' is false")
	}

	if err := taskWorkflow(*vmID, *image, *bootFromSnap, *containerSnapPath); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Press the Enter Key to stop anytime")
	fmt.Scanln()
}

func taskWorkflow(vmID, image string, bootFromSnap bool, containerSnapPath string) (err error) {
	log.Println("Creating network")
	networkManager := networking.NewNetworkManager()
	if err := networkManager.CreateNetwork(vmID); err != nil {
		return fmt.Errorf("creating network: %w", err)
	}

	if !bootFromSnap {
		log.Println("Bootstrapping VM")
		err = bootstrapVM(networkManager, vmID, image)
		if err != nil {
			return fmt.Errorf("bootstrapping VM: %w", err)
		}
	} else {
		err = bootVMfromSnapshot(networkManager, vmID, containerSnapPath)
		if err != nil {
			return fmt.Errorf("boot VM from snapshot: %w", err)
		}
	}
	fmt.Printf("VM available at IP: %s\n", networkManager.GetConfig(vmID).GetCloneIP())

	networkManager.RemoveNetwork(vmID)

	return nil
}

func bootstrapVM(networkManager *networking.NetworkManager, vmID, imageName string) error {
	log.Println("Creating containerd client")
	client, err := containerd.New(containerdAddress)
	if err != nil {
		return fmt.Errorf("creating containerd client: %w", err)
	}

	log.Println("Creating firecracker client")
	fcClient, err := fcclient.New(containerdTTRPCAddress)
	if err != nil {
		return fmt.Errorf("creating firecracker client: %w", err)
	}

	ctx := namespaces.WithNamespace(context.Background(), containerdNamespace)

	log.Printf("Pulling image: %s\n", imageName)
	image, err := client.Pull(ctx, imageName,
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter(snapshotter),
	)
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}

	createVMRequest := &proto.CreateVMRequest{
		VMID: vmID,
		// Enabling Go Race Detector makes in-microVM binaries heavy in terms of CPU and memory.
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  2,
			MemSizeMib: 2048,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  macAddress,
				HostDevName: hostDevName,
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: networkManager.GetConfig(vmID).GetContainerCIDR(),
					GatewayAddr: networkManager.GetConfig(vmID).GetGatewayIP(),
					Nameservers: []string{"8.8.8.8"},
				},
			},
		}},
		NetNS: networkManager.GetConfig(vmID).GetNamespacePath(),
	}

	log.Println("Creating firecracker VM")
	_, err = fcClient.CreateVM(ctx, createVMRequest)
	if err != nil {
		return fmt.Errorf("creating firecracker VM: %w", err)
	}

	log.Println("Creating new container")
	ctr, err := client.NewContainer(
		ctx,
		getSnapKey(vmID),
		containerd.WithSnapshotter(snapshotter),
		containerd.WithNewSnapshot(getSnapKey(vmID), image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			firecrackeroci.WithVMID(vmID),
			firecrackeroci.WithVMNetwork,
		),
		containerd.WithRuntime("aws.firecracker", nil),
	)
	if err != nil {
		return fmt.Errorf("creating new container: %w", err)
	}

	log.Println("Creating new container task")
	task, err := ctr.NewTask(ctx, cio.NewCreator(cio.WithStreams(nil, nil, nil)))
	if err != nil {
		return fmt.Errorf("creating new container task: %w", err)
	}

	log.Println("Starting container task")
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("starting container task: %w", err)
	}

	log.Println("Getting container snapshot mounts")
	mounts, err := client.SnapshotService(snapshotter).Mounts(ctx, getSnapKey(vmID))
	if err != nil {
		return fmt.Errorf("getting container mounts: %w", err)
	}
	if len(mounts) != 1 {
		log.Panic("expected snapshot to only have one mount")
	}
	log.Printf("Container snapshot mount: %s \n", mounts[0].Source)

	return nil
}

func getSnapKey(vmID string) string {
	return "demo-snap" + vmID
}

func bootVMfromSnapshot(networkManager *networking.NetworkManager, vmID, containerSnapPath string) error {
	ctx := context.Background()

	socketFile := "/tmp/firecracker.socket"
	ip, ipNet, err := net.ParseCIDR(networkManager.GetConfig(vmID).GetContainerCIDR())
	if err != nil {
		return fmt.Errorf("failed to parse CIDR: %w", err)
	}
	networkInterface := sdk.NetworkInterface{
		StaticConfiguration: &sdk.StaticNetworkConfiguration{
			MacAddress:  macAddress,
			HostDevName: hostDevName,
			IPConfiguration: &sdk.IPConfiguration{
				IPAddr: net.IPNet{
					IP:   ip,
					Mask: ipNet.Mask,
				},
				Gateway:     net.ParseIP(networkManager.GetConfig(vmID).GetGatewayIP()),
				Nameservers: []string{"8.8.8.8"},
			},
		},
	}
	cfg := sdk.Config{
		SocketPath: socketFile,
		NetworkInterfaces: []sdk.NetworkInterface{
			networkInterface,
		},
		VMID: vmID,
		NetNS: networkManager.GetConfig(vmID).GetNamespacePath(),
	}

	cmd := sdk.VMCommandBuilder{}.WithSocketPath(socketFile).WithBin("/usr/local/bin/firecracker").Build(ctx)
	m, err := sdk.NewMachine(ctx, cfg, sdk.WithProcessRunner(cmd),
		sdk.WithSnapshot("/tmp/mem_file", "/tmp/snapshot_file", containerSnapPath,
			func(c *sdk.SnapshotConfig) { c.ResumeVM = true }))
	if err != nil {
		return fmt.Errorf("creating new firecracker machine: %w", err)
	}

	err = m.Start(ctx)
	if err != nil {
		return fmt.Errorf("starting new firecracker machine: %w", err)
	}

	fmt.Printf("VM available at IP: %s\n", networkManager.GetConfig(vmID).GetCloneIP())
	fmt.Println("Press the Enter Key to stop anytime")
	fmt.Scanln()

	if err := m.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}

	if err := m.StopVMM(); err != nil {
		log.Fatal(err)
	}

	return nil
}
