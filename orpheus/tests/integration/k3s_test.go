package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/k3s"

	"github.com/mathif92/olympus/orpheus/pkg"
)

// waitHealthy polls the provisioner until the target API is reachable; K3s can
// still be coming up right after the container reports ready.
func waitHealthy(t *testing.T, provisioner pkg.Provisioner) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := provisioner.Healthy(ctx); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("provisioner did not become healthy in time")
}

// TestKubeProvisionerRealK3s exercises the real Kubernetes provisioner against
// a live K3s cluster started with testcontainers: namespaces + node-group
// Deployments are created and scaled through the actual Kubernetes API.
func TestKubeProvisionerRealK3s(t *testing.T) {
	if os.Getenv("RUN_K8S_TESTS") == "" {
		t.Skip("set RUN_K8S_TESTS=1 to exercise the real K3s provisioner (needs a privileged Docker daemon)")
	}

	ctx := context.Background()
	k3sContainer, err := k3s.Run(ctx, "rancher/k3s:v1.32.0-k3s1")
	if err != nil {
		t.Fatalf("start k3s container: %v", err)
	}
	defer func() { _ = k3sContainer.Terminate(ctx) }()

	kubeconfig, err := k3sContainer.GetKubeConfig(ctx)
	if err != nil {
		t.Fatalf("read kubeconfig from k3s: %v", err)
	}

	path := filepath.Join(t.TempDir(), "kubeconfig.yaml")
	if err := os.WriteFile(path, kubeconfig, 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	provisioner, err := pkg.NewKubeProvisioner(path)
	if err != nil {
		t.Fatalf("build kube provisioner: %v", err)
	}
	waitHealthy(t, provisioner)

	// Create a cluster (dedicated namespace on the real API server).
	provisioned, err := provisioner.CreateCluster(ctx, pkg.ClusterSpec{Name: "eks-prod"})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if provisioned.ProviderRef == "" || provisioned.Endpoint == "" || provisioned.CAData == "" {
		t.Fatalf("expected full provisioned cluster: %+v", provisioned)
	}

	// Create a node group → Deployment with desired replicas.
	ng, err := provisioner.CreateNodeGroup(ctx, pkg.NodeGroupSpec{
		ClusterRef:  provisioned.ProviderRef,
		Name:        "workers",
		NodeSize:    "olympus-small",
		MinSize:     1,
		DesiredSize: 2,
		MaxSize:     4,
	})
	if err != nil {
		t.Fatalf("create node group: %v", err)
	}
	if ng.ProviderRef == "" {
		t.Fatal("expected node group provider ref")
	}

	// Scale the node group up through the real API.
	if err := provisioner.ScaleNodeGroup(ctx, pkg.NodeGroupSpec{
		ClusterRef:  provisioned.ProviderRef,
		Name:        "workers",
		DesiredSize: 3,
	}); err != nil {
		t.Fatalf("scale node group: %v", err)
	}

	// Cleanup through the real API.
	if err := provisioner.DeleteNodeGroup(ctx, pkg.NodeGroupSpec{ClusterRef: ng.ProviderRef}); err != nil {
		t.Fatalf("delete node group: %v", err)
	}
	if err := provisioner.DeleteCluster(ctx, provisioned.ProviderRef); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}
}
