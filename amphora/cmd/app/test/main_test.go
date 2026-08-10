package main_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/mathif92/olympus/amphora/pkg"
	// Must use os package for file system manipulation in the test setup/teardown
)

const testDir = "../../../tests"

func init() {
	// Setup phase: Ensure the main data directory exists for testing.
	if err := os.MkdirAll(testDir, 0755); err != nil {
		panic("Could not create test storage directory: " + err.Error())
	}
	// Cleanup phase is handled by the defer at the end of the test run.
}

// TestObjectUploadStreaming simulates a full PUT request flow to test the core service functionality using streaming I/O, buckets, and versions.
func TestObjectUploadStreaming(t *testing.T) {
	// --- Setup ---
	fixturesDir := fmt.Sprintf("%s/fixtures", testDir)
	backend := pkg.NewLocalFSBackend(fixturesDir)
	service := pkg.NewStorageService(backend)

	bucketName := "test-images"
	originalFilename := "profile_pic.png"
	versionID := "LATEST" // Test latest versioning
	payload := []byte("This is fake PNG data.")

	// --- Execution ---
	ctx := context.Background()

	key := pkg.GenerateObjectKey(bucketName, originalFilename, versionID)

	meta := pkg.Metadata{
		OriginalFilename: originalFilename,
		ContentType:      "image/png",
		ContentLength:    int64(len(payload)),
		LastModified:     time.Now().Format(time.RFC3339),
		VersionID:        versionID,
	}

	// 1. Execute the streamed storage write operation.
	etag, err := service.Backend.StoreStream(ctx, key, meta, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Failed to store object via streaming: %v", err)
	}

	// 2. Test retrieval immediately after writing
	retrievedReader, retrievedMeta, err := service.Backend.Retrieve(ctx, key)
	if err != nil {
		t.Fatalf("Failed to retrieve object: %v", err)
	}
	defer retrievedReader.Close()
	retrievedBytes, err := io.ReadAll(retrievedReader)
	if err != nil {
		t.Fatalf("Failed to read retrieved stream: %v", err)
	}

	// Validation checks
	if !bytes.Equal(payload, retrievedBytes) {
		t.Errorf("Retrieved data does not match original payload.\nExpected length: %d\nGot length: %d", len(payload), len(retrievedBytes))
	}

	// Metadata integrity check
	expectedETag := etag

	if retrievedMeta == nil || retrievedMeta.ETag != expectedETag {
		t.Errorf("Metadata mismatch! Expected ETag %s, got ETag from retrieval: %s (or nil)", expectedETag, retrievedMeta.ETag)
	}

	// 3. Test non-existent object read
	nonExistentKey := pkg.ObjectKey("definitely-not-here")
	_, _, err = service.Backend.Retrieve(ctx, nonExistentKey)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("object not found")) {
		t.Errorf("Expected retrieval to fail for a non-existent key, but succeeded or failed unexpectedly: %v", err)
	}

	t.Log("✅ Streaming object upload and download cycle test passed successfully!")
}
