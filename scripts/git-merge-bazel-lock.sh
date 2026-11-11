#!/usr/bin/env bash
# Git merge driver for MODULE.bazel.lock: usage git-merge-bazel-lock.sh %O %A %B (base, ours, theirs).
# Same jq program and invocation shape as Bazel's documented driver
# (https://bazel.build/external/lockfile#automatic-resolution), vendored from
# bazelbuild/bazel@9.3.1 scripts/bazel-lockfile-merge.jq so the merge is pinned and offline. Needs jq 1.5+.
#   git-merge-bazel-lock.sh --check   exit 0 if this clone registered the driver (make setup-git) and jq is 1.5+
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ "${1:-}" = "--check" ]; then
  want="scripts/git-merge-bazel-lock.sh %O %A %B"
  got=$(git config --get merge.bazel-lockfile-merge.driver 2>/dev/null || true)
  if [ "$got" != "$want" ]; then
    echo "merge driver not registered for this clone (git config is per clone; re-run after every fresh git clone). Remedy: run 'make setup-git'." >&2
    exit 1
  fi
  ver=$(jq --version 2>/dev/null | sed 's/^jq-//' || true)
  major=${ver%%.*}; rest=${ver#*.}; minor=${rest%%[!0-9]*}
  case "$major" in ''|*[!0-9]*) echo "jq not found; the merge driver needs jq 1.5 or newer" >&2; exit 1 ;; esac
  if [ "$major" -lt 1 ] || { [ "$major" -eq 1 ] && [ "${minor:-0}" -lt 5 ]; }; then
    echo "jq $ver is too old; the merge driver needs jq 1.5 or newer" >&2; exit 1
  fi
  echo "merge driver registered, jq $ver"
  exit 0
fi

jq_script="${BAZEL_LOCK_MERGE_JQ:-$here/../tools/bzl/bazel-lockfile-merge.jq}"
base="$1"; ours="$2"; theirs="$3"
jq -s -f "$jq_script" -- "$base" "$ours" "$theirs" > "$ours.jq_tmp"
mv "$ours.jq_tmp" "$ours"
