#!/usr/bin/env bash
# Task 0.1c/0.1e: runs the full Playwright suite 3 times against a Next build (WEB_BUILD unset)
# using the e2e harness's own isolated global-setup (dynamic free port, PID-scoped temp dir,
# never the live/default service port), capturing a JSON report and routesVisited JSONL per run,
# then classifies the three reports into tests/e2e/baseline/next-e2e-baseline.json (Story 0.1's
# gate rule).
#
# Usage: spikes/s0/run-e2e-baseline.sh [outdir]
#   outdir defaults to a scratch dir under $TMPDIR; the 3 raw JSON reports are NOT committed
#   (Task 0.1c: "kept in the scratchpad, not committed") -- only the classified baseline is.
set -euo pipefail
unset npm_config_user_agent

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../../.." && pwd)"
OUT="${1:-${TMPDIR:-/tmp}/e2e-baseline-next-$(date +%s)}"
mkdir -p "$OUT"
echo "Output dir: $OUT"
echo "Commit: $(git -C "$REPO" rev-parse HEAD)"

cd "$REPO/tests/e2e"

for i in 1 2 3; do
  echo "=== Next baseline run $i/3 starting $(date -u +%FT%TZ) ==="
  rm -f /tmp/e2e-routes-visited.jsonl
  # CI=true: without it, Playwright's html reporter (still in the config's reporter array; only
  # the json entry's path varies per run) decides -- post-run, after the json report is already
  # written -- to serve the report at localhost:9323 and block forever waiting for Ctrl+C
  # (lib/reporters/html.js's `if (process.env.CI || !this._buildResult) return;`). Hit this for
  # real on run 1/3 of the first attempt: it silently hung for ~13h after the actual 8.2h test run
  # had already finished and wiped run-1.json to disk.
  E2E_ROUTES_VISITED_FILE=/tmp/e2e-routes-visited.jsonl \
    NODE_OPTIONS="--require $HERE/../../../../tests/e2e/baseline/route-visit-preload.js" \
    PLAYWRIGHT_JSON_REPORT_PATH="$OUT/run-$i.json" \
    WEB_BUILD=next \
    CI=true \
    npx playwright test 2>&1 | tee "$OUT/run-$i.log" || true
  cp /tmp/e2e-routes-visited.jsonl "$OUT/routes-$i.jsonl" 2>/dev/null || touch "$OUT/routes-$i.jsonl"
  echo "=== Next baseline run $i/3 finished $(date -u +%FT%TZ) ==="
done

cat "$OUT"/routes-*.jsonl > "$OUT/routes-all.jsonl" 2>/dev/null || touch "$OUT/routes-all.jsonl"

cat > "$OUT/meta.json" <<EOF
{
  "commit": "$(git -C "$REPO" rev-parse HEAD)",
  "capturedAt": "$(date -u +%FT%TZ)",
  "nextVersion": "$(node -e "console.log(require('$REPO/web-app/node_modules/next/package.json').version)" 2>/dev/null || echo unknown)",
  "playwrightVersion": "$(node -e "console.log(require('$REPO/tests/e2e/node_modules/@playwright/test/package.json').version)" 2>/dev/null || echo unknown)"
}
EOF

node "$HERE/classify-e2e.mjs" \
  "$REPO/tests/e2e/baseline/next-e2e-baseline.json" \
  "$OUT/meta.json" \
  "$OUT/run-1.json" "$OUT/run-2.json" "$OUT/run-3.json" \
  --routes-jsonl "$OUT/routes-all.jsonl" \
  --build-coupled "$REPO/tests/e2e/baseline/build-coupled-specs.json"

echo "Wrote $REPO/tests/e2e/baseline/next-e2e-baseline.json"
echo "Raw reports and logs kept in $OUT (not committed)"
