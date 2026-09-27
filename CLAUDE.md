# CLAUDE.md

Go modular monolith (module `github.com/example/myapp`) built as a DDD /
hexagonal skeleton: bounded contexts, CQRS read models, a transactional outbox,
and a pluggable event bus (in-process or Kafka). `README.md` explains the
architecture in depth, including a "which package does this belong in" guide.
Read it before adding a new context or layer.

## Toolchain gotcha

The snap `go` shim prints nothing in this environment. Use the real binary:

```bash
export PATH=/snap/go/current/bin:$PATH   # needed: tests/arch shells out to `go list`
make GO=/snap/go/current/bin/go <target> # or pass GO= to make
```

`go.mod` requires Go 1.27.1. `vendor/` exists locally but is gitignored, so
don't commit it. After changing dependencies, run `go mod tidy` (and
`go mod vendor` if you keep the vendor dir).

## Commands

| Task | Command |
|---|---|
| Build `api`, `worker`, `migrate`, `agentctl`, `mockapi` into `./bin` | `make build` |
| Unit tests (includes the arch tests) | `make test` (`go test -race -count=1 ./...`) |
| Single test | `go test -run TestName ./internal/customer/domain/...` |
| Integration tests (in-memory, no infra) | `make test-integration` (`-tags=integration`) |
| Contract tests (event payload shapes) | `go test -tags=contract ./tests/contract/...` (no make target) |
| E2E against a running server | `E2E_BASE_URL=http://localhost:8080 make test-e2e` |
| Full local gate | `GO=/snap/go/current/bin/go scripts/check.sh` |
| Lint | `make lint` (golangci-lint v2 with build tags `integration,contract,e2e`) |
| Regenerate mocks | `make mocks` (mockery v2, configured in `.mockery.yaml`) |
| Regenerate proto, gateway, and OpenAPI | `make proto`, then `make proto-lint` / `make openapi-lint` |
| Install dev tools | `make tools` (pinned versions in `scripts/install-tools.sh`) |
| Local Postgres and Redpanda | `make docker-up` / `make docker-down` |
| Migrations | `go run ./cmd/migrate -dir ./migrations up\|down [-steps N]` (prints the plan only when `DATABASE_URL` is unset) |
| Bring up docker infra + api/worker/mockapi for testing | `make GO=/snap/go/current/bin/go agent-stack-up` |
| Tear it down | `make agent-stack-down` |
| Agent self-test, full (mocks + full gate + integration + stack up/down + HTTP e2e) | `make GO=/snap/go/current/bin/go agent-verify` |
| Mock external-API server (canned HTTP responses, request capture) | `make run-mockapi` |

## Agent self-test harness

`scripts/agent/` gives an agent a way to validate a change end-to-end without
a human driving it, beyond what `make test`/`test-integration` (in-memory)
cover. It is deliberately generic — it hands you a running stack and
composable primitives, not one hardcoded scenario, so it can verify whatever
feature you're actually working on:

- **`scripts/agent/stack-up.sh`** — starts real Postgres + Redpanda
  (`docker-compose`), runs migrations, and starts `mockapi`, `api`, and
  `worker` against them. Idempotent (a second call is a no-op unless
  `--force`); `--skip-docker` assumes infra is already up. Prints a summary
  (URLs, log paths, example `agentctl`/`mockapi` invocations) and **leaves
  everything running** — it does not tear down on its own.
- **`scripts/agent/stack-down.sh`** — tears down whatever `stack-up.sh`
  started (safe no-op if nothing is up).
- **`cmd/agentctl`** — `kafka publish`/`kafka consume`, for **any** event
  name, not a fixed one. `publish` reuses the production `kafka.Bus.Publish`
  path (same topic-naming convention, same JSON wire codec — see
  `kafka.DefaultTopicFor`), so it can trigger any consumer exactly as a real
  producer would. `consume` reads a topic directly (its own reader, no
  `GroupID`) so it never joins — and can't disturb — the app's real consumer
  group.
- **`cmd/mockapi`** — a generic HTTP double for an external API (payment
  gateway, identity provider, ...): canned responses from a JSON routes file
  (`configs/mockapi.routes.example.json`), every request recorded and
  queryable at `GET /__requests`, resettable at `POST /__reset`, and
  reconfigurable **at runtime** via `POST /__routes` — so a running stack can
  be repointed at a new response shape for whatever's being tested without a
  restart. Nothing in the codebase points at it yet (no context makes an
  external HTTP call today) — wire a future integration's client at its
  `-addr` in tests.
- **`scripts/agent/examples/kafka-roundtrip.sh`** — a worked example (not run
  automatically by anything) showing how to compose the above into a
  feature-specific check: publish a `customer.v1.created` event and assert
  both the read-model projection and the notification handler consumed it.
  Copy and adapt this pattern for whatever event/consumer you're verifying.
- **`scripts/agent/verify.sh`** — the single command for "is this change
  good": mocks, the full local gate (`scripts/check.sh`), integration tests,
  then `stack-up.sh` + the generic `tests/e2e` HTTP suite + `stack-down.sh`.
  Flags: `--skip-mocks`, `--skip-integration`, `--skip-e2e`, `--keep-up`
  (leave the stack running after the e2e stage for follow-up poking).

## Running locally: config pitfall

Config precedence is: defaults < JSON file (`CONFIG_PATH`, else
`configs/config.json`) < env vars. See
[internal/platform/config/config.go](internal/platform/config/config.go).

**The committed `configs/config.json` sets a Postgres DSN and Kafka brokers.**
So `make run-api` from the repo root selects the Postgres and Kafka profile
unless that infra is up. Setting an env var to empty does not clear a value;
empty env vars are ignored. To get the zero-infra in-memory profile that the
README describes, point `CONFIG_PATH` at a file without a DSN or brokers, or
run from a directory without `configs/config.json`. Tests use
`config.Default()` directly, which is in-memory.

- An empty `Database.DSN` selects in-memory repos and read models with `transaction.Noop`.
- A set DSN selects the Postgres repo and read model with `transaction.SQL`.
- An empty `Kafka.Brokers` selects `eventbus/local`. A set value selects `eventbus/kafka`, which has retry topics and a DLQ.
- The outbox store is always `outbox.MemoryStore`, even in the Postgres profile. The `outbox_messages` migration exists, but no SQL `Store` is implemented or wired yet.
- JSON duration fields are nanosecond integers. Env vars take Go durations such as `1s`.

Ports: REST (grpc-gateway) on `:8080`, gRPC on `:9090`. Other endpoints:
`/healthz`, `/readyz`, `/openapi.json`, and `/docs`.

## Architecture essentials

- **Composition root**: [internal/bootstrap](internal/bootstrap) is the only
  place where concrete adapters get chosen. `cmd/api` runs HTTP, gRPC, and
  `RunBackground` (outbox relay plus the Kafka consumer loop). `cmd/worker`
  runs only `RunBackground`. To add a context, add a `module.go` in the context
  and wire it in `bootstrap/modules.go`.
- **Context layout** (`internal/<ctx>/`): `domain/` → `application/`
  (`commands/`, `queries/`, `ports.go`, `dto/`) → `infrastructure/` →
  `interfaces/` (gRPC). `module.go` is the context's only public entry point.
  `customer` is the full reference slice. `order` shows a sync cross-context
  call. `notification` consumes events. `identity` and `payment` are
  placeholders.
- **Enforced boundaries** in [tests/arch/layering_test.go](tests/arch/layering_test.go),
  which run with the unit tests:
  - `*/domain` must not import `/infrastructure`, `/interfaces`,
    `internal/eventbus`, `internal/platform`, `database/sql`, or `net/http`.
  - `*/application` must not import `/infrastructure/`, `/interfaces/`, or `net/http`.
  - Bounded contexts (`customer`, `notification`, `order`, `payment`, `identity`)
    must never import each other. They share only the versioned proto
    contracts in `api/proto/*/v1` and integration events. The bootstrap does
    the cross-wiring, e.g. `customerclient.New(customerMod.GRPC())`.
- **Write path**: a command handler runs inside `UoW.Within`, validates value
  objects, calls the aggregate, then `Repo.Save` and
  `Events.Publish(agg.PullEvents()...)`. The customer
  `messaging.Dispatcher` maps domain events to versioned integration events
  (for example `customer.v1.created`) and writes them to the outbox. The relay
  forwards them to the bus, and subscribers (the read-model projection and
  notification) react.
- **Read path**: queries go through the `ReadModel` port (projections), never
  through the aggregate. The read model is eventually consistent. In tests,
  call `app.DrainOutbox(ctx)` to flush the outbox before reading.
- **Ports live with their consumer**: the repository in `domain/`, other ports
  in `application/ports.go`. Adapters live in `infrastructure/`.

## Conventions

- REST is not hand-written. Add or modify RPCs in `api/proto/<ctx>/v1/*.proto`
  with `google.api.http` annotations, then run `make proto`. The generated
  `*.pb.go`, `*.pb.gw.go`, and `api/openapi/myappv1.swagger.json` are
  committed. REST JSON uses proto field names (snake_case).
- Integration event names and payloads are a published contract, pinned by
  `tests/contract`. A breaking change requires a new version (`v1` → `v2`).
- Mocks are generated into a `mocks/` subpackage next to each interface. When
  you add an interface you want mocked, register it in `.mockery.yaml`. Never
  hand-edit files in `mocks/`.
- The import grouping is stdlib, then third-party, then
  `github.com/example/myapp`. It is enforced by goimports `local-prefixes`.
- Migrations are named `NNNN_name.up.sql` / `NNNN_name.down.sql` and each file
  runs in one transaction using the simple protocol, so a file can contain
  multiple statements.
- The `Dockerfile` builder image is `golang:1.26.7`, but `go.mod` requires
  1.27.1. Bump the builder image if a Docker build fails on the toolchain.
