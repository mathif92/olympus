# Console

A web console for the Olympus platform: a Go gateway that serves a React SPA and
reverse-proxies `/api/<service>/*` to each backend.

## Commands (from this directory)

```bash
go build -o console ./cmd/console   # build the gateway binary
go run ./cmd/console                # run the gateway
go vet ./... && go fmt ./...        # static check + format
```

The SPA lives in `../web/` (Node project) and is built into `console/web/` by
Vite. Rebuild with `npm run build` from `../web`.

## Architecture

- Gateway: `cmd/console/main.go`
  - `services` map: name → backend base URL (ports 8080/8083/8084/8086/8087/8088/8089).
  - `newServiceProxy` builds a `httputil.ReverseProxy` that strips the
    `/api/<name>` prefix and returns 502 on backend failure.
  - `/api/health` probes each backend concurrently and returns a map
    `{service: "ok"|"down"|"no response"}`.
  - `spaHandler` serves static files from `CONSOLE_UI_DIR` with a fallback to
    `index.html` for unknown GET paths (client-side routing).
- Frontend: `../web/` (see `web/AGENTS.md`).

## Conventions

- Backend URLs are read once at startup from `CONSOLE_ADDR`, `CONSOLE_UI_DIR`,
  and `<SERVICE>_URL` env vars (defaults point at localhost).
- The proxy passes through `X-Account-Id` from the browser untouched — tenants
  are exercised by every service's own middleware.
- Console adds no authentication of its own; account selection is client-side
  state sent as `X-Account-Id`.

## Gotchas

- The gateway serves the *built* bundle; if `CONSOLE_UI_DIR` is missing the SPA,
  asset requests 404 but the shell still loads. Run `npm run build` in `../web`.
- Running `go run` from the repository root will not find the SPA at the default
  `web/console` path — run from `console/` or set `CONSOLE_UI_DIR` explicitly.