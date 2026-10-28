#!/usr/bin/env bash
# events: the event tap. Prints integration events published since the last
# check started (each check re-marks the tap), across every topic, without
# joining a consumer group.
#
# Usage: _harness/scripts/events.sh [--all] [--raw] [<event-name>]
#   --all   every event still on the topics, not just since the last check
#   --raw   full JSON per event (payload, headers, offsets) instead of a summary
#   Retry and dead-letter topics are included; their topic name says so.
set -euo pipefail
. "$(dirname "$0")/lib.sh"
ensure_agentctl

all=false raw=false name=""
for arg in "$@"; do
	case "$arg" in
		--all) all=true ;;
		--raw) raw=true ;;
		-*) die "unknown flag: $arg" ;;
		*) name="$arg" ;;
	esac
done

args=(-brokers "$HARNESS_KAFKA_BROKERS")
if ! $all; then
	[ -f "$HARNESS_TMP/events.mark" ] || die "no check has run yet; use --all"
	args+=(-since-mark "$HARNESS_TMP/events.mark")
fi
[ -n "$name" ] && args+=(-name "$name")

if $raw; then
	agentctl kafka events "${args[@]}"
else
	agentctl kafka events "${args[@]}" |
		jq -r '"\(.topic)@\(.offset)  \(.name // "UNDECODABLE")  aggregate=\(.aggregate_id // "-")  \(.payload // .raw | tojson)"'
fi
