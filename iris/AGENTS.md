# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Iris is the managed messaging and pub-sub service for the Olympus platform — an SQS + SNS-equivalent broker implemented in Go. It hosts SQS-style queues (send / poll with visibility timeouts / ack, retention-based expiry) and SNS-style topics (subscribe queues or webhook URLs; each publish fans out a message copy into every subscribed queue and POSTs the message body to every webhook subscriber with retries). Tenancy is modelled `accounts → projects → resources`, mirroring the Amphora, Paramdora, Hephaestus, Orpheus, Clio, and Mneme services. Iris is the broker itself: queues, topics, and messages live in Postgres, so published messages survive restarts and there is no separate provisioner or extra infrastructure.

## Architecture

```
cmd/app/main.go              HTTP server, health probe, goose migrations on startup
internal/handler/            HTTP handlers (/queues, /queue/{p}/{n}/..., /topics, /topic/{p}/{n}/...)
pkg/
  └── service.go             Broker: queues, topics, subscribers, publish fan-out,
                             webhook delivery (retries), audit
  └── database/              Postgres client + goose migration runner
migrations/                  goose SQL migrations (applied automatically at startup)
tests/integration/           testcontainers-backed tests (Postgres)
```

### Key Components

1. **API Layer (`internal/handler/iris.go`)**: Registers routes for projects, queue collection `/queues`, queue actions `/queue/{p}/{name}[/send|poll|ack]` (GET/DELETE when no action), topic collection `/topics`, and topic actions `/topic/{p}/{name}[/publish|subscribe|subscribers|unsubscribe]` (GET/DELETE when no action). Tenant is resolved from the `X-Account-Id` header (default `default`) and auto-provisioned via `ensureAccount`. Audit entries are written by the handler after each successful operation.

2. **Broker (`pkg/service.go`)**: All state transitions go through the service:
   - `CreateQueue()`: writes the queue (`state=active`), defaults `visibility_timeout_sec` to 30 and `message_retention_sec` to 86400
   - `SendMessage()`: inserts a message (`state=pending`, `visible_at=now`, `expires_at=now+retention`)
   - `PollMessages()`: in one transaction, re-exposes lapsed `in_flight` messages, locks eligible `pending`+visible messages `FOR UPDATE`, marks them `in_flight` with `visible_at=now+visibility_timeout`, and returns them
   - `AckMessage()`: removes an `in_flight` message by id
   - `SubscribeQueue()`/`SubscribeWebhook()`: attach subscribers to a topic (unique `(topic_id, queue_id)` and `(topic_id, webhook_url)`)
   - `PublishMessage()`: fans a message copy into every active queue subscriber, and for each webhook subscriber records a `webhook_deliveries` row and POSTs the body (up to 3 attempts, exponential backoff, recording `status`/`attempts`/`last_error` per attempt)
   - Every lookup is scoped by `account_id` via `resolveProject` — tenants can never see or mutate another's resources

3. **Database (`pkg/database/`)**: `Client` wraps a pooled `*sql.DB` and exposes `QueryRow`/`Query`/`Exec`/`Begin`. `Migrate()`/`Rollback()` run goose migrations.

### Database Schema

- `accounts`: tenants with `queue_limit` / `used_queues`
- `projects`: namespaces, unique on `(account_id, name)`, cached `queue_count` + `topic_count`
- `queues`: unique `(project_id, name)`, `visibility_timeout_sec` (default 30), `message_retention_sec` (default 86400)
- `queue_messages`: state `pending/available/in_flight/delivered`, `visible_at` (visibility), `expires_at` (retention), indexed `(queue_id, state, visible_at)` and `(expires_at)`
- `topics`: unique `(project_id, name)`
- `topic_subscribers`: `kind queue|webhook` with a CHECK constraint (`queue_id` xor `webhook_url`); unique `(topic_id, queue_id)` and `(topic_id, webhook_url)`
- `webhook_deliveries`: per-publish push record with `status pending/in_flight/delivered/failed`, `attempts`, `last_error`
- `audit_logs`: written by the HTTP handler after each successful operation; `project_id` is `NULL` when not resolvable (insert via `NULLIF($2,'')` to avoid FK violations)

## Commands

### Build and Run

```bash
make build
make up && make run            # Postgres on host 15438, service on :8089
POSTGRES_DSN='...' go run ./cmd/app
```

### Tests

```bash
make fmt && make vet
make test              # go test ./cmd/... ./pkg/...
make test-it           # integration tests via testcontainers (Postgres; needs Docker)
```

### Database Migrations (goose)

```bash
goose -dir migrations create add_column sql
goose -dir migrations postgres "host=localhost port=15438 user=olympus password=olympus_secret dbname=olympus_messaging sslmode=disable" up
goose -dir migrations postgres "host=localhost port=15438 user=olympus password=olympus_secret dbname=olympus_messaging sslmode=disable" down
```

## HTTP Endpoints

- `POST /projects`, `GET /projects`
- `POST /queues`, `GET /queues?project={p}`
- `GET /queue/{p}/{name}`, `DELETE /queue/{p}/{name}`
- `POST /queue/{p}/{name}/send`, `POST /queue/{p}/{name}/poll`, `POST /queue/{p}/{name}/ack`
- `POST /topics`, `GET /topics?project={p}`
- `GET /topic/{p}/{name}`, `DELETE /topic/{p}/{name}`
- `POST /topic/{p}/{name}/publish`
- `POST /topic/{p}/{name}/subscribe`, `GET /topic/{p}/{name}/subscribers`, `POST /topic/{p}/{name}/unsubscribe`
- `GET /health`

All requests carry the tenant in the `X-Account-Id` header.

## Important Implementation Details

- Message/queue states are plain strings (`pkg.Msg*` constants); the desired `in_flight` state is set both in SQL and back onto the returned structs after polling.
- `account_id` scoping lives in `resolveProject` (returns `sql.ErrNoRows` mapped to `pkg.ErrNotFound`); cross-tenant access returns `pkg.ErrNotFound`, and HTTP handlers translate it to `404`.
- Queue/topic action paths are parsed with `splitPath` (`/queue/{p}/{rest}`) then `splitNameAction` (`{name}[/action]`) — action names live in the URL, not the query string.
- `PublishMessage` fanned-out queue messages use fixed retention (`expires_at = now + 1 day`) since a queue's retention is applied at insert; webhook deliveries are synchronous within publish but retried with backoff, so webhook retry tests must assert on `webhook_deliveries` state, not just HTTP counts.
- `attributes` are stored as JSONB; always bind a non-nil byte slice (e.g. `{}`) when inserting, never `nil`, or lib/pq errors with "invalid input syntax for type json".
- `queue_count` / `topic_count` on projects are refreshed from `COUNT(*)` after queue/topic create and delete (`refreshCounts`).
- Audit (`pkg.Audit`) is invoked from the HTTP handler layer, not the service layer.
- Aliases in PlantUML diagrams must not clash with package names (old local PlantUML builds reject that). Use `skinparam backgroundColor white` (no `!theme`).
- `go mod init` must run inside `iris/` — if a `go.mod` appears at the repo root, it was created in the wrong directory and must be moved into the service dir.

## Webhook delivery semantics

Each publish to a topic with webhook subscribers records a `webhook_deliveries`
row (`status=pending`) per subscriber, then `deliverWebhook` POSTs the raw
message body (`Content-Type: application/json`) up to 3 times with exponential
backoff (starting 100 ms). Any 2xx marks `delivered`; otherwise `status=failed`
and `last_error` records the final error. The webhook delivery is synchronous
inside `PublishMessage` so responses (and tests) are deterministic.

## Testing real webhook delivery

`TestWebhookDelivery` and `TestWebhookRetriesOnFailure` use `httptest.NewServer`
as real HTTP receivers and assert on the `webhook_deliveries` table. Requires a
Docker daemon reachable by testcontainers for Postgres.