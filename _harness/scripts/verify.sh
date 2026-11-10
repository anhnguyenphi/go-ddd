#!/usr/bin/env bash
# verify: the whole sandbox pipeline in one command.
#
#   lock -> up (infra + this sandbox's DB, topics, stubs) -> migrate -> seed
#        -> run (build + start apps) -> [e2e] -> check -> report
#
# Everything is left running afterwards, so you can inspect it with
# harness-logs, harness-mocks, harness-events and harness-kafka, or re-run
# checks with harness-check. Stop it with harness-down.
#
# Usage: _harness/scripts/verify.sh [--e2e] [<file.sh>...] [<check-name-regex>]
#   --e2e   also run tests/e2e (the regression scenarios) against this sandbox
# Exit status: 0 PASS, 1 FAIL (the code or a check is wrong), 2 HARNESS ERROR
# (the harness itself could not start; nothing was verified).
set -uo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

e2e=false
args=()
for a in "$@"; do
	case "$a" in
		--e2e) e2e=true ;;
		*) args+=("$a") ;;
	esac
done

started=$SECONDS
rm -f "$HARNESS_TMP/results.tsv" "$HARNESS_TMP/unmatched.jsonl" "$HARNESS_TMP/check.meta" \
	"$HARNESS_TMP/e2e.status" "$HARNESS_TMP/e2e.txt"

setup() { # <step> <script> [args...]
	local step="$1"
	shift
	log "[$step]"
	if ! "$@"; then
		HARNESS_SETUP_ERROR="$step" "$HARNESS_DIR/scripts/report.sh" >/dev/null
		echo
		echo "HARNESS ERROR at step '$step' (sandbox $SANDBOX). Nothing was verified. Fix the"
		echo "harness or its environment first (make harness-logs, make harness-down ARGS=--purge)."
		echo "Report: $HARNESS_TMP/report.md"
		exit 2
	fi
}

setup up "$HARNESS_DIR/scripts/up.sh"
sandbox_load || true
setup migrate "$HARNESS_DIR/scripts/migrate.sh" up
setup seed "$HARNESS_DIR/scripts/seed.sh"
setup run "$HARNESS_DIR/scripts/run.sh"

if $e2e; then
	log "[e2e]"
	"$HARNESS_DIR/scripts/e2e.sh" >/dev/null || true
	log "e2e: $(tail -n 3 "$HARNESS_TMP/e2e.txt" | tr '\n' ' ')"
fi

log "[check]"
"$HARNESS_DIR/scripts/check.sh" ${args[@]+"${args[@]}"}
code=$?
log "verify finished in $((SECONDS - started))s; report: $HARNESS_TMP/report.md"
exit "$code"
