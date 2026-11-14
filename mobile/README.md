# Release Hub for Android

Your own app store for the apps on your [release-hub](../README.md) server: install them, see what's new,
and keep them (and this app) up to date automatically.

Built on [groot-mobile-app](https://github.com/AshikNesin/groot-mobile-app) and kept in sync with it.

- How it works: [`docs/app/README.md`](docs/app/README.md)
- Rules for agents and humans: [`AGENTS.md`](AGENTS.md)
- Setup: [`docs/quick-start.md`](docs/quick-start.md)

```bash
pnpm install
pnpm check            # repo checks + Android lint and unit tests
pnpm android:build    # debug APK at android/app/build/outputs/apk/debug/
pnpm screenshots      # renders the screens (light + dark) to android/app/build/outputs/roborazzi/
pnpm groot:check      # what would sync from the boilerplate
pnpm groot:sync       # pull boilerplate changes (kit, build logic, tooling)
```

The boilerplate's scheduled sync runs from the repo root as `.github/workflows/mobile-sync.yml` and opens a
`chore/groot-mobile-sync` PR; see "Apps inside a monorepo" in `docs/guides/sync.md`.
