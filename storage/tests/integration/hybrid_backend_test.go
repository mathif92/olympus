package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcRedis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/mathif92/olympus/storage/pkg"
	"github.com/mathif92/olympus/storage/pkg/database"
)

// startInfra boots real PostgreSQL and Redis containers and returns a ready
// database.Client wired to both, with all migrations applied.
func startInfra(t *testing.T) (*database.Client, func()) {
	t.Helper()

	ctx := context.Background()

	const (
		dbUser     = "olympus"
		dbPassword = "olympus_secret"
		dbName     = "olympus_storage"
	)

	pgC, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	rdsC, err := tcRedis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("start redis container: %v", err)
	}

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	redisAddr, err := rdsC.PortEndpoint(ctx, "6379/tcp", "")
	if err != nil {
		t.Fatalf("redis host address: %v", err)
	}

	client, err := database.NewClient(database.Config{
		PostgresURL: dsn,
		RedisURL:    redisAddr,
		PoolMax:     10,
		PoolMin:     2,
	})
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}

	if err := applyMigrations(ctx, client); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return client, func() {
		client.Close()
		_ = pgC.Terminate(ctx)
		_ = rdsC.Terminate(ctx)
	}
}

// seedSpace creates the tenant account and the space that the hybrid backend
// derives (space_<bucket>) so the objects FK constraint is satisfied.
func seedSpace(t *testing.T, ctx context.Context, client *database.Client, bucket string) {
	t.Helper()
	spaceID := fmt.Sprintf("space_%s", bucket)
	if _, err := client.Exec(ctx, `
		INSERT INTO accounts (id, display_name, email, plan)
		VALUES ('acc_1', 'Integration', 'integration@olympus.local', 'free')
	`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := client.Exec(ctx, `
		INSERT INTO spaces (id, account_id, name)
		VALUES ($1, 'acc_1', $2)
	`, spaceID, bucket); err != nil {
		t.Fatalf("seed space: %v", err)
	}
}

func newTestBackend(t *testing.T) (*pkg.HybridStorageBackend, func()) {
	t.Helper()
	client, stop := startInfra(t)
	backend := pkg.NewHybridStorageBackend(t.TempDir(), client, client.Cache)
	return backend, stop
}

func TestHybridBackendStoreRetrieveExists(t *testing.T) {
	backend, stop := newTestBackend(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bucket := "test-images"
	key := pkg.GenerateObjectKey(bucket, "profile_pic.png", "LATEST")
	payload := []byte("This is fake PNG data.")

	seedSpace(t, ctx, backend.DB, bucket)

	meta := pkg.Metadata{
		OriginalFilename: "profile_pic.png",
		ContentType:      "image/png",
		ContentLength:    int64(len(payload)),
		LastModified:     time.Now().Format(time.RFC3339),
		VersionID:        "LATEST",
	}

	// 1. Upload
	etag, err := backend.StoreStream(ctx, key, meta, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("StoreStream: %v", err)
	}
	if etag == "" {
		t.Fatal("expected non-empty ETag")
	}

	// 2. Exists
	exists, err := backend.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("expected object to exist after StoreStream")
	}

	// 3. Retrieve
	gotReader, gotMeta, err := backend.Retrieve(ctx, key)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	defer gotReader.Close()
	got, err := io.ReadAll(gotReader)
	if err != nil {
		t.Fatalf("read retrieved stream: %v", err)
	}
	if !bytes.Equal(payload, got) {
		t.Errorf("retrieved data mismatch")
	}
	if gotMeta == nil || gotMeta.ETag != etag {
		t.Errorf("metadata ETag mismatch: expected %q got %v", etag, gotMeta)
	}

	// 4. Metadata persisted in PostgreSQL
	var count int
	if err := backend.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM objects WHERE key_path=$1`, string(key)).Scan(&count); err != nil {
		t.Fatalf("query objects: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 object row, got %d", count)
	}
}

func TestHybridBackendUpsertSameKey(t *testing.T) {
	backend, stop := newTestBackend(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bucket := "test-images"
	key := pkg.GenerateObjectKey(bucket, "profile_pic.png", "LATEST")
	seedSpace(t, ctx, backend.DB, bucket)

	meta := pkg.Metadata{OriginalFilename: "profile_pic.png", ContentType: "image/png", VersionID: "LATEST"}

	if _, err := backend.StoreStream(ctx, key, meta, bytes.NewReader([]byte("v1"))); err != nil {
		t.Fatalf("first StoreStream: %v", err)
	}
	if _, err := backend.StoreStream(ctx, key, meta, bytes.NewReader([]byte("v2"))); err != nil {
		t.Fatalf("second StoreStream: %v", err)
	}

	// Same (space_id, key_path) must collapse to a single row.
	var count int
	if err := backend.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM objects WHERE key_path=$1`, string(key)).Scan(&count); err != nil {
		t.Fatalf("query objects: %v", err)
	}
	if count != 1 {
		t.Errorf("expected single upserted row, got %d", count)
	}

	gotReader, _, err := backend.Retrieve(ctx, key)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	defer gotReader.Close()
	got, err := io.ReadAll(gotReader)
	if err != nil {
		t.Fatalf("read retrieved stream: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("expected latest payload 'v2', got %q", got)
	}
}

func TestHybridBackendNonExistent(t *testing.T) {
	backend, stop := newTestBackend(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exists, err := backend.Exists(ctx, pkg.ObjectKey("no/such/object"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("expected object to not exist")
	}

	if _, _, err := backend.Retrieve(ctx, pkg.ObjectKey("no/such/object")); err == nil {
		t.Fatal("expected Retrieve of missing object to fail")
	}
}
