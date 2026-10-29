'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { evaluate, COUNT_BUDGETS } = require('./perf-budget.js');

const agg = (median, mad = 0) => ({ median, mad, runs: [median] });

function results({ flags = 1, peakInFlight = 4, status0 } = {}) {
  const metrics = { peakInFlight: agg(peakInFlight), longTaskMaxMs: agg(80), queuedOver1s: agg(0) };
  if (status0 !== undefined) metrics.status0Count = agg(status0);
  return {
    scenarios: {
      'load@4x': {
        keptRuns: 5,
        metrics,
        requestsByRpc: {
          GetFeatureFlags: agg(flags),
          ListProgramsConfig: agg(1),
          LogClientEvents: agg(4),
          GetFileTransportCapabilities: agg(1),
          ListWorkflows: agg(1),
        },
      },
    },
  };
}

test('perfBudget_should_FailCheck_When_14GetFeatureFlagsReintroduced_AndBlockingEnabled', () => {
  const out = evaluate(results({ flags: 14 }), null, { blocking: true });
  assert.equal(out.exitCode, 1);
  assert.deepEqual(out.blockingFailures.map((f) => f.metric), ['GetFeatureFlags']);
  assert.match(out.markdown, /GetFeatureFlags/);
});

test('perfBudget_should_PassAndComment_When_AdvisoryMode_AndRegressionPresent', () => {
  const out = evaluate(results({ flags: 14 }), null, { blocking: false });
  assert.equal(out.exitCode, 0);
  assert.equal(out.blockingFailures.length, 1);
  assert.match(out.markdown, /advisory/i);
  assert.match(out.markdown, /GetFeatureFlags/);
});

test('perfBudget_should_NotBlock_When_OnlyTimingMetricsRegress_AndBlockingEnabled', () => {
  const base = results({ peakInFlight: 4 });
  const cur = results({ peakInFlight: 40 });
  const out = evaluate(cur, base, { blocking: true });
  assert.equal(out.exitCode, 0);
  assert.equal(out.blockingFailures.length, 0);
  assert.deepEqual(out.advisoryRegressions.map((r) => r.metric), ['peakInFlight']);
});

test('perfBudget_should_FailCheck_When_Status0CountNonZero_AndBlockingEnabled', () => {
  const out = evaluate(results({ status0: 2 }), null, { blocking: true });
  assert.equal(out.exitCode, 1);
  assert.deepEqual(out.blockingFailures.map((f) => f.metric), ['status0Count']);
});

test('perfBudget_should_ScaleLogClientEventsBudget_When_LoadWindowDiffers', () => {
  assert.equal(COUNT_BUDGETS(16).LogClientEvents, 5);
  assert.equal(COUNT_BUDGETS(60).LogClientEvents, 13);
});

test('perfBudget_should_DegradeToAdvisory_When_BaselineAbsent', () => {
  const out = evaluate(results(), null, { blocking: true });
  assert.equal(out.exitCode, 0);
  assert.match(out.markdown, /no baseline/i);
});

test('perfBudget_should_SkipWithMessage_When_ResultsAbsent', () => {
  const out = evaluate(null, null, { blocking: true });
  assert.equal(out.exitCode, 0);
  assert.match(out.markdown, /no perf results/i);
});
