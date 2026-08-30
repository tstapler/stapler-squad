#!/usr/bin/env bash
# Test-time wrapper for instrumentation/otelc/safeexec's hook_test.go: the
# only repeatable way to run those tests, since hook.go is gated behind the
# `otelcauto` build tag and imports go.opentelemetry.io/otelc/pkg/hook, which
# only exists in go.mod during an `otelc setup` window (see hook.go's
# package doc).
#
# Mirrors scripts/otel-auto-build.sh's module-backup/GOFLAGS/cleanup
# lifecycle rather than shelling out to it directly: that script always
# passes its real go-test target packages straight to `otelc setup`, which
# fails outright when instrumentation/otelc/safeexec is named directly on a
# clean go.mod (a chicken-and-egg import-resolution failure — hook.go's
# unconditional `pkg/hook` import can't resolve before AutoPin adds the
# require). A bootstrap `otelc setup .` first — the same first pass
# build-otel-auto always runs — adds that require as a side effect of
# built-in rule discovery, after which a second `otelc setup` naming
# instrumentation/otelc/safeexec directly succeeds.
#
# telemetry is a `go test` target but deliberately NOT an `otelc setup`
# target: setting up two packages together writes duplicate linkname'd
# runtime symbols into each, which fails to link once both share a `go test`
# binary graph. telemetry's tests don't touch otelc or the hook package, so
# it never needs that scaffolding.
#
# Full investigation trail and exact error output:
# project_plans/go-auto-instrumentation/implementation/spike-verdicts.md
#
# Usage:
#   ./scripts/otel-auto-test.sh
set -euo pipefail

SETUP_PKGS=(./instrumentation/otelc/safeexec/...)
TEST_PKGS=(./instrumentation/otelc/safeexec/... ./telemetry/...)
OTELC_AUTO_TAG="otelcauto"

# hash_cmd: see scripts/otel-auto-build.sh's identical helper — sha256sum
# isn't on PATH by default on macOS (shasum -a 256 is), and the Module
# Mutation Guard below must hard-fail rather than silently false-pass if
# neither is available.
hash_cmd() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    echo "✗ no sha256sum/shasum on PATH — cannot run the Module Mutation Guard" >&2
    exit 1
  fi
}

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

# shellcheck disable=SC2329,SC2317  # invoked indirectly via `trap ... EXIT` above
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
mod_sum_before="$(hash_cmd go.mod go.sum)"

set +e
go test -tags="${OTELC_AUTO_TAG}" "${TEST_PKGS[@]}"
test_status=$?
set -e

mod_sum_after="$(hash_cmd go.mod go.sum)"

if [ "$mod_sum_before" != "$mod_sum_after" ]; then
  echo "✗ Module Mutation Guard: go.mod/go.sum changed during the test step." >&2
  diff <(echo "$mod_sum_before") <(echo "$mod_sum_after") >&2 || true
  git diff -- go.mod go.sum >&2 || true
  exit 1
fi

exit "$test_status"
