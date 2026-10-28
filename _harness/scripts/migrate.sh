#!/usr/bin/env bash
# migrate: apply or roll back migrations on the harness database with the
# production migration tool (cmd/migrate).
#
# Usage: _harness/scripts/migrate.sh [up [N] | down [N] | status]
#   up        apply every pending migration (default)
#   up N      apply at most N
#   down [N]  roll back the last N (default 1)
#   status    print the applied versions
set -euo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

deps_running || die "dependencies are not up; run make harness-up first"

direction="${1:-up}"
steps="${2:-}"

if [ "$direction" = "status" ]; then
	psql_q "SELECT version || '  ' || name || '  ' || applied_at FROM schema_migrations ORDER BY version" \
		|| die "no schema_migrations table yet; run migrate up"
	exit 0
fi
case "$direction" in up | down) ;; *) die "unknown direction '$direction' (want: up | down | status)" ;; esac

build_bin migrate ./cmd/migrate

# Go's flag package stops at the first positional argument, so the flags
# have to come before the direction.
args=(-dir migrations)
[ -n "$steps" ] && args+=(-steps "$steps")
log "migrate $direction${steps:+ $steps}"
"$HARNESS_BIN/migrate" "${args[@]}" "$direction"
