package integration

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/mathif92/olympus/amphora/pkg"
)

// rawClient builds an independent MinIO client for injecting objects that bypass
// the backend (simulating orphaned data / content drift).
func rawClient(t *testing.T, endpoint string) *minio.Client {
	t.Helper()
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioITUser, minioITPass, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("create raw minio client: %v", err)
	}
	return mc
}

func TestReconcileDetectsAndRepairs(t *testing.T) {
	backend, endpoint, stop := startMinioBackend(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bucket := "reconcile-it"
	raw := rawClient(t, endpoint)
	client := backend.ConfigDB()
	seedSpace(t, ctx, client, bucket)

	good := pkg.ObjectKey(bucket + "/good.txt")
	missing := pkg.ObjectKey(bucket + "/missing.txt")
	drift := pkg.ObjectKey(bucket + "/drift.txt")
	orphan := pkg.ObjectKey(bucket + "/orphan.txt")

	meta := pkg.Metadata{
		ContentType:  "text/plain",
		VersionID:    "LATEST",
		LastModified: time.Now().Format(time.RFC3339),
	}

	// Consistent object (both sides agree).
	if _, err := backend.StoreStream(ctx, good, meta, bytes.NewReader([]byte("good"))); err != nil {
		t.Fatalf("store good: %v", err)
	}

	// Missing content: bytes recorded in Postgres but removed from MinIO.
	if _, err := backend.StoreStream(ctx, missing, meta, bytes.NewReader([]byte("missing"))); err != nil {
		t.Fatalf("store missing: %v", err)
	}
	if err := backend.RemoveObject(ctx, missing); err != nil {
		t.Fatalf("remove missing: %v", err)
	}

	// Drift: replaced in MinIO without updating Postgres.
	if _, err := backend.StoreStream(ctx, drift, meta, bytes.NewReader([]byte("v1"))); err != nil {
		t.Fatalf("store drift: %v", err)
	}
	if _, err := raw.PutObject(ctx, "olympus-it", string(drift), bytes.NewReader([]byte("v2-changed")), int64(len("v2-changed")), minio.PutObjectOptions{}); err != nil {
		t.Fatalf("overwrite drift: %v", err)
	}

	// Orphan: present in MinIO but never tracked in Postgres.
	if _, err := raw.PutObject(ctx, "olympus-it", string(orphan), bytes.NewReader([]byte("orphan")), int64(len("orphan")), minio.PutObjectOptions{}); err != nil {
		t.Fatalf("put orphan: %v", err)
	}

	// Dry pass flags none by default.
	pre, err := pkg.Reconcile(ctx, backend, client, pkg.ReconcileOptions{})
	if err != nil {
		t.Fatalf("reconcile (dry): %v", err)
	}
	if len(pre.Missing) != 1 {
		t.Errorf("expected 1 missing row, got %d: %v", len(pre.Missing), pre.Missing)
	}
	if len(pre.Orphans) != 1 {
		t.Errorf("expected 1 orphan, got %d: %v", len(pre.Orphans), pre.Orphans)
	}
	if len(pre.Drift) != 0 {
		t.Errorf("drift detection only runs with RepairDrift enabled; got %d: %v", len(pre.Drift), pre.Drift)
	}
	// Dry-run must not have removed anything.
	if pre.Removed != 0 || pre.Marked != 0 || pre.Repaired != 0 {
		t.Errorf("dry reconcile must be non-destructive: %+v", pre)
	}

	// Full repair pass with all changes enabled.
	res, err := pkg.Reconcile(ctx, backend, client, pkg.ReconcileOptions{
		PurgeOrphans: true,
		MarkMissing:  true,
		RepairDrift:  true,
	})
	if err != nil {
		t.Fatalf("reconcile (repair): %v", err)
	}
	if len(res.Drift) != 1 {
		t.Errorf("expected 1 drift, got %d: %v", len(res.Drift), res.Drift)
	}
	if res.Removed != 1 {
		t.Errorf("expected 1 orphan removed, got %d", res.Removed)
	}
	if res.Marked != 1 {
		t.Errorf("expected 1 row marked missing, got %d", res.Marked)
	}
	if res.Repaired != 1 {
		t.Errorf("expected 1 drift repaired, got %d", res.Repaired)
	}

	// Second pass must be a no-op: fully reconciled.
	again, err := pkg.Reconcile(ctx, backend, client, pkg.ReconcileOptions{
		PurgeOrphans: true,
		MarkMissing:  true,
		RepairDrift:  true,
	})
	if err != nil {
		t.Fatalf("reconcile (idempotency): %v", err)
	}
	if len(again.Orphans) != 0 || len(again.Missing) != 0 || len(again.Drift) != 0 {
		t.Errorf("expected clean re-run, got orphans=%d missing=%d drift=%d",
			len(again.Orphans), len(again.Missing), len(again.Drift))
	}

	// The good object must remain fully intact.
	rdr, gotMeta, err := backend.Retrieve(ctx, good)
	if err != nil {
		t.Fatalf("retrieve good after reconcile: %v", err)
	}
	got, err := io.ReadAll(rdr)
	rdr.Close()
	if err != nil {
		t.Fatalf("read good: %v", err)
	}
	if string(got) != "good" || gotMeta == nil || gotMeta.ETag == "" {
		t.Errorf("good object corrupted after reconcile")
	}
}
