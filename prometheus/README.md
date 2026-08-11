# Prometheus — serverless functions (λ) for the Olympus platform

An on-premise, multi-tenant **function-as-a-service** in the spirit of AWS
Lambda. You create a **function** with a **runtime** and a code **zip**; deploy
the zip as an immutable **version**; then **invoke** it with a JSON event.
Code runs in a short-lived, resource-constrained Docker container built
on-the-fly from your upload, so each function is isolated and ships with just
what it needs.

- **8 runtimes**: Python 3.12, Node.js 20 (JavaScript), TypeScript 5, Java 21,
  Go 1.25, Rust 1.80, C# (.NET 9) and Ruby 3.3.
- **Uniform handler contract**: every runtime receives the JSON event on
  **stdin** and writes its JSON result to **stdout** — one mental model across
  all languages. See `pkg/runtimes.go` for each runtime's fixed entrypoint
  file/function.
- **Versioning**: each zip upload becomes an immutable version (SHA-256 of the
  code); the newest upload is the `active` version that invocations run.
- **Constrained execution**: per-function timeout, memory and CPU limits are
  enforced by the container runtime (`--network none`, `--cpus`, `--memory`,
  plus a kill-with-grace deadline); OOM and timeouts are surfaced in the
  invocation record.
- **Pluggable executor**: a `mock` executor ships by default (echoes the event,
  records invocations) for development and tests; `PROVISIONER=docker` runs real
  Docker builds + containers for each invocation.
- **IAM everywhere**: like every Olympus service it authorizes every request
  against Themis (Bearer JWT + `/authorize`); the console gateway proxies
  `/api/prometheus/*` and the web console ships a Prometheus page.

## API

All routes are tenant-scoped via the `X-Account-Id` header (default `default`)
and require a valid Themis Bearer token.

| Method | Path | Description |
| ------ | ---- | ----------- |
| GET/POST | `/projects` | list / create projects |
| GET | `/runtimes` | list supported runtimes (name, version, handler contract) |
| GET/POST | `/functions?project=` | list / create functions |
| GET/DELETE | `/function/{project}/{name}` | get / delete a function |
| GET/POST | `/function/{project}/{name}/versions` | list versions / deploy a code zip (multipart `code` part) |
| POST | `/function/{project}/{name}/invoke` | invoke the active version with a JSON event body |
| GET | `/function/{project}/{name}/invocations` | recent invocation records |
| GET | `/health` | health probe (pings Postgres + executor) |

### Creating a function

```bash
curl -X POST $HOST/functions \
  -H "Authorization: Bearer $TOKEN" -H 'X-Account-Id: acme' \
  -H 'Content-Type: application/json' \
  -d '{"project":"prod","name":"hello","runtime":"python3.12","handler":"handler","timeout_ms":30000,"memory_mb":256,"cpus":1}'
```

`handler` names the entrypoint in your zip that defines the handler function;
each runtime fixes the file name and function signature (see the runtimes
table below).

### Deploying code

A zip containing your source (plus anything else the runtime needs) is
deployed as a new immutable version and becomes active:

```bash
curl -X POST $HOST/function/prod/hello/versions \
  -H "Authorization: Bearer $TOKEN" -H 'X-Account-Id: acme' \
  -F "code=@handler.zip"
```

Zip rules: max 10 MiB, no absolute paths and no `..` traversal; the archive
must contain the runtime's required entrypoint file (e.g. `handler.py`).
Code is stored as bytes, hashed (SHA-256) and never executed at upload time.

### Invoking

The request body **is** the event; it is streamed to the handler's stdin and
the handler's stdout is captured as the response. Invocations run the **active**
version:

```bash
curl -X POST $HOST/function/prod/hello/invoke \
  -H "Authorization: Bearer $TOKEN" -H 'X-Account-Id: acme' \
  -H 'Content-Type: application/json' -d '{"name":"olympus"}'
```

The response is an invocation record: `status` (`success` | `error` | `timeout`
| `oom`), the JSON `response`/`error`, exit code and duration in ms.

## Runtime handler contract

Each runtime fixes the entrypoint file and the handler's name/signature. The
JSON event arrives on **stdin**; the JSON result goes to **stdout**.

| Runtime | Entrypoint | Handler |
| ------- | ---------- | ------- |
| `python3.12` | `handler.py` | `def handler(event)` returning a JSON-serialisable value |
| `nodejs20` | `handler.js` | `exports.handler = async (event)` |
| `typescript5` | `handler.ts` | `export function handler(event)` (compiled with `tsc`) |
| `java21` | `Handler.java` | `public static String handler(String event)` |
| `go1.25` | `handler.go` | `func Handler(event string) (string, error)` (module `olympus/handler`) |
| `rust1.80` | `Cargo.toml` + `src/handler.rs` | `pub fn handler(String) -> String` |
| `dotnet9` | `Program.cs` | `static string Handler(string eventJson)` |
| `ruby3.3` | `handler.rb` | `def handler(event)` |

## Development

```bash
make up            # start Postgres via docker compose (:15440)
make run           # POSTGRES_DSN=... go run ./cmd/app (listening on :8092)
make test-it       # testcontainers integration suite (needs Docker)
make docker-build  # build olympus/prometheus:latest (runtime stage ships the docker CLI)
```

Real Docker executor (needs a Docker daemon; builds `olympus/prometheus-fn:<id>`
images from uploaded code, cached per version):

```bash
PROVISIONER=docker go run ./cmd/app
RUN_DOCKER_TESTS=1 go test -v ./tests/integration/ -run 'TestDockerExecutor'   # live e2e (python/node/go)
```

Schema is managed by goose in `migrations/` (applied automatically at startup).
The console gateway proxies `/api/prometheus/*` to this service; the web
console ships a Prometheus page (create function → deploy zip → invoke).
