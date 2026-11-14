# customer context

Owns who a customer is: identity, name, email address and lifecycle status.
Status: **reference slice** (commands, CQRS read model, integration events).
Copy its shape for new work.

Domain memory for this context. `tests/arch/context_memory_test.go` fails
when the aggregates or integration events below drift from the code. Update
it with the `update-memory` skill.

## Language

| Term | Meaning | In code |
|---|---|---|
| Customer | A person registered with us. Identified by an opaque id, never by email | `domain.Customer`, `domain.CustomerID` |
| Email | A normalised address: trimmed and lowercased, so `Ada@X.com` and `ada@x.com` are the same email. Validation is deliberately loose (one `@`, a dot in the domain); delivery is the real validation | `domain.Email`, `domain.NewEmail` |
| Active | The state a customer is created in | `domain.StatusActive` (`"active"`) |
| Deactivated | The customer is disabled. There is no way back, and the email can no longer change | `domain.StatusDeactivated` (`"deactivated"`) |
| Inactive | The **read model's** word for deactivated. `customer_reads.status` and the API return `inactive`, while the write side (`customers.status`) stores `deactivated` | `projections/*_read_model.go` |
| Version | Optimistic-concurrency token of the aggregate. Starts at 1, +1 per state change. Not the event schema version (`v1`) | `Customer.Version()` |
| Read model | The projection queries are served from (`customer_reads`). Eventually consistent with the write side | `application.ReadModel` |

## Aggregates

`Customer` (root, `domain/customer.go`). All changes go through its methods,
which record the domain events. The clock is passed in by the application layer.

- Id is required. Name is 1 to 200 characters (`validateName`).
- **An email identifies at most one customer.** The aggregate can't see other
  customers, so this is the domain service `services.EmailUniqueness`, called
  by the command handlers inside `UoW.Within`. `customers_email_key` (unique
  index) is the backstop for a race. A taken email is `ErrConflict` (409).
- `ChangeEmail` on a deactivated customer is `ErrConflict`.
- `ChangeEmail` to the current email is a no-op: no event, no version bump.
- `Deactivate` is idempotent: the second call records nothing.
- A save with a version that is not higher than the stored one is
  `ErrConflict` (both repositories; pinned by `persistence/repotest`).

Domain events (`domain/events`): `CustomerCreated`, `CustomerEmailChanged`,
`CustomerDeactivated`. Each maps 1:1 to an integration event below.

## Events published

Mapped in `infrastructure/messaging/event_mappers.go`, written to the outbox
in the same unit of work as the aggregate. Payloads are pinned by
`tests/contract/testdata/<name>.golden.json`.

| Event | Emitted when | Payload |
|---|---|---|
| `customer.v1.created` | `NewCustomer` succeeds | `id`, `name`, `email` |
| `customer.v1.email_changed` | `ChangeEmail` to a different address | `id`, `old_email`, `new_email` |
| `customer.v1.deactivated` | First `Deactivate` | `id`, `reason` |

This context also subscribes to its own three events to build the read model
(`infrastructure/projections`). The projection is idempotent, and a
redelivered `created` must not revive an `inactive` row (pinned by
`projections/projectiontest`).

## Events consumed

| Event | From | Reaction |
|---|---|---|
| `order.v1.placed` | order | Placeholder: logs `customer context observed order.v1.placed`. No state change (`infrastructure/messaging/event_handlers.go`) |

## Synchronous dependencies

Calls none. Is called by **order**, which uses `GetCustomer` (served from the
read model) to verify a customer exists before placing an order.

## Decisions

Dates are when the decision was recorded here, not when it was made.

- 2026-11-14: Queries read only the read model, never the aggregate. Why: CQRS
  reference slice; the write model stays free of query concerns. Cost: a read
  right after a write can miss it.
- 2026-11-14: Email uniqueness is a domain service plus a unique index, not an
  aggregate rule. Why: the rule spans the whole collection, which one
  aggregate can't see.
- 2026-11-14: Integration payloads carry only ids and changed fields, with
  their own structs (`customerCreatedV1`), not the domain event. Why: the
  domain event can change without breaking the published contract.

## Open questions

- Nothing calls `Customer.Deactivate`: there is no command or RPC for it, so
  `customer.v1.deactivated` is mapped and pinned but never emitted in the
  running app.
- `order.v1.placed` has a consumer here but no producer (see
  `internal/order/CONTEXT.md`). What should the reaction be: mark the customer
  as "converted"?
- Should the read model say `deactivated` like the write side, instead of
  `inactive`? Changing it changes the API response.
