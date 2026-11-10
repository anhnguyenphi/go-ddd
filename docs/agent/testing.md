# Testing

How the test suites fit together, where a new test goes, and how to run each
layer. The live harness has its own guide: [harness.md](harness.md).

## Layers

| Layer | Where | Tag | Infra | Command | What it buys |
|---|---|---|---|---|---|
| Unit | `internal/**/_test.go` | none | none | `make test` | Domain rules, handlers, adapters with fakes or mockery mocks |
| Arch | `tests/arch` | none | none | `make test` | Layering rules, test-only packages kept out of prod, e2e stays black-box |
| Contract | `tests/contract` | `contract` | none | `make test-contract` | Integration-event wire form pinned to golden files |
| Integration (memory) | `tests/integration` | `integration` | none | `make test-integration-short` | The whole app in-process (`bootstrap.Build`), in-memory profile |
| Integration (infra) | `tests/integration` | `integration` | testcontainers | `make test-integration` | Real Postgres 16 + Redpanda: SQL adapters, migrations, Kafka retry/DLQ, every profile |
| E2E | `tests/e2e` | `e2e` | a running deployment | `make test-e2e-harness` / `E2E_BASE_URL=... make test-e2e` | Black-box scenarios against real binaries (harness, staging) |
| Harness smoke | `_harness/smoke` | (bash) | compose | `make harness-verify` | Agent loop: logs, outbound calls, events on topics |

`scripts/check.sh` runs unit, arch, contract and the memory integration
profile. CI (`.github/workflows/ci.yml`) also runs the infra integration suite
with `REQUIRE_INFRA=1`, and the harness plus e2e.

## Write a scenario once, run it everywhere

`internal/testsupport/scenarios` holds black-box behavior scenarios
(`CustomerLifecycle`, `OrderGRPC`, ...). They use only the public contract
(REST paths, `api/proto` clients), make their own unique data, and poll the
eventually consistent read side. The same list runs:

- in `tests/integration` (`TestScenarios`), in-process, once per profile:
  `memory`, `postgres`, `postgres+kafka`;
- in `tests/e2e`, against whatever `E2E_BASE_URL` / `E2E_GRPC_ADDR` point at.

A new user-visible behavior gets a scenario function plus an entry in `All()`.
It's then covered on all three profiles and in e2e with no other change.

## Where does my test go?

- A rule inside one aggregate or value object: a unit test in `domain/`.
- A command or query handler: a unit test with the memory repo, or mockery
  mocks (`mocks.NewRepository(t)`, `mocks.NewReadModel(t)`).
- A new adapter for an existing port (repository, read model): add its
  constructor to the conformance suite in the infra integration tests
  (`repotest.Run`, `projectiontest.Run`). The memory adapter runs the same
  suite as a unit test, so the fast profile can't drift from production.
- A new port: write its conformance package next to the adapters
  (`<port>test`, see `persistence/repotest`). Run it from a memory unit test
  and from `tests/integration/adapters_test.go`.
- A new or changed integration event: add a case in `tests/contract` and run
  `make test-contract ARGS=-update`. Accept only additive diffs in the golden.
  A breaking change gets a `v2` event.
- A migration: nothing to add. `TestMigrations_DownAllThenUpAgain` runs every
  migration down and up again.
- Behavior a client can see: a scenario in `internal/testsupport/scenarios`.
  For the agent loop it also needs a smoke check (`_harness/smoke/`).
- Kafka delivery semantics (retries, DLQ, ordering): `tests/integration/kafka_test.go`.
  Use `newIsolatedBus`, which gives you unique topics.

## Helpers

| Package | Use |
|---|---|
| `testsupport` | `NewApp(t, WithPostgres(dsn), WithKafka(brokers, group), WithGRPC(), WithBackground())` returns a running app plus a REST client and gRPC clients. It always starts from `config.Default()`, so `configs/config.json` is never read. |
| `testsupport/containers` | `PostgresDSN(t)` / `PostgresDB(t)` give you a fresh migrated database per test (cloned from a template in milliseconds). `KafkaBrokers(t)` and `UniqueGroupID(t)` cover Kafka. Wire `TestMain` to `containers.RunMain`. |
| `testsupport/api` | `Client` (`Get`/`Post`/`Patch`, `.Expect(t, 200)`), `Eventually`, `AwaitCustomer`, `UniqueEmail`. It never imports the app. |
| `testsupport/scenarios` | Shared black-box scenarios (see above). |

Test-only packages (`internal/testsupport/...`, `*test` packages such as
`repotest`) must never be imported by production code. `tests/arch` enforces
this.

## Docker and testcontainers

- Infra tests skip when Docker is unreachable, and always with `-short`. Set
  `REQUIRE_INFRA=1` to fail instead (CI does).
- testcontainers-go ignores Docker CLI contexts. With colima, the make targets
  prefix `scripts/testcontainers-env.sh` output (`DOCKER_HOST=...`). When you
  run `go test` by hand, do the same: `env $(scripts/testcontainers-env.sh) go test ...`.
- One Postgres and one Redpanda container per test binary. Images match
  `_harness/docker-compose.yml`. The first infra test pays the startup cost
  (~10s), and joining a fresh Kafka consumer group takes roughly 6-20s.
