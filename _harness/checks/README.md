# Scratch checks

Put `*.sh` check files here while you develop a feature. `make harness-verify`
and `make harness-check` run every `*.sh` in this directory against **your
sandbox** (the one for this worktree). They're gitignored on purpose.

Checks are for **quickly testing your own implementation**: hit the endpoint,
then see the row, the event on Kafka, the outbound call to WireMock, and the
log line, all in one command. They are **not** regression tests:

- They're never committed, and CI never runs them.
- Behavior that must stay fixed gets a test in `tests/` or a scenario in
  `internal/testsupport/scenarios` (see `docs/agent/testing.md`). Run those
  against your sandbox with `make harness-verify ARGS=--e2e`.

CI does run one check file: the tour, `example.sh.sample`, as a self-test of
the DSL itself. Keep it passing when you change the DSL or the behavior it
shows.

Start from the tour: `cp example.sh.sample my-feature.sh`. The DSL reference
is at the top of `_harness/scripts/check_dsl.sh`.
