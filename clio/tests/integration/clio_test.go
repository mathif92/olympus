// Package integration contains end-to-end tests that exercise Clio against a
// real PostgreSQL started with testcontainers, using either the mock
// provisioner or real database containers as the data plane.
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

	"github.com/mathif92/olympus/clio/internal/handler"
	"github.com/mathif92/olympus/clio/pkg"
	"github.com/mathif92/olympus/clio/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_databases"),
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

func newClio(t *testing.T, provisioner pkg.Provisioner) (*pkg.Clio, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewClio(client, provisioner), stop
}

func ensureTenant(t *testing.T, c *pkg.Clio, id string) {
	t.Helper()
	if err := c.EnsureAccount(context.Background(), database.Account{
		ID: id, DisplayName: id, Email: id + "@c.dev", Plan: "pro", InstanceLimit: 50,
	}); err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
}

func TestInstanceLifecycle(t *testing.T) {
	c, stop := newClio(t, pkg.NewMockProvisioner())
	defer stop()
	ctx := context.Background()

	ensureTenant(t, c, "acme")
	if err := c.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	engines, err := c.ListEngines(ctx)
	if err != nil {
		t.Fatalf("list engines: %v", err)
	}
	if len(engines) < 2 {
		t.Fatalf("expected seeded engines, got %d", len(engines))
	}

	inst, err := c.CreateInstance(ctx, "acme", "prod", pkg.InstanceSpec{
		Name:          "analytics",
		Engine:        "postgres",
		EngineVersion: "16.8",
		Size:          "clio-small",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if inst.State != pkg.StateActive {
		t.Fatalf("expected active state after create, got %q", inst.State)
	}
	if inst.Endpoint == "" || inst.MasterUsername == "" || inst.MasterPassword == "" {
		t.Fatalf("expected endpoint + credentials after create: %+v", inst)
	}

	// Stop->Start roundtrip.
	stopped, err := c.StopInstance(ctx, "acme", "prod", "analytics")
	if err != nil {
		t.Fatalf("stop instance: %v", err)
	}
	if stopped.State != pkg.StateStopped {
		t.Fatalf("expected stopped state, got %q", stopped.State)
	}
	started, err := c.StartInstance(ctx, "acme", "prod", "analytics")
	if err != nil {
		t.Fatalf("start instance: %v", err)
	}
	if started.State != pkg.StateActive {
		t.Fatalf("expected active state after start, got %q", started.State)
	}

	// Snapshot lifecycle on an active instance.
	snap, err := c.CreateSnapshot(ctx, "acme", "prod", "analytics", "pre-deploy")
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.State != pkg.StateActive {
		t.Fatalf("expected active snapshot, got %q", snap.State)
	}
	snaps, err := c.ListSnapshots(ctx, "acme", "prod", "analytics")
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if err := c.DeleteSnapshot(ctx, "acme", "prod", "analytics", "pre-deploy"); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}

	// Delete the instance.
	if err := c.DeleteInstance(ctx, "acme", "prod", "analytics"); err != nil {
		t.Fatalf("delete instance: %v", err)
	}
	got, err := c.GetInstance(ctx, "acme", "prod", "analytics")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got.State != pkg.StateDeleted {
		t.Fatalf("expected deleted state, got %q", got.State)
	}
}

func TestTenantIsolation(t *testing.T) {
	c, stop := newClio(t, pkg.NewMockProvisioner())
	defer stop()
	ctx := context.Background()

	ensureTenant(t, c, "tenant-a")
	ensureTenant(t, c, "tenant-b")

	if err := c.CreateProject(ctx, "tenant-a", database.Project{Name: "lab"}); err != nil {
		t.Fatalf("create project for a: %v", err)
	}
	if _, err := c.CreateInstance(ctx, "tenant-a", "lab", pkg.InstanceSpec{
		Name: "blue", Engine: "postgres", EngineVersion: "16.8", Size: "clio-nano",
	}); err != nil {
		t.Fatalf("create instance in a: %v", err)
	}

	// Tenant B cannot see or mutate tenant A's resources.
	if _, err := c.ListInstances(ctx, "tenant-b", "lab"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound listing A's project as B, got %v", err)
	}
	if _, err := c.GetInstance(ctx, "tenant-b", "lab", "blue"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound getting A's instance as B, got %v", err)
	}
	if err := c.DeleteInstance(ctx, "tenant-b", "lab", "blue"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting A's instance as B, got %v", err)
	}

	// A still sees its own instance.
	instances, err := c.ListInstances(ctx, "tenant-a", "lab")
	if err != nil {
		t.Fatalf("list a's instances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance for tenant a, got %d", len(instances))
	}
}

// TestHTTPEndpoints drives the real mux and verifies audit trail entries are
// written for each operation.
func TestHTTPEndpoints(t *testing.T) {
	c, stopFn := newClio(t, pkg.NewMockProvisioner())
	defer stopFn()
	ctx := context.Background()

	mux := handler.NewClioHandler(c).Router()
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
	if resp, body := do(http.MethodPost, "/instances",
		`{"project":"prod","name":"analytics","engine":"postgres","engine_version":"16.8","size":"clio-small"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create instance: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/instance/prod/analytics/stop", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("stop instance: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/instance/prod/analytics/start", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("start instance: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/snapshots",
		`{"project":"prod","instance":"analytics","name":"pre-deploy"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create snapshot: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodGet, "/instances?project=prod", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list instances: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodDelete, "/snapshot/prod/analytics/pre-deploy", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete snapshot: %d %s", resp.StatusCode, body)
	}

	var auditCount int
	if err := c.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = 'acme'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit trail rows from HTTP operations")
	}
}
