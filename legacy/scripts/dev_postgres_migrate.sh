#!/usr/bin/env bash
# Временный Postgres в Docker + alembic upgrade head + проверка колонок.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PORT="${POSTGRES_PORT:-54332}"
CONTAINER="${POSTGRES_CONTAINER:-seo_os_pg_migrate}"
IMAGE="${POSTGRES_IMAGE:-postgres:16-alpine}"

if [[ ! -x "$ROOT/.venv/bin/alembic" ]]; then
  echo "Нет .venv с alembic. Выполните: python3.11 -m venv .venv && .venv/bin/pip install -e ." >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker не найден в PATH." >&2
  exit 1
fi

docker rm -f "$CONTAINER" 2>/dev/null || true
docker run -d \
  --name "$CONTAINER" \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_DB=seo_os \
  -p "${PORT}:5432" \
  "$IMAGE"

cleanup() {
  docker rm -f "$CONTAINER" 2>/dev/null || true
}
trap cleanup EXIT

echo "Ожидание PostgreSQL..."
for _ in $(seq 1 60); do
  if docker exec "$CONTAINER" pg_isready -U postgres -d seo_os >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

export DATABASE_URL="postgresql+psycopg://postgres:postgres@127.0.0.1:${PORT}/seo_os"

echo "Применение миграций..."
"$ROOT/.venv/bin/alembic" upgrade head

echo ""
echo "Колонки keyword_candidates (ожидается metadata):"
docker exec "$CONTAINER" psql -U postgres -d seo_os -c '\d keyword_candidates' | sed -n '1,25p'

echo ""
echo "Колонки published_pages (ожидается metadata):"
docker exec "$CONTAINER" psql -U postgres -d seo_os -c '\d published_pages' | sed -n '1,25p'

echo ""
echo "OK: миграция wave1_initial применена."
