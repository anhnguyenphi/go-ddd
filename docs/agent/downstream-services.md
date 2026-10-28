# Downstream services

Every external service the app calls over the network, with how the harness
fakes it. Keep this table in sync with the code. A downstream that's missing
here is a downstream with no stub, and its calls will fail the harness as
unmatched.

| Service | Called from | Env var (harness value) | WireMock prefix | Stub file | Failure behavior |
|---|---|---|---|---|---|
| *(none yet)* | | | | | |

No bounded context makes an outbound HTTP call today. `order` → `customer`
is an in-process gRPC call inside the monolith, not a downstream.

## Adding one

Follow the `add-downstream-stub` skill. In short:

1. Give the client's base URL an env var (`<SVC>_URL`) in `internal/platform/config`.
2. In `_harness/.env.harness`, set `<SVC>_URL=http://localhost:59999/<svc>`.
3. Stub every call the happy paths make in `_harness/mocks/<svc>.json`, with
   shapes taken from the provider's spec or the client's types.
4. Add a row above, with the failure behavior the code implements
   (error, timeout, retries).
5. Add smoke checks with `--calls 'METHOD /<svc>/...'`, plus at least one
   failure check that uses `make harness-mocks ARGS="scenario <svc> error"`.
