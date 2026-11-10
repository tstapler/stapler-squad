#!/usr/bin/env bash
# Wrapper parity (Task 8.2c, 8.2d): for every wrapper target, `make -n` under BUILD_SYSTEM=make and
# BUILD_SYSTEM=bazel (ACCEPT_SESSION_LOSS=1 skips install-service's deployed-unit preflight, absent on CI) must each produce a non-empty plan, and the build/test wrappers must call the cheap
# gen-freshness check within their first three plan lines. Needs make and bash only, never Bazel.
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 2
fail=0
targets=$(bash tools/bzl/wrapper-targets.sh)
[ -n "$targets" ] || { echo "check-wrappers: no wrapper targets found in make/bazel.mk"; exit 1; }
count=0
for t in $targets; do
  count=$((count + 1))
  for sys in make bazel; do
    plan=$(BUILD_SYSTEM=$sys make --no-print-directory -n "$t" PKG=./config ACCEPT_SESSION_LOSS=1 2>&1) || { echo "check-wrappers: '$t' under BUILD_SYSTEM=$sys: make -n failed"; fail=1; continue; }
    [ -n "$plan" ] || { echo "check-wrappers: '$t' under BUILD_SYSTEM=$sys: empty plan"; fail=1; }
  done
done
for t in build test test-race test-integration; do
  for sys in make bazel; do
    n=$(BUILD_SYSTEM=$sys make --no-print-directory -n "$t" PKG=./config 2>/dev/null | head -3 | grep -c 'check-gen-fresh-cheap')
    [ "$n" = 1 ] || { echo "check-wrappers: '$t' under BUILD_SYSTEM=$sys: no check-gen-fresh-cheap in the first 3 plan lines (got $n)"; fail=1; }
  done
done
n2=$(env BUILD_SYSTEM=bazel make --no-print-directory -n stapler-squad 2>/dev/null | head -3 | grep -c 'check-gen-fresh-cheap')
[ "$n2" = 1 ] || { echo "check-wrappers: 'stapler-squad' under BUILD_SYSTEM=bazel: no check-gen-fresh-cheap first"; fail=1; }
# Under make it is an order-only prerequisite (a phony prerequisite would rebuild the binary every run), so
# it appears after the codegen prerequisites rather than first.
n=$(env BUILD_SYSTEM=make make --no-print-directory -n stapler-squad 2>/dev/null | grep -c 'check-gen-fresh-cheap')
[ "$n" -ge 1 ] || { echo "check-wrappers: 'stapler-squad' under BUILD_SYSTEM=make never runs the gen-freshness note"; fail=1; }
[ "$fail" = 0 ] && echo "wrappers: $count targets ok under both BUILD_SYSTEM values"
exit "$fail"
