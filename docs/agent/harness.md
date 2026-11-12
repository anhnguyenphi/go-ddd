# The harness: a live sandbox per feature

The harness gives every feature you're developing its own **sandbox**: the
real `api` and `worker` binaries, built from that worktree, running against
real Postgres and Redpanda (Kafka), with every downstream HTTP service faked
by WireMock. Many sandboxes run side by side, one per git worktree, so
parallel agents never step on each other.

It's the **agent's dev loop**, not the regression suite:

| | Harness sandbox (`_harness/`) | Regression (`tests/`, `scenarios`) |
|---|---|---|
| Purpose | Quickly see your change working live: the response, rows, events, outbound calls, log lines | Pin behavior so it never breaks again |
| Written as | Scratch `check` lines in `_harness/checks/*.sh` (gitignored) | Go tests (see [testing.md](testing.md)) |
| Committed | Never | Always, with the change |
| Run by CI | No. CI runs `tests/e2e` *in* a sandbox, plus one committed check file, the DSL tour (`example.sh.sample`), as the harness's own self-test | Yes |

So a behavior change ends with a Go test that pins it, **and** live proof in
your sandbox: `make harness-verify ARGS=--e2e` (the regression scenarios
against your real binaries) plus whatever scratch checks you needed to see the
side effects.

## Topology

```
                     shared infra: ONE per machine (compose project myapp-harness)
                     postgres :55432   redpanda :59092   wiremock :59999
                          │                 │                  │
     ┌────────────────────┼─────────────────┼──────────────────┼───────────────┐
     │ sandbox "feature_a"│ (worktree ../feature-a, slot 0)    │               │
     │   api :58100 / gRPC :58101, worker   (host processes, built from that worktree)
     │   db myapp_sb_feature_a   topics sb_feature_a.*   group myapp-sb-feature_a
     │   mocks /sb_feature_a/<svc>/...      state ../feature-a/tmp/harness/feature_a/
     ├─────────────────────────────────────────────────────────────────────────────┤
     │ sandbox "feature_b"  (worktree ../feature-b, slot 1): api :58110 / :58111 ...│
     └─────────────────────────────────────────────────────────────────────────────┘

registry, shared by every worktree: $(git rev-parse --git-common-dir)/myapp-harness/
    sandboxes/<name>.env (slot, worktree)   pids/<name>/   infra.lock
```

- **Sandbox name**: the worktree's directory name, slugged (`feature-a` →
  `feature_a`). A name longer than 24 characters keeps its first 19 and gets
  a 4-hex-digit hash of the whole name, so `…-v1` and `…-v2` stay distinct.
  Override it with `SANDBOX=<name>` (CI uses `ci`).
- **Slot**: the lowest free slot N gives api `58100+10N` and gRPC `58101+10N`.
  It's kept until `harness-down ARGS=--purge`.
- **Slice of the shared infra**: lib.sh derives everything from the name:
  `DATABASE_URL` (its own DB), `KAFKA_TOPIC_PREFIX=sb_<name>` (the app config
  puts it in front of every topic), `KAFKA_GROUP_ID`, and
  `HARNESS_MOCK_PREFIX=sb_<name>`. Downstream `<SVC>_URL`s point at
  `/sb_<name>/<svc>`, and the stubs in `_harness/mocks` are pushed once per
  sandbox with that prefix and tagged `metadata.harnessSandbox`.
- Every probe and tool is scoped to the sandbox: `harness-events` sees only its
  topics, `harness-mocks` only its requests, stubs and scenarios, and
  `harness-logs` only its apps. Breaking a downstream with `mock_scenario`
  affects no one else.
- Apps run on the host for fast rebuilds and native debugging. The infra is
  isolated from the dev stack (`make docker-up`): it has its own compose
  project, ports, and volume.

## Pipeline

`verify` = **lock → up → migrate → seed → run → [e2e] → check → report**. It
leaves everything running afterwards.

| Step | Script | Guarantees |
|---|---|---|
| lock | `lib.sh acquire_lock` | One harness command per sandbox (`tmp/harness/<name>/.lock`). Other sandboxes run freely |
| up | `up.sh` | Shared infra healthy (`infra.sh up`, under `infra.lock`), slot allocated, sandbox DB and topics (+ `.retry.1`, `.dlq`) exist, this sandbox's stubs replaced with exactly `_harness/mocks` (count checked) |
| migrate | `migrate.sh` | The production `cmd/migrate` on the sandbox DB. `down N` proves rollbacks |
| seed | `seed.sh` | `_harness/seed/*.sql`, idempotent, with fixed `harness-` ids |
| run | `run.sh` | api and worker rebuilt from this worktree and restarted detached (`setsid`), `/readyz` green, `worker started` logged, consumer group Stable. When the build gives the same binaries with the same env, the restart (about 20s) is skipped and a new run starts in the existing logs (`HARNESS_FORCE_RESTART=1` restarts anyway) |
| e2e | `e2e.sh` | With `--e2e`: `tests/e2e` (every scenario) against this sandbox. Exit 0 with no test run (`[no tests to run]`) is a FAIL |
| check | `check.sh` | Every `_harness/checks/*.sh` in this worktree, or the files/regex you pass. A file with a bash syntax error, a file that ran no check, or a regex that matched nothing is a FAIL line, not a quiet pass |
| report | `report.sh` | `tmp/harness/<name>/report.md`. **PASS** only when something behavioral ran (a check or e2e), all of it passed, e2e ran if `--e2e` asked for it, no request reached WireMock unstubbed (during checks, between them, or during e2e), and no unexpected ERROR log line exists. `Built from:` names the build the apps run; a **STALE** line means Go sources changed since then, so the result is about the old build |

Exit codes: `0` PASS, `1` FAIL (the code or a check is wrong), `2` HARNESS
ERROR (a setup step failed, so nothing was verified; fix the environment, not
the code).

## Commands

All of them act on the current worktree's sandbox (or `SANDBOX=<name>`).

```bash
make harness-verify ARGS=--e2e                   # everything + the regression scenarios
make harness-verify ARGS='my-feature.sh'         # only that check file; or a name regex
make harness-run                                 # rebuild + restart api/worker after a code change
make harness-check ARGS='customer-'              # re-run scratch checks against the running apps
make harness-e2e ARGS='-run TestScenarios/Customer'   # tests/e2e against the running apps
make harness-logs ARGS=errors                    # unexpected error lines across this sandbox's apps
make harness-logs ARGS='worker 200'              # tail one app
make harness-mocks ARGS=unmatched                # outbound calls with no stub
make harness-mocks ARGS='requests payments'      # what the app sent to one downstream
make harness-mocks ARGS='scenario payments timeout'   # error | timeout | malformed | empty
make harness-mocks ARGS=reset                    # drop scenarios, re-push stubs, clear journal
make harness-logs ARGS=wiremock                  # WireMock's own (shared) log
make harness-events                              # events since the last check started
make harness-events ARGS='--all --raw'           # everything on this sandbox's topics, full JSON
make harness-kafka ARGS=topics
_harness/scripts/kafka.sh publish -name customer.v1.created -aggregate-id x -payload '{"id":"x",...}'
make harness-migrate ARGS='down 1' && make harness-migrate ARGS=up
make harness-down                                # stop this sandbox's apps
make harness-down ARGS=--purge                   # also drop its DB, topics, stubs, slot, tmp dir
make harness-sandboxes                           # every sandbox on this machine
make harness-sandboxes ARGS=gc                   # purge sandboxes whose worktree is gone, and leftover state dirs
eval "$(_harness/scripts/sandboxes.sh env)"      # E2E_BASE_URL, DATABASE_URL, ... for ad-hoc tools
make harness-infra-down                          # stop the shared infra (refuses while sandboxes exist)
```

WireMock's admin API is at `http://localhost:59999/__admin`. It's shared, so
filter by your prefix (`/sb_<name>/`). In check files, `mock_scenario <svc> <kind>`
and `mock_clear_scenarios` (from `lib.sh`) break and restore a downstream for
this sandbox only. Stubs are pushed through the admin API, not mounted, so an
edited stub file needs `make harness-mocks ARGS=reset` (or `harness-up`).

`infra.sh` leaves a running stack alone, so an edit to
`_harness/docker-compose.yml` takes effect only after
`make harness-infra-down ARGS=--force` (when no other sandbox is in use) and
the next `make harness-up`.

Ad-hoc SQL against your sandbox DB:
`docker compose -p myapp-harness -f _harness/docker-compose.yml exec postgres psql -U myapp -d myapp_sb_<name>`.

## Parallel features

One feature, one worktree, one sandbox:

```bash
git worktree add ../myapp-feature-a -b feature-a
cd ../myapp-feature-a && make GO=/snap/go/current/bin/go harness-verify ARGS=--e2e   # sandbox feature_a
```

Subagents get the same thing with `isolation: worktree`. Each implementer can
run its own sandbox without coordinating with the others. When a worktree is
removed, `make harness-sandboxes ARGS=gc` (from any worktree) stops its apps
and drops its slice.

## Writing scratch checks

A check is your acceptance criterion made executable while you work. Write it
**before** the code and watch it fail. It lives in `_harness/checks/*.sh`
(gitignored). Start from `_harness/checks/example.sh.sample`. The DSL is
documented at the top of
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

How a check runs: it saves any unmatched call still in the journal (made
between checks, or by the e2e suite) for the report, then resets this
sandbox's mock journal, the event tap, and log offsets. It fires the one trigger. It polls the positive probes until all
of them hold or `--timeout` (15s) expires (a GET is re-sent on each poll). It
asserts the negative probes after a 1.5s settle delay. It always fails on
unmatched mock calls. With `--produce`, it also fails if the message landed on
a retry or DLQ topic, unless `--dead-letter` says that's the point.

Rules that keep checks useful:

- Use fresh ids per run (`$(new_id)`) and the seeded `harness-cust-000N` for fixtures.
- Make names unique and kebab-case, starting with the area (`customer-`,
  `order-`, `worker-`), so `ARGS='<regex>'` can select them.
- Assert on effects (rows, events, calls, logs), not only on the response.
- Errors a check causes on purpose go in `--expect-errors <regex>`. That
  excuses only the matching lines logged during that check. A truly harmless
  recurring line goes in `_harness/known-log-noise.txt`, with a comment saying
  why it's harmless.
- **When the behavior must stay fixed, turn the check into a Go test.** A
  scenario in `internal/testsupport/scenarios` for anything a client sees; a
  handler unit test for side effects; the adapter's `*_integration_test.go`
  for adapter behavior; `tests/contract` for event payloads. The check
  itself is never committed.

## When it fails

1. Read `tmp/harness/<name>/report.md`, then `tmp/harness/<name>/checks/<check>.txt`
   (the request, the response, the events, the mock requests, and the app log
   lines from that check), or `tmp/harness/<name>/e2e.txt`.
2. `make harness-logs ARGS=errors` shows handler failures and panics.
   A **STALE** line in the report means you edited Go code after the apps
   were built: `make harness-run`, then check again. `FAIL file:<name>` and
   `FAIL filter:<regex>` mean the check file or the regex is wrong (syntax
   error, no `check` reached, no name matched), not the app.
3. Two identical failures in a row mean you should stop editing and inspect:
   `make harness-events ARGS='--raw'`, `make harness-mocks ARGS=requests`, or SQL.
4. HARNESS ERROR (exit 2) means the environment is broken. Try
   `make harness-down ARGS=--purge`, then verify again. If a port is taken, the
   error names the stray process. "Sandbox X belongs to worktree Y" means two
   worktrees share a directory name: set `SANDBOX=<other-name>`.

## Extending it

- **New topic**: add it to `HARNESS_TOPICS` in `_harness/.env.harness`.
- **New downstream**: see the `add-downstream-stub` skill and
  `downstream-services.md`. Its URL is `http://localhost:59999/${HARNESS_MOCK_PREFIX}/<svc>`.
- **New binary**: build and start it in `run.sh`, add it to `HARNESS_APPS` in
  `lib.sh`, and add its log to `_check_new_logs` and `error_lines`.
- **New per-sandbox resource** (a cache, a bucket): derive its name from
  `$SANDBOX` in `lib.sh`, create it in `up.sh`, and drop it in
  `sandbox_purge_resources`.
- **Feature flags**: none exist yet. When they do, load them from a local
  fixture file in `_harness/fixtures/`, default all off, and never use a
  production dump.
