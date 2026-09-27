#!/usr/bin/env bash
# Brings up the full local stack and leaves it running: Docker infra
# (Postgres + Redpanda), migrations, the mock external-API server, the api
# process, and the worker process — all wired together exactly as they would
# be in the Postgres+Kafka profile (see CLAUDE.md's config-precedence note).
#
# This script is deliberately feature-agnostic: it doesn't know or care what
# you're about to test. Once it's up, drive or inspect whatever you're
# working on using the general-purpose tools it hands you (printed at the
# end, and reproduced here):
#
#   - REST API at http://localhost:8080, gRPC at localhost:9090
#   - bin/agentctl kafka publish/consume — trigger or inspect ANY integration
#     event by name, not just one hardcoded scenario
#   - the mock API at http://localhost:9999 — configure routes for whatever
#     external call your feature makes (see configs/mockapi.routes.example.json
#     and POST /__routes to reconfigure without restarting), inspect what it
#     received at GET /__requests
#   - api/worker log files (paths printed below) for anything that only
#     shows up in logs
#   - go test -tags=e2e (or a test you write) against E2E_BASE_URL
#
# See scripts/agent/examples/ for worked examples of composing these into a
# feature-specific check.
#
# Calling this again while a stack is already up is a no-op (prints the same
# summary). Use --force to tear down and restart. Tear down with
# scripts/agent/stack-down.sh.
#
# Usage:
#   scripts/agent/stack-up.sh [--skip-docker] [--force]
#
#   --skip-docker  assume Postgres/Kafka are already up; skip docker-up and
#                  migrations (implies stack-down.sh won't tear docker down
#                  either).
#   --force        tear down a previously running stack first, even if it
#                  still looks healthy.
#
# Env:
#   GO   path to the real go binary (default: go — see CLAUDE.md's snap gotcha)
set -euo pipefail
cd "$(dirname "$0")/../.."
source scripts/agent/lib.sh

GO="${GO:-go}"
COMPOSE="docker compose -f deployments/docker/docker-compose.yml"
SKIP_DOCKER=false
FORCE=false

for arg in "$@"; do
	case "$arg" in
		--skip-docker) SKIP_DOCKER=true ;;
		--force) FORCE=true ;;
		*) die "unknown flag: $arg" ;;
	esac
done

STACK_DIR="${MYAPP_AGENT_STACK_DIR:-/tmp/myapp-agent-stack}"
STATE_FILE="$STACK_DIR/state.env"

if [ -f "$STATE_FILE" ]; then
	# shellcheck source=/dev/null
	source "$STATE_FILE"
	if $FORCE; then
		log "existing stack found — tearing it down first (--force)"
		scripts/agent/stack-down.sh || true
	elif kill -0 "${API_PID:-0}" 2>/dev/null; then
		log "stack already up — nothing to do"
		cat "$STACK_DIR/summary.txt"
		exit 0
	else
		log "stale state file found (process no longer running) — cleaning up"
		rm -rf "$STACK_DIR"
	fi
fi

mkdir -p "$STACK_DIR"
LOGDIR="$STACK_DIR/logs"
mkdir -p "$LOGDIR"

log "building binaries"
"$GO" build -o bin/api ./cmd/api
"$GO" build -o bin/worker ./cmd/worker
"$GO" build -o bin/migrate ./cmd/migrate
"$GO" build -o bin/agentctl ./cmd/agentctl
"$GO" build -o bin/mockapi ./cmd/mockapi

if ! $SKIP_DOCKER; then
	log "starting docker infra (postgres, redpanda)"
	$COMPOSE up -d
	wait_for_cmd 60 postgres -- $COMPOSE exec -T postgres pg_isready -U myapp
	wait_for_cmd 60 redpanda -- bash -c "$COMPOSE exec -T redpanda rpk cluster health | grep -q 'Healthy:.*true'"

	log "applying migrations"
	DATABASE_URL="postgres://myapp:myapp@localhost:5432/myapp?sslmode=disable" \
		./bin/migrate -dir ./migrations up
fi

start_bg mockapi "$LOGDIR/mockapi.log" -- ./bin/mockapi -addr :9999 -routes configs/mockapi.routes.example.json
wait_for_http http://localhost:9999/__health 10 mockapi "${PIDS[mockapi]}"

# No CONFIG_PATH / env overrides: run from the repo root so the committed
# configs/config.json (Postgres + Kafka profile) is picked up, matching the
# docker infra just started.
start_bg api "$LOGDIR/api.log" -- ./bin/api
wait_for_http http://localhost:8080/healthz 30 api "${PIDS[api]}"
wait_for_http http://localhost:8080/readyz 30 "api readiness" "${PIDS[api]}"

start_bg worker "$LOGDIR/worker.log" -- ./bin/worker
wait_for_cmd 15 worker "${PIDS[worker]}" -- grep -q "worker started" "$LOGDIR/worker.log"

cat >"$STATE_FILE" <<EOF
API_PID=${PIDS[api]}
WORKER_PID=${PIDS[worker]}
MOCKAPI_PID=${PIDS[mockapi]}
SKIP_DOCKER=$SKIP_DOCKER
LOGDIR=$LOGDIR
EOF

cat >"$STACK_DIR/summary.txt" <<EOF
myapp agent stack is up.

  REST API      http://localhost:8080   (/healthz, /readyz, /api/v1/..., /docs)
  gRPC API      localhost:9090
  mock API      http://localhost:9999   (GET /__requests, POST /__reset, POST /__routes)
  Postgres      postgres://myapp:myapp@localhost:5432/myapp?sslmode=disable
  Kafka         localhost:9092

  logs:  $LOGDIR/api.log
         $LOGDIR/worker.log
         $LOGDIR/mockapi.log

Drive or inspect any integration event (not just one built-in scenario):
  ./bin/agentctl kafka publish -name <ctx>.v1.<event> -aggregate-id <id> -payload '<json>'
  ./bin/agentctl kafka consume -name <ctx>.v1.<event> -aggregate-id <id> -timeout 10s

Reconfigure the external-API double for whatever your feature calls out to:
  curl -X POST http://localhost:9999/__routes -d '[{"method":"POST","path":"/v1/...","status":200,"body":{...}}]'
  curl http://localhost:9999/__requests

Run the HTTP e2e suite (or your own test) against it:
  E2E_BASE_URL=http://localhost:8080 go test -tags=e2e ./tests/e2e/...

See scripts/agent/examples/ for a worked, feature-specific check.

Tear down: scripts/agent/stack-down.sh
EOF

log "stack is up"
cat "$STACK_DIR/summary.txt"
