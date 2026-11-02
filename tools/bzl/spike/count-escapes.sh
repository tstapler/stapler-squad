#!/usr/bin/env bash
# Task 0.6b: prints "<reads-outside-web-app> <writes-into-source-tree>" for the strace log of
# //web-app:dist_trace. PASS iff it prints "0 0". Usage: count-escapes.sh [log] [--list]
exec python3 -I "$(dirname "$0")/count_escapes.py" "${1:-bazel-bin/web-app/dist_trace.strace}" "${@:2}"
