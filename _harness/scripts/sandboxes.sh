#!/usr/bin/env bash
# sandboxes: the registry of sandboxes sharing this machine's harness infra
# (every worktree of this repo).
#
# Usage: _harness/scripts/sandboxes.sh [list]   name, slot, ports, worktree, apps alive?
#        _harness/scripts/sandboxes.sh env      shell exports for the current sandbox
#                                               (E2E_BASE_URL, E2E_GRPC_ADDR, DATABASE_URL, ...)
#        _harness/scripts/sandboxes.sh gc       purge sandboxes whose worktree no longer exists
set -euo pipefail
. "$(dirname "$0")/lib.sh"

case "${1:-list}" in
	list)
		printf '%-26s %-4s %-24s %-8s %s\n' SANDBOX SLOT API APPS WORKTREE
		for n in $(sandbox_names); do
			slot="$(sandbox_field "$n" SB_SLOT)"
			wt="$(sandbox_field "$n" SB_WORKTREE)"
			http=$((HARNESS_SLOT_BASE + slot * HARNESS_SLOT_STEP))
			alive="down"
			curl -fsS -o /dev/null --max-time 1 "http://localhost:$http/healthz" 2>/dev/null && alive="up"
			[ -d "$wt" ] || wt="$wt (gone)"
			mark=""
			[ "$n" = "$SANDBOX" ] && mark="*"
			printf '%-26s %-4s %-24s %-8s %s\n' "$n$mark" "$slot" "localhost:$http" "$alive" "$wt"
		done
		;;
	env)
		require_sandbox
		cat <<ENV
export SANDBOX=$SANDBOX
export E2E_BASE_URL=$HARNESS_API_URL
export E2E_GRPC_ADDR=$HARNESS_GRPC_ADDR
export DATABASE_URL='$DATABASE_URL'
export KAFKA_BROKERS=$HARNESS_KAFKA_BROKERS
export KAFKA_TOPIC_PREFIX=$KAFKA_TOPIC_PREFIX
ENV
		;;
	gc)
		deps_running || die "shared infra is not up; nothing to clean in it (make harness-infra-up)"
		for n in $(sandbox_names); do
			wt="$(sandbox_field "$n" SB_WORKTREE)"
			[ -n "$wt" ] && [ -d "$wt" ] && continue
			log "purging sandbox $n (worktree ${wt:-unknown} is gone)"
			( HARNESS_PIDS="$HARNESS_REGISTRY/pids/$n"; for app in $HARNESS_APPS; do stop_app "$app"; done )
			sandbox_purge_resources "$n"
			rm -rf "$(sandbox_file "$n")" "$HARNESS_REGISTRY/pids/$n"
		done
		;;
	*) die "unknown command '$1' (want: list | env | gc)" ;;
esac
