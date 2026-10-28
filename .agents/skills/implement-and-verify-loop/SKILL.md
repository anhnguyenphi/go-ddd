---
name: implement-and-verify-loop
description: Use for any task that changes behavior in this repo (a feature, a bug fix, a new endpoint, consumer, or migration). It orchestrates memory → checks first → implement → quality gate → live verify → review → memory, in a bounded loop, and finishes with evidence instead of a claim.
---

# Implement and verify loop

The default way to change behavior here. It works with subagents (preferred)
or single-handed. Then you play each role yourself, and the verifier's rule
still applies: don't fix code while you're reading the harness output.

## 0. Memory

Read `docs/agent/learnings.md` and the parts of
`docs/agent/architecture.md` that the task touches. Check whether the
problem is already recorded.

## 1. Plan, with checks first

- Restate the acceptance criteria as observable effects: the status and body,
  the rows written, the events published, the outbound calls made or not
  made, and the log lines.
- Write those as `check` lines in `_harness/smoke/checks.sh` (or an opt-in
  suite) **before** touching the code. Cover the happy path, at least one
  negative path, and the async side effect if there is one. See
  `docs/agent/harness.md` → "Writing checks".
- Run them against the current code: `make GO=/snap/go/current/bin/go harness-verify ARGS='<regex>'`.
  New behavior should FAIL now. For a bug, the failing check reproduces it.
  If a check passes before the change, it doesn't test the change.

## 2. Implement

Hand the `implementer` subagent one scoped task. Include the files, the
checks it must satisfy, and the constraints from AGENTS.md. Its prompt
must be self-contained. For disjoint pieces, several implementers can
run in parallel. Only the verifier runs the harness.

## 3. Quality gate

Run the `quality-gate` skill. If it fails, send the implementer back with the
fix list.

## 4. Verify live

Hand the `verifier` subagent the check names and what changed. It returns
PASS / FAIL / HARNESS ERROR with evidence.

- FAIL: go back to step 2 with the verifier's report verbatim (the observed
  vs expected values, the evidence, and the suspected file:line).
- HARNESS ERROR: fix the environment, not the code.

## 5. Review

Hand the `reviewer` subagent the changed files and the diff. If it reports
CHANGES REQUESTED, go back to step 2, then 3 and 4 again.

## 6. Memory

Run the `update-memory` skill for anything surprising you learned.

## Bounds

- At most **5** implement → verify iterations. Each one must act on **new
  evidence**. If the same failure shows up twice, stop editing. Inspect the
  system directly (`make harness-events ARGS=--raw`,
  `make harness-mocks ARGS=requests`, SQL, logs) until you understand it.
- After 5 iterations, stop and report what you know, what you tried, and the
  evidence.
- Never weaken a check to get green. If a check was wrong, fix it and say
  so in the final report.

## Final report to the user

```
RESULT: done | blocked
CHANGE: <one paragraph>
EVIDENCE: make harness-verify -> PASS (<n>/<n> checks, 0 unmatched, 0 unexpected errors)
          scripts/check.sh -> OK
NEW CHECKS: <names>
CHECKS CHANGED: <name: why it was wrong> | none
REVIEW: APPROVE (<minor notes>) 
MEMORY: <entries added/edited> | none
```
