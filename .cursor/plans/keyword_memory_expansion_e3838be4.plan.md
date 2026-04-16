---
name: Keyword Memory Expansion
overview: Ввести постоянное хранилище keyword/cluster памяти с инкрементальным дневным ростом и анти-повтором между циклами, чтобы LLM/поиск системно расширяли охват вширь и вглубь.
todos:
  - id: schema-keyword-memory
    content: Спроектировать и добавить SQL-миграции для keyword_memory, cluster_memory, keyword_cluster_links, keyword_run_snapshots
    status: pending
  - id: repos-upsert-layer
    content: Реализовать в db/repos.py upsert/read методы для памяти ключей/кластеров и получения frontier-кандидатов
    status: pending
  - id: integrate-memory-write
    content: Интегрировать запись памяти после keyword_search и привязать snapshot итерации
    status: pending
  - id: refine-frontier-loop
    content: Обновить refine-цикл в campaign, чтобы использовать frontier и снижать повторяемость между днями
    status: pending
  - id: analytics-and-dashboard
    content: Добавить day-by-day summary в done и локальный JSON/HTML дашборд с source effectiveness и cluster->keywords
    status: pending
  - id: vector-memory-phase
    content: Добавить vector memory как secondary слой (semantic retrieval для LLM refine) поверх SQL памяти
    status: pending
  - id: breadth-first-expansion
    content: Добавить explicit breadth-first контур для быстрого роста соседних тем с KPI и forced-pivot
    status: pending
  - id: multi-tier-frontier
    content: Внедрить multi-tier frontier (F1/F2/F3) и не допускать схлопывания в novelty-only
    status: pending
  - id: saturation-and-decay
    content: Добавить saturation model кластеров и temporal decay для памяти ключей
    status: pending
  - id: diversity-and-modes
    content: Ввести enforced diversity для LLM seed generation и явные режимы exploration/exploitation
    status: pending
  - id: active-vector-ops
    content: Реализовать активные vector-операции (semantic gaps, auto-merge clusters) и self-tuning KPI loop
    status: pending
  - id: value-layer-and-metrics
    content: Добавить Keyword Value Layer и расширенные dashboard метрики (novelty decay, hallucination rate и др.)
    status: pending
  - id: seeds-studio-chat-copilot
    content: Добавить в вертикальный UI чат-копилот для генерации/валидации seed-пакетов по заданным подтемам (providers/platforms/brands)
    status: pending
isProject: false
---

# План: Накопительная память ключей и кластеров

## Цель

Сделать устойчивый контур, где каждый новый запуск добавляет только полезно новые ключи/кластеры, переиспользует исторический контекст и не теряет накопленную структуру.

## Текущее состояние и ограничение

- Сейчас анти-повтор реализован через файл `KEYWORD_SEEN_FILE` в `split_novelty()` из [/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/keywords/quality.py](/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/keywords/quality.py).
- Это помогает в рамках одного seen-файла, но не дает удобной аналитики «день за днем» и слабее для межкампанийной памяти.

## Целевая модель хранения

- Добавить БД-память (Postgres) с 3 сущностями:
  - `keyword_memory`: нормализованный ключ, первый/последний seen, источники, счетчики, score, last_pipeline_id.
  - `cluster_memory`: cluster_id/label, first/last seen, coverage_count, status, last_pipeline_id.
  - `keyword_cluster_links`: связь many-to-many ключей и кластеров с историей обновлений.
- Добавить `keyword_run_snapshots` (JSONB): агрегат итерации (источники, новые/повторные, cluster->keywords sample) для отчетности и графиков.

## Гибридная память (добавление vector слоя)

- Основной принцип:
  - `SQL memory` — primary и deterministic (анти-повтор, счетчики, история).
  - `Vector memory` — secondary и semantic (похожие темы/кластеры для расширения).
- Векторный слой не заменяет дедуп в SQL; он улучшает выбор направлений для `keyword_llm_refine`.
- Порядок внедрения:
  1. Сначала стабилизировать SQL memory + frontier.
  2. Затем добавить embeddings для ключей/кластеров.
  3. Перед refine делать retrieval: похожие кластеры + undercovered соседи.
  4. Класть retrieval-контекст в prompt LLM вместе с exact seen из SQL.

## Поток обновления на каждой итерации

1. После `keyword_search` (в `keyword_batch`/`campaign`) сохранить snapshot итерации.
2. Для каждого ключа:
  - normalize + upsert в `keyword_memory`.
  - если ключ уже был, инкрементировать resurfaced_count; если новый — new_count и first_seen.
3. Для кластеров:
  - upsert в `cluster_memory` без удаления (только обновление метрик и last_seen).
4. Обновить связи `keyword_cluster_links`.
5. Сформировать candidate pool для следующего refine: исключить «пережеванные» ключи и дать LLM только новые frontier-направления.

## Анти-повтор и рост day-by-day

- Дедуп ключа: по `normalize_keyword()` + уникальный индекс.
- Дедуп кластера: стабильный `cluster_id` + merge aliases.
- Frontier-логика:
  - приоритет ключам/кластерам с low_seen_count и recent_growth.
  - понижение веса для resurfaced без прироста.
- Политика стагнации:
  - если 2–3 итерации нет прироста frontier-кластеров, включать forced pivot (новые интенты/подниши/geo).

## Ускорение роста вширь по соседним темам (breadth-first)

- Режим итераций:
  - ранние итерации работают в `breadth-first` и максимизируют новые кластеры;
  - в depth-фазу переходим только после достижения порога по кластерам.
- Frontier expansion:
  - из каждого найденного кластера строим соседние подтемы (intent/modifier/problem-solution/geo).
  - LLM обязан добавлять фиксированное число новых seeds из frontier на каждую итерацию.
- Source mix для breadth:
  - использовать multi-source (`autocomplete + trends + youtube`) на ранних циклах;
  - если один источник стагнирует, не снижать общий breadth — усиливать другие источники.
- Анти-повтор на входе:
  - перед source-вызовами исключать seeds с высоким resurfaced без прироста;
  - приоритизировать low_seen / undercovered кластеры.
- Forced pivot:
  - если 2–3 итерации нет прироста новых кластеров, запускать pivot-политику:
    - новые интенты,
    - новые смежные подниши,
    - альтернативные формулировки и контексты.
- Breadth KPI (гейт):
  - `added_clusters_per_iter >= X`,
  - `new_keywords_ratio >= Y`,
  - `repeated_seed_ratio <= Z`.
  - Если гейт не выполнен подряд N раз — автоматический pivot.
- Stop-policy:
  - не завершать рано, пока `target_clusters` не достигнут хотя бы на 60-70%;
  - стагнацию считать только при одновременном провале по кластерам и новым seeds.

## Отчетность и визуализация

- В `done` добавить блоки:
  - daily delta: новые ключи/кластеры за день,
  - cumulative totals,
  - top sources by new clusters,
  - top expanding clusters.
- Добавить exporter `runtime/keyword_runs/<pipeline_id>.json` для локального графика.
- Добавить простой HTML dashboard (Chart.js):
  - рост ключей/кластеров по дням,
  - эффективность источников,
  - таблица cluster -> keywords.
- После vector-фазы добавить блок:
  - semantic neighbors used (какие близкие темы помогли расширению),
  - merge aliases rate (сколько дублей кластеров объединили семантически).

## Файлы, которые меняем

- [/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/keywords/quality.py](/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/keywords/quality.py) — оставить normalize/split_novelty как fallback, добавить вызов memory-layer.
- [/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/temporal/activities_keyword_search.py](/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/temporal/activities_keyword_search.py) — после сбора формировать payload для upsert memory.
- [/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/temporal/workflows/keyword_batch.py](/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/temporal/workflows/keyword_batch.py) — прокинуть summary memory в отчеты.
- [/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/temporal/workflows/campaign.py](/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/temporal/workflows/campaign.py) — использовать memory frontier в refine-цикле.
- [/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/db/repos.py](/Users/mac/ahrefs api LangGraph Temporal/src/seo_os/db/repos.py) — добавить репозитории upsert/read для keyword/cluster memory.
- Новые SQL миграции для таблиц памяти и индексов.
- Новые SQL миграции под vector-индексацию (например pgvector) и поля embeddings.

## Критерии готовности

- Повторный запуск на следующий день увеличивает cumulative keyword/cluster counts без сброса.
- Доля truly new не падает к нулю за счет frontier-пула и pivot-логики.
- В `done` есть прозрачные метрики day-by-day и source effectiveness.
- Есть графический dashboard `cluster -> keywords` и тренды роста.
- После включения vector-слоя растет доля truly new без роста повторов, а merge алиасов кластеров становится стабильным.
- На ранних итерациях видно ускоренный прирост соседних кластеров (breadth KPI выполняется стабильно).

## Критические апгрейды устойчивости (production-hardening)

### 1) Multi-tier frontier (чтобы frontier не схлопывался)

- Ввести 3 очереди frontier:
  - `F1`: truly new clusters (priority 1),
  - `F2`: underdeveloped clusters (priority 2),
  - `F3`: resurfacing with new angle (priority 3).
- Не отключать `F2/F3`; распределять budget итерации между всеми tiers.
- В отчетах показывать `frontier_composition` по долям F1/F2/F3.

### 2) Cluster saturation control

- Расширить `cluster_memory`:
  - `keyword_count`,
  - `expected_size`,
  - `saturation_score = keyword_count / expected_size`.
- Политика приоритета:
  - `saturation_score > 1.2` -> снижать приоритет кластера,
  - `saturation_score < 0.5` -> усиливать expansion.

### 3) Enforced diversity для LLM

- В prompt refine/bootstrap добавить оси diversity:
  - `INTENT`: informational / transactional / problem / comparison,
  - `FORMAT`: question / statement / long-tail,
  - `ANGLE`: beginner / advanced / niche,
  - `ENTITY`: tool / brand / geo / persona.
- Ввести constraints генерации:
  - min 2 seeds на intent-группу,
  - min 3 разных format,
  - min 2 новых angle на итерацию.

### 4) Активный vector layer (не только retrieval в prompt)

- Semantic gaps detection:
  - искать ближайшие кластеры;
  - если похожи семантически, но без общих ключей -> генерировать bridge keywords.
- Cluster auto-merge:
  - при cosine similarity выше порога (например 0.92) формировать merge-кандидаты,
  - фиксировать merge decisions и alias-map в памяти.

### 5) Temporal decay / aging

- Добавить freshness-фактор ключей:
  - `freshness_score = exp(-lambda * days_since_last_seen)`.
- Приоритет frontier считать как композицию novelty + freshness, чтобы память не засорялась вечными хвостами.

### 6) Self-tuning KPI loop

- Если KPI провален подряд:
  - поднять source diversity,
  - поднять pivot aggressiveness,
  - временно увеличить creativity генерации.
- Если KPI стабильно сильный:
  - снижать randomness,
  - усиливать exploitation в глубину.

### 7) Явные режимы Exploration vs Exploitation

- `Exploration mode`:
  - max breadth, low filtering, aggressive pivot.
- `Exploitation mode`:
  - cluster fill, semantic completion, stricter novelty thresholds.
- Переключение:
  - пока `total_clusters < target_clusters * 0.6` -> exploration,
  - иначе -> exploitation.

### 8) Keyword Value Layer (кроме количества)

- Расширить `keyword_memory` полями:
  - `value_score`,
  - `difficulty_score`,
  - `intent_type`.
- Приоритизация:
  - `frontier_priority = novelty * value_score * freshness`.

### 9) Дополнительные метрики dashboard

- `novelty_decay_curve`,
- `cluster_saturation_distribution`,
- `frontier_composition (F1/F2/F3)`,
- `llm_hallucination_rate`,
- `best_source_for_new_clusters` и `yield_per_iteration`.

## UI этап: Seeds Studio + Chat Copilot (текущая вертикаль gambling)

Цель: в текущем окне вертикали дать управляемый цикл:

- общение с чатом о seed-идеях,
- ручное редактирование seed-листов,
- запуск поиска с live-результатами,
- продолжение в breadth/depth без переключения контекста.

### 1) Seeds Studio (главный шаг)

- Блок `LLM предложил seeds` (editable list).
- Блок `Мои seeds` (ручной ввод, bulk paste, quick tags).
- Кнопки:
  - `Regenerate seeds`,
  - `Validate & dedupe`,
  - `Use mixed (LLM + mine)`.
- Дополнительно: quick presets для gambling:
  - `Game providers`,
  - `Casino platforms`,
  - `Casino brands`.

### 2) Chat Copilot внутри этого же окна

- Правая панель чата в том же UI (не отдельная страница).
- Примеры промптов:
  - `Предложи seeds по game providers`,
  - `Дай seeds по casino platforms`,
  - `Сгенерируй seeds по casino brands US`.
- Ответ чата структурировать в JSON-блок:
  - `seed_suggestions[]`,
  - `reasoning_short`,
  - `intent_mix`.
- Кнопки действий на ответ:
  - `Add to LLM seeds`,
  - `Add to My seeds`,
  - `Replace LLM seeds`.

### 3) Проверка перед запуском

- Предупреждения:
  - дубли,
  - слабые/слишком общие seeds,
  - узкие seeds без соседних веток.
- Прогноз (грубый):
  - expected cluster breadth,
  - expected keyword yield,
  - risk of repetition.
- Финальный выбор стратегии:
  - `Run Breadth`,
  - `Run Depth`,
  - `Run Mixed`.

### 4) Run + Live Results

- Статус итераций в реальном времени.
- Новые кластеры (рост вширь) + ключи внутри выбранного кластера.
- KPI-виджеты:
  - added clusters / added keywords,
  - novelty ratio,
  - source effectiveness.
- Кнопки:
  - `Continue Breadth`,
  - `Continue Depth`.

### 5) API контракт для UI (минимум)

- `POST /v1/verticals/{vertical}/seeds/chat-suggest`
  - вход: `message`, `context`, `current_seeds`.
  - выход: `seed_suggestions`, `warnings`, `intent_mix`.
- `POST /v1/verticals/{vertical}/seeds/validate`
  - вход: `llm_seeds`, `my_seeds`.
  - выход: `deduped`, `weak`, `duplicates`, `breadth_estimate`.
- `POST /v1/verticals/{vertical}/run`
  - вход: `mode` (`breadth|depth|mixed`), `seeds`.
  - выход: `campaign_id`, `workflow_id`.

### 6) Ограничение текущего этапа

- Делаем только для `gambling` в существующем окне вертикали.
- Мульти-вертикали добавляем позже, после стабилизации UX и метрик.

