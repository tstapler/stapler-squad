#!/usr/bin/env bash
# One CI shard = two Playwright passes:
#   1. everything except terminal-stress, in parallel (E2E_WORKERS, one server per worker). The spec files are
#      assigned to shards by recorded duration (shard-timings.json, scripts/shard-assign.ts), because Playwright's
#      --shard splits by test count and left one shard at 36 min while others took 16. No usable plan fails the shard.
#   2. terminal-stress only, one worker: those specs assert absolute throughput and heap growth,
#      which sibling workers starve of CPU (see playwright.config.ts).
# Both passes carry a --global-timeout inside E2E_SHARD_BUDGET_S, so a failing-heavy shard stops with a clean
# report (tests not reached are "interrupted" and the comparator/confirmation rerun sees them) instead of being
# killed by the caller's `timeout` with no report.
# Raw test failures stay report-only; the caller still checks for blob reports and runs
# assert-e2e-report.mjs on both JSON reports.
# PR runs may set E2E_SPEC_SELECTION (newline-separated spec paths from scripts/select-specs.ts): only those files are
# planned across the shards, and the stress pass runs only the selected terminal-stress specs, on shard 1. Unset = full suite.
# PR runs set E2E_SKIP_BASELINE_FAIL=1: tests the baseline records as class "fail" are excluded via --grep-invert
# (scripts/baseline-skip.ts) from the plan and both passes; the skipped set is printed and appended to the step summary.
# Spec files in E2E_KEEP_SPECS (the PR's own changed specs) are never skipped.
# main, nightly and workflow_dispatch leave it unset and run everything.
#   usage: run-ci-shard.sh <shard> <total>
set -u
cd "$(dirname "$0")/.." || exit 1
shard=$1 total=$2
projects=(--project=chromium --project=chromium-dom --project=mobile-chromium)
skip_args=()
if [ "${E2E_SKIP_BASELINE_FAIL:-}" = 1 ]; then
  command -v bun >/dev/null 2>&1 || { echo "::error::run-ci-shard: bun is not on PATH" >&2; exit 2; }
  skip_re=$(bun scripts/baseline-skip.ts grep-invert) || { echo "::error::run-ci-shard: baseline-skip failed" >&2; exit 2; }
  skip_args=("--grep-invert=$skip_re")
fi
start=$SECONDS
budget_s=${E2E_SHARD_BUDGET_S:-2280}          # 38 min: inside the workflow's 40 min `timeout`
stress_reserve_s=${E2E_STRESS_RESERVE_S:-240}
pass1_limit_s=$(( budget_s - stress_reserve_s ))

# Duration-balanced file list for this shard. There is deliberately no --shard fallback: the other shards run the
# balanced plan, so one shard silently falling back to count-based --shard=k/N would lose some tests and run others
# twice. Any problem here fails the shard loudly instead.
fail_plan() { echo "::error::run-ci-shard: cannot build the balanced shard plan: $1" >&2; exit 2; }
[ -f shard-timings.json ] || fail_plan "tests/e2e/shard-timings.json is missing"
command -v bun >/dev/null 2>&1 || fail_plan "bun is not on PATH"
list_err=$(mktemp)
list_json=$(E2E_SKIP_STRESS=1 npx playwright test --list --reporter=json "${projects[@]}" ${skip_args[@]+"${skip_args[@]}"} 2>"$list_err") \
  || { cat "$list_err" >&2; fail_plan "playwright --list failed"; }
if [ "${E2E_SKIP_BASELINE_FAIL:-}" = 1 ]; then
  # Count what the regex really removes (parallel-pass list, stress specs excluded), not the baseline row count.
  all_json=$(E2E_SKIP_STRESS=1 npx playwright test --list --reporter=json "${projects[@]}" 2>"$list_err") \
    || { cat "$list_err" >&2; fail_plan "playwright --list (unfiltered) failed"; }
  printf '%s' "$all_json" > "$list_err"; n_all=$(bun scripts/baseline-skip.ts count "$list_err")
  printf '%s' "$list_json" > "$list_err"; n_kept=$(bun scripts/baseline-skip.ts count "$list_err")
  skip_summary=$(bun scripts/baseline-skip.ts summary --removed $(( n_all - n_kept ))) || fail_plan "baseline-skip summary failed"
  printf '%s\n' "$skip_summary"
  [ -n "${GITHUB_STEP_SUMMARY:-}" ] && printf '%s\n' "$skip_summary" >> "$GITHUB_STEP_SUMMARY"
fi
files=$(printf '%s' "$list_json" | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{const r=JSON.parse(d);const s=new Set();const w=x=>{(x.specs||[]).forEach(p=>s.add(p.file));(x.suites||[]).forEach(w)};(r.suites||[]).forEach(w);console.log([...s].join("\n"))})') \
  || fail_plan "could not read the spec list from playwright --list"
[ -n "$files" ] || fail_plan "playwright --list returned no spec files"
selection_mode=0
if [ -n "${E2E_SPEC_SELECTION:-}" ]; then
  selection_mode=1
  files=$(printf '%s\n' "$files" | grep -Fxf <(printf '%s\n' "$E2E_SPEC_SELECTION") || true)
  [ -n "$files" ] || fail_plan "E2E_SPEC_SELECTION matched none of the specs Playwright lists"
  echo "run-ci-shard: selection mode, $(printf '%s\n' "$files" | wc -l | tr -d ' ') spec files in play"
fi
assigned=$(printf '%s\n' "$files" | bun scripts/shard-assign.ts assign --shard "$shard" --total "$total") \
  || fail_plan "shard-assign failed"
if [ -z "$assigned" ] && [ "$selection_mode" != 1 ]; then
  fail_plan "shard $shard/$total got no spec files"
fi
# Portable read loop (macOS bash 3 has no mapfile).
shard_args=()
while IFS= read -r line; do [ -n "$line" ] && shard_args+=("$line"); done <<<"$assigned"

# Selection mode only: a shard with no selected specs is intentional, and the marker tells the workflow's assert step
# so apart from a shard that crashed before reporting.
parallel_rc=0
if [ "${#shard_args[@]}" -eq 0 ]; then
  echo "run-ci-shard: shard $shard/$total has no selected specs; skipping the parallel pass"
  : > shard-pass1-empty
else
  E2E_SKIP_STRESS=1 PLAYWRIGHT_JSON_OUTPUT_NAME="$PWD/shard-report.json" \
    npx playwright test --reporter=list,blob,json "--global-timeout=$(( pass1_limit_s * 1000 ))" "${shard_args[@]}" "${projects[@]}" ${skip_args[@]+"${skip_args[@]}"}
  parallel_rc=$?
fi
remaining_s=$(( budget_s - (SECONDS - start) ))
[ "$remaining_s" -lt 60 ] && remaining_s=60

stress_args=("--shard=$shard/$total")
if [ "$selection_mode" = 1 ]; then
  stress_args=()
  if [ "$shard" = 1 ]; then
    while IFS= read -r line; do [ -n "$line" ] && stress_args+=("tests/e2e/${line//./\\.}\$"); done < <(printf '%s\n' "$E2E_SPEC_SELECTION" | grep '^terminal-stress/' || true)
  fi
  if [ "${#stress_args[@]}" -eq 0 ]; then
    echo "run-ci-shard: selection mode, no stress specs selected for shard $shard; skipping the stress pass"
    : > shard-stress-skipped
    exit $(( parallel_rc ))
  fi
fi

# The blob reporter empties blob-report/ at start unless told not to; keep pass 1's zip.
E2E_STRESS_ONLY=1 E2E_WORKERS=1 PWTEST_BLOB_DO_NOT_REMOVE=1 \
  PLAYWRIGHT_BLOB_OUTPUT_NAME="report-stress-$shard.zip" PLAYWRIGHT_JSON_OUTPUT_NAME="$PWD/shard-report-stress.json" \
  npx playwright test --reporter=list,blob,json "--global-timeout=$(( remaining_s * 1000 ))" "${stress_args[@]}" --project=chromium --project=chromium-dom ${skip_args[@]+"${skip_args[@]}"}
stress_rc=$?

echo "run-ci-shard: parallel pass exit=$parallel_rc, stress pass exit=$stress_rc"
exit $(( parallel_rc != 0 ? parallel_rc : stress_rc ))
