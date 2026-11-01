#!/usr/bin/env bash
# One CI shard = two Playwright passes over the same --shard slice:
#   1. everything except terminal-stress, in parallel (E2E_WORKERS, one server per worker);
#   2. terminal-stress only, one worker: those specs assert absolute throughput and heap growth,
#      which sibling workers starve of CPU (see playwright.config.ts).
# Raw test failures stay report-only; the caller still checks for blob reports and runs
# assert-e2e-report.mjs on both JSON reports.
#   usage: run-ci-shard.sh <shard> <total>
set -u
cd "$(dirname "$0")/.." || exit 1
shard=$1 total=$2
projects=(--project=chromium --project=chromium-dom --project=mobile-chromium)

E2E_SKIP_STRESS=1 PLAYWRIGHT_JSON_OUTPUT_NAME="$PWD/shard-report.json" \
  npx playwright test --reporter=list,blob,json "--shard=$shard/$total" "${projects[@]}"
parallel_rc=$?

# The blob reporter empties blob-report/ at start unless told not to; keep pass 1's zip.
E2E_STRESS_ONLY=1 E2E_WORKERS=1 PWTEST_BLOB_DO_NOT_REMOVE=1 \
  PLAYWRIGHT_BLOB_OUTPUT_NAME="report-stress-$shard.zip" PLAYWRIGHT_JSON_OUTPUT_NAME="$PWD/shard-report-stress.json" \
  npx playwright test --reporter=list,blob,json "--shard=$shard/$total" --project=chromium --project=chromium-dom
stress_rc=$?

echo "run-ci-shard: parallel pass exit=$parallel_rc, stress pass exit=$stress_rc"
exit $(( parallel_rc != 0 ? parallel_rc : stress_rc ))
