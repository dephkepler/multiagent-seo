# Статус реализации и что можно протестировать

Документ фиксирует **что уже сделано** в репозитории и **как это проверить**. Полноценных LLM-агентов и интеграций Ahrefs/GSC/CMS пока нет; добавлен **наблюдаемый slice Trello** (карточка, комментарии, перемещение по колонкам) поверх Temporal + LangGraph.

Шаблон переменных окружения: [`.env.example`](../.env.example) (секреты не коммитить).

**PostgreSQL в Docker:** в корне репозитория [docker-compose.yml](../docker-compose.yml) — `docker compose up -d postgres`. Подождите готовность БД перед `alembic` (иначе «connection refused» / «server closed»): `docker compose up -d --wait postgres` или [scripts/wait_postgres.sh](../scripts/wait_postgres.sh). С хоста — `localhost` и порт `POSTGRES_PORT`; `DATABASE_URL` и `POSTGRES_*` должны совпадать — см. [README](../README.md#postgresql-в-docker).

---

## Что уже готово

### 1. Схема данных

- Миграция `wave1_initial`: основные таблицы кампании, контента, `agent_decisions`, `cost_tracking`, `campaign_budgets` и др.
- Миграция `external_task_links_002`: связь **`pipeline_id` → внешняя задача** (`system=trello`, `external_id` = id карточки). Репозиторий: `src/seo_os/db/repos_external.py`.
- Зеркало enum’ов: `src/seo_os/db/enums.py`.

### 2. Графы LangGraph

| Граф | Файл | Назначение |
|------|------|------------|
| `keyword_trello` | `src/seo_os/graphs/keyword_trello.py` | Создать/переиспользовать карточку в колонке Keyword Research, handoff в state и комментарий в Trello. |
| `content_trello` | `src/seo_os/graphs/content_trello.py` | Комментарий handoff, перенос в Content Draft. |
| `qa_trello` | `src/seo_os/graphs/qa_trello.py` | QA → Done, финальные комментарии. |
| `keyword_skeleton` / `content_skeleton` | `src/seo_os/graphs/` | Старые детерминированные графы **без Trello** (по-прежнему доступны из `run_langgraph`, если указать имя графа). |

По умолчанию **child workflows** вызывают графы **`*_trello`** (см. `keyword_batch.py`, `content_batch.py`, `qa_batch.py`).

### 3. Интеграция Trello

- Клиент: `src/seo_os/integrations/trello.py` ([Trello REST API](https://developer.atlassian.com/cloud/trello/rest/)): создание карточки, смена списка, комментарии.
- Обязательные переменные: `TRELLO_API_KEY`, `TRELLO_TOKEN`, шесть id списков (`TRELLO_LIST_INBOX`, `TRELLO_LIST_KEYWORD_RESEARCH`, …) — см. `.env.example`.
- **Токен пользователя** не «обновляется» при каждом HTTP-запросе: один раз откройте ссылку авторизации (команда `seo-os-trello-auth` печатает готовый URL из `TRELLO_API_KEY`), подтвердите в браузере и положите выданный token в `TRELLO_TOKEN`. С `expiration=never` он долго живёт в `.env`; worker при вызовах API только подставляет `key` + `token`.
- Без настроенного Trello activity `run_langgraph` для `*_trello` вернёт **ошибку с понятным текстом** (не падает молча).

### 4. Temporal

- **Parent:** `SeoCampaignWorkflow` — один общий **`pipeline_id`**, три child подряд: Keyword → Content → QA; после каждой фазы и по завершении — activity **`notify_telegram`** (отчёт в Telegram, если заданы `TELEGRAM_BOT_TOKEN` и chat: из `CampaignInput.telegram_chat_id` или `TELEGRAM_REPORT_CHAT_ID`).
- **Child:** `KeywordBatchWorkflow`, `ContentBatchWorkflow`, `QaBatchWorkflow`.
- **Activity:** `run_langgraph` — бюджет-проверка, запуск графа, шаги в `agent_decisions`; для графов **без** skeleton списание в `cost_tracking` за «LLM» **не** дублируется для `*_trello` (учёт остаётся на проверке бюджета перед фазой).
- **Activity:** `keyword_search` — после фазы `keyword_trello` поддерживает 2 режима:
  - legacy adapter (`search`, `live_autocomplete`, `live_trends`, `live_youtube`) через [`keyword_search.py`](../src/seo_os/integrations/keyword_search.py),
  - direct sources (`direct_discovery`, `direct_autocomplete`, `direct_trends`, `direct_youtube`, `direct_ahrefs`) через локальные integrations и quality-слой (`dedupe/scoring/novelty`).

### 5. CLI и worker

- `seo-os-worker` — загружает `.env` через `python-dotenv`.
- `seo-os-demo` — сид + старт кампании; печатает `keyword_summary`, `content_summary`, `qa_summary`.
- `seo-os-trello-auth` — печатает URL для **одноразового** получения `TRELLO_TOKEN` (см. `.env.example`).
- `seo-os-trello-lists` — по `TRELLO_BOARD_SHORT_LINK` выводит id всех колонок и готовый блок `TRELLO_LIST_*`; `--bootstrap` создаёт на доске шесть стандартных колонок (Inbox … Blocked), если их ещё нет.
- `seo-os-api` — **FastAPI** + uvicorn: `GET /health`, `POST /v1/chat` (OpenAI при `OPENAI_API_KEY`; см. `src/seo_os/api/main.py`).
- `seo-os-telegram-bot` — Telegram long polling, `complete_chat` как HTTP; **`/campaign`** — старт `SeoCampaignWorkflow` с `telegram_chat_id` этого чата (отчёты о фазах сюда же). После успешного старта сохраняется **контекст кампании** для LLM: обычные сообщения идут с префиксом `campaign_id` / `workflow_id`. Команды **`/context`** (показать привязку), **`/context_off`** (сброс). Модуль: [`campaign_context.py`](../src/seo_os/bots/campaign_context.py). Нужны `TELEGRAM_ALLOWED_USER_IDS`, Temporal, worker, Trello. Логика старта: [`campaign_starter.py`](../src/seo_os/temporal/campaign_starter.py).
- `seo-os-db-audit` — последние строки `external_task_links` и `agent_decisions` в консоль (см. раздел «Запуск кампании из Telegram» и проверку БД).
- HTTP **`POST /v1/campaign/start`** — то же без ожидания завершения; заголовок `X-Campaign-Start-Key` = `CAMPAIGN_START_SECRET` в `.env`.

### 6. Вспомогательные скрипты

- `scripts/dev_postgres_migrate.sh` — Postgres в Docker + миграции.

---

## Что ещё не сделано

- **LLM в графах LangGraph** (keyword/content/qa) и tool-calling; в **HTTP-чате** `/v1/chat` и **Telegram-боте** уже используется OpenAI при `OPENAI_API_KEY`.
- Сигналы approve/reject в API.
- Webhook’и Trello в сторону приложения ([документация webhooks](https://developer.atlassian.com/cloud/trello/guides/rest-api/webhooks/)).
- Ahrefs, GSC, CMS.
- Отдельный **FastAPI Keyword Adapter** над вторым репозиторием (pipeline) — вне этого репо; контракт, режимы, этапы — см. [EXTERNAL_KEYWORD_SERVICE.md](EXTERNAL_KEYWORD_SERVICE.md). В SEO OS уже есть клиент, activity `keyword_search` и поля `KeywordBatchInput.keyword_*` (при появлении adapter задайте `KEYWORD_SEARCH_*` в `.env`).

---

## Как протестировать чат (OpenAI)

1. `OPENAI_API_KEY` (и при желании `OPENAI_CHAT_MODEL`) в `.env`.
2. `seo-os-api`, затем например:  
   `curl -s -X POST http://127.0.0.1:8000/v1/chat -H "Content-Type: application/json" -d '{"message":"Привет, кратко опиши SEO OS"}'`

Без ключа ответ будет заглушкой с подсказкой.

**Telegram:** в `.env` задайте `TELEGRAM_BOT_TOKEN`, `OPENAI_API_KEY`, затем `seo-os-telegram-bot`. Проверка **только Telegram** (без OpenAI): команда боту `/ping`. Сводка: `/status`. Писать лучше в личку; в группах учитывайте Group Privacy в BotFather.

---

## Как протестировать Trello-сценарий

1. Создайте на доске списки, соответствующие `.env.example`, и пропишите **id списков** (через URL списка или API).
2. Заполните `TRELLO_API_KEY` и `TRELLO_TOKEN` в `.env`.
3. `pip install -e .`, `alembic upgrade head` (включая `external_task_links_002`).
4. `temporal server start-dev`, затем `seo-os-worker`, затем `seo-os-demo`.
5. На доске [seo-agent](https://trello.com/b/qPDyVJen/seo-agent) должна появиться **одна карточка**, пройдя путь по колонкам согласно графу (Keyword Research → Content Draft → QA → Done) и с комментариями handoff.

Проверка в БД (из корня репозитория, с загруженным `.env`):

```bash
set -a && source .env && set +a
seo-os-db-audit
seo-os-db-audit --limit 40
```

Или вручную в `psql`:

```sql
SELECT * FROM external_task_links ORDER BY created_at DESC LIMIT 5;
SELECT * FROM agent_decisions ORDER BY created_at DESC LIMIT 30;
```

---

## Запуск кампании из Telegram (`/campaign`)

Нужны **три процесса**: `temporal server start-dev`, `seo-os-worker`, `seo-os-telegram-bot`. В `.env`: `DATABASE_URL`, все `TRELLO_*`, `TELEGRAM_BOT_TOKEN`.

1. Узнайте свой **числовой user id** в Telegram (например бот [@userinfobot](https://t.me/userinfobot) или аналог).
2. В `.env` задайте **`TELEGRAM_ALLOWED_USER_IDS=<ваш_id>`** (через запятую, если несколько). Без этого команда **`/campaign` отключена** (безопасность).
3. Перезапустите `seo-os-telegram-bot` после правки `.env`.
4. В **личке** с ботом отправьте **`/campaign`**. В ответ — `campaign_id`, `workflow_id`; на доске Trello появится новая карточка, как после `seo-os-demo`. В тот же чат придут сообщения **по фазам** (keyword / content / qa / done), если worker видит `TELEGRAM_BOT_TOKEN`.
5. Напишите боту обычный текст — ответ LLM будет с **контекстом последней кампании** (пока не выполните **`/context_off`**). **`/context`** — напомнить текущие id.

Если worker или Temporal не запущены, старт workflow завершится ошибкой на стороне клиента при следующем шаге — смотрите логи worker и Temporal UI (http://localhost:8233).

---

## Как протестировать без Trello (ожидаемая ошибка)

Если не заданы `TRELLO_*`, workflow завершится с сообщением вроде «Set TRELLO_API_KEY and TRELLO_TOKEN» / «Missing env TRELLO_LIST_…». Это нормально: сначала настройте `.env`.

---

## Краткий чеклист

- [ ] `alembic upgrade head` включает обе ревизии (`wave1_initial`, `external_task_links_002`).
- [ ] `.env` скопирован из `.env.example` и заполнен.
- [ ] `seo-os-worker` и `seo-os-demo` отрабатывают при запущенном Temporal.
- [ ] На Trello одна карточка на запуск demo, цепочка комментариев и перемещений видна.
- [ ] В `external_task_links` есть строка с `pipeline_id` и `external_id` карточки.

---

## Связь с планом

Архитектурный план: [`.cursor/plans/seo_os_langgraph_temporal_7ef168d3.plan.md`](../.cursor/plans/seo_os_langgraph_temporal_7ef168d3.plan.md).

Следующие шаги: сигналы approve/reject, LLM-узлы в графах Temporal, остальные интеграции.
