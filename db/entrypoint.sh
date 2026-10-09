#!/bin/sh
set -eu

POSTGRES_DB="${DB_ENV_PREFIX}-bharat-index"
export POSTGRES_DB

exec /usr/local/bin/docker-entrypoint.sh "$@"
