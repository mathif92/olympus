# Themis - IAM for the Olympus platform.

An on-premise, multi-tenant IAM service in the spirit of AWS Identity and
Access Management. It manages **users**, **groups**, **roles** and **policies**,
issues AWS-style **access keys**, signs short-lived **JWTs** for principals, and
evaluates policy documents so other Olympus services (or your own) can make
authorization decisions.

- IAM users hold access keys; the secret is returned exactly once at creation
  and only its SHA-256 hash is ever stored.
- Groups contain users; policies attached to a group apply to all its members.
- Roles hold policies and represent a principal that can be assumed via a token.
- Policies are JSON documents (`Version` + `Statement[]`) with `Allow`/`Deny`
  effects, action/resource wildcards (`iam:*`, `clio:Describe*`), and
  explicit-deny-overrides-allow evaluation.
- JWT minting uses HS256 with `THEMIS_JWT_SECRET` (a random one-shot secret is
  used when unset, invalidating minted tokens after restart).

## API

All routes are tenant-scoped via the `X-Account-Id` header (default `default`).

| Method | Path | Description |
| ------ | ---- | ----------- |
| GET/POST | `/projects` | list / create projects |
| GET/POST | `/users?project=` | list / create users |
| GET/DELETE | `/user/{name}?project=` | get / delete a user |
| GET/POST | `/user/{name}/keys?project=` | list / create access keys (secret shown once) |
| DELETE | `/user/{name}/keys/{keyId}?project=` | delete an access key |
| PATCH | `/user/{name}/keys/{keyId}/status?project=` | set key status (`active`/`inactive`) |
| GET/POST | `/groups?project=` | list / create groups |
| GET/DELETE | `/group/{name}?project=` | get / delete a group |
| GET/POST/DELETE | `/group/{name}/members?project=` | list / add / remove members |
| GET/POST | `/roles?project=` | list / create roles |
| GET/DELETE | `/role/{name}?project=` | get / delete a role |
| GET/POST | `/policies?project=` | list / create policies |
| GET/DELETE | `/policy/{name}?project=` | get / delete a policy |
| GET/POST/DELETE | `/attachments` | list / attach / detach policies |
| POST | `/authorize` | evaluate `action`+`resource` for a principal |
| POST | `/tokens` | exchange access key for a signed JWT |
| GET | `/health` | health probe |

## Development

```bash
make up            # start Postgres via docker compose (:15439)
make run           # POSTGRES_DSN=... go run ./cmd/app (listening on :8091)
make test-it       # testcontainers integration suite (needs Docker)
make docker-build  # build olympus/themis:latest
```

Schema is managed by goose in `migrations/` (applied automatically at startup).
The console gateway proxies `/api/themis/*` to this service; the web console
ships a Themis page for managing identities.
