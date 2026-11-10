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
# Exit status: 0 when the report says PASS, 1 otherwise.
set -uo pipefail
. "$(dirname "$0")/lib.sh"
. "$(dirname "$0")/check_dsl.sh"
acquire_lock

require_sandbox
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
for f in ${files[@]+"${files[@]}"}; do
	# shellcheck source=/dev/null
	. "$f"
done

"$HARNESS_DIR/scripts/report.sh"
