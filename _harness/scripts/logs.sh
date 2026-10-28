#!/usr/bin/env bash
# logs: read the app logs of the current harness run.
#
# Usage: _harness/scripts/logs.sh                   last 30 lines of every log
#        _harness/scripts/logs.sh errors [<regex>]  unexpected error lines (known noise
#                                                   filtered), optionally only those matching
#        _harness/scripts/logs.sh <app> [<n>]       last n (default 100) lines of api,
#                                                   worker or wiremock
set -uo pipefail
. "$(dirname "$0")/lib.sh"

case "${1:-}" in
	"")
		for app in $HARNESS_APPS; do
			echo "---- $HARNESS_TMP/$app.log"
			tail -n 30 "$HARNESS_TMP/$app.log" 2>/dev/null || echo "(no log)"
		done
		echo "---- wiremock (docker compose logs)"
		$COMPOSE logs --no-log-prefix --tail 30 wiremock 2>/dev/null || echo "(not running)"
		;;
	wiremock)
		$COMPOSE logs --no-log-prefix --tail "${2:-100}" wiremock
		;;
	errors)
		out="$(error_lines)"
		[ -n "${2:-}" ] && out="$(printf '%s\n' "$out" | grep -E -- "$2")"
		if [ -n "$out" ]; then printf '%s\n' "$out"; else echo "No unexpected error lines."; fi
		;;
	*)
		[ -f "$HARNESS_TMP/$1.log" ] || die "no log for '$1' (want one of: $HARNESS_APPS wiremock)"
		tail -n "${2:-100}" "$HARNESS_TMP/$1.log"
		;;
esac
