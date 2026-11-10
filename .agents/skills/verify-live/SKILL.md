---
name: verify-live
description: Use when asked to test, verify, or prove that a change works end to end (not just unit tests), before claiming a fix or feature is done. Runs the real api and worker from this worktree in its own harness sandbox (shared Postgres, Kafka, mocked downstreams) with make harness-verify, runs the e2e scenarios and any scratch checks, and checks the five things that matter.
---

# Verify live

Full reference: `docs/agent/harness.md`. Always pass `GO=/snap/go/current/bin/go`.
Everything acts on this worktree's sandbox (named after the worktree
directory; `SANDBOX=<name>` overrides it). Other worktrees' sandboxes keep
running undisturbed.

## Run

```bash
make GO=/snap/go/current/bin/go harness-verify ARGS=--e2e                 # e2e + every _harness/checks/*.sh
make GO=/snap/go/current/bin/go harness-verify ARGS='--e2e my-task.sh'    # e2e + one check file
make GO=/snap/go/current/bin/go harness-verify ARGS='customer-'           # only checks matching a regex
```

After a code change with the sandbox already up, a faster loop is:
`make harness-run` (rebuilds and restarts the apps), then
`make harness-check ARGS='<regex>'` and/or `make harness-e2e`.

Exit codes: `0` PASS, `1` FAIL, `2` HARNESS ERROR. Exit 2 means nothing
was verified, so never report it as a test failure or as a pass.

## Look at five things, every time

Even when the result is PASS, check:

1. **Response**: `tmp/harness/<sandbox>/responses/<check>.json`.
2. **Outbound calls**: `make harness-mocks ARGS=requests`. Did the app call
   what it should, with the right body?
3. **Unmatched calls**: report section, or `make harness-mocks ARGS=unmatched`.
   This must be empty. If it isn't, run the `add-downstream-stub` skill.
4. **Events**: `make harness-events` (since the last check) or
   `ARGS='--all --raw'`. Check names, payloads, and nothing on `.retry`/`.dlq`.
5. **Logs**: `make harness-logs ARGS=errors`. This must be empty, or explained
   by `--expect-errors` or `_harness/known-log-noise.txt`.

## No test covers your change yet?

The regression test comes first (`docs/agent/testing.md`). A scratch check
in `_harness/checks/` (copy `example.sh.sample`) lets you watch the live
effects, but it is never committed and doesn't count as coverage.

## Ad-hoc poking

```bash
eval "$(_harness/scripts/sandboxes.sh env)"     # E2E_BASE_URL, DATABASE_URL, KAFKA_TOPIC_PREFIX, ...
curl -s "$E2E_BASE_URL/api/v1/customers?limit=5" | jq .
_harness/scripts/kafka.sh publish -name <ctx>.v1.<event> -aggregate-id <id> -payload '<json>'
make harness-mocks ARGS='scenario <svc> error'   # then reset: make harness-mocks ARGS=reset
docker compose -p myapp-harness -f _harness/docker-compose.yml exec -T postgres \
  psql -U myapp -d myapp_sb_<sandbox> -c 'select * from customer_reads limit 5'
```

## Report

Quote the report's `Result:` and `Sandbox:` lines, the e2e result and check
counts, and, for each claim, the command and the output that backs it. Leave
the sandbox running unless told otherwise. `make harness-down` stops it;
`ARGS=--purge` also drops its data and slot.
