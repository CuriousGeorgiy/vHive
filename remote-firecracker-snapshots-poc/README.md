# Manual creation and booting of remote snapshots
This is based on the initial implementation prototype for full local snapshots. 

## Prerequisites
- Run the ./scripts/cloudlab/setup_node.sh script.
- Copy the snapshot files (memfile, snapfile, infofile) on the cluster node where you want to start the function.

# Steps
- Start firecracker-containerd in a new terminal
```
sudo /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml
```
- Build go program
```
go build
```
- Create a VM.
```
# sudo ./remote-firecracker-snapshots-poc -id "<VM ID>" -image "<URI>" -revision "<revision ID>" -snapshots-base-path "<absolute/path/to/snapshots/folder>"
sudo ./remote-firecracker-snapshots-poc -id "0" -image "docker.io/library/nginx:1.17-alpine" -revision "nginx-0" -snapshots-base-path "/users/glebedev/vhive/remote-firecracker-snapshots-poc/snaps"
```

- Create a VM and make a snapshot from it.
```
# sudo ./remote-firecracker-snapshots-poc -make-snap -id "<VM ID>" -image "<URI>" -revision "<revision ID>" -snapshots-base-path "<absolute/path/to/snapshots/folder>"
sudo ./remote-firecracker-snapshots-poc -make-snap -id "0" -image "docker.io/library/nginx:1.17-alpine" -revision "nginx-0" -snapshots-base-path "/users/glebedev/vhive/remote-firecracker-snapshots-poc/snaps"
```
- Boot from the previously made snapshot.
```
# sudo ./remote-firecracker-snapshots-poc -boot-from-snap -id "<VM ID>" -revision "<revision ID>" -snapshots-base-path "<absolute/path/to/snapshots/folder>"
sudo ./remote-firecracker-snapshots-poc -boot-from-snap -id "0" -revision "nginx-0" -snapshots-base-path "/users/glebedev/vhive/remote-firecracker-snapshots-poc/snaps"
```

Now, the VM is started and this is confirmed by the logs of firecracker-containerd, which also gives the IP address of the VM.

- Send a request
```
curl http://<VM IP address>:<container port>
```

- Connect via ssh
```
# ssh -i <absolute path or file name of private key> root@<VM IP address>
ssh -i "/users/glebedev/vhive/bin/firecracker_rsa" root@172.18.0.3
```