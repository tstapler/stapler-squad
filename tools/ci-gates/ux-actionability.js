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
  // analyze.ts writes findings_count=-1 before doing any real work, then
  // overwrites it with the real count on successful completion — so a
  // negative value here means the analysis started but crashed partway
  // through (not "found nothing"). An *absent* value (empty string, when
  // the whole Claude-analysis step was conditionally skipped because no
  // API key is configured) stays 0, deliberately not actionable — that's
  // an expected, permanent state for repos without the key, not a failure.
  const count = findingsCount === undefined || findingsCount === '' ? 0 : Number(findingsCount);
  const findingsCountUnknown = count < 0;
  return (
    axeOutcome !== 'success' ||
    lighthouseParseFailed ||
    score < LIGHTHOUSE_THRESHOLD ||
    findingsCountUnknown ||
    count > 0
  );
}

module.exports = { isActionable, LIGHTHOUSE_THRESHOLD };
