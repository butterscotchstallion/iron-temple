GO ?= go

.PHONY: tidy vet lint test build migrate run sqlc

## tidy: resolve and pin dependencies (needs GOPROXY reachable)
tidy:
	$(GO) mod tidy

## vet: static checks
vet:
	$(GO) vet ./...

## lint: golangci-lint (must be on PATH)
lint:
	golangci-lint run

## test: run all tests (integration tests need a Docker-compatible daemon)
test:
	$(GO) test ./...

## build: compile everything
build:
	$(GO) build ./...

## migrate: apply migrations to $DATABASE_URL
migrate:
	$(GO) run ./cmd/migrate

## run: apply migrations on startup and serve the API (needs DATABASE_URL)
run:
	$(GO) run ./cmd/server

## sqlc: regenerate the typed data layer (needs sqlc on PATH; Phase 3)
sqlc:
	sqlc generate
