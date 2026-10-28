# Learnings

Shared memory for agents and humans: durable gotchas that earlier tasks ran
into. Read it before every task. Add to it with the `update-memory` skill.

Format: `- <fact>. <why it matters / what to do>. (<file or command>, YYYY-MM-DD)`

Keep it short (around 150 lines). Move stable topics into `architecture.md`,
`harness.md`, or `downstream-services.md`. Edit an existing entry rather than
adding a near-duplicate. Don't record secrets, customer data, the task
changelog (that's git), or anything obvious from the code.

## Toolchain

- The snap `go` shim prints nothing and exits 0, so a "passing" build or test may not have run at all. Use `/snap/go/current/bin/go`, or pass `GO=/snap/go/current/bin/go` to make. (`make`, 2026-10-28)
- The installed `golangci-lint` (`~/go/bin`) was built with an older Go and fails with `typecheck: ... export data version 5 is greater than maximum supported version 4`. That's a tool problem, not a code finding. Rebuild it with `make tools` using Go 1.27 before trusting `make lint`. (`scripts/check.sh`, 2026-10-28)
- The `Dockerfile` builder image is `golang:1.26.7`, but `go.mod` requires 1.27.1. Bump the image if a Docker build fails on the toolchain. (`Dockerfile`, 2026-10-28)

## Config

- `configs/config.json` is the dev profile (Postgres + Kafka), and it's picked up from the repo root automatically. An empty env var does not clear it, because empty values are ignored. Point `CONFIG_PATH` at another file instead. (`internal/platform/config/config.go`, 2026-10-28)
- `KAFKA_MAX_RETRIES=0` means "default" (3), not "no retries". The harness uses 1 hop with a 100ms delay. (`internal/eventbus/kafka/bus.go` `setDefaults`, 2026-10-28)

## Behavior

- The outbox is in-memory even in the Postgres profile. Events written by a process that dies before its relay runs are lost. Don't assume an `outbox_messages` row exists. (`internal/bootstrap/application.go`, 2026-10-28)
- Order placement checks the customer through customer's read model, so a customer created a moment ago can read as unknown (404) until the projection catches up. (`internal/order/infrastructure/customerclient/verifier.go`, 2026-10-28)
- `PlaceOrder` over REST returns 200, not 201. Unlike `CreateCustomer`, it doesn't set the `x-http-code` metadata. Pinned by the smoke check `order-place-for-seeded-customer`. (`internal/order/interfaces/grpc/server.go`, 2026-10-28)
- An unknown customer in `PlaceOrder` returns 404, not 400 or 422, because `ErrNotFound` passes straight through. Pinned by `order-place-unknown-customer`. (2026-10-28)
- An undecodable `customer.v1.created` payload fails both subscribers (projection and notification). It hops through `.retry.1` and lands on `customer.v1.events.dlq` without blocking the partition. (`_harness/smoke/checks_resilience.sh`, 2026-10-28)

## Harness

- A worker shutdown takes about 7s, because Kafka readers drain on SIGTERM. The consumer group then needs about 9s to become Stable again, so `harness-run` takes about 20s. That's expected, and `run.sh` waits for it. (`_harness/scripts/run.sh`, 2026-10-28)
- With colima, a deleted file in a bind mount stays visible in the container for about 20s, because directory listings are cached. That's why WireMock stubs are pushed through `/__admin/mappings/import` (`mock_reset` in `_harness/scripts/lib.sh`) instead of being mounted. Colima also mounts only `$HOME`, so a mount from `/tmp` shows up empty. (2026-10-28)
- JSON passed through `make ... ARGS='...'` loses its double quotes. To publish an event with a payload, call `_harness/scripts/kafka.sh publish ...` directly instead of `make harness-kafka`. (2026-10-28)
