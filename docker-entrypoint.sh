#!/bin/sh
# Fix volume ownership (fresh named volumes are root-owned) then drop
# privileges to the unprivileged app user and exec the server.
set -e
if [ "$(id -u)" = "0" ]; then
    chown -R 10001:10001 /data 2>/dev/null || true
    exec su-exec 10001:10001 "$@"
fi
exec "$@"
