package applications

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/jenkins-x/jx/pkg/jxfactory/connector"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/errors"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// GetWorkspaceKubeConfigGKE returns the GKE kube config
func GetWorkspaceKubeConfigGKE(useGcloud bool, project string, cluster string, region string, zone string) (string, error) {
	jxDir, err := util.ConfigDir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get jx home dir")
	}

	clusterDir := filepath.Join(jxDir, "kubeconfig", "gke", project, cluster)

	location := ""
	args := []string{"container", "clusters", "get-credentials", cluster, "--project", project}
	if region != "" {
		location = region
		args = append(args, "--region", region)
		clusterDir = filepath.Join(clusterDir, "region", region)
	} else {
		location = zone
		args = append(args, "--zone", zone)
		clusterDir = filepath.Join(clusterDir, "zone", zone)
	}

	err = os.MkdirAll(clusterDir, util.DefaultWritePermissions)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create kubeconfig dir %s", clusterDir)
	}

	kubeEnvVar := filepath.Join(clusterDir, "config")
	cmd := util.Command{
		Name: "gcloud",
		Args: args,
		Env: map[string]string{
			"KUBECONFIG": kubeEnvVar,
		},
	}
	_, err = cmd.RunWithoutRetry()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get cluster credentials information for project %s cluster %s location %s", project, cluster, location)
	}
	data, err := ioutil.ReadFile(kubeEnvVar)
	if err != nil {
		return "", errors.Wrapf(err, "failed to load cluster information from %s", kubeEnvVar)
	}
	return string(data), nil
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
