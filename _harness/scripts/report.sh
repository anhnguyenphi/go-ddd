#!/usr/bin/env bash
# report: turn this sandbox's last run into tmp/harness/<sandbox>/report.md
# and print it.
#
# The run passes only when every check passed, something behavioral ran (at
# least one check, or the e2e suite), e2e passed if it ran, no request reached
# WireMock unstubbed, and the app logs have no unexpected error lines. A setup failure (HARNESS_SETUP_ERROR, set by
# verify.sh) is reported as HARNESS ERROR: the harness broke, not the code.
#
# Usage: _harness/scripts/report.sh
# Exit status: 0 for PASS, 1 for FAIL, 2 for HARNESS ERROR.
set -uo pipefail
. "$(dirname "$0")/lib.sh"

results="$HARNESS_TMP/results.tsv"
report="$HARNESS_TMP/report.md"
setup_error="${HARNESS_SETUP_ERROR:-}"

passed=0 failed=0
if [ -f "$results" ]; then
	passed="$(grep -c "$(printf '\tPASS\t')" "$results" || true)"
	failed="$(grep -c "$(printf '\tFAIL\t')" "$results" || true)"
fi

# Unmatched calls caught by checks, plus any that arrived between checks.
unmatched_file="$HARNESS_TMP/.unmatched-report"
cat "$HARNESS_TMP/unmatched.jsonl" 2>/dev/null >"$unmatched_file" || true
if mock_requests unmatched >"$unmatched_file.now" 2>/dev/null; then
	jq -c '.[] | {check: "(between checks)", method, path, query, body}' "$unmatched_file.now" >>"$unmatched_file" 2>/dev/null || true
fi
unmatched_count="$(grep -c . "$unmatched_file" || true)"

errors="$(error_lines)"
error_count="$(printf '%s' "$errors" | grep -c . || true)"

schema="$(psql_q "SELECT coalesce(max(version), 'none') FROM schema_migrations" 2>/dev/null || echo unknown)"
meta="$(tr '\n' ' ' 2>/dev/null <"$HARNESS_TMP/check.meta" || true)"

e2e_code="" e2e_secs=""
if [ -f "$HARNESS_TMP/e2e.status" ]; then
	IFS="$(printf '\t')" read -r e2e_code e2e_secs <"$HARNESS_TMP/e2e.status" || true
fi

if [ -n "$setup_error" ]; then
	result="HARNESS ERROR"
	code=2
elif [ "$failed" = 0 ] && { [ "$passed" -gt 0 ] || [ -n "$e2e_code" ]; } &&
	{ [ -z "$e2e_code" ] || [ "$e2e_code" = 0 ]; } &&
	[ "$unmatched_count" = 0 ] && [ "$error_count" = 0 ]; then
	result="PASS"
	code=0
else
	result="FAIL"
	code=1
fi

{
	echo "# Harness sandbox report: $SANDBOX"
	echo
	echo '```'
	echo "Result: $result   Git: $(git_summary)"
	echo "Sandbox: $SANDBOX (slot ${HARNESS_SLOT:-?}, api ${HARNESS_API_URL:-?}, db $HARNESS_PG_DB, topics $KAFKA_TOPIC_PREFIX.*)"
	echo "Schema: $schema   Run: $(date -u +%Y-%m-%dT%H:%M:%SZ)   $meta"
	[ -n "$setup_error" ] && echo "Setup failed at: $setup_error (see the verify output above it)"
	echo '```'
	echo
	echo "## E2E (tests/e2e against this sandbox)"
	echo
	if [ -z "$e2e_code" ]; then
		echo "Not run (harness-verify ARGS=--e2e, or make harness-e2e)."
	else
		[ "$e2e_code" = 0 ] && echo "PASS (${e2e_secs}s)" || echo "FAIL (exit $e2e_code, ${e2e_secs}s): see $HARNESS_TMP/e2e.txt"
		echo
		echo '```'
		grep -E '^(--- FAIL|FAIL|ok|panic)' "$HARNESS_TMP/e2e.txt" 2>/dev/null | head -n 30
		echo '```'
	fi
	echo
	echo "## Scratch checks (_harness/checks, not regression)"
	echo
	echo '```'
	if [ -s "$results" ]; then
		awk -F '\t' '{ if ($2 == "PASS") printf "PASS %s (%ss)\n", $1, $3; else printf "FAIL %s (%ss)\n     %s\n", $1, $3, $4 }' "$results"
	else
		echo "(no checks ran)"
	fi
	echo "... $passed passed, $failed failed"
	echo '```'
	if [ "$passed" = 0 ] && [ "$failed" = 0 ] && [ -z "$e2e_code" ]; then
		echo
		echo "Nothing behavioral ran: write a check in _harness/checks/ or pass --e2e."
	fi
	echo
	echo "## Unmatched downstream requests"
	echo
	if [ "$unmatched_count" = 0 ]; then
		echo "None."
	else
		echo "Requests that matched no stub in _harness/mocks (skill: add-downstream-stub):"
		echo
		echo '```'
		cat "$unmatched_file"
		echo '```'
	fi
	echo
	echo "## Unexpected error lines"
	echo
	if [ "$error_count" = 0 ]; then
		echo "None."
	else
		echo "ERROR-level lines not excused by _harness/known-log-noise.txt or a check's --expect-errors:"
		echo
		echo '```'
		printf '%s\n' "$errors" | head -n 50
		[ "$error_count" -gt 50 ] && echo "... $((error_count - 50)) more: make harness-logs ARGS=errors"
		echo '```'
	fi
	echo
	echo "Logs: $HARNESS_TMP/{api,worker}.log, make harness-logs ARGS=wiremock · Responses: $HARNESS_TMP/responses/ · Failure evidence: $HARNESS_TMP/checks/"
} >"$report"

cat "$report"
exit "$code"
