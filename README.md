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

## Dev

```
make build && make test
./srv/srv -listen :9100 -db db.sqlite3 -artifacts artifacts -base-url https://...
```

systemd: `release-hub.service` (port 9100).
