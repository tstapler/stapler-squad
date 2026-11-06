#!/usr/bin/env bash
# Gate S0 one-command re-run (plan Task 0.1d). Run on a Bun-installed web-app/ tree; prints the commit it ran on.
#   spikes/s0-rerun.sh [web-app-dir]
# Env: BASELINE_JS_BYTES (default 21666571, the pnpm development-mode build recorded in the plan; stale once main grows,
#      so pass the bytes of a pnpm control build of the same commit when you have one), TOLERANCE_PCT (default 2),
#      CONTROL_JEST_JSON (a `jest --json` output from a control tree; default: expect an empty failing-file list),
#      SKIP="build jest storybook eslint ve" to skip steps.
# Exit status: number of failed checks. Needs: bun, node, a generated web-app/src/gen (make proto-gen).
set -u
APP=$(cd "${1:-web-app}" && pwd)
REPO=$(cd "$APP/.." && pwd)
SPIKES=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BASELINE=${BASELINE_JS_BYTES:-21666571}
TOL=${TOLERANCE_PCT:-2}
SKIP=${SKIP:-}
unset npm_config_user_agent
fail=0
skip() { [[ " $SKIP " == *" $1 "* ]]; }
report() { printf '%-5s %-10s %s\n' "$1" "$2" "$3"; [[ $1 == FAIL ]] && fail=$((fail + 1)); return 0; }

echo "S0 re-run on commit $(git -C "$REPO" rev-parse HEAD) (bun $(bun --version)) in $APP"
cd "$APP" || exit 99

if ! skip build; then
  rm -rf out .next
  if NEXT_BUILD_MODE=development bun run build >"${TMPDIR:-/tmp}/s0-build.log" 2>&1; then
    bytes=$(find out -name '*.js' -type f -print0 | xargs -0 cat | wc -c)
    pct=$(awk -v b="$bytes" -v base="$BASELINE" 'BEGIN { printf "%.2f", (b - base) * 100 / base }')
    within=$(awk -v p="$pct" -v t="$TOL" 'BEGIN { p = (p < 0) ? -p : p; print (p <= t) ? 1 : 0 }')
    report "$([[ $within == 1 ]] && echo PASS || echo FAIL)" build "next build exit 0; JS bytes $bytes vs baseline $BASELINE (${pct}%, tolerance ${TOL}%)"
  else
    report FAIL build "next build failed, see ${TMPDIR:-/tmp}/s0-build.log"
  fi
fi

if ! skip jest; then
  jq="${TMPDIR:-/tmp}/s0-jest.json"
  CI=true ./node_modules/.bin/jest --no-coverage --json --outputFile="$jq" >"${TMPDIR:-/tmp}/s0-jest.log" 2>&1
  failing=$(node -e 'const j=require(process.argv[1]);console.log(j.testResults.filter(r=>r.status!=="passed").map(r=>r.name.replace(process.cwd()+"/","")).sort().join("\n"))' "$jq")
  expected=""
  [[ -n ${CONTROL_JEST_JSON:-} ]] && expected=$(node -e 'const j=require(process.argv[1]);console.log(j.testResults.filter(r=>r.status!=="passed").map(r=>r.name.replace(/^.*\/web-app\//,"")).sort().join("\n"))' "$CONTROL_JEST_JSON")
  counts=$(node -e 'const j=require(process.argv[1]);console.log(j.numTotalTestSuites+" suites, "+j.numTotalTests+" tests, "+j.numFailedTests+" failed")' "$jq")
  if [[ "$failing" == "$expected" ]]; then report PASS jest "failing-file list identical to control ($counts)"; else report FAIL jest "failing files differ from control: $failing"; fi
fi

if ! skip storybook; then
  if ./node_modules/.bin/storybook build -o "${TMPDIR:-/tmp}/s0-storybook" --disable-telemetry >"${TMPDIR:-/tmp}/s0-storybook.log" 2>&1; then
    report PASS storybook "storybook build exit 0"
  else
    report FAIL storybook "storybook build failed: $(grep -m1 -i 'error' "${TMPDIR:-/tmp}/s0-storybook.log" | cut -c1-120)"
  fi
fi

if ! skip eslint; then
  rules=$(./node_modules/.bin/eslint --print-config src/lib/nav/index.ts 2>/dev/null | grep -cE '"(analytics|rpc-lifecycle)/')
  if ./node_modules/.bin/eslint src/lib/nav >"${TMPDIR:-/tmp}/s0-eslint.log" 2>&1 && [[ $rules -gt 0 ]]; then
    report PASS eslint "file: plugins loaded ($rules plugin rules in the resolved config), src/lib/nav clean"
  else
    report FAIL eslint "plugins loaded: $rules rules; see ${TMPDIR:-/tmp}/s0-eslint.log"
  fi
fi

if ! skip ve; then
  if bun "$SPIKES/ve-compile3.ts" >"${TMPDIR:-/tmp}/s0-ve.log" 2>&1; then report PASS ve "VE compile of 3 files"; else report FAIL ve "$(tail -1 "${TMPDIR:-/tmp}/s0-ve.log")"; fi
fi

echo "failed checks: $fail"
exit "$fail"
