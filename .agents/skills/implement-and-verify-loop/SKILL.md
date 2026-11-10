---
name: implement-and-verify-loop
description: Use for any task that changes behavior in this repo (a feature, a bug fix, a new endpoint, consumer, or migration). It orchestrates memory → failing tests + scratch checks first → implement → quality gate → live verify in the worktree's sandbox → review → memory, in a bounded loop, and finishes with evidence instead of a claim.
---

# Implement and verify loop

The default way to change behavior here. It works with subagents (preferred)
or single-handed. Then you play each role yourself, and the verifier's rule
still applies: don't fix code while you're reading the harness output.

Two kinds of tests, two jobs:

- **Regression** (`tests/`, `internal/testsupport/scenarios`): Go tests,
  committed, run by CI. They pin the behavior.
- **Scratch checks** (`_harness/checks/*.sh`, gitignored): your quick look at
  the live system (rows, events, outbound calls, logs) in this worktree's
  harness sandbox. Never committed, never a substitute for a test.

## 0. Memory and workspace

Read `docs/agent/learnings.md` and the parts of
`docs/agent/architecture.md` that the task touches. Check whether the
problem is already recorded.

Work in a dedicated git worktree when other features are in flight. The
harness gives each worktree its own sandbox (`docs/agent/harness.md`), so
parallel tasks never share ports, DBs, topics, or mocks.

## 1. Plan, tests first

- Restate the acceptance criteria as observable effects: the status and body,
  the rows written, the events published, the outbound calls made or not
  made, and the log lines.
- **Regression tests**, written before the code (see
  `docs/agent/testing.md` → "Where does my test go?"): a scenario for anything
  a client sees, `tests/integration` for side effects
  (`testsupport.WithLogCapture` for log-only ones), `tests/contract` for event
  payloads, and unit tests for domain and handler rules. Run them: they must
  FAIL now. For a bug, the failing test reproduces it.
- **Scratch checks** (optional, but cheap): `check` lines in
  `_harness/checks/<task>.sh` for the effects you want to watch live,
  especially ones tests can't easily see (a real Kafka round trip, the call
  sent to WireMock). Run them:
  `make GO=/snap/go/current/bin/go harness-verify ARGS='_harness/checks/<task>.sh'`.

## 2. Implement

Hand the `implementer` subagent one scoped task. Include the files, the tests
and checks it must satisfy, and the constraints from AGENTS.md. Its prompt
must be self-contained. For disjoint pieces, run several implementers in
parallel, each in its own worktree (`isolation: worktree`). Each may use its
own sandbox (`make harness-run`, `make harness-check`) for quick feedback.

## 3. Quality gate

Run the `quality-gate` skill. If it fails, send the implementer back with the
fix list.

## 4. Verify live

Hand the `verifier` subagent the worktree, the check file(s), and what
changed. It runs `make harness-verify ARGS=--e2e` in that worktree's sandbox
and returns PASS / FAIL / HARNESS ERROR with evidence.

- FAIL: go back to step 2 with the verifier's report verbatim (the observed
  vs expected values, the evidence, and the suspected file:line).
- HARNESS ERROR: fix the environment, not the code.

## 5. Review

Hand the `reviewer` subagent the changed files and the diff. If it reports
CHANGES REQUESTED, go back to step 2, then 3 and 4 again.

## 6. Memory and cleanup

Run the `update-memory` skill for anything surprising you learned. When the
worktree is done, `make harness-down ARGS=--purge` there (or
`make harness-sandboxes ARGS=gc` after removing the worktree).

## Bounds

- At most **5** implement → verify iterations. Each one must act on **new
  evidence**. If the same failure shows up twice, stop editing. Inspect the
  system directly (`make harness-events ARGS=--raw`,
  `make harness-mocks ARGS=requests`, SQL, logs) until you understand it.
- After 5 iterations, stop and report what you know, what you tried, and the
  evidence.
- Never weaken a test or check to get green. If one was wrong, fix it and say
  so in the final report.

## Final report to the user

```
RESULT: done | blocked
CHANGE: <one paragraph>
EVIDENCE: make harness-verify ARGS=--e2e -> PASS (sandbox <name>: e2e ok, <n>/<n> checks, 0 unmatched, 0 unexpected errors)
          scripts/check.sh -> OK
          make test-integration -> ok (if adapters/wiring/bus touched)
NEW TESTS: <test/scenario names>
TESTS CHANGED: <name: why it was wrong> | none
REVIEW: APPROVE (<minor notes>)
MEMORY: <entries added/edited> | none
```
