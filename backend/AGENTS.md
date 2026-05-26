# AGENTS.md

This backend is mid-migration to the go2-zero stack: DDD + hexagonal
architecture + OpenAPI-first + chi + pgxpool + zerolog + Sentry +
Firebase auth + Postgres RLS per company.

## New architecture (target)

- `api/openapi.yaml` — source of truth for HTTP contract
- `internal/domain/<feature>/` — entities, repository ports, domain services
- `internal/application/<feature>/` — use-case orchestration
- `internal/infrastructure/`
  - `http/{handlers,middleware,problem,response}/` — chi adapter
  - `db/` — pgxpool setup
  - `persistence/postgres/` — repository implementations
  - `firebaseidentity/` — Firebase ID-token verification
- `internal/oapigen/` — GENERATED ServerInterface + DTOs (do not edit)
- `internal/testutil/mocks/` — shared test helpers
- `pkg/{config,logger,sentry,validate,uuids}/` — cross-cutting helpers

Dependency direction is always inward: infrastructure → application → domain.
Repository interfaces live in `domain/<feature>/repository.go`; concrete
implementations in `infrastructure/persistence/postgres/<feature>_repository.go`.

## Legacy code (current production)

Still wired into `cmd/main.go`:

- `internal/server/`, `internal/application/*.go` (top-level files),
  `internal/repo/`, `internal/llm/`, `internal/wordpress/`, `internal/sheets/`,
  `internal/pexels/`, `internal/dataforseo/`, `internal/checker/`,
  `internal/publisher/`, `internal/prompt/`, `internal/config/`

Each will be removed as its feature is rebuilt into the new structure.

## Reference

For full conventions (OpenAPI validation tag flow, RLS pattern via
`setup_company_rls()` + `uow.WithinTx`, logger `module="api.<feature>.<method>"`
naming, RFC 7807 problem responses) see the reference repo:

- `/Users/user/work/GO2/go2-zero-backend/AGENTS.md`
- `/Users/user/work/GO2/go2-zero-backend/CLAUDE.md`

This file will grow as each pattern gets adopted locally.
