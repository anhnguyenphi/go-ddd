# Learnings

Shared memory for agents and humans: durable gotchas that earlier tasks ran
into. Read it before every task. Add to it with the `update-memory` skill.

Format: `- <fact>. <why it matters / what to do>. (<file or command>, YYYY-MM-DD)`

Keep it short (around 150 lines). Move stable topics into `architecture.md`,
`harness.md`, or `downstream-services.md`. Edit an existing entry rather than
adding a near-duplicate. Don't record secrets, customer data, the task
changelog (that's git), or anything obvious from the code.

## Toolchain

- The snap `go` shim does run Go and returns its exit code, but its output is lost unless stdout is a pipe: `go version` prints nothing, `go version | cat` prints the version, and a failing `go build` exits 1 with no message. So a failure is silent, not hidden. Use `/snap/go/current/bin/go`, or pass `GO=/snap/go/current/bin/go` to make, to see the output. (`make`, 2026-10-28, corrected 2026-11-12)
- `golangci-lint` v2.13.x fails on Go 1.27.3 with `typecheck: ... export data version 5 is greater than maximum supported version 4`. The bundled x/tools is too old, so rebuilding it from source with a newer Go doesn't help. The pin is now v2.15.0 (`scripts/install-tools.sh`). If this error comes back after a Go upgrade, bump the pin; it's a tool problem, not a code finding. (`scripts/check.sh`, 2026-10-31)
- golangci-lint v2.15's errcheck flags a bare `<-errCh` on a `chan error`, and staticcheck S1005 flags `_ = <-errCh`. Handle the received error (e.g. log it unless it's `context.Canceled`). (`cmd/api/main.go`, 2026-10-31)
- The `Dockerfile` builder image is `golang:1.26.7`, but `go.mod` requires 1.27.1. Bump the image if a Docker build fails on the toolchain. (`Dockerfile`, 2026-10-28)

## Config

- `configs/config.json` is the dev profile (Postgres + Kafka), and it's picked up from the repo root automatically. An empty env var does not clear it, because empty values are ignored. Point `CONFIG_PATH` at another file instead. (`internal/platform/config/config.go`, 2026-10-28)
- `KAFKA_MAX_RETRIES=0` means "default" (3), not "no retries". The harness uses 1 hop with a 100ms delay. (`internal/eventbus/kafka/bus.go` `setDefaults`, 2026-10-28)

## Testing

- testcontainers-go ignores Docker CLI contexts, so with colima it fails with `failed to create Docker provider` even though `docker` works. The make targets prefix `scripts/testcontainers-env.sh` (DOCKER_HOST + Ryuk socket override). By hand: `env $(scripts/testcontainers-env.sh) go test ...`. (`make test-integration`, 2026-11-10)
- The memory adapters must behave like Postgres, or the fast profile lies. The conformance suites caught two drifts: the memory repo accepted a stale same-version write, and a redelivered `customer.v1.created` revived a deactivated row in the memory read model. A new adapter goes into `repotest` / `projectiontest`. (`internal/customer/infrastructure/*/…test`, 2026-11-10)
- A fresh Kafka consumer group takes about 6-20s to join on Redpanda, so the first read in the `postgres+kafka` profile is slow. Scenarios poll with `ReadTimeout`; never sleep. Kafka-bus tests use unique topic prefixes (`newIsolatedBus` in `internal/eventbus/kafka/bus_integration_test.go`) so poison messages don't leak across tests. (2026-11-10)

## Behavior

- The outbox is in-memory even in the Postgres profile. Events written by a process that dies before its relay runs are lost. Don't assume an `outbox_messages` row exists. (`internal/bootstrap/application.go`, 2026-10-28)
- Order placement checks the customer through customer's read model, so a customer created a moment ago can read as unknown (404) until the projection catches up. (`internal/order/infrastructure/customerclient/verifier.go`, 2026-10-28)
- `PlaceOrder` over REST returns 200, not 201. Unlike `CreateCustomer`, it doesn't set the `x-http-code` metadata. Pinned by the `OrderREST` scenario. (`internal/order/interfaces/grpc/server.go`, 2026-10-28)
- An unknown customer in `PlaceOrder` returns 404, not 400 or 422, because `ErrNotFound` passes straight through. Pinned by the `OrderREST` scenario. (2026-10-28)
- An undecodable `customer.v1.created` payload fails both subscribers (projection and notification). It hops through `.retry.1` and lands on `customer.v1.events.dlq` without blocking the partition. Bus behavior pinned by `TestKafkaBus_FailingHandlerRetriesThenDeadLetters`; see it live with the poison check in `_harness/checks/example.sh.sample`. (2026-10-28)

## Harness

- The harness is one sandbox per worktree on shared infra (2026-11-10 redesign). The registry of slots lives in the **git common dir** (`.git/myapp-harness/`, also pid files), so every worktree sees it, and it's never committed. Sandbox names come from the worktree's directory name, so two worktrees with the same basename collide: the second gets "belongs to worktree ..." and needs `SANDBOX=<name>`. (`_harness/scripts/lib.sh`, 2026-11-10)
- WireMock is shared, so everything sandbox-scoped goes through metadata and path prefixes: stubs are imported with `deleteAllNotInImport: false`, tagged `harnessSandbox`, ids stripped, and removed with `remove-by-metadata`; the journal is cleared with `POST /__admin/requests/remove` (`urlPathPattern`), never `DELETE /__admin/requests`, which would wipe every sandbox's journal mid-check. (`mock_reset` in `_harness/scripts/lib.sh`, 2026-11-10)
- `order.v1.placed` is consumed by the customer context but no context publishes it yet (`OrderPlaced` is recorded but has no integration mapping). To exercise its consumer live, publish it by hand (`_harness/scripts/kafka.sh publish ...`, or a scratch check with `--produce`). (2026-11-10)
- Integration tests live next to their code (`*_integration_test.go`, `//go:build integration`). The migrate one must be the external `migrate_test` package: `testsupport/containers` imports `migrate`, so an internal test would be an import cycle. Each such package needs its own tagged `TestMain` calling `containers.RunMain`, and starts its own containers. Integration tests cover one adapter against real infra only; whole-app behavior is e2e's job (the in-process `testsupport.NewApp` profiles were removed as duplicates of e2e). (2026-11-11)

- Exit 0 is not proof that something ran. Three quiet passes were closed on 2026-11-12: `go test -run <typo>` prints `ok ... [no tests to run]` (`e2e.sh` now fails it), a check file with a bash syntax error lost every check after the error, and a name regex matching nothing passed on a leftover e2e result (`check.sh` records `FAIL file:<name>` / `FAIL filter:<regex>`). A new gate needs the same question: what does it report when nothing ran? (`_harness/scripts/{e2e,check}.sh`)
- Each `check` clears this sandbox's WireMock journal, and the journal is the only record of an unmatched call. Anything that clears it must call `mock_stash_unmatched <label>` first, or unstubbed calls made by the e2e suite vanish before the report. Entries are deduplicated by the journal `id`. (`_harness/scripts/lib.sh`, 2026-11-12)
- `harness-run` skips the restart when the binaries and env are byte-identical (`run.fingerprint`); the error gate then counts only log lines after `log.baseline`, which is what truncating the logs did on a restart. Anything new that reaches the apps through the environment must go into `run_fingerprint`, or a change to it won't restart them. (`_harness/scripts/{run,lib}.sh`, 2026-11-12)
- Sourcing `lib.sh` must stay free of side effects: state dirs are created by `sandbox_dirs` (from `acquire_lock`, `sandbox_allocate`, `build_bin`), and scripts call `require_sandbox` before `acquire_lock`, so a mistyped `SANDBOX=` leaves nothing behind. (2026-11-12)
- A worker shutdown takes about 7s, because Kafka readers drain on SIGTERM. The consumer group then needs about 9s to become Stable again, so a `harness-run` that restarts takes about 20s. That's expected, and `run.sh` waits for it. (`_harness/scripts/run.sh`, 2026-10-28)
- With colima, a deleted file in a bind mount stays visible in the container for about 20s, because directory listings are cached. That's why WireMock stubs are pushed through `/__admin/mappings/import` (`mock_reset` in `_harness/scripts/lib.sh`) instead of being mounted. Colima also mounts only `$HOME`, so a mount from `/tmp` shows up empty. (2026-10-28)
- JSON passed through `make ... ARGS='...'` loses its double quotes. To publish an event with a payload, call `_harness/scripts/kafka.sh publish ...` directly instead of `make harness-kafka`. (2026-10-28)
