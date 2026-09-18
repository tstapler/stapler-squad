'use strict';

// Marker owns the previous percentage itself (`pct=NN.N`) rather than
// scraping it out of the free-form summary sentence — a future edit to that
// sentence's wording can't silently break parsing this way. Lookup uses the
// broader, pre-`:pct=` prefix (not the full marker, which varies run to run
// since it embeds the *current* run's pct) so it still matches the OLD
// bare `<!-- feature-coverage -->` marker this format replaced — otherwise
// a PR with an already-posted old-format comment gets a permanent
// duplicate on its first post-deploy run, since findExisting would never
// see it and isActionable would treat it as "no prior state".
const LOOKUP_PREFIX = '<!-- feature-coverage';
const MARKER_PREFIX = '<!-- feature-coverage:pct=';
const MARKER_RE = /feature-coverage:pct=([\d.]+)/;

function buildMarker(currentPct) {
  return `${MARKER_PREFIX}${currentPct} -->`;
}

function findExisting(comments) {
  return comments.find((c) => c.body.includes(LOOKUP_PREFIX));
}

function previousCoveragePct(existingBody) {
  if (!existingBody) return null;
  const m = existingBody.match(MARKER_RE);
  return m ? parseFloat(m[1]) : null;
}

// An unchanged coverage number is the steady state, not a resolved problem
// the way a cleared regression is — never actionable, and never deleted
// either, unlike the other 3 workflows' gates (see build.yml's comment step).
function isActionable({ existingBody, currentPct }) {
  if (!existingBody) return true;
  const prev = previousCoveragePct(existingBody);
  if (prev === null) return true; // malformed/missing marker — safe fallback: post
  if (isNaN(currentPct)) return true;
  return currentPct !== prev;
}

// Parses the percentage out of feature-coverage.ts's own summary line, e.g.
// "Feature E2E coverage: 21/50 tested (42%)". Kept alongside previousCoveragePct
// since both parse a percentage out of loosely-structured text — this one
// from the tool's stdout, that one from a prior sticky-comment body.
function parseCoveragePct(summaryLine) {
  if (!summaryLine) return null;
  const m = summaryLine.match(/\(([\d.]+)%\)/);
  return m ? parseFloat(m[1]) : null;
}

module.exports = {
  MARKER_PREFIX,
  buildMarker,
  findExisting,
  previousCoveragePct,
  parseCoveragePct,
  isActionable,
};
