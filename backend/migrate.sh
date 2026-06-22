#!/usr/bin/env bash
# Manual golang-migrate CLI wrapper (ad-hoc up/down/create). Reads CF_DB_* like
# the app config. The app runs migrations itself on startup; this is for manual
# ops. Install the CLI once:
#   go install -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
#
#   ./migrate.sh up
#   ./migrate.sh down 1
#   ./migrate.sh version
set -euo pipefail

if ! command -v migrate >/dev/null 2>&1; then
	echo "golang-migrate CLI not found. Install it once:" >&2
	echo "  go install -tags 'pgx5' github.com/golang-migrate/migrate/v4/cmd/migrate@latest" >&2
	exit 1
fi

host="${CF_DB_HOST:-localhost}"
port="${CF_DB_PORT:-5432}"
user="${CF_DB_USER:-postgres}"
pass="${CF_DB_PASSWORD:-postgres}"
name="${CF_DB_NAME:-contentflow}"
sslmode="${CF_DB_SSLMODE:-disable}"
dir="${CF_MIGRATIONS_DIR:-migrations}"

dsn="pgx5://${user}:${pass}@${host}:${port}/${name}?sslmode=${sslmode}"

exec migrate -path "$dir" -database "$dsn" "$@"
