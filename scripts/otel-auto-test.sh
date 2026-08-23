#!/usr/bin/env bash
# Test-time wrapper for instrumentation/otelc/safeexec's hook_test.go (Story
# 5.1.3 repair-loop finding #7): the only repeatable way to run those tests,
# since hook.go is gated behind the `otelcauto` build tag AND imports
# go.opentelemetry.io/otelc/pkg/hook, which only exists in go.mod during an
# `otelc setup` window (see hook.go's package doc).
#
# Mirrors scripts/otel-auto-build.sh's module-backup / GOFLAGS / cleanup
# lifecycle (byte-identical go.mod/go.sum restore, `otelc cleanup`, stray
# otelc.runtime.go removal) rather than shelling out to that script directly.
# It can't just call `otel-auto-build.sh go test ./instrumentation/otelc/safeexec/...
# ./telemetry/...`, because that script always passes its actual go-test
# target packages straight to `otelc setup` too — and `otelc setup` fails
# outright when instrumentation/otelc/safeexec is one of the packages handed
# to it directly on a clean go.mod:
#
#   Error: [0] failed to run build plan:
#   instrumentation/otelc/safeexec/hook.go:52:2: no required module provides
#   package go.opentelemetry.io/otelc/pkg/hook; to add it:
#           go get go.opentelemetry.io/otelc/pkg/hook
#
# hook.go unconditionally imports pkg/hook, and otelc's package loader can't
# resolve that import before its own AutoPin step has had a chance to add the
# require — a chicken-and-egg failure specific to naming the *rule
# implementation* package itself as a setup target (as opposed to a
# *consumer* of the rule, e.g. session/git, which is how scripts/otel-auto-build.sh
# is normally used). Confirmed empirically (2026-08-23): running
# `otelc setup .` ALONE first — exactly what build-otel-auto's own first pass
# already does — adds the go.opentelemetry.io/otelc/pkg require+replace as a
# side effect of its built-in rule dependency discovery (one of otelc's own
# built-in rules already depends on pkg/hook). Once that require exists, a
# second `otelc setup` call naming instrumentation/otelc/safeexec directly
# succeeds, and a plain `go test` (no toolexec — these tests exercise
# BeforeCommandContext/AfterCommandContext directly via a fake
# hook.HookContext, not a real weave) runs clean.
#
# telemetry is a `go test` target but deliberately NOT an `otelc setup`
# target: `otelc setup` writes a per-package otelc.runtime.go (see
# scripts/otel-auto-build.sh's cleanup comment) into every package it's
# given, each defining the same linkname'd runtime symbol
# (OtelGetStackImpl) — confirmed empirically (2026-08-23) that setting up
# BOTH ./instrumentation/otelc/safeexec/... and ./telemetry/... produces
# "link: duplicated definition of symbol ...OtelGetStackImpl" once both
# packages land in the same `go test` binary graph. telemetry needs none of
# this scaffolding (its tests don't touch otelc or the hook package at
# all), so it's only ever a plain `go test` target, never an `otelc setup`
# one.
#
# Usage:
#   ./scripts/otel-auto-test.sh
set -euo pipefail

SETUP_PKGS=(./instrumentation/otelc/safeexec/...)
TEST_PKGS=(./instrumentation/otelc/safeexec/... ./telemetry/...)
OTELC_AUTO_TAG="otelcauto"

if ! command -v otelc >/dev/null 2>&1; then
  echo "✗ otelc not found on PATH." >&2
  echo "  Install it from open-telemetry/opentelemetry-go-compile-instrumentation:" >&2
  echo "  https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation" >&2
  echo "  (see project_plans/go-auto-instrumentation/implementation/spike-verdicts.md" >&2
  echo "  for the exact install command used in this repo)." >&2
  exit 1
fi

mod_backup_dir="$(mktemp -d)"
trap cleanup EXIT INT TERM

# shellcheck disable=SC2329  # invoked indirectly via `trap ... EXIT` above
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  otelc cleanup >/dev/null 2>&1 || true
  cp "$mod_backup_dir/go.mod" "$mod_backup_dir/go.sum" .
  rm -rf "$mod_backup_dir"
  rm -f otel.instrumentation.go otelc.runtime.go
  # See scripts/otel-auto-build.sh's identical cleanup step for why this is
  # a `find`, not a single `rm -f`: generateRuntimePerPackage (otelc's
  # setup.go) writes an otelc.runtime.go into EVERY package passed as a
  # build target, not just the repo root.
  find . -name otelc.runtime.go -not -path './.otelc-build/*' -delete 2>/dev/null || true
  exit "$status"
}

cp go.mod go.sum "$mod_backup_dir/"

export GOFLAGS="${GOFLAGS:-} -tags=${OTELC_AUTO_TAG}"

echo "▶ otelc setup — bootstrap pass (target: ., adds go.opentelemetry.io/otelc/pkg require via built-in rule discovery)"
otelc setup .

echo "▶ otelc setup — rule package (${SETUP_PKGS[*]})"
otelc setup "${SETUP_PKGS[@]}"

# Module Mutation Guard (matches scripts/otel-auto-build.sh): brackets only
# the actual test run, between setup and cleanup — both `otelc setup` calls
# above are EXPECTED to touch go.mod/go.sum, but `go test` itself must not.
mod_sum_before="$(sha256sum go.mod go.sum 2>/dev/null || true)"

set +e
go test -tags="${OTELC_AUTO_TAG}" "${TEST_PKGS[@]}"
test_status=$?
set -e

mod_sum_after="$(sha256sum go.mod go.sum 2>/dev/null || true)"

if [ "$mod_sum_before" != "$mod_sum_after" ]; then
  echo "✗ Module Mutation Guard: go.mod/go.sum changed during the test step." >&2
  diff <(echo "$mod_sum_before") <(echo "$mod_sum_after") >&2 || true
  git diff -- go.mod go.sum >&2 || true
  exit 1
fi

exit "$test_status"
