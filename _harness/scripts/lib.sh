#!/usr/bin/env bash
# Shared helpers for _harness/scripts/*.sh. Sourced, never run directly.
#
# The harness is a SANDBOX for agents (and humans) developing a feature, not a
# regression suite (that is tests/). Many sandboxes run side by side:
#
#   shared infra (one per machine, docker compose project myapp-harness):
#     postgres :55432   redpanda :59092   wiremock :59999
#   one slice per sandbox:
#     database  myapp_sb_<name>           Kafka topics sb_<name>.<ctx>.v1.events (+ .retry.1, .dlq)
#     group     myapp-sb-<name>           WireMock paths /sb_<name>/<svc>/... (stubs tagged with the sandbox)
#     api/worker on ports 58100+10*slot   state in tmp/harness/<name>/ of the worktree
#
# The sandbox name defaults to the git worktree's directory name (slugged), so
# one worktree = one feature = one sandbox. Override it with SANDBOX=<name>.
# Slots are recorded in a registry shared by every worktree of the repo:
# $(git rev-parse --git-common-dir)/myapp-harness/sandboxes/<name>.env.
#
# Portability: this must run on macOS's bash 3.2, so no associative arrays,
# no ${var,,}, no mapfile, no $EPOCHREALTIME, and empty arrays are expanded
# with ${a[@]+"${a[@]}"} so `set -u` doesn't trip on them.

HARNESS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$HARNESS_ROOT"

HARNESS_DIR="_harness"
GO="${GO:-go}"

log()  { echo "==> [${SANDBOX:-?}] $*"; }
warn() { echo "warning: $*" >&2; }
die()  { echo "error: $*" >&2; exit 1; }

# slug <text> — lowercase [a-z0-9_], at most 24 chars, never empty. A longer
# name keeps its first 19 chars and gets a 4-hex-digit hash of the whole name,
# so siblings that differ only at the end (…-v1, …-v2) stay distinct.
# Idempotent: slugging a slug returns it unchanged.
slug() {
	local s h
	s="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/_/g; s/__*/_/g; s/^_//; s/_$//')"
	if [ "${#s}" -gt 24 ]; then
		h="$(printf '%s' "$s" | cksum | cut -d' ' -f1)"
		s="$(printf '%s' "$s" | cut -c1-19)"
		s="${s%_}_$(printf '%04x' $((h % 65536)))"
	fi
	echo "${s:-default}"
}

# ---- sandbox identity ---------------------------------------------------------

HARNESS_WORKTREE="$(git rev-parse --show-toplevel 2>/dev/null || echo "$HARNESS_ROOT")"
if [ -n "${SANDBOX:-}" ]; then
	SANDBOX="$(slug "$SANDBOX")"
else
	SANDBOX="$(slug "$(basename "$HARNESS_WORKTREE")")"
fi
export SANDBOX

# The registry is shared by every worktree of this repo (the git common dir),
# so two worktrees never pick the same slot or name.
if _common="$(git rev-parse --git-common-dir 2>/dev/null)"; then
	HARNESS_REGISTRY="$(cd "$_common" && pwd)/myapp-harness"
else
	HARNESS_REGISTRY="$HARNESS_ROOT/tmp/harness/.registry"
fi
unset _common
HARNESS_SANDBOXES="$HARNESS_REGISTRY/sandboxes"
mkdir -p "$HARNESS_SANDBOXES"

HARNESS_TMP="tmp/harness/$SANDBOX"
HARNESS_BIN="$HARNESS_TMP/bin"
# Pid files live in the shared registry, not the worktree, so `sandboxes.sh gc`
# can still stop the apps of a sandbox whose worktree was deleted.
HARNESS_PIDS="$HARNESS_REGISTRY/pids/$SANDBOX"

# sandbox_dirs — create this sandbox's state directories. Called by whatever
# first needs them (acquire_lock, sandbox_allocate, build_bin), never just by
# sourcing this file: a read-only command, or one with a mistyped SANDBOX=,
# must not leave an empty sandbox behind.
sandbox_dirs() {
	mkdir -p "$HARNESS_BIN" "$HARNESS_PIDS" "$HARNESS_TMP/responses" "$HARNESS_TMP/vars" "$HARNESS_TMP/checks"
}

# Per-sandbox slice of the shared infra. Defined before .env.harness is
# sourced, so downstream URLs there can use ${HARNESS_MOCK_PREFIX}.
HARNESS_MOCK_PREFIX="sb_$SANDBOX"
KAFKA_TOPIC_PREFIX="sb_$SANDBOX"
KAFKA_GROUP_ID="myapp-sb-$SANDBOX"
HARNESS_PG_DB="myapp_sb_$SANDBOX"

set -a
# shellcheck source=/dev/null
. "$HARNESS_DIR/.env.harness"
DATABASE_URL="postgres://$HARNESS_PG_USER:$HARNESS_PG_PASSWORD@localhost:$HARNESS_PG_PORT/$HARNESS_PG_DB?sslmode=disable"
KAFKA_BROKERS="$HARNESS_KAFKA_BROKERS"
set +a
export HARNESS_MOCK_PREFIX KAFKA_TOPIC_PREFIX KAFKA_GROUP_ID HARNESS_PG_DB DATABASE_URL KAFKA_BROKERS

COMPOSE="docker compose -p myapp-harness -f $HARNESS_DIR/docker-compose.yml"

# Host processes the harness starts per sandbox (see run.sh); down.sh stops them.
HARNESS_APPS="api worker"

HARNESS_SLOT_BASE=58100
HARNESS_SLOT_STEP=10
HARNESS_SLOT_MAX=49

# ---- registry -----------------------------------------------------------------

sandbox_file() { echo "$HARNESS_SANDBOXES/${1:-$SANDBOX}.env"; }

# sandbox_field <name> <KEY> — one value from a registry entry.
sandbox_field() {
	local f
	f="$(sandbox_file "$1")"
	[ -f "$f" ] || return 0
	sed -n "s/^$2=//p" "$f" | head -n 1
}

# sandbox_ports <slot> — sets HTTP_ADDR, GRPC_ADDR, HARNESS_API_URL, HARNESS_GRPC_ADDR.
sandbox_ports() {
	local http=$((HARNESS_SLOT_BASE + $1 * HARNESS_SLOT_STEP))
	HTTP_ADDR=":$http"
	GRPC_ADDR=":$((http + 1))"
	HARNESS_API_URL="http://localhost:$http"
	HARNESS_GRPC_ADDR="localhost:$((http + 1))"
	HARNESS_SLOT="$1"
	export HTTP_ADDR GRPC_ADDR HARNESS_API_URL HARNESS_GRPC_ADDR HARNESS_SLOT
}

# sandbox_load — load this sandbox's slot from the registry, if it has one.
sandbox_load() {
	local slot
	slot="$(sandbox_field "$SANDBOX" SB_SLOT)"
	[ -n "$slot" ] || return 1
	sandbox_ports "$slot"
}

# require_sandbox — die unless this sandbox has a slot (i.e. `up` ran).
require_sandbox() {
	sandbox_load || die "sandbox '$SANDBOX' is not up; run make harness-up (or harness-verify)"
	sandbox_check_owner
}

# sandbox_check_owner — refuse to drive a sandbox registered to another
# worktree that still exists (two worktrees whose directories share a name).
sandbox_check_owner() {
	local owner
	owner="$(sandbox_field "$SANDBOX" SB_WORKTREE)"
	[ -z "$owner" ] && return 0
	[ "$owner" = "$HARNESS_WORKTREE" ] && return 0
	[ -d "$owner" ] || return 0
	die "sandbox '$SANDBOX' belongs to worktree $owner; pick another name: SANDBOX=<name> make harness-..."
}

# sandbox_names — every registered sandbox, one per line.
sandbox_names() {
	local f
	for f in "$HARNESS_SANDBOXES"/*.env; do
		[ -e "$f" ] || continue
		f="$(basename "$f")"
		echo "${f%.env}"
	done
}

# sandbox_allocate — give this sandbox a slot (reusing its own). Holds the
# infra lock so two sandboxes starting at once never pick the same slot.
sandbox_allocate() {
	sandbox_check_owner
	sandbox_dirs
	if sandbox_load; then
		return 0
	fi
	infra_lock
	local used="" n slot=""
	for n in $(sandbox_names); do
		used="$used $(sandbox_field "$n" SB_SLOT) "
	done
	n=0
	while [ "$n" -le "$HARNESS_SLOT_MAX" ]; do
		case "$used" in *" $n "*) ;; *) slot="$n"; break ;; esac
		n=$((n + 1))
	done
	if [ -z "$slot" ]; then
		infra_unlock
		die "no free sandbox slot (max $((HARNESS_SLOT_MAX + 1))); run make harness-sandboxes ARGS=gc, or purge one"
	fi
	{
		echo "SB_SLOT=$slot"
		echo "SB_WORKTREE=$HARNESS_WORKTREE"
		echo "SB_CREATED=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	} >"$(sandbox_file)"
	infra_unlock
	sandbox_ports "$slot"
	log "allocated slot $slot (api $HARNESS_API_URL, gRPC $HARNESS_GRPC_ADDR)"
}

# ---- locks --------------------------------------------------------------------
# Per-sandbox lock: one harness command per sandbox at a time. A pid file; a
# lock whose pid is gone is stale and is taken over. Nested scripts
# (verify -> up -> ...) inherit it through HARNESS_LOCK_HELD.

acquire_lock() {
	[ "${HARNESS_LOCK_HELD:-}" = "$SANDBOX" ] && return 0
	sandbox_dirs
	local lock="$HARNESS_TMP/.lock"
	if ( set -o noclobber; echo "$$" >"$lock" ) 2>/dev/null; then
		:
	else
		local holder
		holder="$(cat "$lock" 2>/dev/null || true)"
		if [ -n "$holder" ] && kill -0 "$holder" 2>/dev/null; then
			die "another harness command (pid $holder) holds sandbox '$SANDBOX' ($lock); wait for it, or kill it if it is stuck"
		fi
		warn "taking over stale lock from pid ${holder:-unknown}"
		echo "$$" >"$lock"
	fi
	export HARNESS_LOCK_HELD="$SANDBOX"
	trap 'release_lock' EXIT
}

release_lock() {
	local lock="$HARNESS_TMP/.lock"
	[ "$(cat "$lock" 2>/dev/null || true)" = "$$" ] && rm -f "$lock"
	return 0
}

# Infra lock: held only while starting/stopping the shared compose stack and
# while allocating a slot. mkdir is atomic; the pid inside detects staleness.
infra_lock() {
	[ "${HARNESS_INFRA_LOCK_HELD:-}" = "1" ] && return 0
	local d="$HARNESS_REGISTRY/infra.lock" deadline=$((SECONDS + 180)) holder
	until mkdir "$d" 2>/dev/null; do
		holder="$(cat "$d/pid" 2>/dev/null || true)"
		if [ -n "$holder" ] && ! kill -0 "$holder" 2>/dev/null; then
			warn "taking over stale infra lock from pid $holder"
			rm -rf "$d"
			continue
		fi
		[ "$SECONDS" -ge "$deadline" ] && die "infra lock $d held by pid ${holder:-unknown} for 180s"
		sleep 0.5
	done
	echo "$$" >"$d/pid"
	HARNESS_INFRA_LOCK_HELD=1
}

infra_unlock() {
	[ "${HARNESS_INFRA_LOCK_HELD:-}" = "1" ] || return 0
	local d="$HARNESS_REGISTRY/infra.lock"
	[ "$(cat "$d/pid" 2>/dev/null || true)" = "$$" ] && rm -rf "$d"
	HARNESS_INFRA_LOCK_HELD=""
	return 0
}

# ---- processes --------------------------------------------------------------
# Apps are detached into their own session (setsid, or nohup as a fallback):
# agent shells often kill the whole process group when a command returns,
# which would take a plain `cmd &` child down with it.

# start_app <name> -- <command...>
start_app() {
	local name="$1"
	shift
	[ "$1" = "--" ] && shift
	local logfile="$HARNESS_TMP/$name.log"
	: >"$logfile"
	if command -v setsid >/dev/null 2>&1; then
		setsid "$@" >>"$logfile" 2>&1 </dev/null &
	else
		nohup "$@" >>"$logfile" 2>&1 </dev/null &
	fi
	echo $! >"$HARNESS_PIDS/$name.pid"
	log "started $name (pid $!, log $logfile)"
}

app_pid() { cat "$HARNESS_PIDS/$1.pid" 2>/dev/null || true; }

app_alive() {
	local pid
	pid="$(app_pid "$1")"
	[ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

# stop_app <name> — SIGTERM, then SIGKILL after 10s.
stop_app() {
	local name="$1" pid
	pid="$(app_pid "$name")"
	rm -f "$HARNESS_PIDS/$name.pid"
	[ -z "$pid" ] && return 0
	kill -0 "$pid" 2>/dev/null || return 0
	log "stopping $name (pid $pid)"
	kill "$pid" 2>/dev/null || true
	local deadline=$((SECONDS + 10))
	while kill -0 "$pid" 2>/dev/null; do
		if [ "$SECONDS" -ge "$deadline" ]; then
			warn "$name (pid $pid) ignored SIGTERM; sending SIGKILL"
			kill -9 "$pid" 2>/dev/null || true
			break
		fi
		sleep 0.2
	done
}

# port_owner <port> — prints whatever is listening on a TCP port, if anything.
# Always succeeds: lsof exits 1 when nothing listens, which under pipefail
# would otherwise abort the caller.
port_owner() {
	if command -v lsof >/dev/null 2>&1; then
		lsof -nP -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | tail -n +2 || true
	elif command -v ss >/dev/null 2>&1; then
		ss -ltnpH "sport = :$1" 2>/dev/null || true
	fi
}

# require_free_port <port> <label> — refuse to start next to a stray process,
# which would otherwise answer our health checks in place of the new one.
require_free_port() {
	local owner
	owner="$(port_owner "$1")"
	[ -z "$owner" ] && return 0
	die "port $1 ($2) is already in use, probably a stray from an earlier run; stop it first (make harness-down) or kill it:
$owner"
}

# wait_http <url> <timeout-s> <label> <app-name>
# Waits for any HTTP answer at url, failing fast if the app's process dies.
wait_http() {
	local url="$1" timeout="$2" label="$3" name="$4"
	local deadline=$((SECONDS + timeout))
	until curl -fsS -o /dev/null "$url" 2>/dev/null; do
		app_alive "$name" || die "$label exited before answering at $url; see $HARNESS_TMP/$name.log:
$(tail -n 20 "$HARNESS_TMP/$name.log" 2>/dev/null)"
		[ "$SECONDS" -ge "$deadline" ] && die "$label not ready at $url after ${timeout}s; see $HARNESS_TMP/$name.log"
		sleep 0.3
	done
	log "$label is ready ($url)"
}

# wait_log <name> <text> <timeout-s>
wait_log() {
	local name="$1" text="$2" timeout="$3"
	local deadline=$((SECONDS + timeout))
	until grep -qF "$text" "$HARNESS_TMP/$name.log" 2>/dev/null; do
		app_alive "$name" || die "$name exited during startup; see $HARNESS_TMP/$name.log:
$(tail -n 20 "$HARNESS_TMP/$name.log" 2>/dev/null)"
		[ "$SECONDS" -ge "$deadline" ] && die "$name never logged \"$text\" after ${timeout}s; see $HARNESS_TMP/$name.log"
		sleep 0.3
	done
	log "$name is ready"
}

# ---- build ------------------------------------------------------------------

# build_bin <name> <package> — builds into tmp/harness/<sandbox>/bin from the working tree.
build_bin() {
	mkdir -p "$HARNESS_BIN"
	"$GO" build -o "$HARNESS_BIN/$1" "$2" || die "build of $2 failed"
}

agentctl() { "$HARNESS_BIN/agentctl" "$@"; }

ensure_agentctl() {
	[ -x "$HARNESS_BIN/agentctl" ] || build_bin agentctl ./cmd/agentctl
}

# ---- datastores -------------------------------------------------------------

deps_running() {
	$COMPOSE ps --status running --services 2>/dev/null | grep -q postgres
}

# psql_q <sql> — runs one statement in this sandbox's DB, prints unaligned rows.
psql_q() {
	$COMPOSE exec -T postgres psql -U "$HARNESS_PG_USER" -d "$HARNESS_PG_DB" \
		-v ON_ERROR_STOP=1 -tAq -c "$1"
}

# psql_admin <sql> — runs one statement in the maintenance DB (postgres).
psql_admin() {
	$COMPOSE exec -T postgres psql -U "$HARNESS_PG_USER" -d postgres \
		-v ON_ERROR_STOP=1 -tAq -c "$1"
}

rpk() {
	$COMPOSE exec -T redpanda rpk "$@" -X brokers="localhost:${HARNESS_KAFKA_BROKERS##*:}"
}

# sandbox_db_create — create this sandbox's database if it doesn't exist.
sandbox_db_create() {
	if [ -z "$(psql_admin "SELECT 1 FROM pg_database WHERE datname = '$HARNESS_PG_DB'")" ]; then
		psql_admin "CREATE DATABASE $HARNESS_PG_DB" >/dev/null || return 1
		log "created database $HARNESS_PG_DB"
	fi
}

# sandbox_topics_create — this sandbox's topics, each with .retry.1 and .dlq.
sandbox_topics_create() {
	local base topic out
	for base in $HARNESS_TOPICS; do
		for topic in "$KAFKA_TOPIC_PREFIX.$base" "$KAFKA_TOPIC_PREFIX.$base.retry.1" "$KAFKA_TOPIC_PREFIX.$base.dlq"; do
			if ! out="$(rpk topic create "$topic" -p 1 -r 1 2>&1)"; then
				echo "$out" | grep -q TOPIC_ALREADY_EXISTS || { echo "create topic $topic: $out" >&2; return 1; }
			fi
		done
	done
}

# sandbox_purge_resources <name> — drop a sandbox's DB, topics, and WireMock
# stubs from the shared infra. Best effort: missing pieces are fine.
sandbox_purge_resources() {
	local name="$1"
	psql_admin "DROP DATABASE IF EXISTS myapp_sb_$name WITH (FORCE)" >/dev/null 2>&1 ||
		warn "could not drop database myapp_sb_$name"
	rpk topic delete -r "^sb_$name\\..*" >/dev/null 2>&1 || true
	mock_remove_sandbox "$name" >/dev/null 2>&1 || true
	curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/requests/remove" \
		-d "{\"urlPathPattern\":\"/sb_$name/.*\"}" >/dev/null 2>&1 || true
}

# ---- mock server (WireMock, shared; every stub and request is scoped) -------

mock_up() { curl -fsS -o /dev/null "$HARNESS_MOCK_URL/__admin/health" 2>/dev/null; }

_mock_meta_pattern() { # <key> <value>
	jq -cn --arg k "$1" --arg v "$2" '{matchesJsonPath: {expression: ("$." + $k), equalTo: $v}}'
}

# mock_remove_sandbox [name] — drop every stub and scenario of a sandbox.
mock_remove_sandbox() {
	_mock_meta_pattern harnessSandbox "${1:-$SANDBOX}" |
		curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings/remove-by-metadata" -d @- >/dev/null
}

# mock_sandbox_mappings — this sandbox's mappings, as WireMock returns them.
mock_sandbox_mappings() {
	_mock_meta_pattern harnessSandbox "$SANDBOX" |
		curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings/find-by-metadata" -d @-
}

# mock_requests [unmatched] — this sandbox's part of WireMock's request
# journal, oldest first, as [{id, at, method, path, query, body, matched,
# route, scenario}]. path has the sandbox prefix stripped, so it reads like the
# downstream sees it (/payments/v1/...). route is the matching stub's "name";
# scenario is set when a failure scenario answered.
mock_requests() {
	curl -fsS "$HARNESS_MOCK_URL/__admin/requests" | jq -c --arg only "${1:-}" --arg pfx "/$HARNESS_MOCK_PREFIX" '
		[.requests[]
		 | select(.request.url | startswith($pfx + "/"))
		 | {id: .id,
		    at: .request.loggedDateString,
		    method: .request.method,
		    path: (.request.url | split("?")[0] | ltrimstr($pfx)),
		    query: (.request.url | split("?")[1] // null),
		    body: (.request.body as $b | if ($b // "") == "" then null else ($b | fromjson? // $b) end),
		    matched: .wasMatched,
		    route: (.stubMapping.name // null),
		    scenario: (.stubMapping.metadata.harnessScenario // null)}
		 | select($only == "" or (.matched | not))]
		| reverse'
}

# mock_stash_unmatched <label> — append this sandbox's unmatched requests to
# unmatched.jsonl, labelled, skipping the ones already there (by journal id).
# Call it BEFORE mock_clear_journal: the journal is the only other record, so
# an unstubbed call made between checks (by the e2e suite, or by hand) would
# otherwise vanish when the next check resets its probes.
mock_stash_unmatched() {
	local out="$HARNESS_TMP/unmatched.jsonl" new
	[ -d "$HARNESS_TMP" ] || return 0
	touch "$out"
	new="$(mock_requests unmatched 2>/dev/null | jq -c --arg check "$1" --slurpfile seen "$out" '
		($seen | map(.id)) as $ids
		| .[] | select(.id as $i | $ids | index($i) | not)
		| {check: $check, id, method, path, query, body}' 2>/dev/null)" || return 0
	[ -n "$new" ] && printf '%s\n' "$new" >>"$out"
	return 0
}

# mock_clear_journal — forget this sandbox's requests (others keep theirs).
mock_clear_journal() {
	curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/requests/remove" \
		-d "{\"urlPathPattern\":\"/$HARNESS_MOCK_PREFIX/.*\"}" >/dev/null
}

# mock_stub_mappings — the mappings in _harness/mocks, rewritten for this
# sandbox: every URL matcher gets the /sb_<name> prefix, ids are dropped (so
# sandboxes never overwrite each other's copies), and each is tagged with
# metadata.harnessSandbox.
mock_stub_mappings() {
	local files=() f
	for f in "$HARNESS_DIR"/mocks/*.json; do
		[ -e "$f" ] && files+=("$f")
	done
	if [ ${#files[@]} -eq 0 ]; then
		echo '[]'
		return 0
	fi
	jq -s --arg pfx "/$HARNESS_MOCK_PREFIX" --arg sb "$SANDBOX" '
		def lit: if . == null then null else $pfx + . end;
		def re: if . == null then null elif startswith("^") then "^" + $pfx + .[1:] else $pfx + . end;
		[.[] | if type == "object" and has("mappings") then .mappings[] else . end]
		| map(del(.id, .uuid)
		      | .persistent = false
		      | .metadata = ((.metadata // {}) + {harnessSandbox: $sb})
		      | .request.url |= lit
		      | .request.urlPath |= lit
		      | .request.urlPattern |= re
		      | .request.urlPathPattern |= re
		      | .request |= with_entries(select(.value != null)))' "${files[@]}"
}

# mock_reset — replace this sandbox's stubs in WireMock with exactly the ones
# in _harness/mocks (which drops its failure scenarios too), then clear its
# journal. Other sandboxes' stubs are untouched.
# The stubs are pushed through the admin API rather than mounted into the
# container: Docker VMs (colima, Docker Desktop) cache bind-mount directory
# listings, so a deleted stub file stays visible to WireMock for ~20s.
mock_reset() {
	local mappings out
	mappings="$(mock_stub_mappings)" || { echo "a stub file in _harness/mocks is not valid JSON" >&2; return 1; }
	mock_remove_sandbox || { echo "could not remove the old stubs of sandbox $SANDBOX" >&2; return 1; }
	if [ "$(printf '%s' "$mappings" | jq 'length')" != 0 ]; then
		if ! out="$(printf '%s' "$mappings" |
			jq -c '{mappings: ., importOptions: {duplicatePolicy: "OVERWRITE", deleteAllNotInImport: false}}' |
			curl -sS --fail-with-body -X POST "$HARNESS_MOCK_URL/__admin/mappings/import" -d @- 2>&1)"; then
			echo "WireMock rejected the stubs: $out" >&2
			return 1
		fi
	fi
	mock_clear_journal
}

# mock_scenario <svc> <error|timeout|malformed|empty> — make every request
# this sandbox sends under /<svc>/ fail that way until mock_clear_scenarios or
# mock_reset. It is a priority-1 runtime stub, so it wins over the stub files,
# only matches this sandbox's prefix, and is never written back to _harness/mocks.
mock_scenario() {
	local svc="${1#/}" response
	svc="${svc%/}"
	case "$2" in
		error) response='{"status":500,"jsonBody":{"error":"harness scenario: simulated downstream error"}}' ;;
		timeout) response='{"status":504,"fixedDelayMilliseconds":30000,"jsonBody":{"error":"harness scenario: simulated timeout"}}' ;;
		malformed) response='{"status":200,"headers":{"Content-Type":"application/json"},"body":"{\"harness\": \"malformed"}' ;;
		empty) response='{"status":200,"headers":{"Content-Type":"application/json"},"jsonBody":[]}' ;;
		*) echo "unknown scenario kind '$2' (want: error | timeout | malformed | empty)" >&2; return 1 ;;
	esac
	jq -cn --arg svc "$svc" --arg kind "$2" --arg pfx "/$HARNESS_MOCK_PREFIX" --arg sb "$SANDBOX" \
		--argjson response "$response" '{
		name: "harness scenario: \($svc) \($kind)",
		priority: 1,
		persistent: false,
		request: {urlPathPattern: "\($pfx)/\($svc)/.*"},
		response: $response,
		metadata: {harnessSandbox: $sb, harnessScenario: "\($svc) \($kind)", harnessScenarioOf: $sb}
	}' | curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings" -d @- >/dev/null
}

# mock_clear_scenarios — drop this sandbox's failure scenarios only.
mock_clear_scenarios() {
	_mock_meta_pattern harnessScenarioOf "$SANDBOX" |
		curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings/remove-by-metadata" -d @- >/dev/null
}

# mock_stub_count_on_disk — how many mappings _harness/mocks defines (a file
# holds one mapping, or {"mappings": [...]}).
mock_stub_count_on_disk() {
	local n=0 f c
	for f in "$HARNESS_DIR"/mocks/*.json; do
		[ -e "$f" ] || continue
		c="$(jq 'if type == "object" and has("mappings") then (.mappings | length) else 1 end' "$f")" ||
			die "stub file $f is not valid JSON"
		n=$((n + c))
	done
	echo "$n"
}

# ---- logs -------------------------------------------------------------------

# error_lines [app...] — ERROR-level lines and crashes from this sandbox's app
# logs, minus known noise (regexps in _harness/known-log-noise.txt) and the
# exact lines checks caused on purpose (--expect-errors, recorded in
# expected-lines.txt). Each line is prefixed with "<app>: ".
# Only lines after the log baseline count (log_offset): run.sh sets it when it
# keeps the apps running, which is what truncating the logs does on a restart.
error_lines() {
	local apps="${*:-api worker}" patterns="$HARNESS_TMP/.noise-patterns" app
	local expected="$HARNESS_TMP/expected-lines.txt"
	[ -d "$HARNESS_TMP" ] || return 0
	grep -vE '^[[:space:]]*(#|$)' "$HARNESS_DIR/known-log-noise.txt" >"$patterns" 2>/dev/null || true
	for app in $apps; do
		[ -f "$HARNESS_TMP/$app.log" ] || continue
		tail -c +"$(($(log_offset "$app") + 1))" "$HARNESS_TMP/$app.log" \
			| grep -E '"level":"ERROR"|^panic:|^fatal:|^fatal error:' \
			| { if [ -s "$patterns" ]; then grep -vE -f "$patterns"; else cat; fi; } \
			| { if [ -s "$expected" ]; then grep -vxF -f "$expected"; else cat; fi; } \
			| sed "s/^/$app: /" || true
	done
}

# log_offset <app> — the byte offset in the app's log where the current run
# begins (0 unless run.sh kept the apps running; see log_baseline_set).
log_offset() {
	local off
	off="$(sed -n "s/^$1=//p" "$HARNESS_TMP/log.baseline" 2>/dev/null | head -n 1)"
	echo "${off:-0}"
}

# log_baseline_set — start a new run in the existing logs, without a restart.
log_baseline_set() {
	local app size
	for app in $HARNESS_APPS; do
		size="$(wc -c <"$HARNESS_TMP/$app.log" 2>/dev/null | tr -d ' ')"
		echo "$app=${size:-0}"
	done >"$HARNESS_TMP/log.baseline"
}

# ---- build identity ---------------------------------------------------------

# tree_stamp — identifies the Go sources of this worktree: HEAD plus every
# uncommitted change to a .go file, go.mod or go.sum (untracked files too).
# run.sh records it with the build; report.sh compares, to say when the
# running apps are older than the code.
tree_stamp() {
	{
		git rev-parse HEAD 2>/dev/null
		git diff HEAD -- '*.go' go.mod go.sum 2>/dev/null
		git ls-files --others --exclude-standard -- '*.go' 2>/dev/null | while IFS= read -r f; do
			echo "$f"
			cksum <"$f" 2>/dev/null
		done
	} | cksum | tr ' ' '-'
}

# run_fingerprint — what the running apps were started from: the two binaries
# and everything that reaches them through the environment. Equal fingerprints
# mean a restart would start the very same thing.
run_fingerprint() {
	{
		cksum <"$HARNESS_BIN/api"
		cksum <"$HARNESS_BIN/worker"
		cksum <"$HARNESS_DIR/.env.harness"
		cksum <"$HARNESS_DIR/config.harness.json"
		echo "$HTTP_ADDR $GRPC_ADDR $DATABASE_URL $KAFKA_TOPIC_PREFIX $KAFKA_GROUP_ID"
	} 2>/dev/null | cksum | tr ' ' '-'
}

# ---- misc -------------------------------------------------------------------

# new_id — a fresh, lowercase, unique id for checks, so re-runs never collide
# with rows a previous run left behind. The counter lives in a file because
# new_id is almost always called in a $(...) subshell.
new_id() {
	local f="$HARNESS_TMP/vars/.seq" n
	n=$(( $(cat "$f" 2>/dev/null || echo 0) + 1 ))
	echo "$n" >"$f"
	echo "h$(date +%s)x$n"
}

git_summary() {
	local branch sha dirty=""
	branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
	sha="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
	[ -n "$(git status --porcelain 2>/dev/null)" ] && dirty=" (dirty)"
	echo "$branch@$sha$dirty"
}

# Load the slot if this sandbox already has one; scripts that need the apps
# call require_sandbox, and up.sh calls sandbox_allocate.
sandbox_load || true
