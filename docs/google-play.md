# Google Play (optional)

Apps with Play enabled get their **.aab** uploads pushed to Google Play
automatically, alongside normal release hub storage. A **single shared service
account** covers all apps; its JSON key is stored in the DB, encrypted at
rest (AES-256-GCM, key from the `RELEASE_HUB_SECRET_KEY` env var, 32
bytes, base64- or hex-encoded):

- `channel=public`   → Play **production** track
- `channel=beta`     → Play **open testing** (Google's track id `beta`)
- `channel=alpha`    → Play **closed testing** on the release hub's `alpha` track,
  **created automatically on first use** via `edits.tracks.create`,
  no Console visit needed
- `channel=internal` → Play **internal testing** track
- `channel=direct` (or any `.apk`) → release hub only, Play untouched

Five channels, Android-convention names. Legacy spellings (`open`,
`closed`, `closed:<name>`) still map correctly on input.

Play's API track ids: `production`, `beta` (open testing), `internal`
(internal testing) plus any manually created closed tracks by name.
`GET /api/apps/{slug}/{platform}/tracks` lists what exists for an app.
Tester groups (Settings → Beta testers) attach on the `alpha` (closed
testing) track only; internal uses Console email lists, open testing is
open to all.

**Full walkthrough**: Play Console setup, service account + JSON key,
granting release access, enabling per app (UI **or** API), testers, and
troubleshooting: see [`play-internal-testing.md`](play-internal-testing.md).

The release itself is recorded even if Play publishing fails; the API
response includes `playRelease` or `playError` so CI can decide whether
to fail.

Setup is now **one service account for the whole release hub**: upload it once, then
enable apps individually (no per-app credential copies to manage):

```bash
# once per release hub: store the shared service account
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -F file=service-account.json \
     https://release-hub.example.com/api/play-accounts
# → {"id":1,"serviceAccount":"release release-hub@project.iam.gserviceaccount.com"}

# per app: enable Play publishing against it
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" -F account=1 \
     https://release-hub.example.com/api/apps/tinyfirewall/play
# → {"playEnabled":true,"serviceAccount":"release-hub@…"}

# disable an app:  -F enable=false
# rotate the key:  re-POST the file to /api/play-accounts (same email replaces)
# remove account:  POST /api/play-accounts/delete  -F id=1
```

In the UI: **Settings → Google Play service accounts** stores the key once;
each app's page has an Enable/disable toggle (dropdown of shared accounts,
or upload a new key inline). Uploading the JSON on an app page also works
and creates the shared account implicitly.

A shared service account with account-wide release access serves many apps;
that's the intended setup.

Upload example:

```bash
./gradlew bundleRelease
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -F file=app-release.aab -F channel=internal -F versionName=1.15 \
     https://release-hub.example.com/api/apps/tinyfirewall/releases
# → {"apkUrl":…, "playRelease":"1.15 (115)"}

# closed testing (track auto-created on first use):
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -F file=app-release.aab -F channel=alpha \
     https://release-hub.example.com/api/apps/tinyfirewall/releases

# open testing / production:
#   -F channel=beta      /   -F channel=public

# which closed track names exist?
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     https://release-hub.example.com/api/apps/tinyfirewall/android/tracks
# → [{"track":"production","channel":"public","isClosed":false,…},
#    {"track":"alpha","channel":"alpha","isClosed":true,…}]
```

Note: Play requires versionCodes to strictly increase per app, and the
first upload of an app must come after its Console declarations
(privacy policy, VPN form, etc.) are complete.
