# Sync

Child apps stay current with the boilerplate through groot's sync engine (`.groot/`). Only the path
patterns differ from groot: they live in `.groot/lib/patterns.ts`.

## Commands

```bash
pnpm groot:check      # dry run: what would change
pnpm groot:sync       # apply clean changes, 3-way merge the rest, write conflict markers
pnpm groot:resolve    # resolve recorded conflicts with the pi coding agent
pnpm groot:upstream   # open a PR on the boilerplate with your kit fix
```

`.github/workflows/sync.yml` runs the check twice a day and opens a `chore/groot-sync` PR. Give it a GitHub
App token (`vars.GROOT_APP_ID`, `secrets.GROOT_APP_PRIVATE_KEY`); a PR opened with the default
`GITHUB_TOKEN` never triggers CI.

## Sync classes

| Class | Examples | What sync does |
|---|---|---|
| KIT | `android/kit/`, `android/build-logic/`, `.github/workflows/`, `scripts/`, `docs/` | 3-way reconcile; children treat these as read-only |
| SHARED | `AGENTS.md`, `package.json`, `.gitignore`, `android/settings.gradle.kts`, `android/gradle/libs.versions.toml` | 3-way merge; both sides edit |
| APP | `android/app/`, `android/version.properties`, `docs/app/`, `README.md` | Never synced or upstreamed |
| IMMUTABLE | secrets, keystores, Android `local.properties`, sync state | Never synced; can't be overridden |

Nothing is blindly overwritten: an edit to a KIT file that doesn't overlap an upstream change is merged
silently, which is why `pnpm check:kit-edits` warns about edited kit files.

## Single-platform apps

An app without iOS (or Android) lists the other platform in `.groot/boilerplate-sync.json`:

```json
"additional_exclusions": ["ios/**", ".github/actions/setup-ios/**", ".swiftlint.yml"]
```

Those paths are never synced or upstreamed, CI skips the missing platform, and `pnpm check` says it skipped it.
`scripts/new-app.sh --platforms android` writes this for you. To add the platform back later, copy its folder
from the boilerplate first: groot won't restore a folder this repo once tracked and then deleted.

## Apps inside a monorepo

A child app can live in a subdirectory of another repo, e.g. a server repo with its companion app in `mobile/`:

```bash
scripts/new-app.sh ../my-server/mobile --platforms android
cd ../my-server/mobile && pnpm groot:setup
```

`new-app.sh` sees that the destination is inside a git repo, so it doesn't `git init` or commit, and leaves out
the files GitHub only reads at a repo root: `.github/workflows/`, `.github/dependabot.yml` and the PR template.
They are listed in `additional_exclusions`, so sync never brings them back. All `pnpm groot:*` and `pnpm check`
commands run from the app's folder, and paths in the sync report are relative to it.

Two things are yours to wire at the repo root:

- **CI.** Add a root workflow that runs the app's checks with `working-directory: mobile` (Android: `./gradlew
  lintDebug testDebugUnitTest` in `mobile/android`). The kit's composite action still works from there:
  `uses: ./mobile/.github/actions/setup-android`.
- **Scheduled sync.** Copy the boilerplate's `sync.yml` to the root, run its steps with
  `working-directory: mobile`, and point the PR body at `mobile/.groot/sync-report.json`.

One groot-mobile-app child per repo: the baseline lives in the repo-wide ref `refs/groot/baseline`.
