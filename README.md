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

### Thinking model per package

When deciding where a piece of code belongs, ask the question next to that
package rather than pattern-matching on file type:

- **`cmd/`** — "How does this process start and what does it wire together?"
- **`internal/platform/`** — "Is this a technical capability with zero business
  meaning (config, logging, http server, db connection)?"
- **`internal/shared/domain/`** — "Is this a DDD primitive every bounded
  context needs (AggregateRoot, DomainEvent, base error types)?"
- **`internal/shared/application/`** — "Is this an application-layer contract
  (Command, Query, UnitOfWork, DomainEventPublisher) every context implements
  the same way?"
- **`internal/shared/infrastructure/`** — "Is this infra glue (clock, id
  generation, tx-in-context) with no domain meaning of its own?"
- **`internal/eventbus/`** — "How does a domain event get from one context's
  transaction to another context's inbox — in-memory, via outbox, via Kafka?"
- **`internal/bootstrap/`** — "Where do concrete adapters get plugged into
  the ports each context's application layer defined?"
- **`internal/customer/`, `internal/order/`, `internal/notification/`,
  `internal/identity/`, `internal/payment/`** (each context, per sub-folder):
  - `domain/` — "What are the business rules and invariants, independent of
    any framework or database?" Broken down by what's actually in
    `customer/domain/`:
    - Aggregate (`customer.go`) — "What invariant must hold on *this one*
      instance at all times?" (e.g. a customer always has a valid email).
      Every state change is a method on the aggregate, never a field set
      from outside — the aggregate is the only thing allowed to break its
      own rules, which is also why it's the only thing allowed to fix them.
    - Value objects (`email.go`, `customer_id.go`, `status.go`) — "Is this a
      piece of data whose validity I never want to re-check once
      constructed?" `domain.NewEmail` either returns something guaranteed
      valid or an error — nothing downstream re-validates it.
    - Domain events (`events/events.go`) — "What already happened that
      other parts of this context, or other contexts entirely, might care
      about?" Recorded on the aggregate, pulled and published after the
      transaction commits — the aggregate doesn't know or care who's
      listening.
    - Repository port (`customer_repository.go`) — "What's the minimum the
      domain needs to load/save itself?", owned here for the same reason
      described in ports/adapters below — the domain defines the shape,
      `infrastructure/persistence` fulfills it.
    - Domain service (`services/email_uniqueness.go`) — "Is this a rule that
      spans more than one aggregate instance, so no single aggregate can
      enforce it alone?" (e.g. email uniqueness across *all* customers).
      Kept separate from the aggregate for that reason, but still pure
      business logic — no transactions, clocks, or IDs, unlike the command
      handlers in `application/` that call it.
  - `application/` — "What use case am I orchestrating, and what ports
    (interfaces) does it need from the outside world?"
  - `infrastructure/` — "How do I fulfill the ports application defined,
    using a real database, message bus, or external service?"
  - `interfaces/` — "How does the outside world (gRPC, REST via gateway,
    Kafka consumer) trigger a use case?"
  - Ports / adapters (e.g. `customer/application`'s repository port +
    `customer/infrastructure/persistence/{memory,postgres}`, or
    `order/application.CustomerVerifier` +
    `order/infrastructure/customerclient`) — "Am I defining a *port* (an
    interface the application layer owns, describing what it needs, with no
    mention of any concrete technology) or writing an *adapter* (a concrete
    implementation of someone else's port, free to mention drivers, SQL,
    gRPC clients, whatever the port hides)?" The port always lives next to
    the code that needs it, never next to the code that satisfies it — that
    ownership direction is what lets `infrastructure/` be swapped (memory
    for postgres, local bus for Kafka) without `application/` or `domain/`
    changing at all.
  - CQRS (`customer`'s `application.ReadModel` port + its
    `infrastructure/projections` implementation) — "Am I changing state
    (goes through the aggregate — one write model, strongly consistent) or
    reading it (goes through a projection shaped for the caller, only
    eventually consistent with the last write)?" Never read through the
    write-side aggregate, and never assume a read right after a write
    reflects it — the projection catches up asynchronously off the same
    integration events other contexts consume.
- **`pkg/`** — "Is this stable and generic enough for code outside this
  module to import?"
- **`api/`** — "What's the versioned contract (proto/OpenAPI) that crosses a
  process boundary?"
- **`migrations/`** — "How does the schema evolve, and can it roll forward
  safely?"
- **`deployments/`** — "How does this run in a real environment (compose,
  k8s, Helm)?"
- **`configs/`** — "What varies per environment, and what's a safe default?"
- **`tests/`** — "What layer of confidence am I buying — unit, integration,
  contract, or end-to-end?"

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
