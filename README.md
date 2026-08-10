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

Read the per-service README for the full API and quick-start:

- [amphora/README.md](amphora/README.md)
- [paramdora/README.md](paramdora/README.md)
- [hephaestus/README.md](hephaestus/README.md)
- [orpheus/README.md](orpheus/README.md)
- [clio/README.md](clio/README.md)

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
- **Metadata in Postgres**: the database is the source of truth; goose owns the
  schema and applies pending migrations at startup.
- **Audit trails**: a service-level `audit_logs` table records every operation.
- **Stateless gateways**: services hold no session, so they scale out horizontally.
- **Greek-mythology naming**: services are named after figures from Olympus
  (the jar, the box, the forge, the orchestra, the muse of history).

## Repository layout

```
olympus/
  ├── amphora/       Amphora          – object storage (S3-compatible)
  ├── paramdora/     Paramdora        – parameter/secrets store
  ├── hephaestus/    Hephaestus       – compute control plane (EC2-equivalent)
  ├── orpheus/       Orpheus          – managed Kubernetes (EKS-equivalent)
  ├── clio/          Clio             – managed relational databases (RDS-equivalent)
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

Looking for design depth? `amphora/specs/` has the storage replication notes,
and each service keeps its own `specs/` folder.