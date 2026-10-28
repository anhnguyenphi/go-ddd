# Architecture for agents

The parts of the architecture an agent needs while changing code. `README.md`
has the long version and a "which package does this belong in" guide.

## Processes

| Binary | Runs | Notes |
|---|---|---|
| `cmd/api` | REST (grpc-gateway) on `HTTP_ADDR`, gRPC on `GRPC_ADDR`, plus `RunBackground` | REST also serves `/healthz`, `/readyz` (pings the DB), `/openapi.json`, and `/docs` |
| `cmd/worker` | `RunBackground` only: the outbox relay and the Kafka consumer loop | No HTTP surface. Logs `worker started` when ready |
| `cmd/migrate` | Applies `migrations/*.sql` and tracks them in `schema_migrations` | Prints the plan only when `DATABASE_URL` is unset |
| `cmd/agentctl` | `kafka publish / consume / mark / events` | Same topic naming and wire codec as the app |

The api and the worker both join the same Kafka consumer group. A message is
handled by whichever process owns the partition.

## Bounded contexts

`internal/<ctx>/`: `domain/` → `application/` (`commands/`, `queries/`,
`ports.go`, `dto/`) → `infrastructure/` → `interfaces/` (gRPC). `module.go` is
the only public entry point. [internal/bootstrap](../../internal/bootstrap) is
the composition root and the only place where adapters are chosen.

| Context | What it shows |
|---|---|
| `customer` | The full reference slice: commands, CQRS read model (`customer_reads`), integration events `customer.v1.{created,email_changed,deactivated}`, and an inbound handler for `order.v1.placed` |
| `order` | A synchronous cross-context call (`customerclient` → customer's gRPC server, in-process). Its repository is always in-memory |
| `notification` | Consumes `customer.v1.created` and "sends" a welcome email by logging `welcome email sent` |
| `identity`, `payment` | Placeholders |

Boundaries are enforced by [tests/arch/layering_test.go](../../tests/arch/layering_test.go):

- `*/domain` must not import `/infrastructure`, `/interfaces`,
  `internal/eventbus`, `internal/platform`, `database/sql`, or `net/http`.
- `*/application` must not import `/infrastructure/`, `/interfaces/`, or `net/http`.
- Contexts never import each other. They share only `api/proto/*/v1` and
  integration events. The bootstrap does the cross-wiring, for example
  `customerclient.New(customerMod.GRPC())`.

## Write path and read path

**Write**: a command handler runs inside `UoW.Within`, validates value
objects, calls the aggregate, then `Repo.Save` and
`Events.Publish(agg.PullEvents()...)`. The customer `messaging.Dispatcher`
maps domain events to versioned integration events and writes them to the
outbox. The relay forwards them to the bus. Subscribers (the read-model
projection, notification) react.

**Read**: queries go through the `ReadModel` port (projections), never
through the aggregate. The read model is eventually consistent. Tests call
`app.DrainOutbox(ctx)` before reading. Harness checks use `--await` or a
re-polled `GET`.

Kafka topics are per context and version: `customer.v1.created` goes to
`customer.v1.events` (`kafka.DefaultTopicFor`). A failing handler hops to
`<topic>.retry.<n>`, and after `KAFKA_MAX_RETRIES` hops it lands on
`<topic>.dlq`.

## Config profiles

Precedence: defaults < JSON file (`CONFIG_PATH`, else `configs/config.json`) <
env vars. See [config.go](../../internal/platform/config/config.go).

- An empty `Database.DSN` selects in-memory repos and read models with `transaction.Noop`.
  A set DSN selects Postgres with `transaction.SQL`.
- An empty `Kafka.Brokers` selects `eventbus/local`. A set value selects `eventbus/kafka`
  (retry topics and DLQ).
- The outbox store is always `outbox.MemoryStore`, even with Postgres. The
  `outbox_messages` table exists, but no SQL store is wired yet.
- **The committed `configs/config.json` sets a Postgres DSN and Kafka
  brokers**, so `make run-api` from the repo root needs that infra.
  Setting an env var to empty does not clear a value, because empty env vars
  are ignored. The harness avoids this with
  `CONFIG_PATH=_harness/config.harness.json` (an empty `{}`).
- JSON duration fields are nanosecond integers. Env vars take Go durations (`1s`).

| | Dev (`configs/config.json`, `make docker-up`) | Harness (`_harness/.env.harness`) |
|---|---|---|
| REST / gRPC | `:8080` / `:9090` | `:58080` / `:59090` |
| Postgres | `localhost:5432/myapp` | `localhost:55432/myapp_harness` |
| Kafka | `localhost:9092` | `localhost:59092` |
| WireMock (downstream stubs) | — | `:59999` |
| Compose project | `myapp` | `myapp-harness` |
