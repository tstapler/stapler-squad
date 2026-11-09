#!/usr/bin/env bash
# Disk alarm for Bazel state (Task 11.4c). Exit 1 and print "ALARM: <path> <du|df> <value>" per breach:
#   du of ~/.cache/bazel above 150 GB; shared disk cache above its 100 GB cap (its built-in GC only runs while
#   a server is idle, so a breach means GC is not running: use the diskcache:gc fallback); df of the cache path or
#   of the bazel-remote data directory above 85%.
#   bazel-disk-alarm.sh [--gauges]   --gauges also prints bazel_cache_kb, bazel_disk_cache_kb, bazel_cache_df_pct
# Env (tests stub the tools): DU, DF, BAZEL_CACHE_DIR, DISK_CACHE, BAZEL_REMOTE_DATA_DIR (optional; unset skips it).
set -u
du_bin="${DU:-du}"; df_bin="${DF:-df}"
cache_dir="${BAZEL_CACHE_DIR:-$HOME/.cache/bazel}"
disk_cache="${DISK_CACHE:-$HOME/.cache/stapler-squad-bazel/disk}"
remote_dir="${BAZEL_REMOTE_DATA_DIR:-}"
gauges=0; [ "${1:-}" = "--gauges" ] && gauges=1

kb_limit_cache=$((150 * 1024 * 1024)); kb_limit_disk=$((100 * 1024 * 1024)); df_limit=85
rc=0

du_kb() { [ -e "$1" ] && "$du_bin" -sk "$1" 2>/dev/null | cut -f1; }
df_pct() { "$df_bin" -P "$1" 2>/dev/null | awk 'NR==2 { gsub("%", "", $5); print $5 }'; }
gb() { awk -v k="$1" 'BEGIN { printf "%.0fG", k / 1048576 }'; }

alarm() { echo "ALARM: $1 $2 $3"; rc=1; }

cache_kb=$(du_kb "$cache_dir"); cache_kb=${cache_kb:-0}
disk_kb=$(du_kb "$disk_cache"); disk_kb=${disk_kb:-0}
[ "$cache_kb" -gt "$kb_limit_cache" ] && alarm "$cache_dir" du "$(gb "$cache_kb")"
[ "$disk_kb" -gt "$kb_limit_disk" ] && alarm "$disk_cache" du "$(gb "$disk_kb")"

pct=""
for p in "$cache_dir" "$remote_dir"; do
  # shellcheck disable=SC2015  # `continue` is the else branch; the && chain has no side effects
  [ -n "$p" ] && [ -e "$p" ] || continue
  v=$(df_pct "$p"); v=${v:-0}
  [ -z "$pct" ] && pct=$v
  [ "$v" -gt "$df_limit" ] && alarm "$p" df "${v}%"
done

if [ "$gauges" = 1 ]; then
  echo "bazel_cache_kb $cache_kb"
  echo "bazel_disk_cache_kb $disk_kb"
  echo "bazel_cache_df_pct ${pct:-0}"
fi
exit "$rc"
