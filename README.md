# release-hub

Self-hosted release distribution hub for personal Android (later iOS) apps.
Single Go binary + SQLite. Owns versioning, artifact storage, channels and the
update-manifest API so coding environments only talk to one endpoint.

## Channels

- `public` — production releases (Play production track)
- `internal` — testing tracks (Play internal testing)
- `direct` — direct APK distribution + update manifest for in-app updaters
  (wire-compatible with Tiny Firewall's `AppUpdater` / `UPDATE_URL`)

## Auth

- **UI**: single admin password, set on first visit (`/setup`), session cookie (30d).
- **API**: `Authorization: Bearer rh_...` tokens, created in the UI (shown once)
  or via `POST /api/tokens`. Stored as SHA-256 hashes.
- **Public, no auth** (devices need them): `GET /api/apps/{slug}/manifest`,
  `GET /artifacts/...`, `GET /health`.
- When self-hosting outside exe.dev, run behind TLS (e.g. Caddy/nginx) — the
  session cookie and bearer tokens must not cross the network in cleartext.

## API

## Apps and platforms

An app is a product (one slug); each platform variant — android, ios — has
its own package/bundle name, signing key, Play credentials, releases and
version codes. `POST /api/apps` registers the product (slug only);
`POST /api/apps/{slug}/platforms` adds each platform (e.g. android now, ios
when it ships). In the UI, registering asks for the slug alone — platforms
are added from the app's own page.

```
POST /api/apps                              form: slug
GET  /api/apps                              → [{slug, platforms:[{platform, packageName}]}]
POST /api/apps/{slug}/platforms             form: platform, packageName
POST /api/apps/{slug}/releases              multipart: file, channel, versionCode?,
                                            versionName?, notes  (versionCode must
                                            increase; default max+1)
POST /api/apps/{slug}/{platform}/releases   same, explicit platform
GET  /api/apps/{slug}/releases              (also /{platform}/… everywhere)
POST /api/tokens                            form: name → token shown once
GET  /api/apps/{slug}/manifest?channel=direct    (public)
GET  /api/apps/{slug}/{platform}/tracks          (Play track inventory)
GET  /api/apps/{slug}/{platform}/testers         (hub tester inventory)
GET  /artifacts/{slug}/{platform}/{file}         (public)
```

Testers: `GET …/testers` returns `{groups, emails}` — the hub-wide Google
Groups (pushable to closed tracks) and individual tester emails
(Settings → Individual tester emails). Google's API cannot push personal
addresses or email lists at all, so the emails list is the copy-paste
source for Play Console's tester email lists.

Platform defaults to `android` when omitted from the path, so
`/api/apps/{slug}/manifest` keeps working for android-only apps.

Upload example:

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=app-release.apk -F channel=direct \
     -F versionCode=142 -F versionName=1.15 \
     https://hub.example.com/api/apps/tinyfirewall/releases
```

## Storage backends

Artifacts live behind a `Storage` interface; pick with flags (no code change):

**Local filesystem (default)**
```
./srv/srv -artifacts /data/artifacts
```

**AWS S3** (also works with R2/MinIO via `-s3-endpoint`)
```
export AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=...
./srv/srv -s3-bucket my-releases -s3-region eu-west-1 [-s3-prefix release-hub]           [-s3-endpoint https://...r2.cloudflarestorage.com]
```

With S3:
- uploads stream to the bucket (multipart for 50MB+ APKs),
- download URLs in the manifest are **presigned** (7-day TTL) so devices fetch
  straight from S3 — the hub never proxies artifact traffic.
- if the bucket is public behind CloudFront, pass `-s3-public-base https://cdn.example.com`
  to use plain URLs instead of presigned ones.

Credentials: static keys via `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, or
any provider in the default AWS chain (env, shared config, IAM role).

## Signing keystores (per app)

App signing keys live in the hub — encrypted like Play credentials
(`RELEASE_HUB_SECRET_KEY`) — so **any** authenticated build environment can
produce signed releases, and the key survives a lost laptop. One keystore
per app; never share across apps.

**Auto-generated at platform creation.** Adding the android platform to an
app (`POST /api/apps/{slug}/platforms`) generates a fresh RSA-2048 keystore
(30-year self-signed cert, random high-entropy password, alias `release`)
and stores it encrypted — no keytool, no upload, no thinking about keys:

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" -F slug=myapp \
     https://hub.example.com/api/apps
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F platform=android -F packageName=io.example.myapp \
     https://hub.example.com/api/apps/myapp/platforms
# → {"slug":"myapp","platform":"android","signingKey":"generated","signingSha256":"26a4…"}
```

The generated key never rotates (rotation breaks installed-base update
signature continuity). To bring an app with an existing key history instead,
upload it explicitly — that replaces the generated one:

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=release.jks -F storePassword=... -F keyAlias=release \
     https://hub.example.com/api/apps/tinyfirewall/signing
# → {"stored":true,"keystoreSha256":"8862…"}
```

Fetch (CI / any coding env):

```bash
curl -sD /tmp/h -H "Authorization: Bearer $HUB_TOKEN" \
     -o app.jks https://hub.example.com/api/apps/tinyfirewall/signing
STORE_PW=$(grep -i x-hub-store-password /tmp/h | cut -d' ' -f2 | tr -d '\r')
ALIAS=$(grep -i x-hub-key-alias /tmp/h | cut -d' ' -f2 | tr -d '\r')
KEY_PW=$(grep -i x-hub-key-password /tmp/h | cut -d' ' -f2 | tr -d '\r')
# verify integrity:
echo "$(grep -i x-hub-keystore-sha256 /tmp/h | cut -d' ' -f2 | tr -d '\r')  app.jks" | sha256sum -c
```

Then point gradle at it (`signingConfigs` reading env vars). Delete with
`POST /api/apps/{slug}/signing/delete`.

⚠️ This endpoint is bearer-auth only and returns live key material — the hub
token now unlocks signing. Keep tokens scoped and the hub behind TLS.
Also keep an offline backup of the keystore: if Play App Signing is NOT
enrolled, losing this key means users can never update the app.

## Google Play (optional)

Apps with Play enabled get their **.aab** uploads pushed to Google Play
automatically, alongside normal hub storage. A **single shared service
account** covers all apps; its JSON key is stored in the DB, encrypted at
rest (AES-256-GCM, key from the `RELEASE_HUB_SECRET_KEY` env var — 32
bytes, base64- or hex-encoded):

- `channel=public`   → Play **production** track
- `channel=beta`     → Play **open testing** (Google's track id `beta`)
- `channel=alpha`    → Play **closed testing** — the hub's `alpha` track,
  **created automatically on first use** via `edits.tracks.create`,
  no Console visit needed
- `channel=internal` → Play **internal testing** track
- `channel=direct` (or any `.apk`) → hub only, Play untouched

Five channels, Android-convention names. Legacy spellings (`open`,
`closed`, `closed:<name>`) still map correctly on input.

Play's API track ids: `production`, `beta` (open testing), `internal`
(internal testing) plus any manually created closed tracks by name.
`GET /api/apps/{slug}/{platform}/tracks` lists what exists for an app.
Tester groups (Settings → Beta testers) attach on the `alpha` (closed
testing) track only — internal uses Console email lists, open testing is
open to all.

**Full walkthrough** — Play Console setup, service account + JSON key,
granting release access, enabling per app (UI **or** API), testers, and
troubleshooting: see [`docs/play-internal-testing.md`](docs/play-internal-testing.md).

The release itself is recorded even if Play publishing fails — the API
response includes `playRelease` or `playError` so CI can decide whether
to fail.

Setup is now **one service account for the whole hub** — upload it once, then
enable apps individually (no per-app credential copies to manage):

```bash
# once per hub: store the shared service account
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=service-account.json \
     https://hub.example.com/api/play-accounts
# → {"id":1,"serviceAccount":"hub@project.iam.gserviceaccount.com"}

# per app: enable Play publishing against it
curl -H "Authorization: Bearer $HUB_TOKEN" -F account=1 \
     https://hub.example.com/api/apps/tinyfirewall/play
# → {"playEnabled":true,"serviceAccount":"hub@…"}

# disable an app:  -F enable=false
# rotate the key:  re-POST the file to /api/play-accounts (same email replaces)
# remove account:  POST /api/play-accounts/delete  -F id=1
```

In the UI: **Settings → Google Play service accounts** stores the key once;
each app's page has an Enable/disable toggle (dropdown of shared accounts,
or upload a new key inline). Uploading the JSON on an app page also works
and creates the shared account implicitly.

A shared service account with account-wide release access serves many apps —
that's the intended setup.

Upload example:

```bash
./gradlew bundleRelease
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=app-release.aab -F channel=internal -F versionName=1.15 \
     https://hub.example.com/api/apps/tinyfirewall/releases
# → {"apkUrl":…, "playRelease":"1.15 (115)"}

# closed testing (track auto-created on first use):
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=app-release.aab -F channel=alpha \
     https://hub.example.com/api/apps/tinyfirewall/releases

# open testing / production:
#   -F channel=beta      /   -F channel=public

# which closed track names exist?
curl -H "Authorization: Bearer $HUB_TOKEN" \
     https://hub.example.com/api/apps/tinyfirewall/android/tracks
# → [{"track":"production","channel":"public","isClosed":false,…},
#    {"track":"alpha","channel":"closed:alpha","isClosed":true,…}]
```

Note: Play requires versionCodes to strictly increase per app, and the
first upload of an app must come after its Console declarations
(privacy policy, VPN form, etc.) are complete.

## Docker

Multi-stage build (static binary, ~20MB image, runs as unprivileged user
10001; the entrypoint chowns the volume then drops privileges via su-exec):

```bash
docker build -t release-hub .
docker volume create release-hub-data
docker run -d --name release-hub \
  -p 9100:9100 \
  -v release-hub-data:/data \
  -e BASE_URL=...                       # see compose below for the flag
  release-hub
```

`/data` holds db.sqlite3 + artifacts — back that volume up.

Set `-base-url` to the public URL (it appears in manifests/download links):

```bash
docker run -d -p 9100:9100 -v release-hub-data:/data \
  release-hub \
  release-hub -listen :9100 -db /data/db.sqlite3 -artifacts /data/artifacts \
  -base-url https://hub.example.com
```

**S3 variant** (artifacts skip the volume entirely):

```bash
docker run -d -p 9100:9100 \
  -e AWS_ACCESS_KEY_ID=... -e AWS_SECRET_ACCESS_KEY=... \
  release-hub \
  release-hub -listen :9100 -db /data/db.sqlite3 \
  -base-url https://hub.example.com \
  -s3-bucket my-releases -s3-region eu-west-1
```

docker-compose:

```yaml
services:
  release-hub:
    build: .
    ports: ["9100:9100"]
    volumes: [release-hub-data:/data]
    environment:
      RELEASE_HUB_SECRET_KEY: ${RELEASE_HUB_SECRET_KEY}
    restart: unless-stopped
    command: >-
      release-hub -listen :9100 -db /data/db.sqlite3
      -artifacts /data/artifacts
      -base-url https://hub.example.com
      # -s3-bucket my-releases -s3-region eu-west-1
volumes:
  release-hub-data:
```

Run behind a TLS-terminating proxy (Caddy/nginx/Traefik) in production —
the session cookie and bearer tokens must not cross the network in cleartext.

## Deploy on Coolify

Coolify can deploy this repo directly — the Dockerfile is already
self-contained (static Go binary, unprivileged user, SQLite in `/data`).
No database service needed.

1. **New Resource → Application** in your Coolify project.
2. Point it at this repo (GitHub / Git provider). Build pack: **Dockerfile**
   (auto-detected). Leave the port at **9100** — that's what the container
   listens on; Coolify maps it to your domain automatically.
3. **Persistence** — Coolify treats `/data` as the state directory. In the
   application settings, add a **persistent storage volume** mounted at `/data`
   (file storage is fine; db.sqlite3 + artifacts live there). Without this,
   every redeploy starts with a fresh empty hub.
4. **Environment variables** (Application → Environment):

   | Variable | Required | Notes |
   |---|---|---|
   | `RELEASE_HUB_SECRET_KEY` | **yes** | 32-byte base64/hex. Encrypts Play credentials and signing keystores at rest. `openssl rand -base64 32`. Losing it = losing stored keys — back it up. |
   | `RELEASE_HUB_BASE_URL` | **yes** | Public address Coolify assigns (e.g. `https://hub.nesin.io`). Used in manifest/download links. |

5. **Base URL** — the manifest/download URLs embed the public address. Set it
   via the `RELEASE_HUB_BASE_URL` env var (also accepted as a `-base-url`
   flag). Use the **https** URL Coolify assigns — Coolify terminates TLS, and
   the session cookie is marked Secure automatically when the base URL is
   https.

6. **Deploy**. First visit to the domain shows the one-time `/setup` page to
   set the admin password; then register your app (it gets a signing key
   automatically) and create an API token for CI/`deploy.sh`.

### S3 artifacts on Coolify

If you'd rather keep APKs out of the Coolify volume, add the AWS env vars
(`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) and extend the start command
with `-s3-bucket my-releases -s3-region eu-west-1` (or `-s3-endpoint` for
R2/MinIO). SQLite still stays in `/data` either way.

### Upgrades

Push to the repo → Coolify rebuilds and redeploys. `/data` persists, so
apps, releases, tokens and stored signing keys survive updates. Migrations
run automatically on startup.

## Dev

```
make build && make test
./srv/srv -listen :9100 -db db.sqlite3 -artifacts artifacts -base-url https://...
```

systemd: `release-hub.service` (port 9100).

Code layout (one file per concern, single canonical route table):

```
cmd/srv/            flags + wiring (storage backend choice)
srv/
  server.go         Server, Options, New(), Serve()
  routes.go         THE route table (used by Serve and tests — no drift)
  auth.go           bearer tokens, sessions, middleware
  api_apps.go       app/platform registration + tokens (API)
  api_releases.go   upload, release list, Play publish (API)
  public.go         manifest + artifact download (device-facing, no auth)
  handlers_ui.go    server-rendered pages
  templates.go      go:embed templates + static assets, render helpers
  signing.go        keystore generation/storage (PKCS#12 w/ friendlyName)
  play.go           Google Play Publishing API client
  storage.go        Storage interface: LocalStorage, S3Storage
  secrets.go        at-rest encryption of keystores/credentials
  templates/*.html  pages ({{define "content"}} over base.html)
  static/           style.css, app.js (embedded, served at /static/)
db/
  migrations/       numbered SQLite migrations (auto-run at startup)
  queries/          sqlc queries → db/dbgen (regen: sqlc generate in db/)
```

