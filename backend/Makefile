GOCMD = go
BINARY = bin/server

.PHONY: dev run-dev kill-stale build run clean db up down stop logs creds generate test test-integration lint format migrate-create migrate-down docker-build

-include .env
export

# Map app-config DB vars from the compose POSTGRES_* vars so migrate.sh and a
# host-run server pick them up without extra env juggling.
export CF_DB_HOST ?= localhost
export CF_DB_PORT ?= $(POSTGRES_HOST_PORT)
export CF_DB_USER ?= $(POSTGRES_USER)
export CF_DB_PASSWORD ?= $(POSTGRES_PASSWORD)
export CF_DB_NAME ?= $(POSTGRES_DB)

COMPOSE := docker compose -f ../devops/compose.yaml --env-file .env

# ── Local dev: app runs on the host (air hot-reload) against the compose Postgres ──
dev: db run-dev

run-dev: kill-stale
	$(GOCMD) tool air

# Kill a stale dev backend from a previous run so air can bind the port.
# Bracket trick ([t]mp) keeps pkill from matching its own command line.
kill-stale:
	@pkill -f '[t]mp/main' 2>/dev/null; pkill -f '[b]in/server' 2>/dev/null; true

build:
	$(GOCMD) build -o $(BINARY) ./cmd/server

run: build
	./$(BINARY)

clean:
	rm -rf bin tmp

# One-off: log the personal Telegram account into MTProto and save the
# session (see cmd/tgsession). Run by hand, once.
tgsession:
	$(GOCMD) run ./cmd/tgsession

# Pull personal-Telegram contacts into telegram_contacts (see cmd/tgimport).
# Safe to rerun any time — upserts by Telegram user id, never duplicates.
tgimport:
	$(GOCMD) run ./cmd/tgimport

# ── Compose: Postgres for dev, full stack (app+frontend) for prod ──
db:
	$(COMPOSE) up -d postgres
	@echo "waiting for postgres..." && sleep 2

up: creds
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down

stop:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f app

# Empty credentials.json placeholder so compose can always bind-mount it
# without silently creating a host-side directory.
creds:
	@test -e credentials.json || (echo "creating empty credentials.json placeholder"; echo "{}" > credentials.json)

# ── Codegen / test / lint ──
generate:
	$(GOCMD) tool oapi-codegen -config api/oapi-codegen.yaml -o internal/oapigen/api.gen.go api/openapi.yaml

test:
	$(GOCMD) test ./...

test-integration:
	$(GOCMD) test -tags=integration ./...

lint:
	@test -z "$$(gofmt -s -l .)" || { gofmt -s -l .; echo "run: gofmt -s -w ."; exit 1; }
	$(GOCMD) vet ./...
	$(GOCMD) run github.com/gordonklaus/ineffassign@latest ./...
	$(GOCMD) run github.com/kisielk/errcheck@latest -ignore 'Close$$' ./...

format:
	gofmt -s -w .

# ── Migrations: the app auto-applies them on startup; these are for manual ops ──
migrate-create:
	@test -n "$(name)" || { echo "usage: make migrate-create name=add_xxx"; exit 1; }
	migrate create -ext sql -dir migrations $(name)

migrate-down:
	./migrate.sh down 1

# ── Docker ──
docker-build:
	docker build -f ../devops/docker/backend.Dockerfile -t multiagent-seo-backend:latest .
