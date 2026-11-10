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
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
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

.PHONY: proto
proto: ## Regenerate Go + gRPC-gateway + OpenAPI from api/proto (needs buf; run `make tools` for the plugins)
	buf generate

.PHONY: proto-lint
proto-lint: ## Lint api/proto/**/*.proto (needs buf)
	buf lint api/proto

.PHONY: openapi-lint
openapi-lint: ## Validate the generated api/openapi/*.swagger.json (needs npx)
	npx --yes @redocly/cli@1.25.11 lint api/openapi/myappv1.swagger.json

.PHONY: build
build: ## Build all binaries into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/api      ./cmd/api
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/worker   ./cmd/worker
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/migrate  ./cmd/migrate
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/agentctl ./cmd/agentctl

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
test: ## Unit + arch tests (no infra)
	$(GO) test -race -count=1 $(PKG)

# testcontainers-go ignores Docker CLI contexts (colima, ...); this prefix
# derives DOCKER_HOST from the active one. Empty when nothing is needed.
TC_ENV = $(shell scripts/testcontainers-env.sh)

.PHONY: test-integration
test-integration: ## Integration: in-process app on memory + real Postgres/Redpanda via testcontainers (skips infra without Docker; REQUIRE_INFRA=1 to fail instead)
	$(TC_ENV) $(GO) test -race -count=1 -tags=integration ./tests/integration/...

.PHONY: test-integration-short
test-integration-short: ## Integration, in-memory profile only (no Docker)
	$(GO) test -race -count=1 -short -tags=integration ./tests/integration/...

.PHONY: test-contract
test-contract: ## Contract: integration-event payloads vs golden files (ARGS=-update to rewrite, additive changes only)
	$(GO) test -count=1 -tags=contract ./tests/contract/... $(ARGS)

.PHONY: test-e2e
test-e2e: ## E2E scenarios against a running deployment (needs E2E_BASE_URL; E2E_GRPC_ADDR optional)
	$(GO) test -count=1 -tags=e2e ./tests/e2e/...

.PHONY: test-e2e-harness
test-e2e-harness: ## E2E scenarios against the running harness (run harness-verify first)
	E2E_BASE_URL=http://localhost:58080 E2E_GRPC_ADDR=localhost:59090 \
		$(GO) test -count=1 -tags=e2e ./tests/e2e/...

.PHONY: test-all
test-all: test test-contract test-integration ## Unit, arch, contract and integration (everything but the harness)

.PHONY: cover
cover: ## Coverage of internal/... across unit + integration tests (report only)
	$(TC_ENV) $(GO) test -count=1 -tags=integration -covermode=atomic -coverpkg=./internal/... \
		-coverprofile=coverage.txt ./internal/... ./tests/integration/...
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

# ---- Harness: the real binaries + Postgres + Redpanda + WireMock, checked live ----
# See docs/agent/harness.md. Everything lands in tmp/harness/ (report.md, logs).

.PHONY: harness-verify
harness-verify: ## Harness: up, migrate, seed, run, smoke, report; exit 0 PASS / 1 FAIL / 2 HARNESS ERROR (ARGS="<regex>" or "--suite <name>")
	GO=$(GO) _harness/scripts/verify.sh $(ARGS)

.PHONY: harness-up
harness-up: ## Harness: start Postgres + Redpanda + WireMock, create topics, reload the stubs
	GO=$(GO) _harness/scripts/up.sh

.PHONY: harness-migrate
harness-migrate: ## Harness: migrations on the harness DB (ARGS="up" | "down 1" | "status")
	GO=$(GO) _harness/scripts/migrate.sh $(ARGS)

.PHONY: harness-seed
harness-seed: ## Harness: (re)load the idempotent fixtures in _harness/seed
	GO=$(GO) _harness/scripts/seed.sh

.PHONY: harness-run
harness-run: ## Harness: rebuild and restart api + worker from the working tree
	GO=$(GO) _harness/scripts/run.sh

.PHONY: harness-smoke
harness-smoke: ## Harness: re-run smoke checks against the running apps (ARGS="<regex>" or "--suite <name>")
	GO=$(GO) _harness/scripts/smoke.sh $(ARGS)

.PHONY: harness-logs
harness-logs: ## Harness: app logs (ARGS="errors [regex]" | "<app> [n]")
	GO=$(GO) _harness/scripts/logs.sh $(ARGS)

.PHONY: harness-mocks
harness-mocks: ## Harness: WireMock (ARGS="requests [svc]" | unmatched | routes | "scenario <svc> <kind>" | reset)
	GO=$(GO) _harness/scripts/mocks.sh $(ARGS)

.PHONY: harness-events
harness-events: ## Harness: events published since the last check (ARGS="--all" | "--raw" | <event-name>)
	GO=$(GO) _harness/scripts/events.sh $(ARGS)

.PHONY: harness-kafka
harness-kafka: ## Harness: agentctl kafka against the harness broker (ARGS="publish -name ..." | topics)
	GO=$(GO) _harness/scripts/kafka.sh $(ARGS)

.PHONY: harness-down
harness-down: ## Harness: stop apps and dependencies (ARGS=--purge also drops data, logs and the report)
	GO=$(GO) _harness/scripts/down.sh $(ARGS)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist coverage.txt coverage.html
