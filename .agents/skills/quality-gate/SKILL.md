---
name: quality-gate
description: Use after implementing a change and before live verification, or before declaring any code change done. Runs build, vet, unit and arch tests, lint, vuln, generated-code freshness, and the contract and integration suites, and turns the output into a concrete fix list.
---

# Quality gate

```bash
export PATH=/snap/go/current/bin:$PATH
```

## 1. Generated code is fresh

If you touched any of these, regenerate, then check that `git status` shows
the regenerated files:

| Touched | Run |
|---|---|
| `api/proto/**/*.proto` | `make proto && make proto-lint` (needs `buf`; `make tools`) |
| An interface listed in `.mockery.yaml`, or a new one to mock | `make mocks` |
| `go.mod` | `go mod tidy` |

Generated files are never hand-edited.

## 2. The local gate

```bash
GO=/snap/go/current/bin/go scripts/check.sh
```

This runs gofmt (it rewrites files, so review the diff), vet (including the
tagged suites), build, `go test -race ./...` (which includes the `tests/arch`
layering rules), the contract suite, the in-memory integration profile,
golangci-lint, and govulncheck. The last two are skipped if they aren't
installed. Say so in your report if they were skipped.

## 3. Infra integration

```bash
make GO=/snap/go/current/bin/go test-integration
```

Run this when you touched an adapter (SQL, Kafka), a migration, wiring in
`internal/bootstrap`, or the event bus. It starts Postgres and Redpanda via
testcontainers and runs the shared scenarios on every profile. If it reports
the infra tests as skipped, Docker wasn't reachable: say so, don't call it a
pass.

A contract failure (in `check.sh`) means you broke a published contract.
Don't "fix" the golden file. Make the change additive (then
`make test-contract ARGS=-update`) or add a `v2` event.

## 4. Turn failures into a fix list

For each failure, write one line: `file:line — what failed — the smallest fix`.
Report it like this:

```
GATE: PASS | FAIL
SKIPPED: <tools not installed, or "none">
FIX LIST:
  - internal/customer/...:42 — TestX: want 409 got 500 — map ErrConflict in mapError
```
