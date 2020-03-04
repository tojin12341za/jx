package applications

import (
	"io/ioutil"

	v1 "github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/pkg/cloud"
	"github.com/jenkins-x/jx/pkg/cmd/clients"
	"github.com/jenkins-x/jx/pkg/config"
	"github.com/jenkins-x/jx/pkg/flagger"
	"github.com/jenkins-x/jx/pkg/gits"
	"github.com/jenkins-x/jx/pkg/kube"
	"github.com/jenkins-x/jx/pkg/kube/naming"
	"github.com/jenkins-x/jx/pkg/kube/services"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Deployment represents an application deployment in a single environment
type Deployment struct {
	*appsv1.Deployment
}

// Environment represents an environment in which an application has been
// deployed
type Environment struct {
	v1.Environment
	Deployments []Deployment
}

// Application represents an application in jx
type Application struct {
	*v1.SourceRepository
	Environments map[string]Environment
}

// List is a collection of applications
type List struct {
	Items []Application
}

// Environments loops through all applications in a list and returns a map with
// all the unique environments
func (l List) Environments() map[string]v1.Environment {
	envs := make(map[string]v1.Environment)

	for _, a := range l.Items {
		for name, env := range a.Environments {
			if _, ok := envs[name]; !ok {
				envs[name] = env.Environment
			}
		}
	}

	return envs
}

// Name returns the application name
func (a Application) Name() string {
	return naming.ToValidName(a.SourceRepository.Spec.Repo)
}

// IsPreview returns true if the environment is a preview environment
func (e Environment) IsPreview() bool {
	return e.Environment.Spec.Kind == v1.EnvironmentKindTypePreview
}

// Version returns the deployment version
func (d Deployment) Version() string {
	return kube.GetVersion(&d.Deployment.ObjectMeta)
}

// Pods returns the ratio of pods that are ready/replicas
func (d Deployment) Pods() string {
	pods := ""
	ready := d.Deployment.Status.ReadyReplicas

	if d.Deployment.Spec.Replicas != nil && ready > 0 {
		replicas := util.Int32ToA(*d.Deployment.Spec.Replicas)
		pods = util.Int32ToA(ready) + "/" + replicas
	}

	return pods
}

// URL returns a deployment URL
func (d Deployment) URL(kc kubernetes.Interface, a Application) string {
	url, _ := services.FindServiceURL(kc, d.Deployment.Namespace, a.Name())
	return url
}

// GetApplications fetches all Apps
func GetApplications(factory clients.Factory) (List, error) {
	list := List{
		Items: make([]Application, 0),
	}

	client, namespace, err := factory.CreateJXClient()
	if err != nil {
		return list, errors.Wrap(err, "failed to create a jx client from applications.GetApplications")
	}

	// fetch ALL repositories
	srList, err := client.JenkinsV1().SourceRepositories(namespace).List(metav1.ListOptions{})
	if err != nil {
		return list, errors.Wrapf(err, "failed to find any SourceRepositories in namespace %s", namespace)
	}

	// fetch all environments
	envMap, _, err := kube.GetOrderedEnvironments(client, namespace)
	if err != nil {
		return list, errors.Wrapf(err, "failed to fetch environments in namespace %s", namespace)
	}

	// only keep permanent environments
	permanentEnvsMap := map[string]*v1.Environment{}
	for _, env := range envMap {
		if env.Spec.Kind.IsPermanent() {
			permanentEnvsMap[env.Spec.Namespace] = env
		}
	}

	// copy repositories that aren't environments to our applications list
	for _, sr := range srList.Items {
		if !kube.IsIncludedInTheGivenEnvs(permanentEnvsMap, &sr) {
			srCopy := sr
			list.Items = append(list.Items, Application{&srCopy, make(map[string]Environment)})
		}
	}

	kubeClient, _, err := factory.CreateKubeClient()

	// fetch deployments by environment (excluding dev)
	deployments := make(map[string]map[string]appsv1.Deployment)
	for _, env := range permanentEnvsMap {
		if env.Spec.Kind != v1.EnvironmentKindTypeDevelopment {
			var envDeployments map[string]appsv1.Deployment
			if env.Spec.RemoteCluster {
				envDeployments, err = GetRemoteDeployments(factory, env)
				if err != nil {
					return list, err
				}
			} else {
				envDeployments, err = kube.GetDeployments(kubeClient, env.Spec.Namespace)
				if err != nil {
					return list, err
				}
			}
			deployments[env.Spec.Namespace] = envDeployments
		}
	}

	err = list.appendMatchingDeployments(permanentEnvsMap, deployments)
	if err != nil {
		return list, err
	}

	return list, nil
}

// GetRemoteDeployments finds the remote cluster's
func GetRemoteDeployments(factory clients.Factory, env *v1.Environment) (map[string]appsv1.Deployment, error) {
	gitURL := env.Spec.Source.URL
	if gitURL == "" {
		log.Logger().Warnf("environment %s does not have a git source URL", env.Name)
		return nil, nil
	}
	requirements, err := GetRequirementsFromGit(gitURL)
	if err != nil {
		return nil, err
	}

	ns := requirements.Cluster.Namespace
	if ns == "" {
		ns = env.Spec.Namespace
		if ns == "" {
			ns = "jx"
		}
	}

	kubeClient, err := getKubeClientFromRequirements(requirements, factory)
	if err != nil {
		return nil, err
	}
	if kubeClient == nil {
		log.Logger().Warnf("remote connection to environment %s not supported for provider %s", env.Name, requirements.Cluster.Provider)
		return nil, nil
	}
	return kube.GetDeployments(kubeClient, env.Spec.Namespace)
}

func getKubeClientFromRequirements(requirements *config.RequirementsConfig, factory clients.Factory) (kubernetes.Interface, error) {
	if requirements.Cluster.Provider == cloud.GKE {
		project := requirements.Cluster.ProjectID
		clusterName := requirements.Cluster.ClusterName
		zone := requirements.Cluster.Zone
		if project == "" {
			log.Logger().Warnf("requirements missing cluster.project")
			return nil, nil
		}
		if clusterName == "" {
			log.Logger().Warnf("requirements missing cluster.clusterName")
			return nil, nil
		}
		if zone == "" {
			log.Logger().Warnf("requirements missing cluster.zone")
			return nil, nil
		}
		kubeConfig, err := GetWorkspaceKubeConfig(true, project, clusterName, "", zone)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create KubeConfig for project %s cluster %s zone %s", project, clusterName, zone)
		}

		factory, err := CreateFactoryFromKubeConfig(kubeConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create kube client factory for project %s cluster %s zone %s", project, clusterName, zone)
		}
		return factory.CreateKubeClient()
	}
	return nil, nil
}

// GetRequirementsFromGit clones the given git repository to get the requirements
func GetRequirementsFromGit(gitURL string) (*config.RequirementsConfig, error) {
	tempDir, err := ioutil.TempDir("", "jx-boot-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}

	log.Logger().Infof("cloning %s to %s", gitURL, tempDir)

	gitter := gits.NewGitCLI()
	err = gitter.Clone(gitURL, tempDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to git clone %s to dir %s", gitURL, tempDir)
	}

	requirements, _, err := config.LoadRequirementsConfig(tempDir)
	if err != nil {
		return requirements, errors.Wrapf(err, "failed to requirements YAML file from %s", tempDir)
	}
	return requirements, nil
}

func getDeploymentAppNameInEnvironment(d appsv1.Deployment, e *v1.Environment) (string, error) {
	labels, err := metav1.LabelSelectorAsMap(d.Spec.Selector)
	if err != nil {
		return "", err
	}

	name := kube.GetAppName(labels["app"], e.Spec.Namespace)
	return name, nil
}

func (l List) appendMatchingDeployments(envs map[string]*v1.Environment, deps map[string]map[string]appsv1.Deployment) error {
	for _, app := range l.Items {
		for envName, env := range envs {
			for _, dep := range deps[envName] {
				depAppName, err := getDeploymentAppNameInEnvironment(dep, env)
				if err != nil {
					return errors.Wrap(err, "getting app name")
				}
				if depAppName == app.Name() && !flagger.IsCanaryAuxiliaryDeployment(dep) {
					depCopy := dep
					app.Environments[env.Name] = Environment{
						*env,
						[]Deployment{{&depCopy}},
					}
				}
			}
		}
	}

	return nil
}
