module github.com/vhive-serverless/vhive

go 1.16

// Copied from firecracker-containerd
replace (
	// Pin gPRC-related dependencies as like containerd v1.5.2
	github.com/gogo/googleapis => github.com/gogo/googleapis v1.3.2
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.5
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63
	google.golang.org/grpc => google.golang.org/grpc v1.27.1
)

replace (
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20161114122254-48702e0da86b
	k8s.io/api => k8s.io/api v0.16.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.7-beta.0
	k8s.io/apiserver => k8s.io/apiserver v0.16.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.6
	k8s.io/client-go => k8s.io/client-go v0.16.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.6
	k8s.io/code-generator => k8s.io/code-generator v0.16.7-beta.0
	k8s.io/component-base => k8s.io/component-base v0.16.6
	k8s.io/cri-api => k8s.io/cri-api v0.16.16-rc.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.6
	k8s.io/kubectl => k8s.io/kubectl v0.16.6
	k8s.io/kubelet => k8s.io/kubelet v0.16.6
	k8s.io/kubernetes => k8s.io/kubernetes v1.16.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.6
	k8s.io/metrics => k8s.io/metrics v0.16.6
	k8s.io/node-api => k8s.io/node-api v0.16.6
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.6
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.16.6
	k8s.io/sample-controller => k8s.io/sample-controller v0.16.6
)

replace (
	github.com/containerd/containerd => github.com/ease-lab/containerd v1.5.5-ids // TODO: change to vhive
	github.com/firecracker-microvm/firecracker-containerd => github.com/amohoste/firecracker-containerd v1.0.0-enhanced-snap
	github.com/vhive-serverless/vhive/examples/protobuf/helloworld => ./examples/protobuf/helloworld
)

require (
	github.com/containerd/containerd v1.5.8
	github.com/containerd/go-cni v1.1.4
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-00010101000000-000000000000
	github.com/ftrvxmtrx/fd v0.0.0-20150925145434-c6d800382fff
	github.com/go-multierror/multierror v1.0.2
	github.com/golang/protobuf v1.5.2
	github.com/google/nftables v0.0.0-20210916140115-16a134723a96
	github.com/montanaflynn/stats v0.6.5
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.0
	github.com/vhive-serverless/vhive/examples/protobuf/helloworld v0.0.0-00010101000000-000000000000
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	github.com/wcharczuk/go-chart v2.0.1+incompatible
	golang.org/x/net v0.0.0-20211005001312-d4b1ae081e3b
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8
	gonum.org/v1/gonum v0.9.0
	gonum.org/v1/plot v0.9.0
	google.golang.org/grpc v1.41.0
	k8s.io/cri-api v0.20.6
)

require (
	github.com/antchfx/xpath v1.2.4 // indirect
	github.com/blend/go-sdk v1.20211025.3 // indirect
	github.com/opencontainers/image-spec v1.0.1
	github.com/tamerh/xml-stream-parser v1.4.0
	github.com/tamerh/xpath v1.0.0 // indirect
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae
	golang.org/x/image v0.0.0-20210220032944-ac19c3e999fb // indirect
)
