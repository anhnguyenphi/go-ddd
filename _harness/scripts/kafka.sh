#!/usr/bin/env bash
# kafka: agentctl's kafka commands, pointed at the harness broker.
#
# Usage: _harness/scripts/kafka.sh publish -name <event> -aggregate-id <id> -payload '<json>'
#        _harness/scripts/kafka.sh consume -name <event> [-aggregate-id <id>] [-count N]
#        _harness/scripts/kafka.sh events [-since-mark tmp/harness/events.mark]
#        _harness/scripts/kafka.sh topics
set -euo pipefail
. "$(dirname "$0")/lib.sh"
ensure_agentctl

[ $# -ge 1 ] || die "usage: kafka.sh <publish|consume|mark|events|topics> [flags]"
sub="$1"
shift
if [ "$sub" = topics ]; then
	exec $COMPOSE exec -T redpanda rpk topic list -X brokers="localhost:${HARNESS_KAFKA_BROKERS##*:}"
fi
agentctl kafka "$sub" -brokers "$HARNESS_KAFKA_BROKERS" "$@"
