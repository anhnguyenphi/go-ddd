#!/usr/bin/env bash
# report: turn the last smoke run into tmp/harness/report.md and print it.
#
# The run passes only when every check passed, at least one check ran, no
# request reached WireMock unstubbed, and the app logs have no
# unexpected error lines. A setup failure (HARNESS_SETUP_ERROR, set by
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
meta="$(tr '\n' ' ' 2>/dev/null <"$HARNESS_TMP/smoke.meta" || true)"

if [ -n "$setup_error" ]; then
	result="HARNESS ERROR"
	code=2
elif [ "$failed" = 0 ] && [ "$passed" -gt 0 ] && [ "$unmatched_count" = 0 ] && [ "$error_count" = 0 ]; then
	result="PASS"
	code=0
else
	result="FAIL"
	code=1
fi

{
	echo "# Harness report"
	echo
	echo '```'
	echo "Result: $result   Git: $(git_summary)"
	echo "Schema: $schema   Run: $(date -u +%Y-%m-%dT%H:%M:%SZ)   $meta"
	[ -n "$setup_error" ] && echo "Setup failed at: $setup_error (see the verify output above it)"
	echo '```'
	echo
	echo "## Smoke checks"
	echo
	echo '```'
	if [ -s "$results" ]; then
		awk -F '\t' '{ if ($2 == "PASS") printf "PASS %s (%ss)\n", $1, $3; else printf "FAIL %s (%ss)\n     %s\n", $1, $3, $4 }' "$results"
	else
		echo "(no checks ran)"
	fi
	echo "... $passed passed, $failed failed"
	echo '```'
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
