module github.com/openkruise/kruise

go 1.16

require (
	github.com/alibaba/pouch v0.0.0-20190328125340-37051654f368
	github.com/appscode/jsonpatch v1.0.1
	github.com/codegangsta/negroni v1.0.0
	github.com/containerd/containerd v1.5.9
	github.com/contiv/executor v0.0.0-20180626233236-d263f4daa3ad // indirect
	github.com/docker/distribution v2.7.2-0.20200708230840-70e0022e42fd+incompatible
	github.com/docker/docker v20.10.2+incompatible
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-openapi/errors v0.20.1 // indirect
	github.com/go-openapi/spec v0.20.3
	github.com/go-openapi/strfmt v0.21.1 // indirect
	github.com/go-openapi/validate v0.20.3 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/robfig/cron v1.2.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/xyproto/simpleredis v0.0.0-20200201215242-1ff0da2967b4
	golang.org/x/net v0.0.0-20211209124913-491a49abca63
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	gomodules.xyz/jsonpatch/v2 v2.2.0
	google.golang.org/grpc v1.38.0
	k8s.io/api v0.22.5
	k8s.io/apiextensions-apiserver v0.22.5
	k8s.io/apimachinery v0.22.5
	k8s.io/apiserver v0.22.5
	k8s.io/client-go v0.22.5
	k8s.io/code-generator v0.22.5
	k8s.io/component-base v0.22.5
	k8s.io/component-helpers v0.22.5
	k8s.io/cri-api v0.22.5
	k8s.io/klog/v2 v2.9.0
	k8s.io/kube-openapi v0.0.0-20211109043538-20434351676c
	k8s.io/kubernetes v1.22.5
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/controller-runtime v0.10.2
)

// Replace to match K8s 1.22.5
replace (
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega => github.com/onsi/gomega v1.10.3
	k8s.io/api => k8s.io/api v0.22.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.5
	k8s.io/apiserver => k8s.io/apiserver v0.22.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.5
	k8s.io/client-go => k8s.io/client-go v0.22.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.5
	k8s.io/code-generator => k8s.io/code-generator v0.22.5
	k8s.io/component-base => k8s.io/component-base v0.22.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.22.5
	k8s.io/cri-api => k8s.io/cri-api v0.22.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.5
	k8s.io/kubectl => k8s.io/kubectl v0.22.5
	k8s.io/kubelet => k8s.io/kubelet v0.22.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.5
	k8s.io/metrics => k8s.io/metrics v0.22.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.22.5
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.22.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.5
)
