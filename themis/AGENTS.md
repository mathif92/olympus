# Themis — IAM service

## Overview

Themis provides multi-tenant identity and access management for the Olympus
platform: users, groups, roles, JSON policies, AWS-style access keys, HS256 JWT
minting, and a policy-evaluation endpoint. It follows the same layout as the
other services (Paramdora/Clio/Mneme).

## Layout

```
cmd/app/main.go            # flags, db client, goose migrations, JWT secret, HTTP
internal/handler/handler.go# route table + request decoding/encoding
pkg/database/              # connection pool, goose migrate, structs
pkg/service.go             # business logic: users/groups/roles/policies/keys
pkg/policy.go              # policy document parsing + evaluation
pkg/jwt.go                 # minimal HS256 sign/verify (stdlib only)
migrations/                # goose SQL migrations (00001..00010)
tests/integration/         # testcontainers end-to-end tests
```

## Conventions

- Every request reads the tenant from `X-Account-Id` (default `default`); all
  rows are scoped by `account_id`/`project_id`. Isolation is enforced by every
  query resolving the project first (`resolveProject`).
- Path style mirrors the other services: `/users?project=x` for lists,
  `/user/{name}?project=x` for single-resource ops, with nested `keys` and
  `members` sub-paths.
- Access key secrets are generated (`AKIA` + 16 chars id, 40-char secret) and
  only the SHA-256 hex hash is persisted; the plaintext secret is returned once
  at creation (`Secret` field is `json:"secret_access_key,omitempty"`).
- Policy documents are validated by `ParsePolicyDocument` (must have ≥1
  Statement; Effect must be Allow/Deny). Evaluation is
  implicit-deny by default, explicit deny overrides allow, wildcards use
  trailing-`*` prefix matching.
- `THEMIS_JWT_SECRET` controls token signing; if unset a random one-shot secret
  is generated (tokens become unverifiable after restart). The Makefile defaults
  it to `dev-secret-change-me` so the other services (shared `authz` middleware)
  can verify tokens in local dev — keep it in sync everywhere.
- `Authorize` resolves the project by name **or** ID (JWT claims carry the
  project ID), always scoped to the calling account (`X-Account-Id`).
- Audit rows are best-effort (`_ = s.Audit(...)`); they never fail a request.

## Commands

```bash
make up          # docker compose postgres (host :15439)
make run         # go run ./cmd/app (listens :8091)
make test-it     # integration suite via testcontainers (needs Docker)
make vet         # go vet ./...
```

## Adding a resource

1. Add a migration in `migrations/` (numbered, `-- +goose Up/Down`).
2. Add the row struct to `pkg/database/structs.go`.
3. Add CRUD to `pkg/service.go` (create → resolve project → insert → update
   `resource_count`; delete → guard `RowsAffected` → `ErrNotFound`).
4. Register routes in `internal/handler/handler.go` and reuse `tail()` for path
   parsing; keep `writeJSON`/`writeErr`/`accountName` helpers shared.
5. Cover it in `tests/integration/service_test.go`.
