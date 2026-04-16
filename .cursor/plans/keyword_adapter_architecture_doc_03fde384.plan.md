---
name: Keyword adapter architecture doc
overview: Закріпити узгоджену архітектуру (тонкий FastAPI-адаптер між SEO OS і keyword pipeline, режими cached/live/paid, виклик з Temporal activity, антипатерни) у [docs/EXTERNAL_KEYWORD_SERVICE.md](docs/EXTERNAL_KEYWORD_SERVICE.md) і додати плейсхолдери в [.env.example](.env.example). Реалізація Python-коду — не входить у цей план.
todos:
  - id: doc-adapter-section
    content: Додати розділ про adapter-service, режими, activity, контракт, антипатерни + оновити mermaid у EXTERNAL_KEYWORD_SERVICE.md
    status: completed
  - id: env-example-keywords
    content: Додати закоментований блок KEYWORD_SEARCH_* в .env.example
    status: completed
  - id: status-optional-line
    content: "За потреби: один рядок посилання в STATUS_AND_TESTING.md"
    status: completed
isProject: false
---

# План: документація adapter/API-шару для keyword search

## Мета

Відобразити у репозиторії **SEO OS** узгоджену схему: **оркестратор (LangGraph / Temporal)** викликає **зовнішній HTTP keyword-service**, який є **тонким адаптером** над окремим репозиторієм з pipeline-скриптами — **без** змішування коду репозиторіїв, **без** `subprocess` з вузла графа, **без** імпорту другого проєкту як пакета.

## Зміни у файлах

### 1. Розширити [docs/EXTERNAL_KEYWORD_SERVICE.md](docs/EXTERNAL_KEYWORD_SERVICE.md)

Додати **новий розділ** (логічно після поточного §2 або як **§2.1 / окремий §7** — краще **вставити після §2 «Де підключати»** як детальний підрозділ **«Архітектура: adapter-service і чому не змішувати репозиторії»**, щоб читач одразу бачив рекомендований патерн перед §3 про env).

Контент скоротити відносно вашого повідомлення, але зберегти суть:

- **Три шари:** SEO OS → **HTTP** → keyword-adapter (FastAPI) → виклики/читання даних другого проєкту (`pipeline/output/` або обмежені виклики внутрішніх функцій).
- **Чому не:** імпорт другого репо, `subprocess` з графа, повна логіка discovery всередині workflow.
- **Перевага окремої Temporal activity** для HTTP виклику (retry, timeout, моніторинг 429/5xx) порівняно з «важким» вузлом лише в LangGraph — узгодити з уже наявним пунктом у §2.
- **Два режими адаптера:** (1) **cached** — читання останніх `pipeline/output/*.json` / нормалізація; (2) **cheap_live** — on-demand дешеві джерела; (3) **validate_paid** — Ahrefs лише опційно / за прапором (квоти, не дефолт на кожен запит).
- **Етапи впровадження:** MVP = cached; потім cheap live; потім paid validation — одним підпунктом.
- **Розширення контракту `POST /v1/search`:** поля `sources` (масив), `mode` (`cached` | `cheap_live` | `validate_paid`) — узгодити з прикладом у §4.2 (додати другий приклад JSON або таблицю «опційні поля»).
- **Внутрішня нормалізація** в адаптері: коротко описати єдину схему (на кшталт `KeywordHit`: `keyword`, `source`, опційно `volume`, `kd`, `intent`…) — без обов’язкового коду в репо SEO OS.
- **Опційно:** `POST /v1/search/batch` — як майбутнє розширення, не обов’язковий для MVP.
- **Антипатерни:** короткий список (як у вашому тексті).
- **Посилання:** `docs/CURSOR_AI_COLLABORATION.md` у другому проєкті — про процес у Cursor, **не** runtime-інтеграція (вже згадано; повторити одним реченням у новому блоці за потреби).

**Діаграма:** додати невеликий **mermaid** `flowchart`: `SeoCampaignWorkflow` → `keyword_search_activity` → `KeywordAdapter` → `PipelinesOrArtifacts`.

**Оновити** існуючу діаграму в §1 (рядки 45–68): замінити вузол `KeywordSearch HTTP` на `**KeywordAdapter`** або додати підпис, що це саме **HTTP API адаптера**, а не сирі скрипти.

### 2. [.env.example](.env.example)

Додати **закоментований блок** для майбутньої інтеграції:

- `KEYWORD_SEARCH_BASE_URL`
- `KEYWORD_SEARCH_API_KEY`
- `KEYWORD_SEARCH_TIMEOUT_SEC`

З коротким коментарем: «виклик keyword-adapter; див. `docs/EXTERNAL_KEYWORD_SERVICE.md`».

### 3. За бажанням (мінімально)

Один рядок у [docs/STATUS_AND_TESTING.md](docs/STATUS_AND_TESTING.md) у блоці про зовнішній сервіс — що детальна архітектура adapter/режимів описана в `EXTERNAL_KEYWORD_SERVICE.md` (якщо ще не дублює існуючий пункт).

## Що не входить у план

- Реалізація `KeywordSearchClient`, нової activity `keyword_search`, змін у `[keyword_trello.py](src/seo_os/graphs/keyword_trello.py)` або FastAPI-сервісу в іншому репозиторії.
- Редагування файлу плану `.cursor/plans/...`.

