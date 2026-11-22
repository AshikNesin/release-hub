# Development

## UI

The web UI is a React + Vite app in `web/`, built on the same UI kit and app
shell as [groot](https://github.com/AshikNesin/groot) so it looks and behaves
like every other groot app: sidebar shell, ⌘K palette (jump to any app),
full-page settings, light/dark/system theme. `make build` builds it and
embeds it into the Go binary (`web/embed.go`, `go:embed`), so deploys are
still one file.

- `web/vendor/groot/`: the vendored `@groot/ui` kit and the `@groot/shell`
  pieces release-hub uses (layout, sidebar, palette, settings chrome, theme,
  API client). Don't edit these; refresh them with
  `web/scripts/sync-groot.sh [path/to/groot]`. Two files there are
  release-hub overrides the sync never touches: `shell/src/store/auth.ts`
  (single admin password, no passkeys) and
  `shell/src/pages/settings/sections.tsx` (types only).
- `web/src/` holds release-hub's own pages: apps, app detail
  (releases / distribution / configuration), login/setup, settings sections.
- The Go side speaks the groot shell's wire contract (`srv/api_v1.go`):
  `{success, data}` / `{success: false, error: {code, message}}`, and a
  `SESSION_INVALID` 401 signs the UI out. Wire types live in
  `web/src/lib/types.ts` next to `srv/api_v1_handlers.go`.

## Dev

Needs Go 1.26+, Node 22+ and pnpm.

```
make build && make test   # UI build + Go binary; Go tests + UI typecheck
make dev                  # Go on :9100 + Vite (HMR) on :5173 → open :5173
./srv/srv -listen :9100 -db db.sqlite3 -artifacts artifacts -base-url https://...
```

A binary built without the UI (`go build` alone) still serves the API and
device endpoints, and shows a "UI not built" page at `/`.

systemd: `release-hub.service` (port 9100).

Code layout (one file per concern, single canonical route table):

```
cmd/srv/            flags + wiring (storage backend choice)
srv/
  server.go         Server, Options, New(), Serve()
  routes.go         THE route table (used by Serve and tests; no drift)
  auth.go           bearer tokens, sessions, middleware
  api_apps.go       app/platform registration + tokens (bearer API)
  api_releases.go   upload, release list, Play publish (bearer API)
  api_oidc.go       OIDC exchange endpoint (bearer API)
  oidc.go           keyless publishing: grants, token exchange, scoping
  github_app.go     this release hub's own GitHub App (one-click keyless setup)
  github_client.go  GitHub App client: installations, repo list, Actions variables
  prune.go          direct-channel release retention (auto-prune after upload)
  mcp.go            MCP server at /mcp (bearer API)
  api_v1.go         UI API: envelope, session + same-origin guard, auth routes
  api_v1_handlers.go UI API: app list/detail, retention, settings, tokens, Play accounts
  ui_helpers.go     Play links/identity, lazy Play-status reconciliation
  spa.go            serves the embedded UI (client routes → index.html)
  public.go         manifest + artifact download (device-facing, no auth)
  signing.go        keystore generation/storage (PKCS#12 w/ friendlyName)
  play.go           Google Play Publishing API client
  storage.go        Storage interface: LocalStorage, S3Storage
  secrets.go        at-rest encryption of keystores/credentials
  vault.go          1Password vault storage for secrets (via Connect)
  vault_bundled.go  runs 1Password Connect inside the release-hub container
  vault_migrate.go  moving stored keys into 1Password; vault status
web/
  embed.go          go:embed of web/dist (the built UI)
  src/              release-hub pages, typed API (lib/releaseHub.ts, lib/types.ts)
  vendor/groot/     vendored groot UI kit + shell (scripts/sync-groot.sh)
db/
  migrations/       numbered SQLite migrations (auto-run at startup)
  queries/          sqlc queries → db/dbgen (regen: sqlc generate in db/)
```
