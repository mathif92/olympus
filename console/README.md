# Console

A web console for the whole Olympus platform: a React single-page application
served by a Go gateway. The gateway reverse-proxies `/api/<service>/*` to every
Olympus service on one origin, so the browser talks to a single host while each
backend keeps its own port behind it.

## Layout

- `cmd/console/main.go` — Go gateway (`:8090`): serves the built SPA, proxies
  `/api/<service>/*` to each backend, and exposes `/api/health`.
- `web/` — the React + Vite + TypeScript SPA sources (see `web/README.md`).
  `npm run build` emits the static bundle into `console/web/console`, which the
  gateway serves by default.

## Running

Boot any Olympus services you want to operate, then:

```bash
go run ./cmd/console     # from console/
```

Open http://localhost:8090 and pick an account in the sidebar (the console sends
it as `X-Account-Id` on every request; the default is `default`).

The gateway proxies by service name and strips the `/api/<service>` prefix:

| Path                     | Backend               |
| ------------------------ | --------------------- |
| `/api/amphora/*`         | `:8080`               |
| `/api/paramdora/*`       | `:8083`               |
| `/api/hephaestus/*`      | `:8084`               |
| `/api/orpheus/*`         | `:8086`               |
| `/api/clio/*`            | `:8087`               |
| `/api/mneme/*`           | `:8088`               |
| `/api/iris/*`            | `:8089`               |
| `/api/health`            | aggregates all of the above |

## Configuration (env vars)

| Var            | Default                            | Meaning                                  |
| -------------- | ---------------------------------- | ---------------------------------------- |
| `CONSOLE_ADDR` | `:8090`                            | Gateway listen address                   |
| `CONSOLE_UI_DIR` | `web/console`                    | Directory of the built SPA to serve      |
| `AMPHORA_URL`  | `http://localhost:8080`            | Amphora backend base URL                 |
| `PARAMDORA_URL` | `http://localhost:8083`          | Paramdora backend base URL               |
| `HEPHAESTUS_URL` | `http://localhost:8084`         | Hephaestus backend base URL              |
| `ORPHEUS_URL`  | `http://localhost:8086`            | Orpheus backend base URL                 |
| `CLIO_URL`     | `http://localhost:8087`            | Clio backend base URL                    |
| `MNEME_URL`    | `http://localhost:8088`            | Mneme backend base URL                   |
| `IRIS_URL`     | `http://localhost:8089`            | Iris backend base URL                    |

## Rebuilding the SPA

The gateway serves whatever is already built in `CONSOLE_UI_DIR`. To rebuild the
React app:

```bash
cd web
npm install        # once
npm run build      # emits ../console/web/console
```

## Development

Run the gateway with `CONSOLE_UI_DIR=/abs/path/to/console/web/console` (or just
`go run ./cmd/console` from this directory). For frontend hot-reload, run Vite's
dev server from `web/` — it proxies `/api` to `http://localhost:8090`:

```bash
cd web
npm run dev
```