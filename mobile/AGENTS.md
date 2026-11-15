# AGENTS.md

groot-mobile-app is a native Android (Kotlin + Compose) and iOS (Swift + SwiftUI) boilerplate.
Improvements land here and sync down to child apps; child fixes go back upstream as PRs.
The full design is in `docs/rfcs/0001-groot-mobile-app.md`.

## Where code goes

| You're changing | Put it in | Synced? |
|---|---|---|
| Screens, ViewModels, navigation, data, custom components | `android/app/`, `ios/App/` | Never — app-owned |
| Brand colors and spacing | `android/app/src/main/kotlin/groot/mobile/ui/brand/`, `ios/App/Sources/UI/Brand/` | Never |
| Theme, tokens, settings screen, build info, logging | `android/kit/`, `ios/GrootKit/` | Yes — read-only in children |
| Gradle conventions | `android/build-logic/` | Yes |
| Versions | `android/version.properties`, `ios/App/Config/Version.xcconfig` | Never |

In a child app, never edit kit files. Copy what you need into app code, or send the fix upstream with
`pnpm groot:upstream`. See `docs/guides/customizing-ui.md`.

## Rules and enforcers

| Rule | Enforcer |
|---|---|
| The kit never imports app code, ViewModels, navigation, networking or databases | `pnpm check:boundaries` |
| No hardcoded colors or dp values outside kit tokens and the app's `brand/` folder | `pnpm check:tokens` |
| Kit files stay unedited in children | `pnpm check:kit-edits` (warns) |
| Versions live in `android/version.properties`, never in build files | `pnpm check:boundaries` |
| Every path a doc mentions exists | `pnpm check:doc-refs` |
| Every change ships with a changeset (`pnpm changeset`; `--empty` for docs/CI-only) | `changeset status` in CI on PRs |
| Kit public API is explicit (`public`, return types) | Kotlin explicit API mode in `groot.android.library` |
| No new dependency without a size-budget note in the PR | `pnpm size:android`, `pnpm size:ios` (3 MB download budget each) + review |

## Commands

```bash
pnpm check            # everything CI runs; skips platforms this machine can't build, and says so
pnpm android:test     # JUnit + Robolectric + Compose UI tests
pnpm android:lint
pnpm size:android     # release AAB -> bundletool download size vs. budget
pnpm ios:test         # (macOS) kit + app unit tests on the simulator
pnpm ios:lint         # (macOS) SwiftLint
pnpm size:ios         # (macOS) iOS download size vs. budget
pnpm screenshots      # renders reference screens to android/app/build/outputs/roborazzi/
pnpm groot:check      # (child apps) what would sync from the boilerplate
```

Setup and prerequisites: `docs/quick-start.md`.

## Conventions

- Small files, one concept each. Screens are stateless composables; a `...Route` wrapper connects a ViewModel.
- Read colors from `MaterialTheme.colorScheme` / `GrootTheme.colors` and spacing from `S` (e.g. `S.lg`).
- Unit tests use fakes passed by constructor: no network, clock or real disk.
- Compose performance: Strong Skipping is on; don't add `@Stable`/`@Immutable` without a measured problem.
  Hoist state, don't allocate new parameter objects per recomposition, use `derivedStateOf` for fast-changing derived values.
- Background work: none in the kit. Apps follow `docs/guides/android.md` (wake-lock vital).

## This app

<!-- Child apps: your own agent notes go below this line. Sync never conflicts with this section. -->

Release Hub for Android: installs and auto-updates apps from a release-hub server. This app lives in
`mobile/` of the release-hub repo (a groot-mobile-app child in a subdirectory): run every `pnpm` command
from this folder, and CI from the repo root (`.github/workflows/mobile*.yml`). Product notes:
`docs/app/README.md`.

- Kit, build-logic, `.groot/`, `scripts/` and `docs/` (outside `docs/app/`) are synced from the
  boilerplate; never edit them here. Kit fixes go upstream with `pnpm groot:upstream`.
- The release hub API is the contract: keep `data/releasehub/ReleaseHubJson.kt` in step with the server's `srv/public.go`,
  `srv/api_apps.go` and `srv/api_releases.go`. Only the bearer API (`/api/...`) and public endpoints
  are for devices; never call `/api/v1` (browser session only).
- Never send the release-release-hub token anywhere but the release hub (artifact URLs may be S3/CDN) and never log it.
- Installs go through `InstallController` only, so the queue, progress and notifications stay consistent.

