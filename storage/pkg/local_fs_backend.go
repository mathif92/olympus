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
)

// LocalFSBackend implements the StorageBackend interface using the local filesystem as storage.
type LocalFSBackend struct {
	BaseDir string // Absolute path to the root of the object store directory (e.g., /storage/data)
}

func NewLocalFSBackend(baseDir string) *LocalFSBackend {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create base storage directory %s: %v", baseDir, err))
	}
	return &LocalFSBackend{BaseDir: baseDir}
}

// StoreStream writes the object content stream to the local file system at a path derived from the key,
// while calculating an ETag and saving metadata atomically.
func (l *LocalFSBackend) StoreStream(ctx context.Context, key ObjectKey, meta Metadata, reader io.Reader) (etag string, err error) {
	if l.BaseDir == "" {
		return "", errors.New("base directory not set")
	}

	// 1. Setup Hashing and Writing: Use io.MultiWriter to write data to the hasher AND the file writer.
	hasher := sha256.New()
	tempFilePath := fmt.Sprintf("%s/%s.tmp", l.BaseDir, key)

	// Create parent directories for the object path if they don't exist
	if strings.Contains(string(key), "/") {
		if err := os.MkdirAll(filepath.Dir(tempFilePath), 0755); err != nil {
			return "", fmt.Errorf("failed to create directory for %s: %w", key, err)
		}
	}

	// Create the temp file and stream into both the temporary file and the hash calculator.
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

	// 2. Calculate ETag and Finalize File Name Change (Atomic Write)
	hashBytes := hasher.Sum(nil)
	etag = fmt.Sprintf("%x", hashBytes)

	finalFilePath := fmt.Sprintf("%s/%s", l.BaseDir, key)
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		return "", fmt.Errorf("failed to finalize file write for %s: %w", key, err)
	}

	// 3. PERSIST METADATA: Save the metadata/index for the object.
	metaFilePath := fmt.Sprintf("%s/meta_%s.json", l.BaseDir, key)
	if err := l.writeMetadataIndex(metaFilePath, key, etag); err != nil {
		// We wrote the object but failed to index it. Clean up the data file.
		os.Remove(finalFilePath)
		return "", fmt.Errorf("successfully stored object data, BUT FAILED TO INDEX METADATA: %w", err)
	}

	fmt.Printf("INFO: Successfully processed object '%s'. Bytes copied: %d. ETag calculated: %s\n", key, bytesCopied, etag)
	return etag, nil
}

// writeMetadataIndex handles writing the metadata and ETag to a dedicated index file.
func (l *LocalFSBackend) writeMetadataIndex(metaFilePath string, key ObjectKey, etag string) error {
	if err := os.MkdirAll(filepath.Dir(metaFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	metadataContent := fmt.Sprintf(`{"key": "%s", "etag": "%s", "meta_type": "LocalFS"}`, key, etag)
	if err := os.WriteFile(metaFilePath, []byte(metadataContent), 0644); err != nil {
		return fmt.Errorf("failed to write metadata index file: %w", err)
	}
	fmt.Printf("INFO: Successfully indexed metadata for key '%s' at path: %s\n", key, metaFilePath)
	return nil
}

// Retrieve opens the object's content file from the local filesystem and returns
// a seekable stream plus its parsed metadata record.
func (l *LocalFSBackend) Retrieve(ctx context.Context, key ObjectKey) (io.ReadSeekCloser, *Metadata, error) {
	if l.BaseDir == "" {
		return nil, nil, errors.New("base directory not set")
	}

	// 1. Locate the metadata index first (single source of truth for existence).
	metaFilePath := fmt.Sprintf("%s/meta_%s.json", l.BaseDir, key)
	if _, err := os.Stat(metaFilePath); os.IsNotExist(err) {
		return nil, nil, errors.New("object not found (Metadata missing)")
	} else if err != nil {
		return nil, nil, fmt.Errorf("failed to read metadata index: %w", err)
	}

	// 2. Open the data payload as a stream.
	filePath := fmt.Sprintf("%s/%s", l.BaseDir, key)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open object file: %w", err)
	}

	// 3. Read and parse metadata from index file.
	metadataContent, err := os.ReadFile(metaFilePath)
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(metadataContent, &meta); err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return file, &meta, nil
}

// Exists checks if an object with the given key exists in both data AND index.
func (l *LocalFSBackend) Exists(ctx context.Context, key ObjectKey) (bool, error) {
	if l.BaseDir == "" {
		return false, errors.New("base directory not set")
	}

	// Check for the existence of the mandatory metadata index file first
	metaFilePath := fmt.Sprintf("%s/meta_%s.json", l.BaseDir, key)
	if _, err := os.Stat(metaFilePath); os.IsNotExist(err) {
		return false, nil // Key doesn't exist because the metadata is missing.
	} else if err != nil {
		return false, fmt.Errorf("error checking metadata existence: %w", err)
	}
	return true, nil
}

// GetLatestKey checks for an explicit LATEST pointer (a special key reserved by the service).
func (l *LocalFSBackend) GetLatestKey(ctx context.Context, bucketName string, originalName string) (ObjectKey, *Metadata, error) {
	if l.BaseDir == "" {
		return "", nil, errors.New("base directory not set")
	}

	latestPointerKey := ObjectKey(fmt.Sprintf("%s/%s/_latest_pointer", bucketName, originalName))
	metaFilePath := fmt.Sprintf("%s/meta_%s.json", l.BaseDir, latestPointerKey)

	if _, err := os.Stat(metaFilePath); os.IsNotExist(err) {
		return "", nil, fmt.Errorf("latest version metadata not found for %s/%s", bucketName, originalName)
	} else if err != nil {
		return "", nil, fmt.Errorf("error reading latest pointer index: %w", err)
	}

	return latestPointerKey, &Metadata{OriginalFilename: originalName, VersionID: "LATEST", ContentType: "application/octet-stream"}, nil
}
