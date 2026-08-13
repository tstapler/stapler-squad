'use strict';

const LIGHTHOUSE_THRESHOLD = 70;

// isActionable is true (comment should post/update) when there's something
// the PR author needs to act on: Axe failed, Lighthouse dropped below
// threshold *or* failed to measure at all, or Claude UX analysis found
// something. A bare `score < 70` alone would silently treat a Lighthouse
// crash (score = 'unknown' -> NaN) as a passing score, since `NaN < 70` is
// `false` in JS — the explicit isNaN check below exists specifically to
// avoid that trap.
function isActionable({ axeOutcome, lighthouseScore, findingsCount }) {
  const score = parseInt(lighthouseScore, 10);
  const lighthouseParseFailed = isNaN(score);
  const count = Number(findingsCount) || 0;
  return (
    axeOutcome !== 'success' ||
    lighthouseParseFailed ||
    score < LIGHTHOUSE_THRESHOLD ||
    count > 0
  );
}

module.exports = { isActionable, LIGHTHOUSE_THRESHOLD };
