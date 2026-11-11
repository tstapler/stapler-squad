#!/usr/bin/env bash
# Cap the repo-owned Starlark surface (ADR-008, Task 11.7a): all tracked .bzl files, macros included, may total
# at most STARLARK_LIMIT lines (default 750). Generated files (a "DO NOT EDIT" line in the first 5 lines) and
# vendored trees are not repo-owned and are not counted.
# Env: STARLARK_LIMIT, STARLARK_ROOT (a git work tree; default: this repo).
set -eu
root="${STARLARK_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
limit="${STARLARK_LIMIT:-750}"
cd "$root"
total=0
report=""
while IFS= read -r f; do
  [ -f "$f" ] || continue
  head -n 5 "$f" | grep -q 'DO NOT EDIT' && continue
  n=$(wc -l < "$f")
  total=$((total + n))
  report="$report$n $f"$'\n'
done < <(git ls-files -- '*.bzl' ':!:third_party/**')
if [ "$total" -le "$limit" ]; then
  echo "starlark surface: $total lines (limit $limit)"
  exit 0
fi
echo "starlark surface: $total lines exceeds the limit of $limit (ADR-008). Largest files:" >&2
printf '%s' "$report" | sort -rn | head -5 | sed 's/^/  /' >&2
exit 1
