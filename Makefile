# Local settings (database URLs etc.) for make run / migrate / seed.
-include .env
export

# Keep these in step with .github/workflows/ci.yml.
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.9.0

GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK   := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# The Bruno CLI needs Node 22+, so it runs in a throwaway container.
BRUNO_CLI_VERSION := 4.3.0
BRUNO := docker run --rm --network host -v "$(CURDIR)/bruno:/collection" -w /collection \
	node:22-alpine npx --yes @usebruno/cli@$(BRUNO_CLI_VERSION)

# An isolated copy of the compose stack: own project, volumes and ports.
API_TEST_PORT := 18082
COMPOSE_TEST := PONY_PORT=$(API_TEST_PORT) POSTGRES_PORT=15432 PROMETHEUS_PORT=19090 \
	docker compose -p dancing-pony-test --env-file .env.example --profile app

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build bin/pony
	go build -trimpath -o bin/pony ./cmd/pony

.PHONY: run
run: ## Run the API locally (needs make db)
	go run ./cmd/pony serve

.PHONY: test
test: ## Run tests with the race detector
	go test -race -count=1 ./...

.PHONY: cover
cover: ## Run tests and open the coverage report
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: fmt
fmt: ## Format code
	$(GOLANGCI_LINT) fmt

.PHONY: vuln
vuln: ## Check dependencies and the standard library for known vulnerabilities
	$(GOVULNCHECK) ./...

# sqlc needs cgo for its Postgres parser, so it runs from its image.
SQLC := docker run --rm -u $$(id -u):$$(id -g) -v "$(CURDIR):/src" -w /src \
	sqlc/sqlc:1.31.1@sha256:70f53171d27b2424e9358869975455a6e955a5aa8e58a998a270a6e34e525537

.PHONY: generate
generate: ## Regenerate Go code from the SQL queries (sqlc)
	$(SQLC) generate

.PHONY: generate-check
generate-check: ## Fail if the generated code is out of date
	$(SQLC) diff

.PHONY: check
check: vet lint test vuln ## Run the Go checks CI runs (see also api-test)

.PHONY: api-test
api-test: ## Run the Bruno collection against the full compose stack, in isolation (needs Docker)
	@trap '$(COMPOSE_TEST) down -v --remove-orphans >/dev/null 2>&1' EXIT; \
	$(COMPOSE_TEST) up -d --build --wait || { $(COMPOSE_TEST) logs; exit 1; }; \
	curl -sf http://127.0.0.1:19090/-/ready >/dev/null || { echo "prometheus not ready"; exit 1; }; \
	$(BRUNO) run --env local --env-var baseUrl=http://127.0.0.1:$(API_TEST_PORT)

.PHONY: up
up: ## Start Postgres and Prometheus in Docker (needs .env)
	docker compose up -d --wait postgres prometheus

.PHONY: db
db: up migrate seed ## Start Postgres, apply migrations and add the demo restaurants

.PHONY: migrate
migrate: ## Apply database migrations (needs make up)
	go run ./cmd/pony migrate up

.PHONY: migrate-status
migrate-status: ## Show which migrations are applied
	go run ./cmd/pony migrate status

.PHONY: seed
seed: ## Add the demo restaurants and dishes if missing
	go run ./cmd/pony seed

.PHONY: down
down: ## Stop the compose stack, keeping its data
	docker compose --profile app down

.PHONY: clean
clean: ## Remove build output
	rm -rf bin coverage.out
