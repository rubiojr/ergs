#!/bin/sh
set -e

# Fix ownership of volumes if running as root
if [ "$(id -u)" = "0" ]; then
    chown -R ergs:ergs /data /config 2>/dev/null || true
    # Run command as ergs user
    exec su-exec ergs "$@"
else
    # Already running as ergs user
    exec "$@"
fi
