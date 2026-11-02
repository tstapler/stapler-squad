#!/usr/bin/env bash
# .bazelignore entries are exact root-relative paths, not patterns. Fail if a node_modules
# directory exists that is neither listed nor under a listed path (Bazel would walk it).
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2
[ -f .bazelignore ] || { echo "check-bazelignore: missing .bazelignore" >&2; exit 1; }

entries=()
while IFS= read -r l; do
  l=${l%%#*}; l=$(echo "$l" | tr -d '[:space:]')
  [ -n "$l" ] && entries+=("$l")
done < .bazelignore

covered() {
  local p="$1" e
  for e in "${entries[@]}"; do
    case "$p" in "$e"|"$e"/*) return 0 ;; esac
  done
  return 1
}

status=0
while IFS= read -r d; do
  d=${d#./}
  covered "$d" || { echo "check-bazelignore: $d is not covered by .bazelignore" >&2; status=1; }
done < <(find . -name node_modules -type d -prune -not -path './.claude/worktrees/*' 2>/dev/null)
exit $status
