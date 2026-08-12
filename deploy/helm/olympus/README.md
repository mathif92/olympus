# Olympus platform Helm chart

Umbrella chart that installs the whole on-premise Olympus cloud platform into a
Kubernetes cluster: ten services (themis, amphora, paramdora, hephaestus,
orpheus, clio, mneme, iris, prometheus, console) plus a bundled PostgreSQL
(housing every service database), Redis (Amphora cache) and MinIO (Amphora
object storage).

```bash
helm install olympus . --namespace olympus --create-namespace --wait --timeout 10m
```

See [INSTALL.md](../INSTALL.md) for a full client-side installation guide
(images, values, first sign-in, real provisioners).

## Layout

```
deploy/helm/olympus/
  Chart.yaml          umbrella dependencies (vendored subcharts, no helm dep build)
  values.yaml         global.jwtSecret / global.postgres / per-service + infra values
  templates/          shared Secret, PostgreSQL + MinIO StatefulSets, Redis, Ingress
  charts/<service>/   one subchart per service (image, port, POSTGRES_DSN, THEMIS_* env)
```

## Validation

```bash
helm lint .
helm template olympus . | kubeconform --strict -summary
```