#!/usr/bin/env bash
# The smoke-check DSL. Sourced by smoke.sh (after lib.sh); the check files in
# _harness/smoke/ are plain bash that call `check` once per behavior.
#
# A check is one acceptance criterion: reset the probes, fire ONE trigger, then
# poll every positive probe until all hold (or --timeout), then assert the
# negative probes, then assert that no request reached WireMock unstubbed.
#
#   check --name <id>                          required, unique, kebab-case
#     trigger (exactly one):
#       --path <path> [--method M] [--header 'K: V']... [--body <json>]
#                                              HTTP request to the api. GET/HEAD are
#                                              re-sent on every poll; other methods are sent once.
#       --produce <event-name> --key <aggregate-id> --body <json-payload>
#                                              produce an integration event to Kafka exactly as
#                                              the app would (agentctl kafka publish)
#     positive probes (polled until they all hold):
#       --status <code>                        HTTP status
#       --jq <expr>                            jq expression over the response body must be true
#       --calls '<METHOD> <path>'              the app called WireMock (path may end in *)
#       --events <name>[@<aggregate-id>]       an integration event was published
#       --await '<sql>' '<value>'              first column of the first row equals value
#       --logs <regex>                         a new api/worker log line matches (grep -E)
#       --dead-letter                          (with --produce) the message reached a .dlq topic
#     negative probes (checked once, after a settle delay):
#       --no-calls <prefix>                    no request under /<prefix>
#       --no-events                            no integration event published at all
#       (with --produce, implied)              no message for --key on a retry or DLQ topic
#     other:
#       --save <var>=<jq expr>                 save a value from the response; read it with $(var <var>)
#       --expect-errors <regex>                error log lines this check causes on purpose: the
#                                              matching lines logged DURING this check are not
#                                              counted as unexpected in the report
#       --timeout <seconds>                    default $HARNESS_CHECK_TIMEOUT (15)
#
# Results are appended to tmp/harness/results.tsv; responses are saved in
# tmp/harness/responses/<name>.json; a failing check's evidence is written to
# tmp/harness/checks/<name>.txt.

HARNESS_CHECK_TIMEOUT="${HARNESS_CHECK_TIMEOUT:-15}"
HARNESS_SETTLE="${HARNESS_SETTLE:-1.5}"
HARNESS_FILTER="${HARNESS_FILTER:-}"
CHECK_RESULTS="$HARNESS_TMP/results.tsv"
mkdir -p "$HARNESS_TMP/checks"

# var <name> — a value saved by an earlier check's --save.
var() {
	if [ -f "$HARNESS_TMP/vars/$1" ]; then
		cat "$HARNESS_TMP/vars/$1"
	else
		warn "var $1 was never saved (did the check that saves it fail or get filtered out?)"
	fi
}

_check_record() { # <name> <PASS|FAIL> <seconds> <reasons>
	local reasons
	reasons="$(printf '%s' "$4" | tr '\t\n' '  ')"
	printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$reasons" >>"$CHECK_RESULTS"
	if [ "$2" = PASS ]; then
		printf 'PASS %s (%ss)\n' "$1" "$3"
	else
		printf 'FAIL %s (%ss): %s\n' "$1" "$3" "$reasons"
	fi
}

_check_mark_logs() {
	CHECK_LOG_OFFSETS=""
	local app size
	for app in api worker; do
		size="$(wc -c <"$HARNESS_TMP/$app.log" 2>/dev/null | tr -d ' ')"
		CHECK_LOG_OFFSETS="$CHECK_LOG_OFFSETS $app:${size:-0}"
	done
}

_check_new_logs() { # [raw] — lines logged since the check started; "raw" drops the "<app>: " prefix
	local pair app off
	for pair in $CHECK_LOG_OFFSETS; do
		app="${pair%%:*}"
		off="${pair#*:}"
		if [ "${1:-}" = raw ]; then
			tail -c +"$((off + 1))" "$HARNESS_TMP/$app.log" 2>/dev/null
		else
			tail -c +"$((off + 1))" "$HARNESS_TMP/$app.log" 2>/dev/null | sed "s/^/$app: /"
		fi
	done
}

_check_events() { # [topic-regex] [extra agentctl args...]
	local re="${1:-\\.events\$}"
	shift || true
	agentctl kafka events -brokers "$HARNESS_KAFKA_BROKERS" \
		-since-mark "$HARNESS_TMP/events.mark" -topic-regex "$re" "$@" 2>&1
}

check() {
	local name="" method="GET" path="" body="" status="" produce="" key=""
	local timeout="$HARNESS_CHECK_TIMEOUT" no_events=0 dead_letter=0
	local jqs=() calls=() nocalls=() events=() await_sql=() await_val=()
	local logs=() headers=() saves=() expect_errors=()

	while [ $# -gt 0 ]; do
		case "$1" in
			--name) name="$2"; shift 2 ;;
			--method) method="$2"; shift 2 ;;
			--path) path="$2"; shift 2 ;;
			--header) headers+=("$2"); shift 2 ;;
			--body) body="$2"; shift 2 ;;
			--status) status="$2"; shift 2 ;;
			--jq) jqs+=("$2"); shift 2 ;;
			--save) saves+=("$2"); shift 2 ;;
			--calls) calls+=("$2"); shift 2 ;;
			--no-calls) nocalls+=("$2"); shift 2 ;;
			--events) events+=("$2"); shift 2 ;;
			--no-events) no_events=1; shift ;;
			--produce) produce="$2"; shift 2 ;;
			--key) key="$2"; shift 2 ;;
			--dead-letter) dead_letter=1; shift ;;
			--await) await_sql+=("$2"); await_val+=("$3"); shift 3 ;;
			--logs) logs+=("$2"); shift 2 ;;
			--expect-errors) expect_errors+=("$2"); shift 2 ;;
			--timeout) timeout="$2"; shift 2 ;;
			*) _check_record "${name:-unnamed}" FAIL 0 "bad check definition: unknown option $1"; return 0 ;;
		esac
	done

	if [ -z "$name" ]; then
		_check_record unnamed FAIL 0 "bad check definition: --name is required"
		return 0
	fi
	if [ -n "$HARNESS_FILTER" ] && ! printf '%s' "$name" | grep -Eq -- "$HARNESS_FILTER"; then
		return 0
	fi
	if { [ -n "$path" ] && [ -n "$produce" ]; } || { [ -z "$path" ] && [ -z "$produce" ]; }; then
		_check_record "$name" FAIL 0 "bad check definition: exactly one of --path or --produce is required"
		return 0
	fi
	if [ -n "$produce" ] && [ -z "$key" ]; then
		_check_record "$name" FAIL 0 "bad check definition: --produce needs --key"
		return 0
	fi

	local resp="$HARNESS_TMP/responses/$name.json"
	local evidence="$HARNESS_TMP/checks/$name.txt"
	local started=$SECONDS reasons="" http_status="" i

	# ---- reset: mock journal, event tap, log offsets
	rm -f "$resp" "$evidence"
	if ! mock_clear_journal 2>/dev/null; then
		_check_record "$name" FAIL 0 "WireMock is not reachable at $HARNESS_MOCK_URL"
		return 0
	fi
	local markerr
	if ! markerr="$(agentctl kafka mark -brokers "$HARNESS_KAFKA_BROKERS" -o "$HARNESS_TMP/events.mark" 2>&1)"; then
		_check_record "$name" FAIL 0 "event tap: $markerr"
		return 0
	fi
	_check_mark_logs

	# ---- trigger
	_check_http() {
		local args=(-sS -o "$resp" -w '%{http_code}' -X "$method" --max-time 15) h
		for h in ${headers[@]+"${headers[@]}"}; do
			args+=(-H "$h")
		done
		if [ -n "$body" ]; then
			args+=(-H 'Content-Type: application/json' --data-binary "$body")
		fi
		http_status="$(curl "${args[@]}" "$HARNESS_API_URL$path" 2>/dev/null)" || http_status="000"
	}
	if [ -n "$produce" ]; then
		local puberr
		if ! puberr="$(agentctl kafka publish -brokers "$HARNESS_KAFKA_BROKERS" \
			-name "$produce" -aggregate-id "$key" -payload "$body" 2>&1)"; then
			_check_record "$name" FAIL $((SECONDS - started)) "produce $produce failed: $puberr"
			return 0
		fi
	else
		_check_http
	fi

	# ---- positive probes, polled
	local resend=0 first=1 deadline=$((timeout + SECONDS)) journal evs got
	case "$method" in GET | HEAD) resend=1 ;; esac
	while :; do
		reasons=""
		if [ -n "$path" ] && [ "$first" = 0 ] && [ "$resend" = 1 ]; then
			_check_http
		fi
		first=0

		if [ -n "$path" ]; then
			if [ -n "$status" ] && [ "$http_status" != "$status" ]; then
				reasons="${reasons}status $http_status, want $status; "
			fi
			for i in ${jqs[@]+"${jqs[@]}"}; do
				jq -e "$i" "$resp" >/dev/null 2>&1 || reasons="${reasons}jq is not true: $i; "
			done
			# A non-idempotent request isn't re-sent, so its response can't change.
			if [ -n "$reasons" ] && [ "$resend" = 0 ]; then
				break
			fi
		fi

		if [ ${#calls[@]} -gt 0 ]; then
			journal="$(mock_requests 2>/dev/null || echo '[]')"
			for i in "${calls[@]}"; do
				printf '%s' "$journal" | jq -e --arg m "${i%% *}" --arg p "${i#* }" \
					'any(.[]; .method == $m and (if ($p | endswith("*")) then (.path | startswith($p | rtrimstr("*"))) else .path == $p end))' \
					>/dev/null || reasons="${reasons}no call: $i; "
			done
		fi

		if [ ${#events[@]} -gt 0 ]; then
			evs="$(_check_events)"
			for i in "${events[@]}"; do
				local ev_name="${i%%@*}" ev_agg=""
				case "$i" in *@*) ev_agg="${i#*@}" ;; esac
				printf '%s\n' "$evs" | jq -e -s --arg n "$ev_name" --arg a "$ev_agg" \
					'any(.[]; .name == $n and ($a == "" or .aggregate_id == $a))' \
					>/dev/null 2>&1 || reasons="${reasons}no event: $i; "
			done
		fi

		if [ "$dead_letter" = 1 ]; then
			evs="$(_check_events '\.dlq$' -aggregate-id "$key")"
			[ -n "$evs" ] || reasons="${reasons}message for $key never reached a .dlq topic; "
		fi

		i=0
		while [ "$i" -lt ${#await_sql[@]} ]; do
			got="$(psql_q "${await_sql[$i]}" 2>&1 | head -n 1)"
			[ "$got" = "${await_val[$i]}" ] ||
				reasons="${reasons}await got '$got', want '${await_val[$i]}': ${await_sql[$i]}; "
			i=$((i + 1))
		done

		for i in ${logs[@]+"${logs[@]}"}; do
			_check_new_logs | grep -Eq -- "$i" || reasons="${reasons}no log line matches /$i/; "
		done

		[ -z "$reasons" ] && break
		[ "$SECONDS" -ge "$deadline" ] && break
		sleep 0.3
	done

	# ---- negative probes, once the positives hold
	if [ -z "$reasons" ]; then
		if [ ${#nocalls[@]} -gt 0 ] || [ "$no_events" = 1 ] || [ -n "$produce" ]; then
			sleep "$HARNESS_SETTLE"
		fi
		if [ ${#nocalls[@]} -gt 0 ]; then
			journal="$(mock_requests 2>/dev/null || echo '[]')"
			for i in "${nocalls[@]}"; do
				i="/${i#/}"
				printf '%s' "$journal" | jq -e --arg p "$i" 'any(.[]; .path | startswith($p))' >/dev/null &&
					reasons="${reasons}unexpected call under $i; "
			done
		fi
		if [ "$no_events" = 1 ]; then
			evs="$(_check_events)"
			[ -n "$evs" ] && reasons="${reasons}unexpected events: $(printf '%s\n' "$evs" | jq -r -s '[.[].name] | join(", ")' 2>/dev/null); "
		fi
		if [ -n "$produce" ] && [ "$dead_letter" = 0 ]; then
			evs="$(_check_events '\.(retry\.[0-9]+|dlq)$' -aggregate-id "$key")"
			[ -n "$evs" ] && reasons="${reasons}message for $key landed on $(printf '%s\n' "$evs" | jq -r -s '[.[].topic] | unique | join(", ")' 2>/dev/null) (a handler failed; see make harness-logs ARGS=errors); "
		fi
	fi

	# ---- always: nothing reached WireMock unstubbed
	local unmatched
	unmatched="$(mock_requests unmatched 2>/dev/null || echo '[]')"
	if [ "$(printf '%s' "$unmatched" | jq 'length')" != 0 ]; then
		reasons="${reasons}unmatched downstream calls: $(printf '%s' "$unmatched" | jq -r '[.[] | .method + " " + .path] | join(", ")'); "
		printf '%s' "$unmatched" | jq -c --arg check "$name" '.[] | {check: $check, method, path, query, body}' \
			>>"$HARNESS_TMP/unmatched.jsonl"
	fi

	# ---- expected errors: excuse exactly the matching lines logged during
	# this check, so the same message from a later, genuine failure still counts.
	for i in ${expect_errors[@]+"${expect_errors[@]}"}; do
		_check_new_logs raw | grep -E -- "$i" >>"$HARNESS_TMP/expected-lines.txt" || true
	done

	# ---- saves (even on failure, so a dependent check fails with a clear cause)
	for i in ${saves[@]+"${saves[@]}"}; do
		jq -r "${i#*=}" "$resp" >"$HARNESS_TMP/vars/${i%%=*}" 2>/dev/null || rm -f "$HARNESS_TMP/vars/${i%%=*}"
	done

	if [ -z "$reasons" ]; then
		_check_record "$name" PASS $((SECONDS - started)) ""
		return 0
	fi

	{
		echo "check:   $name"
		echo "reasons: $reasons"
		[ -n "$path" ] && echo "request: $method $path ${body:+body=$body}"
		[ -n "$produce" ] && echo "produce: $produce key=$key payload=$body"
		[ -n "$path" ] && echo "status:  $http_status" && echo "response:" && head -c 4000 "$resp" 2>/dev/null && echo
		echo "events since the check started:"
		_check_events '.' | jq -c '{topic, name, aggregate_id, payload, headers}' 2>/dev/null
		echo "mock requests since the check started:"
		mock_requests 2>/dev/null | jq -c '.[] | {method, path, matched, route, scenario}'
		echo "app log lines since the check started (last 40):"
		_check_new_logs | tail -n 40
	} >"$evidence" 2>&1
	_check_record "$name" FAIL $((SECONDS - started)) "$reasons(evidence: $evidence)"
	return 0
}
