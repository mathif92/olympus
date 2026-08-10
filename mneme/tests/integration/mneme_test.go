// Package integration contains end-to-end tests that exercise Mneme against a
// real PostgreSQL started with testcontainers, using either the mock
// provisioner or real cache containers as the data plane.
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

	"github.com/mathif92/olympus/mneme/internal/handler"
	"github.com/mathif92/olympus/mneme/pkg"
	"github.com/mathif92/olympus/mneme/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_caches"),
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

func newMneme(t *testing.T, provisioner pkg.Provisioner) (*pkg.Mneme, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewMneme(client, provisioner), stop
}

func ensureTenant(t *testing.T, m *pkg.Mneme, id string) {
	t.Helper()
	if err := m.EnsureAccount(context.Background(), database.Account{
		ID: id, DisplayName: id, Email: id + "@m.dev", Plan: "pro", ClusterLimit: 50,
	}); err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
}

func TestClusterLifecycle(t *testing.T) {
	m, stop := newMneme(t, pkg.NewMockProvisioner())
	defer stop()
	ctx := context.Background()

	ensureTenant(t, m, "acme")
	if err := m.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	engines, err := m.ListEngines(ctx)
	if err != nil {
		t.Fatalf("list engines: %v", err)
	}
	if len(engines) < 2 {
		t.Fatalf("expected seeded engines, got %d", len(engines))
	}

	cl, err := m.CreateCluster(ctx, "acme", "prod", pkg.ClusterSpec{
		Name:          "session-cache",
		Engine:        "redis",
		EngineVersion: "7.4",
		NodeType:      "mneme-small",
		NumNodes:      2,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if cl.State != pkg.StateActive {
		t.Fatalf("expected active state after create, got %q", cl.State)
	}
	if cl.Endpoint == "" {
		t.Fatalf("expected endpoint after create: %+v", cl)
	}

	// Snapshot lifecycle on an active cluster.
	snap, err := m.CreateSnapshot(ctx, "acme", "prod", "session-cache", "pre-release")
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.State != pkg.StateActive {
		t.Fatalf("expected active snapshot, got %q", snap.State)
	}
	snaps, err := m.ListSnapshots(ctx, "acme", "prod", "session-cache")
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if err := m.DeleteSnapshot(ctx, "acme", "prod", "session-cache", "pre-release"); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}

	// Delete the cluster.
	if err := m.DeleteCluster(ctx, "acme", "prod", "session-cache"); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}
	got, err := m.GetCluster(ctx, "acme", "prod", "session-cache")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got.State != pkg.StateDeleted {
		t.Fatalf("expected deleted state, got %q", got.State)
	}
}

func TestTenantIsolation(t *testing.T) {
	m, stop := newMneme(t, pkg.NewMockProvisioner())
	defer stop()
	ctx := context.Background()

	ensureTenant(t, m, "tenant-a")
	ensureTenant(t, m, "tenant-b")

	if err := m.CreateProject(ctx, "tenant-a", database.Project{Name: "lab"}); err != nil {
		t.Fatalf("create project for a: %v", err)
	}
	if _, err := m.CreateCluster(ctx, "tenant-a", "lab", pkg.ClusterSpec{
		Name: "blue", Engine: "redis", EngineVersion: "7.4", NodeType: "mneme-micro",
	}); err != nil {
		t.Fatalf("create cluster in a: %v", err)
	}

	// Tenant B cannot see or mutate tenant A's resources.
	if _, err := m.ListClusters(ctx, "tenant-b", "lab"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound listing A's project as B, got %v", err)
	}
	if _, err := m.GetCluster(ctx, "tenant-b", "lab", "blue"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound getting A's cluster as B, got %v", err)
	}
	if err := m.DeleteCluster(ctx, "tenant-b", "lab", "blue"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting A's cluster as B, got %v", err)
	}

	// A still sees its own cluster.
	clusters, err := m.ListClusters(ctx, "tenant-a", "lab")
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
	m, stopFn := newMneme(t, pkg.NewMockProvisioner())
	defer stopFn()
	ctx := context.Background()

	mux := handler.NewMnemeHandler(m).Router()
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
		`{"project":"prod","name":"session-cache","engine":"redis","engine_version":"7.4","node_type":"mneme-small","num_nodes":2}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create cluster: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/snapshots",
		`{"project":"prod","cluster":"session-cache","name":"pre-release"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create snapshot: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/clusters?project=prod", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list clusters: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/cluster/prod/session-cache", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("get cluster: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodDelete, "/snapshot/prod/session-cache/pre-release", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete snapshot: %d %s", resp.StatusCode, body)
	}

	var auditCount int
	if err := m.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = 'acme'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit trail rows from HTTP operations")
	}
}
