package pkg

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mathif92/olympus/amphora/pkg/database"
)

// HybridStorageBackend combines filesystem storage with PostgreSQL metadata and
// Redis caching, as recommended by the DB_COMPARISON.md architecture.
type HybridStorageBackend struct {
	BaseDir string
	DB      *database.Client
	Cache   *redis.Client
}

// NewHybridStorageBackend creates a hybrid storage backend combining FS + Postgres + Redis.
func NewHybridStorageBackend(baseDir string, db *database.Client, cache *redis.Client) *HybridStorageBackend {
	return &HybridStorageBackend{
		BaseDir: baseDir,
		DB:      db,
		Cache:   cache,
	}
}

// StoreStream writes the object content to the filesystem, calculates an ETag,
// then stores metadata atomically in PostgreSQL and caches it in Redis.
func (h *HybridStorageBackend) StoreStream(ctx context.Context, key ObjectKey, meta Metadata, reader io.Reader) (etag string, err error) {
	if h.BaseDir == "" {
		return "", errors.New("base directory not set")
	}

	// 1. Calculate ETag via streaming and write to a temp file.
	hasher := sha256.New()
	tempFilePath := fmt.Sprintf("%s/%s.tmp", h.BaseDir, key)

	if strings.Contains(string(key), "/") {
		if err := os.MkdirAll(filepath.Dir(tempFilePath), 0755); err != nil {
			return "", fmt.Errorf("failed to create directory for %s: %w", key, err)
		}
	}

	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file %s: %w", key, err)
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempFilePath)
	}()

	multiWriter := io.MultiWriter(tempFile, hasher)
	bytesCopied, err := io.Copy(multiWriter, reader)
	if err != nil {
		return "", fmt.Errorf("failed to copy stream data: %w", err)
	}

	hashBytes := hasher.Sum(nil)
	etag = fmt.Sprintf("%x", hashBytes)

	finalFilePath := fmt.Sprintf("%s/%s", h.BaseDir, key)
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		return "", fmt.Errorf("failed to finalize file write for %s: %w", key, err)
	}

	// 2. Store metadata in PostgreSQL using an upsert.
	if err := persistObjectMeta(ctx, h.DB, key, meta, bytesCopied, etag, finalFilePath); err != nil {
		return "", fmt.Errorf("failed to persist object metadata: %w", err)
	}

	fmt.Printf("INFO: HybridBackend stored object '%s'. ETag: %s.\n", key, etag)

	// 3. Cache metadata in Redis for faster reads (2 hour TTL).
	metaBytes, _ := json.Marshal(meta)
	if h.Cache != nil {
		h.Cache.Set(ctx, "obj:"+string(key), metaBytes, 7200*time.Second)
	}

	return etag, nil
}

// Retrieve opens the object's content stream from the filesystem (with metadata
// served from Redis or PostgreSQL) and returns a seekable stream plus metadata.
func (h *HybridStorageBackend) Retrieve(ctx context.Context, key ObjectKey) (io.ReadSeekCloser, *Metadata, error) {
	if h.BaseDir == "" {
		return nil, nil, errors.New("base directory not set")
	}

	var meta Metadata

	// 1. Try metadata from Redis cache first.
	cacheKey := "obj:" + string(key)
	if h.Cache != nil {
		if cached, err := h.Cache.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			_ = json.Unmarshal(cached, &meta)
		}
	}

	// 2. Open the object data stream from the filesystem.
	filePath := fmt.Sprintf("%s/%s", h.BaseDir, key)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open object file: %w", err)
	}

	// 3. Fetch/refresh metadata from PostgreSQL if not already populated.
	if meta.ETag == "" {
		meta = h.loadMetadata(ctx, key)
	}

	// 4. Cache the retrieved metadata for faster future reads.
	if h.Cache != nil && meta.ETag != "" {
		metaBytes, _ := json.Marshal(meta)
		h.Cache.Set(ctx, cacheKey, metaBytes, 3600*time.Second)
	}

	return file, &meta, nil
}

// loadMetadata fetches object metadata from PostgreSQL.
func (h *HybridStorageBackend) loadMetadata(ctx context.Context, key ObjectKey) Metadata {
	var meta Metadata
	row := h.DB.QueryRow(ctx, `SELECT original_filename, content_type, content_length, etag, version_id
		FROM objects WHERE key_path=$1`, string(key))
	if err := row.Scan(&meta.OriginalFilename, &meta.ContentType, &meta.ContentLength, &meta.ETag, &meta.VersionID); err != nil {
		return meta
	}
	return meta
}

// Exists checks if an object exists using the filesystem metadata index or PostgreSQL.
func (h *HybridStorageBackend) Exists(ctx context.Context, key ObjectKey) (bool, error) {
	if h.BaseDir == "" {
		return false, errors.New("base directory not set")
	}

	// 1. Filesystem metadata index check.
	metaFilePath := fmt.Sprintf("%s/meta_%s.json", h.BaseDir, key)
	if _, err := os.Stat(metaFilePath); err == nil {
		return true, nil
	}

	// 2. Query PostgreSQL as a fallback.
	var count int
	err := h.DB.QueryRow(ctx, `SELECT COUNT(*) FROM objects WHERE key_path=$1`, string(key)).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// deriveSpaceIDFromKey derives a space (bucket) identifier from the object key path.
func deriveSpaceIDFromKey(key ObjectKey) string {
	parts := strings.Split(string(key), "/")
	if len(parts) < 1 || parts[0] == "" {
		return ""
	}
	return "space_" + parts[0]
}

// persistObjectMeta upserts object metadata into PostgreSQL. The storage_path
// records where the bytes physically live (filesystem path, or the object key
// within a MinIO bucket for the S3-backed backend). It is a no-op if db is nil.
func persistObjectMeta(ctx context.Context, db *database.Client, key ObjectKey, meta Metadata,
	contentLength int64, etag string, storagePath string) error {
	if db == nil {
		return nil
	}

	if err := ensureBucket(ctx, db, key); err != nil {
		return err
	}

	spaceID := deriveSpaceIDFromKey(key)
	objectID := objectHashID(key)

	query := `INSERT INTO objects
		(id, space_id, key_path, original_filename, content_type, content_length,
		 etag, version_id, checksum_algorithm, checksum_value, storage_path, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'sha256',$9,$10,'active',NOW(),NOW())
		ON CONFLICT (space_id, key_path) DO UPDATE SET
			content_type = EXCLUDED.content_type,
			content_length = EXCLUDED.content_length,
			etag = EXCLUDED.etag,
			checksum_value = EXCLUDED.checksum_value,
			storage_path = EXCLUDED.storage_path,
			updated_at = NOW()`

	_, err := db.Exec(ctx, query,
		objectID, spaceID, string(key), meta.OriginalFilename, meta.ContentType,
		contentLength, etag, meta.VersionID, etag, storagePath,
	)
	return err
}

// objectHashID returns a stable hex identifier for an object key.
func objectHashID(key ObjectKey) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum[:16])
}

// ensureBucket provisions the default account and the space (bucket) an object
// belongs to when they do not yet exist. This lets a single-tenant local run work
// out of the box; the upserts are DO NOTHING so pre-provisioned tenants are never
// modified.
func ensureBucket(ctx context.Context, db *database.Client, key ObjectKey) error {
	parts := strings.Split(string(key), "/")
	if len(parts) < 1 || parts[0] == "" {
		return fmt.Errorf("object key has no bucket prefix: %q", key)
	}
	bucket := parts[0]

	const accountID = "default"
	if _, err := db.Exec(ctx, `INSERT INTO accounts (id, display_name, email, plan)
			VALUES ($1, 'Default', 'admin@olympus.local', 'free')
			ON CONFLICT (id) DO NOTHING`, accountID); err != nil {
		return fmt.Errorf("ensure default account: %w", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO spaces (id, account_id, name)
			VALUES ($1, $2, $3)
			ON CONFLICT (id) DO NOTHING`,
		"space_"+bucket, accountID, bucket); err != nil {
		return fmt.Errorf("ensure bucket space: %w", err)
	}
	return nil
}
