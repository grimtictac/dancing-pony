# Keep these in step with .github/workflows/ci.yml.
GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION   := v1.9.0

GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK   := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# The Bruno CLI needs Node 22+, so it runs in a throwaway container.
BRUNO_CLI_VERSION := 4.3.0
API_TEST_ADDR     := 127.0.0.1:18080
BRUNO := docker run --rm --network host -v "$(CURDIR)/bruno:/collection" -w /collection \
	node:22-alpine npx --yes @usebruno/cli@$(BRUNO_CLI_VERSION)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build bin/pony
	go build -trimpath -o bin/pony ./cmd/pony

.PHONY: run
run: ## Run the API locally
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

.PHONY: check
check: vet lint test vuln ## Run everything CI runs except api-test

.PHONY: api-test
api-test: build ## Start the API and run the Bruno collection against it (needs Docker)
	@./bin/pony serve -addr $(API_TEST_ADDR) >/dev/null 2>&1 & pid=$$!; \
	trap 'kill $$pid 2>/dev/null' EXIT; \
	for i in $$(seq 50); do \
		kill -0 $$pid 2>/dev/null || { echo "pony failed to start on $(API_TEST_ADDR)"; exit 1; }; \
		curl -sf http://$(API_TEST_ADDR)/health >/dev/null && break; \
		sleep 0.1; \
	done; \
	$(BRUNO) run --env local --env-var baseUrl=http://$(API_TEST_ADDR)

.PHONY: clean
clean: ## Remove build output
	rm -rf bin coverage.out
