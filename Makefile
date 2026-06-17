# Prod deploy helpers — run from the repo root on the prod host.
# COMPOSE always layers in compose.prod.yaml so the external DB volume is never
# missed; forgetting it mounts an empty volume and the DB looks wiped.
COMPOSE := docker compose -f devops/compose.yaml -f devops/compose.prod.yaml --env-file backend/.env

.PHONY: deploy rebuild-backend rebuild-frontend ps logs logs-frontend restart down

deploy:
	$(COMPOSE) up --build -d
	$(COMPOSE) logs -f app

rebuild-backend:
	$(COMPOSE) up --build -d app
	$(COMPOSE) logs -f app

rebuild-frontend:
	$(COMPOSE) up --build -d frontend
	$(COMPOSE) logs -f frontend

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f app

logs-frontend:
	$(COMPOSE) logs -f frontend

restart:
	$(COMPOSE) restart

down:
	$(COMPOSE) down
