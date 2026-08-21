# Prod deploy helpers — run from the repo root on the prod host (132.243.194.137,
# /opt/multiagent-seo — see doc/abalisbotlead/vps-deploy.md, the only verified
# real runbook; there is no separate "8cell.tech" host, that was a dead lead).
# Run `make` (or `make help`) to list every command.
#
# devops/compose.prod.yaml exists in the repo (pins postgres_data to an
# external `contentflow_postgres_data` volume) but the documented real deploy
# has NEVER used it — the VPS was set up from a fresh `git clone` straight
# into plain compose.yaml, no legacy volume to migrate. NOT included here on
# purpose: layering it in would make compose demand an external volume that
# doesn't exist on the real host and fail the deploy. Confirm with the user
# before ever adding it back.
COMPOSE := docker compose -f devops/compose.yaml --env-file backend/.env

# Mutating targets (dev / deploy / rebuild / restart / down) are lock-protected
# so two concurrent runs (e.g. two agents, or a stray background one) fail
# fast instead of fighting over ports/containers and looking hung — see
# scripts/with-lock.sh. Read-only targets (ps/logs/help) stay unlocked, they
# must always be checkable even while a deploy is in progress.
LOCK := scripts/with-lock.sh

.DEFAULT_GOAL := help
.PHONY: help dev dev-clientapp deploy rebuild-backend rebuild-frontend ps logs logs-frontend restart down

help: ## list these commands
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  make %-18s %s\n", $$1, $$2}'

dev: ## local dev: backend (:8080) + admin frontend (:3000) together, Ctrl-C stops both
	LOCK_NAME=local-dev $(LOCK) npx --yes concurrently --kill-others --names "back,front" --prefix-colors "cyan,magenta" \
		"cd backend && make dev" \
		"cd frontend && npm run dev"

# The client Mini App is separate from `make dev` on purpose: it only works
# against a backend started with CF_TELEGRAM_DEV_USER_ID, since Telegram will
# not sign a launch for a plain browser. Set that in backend/.env first, and
# point CF_APP_CORS_ALLOWED_ORIGINS at :3001 — the app calls the API
# cross-origin in dev, same-origin only in production behind the proxy.
dev-clientapp: ## local dev: the client Mini App (:3001) against an already-running backend
	cd clientapp && NEXT_PUBLIC_API_BASE=http://localhost:8080 NEXT_PUBLIC_DEV_INIT_DATA=dev-launch npm run dev

deploy: ## rebuild backend+frontend, restart, then follow backend logs
	LOCK_NAME=prod-deploy $(LOCK) $(COMPOSE) up --build -d
	$(COMPOSE) logs -f app

rebuild-backend: ## rebuild only the backend, then follow its logs
	LOCK_NAME=prod-deploy $(LOCK) $(COMPOSE) up --build -d app
	$(COMPOSE) logs -f app

rebuild-frontend: ## rebuild only the frontend, then follow its logs
	LOCK_NAME=prod-deploy $(LOCK) $(COMPOSE) up --build -d frontend
	$(COMPOSE) logs -f frontend

ps: ## show container status
	$(COMPOSE) ps

logs: ## follow backend logs
	$(COMPOSE) logs -f app

logs-frontend: ## follow frontend logs
	$(COMPOSE) logs -f frontend

restart: ## restart containers without rebuilding
	LOCK_NAME=prod-deploy $(LOCK) $(COMPOSE) restart

down: ## stop the whole stack
	LOCK_NAME=prod-deploy $(LOCK) $(COMPOSE) down
