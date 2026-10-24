#!/bin/sh
# workflow-engine-soak-status.sh — print the workflow engine shadow-soak verdict per caller class.
# Reads /actuator/workflow-engine from the running service (one request). Exit 0: READY;
# 1: NOT READY (no evidence, below the floor, history shorter than W, an expected edge short of N
# samples, seeded divergence, or an unacknowledged stateful edge); 2: unreachable (including a 401
# when auth is enabled; /actuator sits behind the same auth as /actuator/metrics).
# Usage: workflow-engine-soak-status.sh [--config-diff] [--class operator|agent|system] [--ack-file path] [base-url]
# --ack-file posts the owner's acknowledgements for the verdict (JSON, per class: edges, divergent_edges,
# stateful_edges, guard_new as "caller|from->to", and low_volume {config_diff, stranded_preview,
# caller_table, replay_report}); it never changes server state.
# --config-diff prints the live-config-versus-seed diff and the stranded-item preview instead.
base="${SSQ_URL:-http://localhost:8543}"
query="format=text&class=operator"
ackfile=""
while [ $# -gt 0 ]; do
  case "$1" in
    --config-diff) query="$query&config_diff=1" ;;
    --ack-file) shift; ackfile="$1" ;;
    --class)
      [ $# -ge 2 ] || { echo "--class needs operator, agent or system" >&2; exit 2; }
      shift
      case "$1" in
        operator|agent|system) query=$(printf '%s' "$query" | sed "s/class=[a-z]*/class=$1/") ;;
        *) echo "unknown class: $1 (want operator, agent or system)" >&2; exit 2 ;;
      esac ;;
    -*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) base="$1" ;;
  esac
  shift
done
tmp=$(mktemp -d) || exit 2
trap 'rm -rf "$tmp"' EXIT
set -- -sS -D "$tmp/headers" -o "$tmp/body"
if [ -n "$ackfile" ]; then
  [ -r "$ackfile" ] || { echo "cannot read ack file $ackfile" >&2; exit 2; }
  set -- "$@" -H "Content-Type: application/json" --data-binary "@$ackfile"
fi
curl "$@" "$base/actuator/workflow-engine?$query" \
  || { echo "cannot reach $base/actuator/workflow-engine (is the service up, is auth enabled?)" >&2; exit 2; }
cat "$tmp/body"
code=$(sed -n 's/^[Xx]-[Ww]orkflow-[Ee]ngine-[Ss]oak-[Ee]xit: *\([0-9]*\).*/\1/p' "$tmp/headers" | tr -d '\r')
exit "${code:-2}"
