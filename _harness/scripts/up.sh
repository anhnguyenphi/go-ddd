#!/usr/bin/env bash
# up: start the dependencies (Postgres, Redpanda, WireMock), wait until
# they're healthy, create the topics, and reload WireMock so every stub in
# _harness/mocks is loaded. Idempotent: safe to run while already up.
#
# Usage: _harness/scripts/up.sh
set -euo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

command -v docker >/dev/null 2>&1 || die "docker not found on PATH"
command -v jq >/dev/null 2>&1 || die "jq not found on PATH"
docker info >/dev/null 2>&1 || die "the docker daemon is not reachable (start Docker Desktop, colima, ...)"

log "starting dependencies (compose project myapp-harness)"
if ! $COMPOSE up -d --wait --wait-timeout 120 >/dev/null 2>&1; then
	$COMPOSE ps >&2 || true
	die "dependencies did not become healthy; see: $COMPOSE logs"
fi

log "creating topics: $HARNESS_TOPICS (+ .retry.1, .dlq)"
for base in $HARNESS_TOPICS; do
	for topic in "$base" "$base.retry.1" "$base.dlq"; do
		if ! out="$($COMPOSE exec -T redpanda rpk topic create "$topic" -p 1 -r 1 \
			-X brokers="localhost:${HARNESS_KAFKA_BROKERS##*:}" 2>&1)"; then
			echo "$out" | grep -q TOPIC_ALREADY_EXISTS || die "create topic $topic: $out"
		fi
	done
done

log "building agentctl"
build_bin agentctl ./cmd/agentctl

# Push _harness/mocks into WireMock (replacing whatever it had, including
# leftover failure scenarios) and check every stub made it.
want="$(mock_stub_count_on_disk)"
mock_reset || die "WireMock rejected the stubs in _harness/mocks; see: make harness-logs ARGS=wiremock"
got="$(curl -fsS "$HARNESS_MOCK_URL/__admin/mappings" | jq '.meta.total')"
[ "$got" = "$want" ] || die "WireMock has $got stub(s) but _harness/mocks defines $want; see: make harness-logs ARGS=wiremock"
log "WireMock loaded $got stub(s)"

log "dependencies are up"
