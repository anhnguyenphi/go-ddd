#!/usr/bin/env bash
# The full local gate: format check, vet, build, unit + arch tests, contract
# tests, lint, vuln. Integration tests (adapters against testcontainers) run in
# `make test-integration` and in CI, not here,
# so the gate stays fast and works without Docker.
set -euo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-go}"

echo "==> gofmt"
unformatted="$("$GO" fmt ./... )"
if [ -n "$unformatted" ]; then
  echo "reformatted: $unformatted"
fi

echo "==> vet"
"$GO" vet ./...
"$GO" vet -tags=integration,contract,e2e ./...

echo "==> build"
"$GO" build ./...

echo "==> test"
"$GO" test -race -count=1 ./...

echo "==> contract"
"$GO" test -count=1 -tags=contract ./tests/contract/...

# Optional steps — only run if the tool is installed (see scripts/install-tools.sh).
if command -v golangci-lint >/dev/null; then
  echo "==> lint"
  golangci-lint run --build-tags=integration,contract,e2e
else
  echo "==> lint (skipped: golangci-lint not installed — run 'make tools')"
fi

if command -v govulncheck >/dev/null; then
  echo "==> vuln"
  govulncheck ./...
else
  echo "==> vuln (skipped: govulncheck not installed — run 'make tools')"
fi

echo "OK"
