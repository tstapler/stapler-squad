#!/usr/bin/env bash
# Runs one MCP integration suite under a 25m soft budget. Env: SUITE, PACKAGES, RACE (may be empty).
set -u  # no pipefail: PIPESTATUS[0] below carries go test's status
args=(-tags=integration -timeout=1200s -v)
if [ -n "${RACE:-}" ]; then args+=("$RACE"); fi
out=$(mktemp)
timeout 25m go test "${args[@]}" "$PACKAGES" 2>&1 | tee "$out" || true
rc=${PIPESTATUS[0]}
# Only a budget breach carries the marker, not a plain test failure (the auto-fix webhook would
# skip it as "slow, not broken"): the outer `timeout` firing (124), or go test's own -timeout.
# keep this marker prefix in sync with ciBudgetExceededMarker in server/services/github_webhook_pr_fix.go
if [ "$rc" -eq 124 ] || grep -q 'panic: test timed out' "$out"; then
  echo "::error::CI-BUDGET-EXCEEDED: 'mcp-integration ($SUITE)' exceeded its 25m budget"
  echo "CI-BUDGET-EXCEEDED: 'mcp-integration ($SUITE)' exceeded its 25m budget" >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
fi
exit "$rc"
