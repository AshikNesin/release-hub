# ---- build ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/release-hub ./cmd/srv

# ---- run ----
FROM alpine:3.20
RUN adduser -D -u 10001 hub && apk add --no-cache su-exec ca-certificates
COPY --from=build /out/release-hub /usr/local/bin/release-hub
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
VOLUME ["/data"]
EXPOSE 9100
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["release-hub", "-listen", ":9100", "-db", "/data/db.sqlite3", "-artifacts", "/data/artifacts", "-base-url", "http://localhost:9100"]
