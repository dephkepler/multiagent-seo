# SEO OS (wave 1)

Монорепозиторий: **Alembic**-миграции PostgreSQL (волна 1) и пакет `seo_os` с enum-ами, совпадающими с типами в БД.

## Требования

- Python 3.11+
- PostgreSQL 14+ (нужны `gen_random_uuid()`, JSONB)
- Docker (опционально) — постоянный PostgreSQL через `docker compose` или разовый контейнер в `scripts/dev_postgres_migrate.sh`

## Установка

```bash
cd "/path/to/ahrefs api LangGraph Temporal"
python3.11 -m venv .venv
source .venv/bin/activate
pip install -e .
```

Если `pip` пишет `Invalid requirement: '#'`, вы случайно скопировали комментарий в одну строку с `pip` — выполняйте **`pip install -e .`** отдельной строкой, без `# ...` на той же строке.

## Переменные окружения

- `DATABASE_URL` — строка подключения SQLAlchemy, например  
  `postgresql+psycopg://user:pass@localhost:5432/seo_os`

`alembic/env.py` подставляет её вместо заглушки из `alembic.ini`.

### PostgreSQL в Docker

Из корня репозитория:

```bash
docker compose up -d postgres
```

Сразу после `up` сервер внутри контейнера может быть ещё не готов — **не запускайте `alembic` в ту же секунду**. Варианты:

- Docker Compose 2.29+: `docker compose up -d --wait postgres` (ждёт healthcheck).
- Или: `./scripts/wait_postgres.sh`
- Или подождите 5–10 с и проверьте `docker compose ps` (статус **healthy**).

Данные хранятся в volume `seo_os_pgdata`. Подключение **с хоста** (worker, `alembic`, `psql`): `localhost`, порт `POSTGRES_PORT` (по умолчанию `5432`), логин/пароль/БД — как в [`.env.example`](.env.example) (`POSTGRES_*` и тот же пароль в `DATABASE_URL`). Проверка:  
`psql "postgresql://seo_os:seo_os@127.0.0.1:5432/seo_os"` (подставьте свои значения, если меняли).

Если порт `5432` занят локальным PostgreSQL — задайте, например, `POSTGRES_PORT=5433` в `.env` и тот же порт в `DATABASE_URL`.

## Миграции

```bash
export DATABASE_URL="postgresql+psycopg://user:pass@localhost:5432/seo_os"
alembic upgrade head
```

Ревизии: `wave1_initial` (основная схема), `external_task_links_002` (связь `pipeline_id` с Trello и др.). Подробный статус и тесты: [`docs/STATUS_AND_TESTING.md`](docs/STATUS_AND_TESTING.md). Шаблон секретов: [`.env.example`](.env.example).

Просмотр последних строк в `external_task_links` и `agent_decisions`: `seo-os-db-audit` (нужен `DATABASE_URL` в окружении).

### Проверка миграции через Docker

Нужен **запущенный Docker** (на macOS — Docker Desktop). Скрипт поднимает временный Postgres, применяет `alembic upgrade head`, выводит `\d` для проверки колонки `metadata`, затем удаляет контейнер:

```bash
./scripts/dev_postgres_migrate.sh
```

Переменные: `POSTGRES_PORT` (по умолчанию `54332`), `POSTGRES_CONTAINER` (по умолчанию `seo_os_pg_migrate`).

Без Docker: создайте БД в своём PostgreSQL и выполните `export DATABASE_URL=... && alembic upgrade head`.

## Структура

- `alembic/versions/wave1_initial.py` — DDL волны 1
- `src/seo_os/db/enums.py` — зеркало ENUM для приложения
- `scripts/dev_postgres_migrate.sh` — локальный прогон миграции

## Runnable skeleton (Temporal + LangGraph)

1. Установите [Temporal CLI](https://docs.temporal.io/cli) и убедитесь, что команда `temporal` в PATH (на macOS часто: `brew install temporal`). Если `seo-os-demo` / `seo-os-worker` пишут **Connection refused на 127.0.0.1:7233** — dev-сервер Temporal не запущен. Запустите в **отдельном терминале** и оставьте работать:  
   `temporal server start-dev`  
   (по умолчанию `localhost:7233`).

2. Поднимите PostgreSQL и примените миграции (`DATABASE_URL` + `alembic upgrade head`).

3. **Worker** (очередь по умолчанию `seo-os-local`):
   ```bash
   export DATABASE_URL="postgresql+psycopg://..."
   export TEMPORAL_ADDRESS="${TEMPORAL_ADDRESS:-localhost:7233}"
   seo-os-worker
   ```

4. **Демо** (сидит `sites` / `campaigns` / `campaign_budgets` и стартует `SeoCampaignWorkflow` — parent + три child + activity `run_langgraph`):
   ```bash
   export DATABASE_URL="postgresql+psycopg://..."
   seo-os-demo
   ```

Переменные: `TEMPORAL_TASK_QUEUE` (по умолчанию `seo-os-local`) — должна совпадать у worker и клиента.

Что внутри: по умолчанию графы **`keyword_trello`**, **`content_trello`**, **`qa_trello`** (нужны переменные `TRELLO_*` в `.env`); запись шагов в `agent_decisions`, таблица `external_task_links`, проверка бюджета, quality gate в `content_trello` (заглушка). Старые `keyword_skeleton` / `content_skeleton` можно вызвать вручную через `run_langgraph` с соответствующим `graph_name`.

### HTTP API и чат (экспериментально)

Для **чата в браузере или из мобильного клиента** нужен HTTP-сервер. В проекте используется **FastAPI** + **uvicorn** (альтернативы — чистый Starlette, Flask и т.д.; FastAPI удобен для OpenAPI и типизации).

```bash
seo-os-api
# или: seo-os-api --reload --port 8000
```

- `GET /health` — проверка живости.
- `POST /v1/chat` — JSON `{"message": "..."}`; при **`OPENAI_API_KEY`** в `.env` ответ от модели (`OPENAI_CHAT_MODEL`, по умолчанию `gpt-4o-mini`). Без ключа — заглушка с подсказкой. Запуск workflow из чата — отдельный шаг.

Переменные: `CHAT_API_HOST`, `CHAT_API_PORT`, `CORS_ORIGINS`, `OPENAI_API_KEY`, `OPENAI_CHAT_MODEL` — см. [`.env.example`](.env.example). Документация эндпоинтов после запуска: `http://127.0.0.1:8000/docs`.

### Telegram-бот

Тот же ответ, что и у `POST /v1/chat` ([`complete_chat`](src/seo_os/api/chat_backend.py)), через long polling.

```bash
# В .env: TELEGRAM_BOT_TOKEN=... (только из @BotFather, не коммитить)
seo-os-telegram-bot
```

- **`/campaign`** — сид демо-кампании и старт `SeoCampaignWorkflow` (как [`seo-os-demo`](src/seo_os/cli/demo.py), но **без ожидания** завершения). Требуются запущенные **Temporal** и **`seo-os-worker`**, полный **Trello** в `.env`, и **`TELEGRAM_ALLOWED_USER_IDS`** (иначе команда отключена).

Нужны `OPENAI_API_KEY` и токен бота. `TELEGRAM_ALLOWED_USER_IDS` — для `/campaign` обязателен; для обычного чата, если пусто — ответ всем. Если токен когда-либо попал в чат или git — отзовите его в @BotFather и создайте новый.

**HTTP:** `POST /v1/campaign/start` с заголовком `X-Campaign-Start-Key: <CAMPAIGN_START_SECRET>` — тот же старт workflow (секрет обязателен, иначе 503).

## Заметки по схеме

- В `keyword_candidates` и `published_pages` дополнительный JSON называется **`metadata`** (в PostgreSQL это обычное имя колонки).
- В **SQLAlchemy Declarative** атрибут класса нельзя назвать `metadata` (конфликт с `MetaData`). Задавайте другое имя в Python и привязывайте к колонке БД, например:  
  `row_meta: Mapped[dict] = mapped_column("metadata", JSONB, default=dict)`  
  или `mapped_column("metadata", ..., key="row_metadata")` в зависимости от стиля маппинга.
- В `cost_tracking` дополнительные поля по-прежнему в колонке **`extra`** (учёт расходов, не «метаданные страницы»).
