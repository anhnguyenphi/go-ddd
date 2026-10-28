#!/usr/bin/env bash
# mocks: inspect and steer WireMock, which stands in for every downstream
# service. Its admin API (${HARNESS_MOCK_URL}/__admin) also works directly.
#
# Usage: _harness/scripts/mocks.sh requests [<svc>]        requests received (since the last check)
#        _harness/scripts/mocks.sh unmatched               requests that matched no stub
#        _harness/scripts/mocks.sh routes                  stubs currently loaded
#        _harness/scripts/mocks.sh scenario <svc> <kind>   break a downstream until reset; kind is
#                                                          error | timeout | malformed | empty
#        _harness/scripts/mocks.sh scenarios               active scenarios
#        _harness/scripts/mocks.sh reset                   drop scenarios, reload stubs, clear journal
#
# <svc> is the downstream's prefix, i.e. the first path segment of its URL in
# .env.harness (payments for ${HARNESS_MOCK_URL}/payments).
set -euo pipefail
. "$(dirname "$0")/lib.sh"

mock_up || die "WireMock is not reachable at $HARNESS_MOCK_URL; run make harness-up"

summary='.[] | "\(.at)  \(if .matched then "matched  " else "UNMATCHED" end)  \(.method) \(.path)\(if .query then "?" + .query else "" end)  \(.scenario // .route // "")"'

case "${1:-requests}" in
	requests)
		if [ -n "${2:-}" ]; then
			mock_requests | jq -r --arg p "/${2#/}" "map(select(.path | startswith(\$p))) | $summary"
		else
			mock_requests | jq -r "$summary"
		fi
		;;
	unmatched)
		mock_requests unmatched | jq .
		;;
	routes)
		curl -fsS "$HARNESS_MOCK_URL/__admin/mappings" | jq -r '.mappings[] |
			"\(.request.method // "ANY") \(.request.url // .request.urlPath // .request.urlPattern // .request.urlPathPattern) -> \(.response.status // 200)  \(.name // "")"'
		;;
	scenario)
		[ $# -eq 3 ] || die "usage: mocks.sh scenario <svc> <error|timeout|malformed|empty>"
		mock_scenario "$2" "$3" || die "could not add the scenario"
		svc="${2#/}"
		log "every request under /${svc%/}/ now gets: $3 (undo: make harness-mocks ARGS=reset)"
		;;
	scenarios)
		curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings/find-by-metadata" \
			-d '{"matchesJsonPath":"$.harnessScenario"}' | jq -r '.mappings[].metadata.harnessScenario'
		;;
	reset)
		mock_reset
		log "WireMock reset: scenarios dropped, stubs reloaded from _harness/mocks, journal cleared"
		;;
	*)
		die "unknown command '$1' (want: requests | unmatched | routes | scenario | scenarios | reset)"
		;;
esac
