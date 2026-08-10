package pkg

import (
	"context"
	"errors"
	"fmt"

	"github.com/mathif92/olympus/storage/pkg/database"
)

// ReconcileOptions configures the gateway orphan/repair reconciliation pass.
type ReconcileOptions struct {
	// PurgeOrphans deletes MinIO objects that have no matching Postgres metadata.
	PurgeOrphans bool
	// MarkMissing marks Postgres rows whose content is missing in MinIO as 'missing'.
	MarkMissing bool
	// RepairDrift rewrites metadata so Postgres ETags match the content in MinIO.
	RepairDrift bool
}

// ReconcileResult summarizes a reconciliation pass.
type ReconcileResult struct {
	Scanned  int
	Orphans  []string // in MinIO but not in Postgres
	Missing  []string // in Postgres but not in MinIO
	Drift    []string // ETag differs between MinIO and Postgres
	Removed  int
	Marked   int
	Repaired int
}

// Reconcile compares the Postgres metadata index with the objects physically
// present in MinIO and repairs inconsistencies. It is safe against in-flight
// uploads because a fresh upload either commits metadata after its bytes land
// (no orphan) or fails before writing metadata.
func Reconcile(ctx context.Context, backend *MinioStorageBackend, db *database.Client, opts ReconcileOptions) (ReconcileResult, error) {
	if backend == nil {
		return ReconcileResult{}, errors.New("minio backend required")
	}
	if db == nil {
		return ReconcileResult{}, errors.New("metadata database required")
	}

	// 1. Postgres index: key_path -> etag.
	dbKeys, dbEtags, err := loadObjectIndex(ctx, db)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("load postgres index: %w", err)
	}

	// 2. MinIO physical objects.
	minioKeys, err := backend.ListObjectKeys(ctx)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("list minio objects: %w", err)
	}
	minioSet := make(map[string]bool, len(minioKeys))
	for _, k := range minioKeys {
		minioSet[k] = true
	}

	res := ReconcileResult{Scanned: len(minioKeys)}

	// 3. Orphans: in MinIO but not in Postgres.
	for _, k := range minioKeys {
		if dbKeys[k] {
			continue
		}
		res.Orphans = append(res.Orphans, k)
		if opts.PurgeOrphans {
			if err := backend.RemoveObject(ctx, ObjectKey(k)); err != nil {
				return res, fmt.Errorf("remove orphan %s: %w", k, err)
			}
			res.Removed++
		}
	}

	// 4. Missing rows + ETag drift, from the Postgres side.
	for key := range dbKeys {
		if !minioSet[key] {
			res.Missing = append(res.Missing, key)
			if opts.MarkMissing {
				if err := markObjectStatus(ctx, db, key, "missing"); err != nil {
					return res, fmt.Errorf("mark missing %s: %w", key, err)
				}
				res.Marked++
			}
			continue
		}

		if !opts.RepairDrift {
			continue
		}
		remoteETag, err := backend.StatETag(ctx, ObjectKey(key))
		if err != nil {
			return res, fmt.Errorf("stat object %s: %w", key, err)
		}
		if remoteETag != "" && dbEtags[key] != "" && remoteETag != dbEtags[key] {
			res.Drift = append(res.Drift, key)
			if err := updateObjectETag(ctx, db, key, remoteETag); err != nil {
				return res, fmt.Errorf("repair etag %s: %w", key, err)
			}
			res.Repaired++
		}
	}

	return res, nil
}

// loadObjectIndex loads key_path -> etag for every active object in the metadata
// database.
func loadObjectIndex(ctx context.Context, db *database.Client) (map[string]bool, map[string]string, error) {
	keys := make(map[string]bool)
	etags := make(map[string]string)
	rows, err := db.DB.QueryContext(ctx,
		`SELECT key_path, etag FROM objects WHERE status = 'active'`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, etag string
		if err := rows.Scan(&key, &etag); err != nil {
			return nil, nil, err
		}
		keys[key] = true
		etags[key] = etag
	}
	return keys, etags, rows.Err()
}

// markObjectStatus updates an object row's status column.
func markObjectStatus(ctx context.Context, db *database.Client, key, status string) error {
	_, err := db.Exec(ctx, `UPDATE objects SET status=$2, updated_at=NOW() WHERE key_path=$1`, key, status)
	return err
}

// updateObjectETag corrects an object row's ETag to match stored content.
func updateObjectETag(ctx context.Context, db *database.Client, key, etag string) error {
	_, err := db.Exec(ctx,
		`UPDATE objects SET etag=$2, checksum_value=$2, updated_at=NOW() WHERE key_path=$1`, key, etag)
	return err
}
