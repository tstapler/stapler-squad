'use strict';

// True only when zero video artifacts were produced at all — the "expected
// videos but got none" anomaly this workflow's comment exists to flag.
// NOTE (adversarial-review.md Concern, accepted as-is per AC #5's literal
// "zero artifacts produced" wording): a partial-shard failure (e.g. 1 of 2
// expected shards uploads) is not treated as an anomaly here, so it no
// longer gets a comment the way it did before this gate — a real but
// lower-severity visibility regression, left as a documented follow-up
// rather than widening this check to `< expectedShardCount`.
function videoAnomaly(artifacts) {
  return artifacts.length === 0;
}

module.exports = { videoAnomaly };
