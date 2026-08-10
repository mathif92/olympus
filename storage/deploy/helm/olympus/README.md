# Olympus Helm Chart

Deploys the on-premise OlympusStore object storage service.

## What it deploys

- **Gateway** (`Deployment` + `Service` + optional `Ingress`/`HPA`/`PDB`): stateless
  S3-compatible API. Applies goose migrations against Postgres on startup.
- **MinIO** (`StatefulSet` + headless `Service`): the distributed storage workers.
  Each replica owns a `ReadWriteOnce` PVC and participates in a distributed
  MinIO cluster. Gateway pods push/pull object bytes to/from MinIO while metadata
  lives in Postgres + Redis.
- **Config** (`ConfigMap`) and credentials (`Secret`).

## Install

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami   # optional: external MinIO/Postgres
helm dependency update deploy/helm/olympus                  # if you add subcharts
helm install olympus olulympus-the-chart \
  --set postgresql.host=your-postgres \
  --set postgresql.password=your-password \
  --set minio.enabled=true
```

## Storage backend

Set `storageBackend` explicitly (`local`, `hybrid`, `minio`) to override the
default. The default is `minio` when `minio.enabled=true`, otherwise `hybrid`.

For production, consider **disabling** the bundled `minio` and pointing at an
externally-managed MinIO/CEPH/Rook cluster:

```yaml
minio:
  enabled: false
  externalEndpoint: minio.example.com:9000
configMapKeyRef: # not used
```

## Configuration reference (values.yaml)

| Key | Default | Description |
|-----|---------|-------------|
| `storageBackend` | `""` | `local`, `hybrid`, or `minio` (empty=auto) |
| `image.repository`/`tag` | `olympus/olympus` | Gateway image |
| `replicaCount` | `1` | Gateway replicas |
| `postgresql.host/port/dbname/user/password` | local values | DSN parts; assemble into `POSTGRES_DSN` |
| `redis.url` | `redis://redis:6379` | Redis cache DSN |
| `minio.enabled` | `true` | Deploy distributed MinIO |
| `minio.distributedNodes` | `4` | Erasure-coded MinIO server count |
| `minio.storageSize` | `10Gi` | Per-replica PVC size |
| `minio.accessKey`/`secretKey` | dev defaults | MinIO root credentials |
| `ingress.enabled` | `false` | Expose gateway externally |

## Required gateway env (from ConfigMap/Secret)

`POSTGRES_DSN`, `REDIS_URL`, `STORAGE_BACKEND`, `MINIO_ENDPOINT`,
`MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`.

## Example: render locally

```bash
helm template olympus deploy/helm/olympus
helm template olympus deploy/helm/olympus --set minio.enabled=false
```

## Migrations

Goose-owned. The gateway runs `database.Migrate` against Postgres at startup
(before serving), which is idempotent thanks to the version table.