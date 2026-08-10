# Console frontend (`web/`)

React + Vite + TypeScript single-page app for the Olympus console. Built by Vite
into `../console/web/console`, served by the Go gateway in `../console/`.

## Commands (from this directory)

```bash
npm install                  # install deps (once)
./node_modules/.bin/tsc -b   # typecheck (NOT `npx tsc` — resolves a bogus `tsc` package)
npm run build                # emit bundle into ../console/web/console
npm run dev                  # Vite dev server; proxies /api -> http://localhost:8090
```

## Structure

- `src/api/client.ts` — `api()` fetch wrapper. Attaches `X-Account-Id` from
  `localStorage['olympus.tenant']` (default `default`), parses JSON, throws
  `ApiError` on non-2xx. `fetchGatewayHealth` polls `/api/health`.
- `src/api/types.ts` — response shapes for all seven service backends.
- `src/components/ui.tsx` — shared primitives: `Button`, `Badge`/`StateBadge`,
  `Card`, `Field`, `Spinner`, `EmptyState`, `Modal`, `ConfirmButton`,
  `ToastProvider`/`useToast`, and the `useAsync` data-fetching hook.
- `src/components/AppShell.tsx` — sidebar with the `SERVICE_META` map (icon,
  tagline) and the account/tenant switcher (`setTenant`).
- `src/components/PageHeader.tsx` — `PageHeader` + `ProjectPicker` (selected
  project is remembered per service in `sessionStorage['olympus.project.<name>']`).
- `src/components/format.tsx` — `CopyButton`, `formatTime`, `kv` pair renderer.
- `src/pages/*.tsx` — one page per service, each performing full CRUD/actions via
  `api(SERVICE, path, opts)`.

## Conventions

- Every request goes through `api()` so tenant + JSON handling stays in one place.
- Amphora has no tenant header and no list/delete endpoints — `AmphoraPage`
  keeps a client-side registry in `localStorage['olympus.amphora.objects']` keyed
  by bucket.
- Snapshot lists on Clio/Mneme require an instance/cluster selector first
  (backend needs `instance`/`cluster` query params), so those pages surface
  snapshots per resource rather than globally.
- Keep `pagePath`/state initialised from `useAsync` data with `(x ?? [])`
  guards; strict TS (`noUnusedLocals`) is enforced by `tsc -b`.