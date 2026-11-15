# TODO

Pending items for the Android app (`mobile/`) and its setup. Tick them off as you go.

## Set up the hub

- [ ] Register the app on the hub: slug `release-hub`, add the android platform with package
      `io.nesin.releasehub` (the hub generates its signing key).
- [ ] Create an API token for your phone (hub UI: Settings › API tokens). One per device; revoke it if the
      phone is lost, since hub tokens can also read signing keys.

## Configure GitHub (release-hub repo)

- [ ] `vars.RELEASE_HUB_URL` (e.g. `https://hub.example.com`) and an OIDC grant on the hub for
      `repo:ashiknesin/release-hub:ref:refs/heads/main` (App `release-hub` → Configuration →
      Keyless publishing) so `.github/workflows/mobile-release.yml` can publish keyless. A
      `secrets.RELEASE_HUB_TOKEN` also still works if you prefer.
- [ ] `vars.GROOT_APP_ID` + `secrets.GROOT_APP_PRIVATE_KEY` (a GitHub App installed on this repo) so the
      sync PRs from `.github/workflows/mobile-sync.yml` trigger CI. Without it the PRs still open, but
      `mobile.yml` won't run on them.

## First release and install

- [ ] Bump `mobile/android/version.properties` if needed, then run **Mobile Release** (Actions tab).
- [ ] Install the first copy on your phone by hand (open `https://<hub>/apps/release-hub/download` on the
      phone, or the debug APK artifact from a `mobile.yml` run). Allow "Install unknown apps" for the
      browser once.
- [ ] Open Release Hub, connect with the phone's token, allow notifications and "Install apps".

## Test on a real device (not done yet: only tests and screenshots ran)

- [ ] Install an app that isn't installed yet (expect Android's confirmation).
- [ ] Update an app that Release Hub installed (Android 12+: expect no confirmation).
- [ ] Update an app installed by adb or Play (expect a confirmation; Play-signed apps fail with "signed
      with a different key" — uninstall first).
- [ ] Background update: leave the app closed and wait for a check (or set the interval to 1 hour). Check
      the "updated" and "tap to confirm" notifications.
- [ ] Self-update: publish a newer Release Hub, let it update itself.
- [ ] Debug build against a local hub over plain http (`make run`, port 9100).

## Small fixes

- [ ] Set a real feedback address in `mobile/android/app/src/main/res/values/config.xml`
      (`config_feedback_email`); "Send feedback" stays hidden while it's `@example.com`.
- [ ] Confirm the first `mobile.yml` CI run is green, including the size budget (1.45 MB measured
      locally against 3 MB).

## Ideas and follow-ups

- [ ] Hub: read-only (device-scoped) API tokens, so a phone's token can't fetch signing keys.
- [ ] App: a foreground service for very large downloads (background work is limited to ~10 minutes).
- [ ] App: `setRequestUpdateOwnership` on Android 14+, so other installers can't silently replace hub apps.
- [ ] Web UI: a "Get the Android app" link (QR code) to `/apps/release-hub/download`.
- [ ] Lint warning: `android:allowBackup` is deprecated on Android 12+; add `dataExtractionRules`.
