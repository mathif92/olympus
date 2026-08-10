# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Hephaestus is the compute service for the Olympus platform — an EC2-equivalent control plane implemented in Go. It manages virtual compute instances, instance types, block volumes, snapshots, SSH key pairs, security groups, and an audit trail. Tenancy is modelled `accounts → projects → resources`, mirroring the Storage and Paramdora services. The actual computation ("data plane") is pluggable behind a `Provisioner` interface; a mock implementation ships by default.

## Architecture

```
cmd/app/main.go              HTTP server, health probe, goose migrations on startup
internal/handler/            HTTP handlers (/instances, /volumes, /snapshots, ...)
pkg/
  ├── service.go             Control plane: instance lifecycle, volumes, keys, groups, audit
  ├── provisioner.go         Mock data plane (Launch/Start/Stop/Terminate/Healthy)
  ├── types.go               Provisioner interface, instance-state constants, errors
  └── database/              Postgres client + goose migration runner
migrations/                  goose SQL migrations (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres)
```

### Key Components

1. **API Layer (`internal/handler/compute.go`)**: Registers routes for projects, instance types, instance lifecycle (`/instance/{p}/{name}/(start|stop|terminate)`), volumes, snapshots, key pairs, and security groups. Tenant is resolved from the `X-Account-Id` header (default `default`) and auto-provisioned via `ensureAccount`.

2. **Control Plane (`pkg/service.go`)**: All state transitions go through the service:
   - `LaunchInstance()`: validates type + key pair, writes instance (`state=pending`), creates an attached boot volume, calls `Provisioner.Launch`, then flips to `running` with IPs/provider ref
   - `StartInstance` / `StopInstance` / `TerminateInstance`: guarded state transitions; terminated instances reject further starts with `ErrConflict`
   - `CreateVolume` / `DeleteVolume`: delete is rejected while `instance_id` is set (in-use)
   - `CreateSnapshot`: copies size from the source volume
   - `CreateKeyPair`: generates RSA-2048; only the public key + fingerprint are persisted, the private key PEM is returned exactly once
   - `CreateSecurityGroup`: JSONB `rules` (default `[{port:22,cidr:"0.0.0.0/0"}]`)
   - Every lookup is scoped by `account_id` — tenants can never see or mutate another's resources

3. **Provisioner (`pkg/types.go` + `pkg/provisioner.go`)**: The `Provisioner` interface (`Launch`, `Start`, `Stop`, `Terminate`, `Healthy`) is the only boundary between the control plane and the data plane. `MockProvisioner` keeps per-ref state in memory and synthesizes `10.x.x.x` IPs. A real hypervisor backend must implement the same interface; main.go selects it via the `PROVISIONER` env var.

4. **Database (`pkg/database/`)**: `Client` wraps a pooled `*sql.DB`. `Migrate()`/`Rollback()` run goose migrations.

### Database Schema

- `accounts`: tenants with `instance_limit` / `used_instances`
- `projects`: namespaces, unique on `(account_id, name)`, cached `instance_count`
- `instance_types`: catalog (seeded: olympus-nano/small/medium/large)
- `instances`: unique `(project_id, name)`, state machine `pending/running/stopped/terminated`, `provider_ref` from the data plane
- `volumes`: unique `(project_id, name)`, nullable `instance_id` (in-use flag)
- `snapshots`: unique `(project_id, name)`, copy of source volume size
- `key_pairs`: unique `(project_id, name)`; only `public_key` + `fingerprint` are stored
- `security_groups`: unique `(project_id, name)`, JSONB `rules`
- `audit_logs`: written by the HTTP handler after each successful operation

## Commands

### Build and Run

```bash
make build
make up && make run            # Postgres on host 15434, service on :8084
POSTGRES_DSN='...' PROVISIONER=mock go run ./cmd/app
```

### Tests

```bash
make fmt && make vet
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
```

### Database Migrations (goose)

```bash
goose -dir migrations create add_column sql
goose -dir migrations postgres "host=localhost port=15434 user=olympus password=olympus_secret dbname=olympus_compute sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15434 user=olympus password=olympus_secret dbname=olympus_compute sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects`, `GET /projects`
- `GET /types`
- `POST /instances`, `GET /instances?project={p}`
- `GET /instance/{p}/{name}`, `POST /instance/{p}/{name}/(start|stop|terminate)`, `DELETE /instance/{p}/{name}`
- `POST /keypairs`, `GET /keypairs/{p}`
- `POST /security-groups`, `GET /security-groups/{p}`
- `POST /volumes`, `GET /volumes?project={p}`, `DELETE /volume/{p}/{name}`
- `POST /snapshots`, `GET /snapshots?project={p}`
- `GET /health`

All requests carry the tenant in the `X-Account-Id` header.

## Important Implementation Details

- Instance states are plain strings in `pkg.State*` constants; transitions are guarded (`ErrConflict` for start-after-terminate).
- `account_id` scoping lives in `resolveProject`; calling `GetInstance(account, ...)` from another tenant returns `ErrNotFound` (map `sql.ErrNoRows` via `pkg.IsNotFound`).
- Private IPs and provider refs come from the mock provisioner and are NOT persisted on launch failure (the instance stays `pending`).
- `instance_count` on projects is refreshed from `COUNT(*)` after launch/terminate.
- Do not persist private keys — `CreateKeyPair` returns the PEM once and stores only the public material.
- Audit (`pkg.Audit`) is invoked from the HTTP handler layer, not the service layer.
- Aliases in PlantUML diagrams must not clash with package names (old local PlantUML builds reject that).