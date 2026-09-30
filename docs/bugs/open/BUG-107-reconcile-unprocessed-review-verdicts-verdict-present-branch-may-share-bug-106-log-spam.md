# BUG-106: `reconcileUnprocessedReviewVerdicts`'s Verdict-Present Detection Log May Share BUG-105's Spam Shape for FAIL/PARTIAL/UNVERIFIABLE Outcomes [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-11, while fixing BUG-105 (`docs/bugs/fixed/BUG-105-reconcile-unprocessed-review-verdicts-detection-log-spam-while-bouncing-blocked.md`)

## Problem Description

BUG-105 fixed `reconcileUnprocessedReviewVerdicts`'s no-verdict detection log
(`session/backlog_lifecycle_review.go`, the `latest.Edges.ReviewVerdict == nil`
branch) to check `RemediationBlocked(bouncing)` before logging, since nothing
transitions a review-stage item out of "review" while the "bouncing"
remediation gate is blocked/parked, so the sweep re-matches the same dead
session and re-logs forever.

The sibling branch — `latest.Edges.ReviewVerdict != nil`'s "has an unprocessed
%s verdict — applying it now" log — was **not** touched, but appears
structurally identical for one sub-case: when the recorded `OverallOutcome` is
FAIL, PARTIAL, or UNVERIFIABLE, `handleReviewSessionExited`'s switch
(`session/backlog_lifecycle_review.go:147-149`) routes straight to
`autoReopenWithBackoffGate` with no notify wrapped around it — the exact same
gate BUG-105/BUG-046 already established can leave an item parked in "review"
indefinitely with nothing ever transitioning it out. If that reasoning holds,
this sweep's own detection log would repeat on every tick for such an item too.

This is **not confirmed** the way BUG-105 was: no live occurrence has been
observed for this specific branch/outcome combination, and PASS-outcome items
are unaffected (this sweep's caller always sets `forcePush=true`, so a PASS
verdict ships and leaves "review" regardless — no loop). Filed rather than
fixed speculatively, per this repo's evidence-over-inference convention — see
BUG-105's fix notes for why this wasn't folded into that PR.

## Suggested Fix Approach

If confirmed (either via live evidence or a constructed reproduction test
mirroring BUG-105's), apply the same `RemediationBlocked(bouncing)` guard to
this branch, gated additionally on the verdict's `OverallOutcome` being FAIL,
PARTIAL, or UNVERIFIABLE (not PASS — a PASS verdict is a genuinely new action
being taken, shipping the item, not a repeat of a prior no-op).

Whoever picks this up should also grep `session/backlog_lifecycle_review.go`'s
`reconcileUnprocessedReviewVerdicts`/`handleReviewSessionExited` pair for any
other ungated `log.WarningLog()` call before concluding the class is closed —
per BUG-105's reflection note, this is now the second sibling-branch instance
of the same "gate applied at the call site a bug happened to surface, not
audited across every structurally-identical call site" shape.

## Related

- `docs/bugs/fixed/BUG-105-reconcile-unprocessed-review-verdicts-detection-log-spam-while-bouncing-blocked.md` — the confirmed, fixed sibling case
- `docs/bugs/fixed/BUG-046-unprocessed-review-verdict-sweep-reprocesses-same-dead-session-every-tick.md` — original `RemediationBlocked`-as-dedup pattern
