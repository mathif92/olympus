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
- `src/components/ui.tsx` — shared primitives wrapping HeroUI v3: `Button`,
  `Badge`/`StateBadge`, `Card`, `Field`, `SelectField`, `Spinner`, `EmptyState`,
  `Modal`, `ConfirmButton`, `SegmentedTabs`, `useToast`, and the `useAsync`
  data-fetching hook.
- `src/components/AppShell.tsx` — sidebar with the `SERVICE_META` map (icon,
  tagline), the account/tenant switcher (`setTenant`), and the mounted
  `<Toast.Provider placement="bottom end" />`.
- `src/components/PageHeader.tsx` — `PageHeader` + `ProjectPicker` (selected
  project is remembered per service in `sessionStorage['olympus.project.<name>']`).
- `src/components/format.tsx` — `CopyButton`, `formatTime`, `kv` pair renderer.
- `src/pages/*.tsx` — one page per service, each performing full CRUD/actions via
  `api(SERVICE, path, opts)`.

## Conventions

- **HeroUI v3 is the component library.** It is CSS-driven: `src/index.css`
  imports `tailwindcss` then `@heroui/styles`, and the `dark` theme is selected
  by `class="dark" data-theme="dark"` on `<html>` (in `index.html`). There is
  **no `HeroUIProvider` wrapper** and no `plugin` config needed for Tailwind.
- HeroUI v3 exposes **compound components**: `Modal` (Backdrop/Container/Dialog/
  CloseTrigger/Header/Heading/Body/Footer), `Card` (Header/Title/Content),
  `Select` (Trigger/Value/Indicator/Popover), `Tabs` (ListContainer/List/Tab/
  Indicator), `Toast` (Provider + imperative `toast()`). Prefer the shared
  wrappers in `ui.tsx` (`Modal`, `Card`, `SelectField`, `SegmentedTabs`) over
  raw HeroUI pieces.
- **Buttons use `onPress`, not `onClick`.** Map page-facing variants to HeroUI:
  `default → secondary`, `primary → primary`, `danger → danger`,
  `ghost → ghost`.
- `SelectField` takes `value: string` / `onChange: (v: string) => void` (empty
  string = nothing selected) and wraps HeroUI's compound `Select`, whose native
  API is `value: Key | null` / `onChange: (Key | null) => void`.
- Toasts: `const { show } = useToast()`; `show('success' | 'error' | 'info', msg)`.
- Every request goes through `api()` so tenant + JSON handling stays in one place.
- Amphora has no tenant header and no list/delete endpoints — `AmphoraPage`
  keeps a client-side registry in `localStorage['olympus.amphora.objects']` keyed
  by bucket.
- Snapshot lists on Clio/Mneme require an instance/cluster selector first
  (backend needs `instance`/`cluster` query params), so those pages surface
  snapshots per resource rather than globally.
- Keep `pagePath`/state initialised from `useAsync` data with `(x ?? [])`
  guards; strict TS (`noUnusedLocals`) is enforced by `tsc -b`.