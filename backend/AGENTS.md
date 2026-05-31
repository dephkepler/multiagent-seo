# AGENTS.md

DDD + hexagonal backend, modelled on the go2-zero stack: OpenAPI-first + chi +
pgxpool + raw SQL + zerolog + Sentry. Auth is lightweight JWT (HS256) + bcrypt —
**not** Firebase, and there is **no** multi-tenancy / Postgres RLS (deferred; can
be added later if the product is sold per-tenant).

## Architecture

- `api/openapi.yaml` — source of truth for the HTTP contract
- `internal/domain/<feature>/` — entities, ports (repository + service
  interfaces), pure domain logic. Features: `auth`, `user`, `articles`,
  `health`, `wordpress`
- `internal/application/<feature>/` — use-case orchestration. `articles` runs the
  SERP→brief→write→edit→humanize→publish pipeline; long work is dispatched
  through a `JobRunner` (`AsyncRunner`) so `POST /generate` returns 202 and the
  pipeline finishes in the background
- `internal/infrastructure/`
  - `http/{handlers,middleware,problem,response}/` + `router.go` — chi adapter,
    RFC 7807 problem responses, `BearerAuth` middleware
  - `db/` — pgxpool setup
  - `persistence/postgres/` — repository implementations (raw SQL)
  - `jwtauth/` — JWT issue/verify (wraps `pkg/jwt`)
  - `llm/` — LLM clients (`groq`, `claude`) behind a factory; `transport`,
    `retry`, `usage` helpers
  - `dataforseo/`, `sheets/`, `pexels/`, `checker/`, `wordpress/` — generation
    adapters; each has a mock for when its credentials are absent
- `internal/oapigen/` — GENERATED ServerInterface + DTOs (do not edit)
- `pkg/{config,logger,sentry,validate,uuids,jwt}/` — cross-cutting helpers
- `cmd/server/` — HTTP server entrypoint; `cmd/createuser/` — admin tool to seed
  a login user

Dependency direction is always inward: infrastructure → application → domain.
Repository interfaces live in `domain/<feature>/repository.go`; concrete
implementations in `infrastructure/persistence/postgres/<feature>_repository.go`.

## Config & migrations

- Config: `pkg/config` (caarlos0/env, all vars `CF_`-prefixed). Local dev values
  in `backend/.envrc` (direnv); secrets in gitignored `.envrc.local`.
- Migrations: golang-migrate CLI via `make migrate-up` / `migrate-down` /
  `migrate-create` (see `migrate.sh`) — not a custom Go command.
- Docker: `devops/compose.yaml` builds `cmd/server` as the `app` service;
  compose env in gitignored `devops/.env`. Migrations are run from the host, not
  as a compose service.

## Reference

For full conventions (OpenAPI validation-tag flow, logger `module` naming,
RFC 7807 responses, and the RLS/multi-tenancy pattern if we ever adopt it) see:

- `/Users/user/work/GO2/go2-zero-backend/AGENTS.md`
- `/Users/user/work/GO2/go2-zero-backend/CLAUDE.md`
