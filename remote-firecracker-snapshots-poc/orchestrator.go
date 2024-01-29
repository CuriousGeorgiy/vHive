package main

import (
	"context"
	"fmt"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/snapshots"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	"github.com/opencontainers/image-spec/identity"
	"github.com/vhive-serverless/remote-firecracker-snapshots-poc/networking"
	"github.com/vhive-serverless/remote-firecracker-snapshots-poc/snapshotting"
	"log"
	"path/filepath"
	"strings"
)

type VMInfo struct {
	imgName        string
	ctrSnapKey     string
	snapBooted     bool
	ctrSnapDevPath string
	ctr            containerd.Container
	task           containerd.Task
}

type Orchestrator struct {
	cachedImages map[string]containerd.Image
	vms          map[string]VMInfo

	snapshotter     string
	client          *containerd.Client
	fcClient        *fcclient.Client
	snapshotService snapshots.Snapshotter
	leaseManager    leases.Manager
	leases          map[string]*leases.Lease
	networkManager  *networking.NetworkManager
	snapshotManager *snapshotting.SnapshotManager

	// Namespace for requests to containerd  API. Allows multiple consumers to use the same containerd without
	// conflicting eachother. Benefit of sharing content but still having separation with containers and images
	ctx context.Context
}

// NewOrchestrator Initializes a new orchestrator
func NewOrchestrator(snapshotter, containerdNamespace, snapsBasePath string) (*Orchestrator, error) {
	var err error

	orch := new(Orchestrator)
	orch.cachedImages = make(map[string]containerd.Image)
	orch.vms = make(map[string]VMInfo)
	orch.snapshotter = snapshotter
	orch.ctx = namespaces.WithNamespace(context.Background(), containerdNamespace)
	orch.networkManager = networking.NewNetworkManager()

	orch.snapshotManager = snapshotting.NewSnapshotManager(snapsBasePath)
	err = orch.snapshotManager.RecoverSnapshots(snapsBasePath)
	if err != nil {
		return nil, fmt.Errorf("recovering snapshots: %w", err)
	}

	// Connect to firecracker client
	log.Println("Creating firecracker client")
	orch.fcClient, err = fcclient.New(containerdTTRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("creating firecracker client: %w", err)
	}
	log.Println("Created firecracker client")

	// Connect to containerd client
	log.Println("Creating containerd client")
	orch.client, err = containerd.New(containerdAddress)
	if err != nil {
		return nil, fmt.Errorf("creating containerd client: %w", err)
	}
	log.Println("Created containerd client")

	// Create containerd snapshot service
	orch.snapshotService = orch.client.SnapshotService(snapshotter)

	orch.leaseManager = orch.client.LeasesService()
	orch.leases = make(map[string]*leases.Lease)

	return orch, nil
}

// Converts an image name to a url if it is not a URL
func getImageURL(image string) string {
	// Pull from dockerhub by default if not specified (default k8s behavior)
	if strings.Contains(image, ".") {
		return image
	}
	return "docker.io/" + image

}

func (orch *Orchestrator) getContainerImage(imageName string) (*containerd.Image, error) {
	image, found := orch.cachedImages[imageName]
	if !found {
		var err error
		log.Printf("Pulling image %s\n", imageName)

		imageURL := getImageURL(imageName)
		image, err = orch.client.Pull(orch.ctx, imageURL,
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(snapshotter),
		)

		if err != nil {
			return nil, fmt.Errorf("pulling container image: %w", err)
		}
		log.Printf("Successfully pulled %s image with %s\n", image.Name(), snapshotter)

		orch.cachedImages[imageName] = image
	}

	return &image, nil
}

func getImageKey(img containerd.Image, ctx context.Context) (string, error) {
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return "", err
	}
	return identity.ChainID(diffIDs).String(), nil
}

func (orch *Orchestrator) createCtrSnap(snapKey string, img containerd.Image) (string, error) {
	// Get image key (image is parent of container)
	parent, err := getImageKey(img, orch.ctx)
	if err != nil {
		return "", err
	}

	lease, err := orch.leaseManager.Create(orch.ctx, leases.WithID(snapKey))
	if err != nil {
		return "", err
	}
	orch.leases[snapKey] = &lease
	// Update current context to add lease
	ctx := leases.WithLease(orch.ctx, lease.ID)

	mounts, err := orch.snapshotService.Prepare(ctx, snapKey, parent)
	if err != nil {
		return "", err
	}

	if len(mounts) != 1 {
		log.Panic("expected snapshot to only have one mount")
	}

	// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
	return mounts[0].Source, nil
}

func (orch *Orchestrator) createVM(vmID string) error {
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
					PrimaryAddr: orch.networkManager.GetConfig(vmID).GetContainerCIDR(),
					GatewayAddr: orch.networkManager.GetConfig(vmID).GetGatewayIP(),
					Nameservers: []string{"8.8.8.8"},
				},
			},
		}},
		NetNS: orch.networkManager.GetConfig(vmID).GetNamespacePath(),
	}

	log.Println("Creating firecracker VM")
	_, err := orch.fcClient.CreateVM(orch.ctx, createVMRequest)
	if err != nil {
		return fmt.Errorf("creating firecracker VM: %w", err)
	}

	return nil
}

func (orch *Orchestrator) getCtrSnapDevPath(ctrSnapKey string) (string, error) {
	mounts, err := orch.snapshotService.Mounts(orch.ctx, ctrSnapKey)
	if err != nil {
		return "", err
	}
	if len(mounts) != 1 {
		log.Panic("expected snapshot to only have one mount")
	}

	// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
	return mounts[0].Source, nil
}

func (orch *Orchestrator) startContainer(vmID, snapKey, imageName string, img *containerd.Image) error {
	log.Println("Creating new container")
	ctr, err := orch.client.NewContainer(
		orch.ctx,
		snapKey,
		containerd.WithSnapshotter(orch.snapshotter),
		containerd.WithNewSnapshot(snapKey, *img),
		containerd.WithNewSpec(
			oci.WithImageConfig(*img),
			firecrackeroci.WithVMID(vmID),
			firecrackeroci.WithVMNetwork,
		),
		containerd.WithRuntime("aws.firecracker", nil),
	)
	if err != nil {
		return fmt.Errorf("creating new container: %w", err)
	}

	log.Println("Creating new container task")
	task, err := ctr.NewTask(orch.ctx, cio.NewCreator(cio.WithStreams(nil, nil, nil)))
	if err != nil {
		return fmt.Errorf("creating new container task: %w", err)
	}

	log.Println("Starting container task")
	if err := task.Start(orch.ctx); err != nil {
		return fmt.Errorf("starting container task: %w", err)
	}

	ctrSnapDevPath, err := orch.getCtrSnapDevPath(snapKey)
	if err != nil {
		return fmt.Errorf("getting snapshot's disk device path: %w", err)
	}

	// Store snapshot info
	orch.vms[vmID] = VMInfo{
		imgName:        imageName,
		ctrSnapKey:     snapKey,
		ctrSnapDevPath: ctrSnapDevPath,
		snapBooted:     false,
		ctr:            ctr,
		task:           task,
	}

	return nil
}

// Extract changes applied by container on top of image layer
func (orch *Orchestrator) extractPatch(vmID, patchPath string) error {
	vmInfo := orch.vms[vmID]
	vmImage := orch.cachedImages[vmInfo.imgName]

	log.Println("Creating image snapshot")
	tempImageSnapshotKey := fmt.Sprintf("tempimagesnap%s", vmID)
	imgDevPath, err := orch.createCtrSnap(tempImageSnapshotKey, vmImage)
	if err != nil {
		return fmt.Errorf("creating image snapshot: %w", err)
	}
	defer func() {
		orch.snapshotService.Remove(orch.ctx, tempImageSnapshotKey)
		orch.leaseManager.Delete(orch.ctx, *orch.leases[tempImageSnapshotKey])
		delete(orch.leases, vmInfo.ctrSnapKey)
	}()

	log.Println("Creating image and container snapshots")
	imgMountPath, err := mountCtrSnap(imgDevPath, true)
	if err != nil {
		return fmt.Errorf("mounting image snapshot: %w", err)
	}
	defer unmountCtrSnap(imgMountPath)
	ctrSnapMountPath, err := mountCtrSnap(vmInfo.ctrSnapDevPath, true)
	if err != nil {
		return fmt.Errorf("mounting container snapshot: %w", err)
	}
	defer unmountCtrSnap(ctrSnapMountPath)

	log.Println("Creating container snapshot patch")
	err = createPatch(imgMountPath, ctrSnapMountPath, patchPath)
	if err != nil {
		return err
	}

	return err
}

// Apply patch on top of container layer
func (orch *Orchestrator) restoreCtrSnap(ctrSnapDevPath, patchPath string) error {
	log.Println("Mounting container snapshot")
	ctrSnapMountPath, err := mountCtrSnap(ctrSnapDevPath, false)
	if err != nil {
		return err
	}

	log.Println("Applying patch to container snapshot")
	err = applyPatch(ctrSnapMountPath, patchPath)
	if err != nil {
		return err
	}

	err = unmountCtrSnap(ctrSnapMountPath)
	if err != nil {
		return err
	}

	return err
}

func (orch *Orchestrator) createSnapshot(vmID, revision string) error {
	//vmInfo := orch.vms[vmID]

	log.Println("Pausing VM")
	if _, err := orch.fcClient.PauseVM(orch.ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		return fmt.Errorf("pausing VM: %w", err)
	}

	snap, err := orch.snapshotManager.RegisterSnap(revision)
	if err != nil {
		return fmt.Errorf("adding snapshot to snapshot manager: %w", err)
	}

	log.Println("Creating VM snapshot")
	createSnapshotRequest := &proto.CreateSnapshotRequest{
		VMID:         vmID,
		MemFilePath:  snap.GetMemFilePath(),
		SnapshotPath: snap.GetSnapFilePath(),
	}
	if _, err := orch.fcClient.CreateSnapshot(orch.ctx, createSnapshotRequest); err != nil {
		return fmt.Errorf("creating VM snapshot: %w", err)
	}

	log.Println("Extracting container snapshot patch")
	if err := orch.extractPatch(vmID, snap.GetPatchFilePath()); err != nil {
		log.Printf("failed to create container patch file")
		return err
	}

	log.Println("Resuming VM")
	if _, err := orch.fcClient.ResumeVM(orch.ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		return fmt.Errorf("resuming VM: %w", err)
	}

	log.Println("Digging holes in guest memory file")
	if err := digHoles(snap.GetMemFilePath()); err != nil {
		return fmt.Errorf("digging holes in guest memory file: %w", err)
	}

	log.Println("Serializing snapshot information")
	snapInfo := snapshot{
		Img: orch.vms[vmID].imgName,
	}
	if err := serializeSnapInfo(snap.GetInfoFilePath(), snapInfo); err != nil {
		return fmt.Errorf("serializing snapshot information: %w", err)
	}

	return nil
}

func (orch *Orchestrator) restoreSnapInfo(vmID, snapshotKey, infoFile string) (*VMInfo, error) {
	log.Println("Deserializing snapshot information")
	snapInfo, err := deserializeSnapInfo(infoFile)
	if err != nil {
		return nil, fmt.Errorf("deserializing snapshot information: %w", err)
	}

	vmInfo := VMInfo{
		imgName:    snapInfo.Img,
		ctrSnapKey: snapshotKey,
		snapBooted: true,
	}
	orch.vms[vmID] = vmInfo
	return &vmInfo, nil
}

func (orch *Orchestrator) bootVMFromSnapshot(vmID, revision string) error {
	snapKey := getSnapKey(vmID)

	log.Println("Restoring snapshot information")
	vmInfo, err := orch.restoreSnapInfo(vmID, snapKey, filepath.Join(orch.snapshotManager.BasePath, revision, "infofile"))
	if err != nil {
		return fmt.Errorf("restoring snapshot information: %w", err)
	}

	log.Println("Retrieving container image")
	img, err := orch.getContainerImage(vmInfo.imgName)
	if err != nil {
		return fmt.Errorf("getting container image: %w", err)
	}

	log.Println("Creating container snapshot")
	ctrSnapDevPath, err := orch.createCtrSnap(snapKey, *img)
	if err != nil {
		return fmt.Errorf("creating container snapshot: %w", err)
	}

	log.Println("Restoring container snapshot")
	err = orch.restoreCtrSnap(ctrSnapDevPath, filepath.Join(orch.snapshotManager.BasePath, revision, "patchfile"))
	if err != nil {
		return fmt.Errorf("restoring container snapshot: %w", err)
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
					PrimaryAddr: orch.networkManager.GetConfig(vmID).GetContainerCIDR(),
					GatewayAddr: orch.networkManager.GetConfig(vmID).GetGatewayIP(),
					Nameservers: []string{"8.8.8.8"},
				},
			},
		}},
		NetNS:                 orch.networkManager.GetConfig(vmID).GetNamespacePath(),
		LoadSnapshot:          true,
		MemFilePath:           filepath.Join(orch.snapshotManager.BasePath, revision, "memfile"),
		SnapshotPath:          filepath.Join(orch.snapshotManager.BasePath, revision, "snapfile"),
		ContainerSnapshotPath: ctrSnapDevPath,
	}

	log.Println("Creating firecracker VM from snapshot")
	_, err = orch.fcClient.CreateVM(orch.ctx, createVMRequest)
	if err != nil {
		return fmt.Errorf("creating firecracker VM: %w", err)
	}

	return nil
}

func (orch *Orchestrator) stopVm(vmID string) error {
	vmInfo := orch.vms[vmID]

	if !vmInfo.snapBooted {
		fmt.Println("Killing container task")
		if err := vmInfo.task.Kill(orch.ctx, syscall.SIGKILL); err != nil {
			return fmt.Errorf("killing container: %w", err)
		}

		fmt.Println("Waiting for container task to exit")
		exitStatusChannel, err := vmInfo.task.Wait(orch.ctx)
		if err != nil {
			return fmt.Errorf("retrieving container task exit code channel: %w", err)
		}

		<-exitStatusChannel

		fmt.Println("Deleting container task")
		if _, err := vmInfo.task.Delete(orch.ctx); err != nil {
			return fmt.Errorf("deleting container task: %w", err)
		}

		fmt.Println("Deleting container")
		if err := vmInfo.ctr.Delete(orch.ctx, containerd.WithSnapshotCleanup); err != nil {
			return fmt.Errorf("deleting container: %w", err)
		}
	}

	fmt.Println("Stopping VM")
	if _, err := orch.fcClient.StopVM(orch.ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		log.Printf("failed to stop the vm")
		return err
	}

	if vmInfo.snapBooted {
		fmt.Println("Removing snapshot")
		err := orch.snapshotService.Remove(orch.ctx, vmInfo.ctrSnapKey)
		if err != nil {
			log.Printf("failed to deactivate container snapshot")
			return err
		}
		if err := orch.leaseManager.Delete(orch.ctx, *orch.leases[vmInfo.ctrSnapKey]); err != nil {
			return err
		}
		delete(orch.leases, vmInfo.ctrSnapKey)
	}

	fmt.Println("Removing network")
	if err := orch.networkManager.RemoveNetwork(vmID); err != nil {
		log.Printf("failed to cleanup network")
		return err
	}

	return nil
}

func (orch *Orchestrator) tearDown() {
	orch.client.Close()
	orch.fcClient.Close()
}
