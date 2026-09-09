# myapp

A runnable, enterprise-oriented Go skeleton built around **Domain-Driven Design**,
**hexagonal / clean architecture**, an **event bus**, and the **transactional outbox**
pattern. It is structured as a **modular monolith** with hard boundaries between
bounded contexts, so individual contexts can later be extracted into services
without rewrites.

The full rationale for this layout lives in [PROJECT.md](PROJECT.md).

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
    interfaces/      HTTP / gRPC / consumers (delivery adapters)
  notification/      Consumer context — reacts to customer integration events
  identity/ order/ payment/   Placeholder contexts (structure only)
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
integration events — never by importing another context's packages.

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

```bash
# create a customer
curl -sS -XPOST localhost:8080/api/v1/customers \
  -H 'content-type: application/json' \
  -d '{"name":"Ada Lovelace","email":"ada@example.com"}' | jq

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

`GET /healthz` and `GET /readyz` are always available.

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
