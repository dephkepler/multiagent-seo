#!/usr/bin/env bash
# Дождаться готовности Postgres в docker compose (pg_isready).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

USER="${POSTGRES_USER:-seo_os}"
DB="${POSTGRES_DB:-seo_os}"

echo "Ожидание Postgres (service postgres)..."
for _ in $(seq 1 60); do
  if docker compose exec -T postgres pg_isready -U "$USER" -d "$DB" >/dev/null 2>&1; then
    echo "Postgres готов."
    exit 0
  fi
  sleep 1
done
echo "Таймаут: контейнер не ответил за 60 с. Проверьте: docker compose ps && docker compose logs postgres" >&2
exit 1
