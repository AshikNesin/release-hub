# Release Hub for Android

Your own app store for the apps on your release-hub server: it lists every app with an android platform,
installs the newest **direct**-channel APK, and keeps installed apps up to date in the background, itself
included.

## How it works

| Piece | Where |
|---|---|
| Release hub calls (`GET /api/apps`, `…/android/manifest?channel=direct`, `…/android/releases`) | `android/app/src/main/kotlin/groot/mobile/data/releasehub/` |
| Release hub URL, Keystore-encrypted token, update preferences | `android/app/src/main/kotlin/groot/mobile/data/ReleaseHubSettings.kt` |
| Release hub apps joined with installed versions | `android/app/src/main/kotlin/groot/mobile/data/catalog/` |
| Download (SHA-256 checked), PackageInstaller sessions, results | `android/app/src/main/kotlin/groot/mobile/install/` |
| Background checks (WorkManager), notifications | `android/app/src/main/kotlin/groot/mobile/updates/` |
| Screens: apps, app page, connect, settings | `android/app/src/main/kotlin/groot/mobile/feature/` |

- **Connecting.** The app needs the release hub URL and an API token (release hub UI: Settings › API tokens). The token is
  checked against `GET /api/apps` before it is saved, then stored encrypted with an Android Keystore key.
  Release-hub tokens can also read signing keys, so create one per phone and revoke it if the phone is lost.
  Release builds talk https only; debug builds also accept `http://` for a release hub on your LAN (`make run`).
- **What's installable.** Only direct-channel APKs. Internal/public releases are AABs for Play; the release hub's
  manifest endpoint serves the newest direct release, and release history hides `.aab` files.
- **Installing.** Downloads are verified against the manifest's size and SHA-256, then handed to a
  PackageInstaller session. Installs run one at a time; "Update all" queues them.
- **Silent updates.** On Android 12+ the app asks for no confirmation, and Android honours that for apps
  Release Hub installed itself (it is their installer of record; Android 14+ also needs the app to hold
  `UPDATE_PACKAGES_WITHOUT_USER_ACTION`, which it declares). Anything else (first installs, apps installed
  by adb or Play) gets Android's confirmation: on screen when the app is open, as a notification otherwise.
- **Background checks.** WorkManager, every 1/6/12/24 hours (default 6), on unmetered networks by default,
  battery and storage not low. Auto-update can be switched off globally or per app; updates it doesn't
  install are summarized in one notification. Release Hub's own update installs last, since it restarts
  the app.
- **Signing.** An update installs only when the APK is signed with the installed app's key. Apps from the
  release hub are signed with the per-app key it generates; an app installed from Play is signed by Play and
  must be uninstalled before the release hub's build can replace it.

## Size note

The one dependency added on top of the boilerplate is `androidx.work:work-runtime` (background checks,
as `docs/guides/android.md` asks). Networking and JSON use the platform (`HttpURLConnection`, `org.json`).
Check the budget with `pnpm size:android`.

## Releasing Release Hub itself

Register the app on the release hub once (slug `release-hub`, android package `io.nesin.releasehub`), then bump
`android/version.properties` and run the root workflow **Mobile Release**
(`.github/workflows/mobile-release.yml` at the repo root). It fetches the release hub's signing key, builds the
release APK and uploads it to the direct channel, so installed copies update themselves.

## Release hub API additions

The app reads two additive fields the release hub now serves: `notes` and `createdAt` on the manifest, and
`fileName` and `notes` on the releases list. Older hubs work; the app just shows no release notes.
