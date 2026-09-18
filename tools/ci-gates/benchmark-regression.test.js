'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const {
  hasGoTier1Regression,
  hasThroughputRegression,
  hasLatencyRegression,
} = require('./benchmark-regression.js');

// Literal output captured from a real `benchstat old.txt new.txt` run
// (golang.org/x/perf/cmd/benchstat@v0.0.0-20260312031701-16a31bc5fbd0, the
// version pinned in benchmark.yml/build.yml) against an 8-sample baseline
// where Foo regressed +30% and Bar was noise (~).
const REGRESSED = [
  'goos: linux',
  'goarch: amd64',
  'pkg: example',
  'cpu: Test',
  '        │   old.txt   │              new.txt               │',
  '        │   sec/op    │   sec/op     vs base               │',
  'Foo-8     100.0n ± 3%   130.0n ± 2%  +30.00% (p=0.000 n=8)',
  'Bar-8     200.0n ± 1%   200.5n ± 2%        ~ (p=0.862 n=8)',
  'geomean   141.4n        161.4n       +14.16%',
].join('\n');

const INSIGNIFICANT_ONLY = [
  'goos: linux',
  'goarch: amd64',
  'pkg: example',
  'cpu: Test',
  '        │   old.txt   │             old2.txt              │',
  '        │   sec/op    │   sec/op     vs base              │',
  'Foo-8     100.0n ± 3%   100.0n ± 3%       ~ (p=1.000 n=8)',
  'Bar-8     200.0n ± 1%   200.0n ± 1%       ~ (p=1.000 n=8)',
  'geomean   141.4n        141.4n       +0.00%',
].join('\n');

test('hasGoTier1Regression_should_ReturnTrue_When_BenchstatDeltaExceeds20PercentWithSignificance', () => {
  assert.equal(hasGoTier1Regression(REGRESSED), true);
});

test('hasGoTier1Regression_should_ReturnFalse_When_AllBenchstatDeltasAreStatisticallyInsignificantTildeMarks', () => {
  assert.equal(hasGoTier1Regression(INSIGNIFICANT_ONLY), false);
});

test('hasGoTier1Regression_should_ReturnFalse_When_DeltaIsExactlyAtThresholdMinusOneHundredth', () => {
  const line = 'Foo-8   100.0n ± 3%   119.9n ± 2%  +19.99% (p=0.000 n=8)';
  assert.equal(hasGoTier1Regression(line), false);
});

test('hasGoTier1Regression_should_ReturnTrue_When_DeltaIsExactlyAtThreshold', () => {
  const line = 'Foo-8   100.0n ± 3%   120.0n ± 2%  +20.00% (p=0.000 n=8)';
  assert.equal(hasGoTier1Regression(line), true);
});

test('hasGoTier1Regression_should_ReturnFalse_When_OutputIsEmptyOrMissingBaseline', () => {
  assert.equal(hasGoTier1Regression(''), false);
  assert.equal(hasGoTier1Regression('No baseline yet — will be established on next push to main.'), false);
});

test('hasGoTier1Regression_should_IgnoreGeomeanAggregateRow', () => {
  // Only the geomean row crosses 20% — no individual benchmark regressed.
  const line = [
    'Foo-8   100.0n ± 3%   105.0n ± 2%   ~ (p=0.200 n=8)',
    'geomean 100.0n        130.0n        +30.00%',
  ].join('\n');
  assert.equal(hasGoTier1Regression(line), false);
});

test('hasThroughputRegression_should_ReturnTrue_When_ThroughputPctNumIsAtOrBelowNegative50', () => {
  assert.equal(hasThroughputRegression([-10, -60]), true);
});

test('hasThroughputRegression_should_ReturnFalse_When_ThroughputPctNumIsNoisyButAboveNegative50Threshold', () => {
  assert.equal(hasThroughputRegression([-49, 20, -30]), false);
});

test('hasLatencyRegression_should_ReturnTrue_When_LatencyPctNumIsAtOrAbove100', () => {
  assert.equal(hasLatencyRegression([10, 110]), true);
});

test('hasLatencyRegression_should_ReturnFalse_When_LatencyPctNumIsBelow100Threshold', () => {
  assert.equal(hasLatencyRegression([99, -10, 50]), false);
});
