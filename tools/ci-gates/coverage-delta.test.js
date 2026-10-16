'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const {
  isActionable,
  previousCoveragePct,
  parseCoveragePct,
  findExisting,
  buildMarker,
} = require('./coverage-delta.js');

test('isActionable_should_ReturnTrue_When_CurrentCoveragePctDiffersFromPrevious', () => {
  const existingBody = '<!-- feature-coverage:pct=42.0 --> ## 📊 Feature E2E Coverage\n\nFeature E2E coverage: 21/50 tested (42.0%)';
  assert.equal(isActionable({ existingBody, currentPct: 48.3 }), true);
});

test('isActionable_should_ReturnFalse_When_CoverageUnchanged_And_NoDeleteRequested', () => {
  const existingBody = '<!-- feature-coverage:pct=42.0 --> ## 📊 Feature E2E Coverage\n\nFeature E2E coverage: 21/50 tested (42.0%)';
  assert.equal(isActionable({ existingBody, currentPct: 42.0 }), false);
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

test('isActionable_should_ReturnTrue_When_MarkerIsMalformedOrMissing_SafeFallback', () => {
  // Plan Task 3.1.1e's explicit end-to-end case: a malformed/missing marker
  // must fall back to "post", not silently swallow the comparison.
  const malformed = '<!-- feature-coverage --> ## Coverage\n\nBackend (55%) / Frontend (38%) tested';
  assert.equal(isActionable({ existingBody: malformed, currentPct: 55 }), true);
});

test('parseCoveragePct_should_ExtractPercentage_When_SummaryLineWellFormed', () => {
  assert.equal(parseCoveragePct('Feature E2E coverage: 21/50 tested (42%)'), 42);
});

test('parseCoveragePct_should_ReturnNull_When_SummaryLineHasNoPercentage', () => {
  assert.equal(parseCoveragePct('Feature E2E coverage: N/A'), null);
  assert.equal(parseCoveragePct(''), null);
  assert.equal(parseCoveragePct(undefined), null);
});

test('previousCoveragePct_should_ReturnValue_When_MarkerPresent', () => {
  assert.equal(previousCoveragePct('<!-- feature-coverage:pct=42.0 --> text'), 42.0);
});

test('findExisting_should_MatchByStablePrefix_RegardlessOfEmbeddedPct', () => {
  const comments = [{ id: 1, body: '<!-- feature-coverage:pct=42.0 --> old' }];
  assert.equal(findExisting(comments).id, 1);
});

test('findExisting_should_MatchLegacyBareMarker_When_PostedBeforeThePctFormatExisted', () => {
  // Regression guard: a PR with an already-posted <!-- feature-coverage -->
  // comment (the format this item replaced) must still be found — otherwise
  // isActionable sees existingBody=null ("no prior state"), always posts,
  // and the old comment is orphaned as a permanent duplicate forever
  // (this workflow never deletes on unchanged, so nothing ever cleans it up).
  const comments = [{ id: 7, body: '<!-- feature-coverage --> ## Coverage\n\nFeature E2E coverage: 21/50 tested (42%)' }];
  assert.equal(findExisting(comments).id, 7);
});

test('buildMarker_should_EmbedCurrentPct', () => {
  assert.equal(buildMarker(48.3), '<!-- feature-coverage:pct=48.3 -->');
});
