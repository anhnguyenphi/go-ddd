# identity context

Would own users, credentials, sessions and access policy. Status:
**placeholder**. Only a domain skeleton exists: no application layer, no
persistence, no API, not wired in `internal/bootstrap`.

Domain memory for this context. `tests/arch/context_memory_test.go` fails
when the aggregates or integration events below drift from the code. Update
it with the `update-memory` skill.

## Language

| Term | Meaning | In code |
|---|---|---|
| User | Someone who can authenticate. Not the same as a customer: a user has credentials, a customer has a commercial relationship. Nothing links the two yet | `domain.User`, `domain.UserID` |
| Registered | The user exists and has a credential | `domain.UserRegistered` |

## Aggregates

`User` (root, `domain/user.go`). Skeleton: fields (`email`, `passwordHash`,
`registeredAt`) but no constructor or behaviour, so no invariant is enforced
yet. `UserID` must not be empty.

Domain event declared but never recorded: `UserRegistered`.

## Events published

None.

## Events consumed

None.

## Synchronous dependencies

None.

## Decisions

None yet.

## Open questions

- Planned integration events: `identity.v1.*`, e.g. a user-registered event
  (from `doc.go`). Not mapped, not published.
- How does a `User` relate to a `Customer`? Same person, two contexts, no link
  today. Is the email the correlation, or an explicit id?
- The API has no authentication or authorization. This context is where it
  would come from.
