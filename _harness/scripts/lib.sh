#!/usr/bin/env bash
# Shared helpers for _harness/scripts/*.sh. Sourced, never run directly.
#
# Portability: this must run on macOS's bash 3.2, so no associative arrays,
# no ${var,,}, no mapfile, no $EPOCHREALTIME, and empty arrays are expanded
# with ${a[@]+"${a[@]}"} so `set -u` doesn't trip on them.

HARNESS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$HARNESS_ROOT"

HARNESS_DIR="_harness"
HARNESS_TMP="tmp/harness"
HARNESS_BIN="$HARNESS_TMP/bin"
HARNESS_PIDS="$HARNESS_TMP/pids"
mkdir -p "$HARNESS_BIN" "$HARNESS_PIDS" "$HARNESS_TMP/responses" "$HARNESS_TMP/vars"

GO="${GO:-go}"

set -a
# shellcheck source=/dev/null
. "$HARNESS_DIR/.env.harness"
set +a

COMPOSE="docker compose -p myapp-harness -f $HARNESS_DIR/docker-compose.yml"

# Host processes the harness starts (see run.sh); down.sh stops them.
# WireMock is not one of them: it runs in compose.
HARNESS_APPS="api worker"

log()  { echo "==> $*"; }
warn() { echo "warning: $*" >&2; }
die()  { echo "error: $*" >&2; exit 1; }

# ---- lock -------------------------------------------------------------------
# One harness run per checkout at a time. The lock is a pid file; a lock whose
# pid is gone is stale and is taken over. Nested scripts (verify -> up -> ...)
# inherit the lock through HARNESS_LOCK_HELD instead of re-taking it.

acquire_lock() {
	[ "${HARNESS_LOCK_HELD:-}" = "1" ] && return 0
	local lock="$HARNESS_TMP/.lock"
	if ( set -o noclobber; echo "$$" >"$lock" ) 2>/dev/null; then
		:
	else
		local holder
		holder="$(cat "$lock" 2>/dev/null || true)"
		if [ -n "$holder" ] && kill -0 "$holder" 2>/dev/null; then
			die "another harness command (pid $holder) holds $lock; wait for it, or kill it if it is stuck"
		fi
		warn "taking over stale lock from pid ${holder:-unknown}"
		echo "$$" >"$lock"
	fi
	export HARNESS_LOCK_HELD=1
	trap 'release_lock' EXIT
}

release_lock() {
	local lock="$HARNESS_TMP/.lock"
	[ "$(cat "$lock" 2>/dev/null || true)" = "$$" ] && rm -f "$lock"
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

# build_bin <name> <package> — builds into tmp/harness/bin from the working tree.
build_bin() {
	"$GO" build -o "$HARNESS_BIN/$1" "$2" || die "build of $2 failed"
}

agentctl() { "$HARNESS_BIN/agentctl" "$@"; }

ensure_agentctl() {
	[ -x "$HARNESS_BIN/agentctl" ] || build_bin agentctl ./cmd/agentctl
}

# ---- datastores -------------------------------------------------------------

# psql_q <sql> — runs one statement in the harness DB, prints unaligned rows.
psql_q() {
	$COMPOSE exec -T postgres psql -U "$HARNESS_PG_USER" -d "$HARNESS_PG_DB" \
		-v ON_ERROR_STOP=1 -tAq -c "$1"
}

deps_running() {
	$COMPOSE ps --status running --services 2>/dev/null | grep -q postgres
}

# ---- mock server (WireMock) ---------------------------------------------------

mock_up() { curl -fsS -o /dev/null "$HARNESS_MOCK_URL/__admin/health" 2>/dev/null; }

# mock_requests [unmatched] — WireMock's request journal, oldest first, as
# [{at, method, path, query, body, matched, route, scenario}]. route is the
# matching stub's "name"; scenario is set when a failure scenario answered.
mock_requests() {
	curl -fsS "$HARNESS_MOCK_URL/__admin/requests" | jq -c --arg only "${1:-}" '
		[.requests[]
		 | {at: .request.loggedDateString,
		    method: .request.method,
		    path: (.request.url | split("?")[0]),
		    query: (.request.url | split("?")[1] // null),
		    body: (.request.body as $b | if ($b // "") == "" then null else ($b | fromjson? // $b) end),
		    matched: .wasMatched,
		    route: (.stubMapping.name // null),
		    scenario: (.stubMapping.metadata.harnessScenario // null)}
		 | select($only == "" or (.matched | not))]
		| reverse'
}

mock_clear_journal() { curl -fsS -X DELETE "$HARNESS_MOCK_URL/__admin/requests" >/dev/null; }

# mock_reset — replace every stub in WireMock with exactly the ones in
# _harness/mocks (which drops failure scenarios too), then clear the journal.
# The stubs are pushed through the admin API rather than mounted into the
# container: Docker VMs (colima, Docker Desktop) cache bind-mount directory
# listings, so a deleted stub file stays visible to WireMock for ~20s.
mock_reset() {
	local files=() f out
	for f in "$HARNESS_DIR"/mocks/*.json; do
		[ -e "$f" ] && files+=("$f")
	done
	if ! out="$({
		if [ ${#files[@]} -gt 0 ]; then
			jq -s '[.[] | if type == "object" and has("mappings") then .mappings[] else . end]' "${files[@]}"
		else
			echo '[]'
		fi
	} | jq -c '{mappings: map(. + {persistent: false}),
	            importOptions: {duplicatePolicy: "OVERWRITE", deleteAllNotInImport: true}}' \
		| curl -sS --fail-with-body -X POST "$HARNESS_MOCK_URL/__admin/mappings/import" -d @- 2>&1)"; then
		echo "WireMock rejected the stubs: $out" >&2
		return 1
	fi
	mock_clear_journal
}

# mock_scenario <svc> <error|timeout|malformed|empty> — make every request
# under /<svc>/ fail that way until mock_clear_scenarios or mock_reset. It is
# a priority-1 runtime stub, so it wins over the stub files, and it is never
# written back to _harness/mocks.
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
	jq -cn --arg svc "$svc" --arg kind "$2" --argjson response "$response" '{
		name: "harness scenario: \($svc) \($kind)",
		priority: 1,
		persistent: false,
		request: {urlPathPattern: "/\($svc)/.*"},
		response: $response,
		metadata: {harnessScenario: "\($svc) \($kind)"}
	}' | curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings" -d @- >/dev/null
}

mock_clear_scenarios() {
	curl -fsS -X POST "$HARNESS_MOCK_URL/__admin/mappings/remove-by-metadata" \
		-d '{"matchesJsonPath":"$.harnessScenario"}' >/dev/null
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

# error_lines [app...] — ERROR-level lines and crashes from the app logs, minus
# known noise (regexps in _harness/known-log-noise.txt) and the exact lines
# checks caused on purpose (--expect-errors, recorded in expected-lines.txt).
# Each line is prefixed with "<app>: ".
error_lines() {
	local apps="${*:-api worker}" patterns="$HARNESS_TMP/.noise-patterns" app
	local expected="$HARNESS_TMP/expected-lines.txt"
	grep -vE '^[[:space:]]*(#|$)' "$HARNESS_DIR/known-log-noise.txt" >"$patterns" 2>/dev/null || true
	for app in $apps; do
		[ -f "$HARNESS_TMP/$app.log" ] || continue
		grep -E '"level":"ERROR"|^panic:|^fatal:|^fatal error:' "$HARNESS_TMP/$app.log" \
			| { if [ -s "$patterns" ]; then grep -vE -f "$patterns"; else cat; fi; } \
			| { if [ -s "$expected" ]; then grep -vxF -f "$expected"; else cat; fi; } \
			| sed "s/^/$app: /" || true
	done
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
