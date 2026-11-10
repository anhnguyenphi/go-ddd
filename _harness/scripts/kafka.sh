#!/usr/bin/env bash
# kafka: agentctl's kafka commands, pointed at the harness broker and scoped
# to this sandbox's topics ($KAFKA_TOPIC_PREFIX.*).
#
# Usage: _harness/scripts/kafka.sh publish -name <event> -aggregate-id <id> -payload '<json>'
#        _harness/scripts/kafka.sh consume -name <event> [-aggregate-id <id>] [-count N]
#        _harness/scripts/kafka.sh events [-since-mark tmp/harness/<sandbox>/events.mark]
#        _harness/scripts/kafka.sh topics
set -euo pipefail
. "$(dirname "$0")/lib.sh"
ensure_agentctl

[ $# -ge 1 ] || die "usage: kafka.sh <publish|consume|mark|events|topics> [flags]"
sub="$1"
shift
case "$sub" in
	topics) rpk topic list -r "^$KAFKA_TOPIC_PREFIX\\..*" ;;
	publish | consume) agentctl kafka "$sub" -brokers "$HARNESS_KAFKA_BROKERS" -topic-prefix "$KAFKA_TOPIC_PREFIX" "$@" ;;
	events | mark) agentctl kafka "$sub" -brokers "$HARNESS_KAFKA_BROKERS" "$@" ;;
	*) die "unknown subcommand '$sub' (want: publish | consume | mark | events | topics)" ;;
esac
