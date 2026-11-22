# ---- ui ----
FROM node:22-alpine AS ui
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---- build ----
FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The built UI replaces the placeholder dist/ and is embedded into the binary.
COPY --from=ui /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/release-hub ./cmd/srv

# ---- 1Password Connect ----
# The bundled vault server (srv/vault_bundled.go): both Connect executables,
# copied from 1Password's own images at a pinned version and multi-arch
# digest. They run only when OP_CONNECT_CREDENTIALS is set. A Connect
# upgrade is a change here (tag + both digests).
FROM 1password/connect-api:1.8.4@sha256:0f74924c55c9156b8116f59f485223b2c2d30a5148d362562aa27838159bdffa AS op-connect-api
FROM 1password/connect-sync:1.8.4@sha256:ca34850f2a20ca302e688a9347cdb696f29ce3bfc011d2554eff338e315c9089 AS op-connect-sync

# ---- wget for health checks ----
# Platform health checks (Coolify) run curl or wget inside the container.
# Debian slim ships neither; a static busybox wget keeps the image small.
FROM busybox:1.37-musl@sha256:f6a95d611e0a8780fabdb2aec43e31275b2720dfefa3e41b16835bfc42454478 AS busybox

# ---- run ----
# Debian, not Alpine: the Connect executables are linked against glibc.
FROM debian:bookworm-slim
RUN useradd --uid 10001 --user-group --no-create-home --shell /usr/sbin/nologin release-hub
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=op-connect-api /usr/bin/connect-api /usr/local/bin/connect-api
COPY --from=op-connect-sync /usr/bin/connect-sync /usr/local/bin/connect-sync
COPY --from=busybox /bin/busybox /usr/local/bin/busybox
RUN ln -s busybox /usr/local/bin/wget
COPY --from=build /out/release-hub /usr/local/bin/release-hub
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
VOLUME ["/data"]
EXPOSE 9100
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1:9100/health || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["release-hub", "-listen", ":9100", "-db", "/data/db.sqlite3", "-artifacts", "/data/artifacts"]
