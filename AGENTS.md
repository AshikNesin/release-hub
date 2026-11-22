# Agent Instructions

release-hub: a Go server (srv/, SQLite) with a React UI (web/) embedded into
the binary. See docs/development.md for structure, the UI section in particular.

- Build: `make build` (pnpm builds web/, then go build embeds web/dist).
  Test: `make test` (go test ./... and the UI typecheck).
- The UI uses groot's kit and shell, vendored in `web/vendor/groot/`. Never
  edit vendored files; run `web/scripts/sync-groot.sh` to refresh them. The
  only release-hub overrides there are `shell/src/store/auth.ts` and
  `shell/src/pages/settings/sections.tsx`.
- Follow groot's design rules in web/src: semantic colour tokens only
  (`text-success`, `bg-muted`, …, never raw palette classes), `Text` /
  typography classes instead of ad-hoc text sizes, `PageLayout` for every
  routed page, `useAction` for mutations (confirm + toast + invalidate).
- The UI talks only to `/api/v1` (srv/api_v1.go), which uses the groot shell
  envelope. Change `srv/api_v1_handlers.go` and `web/src/lib/types.ts`
  together. The bearer API under `/api/` is what CI and devices use: keep it
  backwards compatible.
- `mobile/` is the Android app, a groot-mobile-app child in a subdirectory. Its rules are in
  `mobile/AGENTS.md`; run its `pnpm` commands from `mobile/`. The app reads the bearer API and the
  public manifest: when changing those responses, update `mobile/android/app/.../data/releasehub/ReleaseHubJson.kt`
  (additive fields only; installed apps on old versions keep parsing the old shape).
