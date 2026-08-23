#!/usr/bin/env bash
# Isolation Guard (Story 2.1.3, ADR-003): proves the opt-in otelc build path
# stays unreachable from every default/CI/deploy target. Dry-runs `make -n`
# for each target below and greps the printed recipe for `otelc` /
# `stapler-squad-otel` — it never invokes build-otel-auto, otelc, or produces
# any binary, so it runs fine on a machine with no otelc installed.
#
# Match ONLY on "otelc" and "stapler-squad-otel" — never on "otel-auto",
# which legitimately appears in this script's own filename and in
# scripts/otel-auto-build.sh's filename once `make -n ci`'s lint-shell leg
# lists first-party shell scripts by name (Makefile:650 SHELL_SCRIPTS).
#
# A trailing word-boundary is required on both terms: this repo's own
# worktree checkouts are named "stapler-squad-otel_<hash>" (this exact repo,
# in fact — see CLAUDE.md's worktree layout), and every absolute path in
# `make -n`'s output is rooted under one, so an unanchored
# "stapler-squad-otel" match false-positives on the checkout's own directory
# name in every recipe line that prints an absolute path (confirmed
# empirically: 7 such false-positive lines in `make -n quick-check`'s
# output alone). "stapler-squad-otel" is only a real match when followed by
# something other than [A-Za-z0-9_] (the binary is always invoked as
# `stapler-squad-otel` followed by whitespace, a quote, `.`, or end of line
# — never by `_`).
set -euo pipefail

TARGETS=(ci ready quick-check pre-commit install-service)
FORBIDDEN='otelc[^A-Za-z0-9_]|otelc$|stapler-squad-otel[^A-Za-z0-9_]|stapler-squad-otel$'

failures=()

for target in "${TARGETS[@]}"; do
  if ! output="$(make -n "$target" 2>&1)"; then
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
  exit 1
fi

exit 0
