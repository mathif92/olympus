// Package integration contains end-to-end tests that exercise Paramdora
// against a real PostgreSQL started with testcontainers.
package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mathif92/olympus/paramdora/pkg"
	"github.com/mathif92/olympus/paramdora/pkg/database"
)

// startPostgres boots a real Postgres, applies the goose migrations, and
// returns a ready database.Client plus a cleanup func.
func startPostgres(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("olympus_parameters"),
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

func newStore(t *testing.T) (*pkg.ParamStore, func()) {
	t.Helper()
	client, stop := startPostgres(t)
	cipher, err := pkg.NewCipher("integration-test-master-key")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return pkg.NewParamStore(client, cipher), stop
}

func TestTenantIsolation(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	if err := store.EnsureAccount(ctx, database.Account{ID: "tenant-a", DisplayName: "A", Email: "a@p.dev", Plan: "pro", ParameterLimit: 1000}); err != nil {
		t.Fatalf("ensure account a: %v", err)
	}
	if err := store.EnsureAccount(ctx, database.Account{ID: "tenant-b", DisplayName: "B", Email: "b@p.dev", Plan: "pro", ParameterLimit: 1000}); err != nil {
		t.Fatalf("ensure account b: %v", err)
	}

	if err := store.CreateProject(ctx, "tenant-a", database.Project{Name: "shared", Description: ""}); err != nil {
		t.Fatalf("create project in account a: %v", err)
	}
	if _, err := store.PutParameter(ctx, "tenant-a", "shared", pkg.PutParameterInput{
		Name: "secret", Value: "a-only", Type: database.TypeSecureString,
	}); err != nil {
		t.Fatalf("put parameter in account a: %v", err)
	}

	// Tenant B cannot see tenant A's project or parameter.
	if _, err := store.ListProjects(ctx, "tenant-b"); err != nil {
		t.Fatalf("list projects b: %v", err)
	}
	if _, err := store.GetParameter(ctx, "tenant-b", "shared", "secret", false); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound for tenant b, got %v", err)
	}

	// And B cannot overwrite A's parameter either.
	if _, err := store.PutParameter(ctx, "tenant-b", "shared", pkg.PutParameterInput{
		Name: "secret", Value: "b-only",
	}); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound when B writes to A's project, got %v", err)
	}
}

func TestPutGetVersioningAndHistory(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	if err := store.EnsureAccount(ctx, database.Account{ID: "acme", DisplayName: "Acme", Email: "acme@p.dev", Plan: "pro", ParameterLimit: 1000}); err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	if err := store.CreateProject(ctx, "acme", database.Project{Name: "checkout-api"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// v1
	p1, err := store.PutParameter(ctx, "acme", "checkout-api", pkg.PutParameterInput{
		Name: "db/host", Value: "db1.internal", Type: database.TypeString,
		Tags: map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("put v1: %v", err)
	}
	if p1.Version != 1 {
		t.Fatalf("expected version 1, got %d", p1.Version)
	}

	// v2 update
	p2, err := store.PutParameter(ctx, "acme", "checkout-api", pkg.PutParameterInput{
		Name: "db/host", Value: "db2.internal", Type: database.TypeString,
	})
	if err != nil {
		t.Fatalf("put v2: %v", err)
	}
	if p2.Version != 2 || p2.Value != "db2.internal" {
		t.Fatalf("expected v2 value db2.internal, got version=%d value=%q", p2.Version, p2.Value)
	}

	got, err := store.GetParameter(ctx, "acme", "checkout-api", "db/host", false)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != 2 || got.Value != "db2.internal" {
		t.Fatalf("unexpected get result: %+v", got)
	}

	hist, err := store.GetParameterHistory(ctx, "acme", "checkout-api", "db/host")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(hist))
	}

	list, err := store.ListParameters(ctx, "acme", "checkout-api", "db/")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "db/host" {
		t.Fatalf("unexpected list: %+v", list)
	}

	if err := store.DeleteParameter(ctx, "acme", "checkout-api", "db/host"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetParameter(ctx, "acme", "checkout-api", "db/host", false); err != pkg.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSecureStringEncryption(t *testing.T) {
	store, stop := newStore(t)
	defer stop()
	ctx := context.Background()

	if err := store.EnsureAccount(ctx, database.Account{ID: "acme", DisplayName: "Acme", Email: "acme@p.dev", Plan: "pro", ParameterLimit: 1000}); err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	if err := store.CreateProject(ctx, "acme", database.Project{Name: "payments"}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if _, err := store.PutParameter(ctx, "acme", "payments", pkg.PutParameterInput{
		Name: "gateway/key", Value: "super-secret-value", Type: database.TypeSecureString,
	}); err != nil {
		t.Fatalf("put secure: %v", err)
	}

	// Without decrypt the value is masked.
	masked, err := store.GetParameter(ctx, "acme", "payments", "gateway/key", false)
	if err != nil {
		t.Fatalf("get masked: %v", err)
	}
	if !masked.Encrypted {
		t.Fatal("expected parameter to be marked encrypted")
	}
	if masked.Value != "" {
		t.Fatalf("expected masked empty value, got %q", masked.Value)
	}

	// With decrypt the original value is returned.
	plain, err := store.GetParameter(ctx, "acme", "payments", "gateway/key", true)
	if err != nil {
		t.Fatalf("get decrypted: %v", err)
	}
	if plain.Value != "super-secret-value" {
		t.Fatalf("expected decrypted value, got %q", plain.Value)
	}

	// The raw row must not contain the plaintext.
	var raw string
	params, err := store.ListParameters(ctx, "acme", "payments", "")
	if err != nil || len(params) != 1 {
		t.Fatalf("list: %v %d", err, len(params))
	}
	if err := store.DB.QueryRow(ctx,
		`SELECT value FROM parameters WHERE name = 'gateway/key'`).Scan(&raw); err != nil {
		t.Fatalf("query raw value: %v", err)
	}
	if raw == "super-secret-value" || raw == "" {
		t.Fatalf("expected ciphertext at rest, got %q", raw)
	}
}
