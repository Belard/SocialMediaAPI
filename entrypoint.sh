#!/bin/sh
set -e

# Fix ownership of the bind-mounted uploads directory at runtime.
# This runs as root (before the USER switch), then drops privileges via su-exec.
chown -R appuser:appgroup /app/uploads

exec su-exec appuser "$@"
