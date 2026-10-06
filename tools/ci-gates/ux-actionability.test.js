'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { isActionable } = require('./ux-actionability.js');

test('isActionable_should_ReturnTrue_When_AxeFails', () => {
  assert.equal(
    isActionable({ axeOutcome: 'failure', lighthouseScore: '85', findingsCount: 0 }),
    true,
  );
});

test('isActionable_should_ReturnTrue_When_LighthouseScoreIsNaN_ViaExplicitIsNaNCheck_NotBareLessThanComparison', () => {
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: 'unknown', findingsCount: 0 }),
    true,
  );
});

test('isActionable_should_ReturnTrue_When_LighthouseScoreBelow70', () => {
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '62', findingsCount: 0 }),
    true,
  );
});

test('isActionable_should_ReturnTrue_When_FindingsCountAboveZero', () => {
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '85', findingsCount: 2 }),
    true,
  );
});

test('isActionable_should_ReturnFalse_When_AxePassesLighthouseHighAndZeroFindings', () => {
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '85', findingsCount: 0 }),
    false,
  );
});

test('isActionable_should_ReturnFalse_When_LighthouseScoreExactlyAtThreshold', () => {
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '70', findingsCount: 0 }),
    false,
  );
});

test('isActionable_should_ReturnTrue_When_FindingsCountIsNegativeSentinel_AnalysisCrashedMidRun', () => {
  // analyze.ts writes findings_count=-1 before doing any work, overwritten
  // with the real count only on successful completion — a negative value
  // means the analysis started but never finished, not "found nothing".
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '85', findingsCount: -1 }),
    true,
  );
});

test('isActionable_should_ReturnFalse_When_FindingsCountAbsent_AnalysisStepNeverRan', () => {
  // Distinct from the crash case: an *absent* count (the Claude-analysis
  // step was conditionally skipped, e.g. no API key configured) is a
  // permanent, expected state for some repos — must stay non-actionable,
  // not reintroduce noise on every green PR forever.
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '85', findingsCount: undefined }),
    false,
  );
  assert.equal(
    isActionable({ axeOutcome: 'success', lighthouseScore: '85', findingsCount: '' }),
    false,
  );
});
