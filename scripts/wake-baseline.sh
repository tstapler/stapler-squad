#!/usr/bin/env bash
# wake-baseline.sh - Epic 4 cost baseline (plan Task 5.1f): how many ScheduleWakeup
# calls per backlog session, measured while agent_wake_triggers is OFF.
#
# Source: the "schedule_wakeup_receipt" JSON log lines that SessionService.ObservePostToolUse
# writes (fields: time, session, role), one per ScheduleWakeup PostToolUse hook receipt.
# It is the durable twin of the agent_schedule_wakeup_calls_total{role} counter, which
# resets on restart.
#
# Usage: wake-baseline.sh [--days N] [--config FILE] [--sessions-file FILE]
#                         [--include-unlinked] [--assume-flag-off] [LOG...]
#        wake-baseline.sh --self-test
#   LOG   JSON-lines log(s); default ~/.stapler-squad/logs/staplersquad.log, or the
#         instances/$STAPLER_SQUAD_INSTANCE/logs/ one when that variable is set.
#         Pass workspace-mode logs (workspaces/<hash>/logs/...) explicitly.
#   --days N            window length, default 7
#   --config FILE       config.json holding feature_flags; default <LOG dir>/../config.json
#   --sessions-file F   one session id per line: every session alive in the window, so
#                       sessions with zero calls count toward the median and p90. Without
#                       it the population is the sessions that made at least one call.
#   --include-unlinked  also count sessions with no backlog link (role unlinked/unknown)
#
# Exit 2 (and no numbers) when the observed window is shorter than --days or when
# agent_wake_triggers is on (globally or in any scope). Log rotation limits the window:
# the earliest record in the log is the earliest the window can start, so a rotated or
# truncated log can make this refuse; pass the rotated files too.
# Requires bash, jq, grep, tac.
set -euo pipefail

DAYS=7
CONFIG=""
SESSIONS_FILE=""
INCLUDE_UNLINKED=0
ASSUME_FLAG_OFF=0
NOW=""
LOGS=()

die() { echo "wake-baseline: $*" >&2; exit 2; }

# epoch of the "time" field of one JSON log line, honouring its UTC offset
# shellcheck disable=SC2016  # jq program, not shell
EPOCH_JQ='def epoch: capture("^(?<d>[0-9T:-]{19})(?:\\.[0-9]+)?(?<z>Z|[+-][0-9]{2}:[0-9]{2})$") as $m
  | ($m.d + "Z" | fromdateiso8601)
    - (if $m.z == "Z" then 0 else (($m.z[0:1] + "1" | tonumber) * ($m.z[1:3] | tonumber * 3600 + ($m.z[4:6] | tonumber) * 60)) end);'

line_epoch() { jq -r "$EPOCH_JQ"' .time | epoch' <<<"$1"; }

flag_is_on() {
  jq -e '(.feature_flags.agent_wake_triggers // false) == true
         or ([.feature_flag_scopes.agent_wake_triggers[]?] | any(. == true))' "$1" >/dev/null
}

run() {
  [[ ${#LOGS[@]} -gt 0 ]] || LOGS=("$DEFAULT_LOG")
  local log first="" last="" f l
  for log in "${LOGS[@]}"; do
    [[ -r $log ]] || die "cannot read log $log"
    f=$(grep -m1 '^{"time"' "$log" || true)
    l=$(tac "$log" | grep -m1 '^{"time"' || true)
    [[ -n $f ]] || die "no JSON log records in $log"
    f=$(line_epoch "$f"); l=$(line_epoch "$l")
    [[ -z $first || $f -lt $first ]] && first=$f
    [[ -z $last || $l -gt $last ]] && last=$l
  done
  local now=${NOW:-$(date +%s)}
  local end=$(( last < now ? last : now ))
  local start=$(( now - DAYS * 86400 ))
  (( first > start )) && start=$first
  local span=$(( end - start ))
  if (( span < DAYS * 86400 )); then
    die "observed window is $(( span > 0 ? span / 3600 : 0 )) h ($(date -u -d "@$start" +%FT%TZ) to $(date -u -d "@$end" +%FT%TZ)); need $DAYS days. Log rotation may have truncated it: pass the rotated logs too, or wait."
  fi

  if [[ -z $CONFIG ]]; then CONFIG="$(dirname "${LOGS[0]}")/../config.json"; fi
  if [[ -r $CONFIG ]]; then
    if flag_is_on "$CONFIG"; then die "agent_wake_triggers is on in $CONFIG; a baseline needs it off"; fi
  elif (( ASSUME_FLAG_OFF == 0 )); then
    die "cannot read $CONFIG to check agent_wake_triggers; pass --config or --assume-flag-off"
  fi

  local extra=""
  [[ -n $SESSIONS_FILE ]] && extra=$(jq -R . "$SESSIONS_FILE" | jq -s -c .)
  cat "${LOGS[@]}" | grep -F '"msg":"schedule_wakeup_receipt"' | jq -s -r \
    --argjson start "$start" --argjson end "$end" --argjson unlinked "$INCLUDE_UNLINKED" \
    --argjson extra "${extra:-[]}" --arg flagsrc "${CONFIG}" "$EPOCH_JQ"'
    def pct($a; $p): $a[((($p * ($a | length)) | ceil) - 1) | if . < 0 then 0 else . end];
    def median($a): ($a | length) as $n | if $n == 0 then null elif $n % 2 == 1 then $a[($n - 1) / 2] else ($a[$n / 2 - 1] + $a[$n / 2]) / 2 end;
    [ .[] | select((.time | epoch) >= $start and (.time | epoch) <= $end)
          | select($unlinked == 1 or (.role != "unlinked" and .role != "unknown")) ] as $calls
    | ($calls | group_by(.session) | map({key: .[0].session, value: length}) | from_entries) as $per
    | (($per | keys) + $extra | unique) as $sessions
    | ([$sessions[] | $per[.] // 0] | sort) as $counts
    | "capture_window_start: \($start | todate)",
      "capture_window_end:   \($end | todate)",
      "window_days:          \((($end - $start) / 86400 * 10 | floor) / 10)",
      "agent_wake_triggers:  off (\($flagsrc))",
      "sessions:             \($sessions | length) (\($per | length) with at least one call)",
      "schedule_wakeup_calls: \($calls | length)",
      "calls_per_session_median: \(median($counts))",
      "calls_per_session_p90:    \(if ($counts | length) == 0 then null else pct($counts; 0.9) end)"'
}

self_test() {
  local dir; dir=$(mktemp -d); trap 'rm -rf "$dir"' RETURN
  local now=1790000000 fail=0
  mkdir -p "$dir/logs"
  # 9 days of noise, then receipts: s1 x1, s2 x3, s3 x5, one unlinked call, one 10-day-old call
  jq -nc --argjson now "$now" '
    def at($d): ($now - $d * 86400) | todate | sub("Z$"; "-07:00") | .;
    {time: at(9), level: "INFO", msg: "boot"},
    ({time: at(8), msg: "schedule_wakeup_receipt", session: "s0", role: "work"}),
    (range(0; 1) | {time: at(6), msg: "schedule_wakeup_receipt", session: "s1", role: "work"}),
    (range(0; 3) | {time: at(5), msg: "schedule_wakeup_receipt", session: "s2", role: "triage"}),
    (range(0; 5) | {time: at(2), msg: "schedule_wakeup_receipt", session: "s3", role: "review"}),
    {time: at(1), msg: "schedule_wakeup_receipt", session: "u1", role: "unlinked"},
    {time: at(0.01), level: "INFO", msg: "tick"}' >"$dir/logs/staplersquad.log"
  echo '{"feature_flags":{"agent_wake_triggers":false}}' >"$dir/config.json"
  local out
  out=$(NOW=$now bash "$0" --now "$now" "$dir/logs/staplersquad.log") || { echo "FAIL: baseline run"; fail=1; }
  echo "$out"
  grep -q '^sessions:             3 (3 with' <<<"$out" || { echo "FAIL: session count"; fail=1; }
  grep -q 'schedule_wakeup_calls: 9$' <<<"$out" || { echo "FAIL: call count (expected 9: s0 is 8 days old, outside the window; u1 is unlinked)"; fail=1; }
  grep -q 'median: 3$' <<<"$out" || { echo "FAIL: median"; fail=1; }
  grep -q 'p90:    5$' <<<"$out" || { echo "FAIL: p90"; fail=1; }

  # zero-call sessions supplied by file pull the median down
  printf 's1\ns2\ns3\nz1\nz2\n' >"$dir/sessions.txt"
  out=$(bash "$0" --now "$now" --sessions-file "$dir/sessions.txt" "$dir/logs/staplersquad.log")
  grep -q 'median: 1$' <<<"$out" || { echo "FAIL: median with zero-call sessions"; fail=1; }

  # refusal: flag on
  echo '{"feature_flag_scopes":{"agent_wake_triggers":{"trigger:at":true}}}' >"$dir/config.json"
  bash "$0" --now "$now" "$dir/logs/staplersquad.log" >/dev/null 2>"$dir/err" && rc=0 || rc=$?
  if ! { [[ $rc -eq 2 ]] && grep -q 'is on' "$dir/err"; }; then echo "FAIL: flag-on refusal (rc=$rc)"; fail=1; fi
  echo '{}' >"$dir/config.json"

  # refusal: window shorter than 7 days
  jq -nc --argjson now "$now" '{time: (($now - 3 * 86400) | todate), msg: "boot"}, {time: ($now | todate), msg: "tick"}' >"$dir/logs/short.log"
  bash "$0" --now "$now" "$dir/logs/short.log" >/dev/null 2>"$dir/err" && rc=0 || rc=$?
  if ! { [[ $rc -eq 2 ]] && grep -q 'need 7 days' "$dir/err"; }; then echo "FAIL: short-window refusal (rc=$rc)"; fail=1; fi
  cat "$dir/err"

  if (( fail == 0 )); then echo "self-test: PASS"; else echo "self-test: FAIL"; return 1; fi
}

while (( $# )); do
  case $1 in
    --self-test) SELF_TEST=1 ;;
    --days) DAYS=$2; shift ;;
    --config) CONFIG=$2; shift ;;
    --sessions-file) SESSIONS_FILE=$2; shift ;;
    --include-unlinked) INCLUDE_UNLINKED=1 ;;
    --assume-flag-off) ASSUME_FLAG_OFF=1 ;;
    --now) NOW=$2; shift ;;
    -h|--help) sed -n '2,/^set -e/p' "$0" | sed '$d'; exit 0 ;;
    -*) die "unknown option $1" ;;
    *) LOGS+=("$1") ;;
  esac
  shift
done

STATE_DIR="$HOME/.stapler-squad"
[[ -n ${STAPLER_SQUAD_INSTANCE:-} ]] && STATE_DIR="$STATE_DIR/instances/$STAPLER_SQUAD_INSTANCE"
DEFAULT_LOG="$STATE_DIR/logs/staplersquad.log"

if [[ ${SELF_TEST:-0} == 1 ]]; then self_test; else run; fi
