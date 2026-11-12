#!/usr/bin/env bash
# e2e: run the black-box regression suite (tests/e2e, i.e. every scenario in
# internal/testsupport/scenarios) against this sandbox's running binaries.
# The result is recorded for report.sh.
#
# Usage: _harness/scripts/e2e.sh [go test args...]   e.g. -run 'TestScenarios/Customer'
# Exit status: go test's.
set -uo pipefail
. "$(dirname "$0")/lib.sh"

require_sandbox
sandbox_dirs
app_alive api || die "api is not running; run make harness-verify (or harness-up + harness-run)"

log "e2e: tests/e2e against $HARNESS_API_URL (gRPC $HARNESS_GRPC_ADDR)"
started=$SECONDS
E2E_BASE_URL="$HARNESS_API_URL" E2E_GRPC_ADDR="$HARNESS_GRPC_ADDR" \
	"$GO" test -count=1 -tags=e2e "$@" ./tests/e2e/... 2>&1 | tee "$HARNESS_TMP/e2e.txt"
code=${PIPESTATUS[0]}
# Exit 0 is not proof that anything ran: `-run` with a typo gives
# "ok ... [no tests to run]", and a broken toolchain can print nothing at all.
if [ "$code" = 0 ] && ! grep -E '^ok[[:space:]].*tests/e2e' "$HARNESS_TMP/e2e.txt" | grep -qv 'no tests to run'; then
	echo "FAIL: go test exited 0 but ran no e2e test (check the -run pattern and the GO toolchain)" | tee -a "$HARNESS_TMP/e2e.txt"
	code=1
fi
printf '%s\t%s\n' "$code" "$((SECONDS - started))" >"$HARNESS_TMP/e2e.status"
exit "$code"
