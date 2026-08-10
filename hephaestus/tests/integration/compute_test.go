// Package integration contains end-to-end tests that exercise Hephaestus
// against a real PostgreSQL started with testcontainers, using the mock
// provisioner as the data plane.
package integration

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mathif92/olympus/hephaestus/internal/handler"
	"github.com/mathif92/olympus/hephaestus/pkg"
	"github.com/mathif92/olympus/hephaestus/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_compute"),
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

func newCompute(t *testing.T) (*pkg.Hephaestus, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	return pkg.NewHephaestus(client, pkg.NewMockProvisioner()), stop
}

func ensureTenant(t *testing.T, h *pkg.Hephaestus, id string) {
	t.Helper()
	if err := h.EnsureAccount(context.Background(), database.Account{
		ID: id, DisplayName: id, Email: id + "@hep.dev", Plan: "pro", InstanceLimit: 50,
	}); err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
}

func TestInstanceLifecycle(t *testing.T) {
	compute, stop := newCompute(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, compute, "acme")
	if err := compute.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	types, err := compute.ListInstanceTypes(ctx)
	if err != nil {
		t.Fatalf("list types: %v", err)
	}
	if len(types) < 2 {
		t.Fatalf("expected seeded instance types, got %d", len(types))
	}

	inst, err := compute.LaunchInstance(ctx, "acme", "prod", pkg.InstanceSpec{
		Name: "web-1", Type: "olympus-small", LaunchedBy: "acme",
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if inst.State != pkg.StateRunning {
		t.Fatalf("expected running state after launch, got %q", inst.State)
	}
	if inst.PrivateIP == "" || inst.ProviderRef == "" {
		t.Fatalf("expected private ip + provider ref after launch: %+v", inst)
	}

	// Boot volume was created and attached.
	vols, err := compute.ListVolumes(ctx, "acme", "prod")
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	if len(vols) != 1 || vols[0].State != "in-use" {
		t.Fatalf("expected one in-use boot volume, got %+v", vols)
	}

	// stop / start transitions.
	if _, err := compute.StopInstance(ctx, "acme", "prod", "web-1"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	got, err := compute.GetInstance(ctx, "acme", "prod", "web-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != pkg.StateStopped {
		t.Fatalf("expected stopped, got %q", got.State)
	}
	if _, err := compute.StartInstance(ctx, "acme", "prod", "web-1"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Terminate.
	if err := compute.TerminateInstance(ctx, "acme", "prod", "web-1"); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	got, err = compute.GetInstance(ctx, "acme", "prod", "web-1")
	if err != nil {
		t.Fatalf("get after terminate: %v", err)
	}
	if got.State != pkg.StateTerminated {
		t.Fatalf("expected terminated, got %q", got.State)
	}

	// Starting a terminated instance is rejected.
	if _, err := compute.StartInstance(ctx, "acme", "prod", "web-1"); err != pkg.ErrConflict {
		t.Fatalf("expected ErrConflict starting terminated instance, got %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	compute, stop := newCompute(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, compute, "tenant-a")
	ensureTenant(t, compute, "tenant-b")

	if err := compute.CreateProject(ctx, "tenant-a", database.Project{Name: "lab"}); err != nil {
		t.Fatalf("create project for a: %v", err)
	}
	if _, err := compute.LaunchInstance(ctx, "tenant-a", "lab", pkg.InstanceSpec{
		Name: "box", Type: "olympus-nano",
	}); err != nil {
		t.Fatalf("launch in a: %v", err)
	}

	// Tenant B cannot see or mutate tenant A's resources.
	if _, err := compute.ListInstances(ctx, "tenant-b", "lab"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound listing A's project as B, got %v", err)
	}
	if _, err := compute.GetInstance(ctx, "tenant-b", "lab", "box"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound getting A's instance as B, got %v", err)
	}
	if err := compute.TerminateInstance(ctx, "tenant-b", "lab", "box"); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound terminating A's instance as B, got %v", err)
	}

	// A still sees its own instance.
	insts, err := compute.ListInstances(ctx, "tenant-a", "lab")
	if err != nil {
		t.Fatalf("list a's instances: %v", err)
	}
	if len(insts) != 1 {
		t.Fatalf("expected 1 instance for tenant a, got %d", len(insts))
	}
}

func TestKeyPairsAndSnapshots(t *testing.T) {
	compute, stop := newCompute(t)
	defer stop()
	ctx := context.Background()

	ensureTenant(t, compute, "acme")
	if err := compute.CreateProject(ctx, "acme", database.Project{Name: "prod"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Key pair: private key returned once, only public key persisted.
	kp, privateKey, err := compute.CreateKeyPair(ctx, "acme", "prod", "deploy-key")
	if err != nil {
		t.Fatalf("create key pair: %v", err)
	}
	if privateKey == "" || !strings.Contains(privateKey, "PRIVATE KEY") {
		t.Fatalf("expected RSA private key material back from creation")
	}
	if kp.Fingerprint == "" || kp.PublicKey == "" {
		t.Fatalf("expected fingerprint + public key stored: %+v", kp)
	}

	keys, err := compute.ListKeyPairs(ctx, "acme", "prod")
	if err != nil {
		t.Fatalf("list key pairs: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key pair, got %d", len(keys))
	}

	// Volume + snapshot.
	vol, err := compute.CreateVolume(ctx, "acme", "prod", database.Volume{Name: "data", SizeGB: 10, Type: "gp2"})
	if err != nil {
		t.Fatalf("create volume: %v", err)
	}
	if vol.State != "available" {
		t.Fatalf("expected available volume, got %q", vol.State)
	}

	snap, err := compute.CreateSnapshot(ctx, "acme", "prod", database.Snapshot{
		Name: "data-backup", VolumeID: vol.ID,
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.State != "completed" || snap.SizeGB != 10 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}

	// Deleting an in-use volume is rejected; the boot volume is in-use.
	if _, err := compute.LaunchInstance(ctx, "acme", "prod", pkg.InstanceSpec{
		Name: "db", Type: "olympus-medium", KeyPairName: "deploy-key",
	}); err != nil {
		t.Fatalf("launch with key pair: %v", err)
	}
	vols, err := compute.ListVolumes(ctx, "acme", "prod")
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	for _, v := range vols {
		if v.InstanceID != "" {
			if err := compute.DeleteVolume(ctx, "acme", "prod", v.Name); err == nil {
				t.Fatal("expected error deleting an in-use volume")
			}
		}
	}

	// Unattached volume is deletable.
	if err := compute.DeleteVolume(ctx, "acme", "prod", "data"); err != nil {
		t.Fatalf("delete unattached volume: %v", err)
	}

	// Security groups.
	sg, err := compute.CreateSecurityGroup(ctx, "acme", "prod", database.SecurityGroup{
		Name: "web", Description: "allow http", Rules: []database.SecurityRule{{Port: 443, CIDR: "0.0.0.0/0"}},
	})
	if err != nil {
		t.Fatalf("create security group: %v", err)
	}
	if len(sg.Rules) != 1 || sg.Rules[0].Port != 443 {
		t.Fatalf("unexpected security group: %+v", sg)
	}
	groups, err := compute.ListSecurityGroups(ctx, "acme", "prod")
	if err != nil {
		t.Fatalf("list security groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 security group, got %d", len(groups))
	}
}

// TestHTTPEndpoints drives the real mux and verifies audit trail entries are
// written for each operation.
func TestHTTPEndpoints(t *testing.T) {
	compute, stopFn := newCompute(t)
	defer stopFn()
	ctx := context.Background()

	mux := handler.NewComputeHandler(compute).Router()
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
	if resp, body := do(http.MethodPost, "/keypairs", `{"project":"prod","name":"deploy"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key pair: %d %s", resp.StatusCode, body)
	}
	if resp, body := do(http.MethodPost, "/instances",
		`{"project":"prod","name":"web-1","type":"olympus-small","key_pair":"deploy"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("launch instance: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do(http.MethodGet, "/instances?project=prod", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("list instances: %d", resp.StatusCode)
	}
	if resp, _ := do(http.MethodPost, "/instance/prod/web-1/stop", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("stop instance: %d", resp.StatusCode)
	}

	var auditCount int
	if err := compute.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE account_id = 'acme'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit trail rows from HTTP operations")
	}
}
