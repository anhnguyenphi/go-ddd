#!/usr/bin/env bash
# up: make sure the shared infra runs, then set up this sandbox's slice of it:
# a slot (ports), its database, its Kafka topics, and its copy of the WireMock
# stubs in _harness/mocks. Idempotent: safe to run while already up.
#
# Usage: _harness/scripts/up.sh
set -euo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

"$HARNESS_DIR/scripts/infra.sh" up
sandbox_allocate

sandbox_db_create || die "could not create database $HARNESS_PG_DB"

log "creating topics: $KAFKA_TOPIC_PREFIX.{$(echo "$HARNESS_TOPICS" | tr ' ' ',')} (+ .retry.1, .dlq)"
sandbox_topics_create || die "could not create the sandbox topics"

log "building agentctl"
build_bin agentctl ./cmd/agentctl

# Push _harness/mocks into WireMock under this sandbox's prefix (replacing its
# old copies, including leftover failure scenarios) and check every stub made it.
want="$(mock_stub_count_on_disk)"
mock_reset || die "WireMock rejected the stubs in _harness/mocks; see: make harness-logs ARGS=wiremock"
got="$(mock_sandbox_mappings | jq '[.mappings[] | select(.metadata.harnessScenario == null)] | length')"
[ "$got" = "$want" ] || die "WireMock has $got stub(s) for sandbox $SANDBOX but _harness/mocks defines $want; see: make harness-logs ARGS=wiremock"
log "WireMock loaded $got stub(s) under /$HARNESS_MOCK_PREFIX/"

log "sandbox is up: db $HARNESS_PG_DB, topics $KAFKA_TOPIC_PREFIX.*, group $KAFKA_GROUP_ID, api $HARNESS_API_URL"
