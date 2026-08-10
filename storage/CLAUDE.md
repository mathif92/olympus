# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OlympusStore is an on-premise, S3-compatible object storage service implemented in Go. It provides HTTP-based object storage with streaming I/O capabilities for efficient handling of large files.

## Architecture

The application follows a tiered architecture:

```
cmd/app/
  └── main.go          # HTTP server entry point with route handlers
pkg/
  ├── local_fs_backend.go  # Filesystem-backed storage implementation
  ├── storage_service.go   # Business logic layer (key generation, orchestration)
  └── types.go             # Shared types (ObjectKey, Metadata, StorageBackend interface)
tests/
  └── fixtures/           # Test fixtures and test data
```

### Key Components

1. **API Layer (`main.go`)**: Handles HTTP request routing for `/object/{bucket}/{key}` endpoints (PUT for upload, GET for download). Extracts bucket name and object key from URL path.

2. **Service Layer (`pkg/storage_service.go`)**: Contains core business logic including:
   - Object key generation with versioning support
   - Key format: `bucket/originalName[:versionId]`
   - Orchestrates backend operations

3. **Storage Backend (`pkg/local_fs_backend.go`)**: Implements `StorageBackend` interface for filesystem operations:
   - `StoreStream()`: Writes object data via streaming, calculates SHA256 hash for ETag, saves metadata atomically
   - `Retrieve()`: Fetches object data and metadata from index
   - `Exists()`: Checks object existence via metadata index
   - Uses temporary files with atomic rename for safe writes

4. **Types**:
   - `ObjectKey`: String type representing unique object identifier
   - `Metadata`: Contains OriginalFilename, ContentType, ContentLength, LastModified, ETag, VersionID
   - `StorageBackend` interface: Defines contract for any storage backend

## Commands

### Build and Run

```bash
# Build the application
go build -o olympus-store ./cmd/app

# Run the application (listens on :8080)
go run ./cmd/app

# Run with custom storage directory
go run ./cmd/app -storage-dir /path/to/storage
```

### Tests

```bash
# Run all tests
go test ./...

# Run a specific test
go test -v -run TestObjectUploadStreaming ./cmd/app/

# Run tests with coverage
go test -cover ./...

# Run a single test file
go test -v ./cmd/app/test/

# Run integration tests (spins up real PostgreSQL + Redis + MinIO via testcontainers; requires Docker)
go test -v ./tests/integration/

# Optionally point migrations elsewhere
OLYMPUS_MIGRATIONS_DIR=/path/to/migrations go test ./tests/integration/
```

### Local execution (make + docker compose)

`make up` boots Postgres (host `15432`), Redis (`16379`) and MinIO (`9000/9001`)
via docker compose; `make run` then starts the gateway against them:

```bash
make up          # start the compose stack
make run         # STORAGE_BACKEND=minio go run ./cmd/app (listening on :8080)
make infra-down  # stop the stack
make test-it     # testcontainers integration suite (needs Docker)
```

Maintenance one-shots (run against `STORAGE_BACKEND=minio`):

```bash
make reconcile          # report only, no changes
make reconcile-repair   # -reconcile -reconcile-mark-missing -reconcile-drift
make backfill-etag      # stamp x-olympus-etag metadata on pre-existing objects
```

A reconcile `CronJob` (daily, `reconciliation.*` in the Helm values) is rendered
by `deploy/helm/olympus/templates/reconcile-cronjob.yaml`.

### Database Migrations (goose)

Migrations live in `migrations/` as numbered SQL files with `-- +goose Up` and
`-- +goose Down` blocks. They run automatically at app startup and are applied
by the integration tests:

```bash
# Apply pending migrations via the running service (it does this on startup),
# or use the goose CLI:
goose -dir migrations postgres "host=localhost dbname=olympus_storage user=olympus password=olympus_secret sslmode=disable" up

# Roll back the last migration
goose -dir migrations postgres "host=localhost dbname=olympus_storage user=olympus password=olympus_secret sslmode=disable" down

# Add a new migration
goose -dir migrations create add_columns sql
```

Note: the migrations are no longer mounted into the Postgres container's
`/docker-entrypoint-initdb.d`; goose owns schema management.

### Development Workflow

```bash
# Clean build
go clean
go build ./cmd/app

# Install dependencies (if any)
go mod download

# Format code
go fmt ./...

# Vet for issues
go vet ./...
```

## Important Implementation Details

### Streaming I/O Pattern

The application uses Go's streaming capabilities to handle large files efficiently:
- Uses `io.Reader` and `io.Writer` interfaces
- Employs `io.MultiWriter` to simultaneously write to file and hasher
- Uses `sha256.New()` with `io.TeeReader`/`io.Copy` for hash calculation
- Writes to temporary files first, then atomically renames to final location

### Object Key Structure

Keys are generated as: `bucket/originalName[:versionId]`
- Example: `test-images/profile_pic.png/LATEST`
- Version ID defaults to "LATEST" if not specified
- Used for versioning support

### Metadata Storage

Each object has an associated metadata file at: `{baseDir}/meta_{key}.json`
Metadata includes:
- key: The object key
- etag: SHA256 hash of content
- meta_type: Backend type identifier

### HTTP Endpoints

- `PUT /object/{bucket}/{key}`: Upload object (requires `X-Object-Filename` header)
- `GET /object/{bucket}/{key}`: Download object
- Response includes ETag header for uploaded objects

## Testing Notes

The test suite uses a fixtures directory at `tests/fixtures/`. Tests verify:
- Upload/download cycle with streaming I/O
- Metadata integrity (ETag matching)
- Non-existent object handling
- Versioning with LATEST pointer

**Test fixes applied:**
- `StoreStream()` now creates parent directories for object paths before writing
- `writeMetadataIndex()` creates parent directory for metadata files
- `Retrieve()` properly reads and parses metadata from index file instead of returning mock data
