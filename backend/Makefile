.PHONY: dev db run stop up down logs creds generate migrate-up migrate-down migrate-create lint

-include .env
export

# Map app-config DB vars from the compose POSTGRES_* vars so migrate.sh and the
# server pick them up without extra env juggling.
export CF_DB_HOST ?= localhost
export CF_DB_PORT ?= $(POSTGRES_HOST_PORT)
export CF_DB_USER ?= $(POSTGRES_USER)
export CF_DB_PASSWORD ?= $(POSTGRES_PASSWORD)
export CF_DB_NAME ?= $(POSTGRES_DB)

COMPOSE := docker compose -f ../devops/compose.yaml --env-file .env

dev: db migrate-up run

generate:
	go tool oapi-codegen -config api/oapi-codegen.yaml -o internal/oapigen/api.gen.go api/openapi.yaml

migrate-up:
	./migrate.sh up

migrate-down:
	./migrate.sh down 1

migrate-create:
	@test -n "$(name)" || { echo "usage: make migrate-create name=add_xxx"; exit 1; }
	migrate create -ext sql -dir migrations $(name)

# creates an empty credentials.json placeholder so docker compose can always
# bind-mount it without silently creating a host-side directory.
creds:
	@test -e credentials.json || (echo "creating empty credentials.json placeholder"; echo "{}" > credentials.json)

db:
	$(COMPOSE) up -d postgres
	@echo "waiting for postgres..." && sleep 2

run:
	go run ./cmd/server

stop:
	$(COMPOSE) down

up: creds
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f app

lint:
	@test -z "$$(gofmt -s -l .)" || { gofmt -s -l .; echo "run: gofmt -s -w ."; exit 1; }
	go vet ./...
	go run github.com/gordonklaus/ineffassign@latest ./...
	go run github.com/kisielk/errcheck@latest -ignore 'Close$$' ./...
