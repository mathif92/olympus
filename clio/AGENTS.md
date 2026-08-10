# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Clio is the managed-relational-database service for the Olympus platform — an RDS-equivalent control plane implemented in Go. It manages database instances, engine/size catalogs, point-in-time snapshots, and an audit trail. Tenancy is modelled `accounts → projects → resources`, mirroring the Amphora, Paramdora, Hephaestus, and Orpheus services. The actual provisioning ("data plane") is pluggable behind a `Provisioner` interface; a mock implementation ships by default, and a real `docker` implementation launches an actual PostgreSQL container per managed instance via testcontainers/Docker.

## Architecture

```
cmd/app/main.go              HTTP server, health probe, goose migrations on startup
internal/handler/            HTTP handlers (/instances, /snapshots, ...)
pkg/
  ├── service.go             Control plane: instance & snapshot lifecycle, audit
  ├── provisioner.go         Mock data plane (in-memory, synthetic endpoints)
  ├── docker_provisioner.go  Real data plane (testcontainers PostgreSQL containers)
  ├── types.go               Provisioner interface, state constants, errors
  └── database/              Postgres client + goose migration runner
migrations/                  goose SQL migrations (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres + real engines)
```

### Key Components

1. **API Layer (`internal/handler/clio.go`)**: Registers routes for projects, engines, instance sizes, instance lifecycle `/instance/{p}/{name}` (GET/DELETE + `/start` + `/stop`), instance collection `/instances`, snapshot collection `/snapshots`, and snapshot actions `/snapshot/{p}/{i}/{name}` (DELETE). Tenant is resolved from the `X-Account-Id` header (default `default`) and auto-provisioned via `ensureAccount`.

2. **Control Plane (`pkg/service.go`)**: All state transitions go through the service:
   - `CreateInstance()`: validates engine/version and size against the seeded catalogs, generates a strong master password (returned **once**, never persisted), writes the instance (`state=creating`), calls `Provisioner.CreateInstance`, then flips to `active` with endpoint + master username + provider ref
   - `DeleteInstance()`: delegates to the provisioner then marks `deleted`; already-deleted instances reject with `ErrConflict`
   - `StartInstance()`/`StopInstance()`: require `stopped`/`active` respectively, delegate to the provisioner, then flip state
   - `CreateSnapshot()`: requires an `active` or `stopped` instance, delegates to the provisioner, records size + provider ref
   - `DeleteSnapshot()`: marks `deleted` after provisioner teardown
   - Every lookup is scoped by `account_id` — tenants can never see or mutate another's resources

3. **Provisioner (`pkg/types.go`)** and implementations:
   - `MockProvisioner` (`pkg/provisioner.go`) keeps no external state, emits synthetic `x.x.x.x:5432` endpoints, echoes the credentials it was given.
   - `DockerProvisioner` (`pkg/docker_provisioner.go`) launches a real `postgres:16-alpine` container per instance via the testcontainers postgres module; `ProviderRef` = `inst-<name>`, endpoint = the container's mapped `host:port`. Snapshots run a real `pg_dump -Fc` inside the container. main.go selects it when `PROVISIONER=docker`.

4. **Database (`pkg/database/`)**: `Client` wraps a pooled `*sql.DB`. `Migrate()`/`Rollback()` run goose migrations.

### Database Schema

- `accounts`: tenants with `instance_limit` / `used_instances`
- `projects`: namespaces, unique on `(account_id, name)`, cached `instance_count`
- `database_engines`: catalog (seeded: postgres 16.8/15.12 active, postgres 14.17 deprecated, mysql 8.4.4 active)
- `instance_sizes`: catalog (seeded: clio-nano/small/medium/large)
- `db_instances`: unique `(project_id, name)`, state machine `pending/creating/active/stopping/stopped/starting/deleting/deleted`, stores `endpoint`, `master_username`, `provider_ref`
- `db_snapshots`: unique `(instance_id, name)`, `size_gb`, `provider_ref` from the data plane
- `audit_logs`: written by the HTTP handler after each successful operation; `project_id` is `NULL` when not resolvable (insert via `NULLIF($2,'')` to avoid FK violations)

## Commands

### Build and Run

```bash
make build
make up && make run            # Postgres on host 15436, service on :8087
POSTGRES_DSN='...' PROVISIONER=mock go run ./cmd/app
```

Real database data plane:

```bash
PROVISIONER=docker POSTGRES_DSN='...' go run ./cmd/app
```

### Tests

```bash
make fmt && make vet
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
RUN_DOCKER_TESTS=1 go test -v ./tests/integration/ -run TestDockerProvisionerRealDatabases   # live engines
```

### Database Migrations (goose)

```bash
goose -dir migrations create add_column sql
goose -dir migrations postgres "host=localhost port=15436 user=olympus password=olympus_secret dbname=olympus_databases sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15436 user=olympus password=olympus_secret dbname=olympus_databases sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects`, `GET /projects`
- `GET /engines`, `GET /instance-sizes`
- `POST /instances`, `GET /instances?project={p}`
- `GET /instance/{p}/{name}`, `POST /instance/{p}/{name}/start`, `POST /instance/{p}/{name}/stop`, `DELETE /instance/{p}/{name}`
- `POST /snapshots`, `GET /snapshots?project={p}&instance={i}`
- `DELETE /snapshot/{p}/{i}/{name}`
- `GET /health`

All requests carry the tenant in the `X-Account-Id` header.

## Important Implementation Details

- Instance/snapshot states are plain strings in `pkg.State*` constants; transitions are guarded (`ErrConflict` for delete-after-delete).
- `account_id` scoping lives in `resolveProject`; calling `GetInstance(account, ...)` from another tenant returns `ErrNotFound` (map `sql.ErrNoRows` via `pkg.IsNotFound`).
- The snapshot URL path is `/snapshot/{project}/{instance}/{name}` — three segments, parsed with `splitPath` then `splitTwo`. It must NOT be parsed with `splitNameAction` (that is for the two-segment `/instance/{p}/{name}[/action]` form). Use `splitTwo`.
- Instances return the master password **exactly once** at creation (`full.MasterPassword = masterPassword` set in service, `json:"master_password,omitempty"`). Never persist it or log it.
- The `docker`/`mock` provisioners are interchangeable per `pkg.Provisioner`; never make the control plane depend on `DockerProvisioner` specifics.
- `instance_count` on projects is refreshed from `COUNT(*)` (excluding `deleted`) after instance create/delete.
- Audit (`pkg.Audit`) is invoked from the HTTP handler layer, not the service layer.
- Aliases in PlantUML diagrams must not clash with package names (old local PlantUML builds reject that). Use `skinparam backgroundColor white` (no `!theme`).

### Snapshot size granularity

`pg_dump` output for a small database is far smaller than one GiB, so
`CreateSnapshot` reports a floor of `1` GB (`if sizeGB == 0 { sizeGB = 1 }`).
Do not change that without also updating tests that assert `SizeGB > 0`.

## Testing the real data plane (RUN_DOCKER_TESTS)

`TestDockerProvisionerRealDatabases` gated by `RUN_DOCKER_TESTS=1`. It
launches a real container, opens a `database/sql` connection to the mapped
endpoint, executes DDL/DML against it, snapshots via `pg_dump`, and exercises
stop/start. Requires a Docker daemon reachable by testcontainers.