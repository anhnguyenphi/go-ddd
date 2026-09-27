#!/usr/bin/env bash
# EXAMPLE — not run by verify.sh or any make target. This is a worked pattern
# for verifying one specific feature (the customer context's Kafka fan-out) on
# top of the generic stack; copy and adapt it for whatever you're testing.
#
# What it proves: publishing a customer.v1.created event directly onto Kafka
# (as a real producer, or a replayed message, would — bypassing the outbox)
# is consumed by BOTH of the context's subscribers:
#   - the read-model projection (assert via the REST read endpoint)
#   - the notification handler (assert via its log line)
#
# Prereqs: scripts/agent/stack-up.sh has already been run.
#
# Usage: scripts/agent/examples/kafka-roundtrip.sh
set -euo pipefail
cd "$(dirname "$0")/../../.."
source scripts/agent/lib.sh

STACK_DIR="${MYAPP_AGENT_STACK_DIR:-/tmp/myapp-agent-stack}"
STATE_FILE="$STACK_DIR/state.env"
[ -f "$STATE_FILE" ] || die "no stack up — run scripts/agent/stack-up.sh first"
# shellcheck source=/dev/null
source "$STATE_FILE"

AGG_ID="example-$(date +%s)"
EMAIL="example+$(date +%s)@example.com"

log "publishing customer.v1.created for aggregate $AGG_ID"
./bin/agentctl kafka publish -brokers localhost:9092 \
	-name customer.v1.created -aggregate-id "$AGG_ID" \
	-payload "{\"id\":\"$AGG_ID\",\"name\":\"Example\",\"email\":\"$EMAIL\"}"

wait_for_http "http://localhost:8080/api/v1/customers/$AGG_ID" 10 "read-model projection"
wait_for_cmd 10 "notification handler" -- grep -q "$EMAIL" "$LOGDIR/worker.log"

log "OK: both the read-model projection and the notification handler consumed the event"
