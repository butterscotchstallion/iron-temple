GO ?= go

# Local dev config lives in .env (gitignored — see .env.example). Load it and
# `export` so every recipe, and the live-reloader's grandchild process
# (`make dev` → air → ./tmp/server), inherits DATABASE_URL and friends. air
# forwards its own environment to the binary it builds, but only what it was
# given — so without this include `make dev`/`make run` abort with
# "DATABASE_URL is required" unless the var was already exported into the
# calling shell. Missing .env is a silent no-op: CI and the cluster inject
# these as real environment variables, not via a file.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: tidy vet lint test build migrate run dev sqlc

## tidy: resolve and pin dependencies (needs GOPROXY reachable)
tidy:
	$(GO) mod tidy

## vet: static checks
vet:
	$(GO) vet ./...

## lint: golangci-lint (must be on PATH)
lint:
	golangci-lint run

# test: all tests. The DB-backed ones need a database, and how to give them one is
# NOT duplicated here — dev/integration-test.sh is the single definition both CI and
# scripts/preflight.sh call. See ../../docs/api-integration-tests.md. With no database
# in reach they fall back to Testcontainers, and without a container runtime for that
# they self-skip, so a green `make test` alone does not mean the suite ran.
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

## dev: like run, but live-reloads — rebuild + restart on .go/.sql changes (needs DATABASE_URL)
dev:
	$(GO) run github.com/air-verse/air@latest

## sqlc: regenerate the typed data layer (needs sqlc on PATH; Phase 3)
sqlc:
	sqlc generate
