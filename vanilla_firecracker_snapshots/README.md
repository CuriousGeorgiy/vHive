## Manual reload of remote snapshots
This is based on the initial implementation prototype for full local snapshots. Credit: Amory Hoste (https://github.com/amohoste).


### Prerequisites
- Run the ./scripts/cloudlab/setup_node.sh script.
- Copy the snapshot files (memfile, snapfile, infofile, patchfile) on the cluster node where you want to start the function.
-  Install docker CE
````
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli aufs-tools

sudo usermod -aG docker ${USER}
sudo su - ${USER}

docker run -d --network host --restart=always --name registry registry:2
````

- Setup network
```
# Save old iptable rules
sudo iptables-save > iptables.rules.old

# Enable ipv4 forwarding (send package from one interface to other on same device)
sudo sh -c "echo 1 > /proc/sys/net/ipv4/ip_forward"

# Nat
sudo iptables -t nat -A POSTROUTING -o eno49 -j MASQUERADE
sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
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
sudo ./vanilla-firecracker-snapshots -make-snap -id "<VM identifier>" -image "docker.io/library/nginx:1.17-alpine" -revision "<revision identifier>" -snapshots-base-path "/users/glebedev/vhive/vanilla_firecracker_snapshots/snaps"
sudo ./vanilla-firecracker-snapshots -make-snap -id "<VM identifier>" -image "docker.io/qorbani/golang-hello-world" -revision "<revision identifier>" -snapshots-base-path "/users/glebedev/vhive/vanilla_firecracker_snapshots/snaps"
```
- Boot from snapshot
```
sudo ./vanilla-firecracker-snapshots -boot-from-snap -id "<VM id>" -revision "<container snapshot revision>" -snapshots-base-path "/users/glebedev/vhive/vanilla_firecracker_snapshots/snaps"
```

Now, the uVM is started and this is confirmed by the logs of firecracker-containerd, which also gives the IP address of the uVM.

- Send a request
```
curl http://<VM IP address>
```
