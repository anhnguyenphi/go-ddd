#!/usr/bin/env bash
# Tears down whatever scripts/agent/stack-up.sh started: kills api/worker/
# mockapi and, unless the stack was brought up with --skip-docker, the
# docker-compose infra. Safe to run even if no stack is up (no-op).
#
# Usage: scripts/agent/stack-down.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
source scripts/agent/lib.sh

STACK_DIR="${MYAPP_AGENT_STACK_DIR:-/tmp/myapp-agent-stack}"
STATE_FILE="$STACK_DIR/state.env"

if [ ! -f "$STATE_FILE" ]; then
	log "no stack found at $STACK_DIR — nothing to do"
	exit 0
fi
# shellcheck source=/dev/null
source "$STATE_FILE"

stop_pid api "${API_PID:-}"
stop_pid worker "${WORKER_PID:-}"
stop_pid mockapi "${MOCKAPI_PID:-}"

if [ "${SKIP_DOCKER:-false}" != "true" ]; then
	log "tearing down docker infra"
	docker compose -f deployments/docker/docker-compose.yml down -v >/dev/null 2>&1 || true
fi

rm -rf "$STACK_DIR"
log "stack is down"
