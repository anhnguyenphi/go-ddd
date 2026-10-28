# Downstream stubs

[WireMock](https://wiremock.org/docs/stubbing/) loads every `*.json` file in
this directory as stub mappings. The directory is mounted read-only into the
`wiremock` container. `make harness-up` (and `make harness-mocks ARGS=reset`)
reloads the files without a restart, and checks that the number of stubs
loaded matches the files.

Use one file per downstream service, named after its prefix: `payments.json`
holds the stubs for everything the app sends to
`${HARNESS_MOCK_URL}/payments/...`.

**There are no stubs today.** No bounded context makes an outbound HTTP call
yet, so any request WireMock receives is unmatched and fails the run.

A file holds one mapping, or `{"mappings": [...]}`:

```json
{
  "mappings": [
    {
      "name": "payments: create charge",
      "request": { "method": "POST", "urlPath": "/payments/v1/charges" },
      "response": {
        "status": 201,
        "jsonBody": { "id": "ch_harness_0001", "status": "succeeded" }
      }
    },
    {
      "name": "payments: get charge",
      "request": { "method": "GET", "urlPathPattern": "/payments/v1/charges/[^/]+" },
      "response": {
        "status": 200,
        "jsonBody": { "id": "ch_harness_0001", "status": "succeeded" }
      }
    }
  ]
}
```

- Give every mapping a `name`. Request listings and failure evidence show it
  as the route that matched.
- Match on `urlPath` (exact) or `urlPathPattern` (regex). Add
  `queryParameters`, `headers`, or `bodyPatterns` only when the response
  really depends on them.
- Copy response shapes from the provider's spec or the client's own types,
  and don't invent them. The `add-downstream-stub` skill covers the full
  procedure.
- Never call `/__admin/mappings/save`. The mount is read-only, and runtime
  stubs must not leak into these files.

Failure scenarios (`error`, `timeout`, `malformed`, `empty`) are not stub
files. They're priority-1 runtime stubs:
`make harness-mocks ARGS="scenario <svc> error"`, or `mock_scenario <svc> <kind>`
inside a check file. `make harness-mocks ARGS=reset` drops them.
