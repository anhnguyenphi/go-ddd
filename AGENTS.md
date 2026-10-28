# AGENTS.md

Go modular monolith (module `github.com/example/myapp`) built as a DDD /
hexagonal skeleton: bounded contexts, CQRS read models, a transactional outbox,
and a pluggable event bus (in-process or Kafka).

This file is the one set of instructions for every agent tool (Claude Code,
Cursor, Codex) and for humans. `CLAUDE.md` only imports it. Before a task,
read:

- [docs/agent/learnings.md](docs/agent/learnings.md): gotchas earlier tasks
  ran into. Always read it.
- [docs/agent/architecture.md](docs/agent/architecture.md): contexts, layers,
  write/read paths, and config profiles.
- [docs/agent/harness.md](docs/agent/harness.md): the live test harness and
  its check DSL.
- [docs/agent/downstream-services.md](docs/agent/downstream-services.md):
  every external service the app calls, and its stub.
- `README.md` explains the architecture in depth, including a "which package
  does this belong in" guide.

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
| Build `api`, `worker`, `migrate`, `agentctl` into `./bin` | `make build` |
| Unit tests (includes the arch tests) | `make test` (`go test -race -count=1 ./...`) |
| Single test | `go test -run TestName ./internal/customer/domain/...` |
| Integration tests (in-memory, no infra) | `make test-integration` (`-tags=integration`) |
| Contract tests (event payload shapes) | `go test -tags=contract ./tests/contract/...` (no make target) |
| Full local gate (fmt, vet, build, tests, lint, vuln) | `GO=/snap/go/current/bin/go scripts/check.sh` |
| Lint | `make lint` (golangci-lint v2 with build tags `integration,contract,e2e`) |
| Regenerate mocks | `make mocks` (mockery v2, configured in `.mockery.yaml`) |
| Regenerate proto, gateway, and OpenAPI | `make proto`, then `make proto-lint` / `make openapi-lint` |
| Install dev tools | `make tools` (pinned versions in `scripts/install-tools.sh`) |
| **Live verify: real binaries + Postgres + Kafka + WireMock** | `make GO=/snap/go/current/bin/go harness-verify` |
| Re-run some checks against the running harness | `make harness-smoke ARGS='<name-regex>'` |
| Error lines from all apps, known noise filtered | `make harness-logs ARGS=errors` |
| Outbound calls that matched no stub | `make harness-mocks ARGS=unmatched` |
| Events published since the last check | `make harness-events` |
| Break a downstream | `make harness-mocks ARGS="scenario <svc> error\|timeout\|malformed\|empty"` |
| Migrations on the harness DB | `make harness-migrate ARGS="up"\|"down 1"\|"status"` |
| Stop the harness / reset everything | `make harness-down` / `make harness-down ARGS=--purge` |
| E2E Go suite against any running server | `E2E_BASE_URL=http://localhost:58080 make test-e2e` |
| Dev infra (not the harness) | `make docker-up` / `make docker-down` |

`make harness-verify` exits 0 (PASS), 1 (FAIL), or 2 (HARNESS ERROR: the
harness could not start, so nothing was verified). It writes
`tmp/harness/report.md` and leaves everything running for inspection.

## Rules

- **Never read real env files or secrets**: `.env`, `.env.local`,
  `configs/config.local.json`, anything outside the repo. The harness uses
  `_harness/.env.harness`, which holds only fake values. Keep it that way.
- **No new dependencies** (Go modules, Docker images, tools) without the
  user's approval.
- **Never hand-edit generated code**: `*.pb.go`, `*.pb.gw.go`,
  `api/openapi/*.swagger.json`, and any `mocks/` package. Regenerate it.
- **Never weaken a check to make it pass.** Change a check only when the
  check itself was wrong, and say so explicitly in your report.
- **No "done" without evidence.** Every claim ("it works", "tests pass") comes
  with the command you ran and its result.
- **Integration events are a published contract.** Pinned by
  `tests/contract`. A breaking payload change needs a new version (`v1` → `v2`).
- **One harness run at a time.** The harness holds a lock in
  `tmp/harness/.lock`. Only the verifier (or you, in a single-agent loop) runs
  it. Implementers working in parallel don't.

## Definition of done

1. Unit tests pass (`make test`, or focused on the touched packages and their
   dependents while iterating), and new logic has focused tests.
2. `scripts/check.sh` passes (fmt, vet, build, tests, lint, vuln). You
   reviewed any auto-fix diff.
3. Generated code (proto, gateway, OpenAPI, mocks) is regenerated, never
   hand-edited, and committed with the change.
4. Behavior change: `make harness-verify` passes **and** a smoke check in
   `_harness/smoke/` covers the new behavior (written before the code).
5. New outbound call: a stub in `_harness/mocks/`, a `<SVC>_URL` in
   `_harness/.env.harness`, and a row in `docs/agent/downstream-services.md`.
6. Migration: applied, rolled back, and re-applied on the harness DB
   (`harness-migrate ARGS="down 1"`, then `ARGS=up`).
7. Anything surprising is recorded in `docs/agent/learnings.md`
   (skill `update-memory`).

## Work loop

For any change in behavior, follow the `implement-and-verify-loop` skill:
read memory, then plan the checks first, then implement, then run the
quality gate, then verify live, then review, then update memory. Allow at
most **5** fix iterations, and act on new evidence each time. If the same
failure shows up twice, stop editing code. Inspect the DB, mocks, events, or
logs directly instead.

Subagents (`.agents/agents/`):

- `implementer`: one scoped change plus unit tests. Doesn't run the harness.
- `verifier`: runs the harness and reports PASS / FAIL / HARNESS ERROR with
  evidence. Has no edit tools.
- `reviewer`: read-only review for correctness, contracts, layering, and
  security.

Skills (`.agents/skills/`):
`implement-and-verify-loop`, `verify-live`, `quality-gate`, `add-endpoint`,
`add-consumer`, `add-downstream-stub`, `add-migration`, `update-memory`.

## Architecture essentials

Full version: [docs/agent/architecture.md](docs/agent/architecture.md).

- **Composition root**: [internal/bootstrap](internal/bootstrap) is the only
  place where concrete adapters get chosen. To add a context, add a
  `module.go` in the context and wire it in `bootstrap/modules.go`.
- **Context layout** (`internal/<ctx>/`): `domain/` → `application/`
  (`commands/`, `queries/`, `ports.go`, `dto/`) → `infrastructure/` →
  `interfaces/` (gRPC). `customer` is the full reference slice.
- **Enforced boundaries** in [tests/arch/layering_test.go](tests/arch/layering_test.go):
  domain imports no infrastructure, platform, eventbus, `database/sql`, or
  `net/http`. Application imports no infrastructure, interfaces, or
  `net/http`. Bounded contexts never import each other. They share only
  `api/proto/*/v1` and integration events.
- **Write path**: command handler → `UoW.Within` → aggregate → `Repo.Save` +
  `Events.Publish` → outbox → relay → bus → subscribers.
- **Read path**: queries go through the `ReadModel` projection, which is
  eventually consistent. In tests, call `app.DrainOutbox(ctx)` before reading.

## Conventions

- REST is not hand-written. Add or modify RPCs in `api/proto/<ctx>/v1/*.proto`
  with `google.api.http` annotations, then run `make proto`. The generated
  files are committed. REST JSON uses proto field names (snake_case).
- Mocks are generated into a `mocks/` subpackage next to each interface.
  Register new interfaces in `.mockery.yaml`.
- The import grouping is stdlib, then third-party, then
  `github.com/example/myapp`. It is enforced by goimports `local-prefixes`.
- Migrations are named `NNNN_name.up.sql` / `NNNN_name.down.sql`. Each file
  runs in one transaction using the simple protocol.
- Harness scripts are portable bash (macOS ships 3.2): no associative arrays
  and no `${var,,}`. See the header of `_harness/scripts/lib.sh`.
