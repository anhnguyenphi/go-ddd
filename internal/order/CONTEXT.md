# order context

Owns orders placed by customers. Status: **partial**. One complete slice,
`PlaceOrder`, which is the reference for a synchronous call into another
context. The aggregate is a skeleton (no lines, no totals, no status).

Domain memory for this context. `tests/arch/context_memory_test.go` fails
when the aggregates or integration events below drift from the code. Update
it with the `update-memory` skill.

## Language

| Term | Meaning | In code |
|---|---|---|
| Order | A purchase placed by one customer at one moment | `domain.Order`, `domain.OrderID` |
| Place | The only way an order comes to exist. There is no draft or cart state | `domain.Place`, `commands.PlaceOrder` |
| Customer | Here, only an id that was **verified to exist at placement time**. Order holds no name, email or status, and does not track the customer afterwards | `Order.customerID` (a plain string) |
| Verify | Ask the customer context "does this id exist?" before building the order | `application.CustomerVerifier` |

## Aggregates

`Order` (root, `domain/order.go`).

- An order requires a customer id (`Place` returns a validation error for an
  empty one).
- An order is only placed for a customer the customer context knows. This is
  checked by the command handler through `CustomerVerifier` **before** the
  aggregate is built, not by the aggregate. An unknown customer is
  `ErrNotFound`, so the API answers 404, not 400 or 422 (pinned by the
  `OrderREST` scenario).
- `Place` records the domain event `OrderPlaced`.

## Events published

None. `Place` records `OrderPlaced`, but the handler never publishes it and
there is no integration mapping (no `infrastructure/messaging`).

## Events consumed

None.

## Synchronous dependencies

| Calls | How | Notes |
|---|---|---|
| customer: `GetCustomer` | `infrastructure/customerclient.Verifier` implements `application.CustomerVerifier` against the public `customerv1` gRPC contract, in-process. Wired in `internal/bootstrap` | Served from customer's **read model**, so a customer created a moment ago can still read as unknown (404) until the projection catches up. `NotFound` is translated back to `ErrNotFound` |

## Decisions

Dates are when the decision was recorded here, not when it was made.

- 2026-11-14: Customer verification is a synchronous call, not an event
  reaction. Why: there is no order to build at all without a definite yes/no
  now. Rejected: placing first and cancelling on a later event.
- 2026-11-14: The port is defined by order and implemented against customer's
  public proto, never its Go packages. Why: the day order becomes its own
  service only the value wired in `bootstrap` changes.
- 2026-11-14: Order keeps only the customer id. Why: customer data belongs to
  the customer context; a copy would go stale.

## Open questions

- `order.v1.placed` is the planned integration event for `OrderPlaced`. The
  customer context already subscribes to it, but nothing publishes it: the
  handler doesn't call `Events.Publish` and there is no mapper.
- The repository is always in-memory, even in the Postgres profile, and
  `PlaceOrder` does not run in `UoW.Within`. Orders are lost on restart.
- Should placing be refused for an `inactive` customer? Today only existence
  is checked.
- `PlaceOrder` over REST returns 200, not 201 (no `x-http-code` metadata).
  Deliberate or an omission? Pinned as-is by the `OrderREST` scenario.
