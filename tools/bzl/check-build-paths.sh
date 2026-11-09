#!/usr/bin/env bash
# Every Bazel-owned BUILD path must be loadable on a clean clone: tracked-eligible (not
# git-ignored), not under a gitlink (submodule), and not hidden by a .bazelignore entry.
#   check-build-paths.sh          hand-written paths from tools/bzl/hand-build-files.txt
#   check-build-paths.sh --full   also every generated/loaded BUILD file (needs bazel; Task 1.1a+)
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2

list=tools/bzl/hand-build-files.txt
fail() { echo "check-build-paths: $1" >&2; exit 1; }

is_under_gitlink() {
  local p="$1" dir
  dir=$(dirname "$p")
  while [ "$dir" != "." ] && [ "$dir" != "/" ]; do
    if git ls-files -s -- "$dir" | awk -v d="$dir" '$1 == "160000" && $4 == d { found = 1 } END { exit !found }'; then
      echo "$dir"; return 0
    fi
    dir=$(dirname "$dir")
  done
  return 1
}

is_bazelignored() {
  local p="$1" entry
  [ -f .bazelignore ] || return 1
  while IFS= read -r entry; do
    entry=${entry%%#*}
    entry=$(echo "$entry" | tr -d '[:space:]')
    [ -z "$entry" ] && continue
    case "$p" in "$entry"|"$entry"/*) echo "$entry"; return 0 ;; esac
  done < .bazelignore
  return 1
}

[ -f "$list" ] || fail "missing $list"
while IFS= read -r line; do
  path=${line%%#*}
  path=$(echo "$path" | tr -d '[:space:]')
  [ -z "$path" ] && continue
  if git check-ignore -q -- "$path" 2>/dev/null; then
    fail "$path is matched by git check-ignore (it would not be committed)"
  fi
  if g=$(is_under_gitlink "$path"); then
    fail "$path is under gitlink $g (submodule contents are not part of this repo)"
  fi
  if i=$(is_bazelignored "$path"); then
    fail "$path is covered by .bazelignore entry '$i' (Bazel would never load it)"
  fi
done < "$list"

if [ "${1:-}" = "--full" ]; then
  bad=$(git ls-files --others --ignored --exclude-standard -- '*BUILD' '*BUILD.bazel' | head -1)
  [ -z "$bad" ] || fail "$bad exists on disk but is git-ignored"
  command -v bazel >/dev/null 2>&1 || fail "--full needs bazel on PATH (cannot evaluate: treated as failure)"
  while IFS= read -r pkg; do
    [ -z "$pkg" ] && continue
    for f in "$pkg/BUILD.bazel" "$pkg/BUILD"; do
      [ -f "$f" ] || continue
      git ls-files --error-unmatch -- "$f" >/dev/null 2>&1 || fail "$f is loaded by Bazel but not git-tracked"
    done
  done < <(bazel query --output=package '//...' 2>/dev/null | sed 's#^//##; s#^@@\?##')
fi
exit 0
