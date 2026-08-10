package pkg

import (
	"context"
	"io"
)

// ObjectKey represents a unique object identifier in the storage system.
type ObjectKey string

// Metadata contains metadata about a stored file/object.
type Metadata struct {
	OriginalFilename string `json:"original_filename"`
	ContentType      string `json:"content_type"`
	ContentLength    int64  `json:"content_length"`
	LastModified     string `json:"last_modified"`
	ETag             string `json:"etag"`
	VersionID        string `json:"version_id"` // e.g. "LATEST"
}

// StorageBackend defines the contract that any storage backend must satisfy.
type StorageBackend interface {
	// StoreStream writes the object content stream to the backend, calculates an
	// ETag (SHA256), persists metadata, and returns the computed ETag.
	StoreStream(ctx context.Context, key ObjectKey, meta Metadata, reader io.Reader) (etag string, err error)
	// Retrieve fetches the object content as a seekable stream plus its metadata.
	// The caller is responsible for closing the returned stream.
	Retrieve(ctx context.Context, key ObjectKey) (io.ReadSeekCloser, *Metadata, error)
	// Exists reports whether an object with the given key exists.
	Exists(ctx context.Context, key ObjectKey) (bool, error)
}
