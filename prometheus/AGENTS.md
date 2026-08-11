# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Prometheus is the serverless-functions service for the Olympus platform — a Lambda-equivalent built in Go. Tenancy is modelled `accounts → projects → resources`, mirroring the other services. Each function is defined by a runtime + code zip; deploying the zip creates an immutable version (the newest is active) that invocations run. Execution is pluggable behind an `Executor` interface: a `mock` executor ships by default, and a real `docker` executor builds a per-version image from the uploaded code and runs the handler in a constrained container.

## Architecture

```
cmd/app/main.go              HTTP server, health probe (pings Postgres + executor), PROVISIONER switch
internal/handler/            HTTP handlers (/projects, /runtimes, /functions, /function/{p}/{n}[/action])
pkg/
  ├── runtimes.go            Runtime registry + embedded scaffold templates (embed runtimes/*)
  ├── runtimes/              One folder per runtime: Dockerfile + entrypoint template
  ├── zip.go                 Zip upload validation (size, traversal, required files) + extraction
  ├── executor.go            Executor interface + status constants (success/error/timeout/oom)
  ├── mock_executor.go       Echo executor for dev/tests
  ├── docker_executor.go     Real executor: docker build + run, constraints, timeout kill
  ├── service.go             Function/version/invocation lifecycle + audit
  └── database/              Postgres client + goose migration runner + structs
migrations/                  goose SQL migrations (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres) + real-Docker executor tests
```

### Key Components

1. **API Layer (`internal/handler/prometheus.go`)**: Registers `/projects`, `/runtimes`, `/functions` and `/function/{project}/{name}[/action]` where `action` is `versions` | `invoke` | `invocations`. Tenant from `X-Account-Id` (default `default`) auto-provisioned via `ensureAccount`.

2. **Runtime registry (`pkg/runtimes.go`)**: 8 runtimes with a fixed handler contract — the JSON event is streamed to the handler's **stdin** and the JSON result to **stdout**. Each `Runtime` fixes its entrypoint: `Handler` (file name), `HandlerFile`, `HandlerFunc`, `RequiredFiles`. Keep these in sync with `pkg/runtimes/*` templates.

3. **Executor (`pkg/executor.go`)** and implementations:
   - `MockExecutor` (`pkg/mock_executor.go`) echoes the event as the response and records invocations — no Docker needed.
   - `DockerExecutor` (`pkg/docker_executor.go`) builds image `olympus/prometheus-fn:<versionID>` from the extracted zip + embedded scaffold (cached via `docker image inspect`), runs it with `--rm -i --network none -m <mem>m --memory-swap <mem>m --cpus <cpus>`, forwards function env vars, enforces the timeout with a context + 2s grace then `docker kill`, and detects OOM via exit code 137. `main.go` selects it when `PROVISIONER=docker`.

4. **Service (`pkg/service.go`)**: `CreateFunction`/`GetFunction`/`ListFunctions`/`DeleteFunction`, `UploadVersion` (activates the new version in a transaction), `ListVersions`, `InvokeFunction` (always runs the **active** version, records the invocation even when execution fails with `status=error`), `ListInvocations`, `EnsureAccount`/`CreateProject`, `Audit`.

### Database Schema

- `accounts`: tenants with `function_limit` / `used_functions`
- `projects`: namespaces, unique on `(account_id, name)`, cached `function_count`
- `functions`: unique `(project_id, name)`, stores `runtime`, `handler`, `timeout_ms`, `memory_mb`, `cpus`, `env_vars` (jsonb), `current_version`
- `function_versions`: immutable snapshots — `code` bytea, `code_sha256`, `is_active`, unique `(function_id, version)`
- `invocations`: `status`, `request` (jsonb), `response`, `error`, `exit_code`, `duration_ms`
- `audit_logs`: written by the HTTP handler after each successful operation

## Commands

### Build and Run

```bash
make build
make up && make run            # Postgres on host 15440, service on :8092
POSTGRES_DSN='...' PROVISIONER=mock go run ./cmd/app
```

Real Docker executor:

```bash
PROVISIONER=docker POSTGRES_DSN='...' go run ./cmd/app
```

### Tests

```bash
make fmt && make vet
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
RUN_DOCKER_TESTS=1 go test -v ./tests/integration/ -run TestDockerExecutor   # live python/node/go + timeout
```

### Database Migrations (goose)

```bash
goose -dir migrations create add_column sql
goose -dir migrations postgres "host=localhost port=15440 user=olympus password=olympus_secret dbname=olympus_functions sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15440 user=olympus password=olympus_secret dbname=olympus_functions sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects`, `GET /projects`
- `GET /runtimes`
- `POST /functions`, `GET /functions?project={p}`
- `GET /function/{p}/{name}`, `DELETE /function/{p}/{name}`
- `GET /function/{p}/{name}/versions`, `POST /function/{p}/{name}/versions` (multipart `code` file part)
- `POST /function/{p}/{name}/invoke` (body **is** the event JSON; runs the active version)
- `GET /function/{p}/{name}/invocations`
- `GET /health`

All requests carry the tenant in the `X-Account-Id` header and are authorized
against Themis via the shared `authz` middleware.

## Important Implementation Details

- **Invoke contract**: the request body is the raw event JSON (streamed to the handler's stdin); the handler's stdout is the response. Invocations always run the **active** version (`getActiveVersion`), never a version selected by the caller.
- **Zip validation** (`pkg/zip.go`): `MaxCodeBytes` = 10 MiB; rejects absolute paths and `..` traversal (backslashes normalised), corrupt archives, and missing runtime `RequiredFiles`. The go runtime needs a `go.mod` generated at build time because `go:embed` refuses directories containing a `go.mod` — its Dockerfile template handles this, do not add one to `pkg/runtimes/go1.25/`.
- **`main.go.tmpl`**: the go runtime's entrypoint must stay named `main.go.tmpl` inside `pkg/runtimes/go1.25/`; if it were `main.go` the module build would try to compile it.
- **`env_vars` jsonb**: use `EnvVarsParam()` when writing env vars to Postgres — it returns SQL `NULL` for an empty map to avoid `lib/pq` coercing `[]byte(nil)` into an empty-string jsonb.
- **Executor interface**: keep `mock`/`docker` interchangeable via `pkg.Executor`; never let the service layer depend on `DockerExecutor` specifics.
- **Function counts**: `used_functions` (accounts) and `function_count` (projects) are refreshed from `COUNT(*)` after create/delete.
- **Audit** (`pkg.Audit`) is invoked from the HTTP handler layer, not the service layer.
- **Docker executor tests** (`tests/integration/docker_executor_test.go`) are gated by `RUN_DOCKER_TESTS=1` and exercise python/node/go end-to-end plus the timeout path; they need a Docker daemon and network on the first build (base-image pulls).
- Aliases in PlantUML diagrams must not clash with package names. Use `skinparam backgroundColor white` (no `!theme`).
