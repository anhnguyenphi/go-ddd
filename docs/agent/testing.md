# Testing

How the test suites fit together, where a new test goes, and how to run each
layer. The live harness has its own guide: [harness.md](harness.md).

## Layers

| Layer | Where | Tag | Infra | Command | What it buys |
|---|---|---|---|---|---|
| Unit | `internal/**/_test.go` | none | none | `make test` | Domain rules, handlers, adapters with fakes or mockery mocks |
| Arch | `tests/arch` | none | none | `make test` | Layering rules, test-only packages kept out of prod, e2e stays black-box, every context's `CONTEXT.md` matches its aggregates and integration events |
| Contract | `tests/contract` | `contract` | none | `make test-contract` | Integration-event wire form pinned to golden files |
| Integration | `*_integration_test.go` next to the adapter it tests | `integration` | testcontainers | `make test-integration` | One adapter against real Postgres 16 / Redpanda: SQL repositories and read models, migrations, Kafka delivery/retry/DLQ |
| E2E | `tests/e2e` | `e2e` | a running deployment | `make test-e2e-harness` / `E2E_BASE_URL=... make test-e2e` | The whole app: black-box scenarios against the real binaries (harness, CI, staging) |

Each layer has one job, with no overlap: integration tests stop at one
adapter's boundary, and everything that needs the wired application (routing,
outbox → bus → projection, cross-context reactions) is e2e's job.

`scripts/check.sh` runs unit, arch and contract. CI (`.github/workflows/ci.yml`) also runs the infra integration suite
with `REQUIRE_INFRA=1`, and the e2e suite against the real binaries in a
harness sandbox (`SANDBOX=ci make harness-verify ARGS=--e2e`), together with
the check-DSL tour (`_harness/checks/example.sh.sample`) as the harness's own
self-test.

**These layers are the regression suite.** The harness
([harness.md](harness.md)) is a different thing: a live sandbox per worktree
where an agent sees its change working. Its `check` files are scratch
(gitignored, never run by CI). Anything that must stay fixed is a test here.

## Scenarios (e2e)

`internal/testsupport/scenarios` holds black-box behavior scenarios
(`CustomerLifecycle`, `OrderGRPC`, ...). They use only the public contract
(REST paths, `api/proto` clients), make their own unique data, and poll the
eventually consistent read side. `tests/e2e` runs them against whatever
`E2E_BASE_URL` / `E2E_GRPC_ADDR` point at: your harness sandbox
(`make harness-verify ARGS=--e2e`, or `make harness-e2e ARGS='-run TestScenarios/<Name>'`
once it's up), CI, or staging.

A new user-visible behavior gets a scenario function plus an entry in `All()`.

## Where does my test go?

- A rule inside one aggregate or value object: a unit test in `domain/`.
- A command or query handler: a unit test with the memory repo, or mockery
  mocks (`mocks.NewRepository(t)`, `mocks.NewReadModel(t)`).
- A new adapter for an existing port (repository, read model): run the
  conformance suite (`repotest.Run`, `projectiontest.Run`) against it in a
  `*_integration_test.go` in the adapter's own package. The memory adapter runs the same suite as a unit
  test, so the fast profile can't drift from production.
- A new port: write its conformance package next to the adapters
  (`<port>test`, see `persistence/repotest`). Run it from a memory unit test
  and from the real adapter's `*_integration_test.go`.
- A new or changed integration event: add a case in `tests/contract` and run
  `make test-contract ARGS=-update`. Accept only additive diffs in the golden.
  A breaking change gets a `v2` event.
- A migration: nothing to add. `TestMigrations_DownAllThenUpAgain` runs every
  migration down and up again.
- Behavior a client can see: a scenario in `internal/testsupport/scenarios`.
- A side effect a client can't see (a notification, a cross-context reaction):
  a unit test of the handler with a mock or fake port, as in
  `internal/notification/application/handlers`. If the effect goes through a
  real adapter (SQL, Kafka, an HTTP client), that adapter gets its own
  `*_integration_test.go`. Watch the wired effect live with a scratch check
  (`--logs`, `--calls`, `--events`); a client-visible consequence gets a
  scenario.
- Kafka delivery semantics (retries, DLQ, ordering): `internal/eventbus/kafka/bus_integration_test.go`.
  Use `newIsolatedBus`, which gives you unique topics.

### Integration test files

An integration test lives in the package of the code it tests, in a file named
`*_integration_test.go` that starts with `//go:build integration` (so `make
test` never starts containers). Use the external `<pkg>_test` package: the
migrate test must, because `testsupport/containers` imports `migrate`. Each
package with infra tests has exactly one tagged `TestMain`:

```go
func TestMain(m *testing.M) { os.Exit(containers.RunMain(m)) }
```

Today: `internal/eventbus/kafka`, `internal/customer/infrastructure/persistence/postgres`,
`internal/customer/infrastructure/projections` and `internal/platform/database/migrate`.
An integration test never wires the whole application; that's e2e.

## Helpers

| Package | Use |
|---|---|
| `testsupport/containers` | `PostgresDSN(t)` / `PostgresDB(t)` give you a fresh migrated database per test (cloned from a template in milliseconds). `KafkaBrokers(t)` and `UniqueGroupID(t)` cover Kafka. Wire `TestMain` to `containers.RunMain`. |
| `testsupport/api` | `Client` (`Get`/`Post`/`Patch`, `.Expect(t, 200)`), `Eventually`, `AwaitCustomer`, `UniqueEmail`. It never imports the app. |
| `testsupport/scenarios` | Black-box scenarios run by `tests/e2e` (see above). |

Test-only packages (`internal/testsupport/...`, `*test` packages such as
`repotest`) must never be imported by production code. `tests/arch` enforces
this.

## Docker and testcontainers

- Infra tests skip when Docker is unreachable, and always with `-short`. Set
  `REQUIRE_INFRA=1` to fail instead (CI does).
- testcontainers-go ignores Docker CLI contexts. With colima, the make targets
  prefix `scripts/testcontainers-env.sh` output (`DOCKER_HOST=...`). When you
  run `go test` by hand, do the same: `env $(scripts/testcontainers-env.sh) go test ...`.
- One Postgres and/or one Redpanda container per test binary, so one per
  package that has integration tests (they start in parallel, as `go test`
  runs packages in parallel; cap with `-p` if the machine struggles). Images
  match `_harness/docker-compose.yml`. The first infra test in each package
  pays the startup cost (~10s), and joining a fresh Kafka consumer group takes
  roughly 6-20s.
