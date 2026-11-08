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
	GRAFANA_PORT=13000 PONY_INTERNAL_PORT=19091 \
	docker compose -p dancing-pony-test --env-file .env.example --profile app

# Load tests: another isolated stack, with the app container limited like
# the droplet (1 vCPU, 512 MB) and bulk data. k6 runs in a container.
LOAD_PORT := 18083
COMPOSE_LOAD := PONY_PORT=$(LOAD_PORT) POSTGRES_PORT=15433 PONY_INTERNAL_PORT=19093 \
	docker compose -p dancing-pony-load --env-file .env.example --profile app \
	-f docker-compose.yml -f loadtest/compose.yml
LOAD_PSQL = $(COMPOSE_LOAD) exec -T postgres sh -c 'psql -q -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" $(1)'
K6 := docker run --rm -i --network host -v "$(CURDIR)/loadtest:/scripts" -w /scripts \
	grafana/k6:2.3.0@sha256:3e4186aea62e11a45560bfc51e19eeafe82fc07ce069c78aff44b7ff893d9a5e
RATE ?= 100
DURATION ?= 30s

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

.PHONY: api-lint
api-lint: ## Lint the OpenAPI spec served at /docs (needs Docker)
	docker run --rm -v "$(CURDIR)/internal/apidocs:/spec:ro" -w /spec redocly/cli:2.76.0 lint --config redocly.yaml openapi.yaml

.PHONY: up
up: ## Start Postgres, Prometheus (:9090) and Grafana (:3000) in Docker (needs .env)
	docker compose up -d --wait postgres prometheus grafana

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

.PHONY: load-up load-test load-stats load-down
load-up: ## Start the load-test stack (app limited to 1 vCPU / 512 MB) with bulk data
	$(COMPOSE_LOAD) up -d --build --wait app
	$(call LOAD_PSQL,-c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements")
	$(call LOAD_PSQL) < loadtest/fixture.sql

load-test: ## Run the k6 browsing load test against it (RATE=100 DURATION=30s)
	$(call LOAD_PSQL,-c "SELECT pg_stat_statements_reset()") >/dev/null
	$(K6) run -e BASE_URL=http://127.0.0.1:$(LOAD_PORT) -e RATE=$(RATE) -e DURATION=$(DURATION) browse.js

load-stats: ## Show where Postgres spent its time during the last load test
	$(call LOAD_PSQL) < loadtest/stats.sql

load-down: ## Remove the load-test stack and its data
	$(COMPOSE_LOAD) down -v --remove-orphans

include loadtest/prod.mk
