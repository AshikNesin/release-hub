# API reference

Everything the release hub serves: channels, auth, and the bearer API that CI and scripts use.

## Channels

- `public`: production releases (Play production track)
- `internal`: testing tracks (Play internal testing)
- `direct`: direct APK distribution + update manifest for in-app updaters
  (wire-compatible with Tiny Firewall's `AppUpdater` / `UPDATE_URL`)

## Auth

- **UI**: single admin password, set on first visit (`/setup`), session cookie (30d).
  The React UI talks to `/api/v1/*` with that cookie only; bearer tokens are
  not accepted there, and cross-site writes are refused (Fetch Metadata/Origin).
- **API**: `Authorization: Bearer rh_...` tokens, created in the UI (shown once)
  or via `POST /api/tokens`. Stored as SHA-256 hashes.
- **Keyless (OIDC)**: GitHub Actions (or any OIDC issuer) can publish without
  a stored secret; see [Keyless publishing](keyless-publishing.md).
  The minted release-hub token is scoped to one app's publish endpoints and lives
  1 hour.
- **Public, no auth** (devices need them): `GET /api/apps/{slug}/manifest`,
  `GET /artifacts/...`, `GET /health`.
- When self-hosting outside exe.dev, run behind TLS (e.g. Caddy/nginx); the
  session cookie and bearer tokens must not cross the network in cleartext.

## Apps and platforms

An app is a product (one slug); each platform variant (android, ios) has
its own package/bundle name, signing key, Play credentials, releases and
version codes. `POST /api/apps` registers the product (slug only);
`POST /api/apps/{slug}/platforms` adds each platform (e.g. android now, ios
when it ships). In the UI, registering asks for the slug alone; platforms
are added from the app's own page.

```
POST /api/apps                              form: slug
GET  /api/apps                              → [{slug, platforms:[{platform, packageName}]}]
DELETE /api/apps/{slug}                     delete the app: platforms, releases,
                                            artifacts, stored signing keys and
                                            OIDC grants (minted tokens die too)
                                            (what Play published stays on Play)
POST /api/apps/{slug}/platforms             form: platform, packageName
POST /api/apps/{slug}/releases              multipart: file, channel, versionCode?,
                                            versionName?, notes  (versionCode must
                                            increase; default max+1)
POST /api/apps/{slug}/{platform}/releases   same, explicit platform
GET  /api/apps/{slug}/releases              (also /{platform}/… everywhere)
POST /api/tokens                            form: name → token shown once
POST /api/oidc/exchange                    form: token (a CI job's OIDC identity
                                            token, audience "release-hub")
                                            → {token, expiresIn, app}: a release hub
                                            token scoped to one app, 1h TTL
GET  /api/apps/{slug}/manifest?channel=direct    (public)
GET  /api/apps/{slug}/{platform}/tracks          (Play track inventory)
GET  /api/apps/{slug}/{platform}/testers         (release hub tester inventory)
GET  /artifacts/{slug}/{platform}/{file}         (public)
```

Testers: `GET …/testers` returns `{groups, emails}`: the release-hub-wide Google
Groups (pushable to closed tracks) and individual tester emails
(Settings → Individual tester emails). Google's API cannot push personal
addresses or email lists at all, so the emails list is the copy-paste
source for Play Console's tester email lists.

Platform defaults to `android` when omitted from the path, so
`/api/apps/{slug}/manifest` keeps working for android-only apps.

Upload example:

```bash
curl -H "Authorization: Bearer $RELEASE_HUB_TOKEN" \
     -F file=app-release.apk -F channel=direct \
     -F versionCode=142 -F versionName=1.15 \
     https://release-hub.example.com/api/apps/tinyfirewall/releases
```
