#!/usr/bin/env bash
# Report (default) or remove Bazel state whose worktree is gone. Dry run unless --apply.
#   bazel-orphan-outputbases.sh [--apply] [--tmp]
# Lists: orphan output bases (DO_NOT_BUILD_HERE names a workspace that no longer exists), their live servers,
# install bases older than 30 days (report only; Bazel's own install-base GC collects them), the shared disk
# cache size against its GC cap, and stale test temp directories (older than 24 h, no open handles).
# --apply removes exactly the orphan bases (killing a live server first); --tmp adds the stale temp dirs.
# It never removes a base whose workspace exists, the shared disk cache, or the repository cache.
# Env (tests override): BAZEL_USER_ROOT, DISK_CACHE, DISK_CACHE_CAP_GB, TMP_ROOT, ORPHAN_TMP_PATTERNS, PROC_ROOT, FUSER.
set -u
apply=0; tmp=0
for a in "$@"; do
  case "$a" in
    --apply) apply=1 ;;
    --tmp) tmp=1 ;;
    *) echo "usage: $0 [--apply] [--tmp]" >&2; exit 2 ;;
  esac
done
user_root="${BAZEL_USER_ROOT:-$HOME/.cache/bazel/_bazel_${USER:-$(id -un)}}"
disk_cache="${DISK_CACHE:-$HOME/.cache/stapler-squad-bazel/disk}"
cap_gb="${DISK_CACHE_CAP_GB:-100}"
tmp_root="${TMP_ROOT:-/tmp}"
tmp_patterns="${ORPHAN_TMP_PATTERNS:-ssq-*}"
proc_root="${PROC_ROOT:-/proc}"
fuser_bin="${FUSER:-fuser}"
mode="dry run"; [ "$apply" = 1 ] && mode="apply"
echo "bazel-orphan-outputbases ($mode)"

human() { awk -v k="$1" 'BEGIN { printf "%.1f MB", k / 1024 }'; }

# Prints the pid of the live server of base $1 if its cmdline names --output_base=<base>, else nothing.
live_server_pid() {
  local base="$1" pid cmdline
  [ -f "$base/server/server.pid.txt" ] || return 0
  pid=$(tr -dc '0-9' < "$base/server/server.pid.txt")
  [ -n "$pid" ] && [ -r "$proc_root/$pid/cmdline" ] || return 0
  cmdline=$(tr '\0' '\n' < "$proc_root/$pid/cmdline")
  if printf '%s\n' "$cmdline" | grep -Fxq -- "--output_base=$base"; then echo "$pid"; fi
}

kill_server() {
  local pid="$1" i=0
  kill "$pid" 2>/dev/null
  while [ -d "$proc_root/$pid" ] && [ "$i" -lt 50 ]; do sleep 0.2; i=$((i + 1)); done
  if [ -d "$proc_root/$pid" ]; then kill -9 "$pid" 2>/dev/null; sleep 0.5; fi
  [ ! -d "$proc_root/$pid" ]
}

remove_tree() { chmod -R u+w "$1" 2>/dev/null; rm -rf "$1"; }

echo "== orphan output bases ($user_root)"
orphans=0
if [ -d "$user_root" ]; then
  for base in "$user_root"/*/; do
    base="${base%/}"
    marker="$base/DO_NOT_BUILD_HERE"
    [ -f "$marker" ] || continue
    ws=$(cat "$marker")
    [ -e "$ws" ] && continue
    orphans=$((orphans + 1))
    size=$(du -sk "$base" 2>/dev/null | cut -f1)
    pid=$(live_server_pid "$base")
    echo "orphan: $base ($(human "${size:-0}")) workspace gone: $ws${pid:+ [live server pid $pid]}"
    if [ "$apply" = 1 ]; then
      if [ -n "$pid" ] && ! kill_server "$pid"; then
        echo "  skipped: server pid $pid did not exit" >&2
        continue
      fi
      remove_tree "$base" && echo "  removed"
    fi
  done
fi
[ "$orphans" = 0 ] && echo "none"

echo "== install bases older than 30 days (report only)"
found=0
if [ -d "$user_root/install" ]; then
  while IFS= read -r d; do
    [ -n "$d" ] || continue
    found=1
    echo "old install base: $d ($(human "$(du -sk "$d" | cut -f1)"))"
  done < <(find "$user_root/install" -mindepth 1 -maxdepth 1 -type d -mtime +30 2>/dev/null)
fi
[ "$found" = 0 ] && echo "none"

echo "== shared disk cache (never removed)"
if [ -d "$disk_cache" ]; then
  kb=$(du -sk "$disk_cache" 2>/dev/null | cut -f1)
  echo "disk cache: $disk_cache $(human "${kb:-0}") of ${cap_gb} GB GC cap"
else
  echo "disk cache: $disk_cache absent"
fi

in_use() {
  local d="$1" f
  command -v "$fuser_bin" >/dev/null 2>&1 || return 0   # cannot prove it is idle
  while IFS= read -r -d '' f; do
    "$fuser_bin" -s "$f" 2>/dev/null && return 0
  done < <(find "$d" -print0 2>/dev/null)
  return 1
}

echo "== stale test temp dirs under $tmp_root (older than 24 h, no open handles; --apply --tmp removes)"
stale=0
for pat in $tmp_patterns; do
  while IFS= read -r d; do
    [ -n "$d" ] || continue
    in_use "$d" && continue
    stale=$((stale + 1))
    echo "stale temp: $d ($(human "$(du -sk "$d" 2>/dev/null | cut -f1)"))"
    if [ "$apply" = 1 ] && [ "$tmp" = 1 ]; then remove_tree "$d" && echo "  removed"; fi
  done < <(find "$tmp_root" -mindepth 1 -maxdepth 1 -name "$pat" -mmin +1440 2>/dev/null)
done
[ "$stale" = 0 ] && echo "none"
if [ "$apply" = 0 ]; then echo "dry run: nothing removed; re-run with --apply (and --tmp for temp dirs)"; fi
exit 0
