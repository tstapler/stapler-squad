#!/usr/bin/env bash
# Trim a restored GOCACHE before CI re-saves it: drop entries not touched for MAX_AGE_DAYS, then
# drop oldest-first until the directory is under MAX_MB. Go refreshes an entry's mtime on use, so
# mtime tracks "still needed". Linux (GNU find) only; a no-op elsewhere.
# Usage: trim-go-cache.sh <gocache-dir> [max-mb] [max-age-days]
set -euo pipefail

dir="${1:?usage: trim-go-cache.sh <gocache-dir> [max-mb] [max-age-days]}"
max_mb="${2:-1500}"
max_age_days="${3:-7}"

[ -d "$dir" ] || { echo "trim-go-cache: $dir missing, nothing to trim"; exit 0; }
find . -maxdepth 0 -printf "" >/dev/null 2>&1 || { echo "trim-go-cache: GNU find required, skipping"; exit 0; }

mb() { du -sm "$dir" | cut -f1; }
before="$(mb)"

find "$dir" -type f -mtime "+$max_age_days" -not -name README -not -name trim.txt -delete

if [ "$(mb)" -gt "$max_mb" ]; then
  limit_bytes=$((max_mb * 1024 * 1024))
  total="$(find "$dir" -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')"
  # Oldest first; stop once the remaining total fits.
  find "$dir" -type f -not -name README -not -name trim.txt -printf '%T@ %s %p\n' | sort -n |
    while read -r _ size path; do
      [ "$total" -le "$limit_bytes" ] && break
      rm -f -- "$path"
      total=$((total - size))
    done || true
fi

echo "trim-go-cache: $dir ${before}MB -> $(mb)MB (age>${max_age_days}d dropped, cap ${max_mb}MB)"
