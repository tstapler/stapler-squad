# BUG-060: `reconcileBouncingItems` Discards the Actual Review Verdict, Always Saying "no PASS verdict" [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-05, during the Phase D recurring-shape check for BUG-059 (`docs/bugs/fixed/BUG-059-orphaned-triage-stuck-context-discards-classified-end-reason.md`) — the same "reconciler reads a richer upstream signal but reduces it to a boolean before the operator ever sees it" shape, found in a sibling reconcile function in the same file.
**Impact**: Low. Cosmetic/diagnostic only — does not affect detection correctness (the `bouncing` stuck reason still fires at the right threshold), only the operator-facing message's usefulness.

## Problem Description

`reconcileBouncingItems` (`session/backlog_lifecycle.go`, ~line 3108) calls:

```go
outcome, verdictErr := er.GetMostRecentReviewVerdictForItem(ctx, item.ID)
...
hasPass := outcome == ReviewOutcomePass
if !isBouncing(count, hasPass) {
    continue
}
```

`outcome` is a `ReviewOutcome` — the actual most recent review verdict (e.g. `Fail`,
`Partial`, `Unverifiable`, in addition to `Pass`) — but it is immediately reduced to a
single `hasPass bool` and discarded. Both the `MarkStuck` `reasonDetail` (line ~3193,
`"bounced in_progress<->review %d times in the last %s with no PASS verdict"`) and the
operator notification body (line ~3213, "...with no PASS verdict. It may be stuck in a
non-converging rework loop.") say only "no PASS verdict" — never which non-passing
verdict actually kept recurring across the bounce cycles.

An operator seeing "Item is thrashing between work and review" has no way to tell, from
the stuck-state context alone, whether the item is bouncing because review keeps
returning `FAIL` (a real defect the work session isn't fixing), `PARTIAL` (incremental
progress that keeps falling short), or `UNVERIFIABLE` (a review-tooling problem, not a
code problem) — three very different next actions for a human to take.

## Why This Is the Same Shape as BUG-059

BUG-059 (`reconcileOrphanedTriageItems`) had `EndReason` already loaded on the struct in
hand but never interpolated into the message. This is structurally identical:
`GetMostRecentReviewVerdictForItem` is already called, its result is already in a local
variable in scope at the `reasonDetail`/notify call sites a few lines down — it's simply
narrowed to a boolean before it gets there.

## Suggested Fix (not done here — separate design decision from BUG-059)

Unlike BUG-059's fix (a straight passthrough of one string field), this one has a real
design question to resolve first: a bounce can span several review cycles, potentially
with *different* non-passing verdicts across them (e.g. `PARTIAL` then `FAIL` then
`PARTIAL` again) — "the most recent verdict" is one reasonable choice but not
necessarily the most useful one (a verdict *histogram* across the lookback window might
be more actionable). Whoever picks this up should decide:

1. Surface just the most recent non-passing `outcome` value (simplest, matches BUG-059's
   pattern exactly), or
2. Surface a short summary of the verdict distribution across the bounce window (more
   information, more implementation work — would need a new repository query beyond
   `GetMostRecentReviewVerdictForItem`).

Either way, once this and BUG-059 are both closed, consider extracting a small shared
helper (e.g. `reasonDetailWithClassification(base, classification string) string`) used
at every `MarkStuck` call site in this file that has a categorized upstream signal
available, per BUG-059's Phase D note — two confirmed instances of the same shape in one
file is enough to justify it.

## Related

- `docs/bugs/fixed/BUG-059-orphaned-triage-stuck-context-discards-classified-end-reason.md` —
  the sibling bug this was found while investigating, and the origin of the "diagnostic
  data captured upstream but the reasonDetail/context string ignores it" recurring-shape
  name.
