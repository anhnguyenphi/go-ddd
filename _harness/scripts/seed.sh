#!/usr/bin/env bash
# seed: load the fixture data in _harness/seed/*.sql, in lexical order. Every
# file must be idempotent, so re-seeding restores the fixtures to a known state.
#
# Usage: _harness/scripts/seed.sh
set -euo pipefail
. "$(dirname "$0")/lib.sh"
acquire_lock

deps_running || die "dependencies are not up; run make harness-up first"

for f in "$HARNESS_DIR"/seed/*.sql; do
	[ -e "$f" ] || continue
	log "seed $(basename "$f")"
	$COMPOSE exec -T postgres psql -U "$HARNESS_PG_USER" -d "$HARNESS_PG_DB" \
		-v ON_ERROR_STOP=1 -q -1 -f - <"$f" >/dev/null || die "seed file $f failed"
done
