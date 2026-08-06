# BUG-060: `reconcileBouncingItems` Discards the Actual Review Verdict, Always Saying "no PASS verdict" [SEVERITY: Low]

**Status**: Fixed
**Priority**: Medium
**Fixed in**: this branch (2026-08-05)
**Discovered**: 2026-08-05, during the Phase D recurring-shape check for BUG-059
(`docs/bugs/fixed/BUG-059-orphaned-triage-stuck-context-discards-classified-end-reason.md`)
— the same "reconciler reads a richer upstream signal but reduces it to a boolean
before the operator ever sees it" shape, found in a sibling reconcile function in the
same file. Filed as BUG-060 on branch `worktree-agent-ae30257bfec1d61a3` (PR #356,
unmerged at the time this fix was written) as
`docs/bugs/open/BUG-060-bouncing-items-reason-detail-discards-review-verdict.md`;
this doc supersedes that filing with the fix. When PR #356 merges, its copy of the
open doc at that path should be deleted rather than left alongside this one.

## Symptoms

`backlog_stuck_states.context` for a `bouncing` reason only ever contained a generic
message: `"bounced in_progress<->review 3 times in the last 24h0m0s with no PASS
verdict"`. Confirmed live in a running instance's DB
(`~/.stapler-squad/workspaces/*/sessions.db`, table `backlog_stuck_states`), item
`7a383b3b-a7f3-4a5d-9ead-3835b2cc81b7`.

An operator seeing "Item is thrashing between work and review" had no way to tell,
from the stuck-state context alone, whether the item was bouncing because review kept
returning `FAIL` (a real defect the work session isn't fixing), `PARTIAL` (incremental
progress that keeps falling short), or `UNVERIFIABLE` (a review-tooling problem, not a
code problem) — three very different next actions for a human to take — despite
`reconcileBouncingItems` having already fetched the actual verdict from the DB to
decide whether to flag the item at all.

## Root Cause

`reconcileBouncingItems` (`session/backlog_lifecycle.go`, ~line 3108) called:

```go
outcome, verdictErr := er.GetMostRecentReviewVerdictForItem(ctx, item.ID)
...
hasPass := outcome == ReviewOutcomePass
if !isBouncing(count, hasPass) {
    continue
}
```

`outcome` — the actual most recent review verdict (`Fail`, `Partial`,
`Unverifiable`, in addition to `Pass`) — was immediately reduced to a bare `hasPass
bool` and discarded. Neither the `MarkStuck` `reasonDetail` nor the operator
notification body referenced it further; both always said only "no PASS verdict."

This is the same discard-after-fetch shape **BUG-059** fixed for
`reconcileOrphanedTriageItems`: `EndReason` was already loaded on the struct in hand
but never interpolated into the message — here, `GetMostRecentReviewVerdictForItem`
is already called and its result is already in scope at the `reasonDetail`/notify
call sites a few lines down, simply narrowed to a boolean before it gets there.

## Design Decision

A bounce can span several review cycles with different non-passing verdicts across
them (e.g. `PARTIAL` then `FAIL` then `PARTIAL` again). Of the two options the
original filing raised — (1) surface just the most recent non-passing verdict, or
(2) surface a distribution across the bounce window — this fix takes **option 1**:
only the single most recent verdict (outcome + a bounded summary excerpt) is
surfaced. This matches BUG-059's precedent and keeps the addition proportional to a
diagnostic string rather than a report. If a future need arises for the full
history, `GetRecentReviewVerdictSummaries` (see Fix below) already accepts a
`limit` parameter — no schema or query change would be required to extend it.

## Fix

**`session/backlog_lifecycle.go`** (`reconcileBouncingItems`):
- Replaced the `GetMostRecentReviewVerdictForItem` call with
  `GetRecentReviewVerdictSummaries(ctx, item.ID, 1)` — an existing storage method
  (already used by `IsRepeatedFailure`/`AutoReopenAfterFailedReview` in
  `server/services/backlog_service_triage.go`) that runs the identical "most recent
  ItemSession with a verdict" query but returns the full `ReviewVerdictSummary`
  (`OverallOutcome` + `Summary`), not just the outcome. No new storage method was
  needed.
- `hasPass` is now derived from `latestOutcome` (gating behavior unchanged).
- When a verdict exists, both the `MarkStuck` context and the operator notification
  body now append `(most recent verdict: <OUTCOME> — <summary>)`, with the summary
  passed through `sanitizeField(..., 500)` — the same truncation/HTML-stripping
  convention already used for rendering `ReviewVerdict.Summary` elsewhere
  (`session/backlog_context.go:179`, `session/backlog_review.go:136`).

## Regression Tests

`session/backlog_lifecycle_stuck_test.go`:
- `TestReconcileBouncingItems_should_surfaceVerdictOutcomeAndSummaryInContext_When_BouncingWithFailedVerdict` —
  attaches a FAIL verdict with a distinctive summary to a bouncing item's review
  session, then asserts both the persisted `BacklogStuckState.Context` and the
  notification body contain the verdict's outcome and summary text, not just the
  generic bounce message.

Verified: `go test ./session ./server/services` (all green), `make build`,
`make lint` (0 issues).

## Phase D — Reflect

**Classification**: Semantic/Intent gap — the code compiled and behaved exactly as
written (compute a bool from a fetched value), but the intent behind fetching a rich
`ReviewVerdict` in the first place was to enable this decision, not to make it the
*only* use of that data before discarding it.

**Earliest enforcement point**: The regression test is the earliest achievable
level. Nothing here is catchable at compile time or via lint/static analysis — Go
permits reading one field off a richer struct and discarding the rest with no
signal, and no generic rule can flag "this fetched value's other fields were never
used" without an unacceptable false-positive rate across the codebase.

**Recurring shape / shared-helper question**: This is the **2nd confirmed
instance** of "diagnostic data is fetched but collapsed to a boolean/decision
before reaching the operator-facing string" — the 1st was **BUG-059**
(`reconcileOrphanedTriageItems` discarding the classified `EndReason`). BUG-059's
own filing note for this bug suggested that, once both are closed, a shared helper
(e.g. `reasonDetailWithClassification(base, classification string) string`) might
be justified. Having now implemented both:
- BUG-059's fix (`triageEndReasonOrUnknown`) threads a single fallback-safe string
  *inline mid-sentence* (`"ended (%s) without..."`).
- This fix appends a *two-part, conditionally-present* `outcome — summary` suffix
  at the end of the message, only when a verdict exists at all.

  These are different message shapes over different data shapes (one string vs an
  outcome/summary pair), and BUG-059's fix lives on a separate, still-unmerged
  branch (PR #356) that this change cannot edit. Extracting a shared helper now
  would produce an abstraction with effectively one real caller in this diff —
  exactly what `.claude/rules/interface-pollution-checklist.md` calls a
  speculative extraction. **Not done here, deliberately** — if a 3rd instance of
  this shape appears with a payload similar enough to unify cleanly with either of
  these two, that is the point to extract a shared helper, not before.

Any future stuck-state reconciler that fetches richer diagnostic data than a bare
gate condition needs should ask: **does the operator-facing context string
actually carry that detail, or was it thrown away after the gate check?**

## Related

- `docs/bugs/fixed/BUG-059-orphaned-triage-stuck-context-discards-classified-end-reason.md` —
  the sibling bug this was found while investigating, and the origin of the
  "diagnostic data captured upstream but the reasonDetail/context string ignores
  it" recurring-shape name.
