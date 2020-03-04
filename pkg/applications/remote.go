package applications

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"cloud.google.com/go/container/apiv1"
	"github.com/jenkins-x/jx/pkg/jxfactory/connector"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/errors"

	containerpb "google.golang.org/genproto/googleapis/container/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	kubeConfigTemplate = `apiVersion: v1
kind: Config
current-context: my-cluster
contexts: [{name: my-cluster, context: {cluster: cluster-1, user: user-1}}]
users: [{name: user-1, user: {auth-provider: {name: gcp}}}]
clusters:
- name: cluster-1
  cluster:
    server: "https://%s"
    certificate-authority-data: "%s"
`
)

type clusterInfo struct {
	// [Output only] The IP address of this cluster's master endpoint.
	// The endpoint can be accessed from the internet at
	// `https://username:password@endpoint/`.
	//
	// See the `masterAuth` property of this resource for username and
	// password information.
	Endpoint string `json:"endpoint,omitempty"`

	// The authentication information for accessing the master endpoint.
	MasterAuth *masterAuth `json:"masterAuth,omitempty"`
}

type masterAuth struct {
	// [Output only] Base64-encoded public certificate that is the root of
	// trust for the cluster.
	ClusterCaCertificate string `json:"clusterCaCertificate,omitempty"`
}

func GetWorkspaceKubeConfig(useGcloud bool, project string, cluster string, region string, zone string) (string, error) {
	location := region
	if location == "" {
		location = zone
	}
	if useGcloud {
		args := []string{"container", "clusters", "describe", "--project", project, cluster, "--format", "json"}
		if region != "" {
			args = append(args, "--region", region)
		} else {
			args = append(args, "--zone", zone)
		}
		cmd := util.Command{
			Name: "gcloud",
			Args: args,
		}
		text, err := cmd.RunWithoutRetry()
		if err != nil {
			return "", errors.Wrapf(err, "failed to query cluster information for project %s cluster %s location %s", project, cluster, location)
		}
		if text == "" {
			return "", errors.Wrapf(err, "no results querying cluster information for project %s cluster %s location %s", project, cluster, location)
		}
		//fmt.Printf("got json: %s\n", text)
		cl := &clusterInfo{}
		err = json.Unmarshal([]byte(text), cl)
		if err != nil {
			return "", errors.Wrapf(err, "failed to unmarshal yaml results querying cluster information for project %s cluster %s location %s", project, cluster, location)
		}
		if cl.MasterAuth == nil {
			return "", fmt.Errorf("no cluster.MasterAuth when querying cluster information for project %s cluster %s location %s", project, cluster, location)
		}
		return createKubeConfig(cl.MasterAuth.ClusterCaCertificate, cl.Endpoint, project, cluster, location)
	}

	ctx := context.Background()
	c, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to create gke cluster manager client")
	}

	name := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, cluster)
	req := &containerpb.GetClusterRequest{
		Name: name,
	}
	cl, err := c.GetCluster(ctx, req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to query cluster information for project %s cluster %s location %s", project, cluster, location)
	}
	if cl == nil {
		return "", fmt.Errorf("no Cluster found for project %s cluster %s location %s", project, cluster, location)
	}
	if cl.MasterAuth == nil {
		return "", fmt.Errorf("no Cluster.MasterAuth for project %s cluster %s location %s", project, cluster, location)
	}
	return createKubeConfig(cl.MasterAuth.ClusterCaCertificate, cl.Endpoint, project, cluster, location)

}

func createKubeConfig(clusterCaCertificate string, endpoint string, project string, cluster string, location string) (string, error) {
	if clusterCaCertificate == "" {
		return "", fmt.Errorf("no ClusterCaCertificate for project %s cluster %s location %s", project, cluster, location)
	}
	if endpoint == "" {
		return "", fmt.Errorf("no Endpoint for project %s cluster %s location %s", project, cluster, location)
	}
	return fmt.Sprintf(kubeConfigTemplate, endpoint, clusterCaCertificate), nil
}

// CreateFactoryFromKubeConfig creates a new connection factory from the given kube config
func CreateFactoryFromKubeConfig(kubeConfig string) (*connector.ConfigClientFactory, error) {
	file, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp file")
	}
	fileName := file.Name()
	err = ioutil.WriteFile(fileName, []byte(kubeConfig), util.DefaultWritePermissions)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to save temp file %s", fileName)
	}
	server := ""
	prefix := "server: "
	lines := strings.Split(kubeConfig, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			server = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			server = strings.TrimPrefix(server, `"`)
			server = strings.TrimSuffix(server, `"`)
			break
		}
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{Precedence: []string{fileName}},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: server}}).ClientConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client-go config for file %s", fileName)
	}
	return connector.NewConfigClientFactory("remote", config), nil
}
