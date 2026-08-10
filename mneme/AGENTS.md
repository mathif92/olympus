# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Mneme is the managed in-memory caching service for the Olympus platform — an ElastiCache-equivalent control plane implemented in Go. It manages cache clusters, engine/node-type catalogs, point-in-time snapshots, and an audit trail. Tenancy is modelled `accounts → projects → resources`, mirroring the Amphora, Paramdora, Hephaestus, Orpheus, and Clio services. The actual provisioning ("data plane") is pluggable behind a `Provisioner` interface; a mock implementation ships by default, and a real `docker` implementation launches an actual Redis container per managed cluster via testcontainers/Docker.

## Architecture

```
cmd/app/main.go              HTTP server, health probe, goose migrations on startup
internal/handler/            HTTP handlers (/clusters, /snapshots, ...)
pkg/
  ├── service.go             Control plane: cluster & snapshot lifecycle, audit
  ├── provisioner.go         Mock data plane (in-memory, synthetic endpoints)
  ├── docker_provisioner.go  Real data plane (testcontainers Redis containers)
  ├── types.go               Provisioner interface, state constants, errors
  └── database/              Postgres client + goose migration runner
migrations/                  goose SQL migrations (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres + real engines)
```

### Key Components

1. **API Layer (`internal/handler/mneme.go`)**: Registers routes for projects, engines, node types, cluster lifecycle `/cluster/{p}/{name}` (GET/DELETE), cluster collection `/clusters`, snapshot collection `/snapshots`, and snapshot actions `/snapshot/{p}/{c}/{name}` (DELETE). Tenant is resolved from the `X-Account-Id` header (default `default`) and auto-provisioned via `ensureAccount`.

2. **Control Plane (`pkg/service.go`)**: All state transitions go through the service:
   - `CreateCluster()`: validates engine/version and node type against the seeded catalogs, writes the cluster (`state=creating`), calls `Provisioner.CreateCluster`, then flips to `active` with endpoint + provider ref
   - `DeleteCluster()`: delegates to the provisioner then marks `deleted`; already-deleted clusters reject with `ErrConflict`
   - `CreateSnapshot()`: requires an `active` cluster, delegates to the provisioner, records size + provider ref
   - `DeleteSnapshot()`: marks `deleted` after provisioner teardown
   - Every lookup is scoped by `account_id` — tenants can never see or mutate another's resources

3. **Provisioner (`pkg/types.go`)** and implementations:
   - `MockProvisioner` (`pkg/provisioner.go`) keeps no external state, emits synthetic `x.x.x.x:6379` endpoints.
   - `DockerProvisioner` (`pkg/docker_provisioner.go`) launches a real `redis:7-alpine` container per cluster via the testcontainers redis module (v0.43.0 — the redis module is NOT in v0.44.0); `ProviderRef` = `cache-<name>`, endpoint = the container's mapped `host:port`. Snapshots run `redis-cli SAVE` inside the container and stream out `/data/dump.rdb`. main.go selects it when `PROVISIONER=docker`.

4. **Database (`pkg/database/`)**: `Client` wraps a pooled `*sql.DB`. `Migrate()`/`Rollback()` run goose migrations.

### Database Schema

- `accounts`: tenants with `cluster_limit` / `used_clusters`
- `projects`: namespaces, unique on `(account_id, name)`, cached `cluster_count`
- `cache_engines`: catalog (seeded: redis 7.4/7.2 active, redis 6.2 deprecated, memcached 1.6.22 active)
- `node_types`: catalog (seeded: mneme-micro/small/medium/large)
- `cache_clusters`: unique `(project_id, name)`, state machine `pending/creating/active/deleting/deleted`, stores `endpoint`, `provider_ref`
- `cache_snapshots`: unique `(cluster_id, name)`, `size_mb`, `provider_ref` from the data plane
- `audit_logs`: written by the HTTP handler after each successful operation; `project_id` is `NULL` when not resolvable (insert via `NULLIF($2,'')` to avoid FK violations)

## Commands

### Build and Run

```bash
make build
make up && make run            # Postgres on host 15437, service on :8088
POSTGRES_DSN='...' PROVISIONER=mock go run ./cmd/app
```

Real cache data plane:

```bash
PROVISIONER=docker POSTGRES_DSN='...' go run ./cmd/app
```

### Tests

```bash
make fmt && make vet
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
RUN_DOCKER_TESTS=1 go test -v ./tests/integration/ -run TestDockerProvisionerRealRedis   # live engines
```

### Database Migrations (goose)

```bash
goose -dir migrations create add_column sql
goose -dir migrations postgres "host=localhost port=15437 user=olympus password=olympus_secret dbname=olympus_caches sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15437 user=olympus password=olympus_secret dbname=olympus_caches sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects`, `GET /projects`
- `GET /engines`, `GET /node-types`
- `POST /clusters`, `GET /clusters?project={p}`
- `GET /cluster/{p}/{name}`, `DELETE /cluster/{p}/{name}`
- `POST /snapshots`, `GET /snapshots?project={p}&cluster={c}`
- `DELETE /snapshot/{p}/{c}/{name}`
- `GET /health`

All requests carry the tenant in the `X-Account-Id` header.

## Important Implementation Details

- Cluster/snapshot states are plain strings in `pkg.State*` constants; transitions are guarded (`ErrConflict` for delete-after-delete).
- `account_id` scoping lives in `resolveProject`; calling `GetCluster(account, ...)` from another tenant returns `ErrNotFound` (map `sql.ErrNoRows` via `pkg.IsNotFound`).
- The snapshot URL path is `/snapshot/{project}/{cluster}/{name}` — three segments, parsed with `splitPath` then `splitTwo`.
- The `docker`/`mock` provisioners are interchangeable per `pkg.Provisioner`; never make the control plane depend on `DockerProvisioner` specifics.
- `cluster_count` on projects is refreshed from `COUNT(*)` (excluding `deleted`) after cluster create/delete.
- Audit (`pkg.Audit`) is invoked from the HTTP handler layer, not the service layer.
- Aliases in PlantUML diagrams must not clash with package names (old local PlantUML builds reject that). Use `skinparam backgroundColor white` (no `!theme`).
- The testcontainers redis module exists only up to v0.43.0; if you bump other testcontainers deps, pin `modules/redis@v0.43.0` back explicitly or `go get` will silently downgrade it.
- `go mod init` must run inside `mneme/` — if a `go.mod` appears at the repo root, it was created in the wrong directory and must be moved into the service dir.

### Snapshot size granularity

An RDB dump for an empty cache is far smaller than one MiB, so `CreateSnapshot`
reports a floor of `1` MB (`if sizeMB == 0 { sizeMB = 1 }`). Do not change that
without also updating tests that assert `SizeMB > 0`.

### Docker provisioner engine support

`DockerProvisioner.CreateCluster` only supports `engine == "redis"` and
returns an error for anything else (the mock supports the full catalog
including memcached). Do not silently extend this without adding a corresponding
real engine image + snapshot flow.

## Testing the real data plane (RUN_DOCKER_TESTS)

`TestDockerProvisionerRealRedis` gated by `RUN_DOCKER_TESTS=1`. It launches a
real container, opens a `go-redis/v9` client to the mapped endpoint, `SET`/`GET`
keys, snapshots via `SAVE` → `dump.rdb`, and cleans up. Requires a Docker
daemon reachable by testcontainers.