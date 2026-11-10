#!/usr/bin/env bash
# Task 6.1b: on this host //... must analyse with zero errors, and at least one target must be
# reported incompatible by the platform provider (the windows-only canary, ADR-005).
# Needs bazel: tools/bazel when BAZEL_REAL is set (bazelisk sets it), else plain `bazel` as CI invokes it
# (tools/bazel exits 2 without BAZEL_REAL, which this script's grep-based checks would read as "0 incompatible"). Exits 1 on failure.
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2
if [ -n "${BAZEL_REAL:-}" ]; then bz=tools/bazel; else bz=bazel; fi

errs=$($bz build --nobuild --keep_going //... 2>&1 | grep -c '^ERROR')
if [ "$errs" -ne 0 ]; then
  echo "check-platform-targets: $errs ERROR lines from 'bazel build --nobuild --keep_going //...'" >&2
  exit 1
fi

incompat=$($bz cquery --keep_going --output=starlark \
  --starlark:expr='"incompatible" if "IncompatiblePlatformProvider" in providers(target) else "ok"' \
  '//...' 2>/dev/null | grep -c incompatible)
if [ "$incompat" -lt 1 ]; then
  echo "check-platform-targets: no target is reported incompatible; windows-only target_compatible_with is not taking effect" >&2
  exit 1
fi
echo "check-platform-targets: ok ($incompat incompatible targets)"
