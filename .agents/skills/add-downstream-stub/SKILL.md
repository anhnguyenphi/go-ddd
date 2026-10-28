---
name: add-downstream-stub
description: Use when code starts calling an external HTTP service (a payment gateway, an identity provider, any other team's API), or when the harness report lists unmatched downstream requests. It reads the client's contract, writes WireMock stubs, wires the env var, and adds call and failure checks.
---

# Add a downstream stub

Every outbound HTTP call in the harness goes to WireMock, with one path
prefix per downstream service. A call that matches no stub fails the run.
That's deliberate: a new dependency can't slip in unnoticed.

## Steps

1. **Contract**: read the client code you're calling the service with (base
   URL, paths, methods, request and response types, timeouts, retries). Take
   response shapes from the provider's OpenAPI or proto spec, or from the
   client's own response structs. Don't invent fields.
2. **Config**: the client's base URL must come from config (an env var
   `<SVC>_URL` in `internal/platform/config`), never a hard-coded host.
3. **Harness env**: in `_harness/.env.harness`, add
   `<SVC>_URL=http://localhost:59999/<svc>` (a literal URL: `HARNESS_MOCK_URL` is defined further down the file).
   Use only fake credentials, marked as fake (`harness-fake-...`).
4. **Stubs**: create `_harness/mocks/<svc>.json`. The format is in
   `_harness/mocks/README.md` (WireMock mappings). Paths include the prefix
   (`/<svc>/v1/...`): use `urlPath` for an exact path, or `urlPathPattern`
   (a regex) for ids. Give each stub a `name`. Stub every call
   the happy paths make, using the fixture ids from `_harness/seed` where the
   response refers to them.
5. **Registry**: add a row to `docs/agent/downstream-services.md`.
6. **Checks**:
   - The happy path asserts the call: `--calls 'POST /<svc>/v1/...'`.
   - Validation and auth failures assert there was no call: `--no-calls <svc>`.
   - At least one failure path: put the scenario in its own check file or
     around a check in an opt-in suite:
     ```bash
     mock_scenario <svc> error
     check --name <thing>-<svc>-down --method POST --path ... --status 502 --no-events
     mock_clear_scenarios
     ```
     Available kinds: `error` (500), `timeout` (hangs 30s), `malformed`
     (invalid JSON), and `empty` (`[]`).
7. Run `make harness-up` (it reloads the stubs and checks the count), then
   `verify-live`. The "Unmatched downstream requests" section must say None.

Make sure client retries are short or disabled under the harness env, so a
failure check fails fast.
