# Contentflow — флоу

Сервис берёт ключ ("crown play casino"), пишет SEO-статью, заливает черновик в WordPress.

---

## Главный флоу

```
POST /generate { keyword, site, auto_publish }
       │
       ▼
  [синхронно ~1 сек]
       │
       ├─ берём кластер ключей из Google Sheets       (нет → 404)
       ├─ резолвим WordPress-сайт по алиасу           (нет → 400)
       ├─ резолвим LLM-клиента                        (claude / groq)
       ├─ INSERT articles (status=generating)
       │
       ▼
  ◄── 202 { article_id, status_url }                  клиент уходит
       │
       ▼
  [фоновая горутина, до 15 минут]
       │
       │   step 1/5  DataForSEO    →  топ-10 SERP + PAA
       │   step 2/5  LLM brief     →  план статьи (H2/H3)
       │   step 3/5  LLM writer    →  полная статья (Markdown)
       │   step 4/5  LLM editor    →  SEO-причёска
       │   step 5/5  цикл проверки ▼
       │
       │      ┌────────────────────────────────────────┐
       │      │  HuggingFace.Check → ai_score          │
       │      │                                        │
       │      │  ai_score < 0.8  ─►  PASS, выходим     │
       │      │  ai_score ≥ 0.8  ─►  LLM humanize      │
       │      │                      и снова в Check   │
       │      │                                        │
       │      │  до 3 итераций; не дотянули → пишем    │
       │      │  как есть                              │
       │      └────────────────────────────────────────┘
       │
       │   Markdown → HTML (+Pexels картинки)
       │   WordPress: создать черновик
       │   UPDATE articles SET status='draft', wp_post_id
       │
       ▼
   auto_publish?
       │
       ├─ нет → конец. Ждём ручного POST /articles/{id}/publish
       │
       └─ да  → WordPress: status=publish
                UPDATE articles SET status='published', wp_post_url
```

---

## Статусы

```
generating  ─►  draft  ─►  published
     │
     └────►  failed   (финал, обратно нет)
```

---

## Ручная публикация черновика

```
POST /articles/{id}/publish
       │
       ▼
  берём wp_post_id из БД
  дёргаем WordPress publish
  UPDATE articles SET status='published'
```

Сайт берётся из колонки `articles.site` — публикация идёт туда же,
где лежит черновик, даже спустя неделю.

---

## Внешние сервисы

```
Google Sheets   →  список target keywords для темы          ОБЯЗАТЕЛЕН
LLM (Claude/Groq)  →  brief / writer / editor / humanize    ОБЯЗАТЕЛЕН
PostgreSQL      →  состояние статьи                          ОБЯЗАТЕЛЕН
WordPress       →  публикация                                ОБЯЗАТЕЛЕН

DataForSEO      →  SERP конкурентов                  опционально (→ mock)
HuggingFace     →  AI-детектор                       опционально (→ всегда pass)
Pexels          →  фото для [IMG|...] плейсхолдеров  опционально (→ выкидываем)
```

---

## Multi-site WordPress

```yaml
# config.yaml
wordpress:
  default:                  # обязательный алиас
    url: "https://site-a.com"
    user: "admin"
    appPassword: "xxxx xxxx xxxx xxxx"
  playpulse:                # любой свой
    url: "https://playpulse.tech"
    user: "admin"
    appPassword: "xxxx xxxx xxxx xxxx"
```

`POST /generate { site: "playpulse" }` → черновик уйдёт туда.
Пусто → `default`. Неизвестный алиас → `400 unknown_site`.

---

## Shutdown

```
SIGTERM
  │
  ├─ HTTP сервер закрывается (30 сек)
  ├─ bgCancel() — фоновые горутины получают cancel
  ├─ wg.Wait() — ждём дренажа
  └─ закрываем PG pool
```
