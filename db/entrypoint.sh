#!/bin/sh
set -eu

if [ -z "${APP_ENV:-}" ]; then
    echo "APP_ENV must be set (for example: dev, qa, or prod)" >&2
    exit 1
fi

POSTGRES_DB="${APP_ENV}-bharat-index"
export POSTGRES_DB

exec /usr/local/bin/docker-entrypoint.sh "$@"
