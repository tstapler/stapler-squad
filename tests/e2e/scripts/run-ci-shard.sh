#!/usr/bin/env bash
# One CI shard = two Playwright passes:
#   1. everything except terminal-stress, in parallel (E2E_WORKERS, one server per worker). The spec files are
#      assigned to shards by recorded duration (shard-timings.json, scripts/shard-assign.ts), because Playwright's
#      --shard splits by test count and left one shard at 36 min while others took 16. Without a usable timings
#      file or bun it falls back to --shard.
#   2. terminal-stress only, one worker: those specs assert absolute throughput and heap growth,
#      which sibling workers starve of CPU (see playwright.config.ts).
# Both passes carry a --global-timeout inside E2E_SHARD_BUDGET_S, so a failing-heavy shard stops with a clean
# report (tests not reached are "interrupted" and the comparator/confirmation rerun sees them) instead of being
# killed by the caller's `timeout` with no report.
# Raw test failures stay report-only; the caller still checks for blob reports and runs
# assert-e2e-report.mjs on both JSON reports.
#   usage: run-ci-shard.sh <shard> <total>
set -u
cd "$(dirname "$0")/.." || exit 1
shard=$1 total=$2
projects=(--project=chromium --project=chromium-dom --project=mobile-chromium)
start=$SECONDS
budget_s=${E2E_SHARD_BUDGET_S:-2280}          # 38 min: inside the workflow's 40 min `timeout`
stress_reserve_s=${E2E_STRESS_RESERVE_S:-240}
pass1_limit_s=$(( budget_s - stress_reserve_s ))

# Duration-balanced file list for this shard, or the count-based --shard fallback.
shard_args=("--shard=$shard/$total")
if [ -f shard-timings.json ] && command -v bun >/dev/null 2>&1; then
  list_json=$(E2E_SKIP_STRESS=1 npx playwright test --list --reporter=json "${projects[@]}" 2>/dev/null || true)
  files=$(printf '%s' "$list_json" | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{try{const r=JSON.parse(d);const s=new Set();const w=x=>{(x.specs||[]).forEach(p=>s.add(p.file));(x.suites||[]).forEach(w)};(r.suites||[]).forEach(w);console.log([...s].join("\n"))}catch(e){}})')
  if [ -n "$files" ] && assigned=$(printf '%s\n' "$files" | bun scripts/shard-assign.ts assign --shard "$shard" --total "$total") && [ -n "$assigned" ]; then
    mapfile -t shard_args <<<"$assigned"
  else
    echo "run-ci-shard: duration assignment unavailable, falling back to --shard"
  fi
fi

E2E_SKIP_STRESS=1 PLAYWRIGHT_JSON_OUTPUT_NAME="$PWD/shard-report.json" \
  npx playwright test --reporter=list,blob,json "--global-timeout=$(( pass1_limit_s * 1000 ))" "${shard_args[@]}" "${projects[@]}"
parallel_rc=$?
remaining_s=$(( budget_s - (SECONDS - start) ))
[ "$remaining_s" -lt 60 ] && remaining_s=60

# The blob reporter empties blob-report/ at start unless told not to; keep pass 1's zip.
E2E_STRESS_ONLY=1 E2E_WORKERS=1 PWTEST_BLOB_DO_NOT_REMOVE=1 \
  PLAYWRIGHT_BLOB_OUTPUT_NAME="report-stress-$shard.zip" PLAYWRIGHT_JSON_OUTPUT_NAME="$PWD/shard-report-stress.json" \
  npx playwright test --reporter=list,blob,json "--global-timeout=$(( remaining_s * 1000 ))" "--shard=$shard/$total" --project=chromium --project=chromium-dom
stress_rc=$?

echo "run-ci-shard: parallel pass exit=$parallel_rc, stress pass exit=$stress_rc"
exit $(( parallel_rc != 0 ? parallel_rc : stress_rc ))
