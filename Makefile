# myapp — enterprise DDD skeleton
#
# The snap `go` shim may be broken in some shells; override with:
#   make GO=/snap/go/current/bin/go build

GO         ?= go
BIN_DIR    ?= bin
PKG        := ./...
LDFLAGS    := -s -w

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: tools
tools: ## Install dev tools (golangci-lint, govulncheck, ...)
	GO=$(GO) scripts/install-tools.sh

.PHONY: mocks
mocks: ## Regenerate test mocks (mockery; run `make tools` first)
	mockery
	$(GO) mod tidy

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	$(GO) mod tidy

.PHONY: build
build: ## Build all binaries into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/api     ./cmd/api
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/worker  ./cmd/worker
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/migrate ./cmd/migrate

.PHONY: run-api
run-api: ## Run the HTTP API
	$(GO) run ./cmd/api

.PHONY: run-worker
run-worker: ## Run the background worker
	$(GO) run ./cmd/worker

.PHONY: vet
vet: ## go vet
	$(GO) vet $(PKG)

.PHONY: test
test: ## Run unit tests
	$(GO) test -race -count=1 $(PKG)

.PHONY: test-integration
test-integration: ## Run integration tests (requires infra; see tests/integration)
	$(GO) test -race -count=1 -tags=integration ./tests/integration/...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests
	$(GO) test -count=1 -tags=e2e ./tests/e2e/...

.PHONY: cover
cover: ## Unit tests with coverage report
	$(GO) test -covermode=atomic -coverprofile=coverage.txt $(PKG)
	$(GO) tool cover -func=coverage.txt | tail -1

.PHONY: lint
lint: ## Run golangci-lint (run `make tools` first)
	golangci-lint run --build-tags=integration,contract,e2e

.PHONY: vuln
vuln: ## Scan for known vulnerabilities (run `make tools` first)
	govulncheck ./...

.PHONY: fmt
fmt: ## Format the tree
	$(GO) fmt $(PKG)

.PHONY: migrate
migrate: ## Print the migration plan
	$(GO) run ./cmd/migrate -dir ./migrations up

.PHONY: docker-up
docker-up: ## Start local infra (postgres, kafka)
	docker compose -f deployments/docker/docker-compose.yml up -d

.PHONY: docker-down
docker-down: ## Stop local infra
	docker compose -f deployments/docker/docker-compose.yml down -v

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist coverage.txt coverage.html
