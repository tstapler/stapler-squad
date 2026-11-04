#!/usr/bin/env bash
# Prints the make targets in make/bazel.mk whose recipe prints the [build-system=bazel] banner or uses the
# `wrap` or `bzl_*` helpers, one per line. Shared by check-make-help.sh and check-wrappers.sh so Story 8.2's
# wrapper targets are covered as they land.
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
awk '
  /^[a-zA-Z0-9_.-]+( [a-zA-Z0-9_.-]+)*:([^=]|$)/ {
    split($0, h, ":"); n = split(h[1], cur, " ")
    next
  }
  /^\t.*(\[build-system=bazel\]|\$\(call (wrap|bzl_)|\$\(BAZEL\))/ {
    for (i = 1; i <= n; i++) if (cur[i] !~ /^\./) print cur[i]
  }
' "$root/make/bazel.mk" | sort -u
