package integration

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	tcMinio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/mathif92/olympus/amphora/pkg"
)

const (
	minioITUser = "minioadmin"
	minioITPass = "minioadmin"
)

// startMinioBackend boots a Postgres+Redis stack and a real MinIO container,
// runs migrations, and returns a MinIO-backed backend wired to all of them.
func startMinioBackend(t *testing.T) (*pkg.MinioStorageBackend, string, func()) {
	t.Helper()
	ctx := context.Background()

	client, stop := startInfra(t)

	mc, err := tcMinio.Run(ctx, "minio/minio:latest",
		tcMinio.WithUsername(minioITUser),
		tcMinio.WithPassword(minioITPass),
	)
	if err != nil {
		stop()
		t.Fatalf("start minio container: %v", err)
	}

	endpoint, err := mc.ConnectionString(ctx)
	if err != nil {
		stop()
		_ = mc.Terminate(ctx)
		t.Fatalf("minio endpoint: %v", err)
	}

	backend, err := pkg.NewMinioStorageBackend(pkg.MinioConfig{
		Endpoint:  endpoint,
		AccessKey: minioITUser,
		SecretKey: minioITPass,
		Bucket:    "olympus-it",
		DB:        client,
	})
	if err != nil {
		stop()
		_ = mc.Terminate(ctx)
		t.Fatalf("create minio backend: %v", err)
	}

	return backend, endpoint, func() {
		stop()
		_ = mc.Terminate(ctx)
	}
}

func TestMinioRoundTrip(t *testing.T) {
	backend, _, stop := startMinioBackend(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	bucket := "test-images"
	key := pkg.GenerateObjectKey(bucket, "pic.png", "LATEST")
	payload := []byte("minio-backed object payload.")

	meta := pkg.Metadata{
		OriginalFilename: "pic.png",
		ContentType:      "image/png",
		ContentLength:    int64(len(payload)),
		LastModified:     time.Now().Format(time.RFC3339),
		VersionID:        "LATEST",
	}

	// Seed the space so the Postgres objects FK is satisfied.
	seedSpace(t, ctx, backend.ConfigDB(), bucket)

	etag, err := backend.StoreStream(ctx, key, meta, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("StoreStream: %v", err)
	}
	if etag == "" {
		t.Fatal("expected non-empty ETag")
	}

	exists, err := backend.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("expected object to exist after StoreStream")
	}

	reader, gotMeta, err := backend.Retrieve(ctx, key)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !bytes.Equal(payload, got) {
		t.Errorf("retrieved data mismatch")
	}
	if gotMeta == nil || gotMeta.ETag != etag {
		t.Errorf("ETag mismatch: want %q got %v", etag, gotMeta)
	}
}

func TestMinioNonExistent(t *testing.T) {
	backend, _, stop := startMinioBackend(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
