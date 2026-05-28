#!/usr/bin/env bash
# Thin wrapper around golang-migrate, built from the module with the pgx5 driver
# (so no separately-installed CLI is needed). Reads CF_DB_* like the app config.
#
#   ./migrate.sh up
#   ./migrate.sh down 1
#   ./migrate.sh version
set -euo pipefail

host="${CF_DB_HOST:-localhost}"
port="${CF_DB_PORT:-5432}"
user="${CF_DB_USER:-postgres}"
pass="${CF_DB_PASSWORD:-postgres}"
name="${CF_DB_NAME:-contentflow}"
sslmode="${CF_DB_SSLMODE:-disable}"
dir="${CF_MIGRATIONS_DIR:-migrations}"

dsn="pgx5://${user}:${pass}@${host}:${port}/${name}?sslmode=${sslmode}"

exec go run -tags pgx5 github.com/golang-migrate/migrate/v4/cmd/migrate \
	-path "$dir" -database "$dsn" "$@"
