#!/usr/bin/env bash
# The full local gate: format check, vet, build, unit tests, arch tests.
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

echo "==> build"
"$GO" build ./...

echo "==> test"
"$GO" test -race -count=1 ./...

# Optional steps — only run if the tool is installed (see scripts/install-tools.sh).
if command -v golangci-lint >/dev/null; then
  echo "==> lint"
  golangci-lint run
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
