# The harness

`make harness-verify` runs the real `api` and `worker` binaries, built from
the working tree, against real Postgres and Redpanda (Kafka), with every
downstream HTTP service faked by WireMock. It then runs smoke checks that
assert on what actually happened: the HTTP response, outbound calls,
published events, DB rows, and log lines. The result is one exit code and
one report.

Unit and integration tests cover each box with mocks. The harness covers the
wiring between the boxes: env vars and startup, SQL against the real schema,
topic names and payloads, consumer groups, and retries and DLQ.

## Topology

```
make harness-* ──> _harness/scripts/*.sh                        tmp/harness/
                       │                                          report.md, results.tsv
                       │ builds from the working tree             api.log worker.log
                       ▼                                          responses/<check>.json
  host processes:  api :58080 / gRPC :59090    worker             checks/<check>.txt (failures)
                       │                                          bin/  pids/  .lock
  docker compose (project myapp-harness):
                   postgres :55432 (myapp_harness)   redpanda :59092
                   wiremock :59999 (one prefix per downstream; stubs pushed from _harness/mocks)
```

Why the apps run on the host: fast rebuilds, no image build per iteration,
and native debugging. Isolation: the harness has its own compose project,
ports (5xxxx), and volume, so it never touches the dev stack from
`make docker-up`.

## Pipeline

`verify` = **lock → up → migrate → seed → run → smoke → report**. It leaves
everything running afterwards.

| Step | Script | Guarantees |
|---|---|---|
| lock | `lib.sh acquire_lock` | One harness command per checkout (`tmp/harness/.lock`). A stale lock (dead pid) is taken over |
| up | `up.sh` | Postgres and Redpanda healthy; topics `$HARNESS_TOPICS` (+ `.retry.1`, `.dlq`) exist; WireMock stubs replaced with exactly `_harness/mocks` (scenarios dropped, count checked) |
| migrate | `migrate.sh` | The production `cmd/migrate`. `down N` proves rollbacks |
| seed | `seed.sh` | `_harness/seed/*.sql`, idempotent, with fixed `harness-` ids |
| run | `run.sh` | api and worker rebuilt, restarted detached (`setsid`), `/readyz` green, `worker started` logged, consumer group Stable |
| smoke | `smoke.sh` | `_harness/smoke/checks.sh`, plus opt-in `checks_<suite>.sh` |
| report | `report.sh` | `tmp/harness/report.md`. **PASS** only when every check passed, no request reached WireMock unstubbed, and no unexpected ERROR log line exists |

Exit codes: `0` PASS, `1` FAIL (the code or a check is wrong), `2` HARNESS
ERROR (a setup step failed, so nothing was verified; fix the environment, not
the code).

## Commands

```bash
make harness-verify                              # everything; ARGS='<regex>' or '--suite resilience'
make harness-smoke ARGS='customer-'              # re-run checks against the running apps
make harness-run                                 # rebuild + restart api/worker after a code change
make harness-logs ARGS=errors                    # unexpected error lines across apps
make harness-logs ARGS='worker 200'              # tail one app
make harness-mocks ARGS=unmatched                # outbound calls with no stub
make harness-mocks ARGS='requests payments'      # what the app sent to one downstream
make harness-mocks ARGS='scenario payments timeout'   # error | timeout | malformed | empty
make harness-mocks ARGS=reset                    # drop scenarios, re-push stubs, clear journal
make harness-logs ARGS=wiremock                  # WireMock's own log (stub load errors)
make harness-events                              # events since the last check started
make harness-events ARGS='--all --raw'           # everything on the topics, full JSON
make harness-kafka ARGS=topics
_harness/scripts/kafka.sh publish -name customer.v1.created -aggregate-id x -payload '{"id":"x",...}'
make harness-migrate ARGS='down 1' && make harness-migrate ARGS=up
make harness-down                                # stop; ARGS=--purge drops data, logs, report
```

WireMock's admin API is at `http://localhost:59999/__admin` (for example
`/__admin/requests`, `/__admin/mappings`). In check files, `mock_scenario <svc> <kind>`
and `mock_clear_scenarios` (from `lib.sh`) break and restore a downstream.
Stubs are pushed from `_harness/mocks` through the admin API, not mounted,
so an edited stub file needs `make harness-mocks ARGS=reset` (or `harness-up`).

Ad-hoc SQL against the harness DB:
`docker compose -p myapp-harness -f _harness/docker-compose.yml exec postgres psql -U myapp -d myapp_harness`.

## Writing checks

A check is an acceptance criterion. Write it **before** the code, and watch
it fail. The DSL is documented at the top of
[_harness/scripts/check_dsl.sh](../../_harness/scripts/check_dsl.sh).

```bash
# Happy path: response, DB state on both sides, the event, and its consumer.
email="create-$(new_id)@harness.test"
check --name customer-create-happy --method POST --path /api/v1/customers \
	--body "{\"name\":\"Harness Create\",\"email\":\"$email\"}" \
	--status 201 --jq '.id | length > 0' --save customer_id=.id \
	--events customer.v1.created \
	--await "SELECT status FROM customer_reads WHERE email = '$email'" active \
	--logs "\"msg\":\"welcome email sent\".*\"to\":\"$email\""

# Negative path: validation stops the request before anything is published.
check --name customer-create-invalid-email --method POST --path /api/v1/customers \
	--body '{"name":"x","email":"not-an-email"}' --status 400 --no-events

# Async: produce an event as another producer would, then assert the side effect.
id="$(new_id)"
check --name worker-customer-created-projects-and-notifies \
	--produce customer.v1.created --key "$id" \
	--body "{\"id\":\"$id\",\"name\":\"P\",\"email\":\"p-$id@harness.test\"}" \
	--await "SELECT email FROM customer_reads WHERE id = '$id'" "p-$id@harness.test"
```

How a check runs: it resets the mock journal, the event tap, and log offsets.
It fires the one trigger. It polls the positive probes until all of them hold
or `--timeout` (15s) expires (a GET is re-sent on each poll). It asserts the
negative probes after a 1.5s settle delay. It always fails on unmatched mock
calls. With `--produce`, it also fails if the message landed on a retry or DLQ
topic, unless `--dead-letter` says that's the point.

Rules that keep checks reliable:

- Use fresh ids per run (`$(new_id)`) and the seeded `harness-cust-000N` for fixtures.
- Make names unique and kebab-case, starting with the area (`customer-`,
  `order-`, `worker-`), so `ARGS='<regex>'` can select them.
- Assert on effects (rows, events, calls, logs), not only on the response.
- Errors a check causes on purpose go in `--expect-errors <regex>`. That
  excuses only the matching lines logged during that check. A truly harmless
  recurring line goes in `_harness/known-log-noise.txt`, with a comment saying
  why it's harmless.
- Put flag-on paths and checks that break things on purpose in an opt-in suite
  (`_harness/smoke/checks_<name>.sh`, run with `--suite <name>`).

## When it fails

1. Read `tmp/harness/report.md`, then `tmp/harness/checks/<name>.txt`. The
   evidence file has the request, the response, the events, the mock requests,
   and the app log lines from that check.
2. `make harness-logs ARGS=errors` shows handler failures and panics.
3. Two identical failures in a row mean you should stop editing and inspect:
   `make harness-events ARGS='--raw'`, `make harness-mocks ARGS=requests`, or SQL.
4. HARNESS ERROR (exit 2) means the environment is broken. Try
   `make harness-down ARGS=--purge`, then verify again. If a port is taken, the
   error names the stray process.

## Extending it

- **New topic**: add it to `HARNESS_TOPICS` in `_harness/.env.harness`.
- **New downstream**: see the `add-downstream-stub` skill and
  `downstream-services.md`.
- **New binary**: build and start it in `run.sh`, add it to `HARNESS_APPS` in
  `lib.sh`, and add its log to `_check_new_logs` and `error_lines`.
- **Feature flags**: none exist yet. When they do, load them from a local
  fixture file in `_harness/fixtures/`, default all off, and never use a
  production dump.
