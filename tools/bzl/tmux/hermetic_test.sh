#!/usr/bin/env bash
# S5: a build that exits 0 is not evidence the binary is hermetic. Fail if tmux dynamically
# needs libncurses/libevent (they must be linked statically) and check it reports the pinned version.
set -euo pipefail
tmux="$1" pin="$2"
needed="$(readelf -d "$tmux" | awk '/NEEDED/ {print $5}')"
if grep -E 'ncurses|libevent|tinfo' <<<"$needed"; then
  echo "tmux dynamically links a host library (see above); static link flags regressed" >&2
  exit 1
fi
want="tmux $(sed -n 's/^tag=//p' "$pin")"
got="$("$tmux" -V)"
[ "$got" = "$want" ] || { echo "version '$got' != pinned '$want'" >&2; exit 1; }
echo "ok: $got, NEEDED: ${needed//$'\n'/ }"
