#!/usr/bin/env bash
# Slot-holder wall-time history (one line per finished build/test: "<epoch> <secs> <cold|warm>").
# Source it for slot_history_record / slot_history_median, or run:
#   slot_history.sh run <exit-file|-> <cold|warm|none> -- <command...>
# `run` executes the command, records its wall time (skipped for kind none), and writes
# "exit=<n> finished=<epoch>" to the exit file, then exits with the command's status.
# Kept separate from tools/bazel so a detached run can finish the bookkeeping after the wrapper exits.
slot_history_file() {
  printf '%s\n' "${BAZEL_SLOT_HISTORY:-${XDG_STATE_HOME:-$HOME/.local/state}/stapler-squad/bazel-slot-history}"
}

slot_history_record() { # <secs> <kind>
  local f lines
  f=$(slot_history_file)
  mkdir -p "$(dirname "$f")" 2>/dev/null || return 0
  printf '%s %s %s\n' "$(date +%s)" "$1" "$2" >> "$f" 2>/dev/null || return 0
  lines=$(wc -l < "$f" 2>/dev/null || echo 0)
  if [ "$lines" -gt 200 ]; then tail -n 100 "$f" > "$f.tmp.$$" && mv "$f.tmp.$$" "$f"; fi
  return 0
}

# Median seconds of the last <count> samples of <kind> (empty kind = any); prints nothing and
# returns 1 when fewer than <min> samples exist.
slot_history_median() { # <kind|""> <count> [min]
  local f
  f=$(slot_history_file)
  [ -f "$f" ] || return 1
  awk -v kind="$1" '(kind == "" || $3 == kind) { print $2 }' "$f" | tail -n "$2" | sort -n | awk -v min="${3:-1}" '
    { v[NR] = $1 }
    END { if (NR < min || NR == 0) exit 1; print v[int((NR + 1) / 2)] }'
}

slot_history_run() {
  local exitfile=$1 kind=$2 start rc=0 tmp
  shift 3
  start=$(date +%s)
  "$@" || rc=$?
  [ "$kind" = none ] || slot_history_record "$(($(date +%s) - start))" "$kind"
  if [ "$exitfile" != "-" ]; then
    tmp="$exitfile.tmp.$$"
    printf 'exit=%s finished=%s\n' "$rc" "$(date +%s)" > "$tmp" && mv "$tmp" "$exitfile"
  fi
  return "$rc"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  case "${1:-}" in
    run) shift; slot_history_run "$@"; exit $? ;;
    *) echo "usage: slot_history.sh run <exit-file|-> <cold|warm|none> -- <command...>" >&2; exit 2 ;;
  esac
fi
