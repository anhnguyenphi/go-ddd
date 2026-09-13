# myapp

A runnable, enterprise-oriented Go skeleton built around **Domain-Driven Design**,
**hexagonal / clean architecture**, an **event bus**, and the **transactional outbox**
pattern. It is structured as a **modular monolith** with hard boundaries between
bounded contexts, so individual contexts can later be extracted into services
without rewrites.

## Layout

```
cmd/                 Process entrypoints (api, worker, migrate)
internal/
  platform/          Technical capabilities shared by the whole app
                     (config, logging, http, database)
  shared/            Genuinely shared *architectural* abstractions
    domain/          AggregateRoot, DomainEvent, base errors
    application/     Command/Query/UnitOfWork/DomainEventPublisher contracts
    infrastructure/  clock, id, transaction (tx-in-context)
  eventbus/          Transport-agnostic event bus + local/kafka/outbox impls
  bootstrap/         Composition root — wires everything together
  customer/          Fully implemented bounded context (reference slice)
    domain/          Aggregates, value objects, domain events, policies
    application/     Commands, queries, DTOs, ports
    infrastructure/  Persistence, messaging (mappers), projections
    interfaces/      gRPC (also transcoded to REST by grpc-gateway) / consumers
  notification/      Consumer context — reacts to customer integration events
  order/             Sync-communication example — PlaceOrder calls customer
                     directly (see below); gRPC + REST, same as customer
  identity/ payment/ Placeholder contexts (structure only)
pkg/                 Library code safe for external import
api/                 OpenAPI + protobuf contracts
migrations/          SQL migrations (customer schema + outbox)
deployments/         docker-compose, Kubernetes, Helm
configs/             Config files (env vars override everything)
tests/               integration / contract / e2e suites
```

## Dependency rule

```
interfaces ──> application ──> domain <── infrastructure
```

Domain depends on nothing but the standard library and `shared/domain`.
Cross-context communication happens **only** through commands, queries, and
integration events — never by importing another context's packages
(`tests/arch/layering_test.go` enforces this by inspecting the real import
graph, not just by convention).

## Cross-context communication: async vs sync

Two contexts need to talk to each other for different reasons, and this
skeleton has a reference example of each:

- **Async (event-driven)** — `customer` → `notification`. Creating a customer
  publishes `customer.created` to the outbox; a relay forwards it to the event
  bus; `notification` reacts whenever it gets around to it. Nothing about
  creating a customer needs to wait for a welcome email to send, so this is
  fire-and-forget, and `notification` can be down without breaking customer
  creation.
- **Sync (request/response)** — `order` → `customer`. Placing an order needs a
  definite yes/no about the customer *right now*: there is no order to build
  at all if the customer doesn't exist, so nothing is gained by publishing an
  event and reacting to it later. `order/application.CustomerVerifier` is a
  port order's own application layer defines; `order/infrastructure/customerclient`
  implements it with a call, in-process, against the same `customerv1` gRPC
  contract external clients use. Neither context imports the other's
  internal packages — only the versioned proto contract is shared, and the
  composition root (`internal/bootstrap`) is what actually wires one
  context's service into the other's adapter.

Order's own delivery adapter (`internal/order/interfaces/grpc`) exposes this
as `order.v1.OrderService/PlaceOrder` on the same gRPC server customer runs
on (`:9090`), transcoded to REST on `:8080` exactly like customer:

```bash
curl -sS -XPOST localhost:8080/api/v1/orders \
  -H 'content-type: application/json' \
  -d '{"customer_id":"<id-from-creating-a-customer-above>"}' | jq

grpcurl -plaintext -d '{"customer_id":"<id-from-creating-a-customer-above>"}' \
  localhost:9090 order.v1.OrderService/PlaceOrder
```

Both land on the same merged OpenAPI doc at `/openapi.json` / `/docs`
(`api/openapi/myappv1.swagger.json` — see buf.gen.yaml's `allow_merge`).

Try it in code: `internal/order/application/commands/place_order_test.go` is
a fast unit test with a fake verifier; `tests/integration/order_sync_test.go`
wires the real `customer` context and calls `PlaceOrder` over both gRPC and
REST — including the case where the customer doesn't exist yet.

## Quick start

The snap `go` wrapper is broken in some shells. If `go version` prints nothing,
use the real binary:

```bash
export GO=/snap/go/current/bin/go     # or wherever your toolchain lives
```

```bash
make build          # compile api, worker, migrate into ./bin
make test           # unit tests
make run-api        # start the HTTP API on :8080
```

### Try the reference slice

`api/proto/customer/v1/customer.proto` is the single source of truth for the
customer use cases: gRPC on `:9090` is the real service
(`internal/customer/interfaces/grpc`), and REST on `:8080` is
[grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) transcoding
HTTP/JSON straight to that same service in-process, per the `google.api.http`
annotations in the proto — there is no separate hand-written HTTP adapter.

```bash
# create a customer (REST, via the gateway)
curl -sS -XPOST localhost:8080/api/v1/customers \
  -H 'content-type: application/json' \
  -d '{"name":"Ada Lovelace","email":"ada@example.com"}' | jq

# the same call over gRPC
grpcurl -plaintext -d '{"name":"Ada","email":"ada@example.com"}' \
  localhost:9090 customer.v1.CustomerService/CreateCustomer

# the create flow: command -> aggregate -> outbox (same tx) -> relay -> event bus
#   -> notification context logs a "welcome email"
#   -> customer read-model projection updates

# read it back (served from the projection / read model)
curl -sS localhost:8080/api/v1/customers | jq
curl -sS localhost:8080/api/v1/customers/<id> | jq

# change the email
curl -sS -XPATCH localhost:8080/api/v1/customers/<id>/email \
  -H 'content-type: application/json' -d '{"email":"ada@newmail.com"}' | jq
```

`GET /healthz` and `GET /readyz` are always available. The OpenAPI doc — also
generated from the proto, not hand-maintained — is served at `/openapi.json`
/ `/docs`:

```bash
make proto        # regenerate api/proto/**/*.pb.go, *.pb.gw.go, api/openapi/*.swagger.json (needs buf; `make tools` for the plugins)
make proto-lint   # lint api/proto/**/*.proto (needs buf)
make openapi-lint # validate the generated api/openapi/*.swagger.json (needs npx)
```

## Swapping infrastructure

The default wiring is 100% in-memory so the skeleton runs with zero external
dependencies. To move to real infrastructure:

| Concern        | In-memory (default)                         | Production drop-in                                  |
| -------------- | ------------------------------------------- | -------------------------------------------------- |
| Repository     | `customer/infrastructure/persistence/memory`| `.../persistence/postgres` (implemented, needs a driver) |
| Unit of work   | `shared/infrastructure/transaction.Noop`    | `transaction.SQL` (wraps `*sql.DB`)               |
| Outbox store   | `eventbus/outbox.MemoryStore`               | a `outbox.Store` backed by the `outbox_messages` table |
| Event bus      | `eventbus/local.Bus`                        | `eventbus/kafka.Bus`                              |

All swaps happen in [internal/bootstrap](internal/bootstrap); no domain or
application code changes.
