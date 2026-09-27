#!/usr/bin/env bash
# Shared helpers for scripts/agent/*.sh. Not meant to be run directly.

log()  { echo "==> $*"; }
die()  { echo "error: $*" >&2; exit 1; }

# wait_for_http <url> <timeout-seconds> <label> [pid]
# Polls url until it returns any HTTP status (i.e. something is listening and
# answering), or dies after timeout.
#
# If [pid] is given, it must be the PID of the process that is *supposed* to
# be answering at url (as recorded by start_bg). Without this, a stray
# process left over from a previous, uncleanly-stopped run — e.g. one that
# still holds the port after this run's own process failed to bind and
# exited — can make the wait falsely succeed against the WRONG process. With
# it, the wait fails fast and says so as soon as pid dies, instead of timing
# out or (worse) silently passing against a stranger's process.
wait_for_http() {
	local url="$1" timeout="${2:-30}" label="${3:-$1}" pid="${4:-}"
	local deadline=$((SECONDS + timeout))
	while true; do
		if curl -fsS -o /dev/null "$url" 2>/dev/null; then
			# A successful response only counts if it's OUR process answering —
			# not a stray from a previous run still holding the port (checked
			# here too, not just on the failure path below, since a stray that
			# answers instantly would otherwise never hit that check).
			if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
				die "$label answered at $url, but pid $pid is gone — that's a STRAY process still holding the port from a previous run, not this one; kill it (check 'lsof -i' the port) and retry"
			fi
			break
		fi
		if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
			die "$label (pid $pid) exited before answering at $url — check its log"
		fi
		if (( SECONDS >= deadline )); then
			die "$label never became ready at $url (waited ${timeout}s)"
		fi
		sleep 0.5
	done
	log "$label is ready ($url)"
}

# wait_for_cmd <timeout-seconds> <label> [pid] -- <command...>
# Polls a command until it exits 0, or dies after timeout. [pid], if given, is
# checked the same way as in wait_for_http — see its comment.
wait_for_cmd() {
	local timeout="$1" label="$2"
	shift 2
	local pid=""
	if [ "${1:-}" != "--" ]; then
		pid="$1"
		shift
	fi
	[ "${1:-}" = "--" ] && shift
	local deadline=$((SECONDS + timeout))
	until "$@" >/dev/null 2>&1; do
		if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
			die "$label (pid $pid) exited before becoming ready — check its log"
		fi
		if (( SECONDS >= deadline )); then
			die "$label never became ready (waited ${timeout}s)"
		fi
		sleep 0.5
	done
	log "$label is ready"
}

# start_bg <name> <logfile> -- <command...>
# Runs command in the background, redirected to logfile, and records its PID
# in PIDS[<name>] for stop_all to clean up later.
declare -A PIDS
start_bg() {
	local name="$1" logfile="$2"
	shift 2
	[ "$1" = "--" ] && shift
	log "starting $name (log: $logfile)"
	("$@" >"$logfile" 2>&1) &
	PIDS["$name"]=$!
}

# stop_all — kills every process started with start_bg in *this* script
# invocation, quietly. Only useful within a single run (see stop_pid for
# killing a PID recorded by a previous, separate invocation).
stop_all() {
	for name in "${!PIDS[@]}"; do
		stop_pid "$name" "${PIDS[$name]}"
	done
}

# stop_pid <label> <pid> — SIGTERMs pid if alive and waits briefly for it to
# exit. Unlike `wait`, this works even when pid isn't this shell's child
# (e.g. it was recorded by a previous script invocation, as stack-down.sh
# does), by polling `kill -0` instead.
stop_pid() {
	local label="$1" pid="$2"
	[ -z "$pid" ] && return 0
	if ! kill -0 "$pid" 2>/dev/null; then
		return 0
	fi
	log "stopping $label (pid $pid)"
	kill "$pid" 2>/dev/null || true
	local deadline=$((SECONDS + 10))
	while kill -0 "$pid" 2>/dev/null; do
		if (( SECONDS >= deadline )); then
			log "$label (pid $pid) didn't exit in time, sending SIGKILL"
			kill -9 "$pid" 2>/dev/null || true
			break
		fi
		sleep 0.2
	done
}
