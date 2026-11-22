#!/bin/sh
# Fix volume ownership (fresh named volumes are root-owned) then drop
# privileges to the unprivileged app user and exec the server. The bundled
# 1Password Connect server (when configured) keeps its state in
# /data/op-connect and runs as the same user, started by release-hub.
set -e
if [ "$(id -u)" = "0" ]; then
    chown -R 10001:10001 /data 2>/dev/null || true
    exec setpriv --reuid=10001 --regid=10001 --clear-groups "$@"
fi
exec "$@"
