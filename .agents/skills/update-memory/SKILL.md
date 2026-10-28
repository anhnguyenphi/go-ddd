---
name: update-memory
description: Use at the end of a task, or right after discovering something surprising (behavior that contradicts docs or naming, a config or startup trap, a harness quirk), to record it in docs/agent/learnings.md so the next task starts smarter.
---

# Update memory

Shared memory is `docs/agent/learnings.md`. It's in git and reviewed like
code, and every agent reads it before every task.

## Record

- Behavior that contradicts the docs, the specs, or the naming.
- Data-shape traps (types in JSON payloads, nullable columns, id formats).
- Config and startup traps, with their fix.
- Harness quirks and slow or flaky areas, with the reason.

## Don't record

- What you did (that's the commit message or PR).
- Anything obvious from reading the code.
- Secrets, customer data, or anything from real env files.
- Near-duplicates. Edit the existing entry instead.

## How

1. Read the whole file first, and update an existing entry if one covers it.
2. Add one line under the right heading:
   `- <fact>. <why it matters / what to do>. (<file or command>, YYYY-MM-DD)`
3. Remove entries the code has made untrue.
4. If the file goes over about 150 lines, move stable topics into
   `docs/agent/architecture.md`, `harness.md`, or `downstream-services.md`.
5. Mention the change in your final report (`MEMORY: ...`).
