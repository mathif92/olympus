package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mathif92/olympus/storage/pkg"
)

// ObjectHandler handles HTTP requests for object storage operations.
type ObjectHandler struct {
	Service *pkg.StorageService
}

// NewObjectHandler creates a new ObjectHandler with the given service.
func NewObjectHandler(service *pkg.StorageService) *ObjectHandler {
	return &ObjectHandler{
		Service: service,
	}
}

// parseObjectPath splits a /object/{bucket}/{key} path into its bucket and key
// components. It returns ok=false when the URL does not match that shape.
func parseObjectPath(path string) (bucket, key string, ok bool) {
	tail := strings.TrimPrefix(path, "/object/")
	if tail == "" {
		return "", "", false
	}
	seg := strings.SplitN(tail, "/", 2)
	if len(seg) < 2 {
		return "", "", false
	}
	return seg[0], seg[1], true
}

// HandleFunc returns an http.HandlerFunc that handles object storage requests.
func (h *ObjectHandler) HandleFunc(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		h.handlePutObject(w, r)
	} else if r.Method == http.MethodGet {
		h.handleGetObject(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePutObject handles PUT requests for uploading objects.
func (h *ObjectHandler) handlePutObject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	originalName := r.Header.Get("X-Object-Filename")

	if originalName == "" {
		http.Error(w, "Missing X-Object-Filename header", http.StatusBadRequest)
		return
	}

	// Extract bucket name from path: /object/{bucket}/{key}
	bucketName, _, ok := parseObjectPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid URL format. Expected /object/{bucket}/{key}", http.StatusBadRequest)
		return
	}

	// Check file write capability
	if err := pkg.CheckFileWriteCapability(); err != nil {
		http.Error(w, fmt.Sprintf("Permission denied: %v", err), http.StatusInternalServerError)
		return
	}

	// Determine version ID from header or default to LATEST
	versionID := r.Header.Get("X-Object-Version-Id")
	if versionID == "" {
		versionID = "LATEST"
	}

	// Generate object key
	key := pkg.GenerateObjectKey(bucketName, originalName, versionID)

	// Create metadata
	meta := pkg.Metadata{
		OriginalFilename: originalName,
		ContentType:      r.Header.Get("Content-Type"),
		LastModified:     time.Now().Format(time.RFC3339),
		VersionID:        versionID,
	}

	// Stream body directly to backend
	reader := r.Body
	defer reader.Close()

	etag, err := h.Service.Backend.StoreStream(ctx, key, meta, reader)
	if err != nil {
		http.Error(w, fmt.Sprintf("Storage failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Success response with ETag
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Object stored successfully!\nBucket: %s\nKey: %s\nVersionID: %s\nETag: %s", bucketName, key, versionID, etag)
}

// handleGetObject handles GET requests for downloading objects.
func (h *ObjectHandler) handleGetObject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract bucket name and object key from path: /object/{bucket}/{key}
	bucketName, keyPart, ok := parseObjectPath(r.URL.Path)
	if !ok {
		http.Error(w, "Invalid URL format. Expected /object/{bucket}/{key}", http.StatusBadRequest)
		return
	}

	// Reconstruct the same key the uploader derived (bucketName/originalName/LATEST).
	objectKey := pkg.ObjectKey(fmt.Sprintf("%s/%s/%s", bucketName, keyPart, "LATEST"))

	// Check if object exists
	exists, err := h.Service.Backend.Exists(ctx, objectKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Service error checking existence: %v", err), http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, "Object not found (Check bucket name and key)", http.StatusNotFound)
		return
	}

	// Retrieve the object content stream and metadata
	reader, meta, err := h.Service.Backend.Retrieve(ctx, objectKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Service error retrieving object: %v", err), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	if meta == nil || meta.ETag == "" {
		http.Error(w, "Warning: Metadata missing for this object, ETag will not be provided.", http.StatusPartialContent)
		return
	}
	w.Header().Set("ETag", meta.ETag)

	var modTime time.Time
	if meta.LastModified != "" {
		if parsed, perr := time.Parse(time.RFC3339, meta.LastModified); perr == nil {
			modTime = parsed
		}
	}

	// Stream data to the response (supports ranges and avoids buffering large files).
	http.ServeContent(w, r, "object", modTime, reader)
}
