package integration

import (
	"context"
	"database/sql"
	"net"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/mathif92/olympus/clio/pkg"
)

// TestDockerProvisionerRealDatabases exercises the real provisioner against a
// live Docker daemon: each managed instance is an actual PostgreSQL container,
// and snapshots are real pg_dump backups taken from inside that container.
func TestDockerProvisionerRealDatabases(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") == "" {
		t.Skip("set RUN_DOCKER_TESTS=1 to exercise the real docker provisioner (needs a Docker daemon)")
	}

	ctx := context.Background()
	provisioner := pkg.NewDockerProvisioner("postgres:16-alpine")

	creds, err := provisioner.CreateInstance(ctx, pkg.InstanceSpec{
		Name:           "analytics",
		MasterUsername: "master_analytics_admin",
		MasterPassword: "S3cretCl10!",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	t.Cleanup(func() { _ = provisioner.DeleteInstance(ctx, creds.ProviderRef) })

	if creds.Endpoint == "" || creds.MasterUsername == "" || creds.MasterPassword == "" {
		t.Fatalf("expected full instance credentials: %+v", creds)
	}

	// The instance is a real, reachable database — write and read through it.
	host, port, err := net.SplitHostPort(creds.Endpoint)
	if err != nil {
		t.Fatalf("parse endpoint %q: %v", creds.Endpoint, err)
	}
	dsn := "host=" + host + " port=" + port +
		" user=" + creds.MasterUsername + " password=" + creds.MasterPassword +
		" dbname=clio sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open instance db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping instance db (%s): %v", dsn, err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS events (id SERIAL PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table in instance: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO events (name) VALUES ('launch')`); err != nil {
		t.Fatalf("insert into instance: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row in real database, got %d", count)
	}

	// Drop the table so the snapshot captures an empty schema, then take a
	// real logical backup.
	if _, err := db.ExecContext(ctx, `DROP TABLE events`); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	snap, err := provisioner.CreateSnapshot(ctx, pkg.SnapshotSpec{
		InstanceRef: creds.ProviderRef,
		Name:        "pre-launch",
	})
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snap.ProviderRef == "" || snap.SizeGB <= 0 {
		t.Fatalf("expected non-empty real snapshot: %+v", snap)
	}

	if err := provisioner.DeleteSnapshot(ctx, snap.ProviderRef); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}

	// Stop/start lifecycle against the real container.
	if err := provisioner.StopInstance(ctx, creds.ProviderRef); err != nil {
		t.Fatalf("stop instance: %v", err)
	}
	if err := provisioner.StartInstance(ctx, creds.ProviderRef); err != nil {
		t.Fatalf("start instance: %v", err)
	}
}
