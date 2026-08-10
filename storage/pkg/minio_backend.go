package pkg

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/mathif92/olympus/storage/pkg/database"
)

// ErrObjectNotFound indicates an object does not exist in the storage backend.
var ErrObjectNotFound = errors.New("object not found")

// MinioConfig holds the connection settings for an S3/MinIO reachable on-prem.
type MinioConfig struct {
	Endpoint  string // e.g. "minio.example.com:9000" or "minio:9000"
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UsingSSL  bool
	// DB is the metadata store; object index upserts are persisted here when set.
	DB *database.Client
}

// MinioStorageBackend implements the StorageBackend interface, persisting object
// bytes to an external S3-compatible store (MinIO) while keeping metadata in the
// application database. Gateway pods therefore remain stateless and scale out.
type MinioStorageBackend struct {
	client *minio.Client
	cfg    MinioConfig
}

// NewMinioStorageBackend creates a MinIO-backed backend and ensures the bucket
// exists.
func NewMinioStorageBackend(cfg MinioConfig) (*MinioStorageBackend, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("minio endpoint is required")
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "olympus"
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UsingSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("create minio bucket %q: %w", cfg.Bucket, err)
		}
	}

	return &MinioStorageBackend{client: client, cfg: cfg}, nil
}

// objectName returns the flat storage key used inside the MinIO bucket. Key names
// already embed the space/bucket, so this yields a stable, collision-free id.
func (m *MinioStorageBackend) objectName(key ObjectKey) string {
	return string(key)
}

// ConfigDB exposes the metadata database client (used by tests/instrumentation).
func (m *MinioStorageBackend) ConfigDB() *database.Client {
	return m.cfg.DB
}

// StoreStream writes the object stream to a temp file while hashing, then uploads
// it to MinIO and persists metadata.
func (m *MinioStorageBackend) StoreStream(ctx context.Context, key ObjectKey, meta Metadata, reader io.Reader) (string, error) {
	// 1. Stream to a temp file while computing our SHA-256 ETag.
	tmp, err := os.CreateTemp("", "olympus-minio-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), reader)
	if err != nil {
		return "", fmt.Errorf("stream object data: %w", err)
	}
	etag := fmt.Sprintf("%x", hasher.Sum(nil))

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind temp file: %w", err)
	}

	// 2. Upload to MinIO.
	_, err = m.client.PutObject(ctx, m.cfg.Bucket, m.objectName(key), tmp, size, minio.PutObjectOptions{
		ContentType: meta.ContentType,
		UserMetadata: map[string]string{
			"x-olympus-etag":    etag,
			"x-olympus-origin":  meta.OriginalFilename,
			"x-olympus-version": meta.VersionID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("upload object to minio: %w", err)
	}

	// 3. Persist metadata (source of truth).
	if err := persistObjectMeta(ctx, m.cfg.DB, key, meta, size, etag, m.objectName(key)); err != nil {
		return "", fmt.Errorf("persist object metadata: %w", err)
	}

	fmt.Printf("INFO: MinioBackend stored object '%s'. ETag: %s.\n", key, etag)
	return etag, nil
}

// Retrieve streams the object bytes from MinIO and returns a seekable stream plus
// metadata (DB-first, falling back to MinIO object metadata).
func (m *MinioStorageBackend) Retrieve(ctx context.Context, key ObjectKey) (io.ReadSeekCloser, *Metadata, error) {
	obj, err := m.client.GetObject(ctx, m.cfg.Bucket, m.objectName(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get object from minio: %w", err)
	}

	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		if isNotFound(err) {
			return nil, nil, ErrObjectNotFound
		}
		return nil, nil, fmt.Errorf("stat object in minio: %w", err)
	}

	meta, err := m.loadMetadata(ctx, key)
	if err != nil || meta.ETag == "" {
		meta = &Metadata{
			OriginalFilename: userMeta(info, "x-olympus-origin"),
			ContentType:      info.ContentType,
			ContentLength:    info.Size,
			ETag:             userMeta(info, "x-olympus-etag"),
			VersionID:        userMeta(info, "x-olympus-version"),
		}
	}
	if meta.ETag == "" {
		meta.ETag = info.ETag
	}

	return obj, meta, nil
}

// loadMetadata reads object metadata from PostgreSQL.
func (m *MinioStorageBackend) loadMetadata(ctx context.Context, key ObjectKey) (*Metadata, error) {
	if m.cfg.DB == nil {
		return nil, errors.New("no metadata database configured")
	}
	var meta Metadata
	row := m.cfg.DB.QueryRow(ctx, `SELECT original_filename, content_type, content_length, etag, version_id
		FROM objects WHERE key_path=$1`, string(key))
	if err := row.Scan(&meta.OriginalFilename, &meta.ContentType, &meta.ContentLength, &meta.ETag, &meta.VersionID); err != nil {
		return nil, err
	}
	return &meta, nil
}

// Exists reports whether the object is present in MinIO.
func (m *MinioStorageBackend) Exists(ctx context.Context, key ObjectKey) (bool, error) {
	_, err := m.client.StatObject(ctx, m.cfg.Bucket, m.objectName(key), minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// isNotFound reports whether an S3 error is a missing-key / not-found response.
func isNotFound(err error) bool {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		return resp.Code == "NoSuchKey" || resp.Code == "NotFound" || resp.Code == "NoSuchObject"
	}
	return false
}

// Healthy verifies the MinIO deployment is reachable and the bucket exists.
func (m *MinioStorageBackend) Healthy(ctx context.Context) error {
	ok, err := m.client.BucketExists(ctx, m.cfg.Bucket)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("bucket " + m.cfg.Bucket + " missing")
	}
	return nil
}

// ListObjectKeys iterates every object key currently stored in MinIO.
func (m *MinioStorageBackend) ListObjectKeys(ctx context.Context) ([]string, error) {
	var keys []string
	for obj := range m.client.ListObjects(ctx, m.cfg.Bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

// RemoveObject deletes an object from the MinIO bucket.
func (m *MinioStorageBackend) RemoveObject(ctx context.Context, key ObjectKey) error {
	return m.client.RemoveObject(ctx, m.cfg.Bucket, m.objectName(key), minio.RemoveObjectOptions{})
}

// StatETag returns the ETag recorded on the object in MinIO (header ETag or the
// x-olympus-etag user metadata), and the content size.
func (m *MinioStorageBackend) StatETag(ctx context.Context, key ObjectKey) (string, error) {
	info, err := m.client.StatObject(ctx, m.cfg.Bucket, m.objectName(key), minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return "", ErrObjectNotFound
		}
		return "", err
	}
	if v := userMeta(info, "x-olympus-etag"); v != "" {
		return v, nil
	}
	return strings.Trim(info.ETag, `"`), nil
}

// userMeta returns an object's user metadata value regardless of how minio-go
// normalized the header key case.
func userMeta(info minio.ObjectInfo, key string) string {
	for k, v := range info.UserMetadata {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// ObjectUserMetadata returns the user-metadata attached to an object in MinIO
// (keys are normalized to Title-Case by minio-go on read).
func (m *MinioStorageBackend) ObjectUserMetadata(ctx context.Context, key ObjectKey) (map[string]string, error) {
	info, err := m.client.StatObject(ctx, m.cfg.Bucket, m.objectName(key), minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return info.UserMetadata, nil
}

// SetUserMetadata rewrites an object's user metadata via a server-side copy.
// This stamps the x-olympus-* markers on content written before drift-tracking
// existed, without re-uploading the bytes.
func (m *MinioStorageBackend) SetUserMetadata(ctx context.Context, key ObjectKey, meta map[string]string) error {
	dst := minio.CopyDestOptions{
		Bucket:          m.cfg.Bucket,
		Object:          m.objectName(key),
		UserMetadata:    meta,
		ReplaceMetadata: true,
	}
	src := minio.CopySrcOptions{
		Bucket: m.cfg.Bucket,
		Object: m.objectName(key),
	}
	_, err := m.client.CopyObject(ctx, dst, src)
	if err != nil {
		return fmt.Errorf("rewrite user metadata for %s: %w", key, err)
	}
	return nil
}
