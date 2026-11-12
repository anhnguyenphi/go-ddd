#!/usr/bin/env bash
# run: build api and worker from the working tree, (re)start them detached
# with this sandbox's env, and wait until they're ready, including the
# worker's Kafka consumer group having settled, so the first async check
# doesn't race it.
#
# When the build produced the very binaries that are already running, with the
# same environment, the restart (about 20s) is skipped: the apps keep running
# and a new run starts in their logs. HARNESS_FORCE_RESTART=1 restarts anyway.
#
# Usage: _harness/scripts/run.sh
set -euo pipefail
. "$(dirname "$0")/lib.sh"
require_sandbox
acquire_lock

deps_running || die "shared infra is not up; run make harness-up first"

log "building api, worker, agentctl"
build_bin api ./cmd/api
build_bin worker ./cmd/worker
build_bin agentctl ./cmd/agentctl
{
	tree_stamp
	echo "$(git_summary), $(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"$HARNESS_TMP/build.stamp"

fingerprint="$(run_fingerprint)"
if [ -z "${HARNESS_FORCE_RESTART:-}" ] && app_alive api && app_alive worker &&
	[ "$(cat "$HARNESS_TMP/run.fingerprint" 2>/dev/null || true)" = "$fingerprint" ] &&
	curl -fsS -o /dev/null --max-time 5 "$HARNESS_API_URL/readyz" 2>/dev/null; then
	# Same semantics as a restart, minus the wait: earlier log lines, and the
	# lines earlier checks excused, belong to the previous run.
	log_baseline_set
	rm -f "$HARNESS_TMP/expected-lines.txt"
	log "binaries and env unchanged; api and worker keep running (HARNESS_FORCE_RESTART=1 to restart)"
	log "apps are up: api $HARNESS_API_URL (gRPC $HARNESS_GRPC_ADDR), worker"
	exit 0
fi

# In parallel: each drains its Kafka readers on SIGTERM, which takes seconds.
stop_app api &
stop_app worker &
wait
# Logs restart with the apps, so the lines checks excused restart too.
# An e2e result from the previous build no longer says anything about this one.
rm -f "$HARNESS_TMP/expected-lines.txt" "$HARNESS_TMP/e2e.status" "$HARNESS_TMP/e2e.txt" \
	"$HARNESS_TMP/log.baseline" "$HARNESS_TMP/run.fingerprint"

require_free_port "${HTTP_ADDR##*:}" "api http"
require_free_port "${GRPC_ADDR##*:}" "api grpc"

start_app api -- "$HARNESS_BIN/api"
wait_http "$HARNESS_API_URL/readyz" 30 api api

start_app worker -- "$HARNESS_BIN/worker"
wait_log worker "worker started" 30

# Both processes join the same consumer group. Until it is Stable, a
# produced message can sit unconsumed for a rebalance or two.
log "waiting for consumer group $KAFKA_GROUP_ID to settle"
deadline=$((SECONDS + 60))
until rpk group describe "$KAFKA_GROUP_ID" 2>/dev/null | grep -qE '^STATE +Stable'; do
	app_alive api || die "api exited; see $HARNESS_TMP/api.log"
	app_alive worker || die "worker exited; see $HARNESS_TMP/worker.log"
	[ "$SECONDS" -ge "$deadline" ] && die "consumer group $KAFKA_GROUP_ID not Stable after 60s"
	sleep 0.5
done
echo "$fingerprint" >"$HARNESS_TMP/run.fingerprint"
log "apps are up: api $HARNESS_API_URL (gRPC $HARNESS_GRPC_ADDR), worker"
