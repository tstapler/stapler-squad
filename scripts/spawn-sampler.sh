#!/usr/bin/env bash
# spawn-sampler.sh - estimate short-lived process creation rate by command prefix.
# Usage: scripts/spawn-sampler.sh [seconds=60] [interval=0.5]
# Polls `ps` (no sudo), counts PIDs first seen per sample. Processes that live
# shorter than the interval are missed, so the result is a LOWER BOUND.
# Each sample itself execs ~10 short processes (ps, awk, sort, ...): the sampler
# adds load to the host it measures, so use a longer interval on a busy machine.
set -euo pipefail
dur=${1:-60}; iv=${2:-0.5}
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
: > "$tmp/seen"; : > "$tmp/new"
end=$(( $(date +%s) + dur )); first=1
while [ "$(date +%s)" -lt "$end" ]; do
  ps -axo pid=,ppid=,command= | awk '{pid=$1; ppid=$2; $1="";$2=""; sub(/^ +/,""); print pid"\t"ppid"\t"$0}' > "$tmp/cur"
  if [ "$first" = 1 ]; then cut -f1 "$tmp/cur" | sort -u > "$tmp/seen"; first=0
  else
    cut -f1 "$tmp/cur" | sort -u > "$tmp/ids"
    comm -13 "$tmp/seen" "$tmp/ids" > "$tmp/fresh"
    [ -s "$tmp/fresh" ] && awk -F'\t' 'NR==FNR{f[$1]=1;next} ($1 in f)' "$tmp/fresh" "$tmp/cur" >> "$tmp/new"
    sort -u "$tmp/seen" "$tmp/ids" > "$tmp/s2"; mv "$tmp/s2" "$tmp/seen"
  fi
  sleep "$iv"
done
n=$(awk -F'\t' -v me="$$" '$2!=me' "$tmp/new" | wc -l | tr -d ' ')
printf 'new processes seen: %s in %ss (%.2f/s, lower bound)\n\n' "$n" "$dur" "$(echo "$n/$dur" | bc -l)"
echo "by command (first 2 words, basename of path):"
awk -F'\t' -v me="$$" '$2!=me {split($3,a," "); k=a[1]; sub(/.*\//,"",k); b=a[2]; sub(/.*\//,"",b); print k" "b}' "$tmp/new" | sort | uniq -c | sort -rn | head -25
