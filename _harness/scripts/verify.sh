#!/usr/bin/env bash
# verify: the whole pipeline in one command.
#
#   lock -> up (deps + stubs) -> migrate -> seed -> run (build + start apps)
#        -> smoke (checks) -> report (tmp/harness/report.md)
#
# Everything is left running afterwards, so you can inspect it with
# harness-logs, harness-mocks, harness-events and harness-kafka, or re-run a
# subset with harness-smoke. Stop it with harness-down.
#
# Usage: _harness/scripts/verify.sh [--suite <name>]... [<check-name-regex>]
# Exit status: 0 PASS, 1 FAIL (the code or a check is wrong), 2 HARNESS ERROR
# (the harness itself could not start; nothing was verified).
set -uo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

started=$SECONDS
rm -f "$HARNESS_TMP/results.tsv" "$HARNESS_TMP/unmatched.jsonl" "$HARNESS_TMP/smoke.meta"

setup() { # <step> <script> [args...]
	local step="$1"
	shift
	log "[$step]"
	if ! "$@"; then
		HARNESS_SETUP_ERROR="$step" "$HARNESS_DIR/scripts/report.sh" >/dev/null
		echo
		echo "HARNESS ERROR at step '$step'. Nothing was verified. Fix the harness or its"
		echo "environment first (make harness-logs, make harness-down ARGS=--purge)."
		echo "Report: $HARNESS_TMP/report.md"
		exit 2
	fi
}

setup up "$HARNESS_DIR/scripts/up.sh"
setup migrate "$HARNESS_DIR/scripts/migrate.sh" up
setup seed "$HARNESS_DIR/scripts/seed.sh"
setup run "$HARNESS_DIR/scripts/run.sh"

log "[smoke]"
"$HARNESS_DIR/scripts/smoke.sh" "$@"
code=$?
log "verify finished in $((SECONDS - started))s; report: $HARNESS_TMP/report.md"
exit "$code"
