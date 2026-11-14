# payment context

Would own payment intents, captures and refunds. Status: **placeholder**.
Only a domain skeleton exists: no application layer, no persistence, no API,
not wired in `internal/bootstrap`.

Domain memory for this context. `tests/arch/context_memory_test.go` fails
when the aggregates or integration events below drift from the code. Update
it with the `update-memory` skill.

## Language

| Term | Meaning | In code |
|---|---|---|
| Payment | Money taken for one order | `domain.Payment`, `domain.PaymentID` |
| Amount | Minor units (cents) as an integer, plus a currency code. Never a float | `Payment.amountCents`, `Payment.currency` |
| Completed | The money was captured | `domain.PaymentCompleted` |

## Aggregates

`Payment` (root, `domain/payment.go`). Skeleton: fields (`orderID`,
`amountCents`, `currency`, `completedAt`) but no constructor or behaviour, so
no invariant is enforced yet. `PaymentID` must not be empty.

Domain event declared but never recorded: `PaymentCompleted`.

## Events published

None.

## Events consumed

None.

## Synchronous dependencies

None. A payment gateway would be the first real downstream HTTP service of
the app (see the `add-downstream-stub` skill and
`docs/agent/downstream-services.md`).

## Decisions

None yet.

## Open questions

- Planned integration events: `payment.v1.*` (from `doc.go`). Not mapped, not
  published.
- A payment refers to an order by id, but nothing starts a payment when an
  order is placed. Is that an event reaction to the planned order-placed
  event, or a call from order?
- One payment per order, or several (partial captures, retries, refunds)?
