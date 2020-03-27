package opts_test

import (
	"testing"

	"github.com/jenkins-x/jx/pkg/cmd/opts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDomain(t *testing.T) {
	t.Parallel()

	type testData struct {
		Name       string
		Provider   string
		Expected   string
		ExternalIP string
		NodePort   bool
		Resources  []runtime.Object
	}

	ingressNamespace := "nginx"
	ingressService := "nginx-ingress-controller"

	testCases := []testData{
		{
			Name:     "on-premise-nodePort",
			Expected: "35.189.202.25:30123",
			Provider: "kubernetes",
			NodePort: true,
			Resources: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressService,
						Namespace: ingressNamespace,
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								NodePort: 30123,
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: corev1.NodeSpec{},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "1.2.3.4",
							},
							{
								Type:    corev1.NodeExternalIP,
								Address: "35.189.202.25",
							},
						},
					},
				},
			},
		},
		{
			Name:       "on-premise-nodePort-externalIP",
			Expected:   "1.2.3.4:30123",
			ExternalIP: "1.2.3.4",
			Provider:   "kubernetes",
			NodePort:   true,
			Resources: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressService,
						Namespace: ingressNamespace,
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								NodePort: 30123,
							},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
					Spec: corev1.NodeSpec{},
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "1.2.3.4",
							},
							{
								Type:    corev1.NodeExternalIP,
								Address: "35.189.202.25",
							},
						},
					},
				},
			},
		},
		{
			Name:     "gke-loadBalancer",
			Expected: "35.205.151.95.nip.io",
			Provider: "gke",
			Resources: []runtime.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ingressService,
						Namespace: ingressNamespace,
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeLoadBalancer,
					},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "35.205.151.95",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		co := &opts.CommonOptions{}
		co.BatchMode = true

		kubeClient := fake.NewSimpleClientset(tc.Resources...)
		actual, err := co.GetDomain(kubeClient, "", tc.Provider, ingressNamespace, ingressService, tc.ExternalIP, tc.NodePort)
		require.NoError(t, err, "failed to get domain for test %s", tc.Name)

		assert.Equal(t, tc.Expected, actual, "GetDomain for %s", tc.Name)
		t.Logf("test %s generated domain %s", tc.Name, actual)
	}
}
