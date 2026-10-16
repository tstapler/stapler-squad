#!/usr/bin/env bash
# Isolation Guard (Story 2.1.3, ADR-003): proves the opt-in otelc build path
# stays unreachable from every default/CI/deploy target, by dry-running
# `make -n` for each target and grepping the recipe for `otelc`/
# `stapler-squad-otel`. Never invokes build-otel-auto/otelc itself, so it
# needs no otelc install. Match patterns require a trailing word boundary:
# this repo's own worktree checkouts are named `stapler-squad-otel_<hash>`,
# so an unanchored match false-positives on the checkout's own directory name
# in every printed absolute path.
#
# --self-test proves this detection logic actually fires: it injects a
# deliberate leak into a temp copy of the Makefile and asserts the check
# fails on it (naming the offending target), then asserts the check still
# passes cleanly on the real, unmodified Makefile.
#
# Usage:
#   ./scripts/otel-auto-isolation-guard.sh
#   ./scripts/otel-auto-isolation-guard.sh --self-test
set -euo pipefail

TARGETS=(ci ready quick-check pre-commit install-service)
FORBIDDEN='otelc[^A-Za-z0-9_]|otelc$|stapler-squad-otel[^A-Za-z0-9_]|stapler-squad-otel$'

SELF_TEST="false"
for arg in "$@"; do
  case "$arg" in
    --self-test) SELF_TEST="true" ;;
    *)
      echo "✗ unknown flag: $arg" >&2
      exit 1
      ;;
  esac
done

# check_makefile <makefile-path> — dry-runs each target in TARGETS against
# the given Makefile and greps its printed recipe for $FORBIDDEN. Prints a
# PASS/FAIL line per target and returns non-zero if any target is reachable.
check_makefile() {
  local makefile="$1"
  local failures=()
  local target output
  for target in "${TARGETS[@]}"; do
    if ! output="$(make -f "$makefile" -n "$target" 2>&1)"; then
      echo "✗ FAIL: could not dry-run target '$target' (missing/broken Makefile target?)" >&2
      echo "$output" >&2
      failures+=("$target")
      continue
    fi
    if echo "$output" | grep -E -q "$FORBIDDEN"; then
      echo "✗ FAIL: '$target' is reachable to the otelc auto-instrumentation path" >&2
      echo "$output" | grep -E "$FORBIDDEN" >&2
      failures+=("$target")
    else
      echo "✅ PASS: $target"
    fi
  done
  if [ "${#failures[@]}" -gt 0 ]; then
    echo "✗ Isolation Guard failed for: ${failures[*]}" >&2
    return 1
  fi
  return 0
}

if [ "$SELF_TEST" = "true" ]; then
  tmp_makefile="$(mktemp)"
  trap 'rm -f "$tmp_makefile"' EXIT

  cp Makefile "$tmp_makefile"
  # Append a second `ci:` rule with a deliberate leak in its recipe. GNU Make
  # merges prerequisite lists across repeated non-double-colon rules and uses
  # the LAST recipe defined for the target — since the real `ci:` rule has no
  # recipe of its own (only prerequisites), this doesn't trigger an
  # "overriding recipe" warning, and `make -n ci` against this copy will
  # include the injected echo line.
  printf '\nci:\n\t@echo "stapler-squad-otel leaked into ci"\n' >>"$tmp_makefile"

  echo "▶ self-test half 1: detection must FAIL on the tampered Makefile copy"
  half1_ok="true"
  if check_makefile "$tmp_makefile"; then
    echo "✗ FAIL (self-test): guard passed against a Makefile with a deliberate leak injected into 'ci'" >&2
    half1_ok="false"
  else
    echo "✅ PASS (self-test): guard correctly failed on the tampered copy"
  fi

  echo "▶ self-test half 2: detection must still PASS cleanly on the real Makefile"
  half2_ok="true"
  if check_makefile "Makefile"; then
    echo "✅ PASS (self-test): guard correctly passed on the real Makefile"
  else
    echo "✗ FAIL (self-test): guard failed against the real, unmodified Makefile" >&2
    half2_ok="false"
  fi

  if [ "$half1_ok" = "true" ] && [ "$half2_ok" = "true" ]; then
    echo "✅ otel-auto-isolation-guard --self-test: both halves passed"
    exit 0
  fi
  echo "✗ otel-auto-isolation-guard --self-test: FAILED" >&2
  exit 1
fi

check_makefile "Makefile"
