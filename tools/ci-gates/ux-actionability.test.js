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
