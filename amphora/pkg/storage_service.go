package pkg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// StorageService is the business logic layer that orchestrates storage backends,
// key generation, and versioning logic.
type StorageService struct {
	Backend StorageBackend
}

// NewStorageService creates a StorageService wrapping the provided backend.
func NewStorageService(backend StorageBackend) *StorageService {
	return &StorageService{Backend: backend}
}

// GenerateObjectKey builds a unique object key from bucket, original name and
// version ID using the format: bucket/originalName/versionID.
func GenerateObjectKey(bucketName, originalName, versionID string) ObjectKey {
	return ObjectKey(fmt.Sprintf("%s/%s/%s", bucketName, originalName, versionID))
}

// CheckFileWriteCapability verifies that the current working directory is writable,
// returning an error if files cannot be written (e.g. read-only mount).
func CheckFileWriteCapability() error {
	tmp, err := os.CreateTemp("", "olympus-write-check-*")
	if err != nil {
		return errors.New("filesystem is not writable: " + err.Error())
	}
	tmp.Close()
	os.Remove(tmp.Name())
	return nil
}

// Store streams object data into the backend and returns the computed ETag.
func (s *StorageService) Store(ctx context.Context, key ObjectKey, meta Metadata, reader io.Reader) (string, error) {
	return s.Backend.StoreStream(ctx, key, meta, reader)
}

// Retrieve fetches the object content stream and metadata from the backend.
func (s *StorageService) Retrieve(ctx context.Context, key ObjectKey) (io.ReadSeekCloser, *Metadata, error) {
	return s.Backend.Retrieve(ctx, key)
}

// Exists reports whether the object exists in the backend.
func (s *StorageService) Exists(ctx context.Context, key ObjectKey) (bool, error) {
	return s.Backend.Exists(ctx, key)
}

// LookupLatest returns the latest version pointer for a bucket/original name.
func (s *StorageService) LookupLatest(ctx context.Context, bucketName, originalName string) (ObjectKey, *Metadata, error) {
	if lb, ok := s.Backend.(*LocalFSBackend); ok {
		return lb.GetLatestKey(ctx, bucketName, originalName)
	}
	return "", nil, errors.New("backend does not support LATEST pointer lookup")
}
