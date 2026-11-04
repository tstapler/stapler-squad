#!/usr/bin/env bash
# `bazel test` for the BUILD_SYSTEM=bazel wrappers (Story 8.7): quiet run, then a bounded summary.
#   run-bazel-test.sh <bazel test args...>
# Success: one result line (plus the cached-target note). Failure: failing test names with their
# bazel-testlogs/<pkg>/<target>/test.log paths via //tools/bzl/testsummary, at most 60 lines.
# On a terminal the Bazel stream is shown live. Exit code is bazel's.
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 2
BAZEL_REAL=${BAZEL_REAL:-$(command -v bazelisk || command -v bazel || true)}
export BAZEL_REAL
if [ -z "$BAZEL_REAL" ]; then echo "run-bazel-test: bazelisk or bazel not on PATH (BUILD_SYSTEM=bazel needs one)" >&2; exit 2; fi
log=$(mktemp "${TMPDIR:-/tmp}/bazel-test.XXXXXX") || exit 2
trap 'rm -f "$log"' EXIT

if [ -t 1 ]; then
  tools/bazel test "$@" 2>&1 | tee "$log"
  rc=${PIPESTATUS[0]}
else
  tools/bazel test "$@" >"$log" 2>&1
  rc=$?
fi
cached=$(grep -c '(cached)' "$log")

if [ "$rc" = 0 ]; then
  grep -E '^Executed [0-9]+ out of|^INFO: Build completed successfully' "$log" | tail -2
  [ "$cached" -gt 0 ] && echo "($cached test targets cached; rerun with FORCE=1)"
  exit 0
fi

testlogs=$(readlink -f bazel-testlogs 2>/dev/null || true)
if [ -n "$testlogs" ] && [ -d "$testlogs" ]; then
  tools/bazel run --ui_event_filters=-info --noshow_progress //tools/bzl/testsummary -- \
    -testlogs "$testlogs" -summary -cached "$cached" -max-lines 60 2>/dev/null || true
fi
# A build failure leaves no test.xml: show the tail of Bazel's own output instead.
if ! grep -qE '^(//|  )[^ ]*.*FAILED|FAILED in' "$log"; then
  grep -E '^(ERROR|FAILED|Error)' "$log" | head -20
fi
echo "bazel test exited $rc"
exit "$rc"
