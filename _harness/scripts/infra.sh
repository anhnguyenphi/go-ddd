#!/usr/bin/env bash
# infra: the shared dependencies every sandbox slices up (Postgres, Redpanda,
# WireMock), as one docker compose project. Idempotent. You rarely call this
# directly: up.sh starts the infra when it isn't running.
#
# Usage: _harness/scripts/infra.sh up
#        _harness/scripts/infra.sh status
#        _harness/scripts/infra.sh down [--force] [--purge]
#          refuses while sandboxes are registered (other agents may be using
#          them) unless --force; --purge also deletes the data volume.
set -euo pipefail
. "$(dirname "$0")/lib.sh"

infra_up() {
	command -v docker >/dev/null 2>&1 || die "docker not found on PATH"
	command -v jq >/dev/null 2>&1 || die "jq not found on PATH"
	docker info >/dev/null 2>&1 || die "the docker daemon is not reachable (start Docker Desktop, colima, ...)"
	infra_lock
	if deps_running && mock_up; then
		infra_unlock
		return 0
	fi
	log "starting shared infra (compose project myapp-harness)"
	if ! $COMPOSE up -d --wait --wait-timeout 120 >/dev/null 2>&1; then
		$COMPOSE ps >&2 || true
		infra_unlock
		die "shared infra did not become healthy; see: $COMPOSE logs"
	fi
	infra_unlock
	log "shared infra is up: postgres :$HARNESS_PG_PORT, kafka $HARNESS_KAFKA_BROKERS, wiremock $HARNESS_MOCK_URL"
}

case "${1:-status}" in
	up) infra_up ;;
	status)
		$COMPOSE ps
		echo
		echo "sandboxes registered: $(sandbox_names | tr '\n' ' ')"
		;;
	down)
		shift
		force=false purge=false
		for a in "$@"; do
			case "$a" in
				--force) force=true ;;
				--purge) purge=true ;;
				*) die "unknown flag: $a" ;;
			esac
		done
		names="$(sandbox_names | tr '\n' ' ')"
		if [ -n "${names// /}" ] && ! $force; then
			die "sandboxes still registered: $names
stop them first (make harness-down ARGS=--purge in each worktree, or make harness-sandboxes ARGS=gc), or pass --force"
		fi
		infra_lock
		if $purge; then
			log "removing shared infra and its data volume"
			$COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true
			rm -f "$HARNESS_SANDBOXES"/*.env
		else
			log "stopping shared infra (data kept)"
			$COMPOSE down --remove-orphans >/dev/null 2>&1 || true
		fi
		infra_unlock
		;;
	*) die "unknown command '$1' (want: up | status | down)" ;;
esac
