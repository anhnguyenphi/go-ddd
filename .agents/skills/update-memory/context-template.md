# <ctx> context

<What this context owns, in one sentence.> Status: **<reference slice |
partial | consumer slice | placeholder>**. <What exists and what doesn't.>

Domain memory for this context. `tests/arch/context_memory_test.go` fails
when the aggregates or integration events below drift from the code. Update
it with the `update-memory` skill.

## Language

Only terms with a domain meaning, especially ones that differ from everyday
use or from another context's use of the same word.

| Term | Meaning | In code |
|---|---|---|
| <Term> | <What it means here, and what it does not mean> | `<type or function>` |

## Aggregates

`<Aggregate>` (root, `domain/<file>.go`). One line per invariant, with what
guards it. Say so when a rule is enforced outside the aggregate.

- <Rule>. (`<method>`)

## Events published

Integration events only, as whole names in backticks. "None." when empty.

| Event | Emitted when | Payload |
|---|---|---|
| `<ctx>.v1.<event>` | <trigger> | <fields>; pinned by `tests/contract` |

## Events consumed

| Event | From | Reaction |
|---|---|---|
| `<other>.v1.<event>` | <context> | <what happens, and how it is idempotent> |

## Synchronous dependencies

Calls into other contexts or downstream services, and who calls this one.

## Decisions

- YYYY-MM-DD: <decision>. Why: <reason>. Rejected: <alternative>.

## Open questions

- <A gap, a planned event, a rule nobody has decided.>
