# multiagent-seo

Go backend (hexagonal: domain → application → infrastructure) + Next.js frontend. Деплой — SSH на
удалённый Docker-хост, без CI/CD.

Один прод (132.243.194.137, `/opt/multiagent-seo`) на весь этот бэкенд — SEO/линкбилдинг и
Abalis-бот не два продукта, а две фичи одного backend'а, деплоятся вместе. Runbook —
`doc/abalisbotlead/vps-deploy.md` (имя историческое, по факту это общий прод-раннбук).
`doc/deploy/deploy.md` — заглушка про несуществующий хост (8cell.tech), не источник правды.

## Слои backend (`backend/internal/`)
- `domain/` — модели + порты (интерфейсы); не знает про Postgres/Groq/WordPress
- `application/` — use-case сервисы, оркестрируют домен через порты
- `infrastructure/` — конкретные адаптеры (db, llm, wordpress, sheets, telegram, ...)
- сборка/DI — `backend/internal/root/` (`main.go` теперь тонкий, 31 строка)

Полная карта слоёв, DI, поток данных — `doc/architecture/architecture.md`. Как читать/строить
фичу по этим слоям — `doc/standards/feature-architecture-guide.md`.

## Команды
- `make dev` (корень) — backend :8080 + frontend :3000 вместе
- `cd backend && make dev` — только backend (air hot-reload)
- `cd backend && make test` — юнит-тесты
- `cd backend && make test-integration` — интеграционные (поднимают Postgres через testcontainers,
  нужен Docker)
- `cd backend && make lint` — `check-migrations.sh` + `gofmt -s -l` + `go vet` + `ineffassign` +
  `errcheck`
- `cd frontend && npm run lint` / `npm run typecheck`
- прод-деплой — с прод-хоста, см. `doc/abalisbotlead/vps-deploy.md` (`dc up --build -d`);
  корневой `make deploy` тоже есть, но его соответствие реальному runbook'у (в частности флаг
  `-f devops/compose.prod.yaml`) не подтверждено — см. пометку в `doc/deploy/deploy.md`, не
  использовать не проверив.

## Код-стандарты
@.claude/rules/naming.md
@.claude/rules/error-handling-logging.md
@.claude/rules/solid-dry-kiss.md

Это дистилляты. Полные версии — с примерами «было→стало» и объяснением «почему больно» на
конкретных багах — в `doc/standards/`.

## Документация
`doc/` — architecture, audit (находки + приоритеты запуска), deploy, standards, история фич.
**Gitignored осознанно** (`.gitignore:18`) — рабочие/аудит-доки не коммитим, но они есть в
файловой системе, читай их напрямую. Индекс — `doc/README.md`.
