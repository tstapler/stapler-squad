#!/usr/bin/env bash
# Cheap BUILD-staleness heuristic (no Bazel, target < 300 ms): flags packages where a Go file was
# added, removed, renamed or had an import line changed while the package's BUILD.bazel is untouched.
#   check-build-fresh-cheap.sh [--warn]   --warn always exits 0 (the wrapper's mode); otherwise 1 on a hit.
# False negatives are possible; the authoritative gate is `gazelle -mode=diff` in CI.
set -u
warn=""
[ "${1:-}" = "--warn" ] && warn=1
root=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "BUILD: fresh"; exit 0; }
cd "$root" || exit 0

skip_list=tools/bzl/no-build-packages.txt
stale=""
seen=" "
while IFS= read -r line; do
  [ -n "$line" ] || continue
  code=${line:0:2}
  path=${line:3}
  case "$path" in *" -> "*) path=${path##* -> } ;; esac
  case "$path" in gen/*|session/ent/*|vendor/*|node_modules/*|.claude/worktrees/*|*/node_modules/*) continue ;; esac
  dir=$(dirname "$path")
  case "$seen" in *" $dir "*) continue ;; esac
  structural=""
  case "$code" in
    '??'|A*|*A|D*|*D|R*|*R) structural=1 ;;
    *) if git diff -U0 HEAD -- "$path" 2>/dev/null | grep -qE '^[+-][[:space:]]*(import )?"[^"]+"$'; then structural=1; fi ;;
  esac
  [ -n "$structural" ] || continue
  # A rename of a file into another directory also touches the old directory; the new path is checked here.
  if [ -f "$skip_list" ] && grep -qE "^$dir([[:space:]]|\$)" "$skip_list"; then continue; fi
  if [ -n "$(git status --porcelain -- "$dir/BUILD.bazel" "$dir/BUILD" 2>/dev/null)" ]; then
    seen="$seen$dir "
    continue
  fi
  seen="$seen$dir "
  stale="$stale$dir
"
done < <(git status --porcelain -uall -- '*.go' 2>/dev/null)

if [ -z "$stale" ]; then
  echo "BUILD: fresh"
  exit 0
fi
printf '%s' "$stale" | while IFS= read -r p; do
  echo "note: BUILD.bazel may be stale for $p - run 'make gazelle'"
done
[ -n "$warn" ] && exit 0
exit 1
