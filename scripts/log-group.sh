#!/usr/bin/env bash
# log-group.sh — Datadog-style "log pattern" clustering for stapler-squad's
# JSON-lines logs. Groups lines by (level, msg) with dynamic substrings
# (UUIDs, paths, numbers, quoted blobs) normalized out, so structurally
# identical messages collapse into one row even when a value happens to be
# interpolated into the message text instead of a separate JSON field.
#
# See docs/how-to/debug-with-logs.md for when/why to reach for this.
#
# Usage:
#   log-group.sh [-n TOP] [-l LEVEL] [FILE ...]
#
#   -n TOP     show only the top N patterns (default 30)
#   -l LEVEL   only include this level (INFO, WARN, ERROR, DEBUG) — case-insensitive
#   FILE ...   one or more log files; defaults to the live log at
#              ~/.stapler-squad/logs/staplersquad.log. ".gz" files are
#              decompressed automatically. Use "-" to read JSON-lines from stdin.
#
# Examples:
#   log-group.sh                                  # top 30 patterns in the live log
#   log-group.sh -n 50 ~/.stapler-squad/logs/*.log.gz
#   log-group.sh -l WARN
#   journalctl --user -u stapler-squad -o cat | log-group.sh -

set -eu
# Not pipefail: `head` intentionally closes its stdin early once it has
# enough lines, which sends SIGPIPE up the pipeline (jq/sed exit 141) — that
# is expected truncation, not a real failure.

top=30
level_filter=""
while getopts "n:l:h" opt; do
  case "$opt" in
    n) top="$OPTARG" ;;
    l) level_filter="$(echo "$OPTARG" | tr '[:lower:]' '[:upper:]')" ;;
    h)
      sed -n '2,25p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option" >&2
      exit 1
      ;;
  esac
done
shift $((OPTIND - 1))

files=("$@")
if [ "${#files[@]}" -eq 0 ]; then
  files=("$HOME/.stapler-squad/logs/staplersquad.log")
fi

cat_any() {
  for f in "${files[@]}"; do
    if [ "$f" = "-" ]; then
      cat
    elif [[ "$f" == *.gz ]]; then
      gzip -dc -- "$f"
    else
      cat -- "$f"
    fi
  done
}

cat_any \
  | jq -r --arg lvl "$level_filter" '
      select(.msg != null)
      | select($lvl == "" or (.level // "INFO") == $lvl)
      | "\(.level // "INFO")\t\(.msg)"
    ' 2>/dev/null \
  | sed -E \
      -e 's/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/<uuid>/g' \
      -e 's#(/[A-Za-z0-9_.-]+){2,}#<path>#g' \
      -e 's/\{[^{}]*\}/<obj>/g' \
      -e 's/[0-9]+/<n>/g' \
  | sort \
  | uniq -c \
  | sort -rn \
  | head -n "$top" \
  | awk '{count=$1; $1=""; printf "%6d  %s\n", count, substr($0,2)}'
