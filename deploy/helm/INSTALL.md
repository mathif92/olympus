# Installing the Olympus platform in a Kubernetes cluster

This guide is for a client installing the on-premise Olympus cloud platform from
this repository into their own Kubernetes cluster. Everything is packaged as a
single Helm umbrella chart that installs all ten services plus the bundled
PostgreSQL (one database per service), Redis (cache) and MinIO (object storage
backends).

## Fastest way: one command

> Requires `kubectl`, `helm` and `docker`. Works against any cluster.

```bash
# local cluster (kind / minikube / Docker Desktop): build + load images
./deploy/install.sh --context minikube

# any remote cluster: build + push to a registry you own
./deploy/install.sh --context <cluster> --registry ghcr.io/<you>/olympus

# cluster that can already pull prebuilt images
./deploy/install.sh --context <cluster> --image-prefix ghcr.io/<you>/olympus --skip-build
```

The installer sanity-checks your tooling and the target cluster, asks you to
confirm the target (it refuses to touch staging/production-looking contexts),
builds all ten service images, generates secrets once (kept in
`~/.olympus/secrets-<release>.env` for reuse), runs `helm install` and prints
the console URL plus next steps. See `./deploy/install.sh --help` for all flags
(`--namespace`, `--release`, `--ingress`, `--values`, `--set`, `--dry-run`, ...).
The remainder of this guide covers the manual, step-by-step path if you prefer
to drive the Helm chart directly.

## Without Kubernetes: bare binaries on VMs/servers

If you don't run Kubernetes at all, every service is a plain Go binary with no
K8s-specific dependencies — you can stand the platform up "old school" by
running the compiled binaries directly across a small cluster of VMs or
servers. This is a **pending / manual** approach: there is no script or
orchestration for it yet, so you drive each process yourself (systemd,
supervisord, or a plain `nohup`). Everything else in this doc assumes the Helm
chart — this section is for the no-Kubernetes case only.

### Layout

Two VM roles:

| Role | Hosts what |
| ---- | ---------- |
| **Data plane** | PostgreSQL (all 9 service databases), Redis, MinIO |
| **Control plane** | the ten service binaries |

Within the control plane there are no hard rules about which service goes on
which machine — they only talk to each other and to the data plane over TCP, so
you can colocate all of them on one box for testing, or spread them one-per-VM
or in any grouping that suits you, as long as they can resolve each other.

### The wiring contract

Each service is configured by environment variables — exactly the ones the
Helm chart sets for the in-cluster pods (see each subchart's
`templates/deployment.yaml`). The essentials, common to all services:

| Variable | Purpose |
| -------- | ------- |
| `POSTGRES_DSN` | libpq DSN to the service's own database, e.g. `host=10.0.0.5 port=5432 user=olympus password=... dbname=olympus_themis sslmode=disable`. |
| `THEMIS_URL` | Base URL of the Themis IAM service, e.g. `http://10.0.0.11:8091`. |
| `THEMIS_JWT_SECRET` | Shared HMAC secret every service uses to verify JWTs. Must match cluster-wide. |

Each service uses its **own** database on the shared Postgres (one of the nine
`olympus_*` databases the chart creates):

| Service | Port | Database | Extra env (beyond the common three) |
| ------- | ---- | -------- | ----------------------------------- |
| Themis | 8091 | `olympus_themis` | — |
| Amphora | 8080 | `olympus_storage` | `REDIS_URL`, `STORAGE_BACKEND`, `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` |
| Paramdora | 8083 | `olympus_parameters` | — |
| Hephaestus | 8084 | `olympus_compute` | `PROVISIONER` |
| Orpheus | 8086 | `olympus_orchestration` | `PROVISIONER` |
| Clio | 8087 | `olympus_databases` | `PROVISIONER`, `DOCKER_HOST` |
| Mneme | 8088 | `olympus_caches` | `PROVISIONER`, `DOCKER_HOST` |
| Iris | 8089 | `olympus_messaging` | — |
| Prometheus | 8092 | `olympus_functions` | `PROVISIONER`, `DOCKER_HOST`, `PROMETHEUS_IMAGE_PREFIX` |
| Console | 8090 | — (no DB) | `THEMIS_URL`, and one `<SERVICE>_URL` per backend, e.g. `AMPHORA_URL=http://10.0.0.11:8080`, `THEMIS_URL=http://10.0.0.11:8091`, ... |

So on a control-plane VM the runtime is, e.g. for Amphora:

```bash
set -a; source /opt/olympus/.env; set +a   # shared on every host
export POSTGRES_DSN="host=10.0.0.5 port=5432 user=olympus password=$PG_PASSWORD dbname=olympus_storage sslmode=disable"
export THEMIS_URL="http://10.0.0.11:8091"
export STORAGE_BACKEND=minio
export REDIS_URL="redis://10.0.0.5:6379"
export MINIO_ENDPOINT="http://10.0.0.5:9000"
/opt/olympus/amphora
```

Keep a shared `.env` sourced on every control-plane host so `THEMIS_URL` and
`THEMIS_JWT_SECRET` stay identical everywhere. See each subchart's
`templates/deployment.yaml` and `values.yaml` for the authoritative list — the
env names may drift between versions.

### Steps

```bash
# 1. Build the binaries on a build host
for svc in themis amphora paramdora hephaestus orpheus clio mneme iris prometheus; do
  (cd $svc && CGO_ENABLED=0 go build -o /out/$svc ./cmd/app)
done
(cd console && CGO_ENABLED=0 go build -o /out/console ./cmd/console)   # console builds from ./cmd/console

# 2. Ship the /out binaries + a shared .env to each machine, e.g.
scp /out/* root@vm1:/opt/olympus/
scp /out/* root@vm2:/opt/olympus/
```

Then on the data-plane VM run Postgres/Redis/MinIO (packaged, or via the same
images they would use in Kubernetes), and on each control-plane VM:

```bash
# source the same shared env on every host
set -a; source /opt/olympus/.env; set +a
# example: run Amphora on this box
/opt/olympus/amphora
```

### What's NOT covered here

- No systemd unit files, no log rotation, no TLS, no reverse proxy in front of
  the console, no upgrade path, no failover for the data-plane services
  (Postgres/Redis/MinIO are each a single instance today).
- No provisioning automation — you place binaries and env yourself.
- The `docker` provisioners of Clio/Mneme/Prometheus need a Docker daemon
  reachable from that service (e.g. `DOCKER_HOST`), same caveat as the
  cluster path below.

Treat this as a reference for feasibility, not a supported deployment story.
The Helm chart remains the supported path, and `deploy/install.sh` is the
one-command way into it.

## What you get

| Component      | Notes |
| -------------- | ----- |
| Console        | Web UI + gateway, single ingress entry point |
| Themis         | IAM (users, roles, policies, access keys, JWT) |
| Amphora        | S3-compatible object storage (MinIO backend by default) |
| Paramdora      | Parameter/secrets store |
| Hephaestus     | Compute control plane (mock provisioner) |
| Orpheus        | Managed Kubernetes control plane (mock provisioner) |
| Clio           | Managed relational databases (mock provisioner) |
| Mneme          | Managed in-memory caches (mock provisioner) |
| Iris           | Messaging broker (SQS + SNS) |
| Prometheus     | Serverless functions (mock executor by default) |
| PostgreSQL     | Single StatefulSet hosting all 9 service databases |
| Redis          | Cache for Amphora |
| MinIO          | Object storage backing Amphora |

All services default to **mock** provisioners — they come up and are fully
exerciseable through the console without needing a Docker daemon inside the
cluster. See [Enabling real provisioners](#enabling-real-provisioners) below
to wire live Docker-based provisioning.

> Maturity: this is an early-stage platform skeleton. IAM (Themis) and object
> storage (Amphora→MinIO) are fully real; the compute / managed-DB / managed-
> cache / managed-K8s control planes are mock-by-default; the data plane is
> single-node (no HA, backups or failover); no TLS, key rotation or
> observability stack by default. See the "Maturity & roadmap" section in the
> root README. Treat this as exerciseable, not production-hardened.

## Prerequisites

- A Kubernetes cluster (any CNCF-conformant distribution; all objects are
  standard `apps/v1` / `networking.k8s.io/v1`, no CRDs required).
- `helm` v3 (the chart does not require `helm dependency build` — subcharts are
  vendored under `charts/`).
- `kubectl` configured against the target cluster.
- A container registry the cluster can pull from, or `minikube image load` /
  `kind load docker-image` for local testing.

## 1. Build and publish the images

Each service is a self-contained image built from its folder's Dockerfile:

```bash
for svc in themis amphora paramdora hephaestus orpheus clio mneme iris prometheus; do
  (cd $svc && make docker-build)            # produces olympus/<svc>:latest
done
(cd console && docker build -t olympus/console:latest .)   # console has no Makefile docker-build
```

Push the images to your registry (or load them into your node/runtime):

```bash
REGISTRY=registry.example.com/olympus
for svc in themis amphora paramdora hephaestus orpheus clio mneme iris prometheus console; do
  docker tag olympus/$svc:latest $REGISTRY/$svc:latest
  docker push $REGISTRY/$svc:latest
done
```

## 2. Configure values

Copy the shipped defaults and adjust for your environment:

```bash
cp deploy/helm/olympus/values.yaml my-olympus-values.yaml
```

The important knobs:

| Key | Default | Meaning |
| --- | ------- | ------- |
| `global.jwtSecret` | `dev-secret-change-me` | JWT secret shared by every service. **Change it.** |
| `global.postgres.password` | `olympus_secret` | Postgres superuser password. **Change it.** |
| `global.minio.accessKey` / `secretKey` | `olympus_admin` / `olympus_secret_admin` | MinIO credentials. Change them. |
| `<service>.enabled` | `true` | Turn individual services on/off. |
| `<service>.image.repository`/`tag` | `olympus/<service>:latest` | Where each image comes from. |
| `ingress.enabled`, `ingress.hosts` | `false` | Expose the console through an Ingress. |
| `postgres.storageSize` / `minio.storageSize` | `10Gi` | Persistent volume sizes (set `storageClass` if your cluster requires it). |
| `prometheus.provisioner` | `mock` | `docker` to run real function containers (needs the Docker daemon). |

See every option in `deploy/helm/olympus/values.yaml`.

## 3. Install

```bash
helm install olympus deploy/helm/olympus -f my-olympus-values.yaml \
  --namespace olympus --create-namespace --wait --timeout 10m
```

Watch it come up:

```bash
kubectl -n olympus get pods -w
```

When everything is `Running`/`Ready`, the console is reachable at the Service
`olympus-console` (port 8090), or through your Ingress host:

```bash
kubectl -n olympus port-forward svc/olympus-console 8090:8090
# open http://localhost:8090
```

## 4. First sign-in

1. Open the console.
2. Create your first IAM user under **Themis**, then create an access key for it.
3. Attach a policy allowing the user to act on the services you use (e.g. a
   wildcard policy, or scoped per service).
4. Use the access key in the console header to mint a JWT — all subsequent
   `/api/<service>/*` calls are authorized against Themis.

Every request carries the tenant in the `X-Account-Id` header (default
`default`); resources live under projects owned by that account, exactly like
the local single-machine deployment.

## 5. Upgrading / uninstalling

```bash
helm upgrade olympus deploy/helm/olympus -f my-olympus-values.yaml \
  --namespace olympus --wait --timeout 10m

helm uninstall olympus --namespace olympus
```

Postgres and MinIO store data on `ReadWriteOnce` PersistentVolumeClaims named
`data-<release>-postgres-0` / `data-<release>-minio-0`. Uninstalling the Helm
release does **not** delete the PVCs — delete them manually to wipe all data.

## Enabling real provisioners

By default everything runs against mock provisioners so the control planes
boot without extra infrastructure. To use the real data planes:

- **Amphora → MinIO** is already the default backend (bundled MinIO).
- **Clio / Mneme** ship a `docker` provisioner that launches real
  PostgreSQL/Redis containers via testcontainers. Set `clio.provisioner: docker`
  (and `mneme.provisioner: docker`); the pods mount the host Docker socket
  (`clio.dockerHostSocket` / `mneme.dockerHostSocket`, default
  `/var/run/docker.sock`).
- **Hephaestus** currently ships the mock provisioner only.
- **Orpheus** ships a `kube` provisioner that manages clusters through the
  Kubernetes API; provide a kubeconfig via `orpheus.kubeconfig` (mounted as a
  secret).
- **Prometheus** real executor: set `prometheus.provisioner: docker` and
  `prometheus.dockerHostSocket: /var/run/docker.sock` (already the default) to
  build and run function containers on the host Docker daemon.

> Security note: mounting the host Docker socket grants the pod host-level
> control. In a cluster without a trusted privilege boundary, prefer running a
> dedicated DinD (Docker-in-Docker) pod/sidecar and pointing `DOCKER_HOST` at
> it.

## Validating a chart before applying

Render and schema-check without a cluster:

```bash
helm lint deploy/helm/olympus
helm template olympus deploy/helm/olympus | kubeconform --strict -summary
```