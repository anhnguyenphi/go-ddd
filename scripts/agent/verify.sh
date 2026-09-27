#!/usr/bin/env bash
# The single entrypoint for an agent to self-validate a change: regenerate
# mocks, run the full local gate (fmt/vet/build/unit+arch tests/lint/vuln),
# integration tests, then bring up the full stack and run the HTTP e2e suite
# against it. Each stage can be skipped for a faster inner loop.
#
# This script only runs the generic suites (tests/integration, tests/e2e) —
# it has no opinion about which feature you're changing. For a feature-
# specific check (a particular Kafka event, a particular external call via
# mockapi, ...), bring the stack up yourself with scripts/agent/stack-up.sh
# and drive it directly; see scripts/agent/examples/ for a worked pattern.
#
# Usage:
#   scripts/agent/verify.sh [--skip-mocks] [--skip-integration] [--skip-e2e] [--keep-up]
#
#   --keep-up  leave the stack up after the e2e stage (success or failure)
#              instead of tearing it down, for follow-up poking.
#
# Env:
#   GO   path to the real go binary (default: go — see CLAUDE.md's snap gotcha)
set -euo pipefail
cd "$(dirname "$0")/../.."
source scripts/agent/lib.sh

GO="${GO:-go}"
SKIP_MOCKS=false
SKIP_INTEGRATION=false
SKIP_E2E=false
KEEP_UP=false

for arg in "$@"; do
	case "$arg" in
		--skip-mocks) SKIP_MOCKS=true ;;
		--skip-integration) SKIP_INTEGRATION=true ;;
		--skip-e2e) SKIP_E2E=true ;;
		--keep-up) KEEP_UP=true ;;
		*) die "unknown flag: $arg" ;;
	esac
done

if ! $SKIP_MOCKS; then
	if command -v mockery >/dev/null; then
		log "regenerating mocks"
		mockery
		"$GO" mod tidy
	else
		log "mocks (skipped: mockery not installed — run 'make tools')"
	fi
fi

log "full local gate (fmt, vet, build, unit + arch tests, lint, vuln)"
GO="$GO" scripts/check.sh

if ! $SKIP_INTEGRATION; then
	log "integration tests"
	"$GO" test -race -count=1 -tags=integration ./tests/integration/...
fi

if ! $SKIP_E2E; then
	log "bringing up the stack for the HTTP e2e suite"
	scripts/agent/stack-up.sh
	if ! $KEEP_UP; then
		trap 'scripts/agent/stack-down.sh' EXIT
	fi

	log "HTTP e2e suite"
	E2E_BASE_URL=http://localhost:8080 "$GO" test -count=1 -tags=e2e ./tests/e2e/...

	if $KEEP_UP; then
		log "--keep-up set: leaving the stack running (scripts/agent/stack-down.sh to tear down)"
	fi
fi

log "ALL CHECKS PASSED"
