# notification context

Owns telling people things (welcome messages today). Status: **consumer
slice**. It has no API and no stored state; it only reacts to other contexts'
integration events. The reference for an asynchronous cross-context consumer.

Domain memory for this context. `tests/arch/context_memory_test.go` fails
when the aggregates or integration events below drift from the code. Update
it with the `update-memory` skill.

## Language

| Term | Meaning | In code |
|---|---|---|
| Notification | A message sent to a person. The channel (email, SMS, push) is an infrastructure detail | `domain.NotificationService` |
| Welcome | The notification a person gets once, when they become a customer | `NotificationService.SendWelcome` |
| Sent | Today: a `welcome email sent` log line. Nothing is delivered | `infrastructure/email.LogSender` |

## Aggregates

None. There is no aggregate, repository or table. The domain layer is the
`NotificationService` port only.

## Events published

None.

## Events consumed

| Event | From | Reaction |
|---|---|---|
| `customer.v1.created` | customer | `SendWelcome(email, name)` (`application/handlers/customer_created_handler.go`). An undecodable payload returns an error, so the message goes to `.retry.1` and then the DLQ |

The handler declares its own copy of the event name and payload struct. The
coupling is the versioned contract, never the customer context's Go code.

## Synchronous dependencies

None.

## Decisions

Dates are when the decision was recorded here, not when it was made.

- 2026-11-14: The welcome is triggered by an event, not called by customer.
  Why: creating a customer must not wait for, or fail because of, a
  notification. Notification can be down without breaking customer creation.

## Open questions

- The handler is **not idempotent**: delivery is at least once, and a
  redelivered `customer.v1.created` sends the welcome again. Harmless while
  "send" is a log line; a real sender needs a dedupe on `Event.ID`.
- No record of what was sent, so nothing can answer "was this customer
  welcomed?".
- `customer.v1.email_changed` is ignored. Should a change be confirmed to the
  old and the new address?
