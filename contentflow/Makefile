.PHONY: dev db migrate run stop up down logs creds

-include .env
export

CF_DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_HOST_PORT)/$(POSTGRES_DB)?sslmode=disable
export CF_DATABASE_URL

dev: db migrate run

# creates an empty credentials.json placeholder so docker compose can always
# bind-mount it without silently creating a host-side directory.
creds:
	@test -e credentials.json || (echo "creating empty credentials.json placeholder"; echo "{}" > credentials.json)

db:
	docker compose up -d postgres
	@echo "waiting for postgres..." && sleep 2

migrate:
	go run ./cmd/migrate

run:
	go run ./cmd/main.go

stop:
	docker compose down

up: creds
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f app
