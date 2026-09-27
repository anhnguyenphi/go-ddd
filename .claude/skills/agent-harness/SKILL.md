---
name: agent-harness
description: Use when asked to test, verify, or validate a change to this repo end-to-end (not just unit tests) — before claiming a fix or feature works, or before running the full regression gate. Brings up a real Postgres+Kafka stack and hands you composable tools (agentctl, mockapi) to check ANY feature, not one fixed scenario.
---

# Agent self-test harness

`scripts/agent/` validates changes against a real running system (Postgres,
Kafka/Redpanda, the `api` and `worker` processes) instead of only in-memory
unit tests. It is generic — it hands you a running stack and tools, not one
hardcoded test — so use it for whatever feature you're actually changing.

**Toolchain gotcha**: the snap `go` shim prints nothing here. Pass
`GO=/snap/go/current/bin/go` to every `make` command below (or
`export PATH=/snap/go/current/bin:$PATH` once per shell).

## Which command

- **Wrapping up a change, want the full regression gate** (mocks, lint,
  build, unit+arch tests, vuln, integration tests, HTTP e2e)?
  ```
  make GO=/snap/go/current/bin/go agent-verify
  ```
  Skip stages while iterating: `ARGS="--skip-mocks --skip-integration --skip-e2e"`.
  Leave the stack up afterward for follow-up poking: `ARGS="--keep-up"`.

- **Want to verify one specific feature interactively** (a new Kafka
  consumer, a new outbound call, a new endpoint)?
  ```
  make GO=/snap/go/current/bin/go agent-stack-up
  ```
  ...drive it directly (see below)...
  ```
  make agent-stack-down
  ```

`stack-up.sh` is idempotent (calling it again while already up just prints
the same summary and exits). `stack-down.sh` is a safe no-op if nothing is
up. **Always tear down when done** — a stray process left on one of these
ports from a previous run can make the next `stack-up.sh` either fail, or
(worse) silently pass its health checks against the wrong, stale process.

## Driving the stack

`stack-up.sh` prints a summary with exact URLs and log paths on every run.
In short:

- REST API: `http://localhost:8080` (gRPC: `localhost:9090`)
- Mock external-API double: `http://localhost:9999`
- Postgres: `postgres://myapp:myapp@localhost:5432/myapp?sslmode=disable`
- Kafka: `localhost:9092`
- Logs: `/tmp/myapp-agent-stack/logs/{api,worker,mockapi}.log`

**Trigger or inspect any integration event** (any bounded context, any event
name — not just `customer.v1.created`) with `./bin/agentctl`:

```bash
./bin/agentctl kafka publish -name <ctx>.v1.<event> -aggregate-id <id> \
  -payload '<json>'           # byte-identical wire format to the real app
./bin/agentctl kafka consume -name <ctx>.v1.<event> -aggregate-id <id> \
  -timeout 10s                 # own reader — never joins the app's consumer group
```

**Simulate an external API call** your feature makes, and inspect what it
received, with `./bin/mockapi` (already running as part of the stack):

```bash
curl -X POST http://localhost:9999/__routes \
  -d '[{"method":"POST","path":"/v1/...","status":200,"body":{...}}]'
curl http://localhost:9999/v1/...             # your code calls this
curl http://localhost:9999/__requests         # what it actually received
curl -X POST http://localhost:9999/__reset    # clear between checks
```

**Run the HTTP e2e suite** (or a test you write) against the stack:

```bash
E2E_BASE_URL=http://localhost:8080 go test -tags=e2e ./tests/e2e/...
```

See `scripts/agent/examples/kafka-roundtrip.sh` for a full worked pattern —
publish an event, assert both the read-model projection and a downstream
consumer reacted — and copy/adapt it rather than writing a check from
scratch.

## Cleanup

Run `make agent-stack-down` (or `scripts/agent/stack-down.sh`) when finished,
including after a failed check — it stops the tracked api/worker/mockapi
processes and runs `docker compose down -v`, unless the stack was started
with `--skip-docker`.
