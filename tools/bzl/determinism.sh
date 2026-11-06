#!/usr/bin/env bash
# Task 7.7a: build the targets (default //:stapler-squad, release config) in two clean output bases with no
# disk or remote cache, then compare the execution logs with tools/bzl/determinism.
# usage: determinism.sh [target...]       env: BAZEL (default bazel), DETERMINISM_TMP (scratch dir)
# Exit 0 deterministic, 1 digest mismatch, 2 setup or build failure.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

bazel="${BAZEL:-bazel}"
targets=("$@")
((${#targets[@]})) || targets=(//:stapler-squad)
tmp="$(mktemp -d "${DETERMINISM_TMP:-${TMPDIR:-$HOME/.cache/tmp}}/determinism.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

for n in 1 2; do
  "$bazel" --output_base="$tmp/ob$n" build --config=release --disk_cache= --remote_cache= \
    --execution_log_json_file="$tmp/exec$n.json" "${targets[@]}" || { echo "determinism: build $n failed" >&2; exit 2; }
done

"$bazel" run //tools/bzl/determinism -- -a "$tmp/exec1.json" -b "$tmp/exec2.json" -downgrades "$PWD/tools/bzl/hermetic-downgrades.txt"
