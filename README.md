# Olympus

An on-premise cloud platform, built from a family of small, focused Go
services. Each service is a self-contained microservice exposing a named,
HTTP-based API over the same multi-tenant spine: `accounts → projects →
resources`, with PostgreSQL holding metadata (managed by goose migrations) and
an audit trail. They share one codebase-wide layout, so learning one service
transfers to the rest.

## The platform

Olympus provides the classic building blocks of a cloud, one service at a time:

- **Store objects** — Amphora, an S3-compatible object storage service
  with streaming I/O and SHA-256 ETags.
- **Hold configuration & secrets** — Paramdora, an AWS-SSM-parameter-store-style
  service with versioned parameters and AES-256-GCM encrypted secure strings.
- **Run compute** — Hephaestus, an EC2-equivalent compute control plane with a
  pluggable provisioner, instance lifecycle, volumes, snapshots, key pairs and
  security groups.
- **Manage Kubernetes** — Orpheus, an EKS-equivalent managed-Kubernetes control
  plane with pluggable provisioning: clusters, node groups and kubeconfigs
  (real provisioning via a live Kubernetes API).
- **Host managed databases** — Clio, an RDS-equivalent managed-relational-database
  control plane with engine/size catalogs, instance start/stop, and
  point-in-time snapshots (real provisioning via actual PostgreSQL containers).
- **Cache in memory** — Mneme, an ElastiCache-equivalent managed-cache control
  plane with engine/node-type catalogs and point-in-time snapshots (real
  provisioning via actual Redis containers).
- **Deliver messages** — Iris, an SQS + SNS-equivalent messaging broker with
  queues (visibility + retention), topics, queue fan-out and real webhook HTTP
  delivery; the broker itself, so published messages survive restarts.
- **Control who does what** — Themis, an IAM service: multi-tenant
  users/groups/roles/policies, AWS-style access keys and HS256 JWTs. Every
  service now authorizes requests against Themis (Bearer JWT + `/authorize`),
  so access is enforced per action/resource and fails closed.
- **Run serverless functions** — Prometheus, a Lambda-equivalent service:
  upload a code zip for one of 8 runtimes (Python, Node/JS, TypeScript, Java,
  Go, Rust, C#/.NET, Ruby), deploy it as an immutable version, and invoke it
  with a JSON event in a resource-constrained Docker container.
- **Operate it all** — Console, a web console (React SPA + Go gateway) that
  drives every service from one place: single sign-through via `X-Account-Id`
  (or a Themis token), one panel per service, browsing and operating real
  resources end to end.
- **Deploy it anywhere** — install the whole platform into any Kubernetes
  cluster (all services + bundled Postgres/Redis/MinIO) with a single command:
  `deploy/install.sh` on kind/minikube/Docker Desktop, or `--registry` on any
  cloud cluster. The `deploy/helm/olympus` umbrella chart backs it. See
  [deploy/helm/INSTALL.md](deploy/helm/INSTALL.md) — which also sketches the
  no-Kubernetes option of running the plain Go binaries across a small cluster
  of VMs/servers.

Each service is stateless, scales horizontally, and keeps its state in
Postgres. Internal references stay mythological (the Greek gods dwell on
Olympus; they forged, stored, and safeguarded here).

## Services

| Service    | Port      | Postgres (host) | What it does                                          | Module                              |
| ---------- | --------- | --------------- | ----------------------------------------------------- | ----------------------------------- |
| **amphora** | `:8080`   | `15432`         | S3-compatible object storage (local/hybrid/minio backends) | `github.com/mathif92/olympus/amphora` |
| **paramdora** | `:8083` | `15433`       | Multi-tenant parameter store with versioning + encryption | `github.com/mathif92/olympus/paramdora` |
| **hephaestus** | `:8084` | `15434`   | EC2-equivalent compute control plane (pluggable provisioner) | `github.com/mathif92/olympus/hephaestus` |
| **orpheus** | `:8086` | `15435`   | EKS-equivalent managed Kubernetes (mock or real `kube` provisioner) | `github.com/mathif92/olympus/orpheus` |
| **clio**    | `:8087` | `15436`   | RDS-equivalent managed relational databases (mock or real `docker` provisioner) | `github.com/mathif92/olympus/clio` |
| **mneme**   | `:8088` | `15437`   | ElastiCache-equivalent managed in-memory caches (mock or real `docker` provisioner) | `github.com/mathif92/olympus/mneme` |
| **iris**    | `:8089` | `15438`   | SQS + SNS-equivalent messaging broker (queues, topics, fan-out, webhooks) | `github.com/mathif92/olympus/iris` |
| **themis**  | `:8091` | `15439`   | IAM: users/groups/roles/policies, access keys, JWTs, policy evaluation | `github.com/mathif92/olympus/themis` |
| **prometheus** | `:8092` | `15440` | Serverless functions (λ): 8 Docker runtimes, zip upload, versioning, JSON-event invoke | `github.com/mathif92/olympus/prometheus` |
| **authz**   | —       | —         | Shared library: verifies Themis JWTs and enforces `/authorize` in every service | `github.com/mathif92/olympus/authz` |
| **console**  | `:8090` | —         | Web console: React SPA served by a Go gateway that reverse-proxies `/api/<service>/*` to every service above | `github.com/mathif92/olympus/console` |

Read the per-service README for the full API and quick-start:

- [amphora/README.md](amphora/README.md)
- [paramdora/README.md](paramdora/README.md)
- [hephaestus/README.md](hephaestus/README.md)
- [orpheus/README.md](orpheus/README.md)
- [clio/README.md](clio/README.md)
- [mneme/README.md](mneme/README.md)
- [iris/README.md](iris/README.md)
- [themis/README.md](themis/README.md)
- [prometheus/README.md](prometheus/README.md)
- [console/README.md](console/README.md)

## Shared conventions

Every service follows the same shape:

```
cmd/app/main.go              HTTP server entry point, health probe, backend selection
internal/handler/            HTTP handlers
pkg/
  ├── service.go             Business logic (multi-tenant CRUD + orchestration)
  └── database/              Postgres client + goose migration runner
migrations/                  Numbered goose SQL files (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres, and infrastructure per service)
specs/                       Design notes + a PlantUML architecture diagram (rendered .png in the README)
Makefile                     build / run / fmt / vet / test targets
```

Cross-cutting patterns:

- **Multi-tenancy**: every request carries an `X-Account-Id` header; all rows are
  scoped by `account_id`, and resources live under `projects` ownership.
- **IAM everywhere**: services share the `authz` module. Each backend verifies a
  Themis-issued `Authorization: Bearer <JWT>` locally (shared `THEMIS_JWT_SECRET`),
  then asks Themis `/authorize` for `action`/`resource` before acting — missing
  token → 401, explicit/implicit deny → 403, Themis unreachable → 503 (fail
  closed). Actions are `<service>:<METHOD>` (e.g. `iris:POST`) on `<path>`
  resources (e.g. `/queues`); policies use AWS-style `*` wildcards.
- **Metadata in Postgres**: the database is the source of truth; goose owns the
  schema and applies pending migrations at startup.
- **Audit trails**: a service-level `audit_logs` table records every operation.
- **Stateless gateways**: services hold no session, so they scale out horizontally.
- **Greek-mythology naming**: services are named after figures from Olympus
  (the jar, the box, the forge, the orchestra, the muse of history, the muse
  of memory, the herald of the gods, the goddess of law).

## Repository layout

```
olympus/
  ├── amphora/       Amphora          – object storage (S3-compatible)
  ├── paramdora/     Paramdora        – parameter/secrets store
  ├── hephaestus/    Hephaestus       – compute control plane (EC2-equivalent)
  ├── orpheus/       Orpheus          – managed Kubernetes (EKS-equivalent)
  ├── clio/          Clio             – managed relational databases (RDS-equivalent)
  ├── mneme/         Mneme            – managed in-memory caches (ElastiCache-equivalent)
  ├── iris/          Iris             – messaging broker (SQS + SNS-equivalent)
  ├── themis/        Themis           – IAM: identities, policies, access keys, JWTs
  ├── prometheus/    Prometheus       – serverless functions (Lambda-equivalent)
  ├── authz/         Shared authz     – JWT verification + Themis authorize middleware
  ├── deploy/        K8s umbrella chart – install the whole platform in a cluster (deploy/helm)
  ├── console/       Console          – web console: Go gateway (:8090) + built React SPA
  ├── web/           Console frontend – React + Vite + TypeScript source (builds into console/web/console)
  ├── .gitignore
  └── README.md      (this file)
```

## Getting started

Each service boots its own local infrastructure with `make up` (a
`docker-compose.yml` in the service folder) and serves on its own port — see
the per-service README. From a service directory:

```bash
make up          # start local Postgres (+ Redis/MinIO for storage)
make run         # run the service against the local stack
make test-it     # integration suite via testcontainers (needs Docker)
```

To run the whole platform front to back, boot each service (or just the ones
you want), then the console. Every service's `make run` points at Themis and
shares `THEMIS_JWT_SECRET` (default `dev-secret-change-me`), so start Themis
first — otherwise the other services fail closed on every request:

```bash
(cd themis && make up && make run)     # IAM + policy engine on :8091
(cd iris && make up && make run)       # (any service you want on its port)
(cd console && go run ./cmd/console)   # serves the web UI on :8090
```

Sign in from the console header with a Themis access key (Themis → users → a
user → "Access keys") to mint a token; every `/api/<service>/*` call then
carries it and is authorized against Themis.

The console first serves the built React app from `console/web/console`
(rebuilt from `web/` with `cd web && npm run build`). Its gateway proxies
`/api/<service>/*` to each backend and mounts an aggregated health check at
`/api/health`.

Looking for design depth? `amphora/specs/` has the storage replication notes,
and each service keeps its own `specs/` folder.