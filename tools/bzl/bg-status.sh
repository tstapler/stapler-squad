#!/usr/bin/env bash
# Poll a detached or BAZEL_BG=1 run: prints "running" or "finished exit=<n>" from .bazel-logs/last-exit.
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
f="${BAZEL_LOG_DIR:-$root/.bazel-logs}/last-exit"
if [ ! -f "$f" ]; then
  echo running
  exit 0
fi
code=$(sed -n 's/^exit=\([0-9]*\).*/\1/p' "$f" | head -n 1)
echo "finished exit=${code:-?}"
