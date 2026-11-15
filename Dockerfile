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
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The built UI replaces the placeholder dist/ and is embedded into the binary.
COPY --from=ui /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/release-hub ./cmd/srv

# ---- run ----
FROM alpine:3.20
RUN adduser -D -u 10001 release-hub && apk add --no-cache su-exec ca-certificates
COPY --from=build /out/release-hub /usr/local/bin/release-hub
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
VOLUME ["/data"]
EXPOSE 9100
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["release-hub", "-listen", ":9100", "-db", "/data/db.sqlite3", "-artifacts", "/data/artifacts"]
