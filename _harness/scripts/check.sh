#!/usr/bin/env bash
# check: run scratch checks against this sandbox's running apps, then write
# the report. Checks are the agent's quick feedback loop, not regression
# (that is tests/); see the header of check_dsl.sh.
#
# Usage: _harness/scripts/check.sh [<file.sh>...] [<name-regex>]
#   (no args)      every _harness/checks/*.sh in this worktree
#   <file.sh>      only these check files (any path)
#   <name-regex>   only run checks whose --name matches (grep -E)
#
# With no check files at all, only the always-on gates run (apps alive, no
# unmatched downstream calls, no unexpected error lines).
#
# Asking for checks and running none is a FAIL, not a quiet pass: a check file
# with a bash syntax error, a file that ran no check, or a name regex that
# matched nothing each get a FAIL line in the results.
#
# Exit status: 0 when the report says PASS, 1 otherwise.
set -uo pipefail
. "$(dirname "$0")/lib.sh"
. "$(dirname "$0")/check_dsl.sh"
require_sandbox
acquire_lock

files=()
while [ $# -gt 0 ]; do
	case "$1" in
		-*) die "unknown flag: $1" ;;
		*.sh)
			[ -f "$1" ] || die "no check file $1"
			files+=("$1")
			shift
			;;
		*) HARNESS_FILTER="$1"; shift ;;
	esac
done

app_alive api || die "api is not running; run make harness-verify (or harness-up + harness-run)"
app_alive worker || die "worker is not running; run make harness-verify (or harness-up + harness-run)"
mock_up || die "WireMock is not reachable at $HARNESS_MOCK_URL; run make harness-up"
ensure_agentctl

if [ ${#files[@]} -eq 0 ]; then
	for f in "$HARNESS_DIR"/checks/*.sh; do
		[ -e "$f" ] && files+=("$f")
	done
fi

: >"$CHECK_RESULTS"
: >"$HARNESS_TMP/unmatched.jsonl"
rm -rf "$HARNESS_TMP/checks"/* "$HARNESS_TMP/responses"/* "$HARNESS_TMP/vars"/*
{
	echo "checks=${files[*]+${files[*]}}"
	echo "filter=${HARNESS_FILTER:-}"
} >"$HARNESS_TMP/check.meta"

if [ ${#files[@]} -eq 0 ]; then
	log "no check files in $HARNESS_DIR/checks/; only the always-on gates run"
else
	log "checks: ${files[*]}${HARNESS_FILTER:+ (filter: $HARNESS_FILTER)}"
fi
_rows() { grep -c . "$CHECK_RESULTS" 2>/dev/null || true; }
for f in ${files[@]+"${files[@]}"}; do
	# A syntax error stops bash partway through the file, silently dropping
	# every check after it.
	if ! syntax="$(bash -n "$f" 2>&1)"; then
		_check_record "file:$(basename "$f")" FAIL 0 "bash syntax error, no check in it ran: $syntax"
		continue
	fi
	rows_before="$(_rows)"
	# shellcheck source=/dev/null
	. "$f"
	if [ -z "$HARNESS_FILTER" ] && [ "$(_rows)" = "$rows_before" ]; then
		_check_record "file:$(basename "$f")" FAIL 0 "the file ran no check (no \`check\` call was reached)"
	fi
done
if [ -n "$HARNESS_FILTER" ] && [ "$(_rows)" = 0 ]; then
	_check_record "filter:$HARNESS_FILTER" FAIL 0 "no check name matches this regex in: ${files[*]+${files[*]}}"
fi

"$HARNESS_DIR/scripts/report.sh"
