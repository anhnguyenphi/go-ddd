# order context

Owns orders placed by customers. Status: **partial**. One complete slice,
`PlaceOrder`, which is the reference for a synchronous call into another
context. An order is one product for one amount: no lines, no status.

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
| Product name | What is bought, as free text. There is no product catalogue, so it is not an id | `domain.ProductName`, `product_name` |
| Amount | What the customer pays for the order, in **minor units** of its currency (1999 EUR = 19.99 EUR). Never a quantity | `domain.Money`, `amount` + `currency` |

## Aggregates

`Order` (root, `domain/order.go`).

- An order requires a customer id (`Place` returns a validation error for an
  empty one).
- An order is only placed for a customer the customer context knows. This is
  checked by the command handler through `CustomerVerifier` **before** the
  aggregate is built, not by the aggregate. An unknown customer is
  `ErrNotFound`, so the API answers 404, not 400 or 422 (pinned by the
  `OrderREST` scenario).
- An order requires a product name that is not blank (trimmed before it is
  stored).
- An order requires an amount greater than zero, in a currency given as an
  ISO 4217 code of three uppercase letters (`domain.NewMoney`).
- The handler builds `ProductName` and `Money` **before** it calls the
  customer context, so invalid input never costs a cross-context call and is
  400 even for an unknown customer.
- `Place` records the domain event `OrderPlaced`. The handler saves the order
  and publishes it in one `UoW.Within`.

## Events published

| Event | When | Payload | Consumers |
|---|---|---|---|
| `order.v1.placed` | An order was placed (`OrderPlaced`, mapped in `infrastructure/messaging`) | `id`, `customer_id`, `product_name`, `amount` (int64, minor units), `currency`. Aggregate id = order id. Pinned by `tests/contract/testdata/order.v1.placed.golden.json` | customer (logs it only) |

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
- 2026-11-26: Amount is money as integer minor units plus a currency code.
  Why: exact arithmetic, and a number without a currency is ambiguous the
  moment a second currency exists. Rejected: a float price; a quantity.
- 2026-11-26: `product_name`, `amount` and `currency` are required in
  `order.v1.PlaceOrder`, with no default. A client that sends only
  `customer_id` now gets 400. Why: order has no external clients yet (status
  partial), and an order with no product or price means nothing. Rejected:
  defaulting missing fields; a `v2` RPC.
- 2026-11-26: Zero is not a valid amount. Why: an order placed for nothing is
  almost always a client bug. Revisit if free orders become a feature.

## Open questions

- The repository is always in-memory, even in the Postgres profile, so
  orders are lost on restart. `PlaceOrder` runs in `UoW.Within`, but neither
  the repository nor the (in-memory) outbox enlists in the SQL transaction.
  So when publishing fails, the order stays stored with no `order.v1.placed`,
  and a client retry places a second order with a new id (pinned by
  `TestPlaceOrder_PublishFailureFailsTheCommand`).
- One product per order. Order lines (several products, a quantity each, a
  total) are not modelled yet; `product_name` + `amount` would become one line.
- Should placing be refused for an `inactive` customer? Today only existence
  is checked.
- `PlaceOrder` over REST returns 200, not 201 (no `x-http-code` metadata).
  Deliberate or an omission? Pinned as-is by the `OrderREST` scenario.
