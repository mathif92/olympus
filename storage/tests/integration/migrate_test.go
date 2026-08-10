// Package integration contains end-to-end tests that exercise the storage
// service against real infrastructure spun up with testcontainers.
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mathif92/olympus/storage/pkg/database"
)

// migrationsDir resolves to the SQL migrations directory relative to this
// package, assuming the test is run from tests/integration.
func migrationsDir() (string, error) {
	if override := os.Getenv("OLYMPUS_MIGRATIONS_DIR"); override != "" {
		return override, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Tests run with cwd == the package directory (tests/integration).
	return filepath.Abs(filepath.Join(wd, "..", "..", "migrations"))
}

// applyMigrations runs all goose migrations defined in migrationsDir.
func applyMigrations(ctx context.Context, client *database.Client) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}
	if err := database.Migrate(client.DB, dir); err != nil {
		return fmt.Errorf("apply goose migrations from %q: %w", dir, err)
	}
	return nil
}

// rollbackMigrations rolls back a single goose migration (used to verify Down).
func rollbackMigrations(ctx context.Context, client *database.Client) error {
	dir, err := migrationsDir()
	if err != nil {
		return err
	}
	if err := database.Rollback(client.DB, dir); err != nil {
		return fmt.Errorf("rollback goose migration from %q: %w", dir, err)
	}
	return nil
}

// tableExists reports whether a given table exists in the public schema.
type tableChecker struct{ client *database.Client }

func (t *tableChecker) exists(ctx context.Context, name string) (bool, error) {
	var n int
	err := t.client.QueryRow(ctx,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`,
		name).Scan(&n)
	return n > 0, err
}

func TestGooseMigrationsUpAndDown(t *testing.T) {
	client, stop := startInfra(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	check := &tableChecker{client: client}

	// All four tables exist after Up.
	for _, tbl := range []string{"accounts", "spaces", "objects", "audit_logs"} {
		ok, err := check.exists(ctx, tbl)
		if err != nil {
			t.Fatalf("check table %s: %v", tbl, err)
		}
		if !ok {
			t.Fatalf("expected table %q to exist after migrations", tbl)
		}
	}

	// Roll back one migration -> audit_logs should disappear.
	if err := rollbackMigrations(ctx, client); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	ok, err := check.exists(ctx, "audit_logs")
	if err != nil {
		t.Fatalf("check audit_logs after down: %v", err)
	}
	if ok {
		t.Fatal("expected audit_logs to be dropped after rolling back last migration")
	}

	// Re-apply -> audit_logs returns.
	if err := applyMigrations(ctx, client); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	ok, err = check.exists(ctx, "audit_logs")
	if err != nil {
		t.Fatalf("check audit_logs after re-apply: %v", err)
	}
	if !ok {
		t.Fatal("expected audit_logs to exist after re-applying migrations")
	}
}
