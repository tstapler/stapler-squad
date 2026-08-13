'use strict';

// Regression thresholds for benchmark.yml's 3 PR-comment jobs.
// go-tier1 is repeated-trial + significance-tested (benchstat, count=8), so it
// reuses build.yml's benchmark-gate 20% figure. frontend-throughput/e2e-latency
// are single-sample Playwright measurements with no significance test, so they
// use a much coarser "2x swing" threshold instead — see ADR-002 (amended).
const GO_TIER1_THRESHOLD_PCT = 20;
const THROUGHPUT_HALVED_PCT = -50;
const LATENCY_DOUBLED_PCT = 100;

// Matches a benchstat (golang.org/x/perf, modern table format) per-benchmark
// delta line, e.g.:
//   Foo-8     100.0n ± 3%   130.0n ± 2%  +30.00% (p=0.000 n=8)
//   Bar-8     200.0n ± 1%   200.5n ± 2%        ~ (p=0.862 n=8)
// A "~" delta means benchstat's own default alpha=0.05 significance test
// found no real difference — those rows never match this regex, so they're
// excluded automatically. NOTE: the pinned benchstat version has no
// `-delta-test` flag (removed in the table-format rewrite); significance
// filtering is already built in via `~`, no extra flag needed.
const DELTA_LINE = /^(\S+)\s+.+?\s+([+-]?\d+(?:\.\d+)?)%\s+\(p=/;

function hasGoTier1Regression(benchstatOutput, thresholdPct) {
  const threshold = thresholdPct === undefined ? GO_TIER1_THRESHOLD_PCT : thresholdPct;
  if (!benchstatOutput) return false;
  return benchstatOutput.split('\n').some((line) => {
    const trimmed = line.trim();
    // geomean is an aggregate across all benchmarks, not a specific
    // regression — excluded so one fast benchmark can't mask, or one slow
    // benchmark can't be double-counted via, the summary row.
    if (trimmed.indexOf('geomean') === 0) return false;
    const m = trimmed.match(DELTA_LINE);
    if (!m) return false;
    return parseFloat(m[2]) >= threshold;
  });
}

function hasThroughputRegression(pctNums, thresholdPct) {
  const threshold = thresholdPct === undefined ? THROUGHPUT_HALVED_PCT : thresholdPct;
  return pctNums.some((pct) => typeof pct === 'number' && !isNaN(pct) && pct <= threshold);
}

function hasLatencyRegression(pctNums, thresholdPct) {
  const threshold = thresholdPct === undefined ? LATENCY_DOUBLED_PCT : thresholdPct;
  return pctNums.some((pct) => typeof pct === 'number' && !isNaN(pct) && pct >= threshold);
}

// Decision for the sticky-comment CRUD step: post/update while regressing,
// delete a stale comment once it clears, otherwise leave things alone.
function gateAction(hasRegression, hasExistingComment) {
  if (hasRegression) return 'post';
  if (hasExistingComment) return 'delete';
  return 'noop';
}

module.exports = {
  GO_TIER1_THRESHOLD_PCT,
  THROUGHPUT_HALVED_PCT,
  LATENCY_DOUBLED_PCT,
  hasGoTier1Regression,
  hasThroughputRegression,
  hasLatencyRegression,
  gateAction,
};
