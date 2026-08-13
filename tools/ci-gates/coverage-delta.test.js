'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { isActionable, previousCoveragePct, gateAction, findExisting, buildMarker } = require('./coverage-delta.js');

test('isActionable_should_ReturnTrue_When_CurrentCoveragePctDiffersFromPrevious', () => {
  const existingBody = '<!-- feature-coverage:pct=42.0 --> ## 📊 Feature E2E Coverage\n\nFeature E2E coverage: 21/50 tested (42.0%)';
  assert.equal(isActionable({ existingBody, currentPct: 48.3 }), true);
});

test('isActionable_should_ReturnFalse_When_CoverageUnchanged_And_NoDeleteRequested', () => {
  const existingBody = '<!-- feature-coverage:pct=42.0 --> ## 📊 Feature E2E Coverage\n\nFeature E2E coverage: 21/50 tested (42.0%)';
  assert.equal(isActionable({ existingBody, currentPct: 42.0 }), false);
  // Unchanged coverage is never a "delete" — the gate has only post/noop, no
  // delete branch at all, unlike the other 3 workflows.
  assert.equal(gateAction(false), 'noop');
});

test('isActionable_should_ReturnTrue_When_NoExistingComment', () => {
  assert.equal(isActionable({ existingBody: null, currentPct: 10 }), true);
});

test('previousCoveragePct_should_ReturnNull_When_CommentTemplateGainsASecondPercentage', () => {
  // Simulates a future per-category breakdown adding a second parenthesized
  // percentage before the marker's own value would be reached by a naive
  // free-text scrape — the anchored marker format is immune to this, but this
  // test documents the exact fragility architecture-review flagged so a
  // future template change is caught by a failing assertion instead of
  // discovered live if the marker itself is ever malformed.
  const malformed = '<!-- feature-coverage --> ## Coverage\n\nBackend (55%) / Frontend (38%) tested';
  assert.equal(previousCoveragePct(malformed), null);
});

test('previousCoveragePct_should_ReturnValue_When_MarkerPresent', () => {
  assert.equal(previousCoveragePct('<!-- feature-coverage:pct=42.0 --> text'), 42.0);
});

test('findExisting_should_MatchByStablePrefix_RegardlessOfEmbeddedPct', () => {
  const comments = [{ id: 1, body: '<!-- feature-coverage:pct=42.0 --> old' }];
  assert.equal(findExisting(comments).id, 1);
});

test('buildMarker_should_EmbedCurrentPct', () => {
  assert.equal(buildMarker(48.3), '<!-- feature-coverage:pct=48.3 -->');
});
