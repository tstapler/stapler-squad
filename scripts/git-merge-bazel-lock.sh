#!/usr/bin/env bash
# Git merge driver for MODULE.bazel.lock: usage git-merge-bazel-lock.sh %O %A %B (base, ours, theirs).
# Same jq program and invocation shape as Bazel's documented driver
# (https://bazel.build/external/lockfile#automatic-resolution), vendored from
# bazelbuild/bazel@9.3.1 scripts/bazel-lockfile-merge.jq so the merge is pinned and offline. Needs jq 1.5+.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
jq_script="${BAZEL_LOCK_MERGE_JQ:-$here/../tools/bzl/bazel-lockfile-merge.jq}"
base="$1"; ours="$2"; theirs="$3"
jq -s -f "$jq_script" -- "$base" "$ours" "$theirs" > "$ours.jq_tmp"
mv "$ours.jq_tmp" "$ours"
