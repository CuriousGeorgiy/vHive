## Manual reload of remote snapshots
This is based on the initial implementation prototype for full local snapshots. Credit: Amory Hoste (https://github.com/amohoste).


### Prerequisites
- Run the ./scripts/cloudlab/setup_node.sh script.
- Copy the snapshot files (memfile, snapfile, infofile) on the cluster node where you want to start the function.
-  Install docker CE
````
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli aufs-tools

sudo usermod -aG docker ${USER}
sudo su - ${USER}
````

- Start local docker registry server
```
docker run -d --network host --restart=always --name registry registry:2
```

# Steps
- Start firecracker-containerd in a new terminal
```
sudo /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml
sudo /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee fctr.out
```
- Build go program for reloading
```
go build
```
- Create a snapshot
```
sudo ./vanilla-firecracker-snapshots -make-snap -id "0" -image "docker.io/library/nginx:1.17-alpine" -revision "0" -snapshots-base-path "/users/glebedev/vhive/vanilla_firecracker_snapshots/snaps"
sudo ./vanilla-firecracker-snapshots -make-snap -id "<VM identifier>" -image "docker.io/qorbani/golang-hello-world" -revision "<revision identifier>" -snapshots-base-path "/users/glebedev/vhive/vanilla_firecracker_snapshots/snaps"
"docker.io/curiousgeorgiy/golang-hello-world:latest"
```
- Boot from snapshot
```
sudo ./vanilla-firecracker-snapshots -boot-from-snap -id "0" -revision "0" -snapshots-base-path "/users/glebedev/vhive/vanilla_firecracker_snapshots/remote-snaps"
```

Now, the uVM is started and this is confirmed by the logs of firecracker-containerd, which also gives the IP address of the uVM.

- Send a request
```
curl http://<VM IP address>:<container port>
```
