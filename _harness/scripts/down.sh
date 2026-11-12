#!/usr/bin/env bash
# down: stop this sandbox's apps. The shared infra keeps running for the
# other sandboxes (stop it with make harness-infra-down).
#
# Usage: _harness/scripts/down.sh [--purge]
#   --purge  also drop this sandbox's database, topics and WireMock stubs,
#            release its slot, and delete tmp/harness/<sandbox> (logs, report,
#            built binaries): a clean slate for the next run.
set -euo pipefail
. "$(dirname "$0")/lib.sh"

if [ ! -f "$(sandbox_file)" ] && [ ! -d "$HARNESS_TMP" ] && [ ! -d "$HARNESS_PIDS" ]; then
	log "no sandbox named '$SANDBOX' here; nothing to stop (make harness-sandboxes lists them)"
	exit 0
fi
acquire_lock

purge=false
for arg in "$@"; do
	case "$arg" in
		--purge) purge=true ;;
		*) die "unknown flag: $arg" ;;
	esac
done

sandbox_check_owner
for app in $HARNESS_APPS; do
	stop_app "$app"
done

if $purge; then
	if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 && deps_running; then
		log "dropping $HARNESS_PG_DB, topics $KAFKA_TOPIC_PREFIX.*, and stubs under /$HARNESS_MOCK_PREFIX/"
		sandbox_purge_resources "$SANDBOX"
	else
		warn "shared infra not running; only the slot and local state are released"
	fi
	rm -rf "$(sandbox_file)" "$HARNESS_PIDS"
	release_lock
	trap - EXIT
	rm -rf "$HARNESS_TMP"
	log "sandbox purged"
else
	log "sandbox stopped (data kept; --purge to drop it)"
fi
