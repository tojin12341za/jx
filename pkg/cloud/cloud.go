package cloud

import (
	"sort"
	"strings"
)

const (
	AKS        = "aks"
	ALIBABA    = "alibaba"
	AWS        = "aws"
	EKS        = "eks"
	GKE        = "gke"
	ICP        = "icp"
	IKS        = "iks"
	KIND       = "kind"
	KUBERNETES = "kubernetes"
	MINIKUBE   = "minikube"
	MINISHIFT  = "minishift"
	OKE        = "oke"
	OPENSHIFT  = "openshift"
	PKS        = "pks"
)

// KubernetesProviders list of all available Kubernetes providers
var KubernetesProviders = []string{AKS, ALIBABA, AWS, EKS, GKE, KIND, KUBERNETES, ICP, IKS, OKE, OPENSHIFT, MINIKUBE, MINISHIFT, PKS}

// KubernetesProviderOptions returns all the Kubernetes providers as a string
func KubernetesProviderOptions() string {
	values := []string{}
	values = append(values, KubernetesProviders...)
	sort.Strings(values)
	return strings.Join(values, ", ")
}
