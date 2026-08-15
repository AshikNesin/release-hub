# release-hub

Self-hosted release distribution hub for personal Android (later iOS) apps.
Single Go binary + SQLite. Owns versioning, artifact storage, channels and the
update-manifest API so coding environments only talk to one endpoint.

## Channels

- `public` — production releases (future: Play/App Store promotion)
- `internal` — testing tracks
- `api-share` — direct APK distribution + update manifest for in-app updaters
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
GET  /api/apps/{slug}/manifest?channel=api-share   (public)
GET  /artifacts/{slug}/{file}                       (public)
```

Upload example:

```bash
curl -H "Authorization: Bearer $HUB_TOKEN" \
     -F file=app-release.apk -F channel=api-share \
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

## Google Play (optional)

When `-play-creds-dir` is set, apps that have a service-account JSON file
named `<packageName>.json` in that directory get their **.aab** uploads
pushed to Google Play automatically, alongside normal hub storage:

- `channel=public`   → Play **production** track
- `channel=internal` → Play **internal testing** track
- `channel=api-share` (or any `.apk`) → hub only, Play untouched

The release itself is recorded even if Play publishing fails — the API
response includes `playRelease` or `playError` so CI can decide whether
to fail.

Setup (one-time):

1. Play Console → Users & permissions → API access → link a Google Cloud
   project and create a service account; grant it release permissions.
2. Download the service-account JSON key.
3. Save it as `/etc/release-hub/play/io.nesin.tinyfirewall.json` (i.e.
   `<packageName>.json` inside the creds dir; one file per app — an
   account with account-wide access can serve many apps).
4. Start the hub with `-play-creds-dir /etc/release-hub/play`.

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
    volumes:
      - release-hub-data:/data
      - ./play-creds:/data/play:ro        # optional: service-account JSONs
    restart: unless-stopped
    command: >-
      release-hub -listen :9100 -db /data/db.sqlite3
      -artifacts /data/artifacts
      -base-url https://hub.example.com
      -play-creds-dir /data/play
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
