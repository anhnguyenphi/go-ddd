---
name: verify-live
description: Use when asked to test, verify, or prove that a change works end to end (not just unit tests), before claiming a fix or feature is done. Runs the real api and worker against Postgres, Kafka, and mocked downstreams with make harness-verify, and checks the five things that matter. Use it for any feature, not one fixed scenario.
---

# Verify live

Full reference: `docs/agent/harness.md`. Always pass `GO=/snap/go/current/bin/go`.

## Run

```bash
make GO=/snap/go/current/bin/go harness-verify                   # all default checks
make GO=/snap/go/current/bin/go harness-verify ARGS='customer-'  # only matching checks
make GO=/snap/go/current/bin/go harness-verify ARGS='--suite resilience'
```

After a code change with the harness already up, a faster loop is:
`make harness-run` (rebuilds and restarts the apps), then
`make harness-smoke ARGS='<regex>'`.

Exit codes: `0` PASS, `1` FAIL, `2` HARNESS ERROR. Exit 2 means nothing
was verified, so never report it as a test failure or as a pass.

## Look at five things, every time

Even when the result is PASS, check:

1. **Response**: `tmp/harness/responses/<check>.json`.
2. **Outbound calls**: `make harness-mocks ARGS=requests`. Did the app call
   what it should, with the right body?
3. **Unmatched calls**: report section, or `make harness-mocks ARGS=unmatched`.
   This must be empty. If it isn't, run the `add-downstream-stub` skill.
4. **Events**: `make harness-events` (since the last check) or
   `ARGS='--all --raw'`. Check names, payloads, and nothing on `.retry`/`.dlq`.
5. **Logs**: `make harness-logs ARGS=errors`. This must be empty, or explained
   by `--expect-errors` or `_harness/known-log-noise.txt`.

## No check covers your change yet?

Add one to `_harness/smoke/checks.sh` first (see "Writing checks" in
`docs/agent/harness.md`), run it, and see it fail before the fix. A
behavior change without a check isn't done (AGENTS.md, Definition of done #4).

## Ad-hoc poking

```bash
curl -s localhost:58080/api/v1/customers?limit=5 | jq .
_harness/scripts/kafka.sh publish -name <ctx>.v1.<event> -aggregate-id <id> -payload '<json>'
make harness-mocks ARGS='scenario <svc> error'   # then reset: make harness-mocks ARGS=reset
docker compose -p myapp-harness -f _harness/docker-compose.yml exec -T postgres \
  psql -U myapp -d myapp_harness -c 'select * from customer_reads limit 5'
```

## Report

Quote the report's `Result:` line, the check counts, and, for each claim,
the command and the output that backs it. Leave the harness running unless
told otherwise. `make harness-down` stops it.
