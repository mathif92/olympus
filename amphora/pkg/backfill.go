package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mathif92/olympus/amphora/pkg/database"
)

// BackfillEtags stamps the x-olympus-etag user-metadata marker onto every object
// stored in MinIO that predates drift-tracking, using the ETag already recorded
// in the metadata database. This is a one-time maintenance operation; it performs
// a metadata-only server-side copy and does not re-upload object bytes.
func BackfillEtags(ctx context.Context, backend *MinioStorageBackend, db *database.Client) (int, error) {
	if backend == nil {
		return 0, errors.New("minio backend required")
	}
	if db == nil {
		return 0, errors.New("metadata database required")
	}

	_, dbEtags, err := loadObjectIndex(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("load postgres index: %w", err)
	}

	keys, err := backend.ListObjectKeys(ctx)
	if err != nil {
		return 0, fmt.Errorf("list minio objects: %w", err)
	}

	count := 0
	for _, key := range keys {
		md, err := backend.ObjectUserMetadata(ctx, ObjectKey(key))
		if err != nil {
			return count, fmt.Errorf("inspect metadata for %s: %w", key, err)
		}
		if metaValue(md, "x-olympus-etag") != "" {
			continue // already stamped
		}
		etag, ok := dbEtags[key]
		if !ok || etag == "" {
			continue // no DB record to derive an ETag from
		}
		if err := backend.SetUserMetadata(ctx, ObjectKey(key), map[string]string{
			"x-olympus-etag": etag,
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// metaValue returns a user-metadata value per key regardless of header key casing.
func metaValue(md map[string]string, key string) string {
	for k, v := range md {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
