# Prod deploy helpers — run from the repo root on the prod host.
# Run `make` (or `make help`) to list every command.
# COMPOSE always layers in compose.prod.yaml so the external DB volume is never
# missed; forgetting it mounts an empty volume and the DB looks wiped.
COMPOSE := docker compose -f devops/compose.yaml -f devops/compose.prod.yaml --env-file backend/.env

.DEFAULT_GOAL := help
.PHONY: help dev deploy rebuild-backend rebuild-frontend ps logs logs-frontend restart down

help: ## list these commands
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  make %-18s %s\n", $$1, $$2}'

dev: ## local dev: backend (:8080) + frontend (:3000) together, Ctrl-C stops both
	npx --yes concurrently --kill-others --names "back,front" --prefix-colors "cyan,magenta" \
		"cd backend && make dev" \
		"cd frontend && npm run dev"

deploy: ## rebuild backend+frontend, restart, then follow backend logs
	$(COMPOSE) up --build -d
	$(COMPOSE) logs -f app

rebuild-backend: ## rebuild only the backend, then follow its logs
	$(COMPOSE) up --build -d app
	$(COMPOSE) logs -f app

rebuild-frontend: ## rebuild only the frontend, then follow its logs
	$(COMPOSE) up --build -d frontend
	$(COMPOSE) logs -f frontend

ps: ## show container status
	$(COMPOSE) ps

logs: ## follow backend logs
	$(COMPOSE) logs -f app

logs-frontend: ## follow frontend logs
	$(COMPOSE) logs -f frontend

restart: ## restart containers without rebuilding
	$(COMPOSE) restart

down: ## stop the whole stack
	$(COMPOSE) down
