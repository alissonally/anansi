# Anansi — common tasks. Run `make help`.
WAILS ?= $(shell command -v wails 2>/dev/null || echo $(HOME)/go/bin/wails)
TAGS  := webkit2_41

.PHONY: help deps dev build package test test-go test-race test-integration test-frontend vet typecheck lint check it-up it-down

help:
	@grep -E '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

deps: ## Install Go modules, npm packages and the Wails CLI
	go mod download
	cd frontend && npm ci
	command -v wails >/dev/null || go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

dev: ## Run in development mode with hot reload
	$(WAILS) dev -tags $(TAGS)

build: ## Build the desktop binary (build/bin/anansi)
	$(WAILS) build -clean -trimpath -tags $(TAGS) -ldflags "-s -w"

package: ## Build tar.gz and RPM packages into dist/
	./scripts/package-linux.sh

test: test-go test-frontend ## Unit tests (Go + frontend)

test-go: ## Go unit tests (integration tests skip without servers)
	go test ./...

test-race: ## Go tests with the race detector
	go test -race ./...

test-integration: ## Integration tests against disposable MySQL/MariaDB containers
	./scripts/integration-db.sh up
	eval "$$(./scripts/integration-db.sh env)" && go test -race -count=1 -v ./internal/integration/

it-up: ## Start MySQL/MariaDB test containers
	./scripts/integration-db.sh up

it-down: ## Remove MySQL/MariaDB test containers
	./scripts/integration-db.sh down

test-frontend: ## Frontend unit tests (Vitest)
	cd frontend && npm test

vet: ## go vet
	go vet ./...

typecheck: ## TypeScript type check (vue-tsc)
	cd frontend && npm run typecheck

lint: ## ESLint + gofmt check
	cd frontend && npm run lint
	@test -z "$$(gofmt -l internal main.go)" || (gofmt -l internal main.go; exit 1)

check: vet lint typecheck test ## Everything that runs without servers
