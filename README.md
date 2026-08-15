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

```
POST /api/apps                     form: slug, packageName, platform
GET  /api/apps
POST /api/apps/{slug}/releases     multipart: file, channel, versionCode?,
                                   versionName?, notes  (versionCode must
                                   increase; default max+1)
GET  /api/apps/{slug}/releases
POST /api/tokens                   form: name → token shown once
GET  /api/apps/{slug}/manifest?channel=direct    (public)
GET  /artifacts/{slug}/{file}                       (public)
```

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

**Auto-generated at app creation.** Registering an Android app
(`POST /api/apps`) generates a fresh RSA-2048 keystore (30-year self-signed
cert, random high-entropy password, alias `release`) and stores it encrypted —
no keytool, no upload, no thinking about keys:

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F slug=myapp -F packageName=io.example.myapp \
     https://hub.example.com/api/apps
# → {"slug":"myapp","signingKey":"generated","signingSha256":"26a4…"}
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
automatically, alongside normal hub storage. The service-account JSON is
stored in the DB, encrypted at rest (AES-256-GCM, key from the
`RELEASE_HUB_SECRET_KEY` env var — 32 bytes, base64- or hex-encoded):

- `channel=public`   → Play **production** track
- `channel=internal` → Play **internal testing** track
- `channel=direct` (or any `.apk`) → hub only, Play untouched

The release itself is recorded even if Play publishing fails — the API
response includes `playRelease` or `playError` so CI can decide whether
to fail.

Setup (per app, one API call — no server filesystem access needed):

1. Play Console → Users & permissions → API access → link a Google Cloud
   project and create a service account; grant it release permissions.
2. Generate a 32-byte key: `openssl rand -base64 32`, export it as
   `RELEASE_HUB_SECRET_KEY` for the hub process.
3. Enable for the app:

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=service-account.json \
     https://hub.example.com/api/apps/tinyfirewall/play
# → {"playEnabled":true,"serviceAccount":"hub@project.iam.gserviceaccount.com"}

# disable:  -F enable=false
```

A shared service account with account-wide release access can serve
many apps — one credential uploaded per app is fine.

Upload example:

```bash
./gradlew bundleRelease
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=app-release.aab -F channel=internal -F versionName=1.15 \
     https://hub.example.com/api/apps/tinyfirewall/releases
# → {"apkUrl":…, "playRelease":"1.15 (115)"}
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

## Dev

```
make build && make test
./srv/srv -listen :9100 -db db.sqlite3 -artifacts artifacts -base-url https://...
```

systemd: `release-hub.service` (port 9100).

