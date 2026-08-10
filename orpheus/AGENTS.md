# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Orpheus is the managed-Kubernetes service for the Olympus platform — an EKS-equivalent control plane implemented in Go. It manages Kubernetes clusters, worker node groups, a kubeconfig service, and an audit trail. Tenancy is modelled `accounts → projects → resources`, mirroring the Amphora, Paramdora, and Hephaestus services. The actual provisioning ("data plane") is pluggable behind a `Provisioner` interface; a mock implementation ships by default, and a real `kube` implementation provisions namespaces and node-group Deployments through a live Kubernetes API via `KUBECONFIG`.

## Architecture

```
cmd/app/main.go              HTTP server, health probe, goose migrations on startup
internal/handler/            HTTP handlers (/clusters, /nodegroups, ...)
pkg/
  ├── service.go             Control plane: cluster & node-group lifecycle, kubeconfig, audit
  ├── provisioner.go         Mock data plane (in-memory, synthetic endpoints)
  ├── k8s_provisioner.go     Real data plane (client-go: namespaces + Deployments)
  ├── kubeconfig.go          kubeconfig document renderer
  ├── types.go               Provisioner interface, state constants, errors
  └── database/              Postgres client + goose migration runner
migrations/                  goose SQL migrations (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres + K3s)
```

### Key Components

1. **API Layer (`internal/handler/orpheus.go`)**: Registers routes for projects, Kubernetes versions, node sizes, cluster lifecycle `/cluster/{p}/{name}` (GET/DELETE + `/kubeconfig`), node-group collection `/nodegroups`, and node-group actions `/nodegroup/{p}/{c}/{name}` (scale/DELETE). Tenant is resolved from the `X-Account-Id` header (default `default`) and auto-provisioned via `ensureAccount`.

2. **Control Plane (`pkg/service.go`)**: All state transitions go through the service:
   - `CreateCluster()`: validates the Kubernetes version against the seeded catalog, writes the cluster (`state=creating`), calls `Provisioner.CreateCluster`, then flips to `active` with endpoint, CA data, and a rendered kubeconfig
   - `DeleteCluster()`: delegates to the provisioner then marks `deleted`; already-deleted clusters reject with `ErrConflict`
   - `CreateNodeGroup()`: requires an active cluster, default sizes (`min`/`desired`/`max` → 1/1/desired), delegates to the provisioner
   - `ScaleNodeGroup()`: bounded by the node group's configured min/max; out-of-bounds desired sizes are rejected
   - `DeleteNodeGroup()`: marks `deleted` after provisioner teardown
   - `ClusterKubeconfig()`: returns the kubeconfig rendered at cluster creation
   - Every lookup is scoped by `account_id` — tenants can never see or mutate another's resources

3. **Provisioner (`pkg/types.go`)** and implementations:
   - `MockProvisioner` (`pkg/provisioner.go`) keeps no external state, emits synthetic `https://x.x.x.1:6443` endpoints and mock CA data.
   - `KubeProvisioner` (`pkg/k8s_provisioner.go`) builds a client-go client from a kubeconfig path; each managed cluster is a dedicated `or-{name}` **namespace** on the target cluster, each node group is a `ng-{name}` **Deployment** whose replica count is the desired node count. Scaling calls the real Kubernetes API. main.go selects it when `PROVISIONER=kube` and requires `ORPHEUS_KUBECONFIG`.

4. **Database (`pkg/database/`)**: `Client` wraps a pooled `*sql.DB`. `Migrate()`/`Rollback()` run goose migrations.

### Database Schema

- `accounts`: tenants with `cluster_limit` / `used_clusters`
- `projects`: namespaces, unique on `(account_id, name)`, cached `cluster_count`
- `kubernetes_versions`: catalog (seeded: 1.31/1.30 active, 1.29 deprecated)
- `node_sizes`: catalog (seeded: olympus-nano/small/medium/large)
- `clusters`: unique `(project_id, name)`, state machine `pending/creating/active/deleting/deleted`, stores `endpoint`, `ca_cert`, `kubeconfig`, `provider_ref`
- `node_groups`: unique `(cluster_id, name)`, min/desired/max sizes, `provider_ref` from the data plane
- `audit_logs`: written by the HTTP handler after each successful operation; `project_id` is `NULL` when not resolvable (insert via `NULLIF($2,'')` to avoid FK violations)

## Commands

### Build and Run

```bash
make build
make up && make run            # Postgres on host 15435, service on :8086
POSTGRES_DSN='...' PROVISIONER=mock go run ./cmd/app
```

Real Kubernetes data plane:

```bash
PROVISIONER=kube ORPHEUS_KUBECONFIG=~/.kube/config POSTGRES_DSN='...' go run ./cmd/app
```

### Tests

```bash
make fmt && make vet
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
RUN_K8S_TESTS=1 go test -v ./tests/integration/ -run TestKubeProvisionerRealK3s   # live K3s
```

### Database Migrations (goose)

```bash
goose -dir migrations create add_column sql
goose -dir migrations postgres "host=localhost port=15435 user=olympus password=olympus_secret dbname=olympus_orchestration sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15435 user=olympus password=olympus_secret dbname=olympus_orchestration sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects`, `GET /projects`
- `GET /versions`, `GET /node-sizes`
- `POST /clusters`, `GET /clusters?project={p}`
- `GET /cluster/{p}/{name}`, `GET /cluster/{p}/{name}/kubeconfig`, `DELETE /cluster/{p}/{name}`
- `POST /nodegroups`, `GET /nodegroups?project={p}&cluster={c}`
- `POST /nodegroup/{p}/{c}/{name}/scale`, `DELETE /nodegroup/{p}/{c}/{name}`
- `GET /health`

All requests carry the tenant in the `X-Account-Id` header.

## Important Implementation Details

- Cluster/node-group states are plain strings in `pkg.State*` constants; transitions are guarded (`ErrConflict` for delete-after-delete).
- `account_id` scoping lives in `resolveProject`; calling `GetCluster(account, ...)` from another tenant returns `ErrNotFound` (map `sql.ErrNoRows` via `pkg.IsNotFound`).
- The node-group URL path is `/nodegroup/{project}/{cluster}/{name}[/scale]` — it has **three** leading path segments plus an optional action, so it must NOT be parsed with `splitNameAction` (that is for the two-segment `/cluster/{p}/{name}[/kubeconfig]` form). Use `splitNodeGroup`.
- kubeconfigs are rendered at cluster creation (`pkg/renderKubeconfig`) from the provisioner-supplied endpoint + base64 CA; `cluster.Kubeconfig` is stored and served on request.
- The mock/K3s provisioners are interchangeable per `pkg.Provisioner`; never make the control plane depend on `KubeProvisioner` specifics.
- `cluster_count` on projects is refreshed from `COUNT(*)` (excluding `deleted`) after cluster create/delete.
- Audit (`pkg.Audit`) is invoked from the HTTP handler layer, not the service layer.
- Aliases in PlantUML diagrams must not clash with package names (old local PlantUML builds reject that).
- The `KubeProvisioner` refuses to delete the `default`/`kube-system` namespaces (protected).