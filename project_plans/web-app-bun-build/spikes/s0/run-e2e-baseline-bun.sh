#!/usr/bin/env bash
# Task 7.11c / Story 8.1a: runs the full Playwright suite 3 times against a Bun build
# (WEB_BUILD=bun) using the e2e harness's own isolated global-setup (dynamic free port,
# PID-scoped temp dir -- never :8543), capturing a JSON report per run. Deliberate deviation from
# the plan's literal "a ManualInstance (ports 62871-62880)" wording for this task: global-setup.ts
# has no mode to point Playwright at an already-running external server, and the harness's own
# per-run isolation (dynamic port, PID-scoped temp dir, private tmux socket) already satisfies the
# actual safety goal that recipe protects -- never touching the live/default service port -- more
# robustly than hand-managing a long-lived ManualInstance across 3 sequential full-suite runs.
#
# Usage: spikes/s0/run-e2e-baseline-bun.sh [outdir]
# Then compare each run against the Next baseline + e2e-next's report with scripts/e2e-baseline-compare.ts
# (run that separately per run -- see spikes/s0/README or the PR description for the exact invocation).
set -euo pipefail
unset npm_config_user_agent

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../../.." && pwd)"
OUT="${1:-${TMPDIR:-/tmp}/e2e-baseline-bun-$(date +%s)}"
mkdir -p "$OUT"
echo "Output dir: $OUT"
echo "Commit: $(git -C "$REPO" rev-parse HEAD)"

cd "$REPO/tests/e2e"

for i in 1 2 3; do
  echo "=== Bun run $i/3 starting $(date -u +%FT%TZ) ==="
  # CI=true: avoids Playwright's html reporter blocking forever serving its report after the run
  # finishes (see run-e2e-baseline.sh's identical note -- hit this for real on the Next capture).
  PLAYWRIGHT_JSON_REPORT_PATH="$OUT/bun-run-$i.json" \
    WEB_BUILD=bun \
    CI=true \
    npx playwright test 2>&1 | tee "$OUT/bun-run-$i.log" || true
  echo "=== Bun run $i/3 finished $(date -u +%FT%TZ) ==="
done

echo "Raw Bun reports and logs in $OUT (not committed)"
echo "Next, run scripts/e2e-baseline-compare.ts against tests/e2e/baseline/next-e2e-baseline.json,"
echo "tests/e2e/baseline/bun-expected-diff.json and the e2e-next-equivalent report, --bun-reports $OUT/bun-run-{1,2,3}.json"
