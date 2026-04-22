.PHONY: dev db migrate run stop up down logs

-include .env
export

CF_DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_HOST_PORT)/$(POSTGRES_DB)?sslmode=disable
export CF_DATABASE_URL

dev: db migrate run

db:
	docker compose up -d postgres
	@echo "waiting for postgres..." && sleep 2

migrate:
	go run ./cmd/migrate

run:
	go run ./cmd/main.go

stop:
	docker compose down

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f app
