'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { videoAnomaly } = require('./video-anomaly.js');

test('videoAnomaly_should_ReturnTrue_When_ZeroArtifactsProduced', () => {
  assert.equal(videoAnomaly([]), true);
});

test('videoAnomaly_should_ReturnFalse_When_ArtifactsProduced', () => {
  assert.equal(videoAnomaly([{ name: 'e2e-videos-pr1-shard1' }]), false);
});

test('videoAnomaly_should_ReturnFalse_When_OnlyPartialShardsProduced', () => {
  // Documents the accepted Concern: a partial failure (1 of 2 shards) is not
  // flagged as an anomaly by this gate — see the module's doc comment.
  assert.equal(videoAnomaly([{ name: 'shard1' }]), false);
});
