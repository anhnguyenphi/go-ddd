# Context map

The index of the domain memory. Each bounded context keeps its language,
invariants, events and decisions in `internal/<ctx>/CONTEXT.md`, next to the
code. Read this page first, then the file of every context you touch.

Two memories, two jobs:

- **Domain memory** (this page and the `CONTEXT.md` files): what the business
  terms mean, which rules hold, what crosses a boundary, and why.
- **[learnings.md](learnings.md)**: operational gotchas (toolchain, config,
  tests, harness).

`tests/arch/context_memory_test.go` (in `make test`) fails when a context has
no `CONTEXT.md`, when an aggregate or an integration event in the code is
missing from it, when it lists an event the code doesn't have, or when this
page doesn't link it. Update the files with the `update-memory` skill.

## Contexts

| Context | Owns | Status | Memory |
|---|---|---|---|
| customer | Who a customer is: identity, name, email, lifecycle | Reference slice | [internal/customer/CONTEXT.md](../../internal/customer/CONTEXT.md) |
| order | Orders placed by customers | Partial (`PlaceOrder` only) | [internal/order/CONTEXT.md](../../internal/order/CONTEXT.md) |
| notification | Telling people things (welcome messages) | Consumer slice, no state | [internal/notification/CONTEXT.md](../../internal/notification/CONTEXT.md) |
| identity | Users, credentials, access | Placeholder | [internal/identity/CONTEXT.md](../../internal/identity/CONTEXT.md) |
| payment | Money taken for orders | Placeholder | [internal/payment/CONTEXT.md](../../internal/payment/CONTEXT.md) |

`internal/shared` is the shared kernel (aggregate base, error sentinels,
command/query/UoW contracts), not a context. It has no domain language.

## Relationships

Upstream decides the contract; downstream conforms to it.

| Upstream → downstream | Kind | Contract | State |
|---|---|---|---|
| customer → notification | Async, published language | `customer.v1.created` | Live |
| customer → order | Sync, customer/supplier. Order calls through its own port and an anti-corruption adapter (`order/infrastructure/customerclient`) | `customerv1.CustomerService/GetCustomer` | Live |
| order → customer | Async, published language | `order.v1.placed` | Consumer only: nothing publishes it yet |

Contexts never import each other (`tests/arch/layering_test.go`). They share
only `api/proto/*/v1` and integration event names with their payload shapes.
`internal/bootstrap` does the cross-wiring.

## Same word, different meaning

| Word | In | Means |
|---|---|---|
| Customer | customer | The aggregate: identity, name, email, status |
| Customer | order | An id that was verified to exist when the order was placed. Nothing more |
| Customer | notification | A recipient: the `email` and `name` from one event |
| User / Customer | identity / customer | Someone who can log in vs someone with a commercial relationship. Not linked today |
| Deactivated / Inactive | customer write side / read model and API | The same state under two names |
| Version | aggregate / event name | Optimistic-concurrency counter vs schema version of a contract (`v1`) |

## Cross-context decisions

Format: `- YYYY-MM-DD: <decision>. Why: <reason>. Rejected: <alternative>.`
Dates are when the decision was recorded here, not when it was made.

- 2026-11-14: Async by default; sync only when the caller cannot proceed
  without the answer. Why: an event consumer can be down without breaking
  the producer. `order → customer` is the one sync call, because there is no
  order to build without a yes/no.
- 2026-11-14: A consumer declares its own copy of the event name and payload
  struct. Why: the coupling is the versioned contract, not the producer's
  code. A breaking payload change is a new `vN`, pinned by `tests/contract`.
- 2026-11-14: A sync call into another context goes through that context's
  public proto contract, in-process. Why: extracting a context into its own
  service then changes only the wiring in `internal/bootstrap`.
