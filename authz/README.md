# authz

Shared authorization library for every Olympus service.

Each service verifies a Themis-issued `Authorization: Bearer <JWT>` locally
(HS256, shared `THEMIS_JWT_SECRET`) and then asks Themis `/authorize` whether
the principal may perform the request's action on its resource. Enforcement is
**fail closed**:

| Condition                            | Status |
| ------------------------------------ | ------ |
| No / invalid / expired bearer token  | `401`  |
| Themis denies (or implicit deny)     | `403`  |
| Themis unreachable / evaluation error| `503`  |

On success the middleware binds `X-Account-Id` from the token's account claim
and exposes the claims via `authz.ContextClaims(r.Context())`.

## Wiring

Add the module and wire the middleware in `cmd/app/main.go`:

```go
import "github.com/mathif92/olympus/authz"

authzClient := authz.NewClient(getenv("THEMIS_URL", "http://localhost:8091"),
                               getenv("THEMIS_JWT_SECRET", ""))
mux := http.NewServeMux()
mux.HandleFunc("/health", healthHandler(...))              // not protected
mux.Handle("/", authzClient.Middleware(authz.ServiceMapper("iris"))(coreRouter))
```

`ServiceMapper(service)` derives `action = "<service>:<METHOD>"` (e.g.
`iris:POST`) and `resource = "<path>"` (e.g. `/queues`), so IAM policies use
AWS-style wildcards: `Action: ["iris:*"]`, `Resource: ["/queues/*"]`.

Set the same `THEMIS_JWT_SECRET` in Themis and every service (the dev default
is `dev-secret-change-me`; the Makefiles wire it for `make run`).

## API

- `NewClient(themisURL, jwtSecret)` — build a client.
- `(*Client).Verify(token)` — parse/verify a token into `Claims`.
- `(*Client).Authorize(ctx, account, project, principalType, principalName, action, resource)` —
  ask Themis for a decision.
- `(*Client).Middleware(mapper)` — HTTP middleware; fail-closed enforcement.
- `ContextClaims(ctx)` — recover the verified claims in downstream handlers.
