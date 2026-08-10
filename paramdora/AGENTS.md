# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Paramdora is a multi-tenant, Parameter-Store-style service for the Olympus platform, implemented in Go. It provides HTTP-based storage of named parameters (plain strings, string lists, and AES-256-GCM encrypted secure strings) with full version history and an audit trail. Tenancy is modelled `accounts → projects → parameters`, mirroring the Storage service.

## Architecture

The service follows the same tiered layout as the Storage service:

```
cmd/app/main.go          HTTP server entry point, health probe, goose migrations on startup
internal/handler/        HTTP handlers (/parameter, /parameters, /projects, /health)
pkg/
  ├── service.go         Business logic: multi-tenant CRUD, versioning, history
  ├── encrypt.go         AES-256-GCM cipher for secure_string values
  └── database/          Postgres client + goose migration runner
migrations/              goose SQL migrations (applied automatically at startup)
tests/integration/       testcontainers-backed tests (Postgres)
```

### Key Components

1. **API Layer (`internal/handler/parameter.go`)**: Routes `PUT/GET/DELETE /parameter/{project}/{name}`, `GET /parameters/{project}?prefix=`, and `POST/GET /projects`. Tenant is resolved from the `X-Account-Id` header (defaults to `default`) and auto-provisioned on first request.

2. **Service Layer (`pkg/service.go`)**: Core business logic:
   - `PutParameter()`: upserts a parameter, bumping `version` on conflict and appending an immutable row to `parameter_versions`
   - `GetParameter()`: reads a parameter; secure values are returned empty unless `decrypt=true`
   - `ListParameters()`, `DeleteParameter()`, `GetParameterHistory()`
   - `EnsureAccount()` / `CreateProject()`: tenant and namespace provisioning
   - `Audit()`: writes the audit trail
   - All lookups are scoped by `account_id`, so tenants can never read or write each other's projects

3. **Encryption (`pkg/encrypt.go`)**: `Cipher` seals `secure_string` values with AES-256-GCM. The master key comes from `PARAMDORA_MASTER_KEY` (or `KEY_ENC_MASTER`); an empty key yields a random one-shot key so secrets are unreadable after restart.

4. **Database (`pkg/database/`)**: `Client` wraps a pooled `*sql.DB` for PostgreSQL. `Migrate()`/`Rollback()` run the goose migrations.

### Database Schema

- `accounts`: tenants; `plan`, `parameter_limit`, `used_parameters`
- `projects`: named namespaces per account; `parameter_count`, unique on `(account_id, name)`
- `parameters`: versioned entries; unique on `(project_id, name)`, `is_encrypted`, `key_id`, `tier`, JSONB `tags`
- `parameter_versions`: immutable history (FK to `parameters`, cascade delete)
- `audit_logs`: audit trail (FK to `projects`, `ON DELETE SET NULL`)

## Commands

### Build and Run

```bash
# Build the service
make build

# Run against the local docker-compose stack (Postgres on host 15433), listening on :8083
make up && make run

# Run with a persistent encryption key
PARAMDORA_MASTER_KEY='change-me' go run ./cmd/app

# Custom Postgres
POSTGRES_DSN='host=... user=... password=... dbname=olympus_parameters sslmode=disable' go run ./cmd/app
```

### Tests

```bash
make fmt && make vet   # formatting + static analysis
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
```

### Database Migrations (goose)

Migrations live in `migrations/` as numbered SQL files with `-- +goose Up` and
`-- +goose Down` blocks, applied automatically at startup:

```bash
# Add a new migration
goose -dir migrations create add_column sql

# Apply / roll back via the goose CLI
goose -dir migrations postgres "host=localhost port=15433 user=olympus password=olympus_secret dbname=olympus_parameters sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15433 user=olympus password=olympus_secret dbname=olympus_parameters sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects` / `GET /projects`: create / list project namespaces
- `PUT /parameter/{project}/{name}`: create or update a parameter (body `{value, type, description, tier, tags}`)
- `GET /parameter/{project}/{name}`: read a parameter (`?decrypt=true` returns secure values, `?history=true` returns all versions)
- `DELETE /parameter/{project}/{name}`: delete a parameter and its history
- `GET /parameters/{project}?prefix=`: list parameters with an optional name-prefix filter
- `GET /health`: PostgreSQL connectivity probe

All requests carry the tenant in the `X-Account-Id` header.

## Important Implementation Details

- Parameter `data_type` values: `string`, `string_list`, `secure_string` (default when empty is `secure_string`).
- `secure_string` values are stored as base64 AES-GCM ciphertext; plaintext is never persisted, masked by default on read, and decryptable only with `?decrypt=true`.
- Versioning: `PutParameter` generates a fresh id that is ignored on conflict (the row keeps its original id — always read the `id` returned from `RETURNING`), increments `parameters.version`, and appends to `parameter_versions`.
- `parameter_count` on `projects` is recalculated after each put/delete with a `COUNT(*)` so it never drifts.
- Aliases in PlantUML diagrams must not clash with package names (old local PlantUML builds reject that).