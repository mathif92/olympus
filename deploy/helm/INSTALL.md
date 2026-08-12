# Installing the Olympus platform in a Kubernetes cluster

This guide is for a client installing the on-premise Olympus cloud platform from
this repository into their own Kubernetes cluster. Everything is packaged as a
single Helm umbrella chart that installs all ten services plus the bundled
PostgreSQL (one database per service), Redis (cache) and MinIO (object storage
backends).

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