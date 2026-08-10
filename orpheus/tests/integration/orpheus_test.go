// Package integration contains end-to-end tests that exercise Orpheus
// against a real PostgreSQL started with testcontainers, using either the mock
// provisioner or a real K3s cluster as the data plane.
package integration

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mathif92/olympus/orpheus/internal/handler"
	"github.com/mathif92/olympus/orpheus/pkg"
	"github.com/mathif92/olympus/orpheus/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_orchestration"),
		postgres.WithUsername("olympus"),
		postgres.WithPassword("olympus_secret"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	url, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	client, err := database.NewClient(database.Config{
		PostgresURL: url,
		PoolMax:     10,
		PoolMin:     2,
		PoolTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new database client: %v", err)
	}

	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	if err := database.Migrate(client.DB, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	stop := func() {
		client.Close()
		_ = pg.Terminate(context.Background())
	}
	return client, stop
}

func newOrpheus(t *testing.T, provisioner pkg.Provisioner) (*pkg.Orpheus, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewOrpheus(client, provisioner), stop
}

func ensureTenant(t *testing.T, o *pkg.Orpheus, id string) {
	t.Helper()
	if err := o.EnsureAccount(context.Background(), database.Account{
		ID: id, DisplayName: id, Email: id + "@or.dev", Plan: "pro", ClusterLimit: 50,
	}); err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
}

func TestClusterLifecycle(t *testing.T) {
	o, stop := newOrpheus(t, pkg.NewMockProvisioner())
	defer stop()
	ctx := context.Background()

	ensureTenant(t, o, "acme")
	if err := o.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	versions, err := o.ListKubernetesVersions(ctx)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected seeded kubernetes versions, got %d", len(versions))
	}

	cluster, err := o.CreateCluster(ctx, "acme", "prod", pkg.ClusterSpec{
		Name: "eks-prod", KubernetesVersion: "1.30",
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if cluster.State != pkg.StateActive {
		t.Fatalf("expected active state after create, got %q", cluster.State)
	}
	if cluster.Endpoint == "" || cluster.Kubeconfig == "" {
		t.Fatalf("expected endpoint + kubeconfig after create: %+v", cluster)
	}

	// Node group lifecycle: create on the active cluster.
	ng, err := o.CreateNodeGroup(ctx, "acme", "prod", "eks-prod", pkg.NodeGroupSpec{
		Name: "workers", NodeSize: "olympus-small", MinSize: 1, DesiredSize: 2, MaxSize: 4,
	})
	if err != nil {
		t.Fatalf("create node group: %v", err)
	}
	if ng.State != pkg.StateActive || ng.DesiredSize != 2 {
		t.Fatalf("unexpected node group: %+v", ng)
	}

	// Scale within bounds.
	scaled, err := o.ScaleNodeGroup(ctx, "acme", "prod", "eks-prod", "workers", 3)
	if err != nil {
		t.Fatalf("scale node group: %v", err)
	}
	if scaled.DesiredSize != 3 {
		t.Fatalf("expected desired 3 after scale, got %d", scaled.DesiredSize)
	}

	// Scale out of bounds is rejected.
	if _, err := o.ScaleNodeGroup(ctx, "acme", "prod", "eks-prod", "workers", 7); err == nil {
		t.Fatal("expected error scaling above max")
	}

	// Kubeconfig roundtrip.
	kc, err := o.ClusterKubeconfig(ctx, "acme", "prod", "eks-prod")
	if err != nil {
		t.Fatalf("get kubeconfig: %v", err)
	}
	if kc == "" {
		t.Fatal("expected non-empty kubeconfig")
	}

	// Delete the node group then the cluster.
	if err := o.DeleteNodeGroup(ctx, "acme", "prod", "eks-prod", "workers"); err != nil {
		t.Fatalf("delete node group: %v", err)
	}
	if err := o.DeleteCluster(ctx, "acme", "prod", "eks-prod"); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}
	got, err := o.GetCluster(ctx, "acme", "prod", "eks-prod")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got.State != pkg.StateDeleted {
		t.Fatalf("expected deleted state, got %q", got.State)
	}
}

func TestTenantIsolation(t *testing.T) {
	o, stop := newOrpheus(t, pkg.NewMockProvisioner())
	defer stop()
	ctx := context.Background()

	ensureTenant(t, o, "tenant-a")
	ensureTenant(t, o, "tenant-b")

	if err := o.CreateProject(ctx, "tenant-a", database.Project{Name: "lab"}); err != nil {
		t.Fatalf("create project for a: %v", err)
	}
	if _, err := o.CreateCluster(ctx, "tenant-a", "lab", pkg.ClusterSpec{
		Name: "blue", KubernetesVersion: "1.30",
	}); err != nil {
		t.Fatalf("create cluster in a: %v", err)
	}

	// Tenant B cannot see or mutate tenant A's resources.
	if _, err := o.ListClusters(ctx, "tenant-b", "lab"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound listing A's project as B, got %v", err)
	}
	if _, err := o.GetCluster(ctx, "tenant-b", "lab", "blue"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound getting A's cluster as B, got %v", err)
	}
	if err := o.DeleteCluster(ctx, "tenant-b", "lab", "blue"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting A's cluster as B, got %v", err)
	}

	// A still sees its own cluster.
	clusters, err := o.ListClusters(ctx, "tenant-a", "lab")
	if err != nil {
		t.Fatalf("list a's clusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster for tenant a, got %d", len(clusters))
	}
}

// TestHTTPEndpoints drives the real mux and verifies audit trail entries are
// written for each operation.
func TestHTTPEndpoints(t *testing.T) {
	o, stopFn := newOrpheus(t, pkg.NewMockProvisioner())
	defer stopFn()
	ctx := context.Background()

	mux := handler.NewOrpheusHandler(o).Router()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	do := func(method, path, body string) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Account-Id", "acme")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp, string(data)
	}

	if resp, body := do(http.MethodPost, "/projects", `{"name":"prod"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/clusters",
		`{"project":"prod","name":"eks-prod","kubernetes_version":"1.30"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create cluster: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/nodegroups",
		`{"project":"prod","cluster":"eks-prod","name":"workers","node_size":"olympus-small","min_size":1,"desired_size":2,"max_size":4}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create node group: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/nodegroup/prod/eks-prod/workers/scale", `{"desired_size":3}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("scale node group: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/clusters?project=prod", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list clusters: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/cluster/prod/eks-prod/kubeconfig", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("get kubeconfig: %d %s", resp.StatusCode, body)
	}

	var auditCount int
	if err := o.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = 'acme'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit trail rows from HTTP operations")
	}
}
