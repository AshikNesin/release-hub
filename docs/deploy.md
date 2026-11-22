# Deploy

Run release-hub with Docker, on Coolify, or straight from the binary. Artifacts can live on disk or in S3.

## Docker

Multi-stage build (static binary on Debian slim, with the bundled 1Password
Connect executables, see [1Password vault](1password.md); runs as
unprivileged user 10001; the entrypoint chowns the volume then drops
privileges via setpriv):

```bash
docker build -t release-hub .
docker volume create release-hub-data
docker run -d --name release-hub \
  -p 9100:9100 \
  -v release-hub-data:/data \
  -e BASE_URL=...                       # see compose below for the flag
  release-hub
```

`/data` holds db.sqlite3 + artifacts; back that volume up.

Set `-base-url` to the public URL (it appears in manifests/download links):

```bash
docker run -d -p 9100:9100 -v release-hub-data:/data \
  release-hub \
  release-hub -listen :9100 -db /data/db.sqlite3 -artifacts /data/artifacts \
  -base-url https://release-hub.example.com
```

**S3 variant** (artifacts skip the volume entirely):

```bash
docker run -d -p 9100:9100 \
  -e AWS_ACCESS_KEY_ID=... -e AWS_SECRET_ACCESS_KEY=... \
  release-hub \
  release-hub -listen :9100 -db /data/db.sqlite3 \
  -base-url https://release-hub.example.com \
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
      -base-url https://release-hub.example.com
      # -s3-bucket my-releases -s3-region eu-west-1
volumes:
  release-hub-data:
```

Run behind a TLS-terminating proxy (Caddy/nginx/Traefik) in production;
the session cookie and bearer tokens must not cross the network in cleartext.

## Deploy on Coolify

Coolify can deploy this repo directly: the Dockerfile is already
self-contained (static Go binary, unprivileged user, SQLite in `/data`).
No database service needed.

1. **New Resource → Application** in your Coolify project.
2. Point it at this repo (GitHub / Git provider). Build pack: **Dockerfile**
   (auto-detected). Leave the port at **9100**: that's what the container
   listens on; Coolify maps it to your domain automatically.
3. **Persistence**: Coolify treats `/data` as the state directory. In the
   application settings, add a **persistent storage volume** mounted at `/data`
   (file storage is fine; db.sqlite3 + artifacts live there). Without this,
   every redeploy starts with a fresh empty release hub.
4. **Environment variables** (Application → Environment):

   | Variable | Required | Notes |
   |---|---|---|
   | `RELEASE_HUB_SECRET_KEY` | **yes** | 32-byte base64/hex. Encrypts Play credentials and signing keystores at rest. `openssl rand -base64 32`. Losing it = losing stored keys; back it up. |
   | `RELEASE_HUB_BASE_URL` | **yes** | Public address Coolify assigns (e.g. `https://hub.nesin.io`). Used in manifest/download links. |
   | `OP_CONNECT_CREDENTIALS` | no | 1Password: the `1password-credentials.json` contents (JSON or base64). Runs Connect inside the container; see [1Password vault](1password.md). |
   | `OP_CONNECT_TOKEN` | with the above | 1Password Connect access token, read + write, one vault. |

5. **Base URL**: the manifest/download URLs embed the public address. Set it
   via the `RELEASE_HUB_BASE_URL` env var (also accepted as a `-base-url`
   flag). Use the **https** URL Coolify assigns; Coolify terminates TLS, and
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
  straight from S3; the release hub never proxies artifact traffic.
- if the bucket is public behind CloudFront, pass `-s3-public-base https://cdn.example.com`
  to use plain URLs instead of presigned ones.

Credentials: static keys via `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, or
any provider in the default AWS chain (env, shared config, IAM role).
