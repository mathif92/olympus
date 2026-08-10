package pkg

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeProvisioner provisions Orpheus clusters inside a real Kubernetes API
// server (selected via KUBECONFIG). Each managed "cluster" becomes a dedicated
// namespace, and each node group becomes a Deployment whose replica count is
// the desired node count — so control-plane metadata, namespaces and scale
// operations all hit the real Kubernetes API.
type KubeProvisioner struct {
	client kubernetes.Interface
	// targetHost and targetCAData are the API-server endpoint and CA of the
	// target cluster, surfaced back through the generated kubeconfigs.
	targetHost   string
	targetCAData []byte
}

// NewKubeProvisioner builds a provisioner from a kubeconfig file path.
func NewKubeProvisioner(kubeconfigPath string) (*KubeProvisioner, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	p := &KubeProvisioner{
		client:       client,
		targetHost:   restCfg.Host,
		targetCAData: restCfg.CAData,
	}
	return p, nil
}

func clusterNamespace(name string) string {
	return "or-" + sanitize(name)
}

func nodeGroupDeploymentName(name string) string {
	return "ng-" + sanitize(name)
}

func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// CreateCluster creates a dedicated namespace on the target cluster.
func (p *KubeProvisioner) CreateCluster(ctx context.Context, spec ClusterSpec) (*ProvisionedCluster, error) {
	ns := clusterNamespace(spec.Name)
	_, err := p.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
			Labels: map[string]string{
				"managed-by": "orpheus",
				"cluster":    sanitize(spec.Name),
			},
		},
	}, metav1.CreateOptions{})
	if err != nil && !isAlreadyExists(err) {
		return nil, fmt.Errorf("create namespace %q: %w", ns, err)
	}
	p.WaitForNamespaceReady(ctx, ns, 15*time.Second)
	ca := base64.StdEncoding.EncodeToString(p.targetCAData)
	return &ProvisionedCluster{ProviderRef: ns, Endpoint: p.targetHost, CAData: ca}, nil
}

// DeleteCluster removes the dedicated namespace and everything in it.
func (p *KubeProvisioner) DeleteCluster(ctx context.Context, providerRef string) error {
	if providerRef == "" || providerRef == "default" || providerRef == "kube-system" {
		return fmt.Errorf("refusing to delete protected namespace %q", providerRef)
	}
	if err := p.client.CoreV1().Namespaces().Delete(ctx, providerRef, metav1.DeleteOptions{}); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete namespace %q: %w", providerRef, err)
	}
	return nil
}

// CreateNodeGroup creates a Deployment in the cluster's namespace with the
// desired number of replicas as the worker nodes.
func (p *KubeProvisioner) CreateNodeGroup(ctx context.Context, spec NodeGroupSpec) (*ProvisionedNodeGroup, error) {
	ns := spec.ClusterRef
	name := nodeGroupDeploymentName(spec.Name)
	replicas := int32(spec.DesiredSize)
	_, err := p.client.AppsV1().Deployments(ns).Create(ctx, nodeGroupDeployment(ns, name, replicas), metav1.CreateOptions{})
	if err != nil && !isAlreadyExists(err) {
		return nil, fmt.Errorf("create deployment %q: %w", name, err)
	}
	return &ProvisionedNodeGroup{ProviderRef: ns + "/" + name}, nil
}

// ScaleNodeGroup updates an existing Deployment's replica count.
func (p *KubeProvisioner) ScaleNodeGroup(ctx context.Context, spec NodeGroupSpec) error {
	ns := spec.ClusterRef
	name := nodeGroupDeploymentName(spec.Name)
	replicas := int32(spec.DesiredSize)

	got, err := p.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment %q: %w", name, err)
	}
	got.Spec.Replicas = &replicas
	if _, err := p.client.AppsV1().Deployments(ns).Update(ctx, got, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("scale deployment %q: %w", name, err)
	}
	return nil
}

// DeleteNodeGroup removes the node-group Deployment.
func (p *KubeProvisioner) DeleteNodeGroup(ctx context.Context, spec NodeGroupSpec) error {
	ns, name := splitRef(spec.ClusterRef)
	if ns == "" || name == "" {
		return fmt.Errorf("malformed node-group reference %q", spec.ClusterRef)
	}
	if err := p.client.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete deployment %q: %w", name, err)
	}
	return nil
}

// Healthy verifies the target API server responds.
func (p *KubeProvisioner) Healthy(ctx context.Context) error {
	_, err := p.client.Discovery().ServerVersion()
	return err
}

// WaitForNamespaceReady waits up to d for a namespace to exist.
func (p *KubeProvisioner) WaitForNamespaceReady(ctx context.Context, ns string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := p.client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("timed out waiting for namespace " + ns)
}

func nodeGroupDeployment(ns, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "worker",
							Image: "registry.k8s.io/pause:3.9",
							Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt32(8080),
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resourceMustParse("100m"),
									corev1.ResourceMemory: resourceMustParse("64Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func splitRef(ref string) (ns, name string) {
	ns, name, ok := strings.Cut(ref, "/")
	if !ok {
		return "", ""
	}
	return ns, name
}

func isAlreadyExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "AlreadyExists")
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}
