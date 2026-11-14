---
name: update-memory
description: Use at the end of a task, or right after learning something a later task needs. Domain knowledge (a new or changed term, an aggregate invariant, an integration event published or consumed, a call into another context, a decision and its reason, an open question answered) goes into the bounded context's internal/<ctx>/CONTEXT.md. Anything surprising about tooling, config, tests, or the harness goes into docs/agent/learnings.md.
---

# Update memory

Two memories, both in git and reviewed like code. Pick by what you learned:

| You learned | It goes in |
|---|---|
| What a business term means, or that the code and the business use different words | `internal/<ctx>/CONTEXT.md` → Language |
| A rule an aggregate guards, or a rule enforced outside it (and where) | `CONTEXT.md` → Aggregates |
| An integration event the context emits or reacts to | `CONTEXT.md` → Events published / Events consumed |
| A call into another context | `CONTEXT.md` → Synchronous dependencies, and the Relationships table in `docs/agent/context-map.md` |
| Why something is the way it is, and what was rejected | `CONTEXT.md` → Decisions (cross-context: `context-map.md`) |
| A gap, a planned event, a rule nobody has decided | `CONTEXT.md` → Open questions |
| The same word meaning different things in two contexts | `context-map.md` → Same word, different meaning |
| A toolchain, config, startup, test, or harness trap | `docs/agent/learnings.md` |

## Domain memory (`CONTEXT.md`)

1. Read the context's whole file first. Edit the existing line instead of
   adding a second one that disagrees with it.
2. Write rules, not narrative: one line per invariant, with the method or
   file that guards it. State what is true now. History is in git.
3. A decision is one line:
   `- YYYY-MM-DD: <decision>. Why: <reason>. Rejected: <alternative>.`
   A decision that is reversed is edited, with the new date and reason.
4. An event that doesn't exist in the code yet is an open question, not a row
   under Events published or consumed. When the change answers an open
   question, remove it.
5. Ask the user when the code can't tell you (why a rule exists, what a term
   means to the business). Don't invent a reason: write the question under
   Open questions.
6. New context: copy [context-template.md](context-template.md) to
   `internal/<ctx>/CONTEXT.md` and add a row to `docs/agent/context-map.md`.
7. Run `go test -count=1 ./tests/arch/...`. `context_memory_test.go` checks
   the headings, the aggregates, the integration events (both directions),
   the 200-line cap, and the link from the context map. It can't check
   meanings, invariants, or reasons. Those are on you.

## Learnings (`docs/agent/learnings.md`)

Record:

- Behavior of tooling or infrastructure that contradicts its docs or naming.
- Data-shape traps (types in JSON payloads, nullable columns, id formats).
- Config and startup traps, with their fix.
- Harness quirks and slow or flaky areas, with the reason.

How:

1. Read the whole file first, and update an existing entry if one covers it.
2. Add one line under the right heading:
   `- <fact>. <why it matters / what to do>. (<file or command>, YYYY-MM-DD)`
3. Remove entries the code has made untrue.
4. If the file goes over about 150 lines, move stable topics into
   `docs/agent/architecture.md`, `harness.md`, or `downstream-services.md`.

## Don't record (in either)

- What you did (that's the commit message or PR).
- Anything obvious from reading the code: a field list, a file layout.
- Secrets, customer data, or anything from real env files.
- Near-duplicates. Edit the existing entry instead.

Mention every change in your final report (`MEMORY: ...`).
