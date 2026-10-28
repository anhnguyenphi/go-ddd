#!/usr/bin/env bash
# down: stop the apps, then the dependencies (Postgres, Redpanda, WireMock).
#
# Usage: _harness/scripts/down.sh [--purge]
#   --purge  also delete the database volume and tmp/harness (logs, report,
#            built binaries): a clean slate for the next run.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

purge=false
for arg in "$@"; do
	case "$arg" in
		--purge) purge=true ;;
		*) die "unknown flag: $arg" ;;
	esac
done

for app in $HARNESS_APPS; do
	stop_app "$app"
done

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
	if $purge; then
		log "removing dependencies and their volumes"
		$COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true
	else
		log "stopping dependencies (data kept; --purge to delete it)"
		$COMPOSE down --remove-orphans >/dev/null 2>&1 || true
	fi
else
	warn "docker daemon not reachable; skipped stopping dependencies"
fi

if $purge; then
	release_lock
	trap - EXIT
	rm -rf "$HARNESS_TMP"
fi
log "harness is down"
