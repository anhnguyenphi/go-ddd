#!/usr/bin/env bash
# smoke: run the smoke checks against the running apps, then write the report.
#
# Usage: _harness/scripts/smoke.sh [--suite <name>]... [<name-regex>]
#   <name-regex>    only run checks whose --name matches (grep -E)
#   --suite <name>  also run the opt-in suite _harness/smoke/checks_<name>.sh;
#                   --suite all runs every opt-in suite
#
# Exit status: 0 when the report says PASS, 1 otherwise.
set -uo pipefail
. "$(dirname "$0")/lib.sh"
. "$(dirname "$0")/check_dsl.sh"
acquire_lock

suites=()
while [ $# -gt 0 ]; do
	case "$1" in
		--suite) suites+=("$2"); shift 2 ;;
		-*) die "unknown flag: $1" ;;
		*) HARNESS_FILTER="$1"; shift ;;
	esac
done

app_alive api || die "api is not running; run make harness-verify (or harness-up + harness-run)"
app_alive worker || die "worker is not running; run make harness-verify (or harness-up + harness-run)"
mock_up || die "WireMock is not reachable at $HARNESS_MOCK_URL; run make harness-up"
ensure_agentctl

files=("$HARNESS_DIR/smoke/checks.sh")
for s in ${suites[@]+"${suites[@]}"}; do
	if [ "$s" = all ]; then
		for f in "$HARNESS_DIR"/smoke/checks_*.sh; do
			[ -e "$f" ] && files+=("$f")
		done
	else
		[ -f "$HARNESS_DIR/smoke/checks_$s.sh" ] || die "no suite '$s' (want one of: $(cd "$HARNESS_DIR/smoke" && ls checks_*.sh 2>/dev/null | sed 's/^checks_//; s/\.sh$//' | tr '\n' ' '))"
		files+=("$HARNESS_DIR/smoke/checks_$s.sh")
	fi
done

: >"$CHECK_RESULTS"
: >"$HARNESS_TMP/unmatched.jsonl"
rm -rf "$HARNESS_TMP/checks"/* "$HARNESS_TMP/responses"/* "$HARNESS_TMP/vars"/*
{
	echo "suites=default${suites[@]+ ${suites[*]}}"
	echo "filter=${HARNESS_FILTER:-}"
} >"$HARNESS_TMP/smoke.meta"

log "smoke: $(printf '%s ' "${files[@]}")${HARNESS_FILTER:+(filter: $HARNESS_FILTER)}"
for f in "${files[@]}"; do
	# shellcheck source=/dev/null
	. "$f"
done

"$HARNESS_DIR/scripts/report.sh"
