# ADR-002: Idle Items Are Removed From the Review Queue, Not Visually Collapsed

**Status**: Accepted
**Date**: 2026-09-04
**Project**: notification-revamp

## Context

Requirements.md's Open Question 3 asks whether "idle, ready for next task"
should be removed from the review queue entirely or kept as a lowest-priority,
visually-collapsed tier, "leaning toward removal... but confirm during UX
design (Phase 3) rather than presupposing here." Requirements.md's own Scope
item 6 already states the intended shape rather more concretely: "idle-only
'ready for next task' sessions move to a status chip on the Sessions list
rather than occupying review-queue slots" — this decision formalizes and
confirms that reading against Phase 2 research and Phase 3 design.

Two independent lines of research converge on the same answer:

- **architecture.md §5**: `Priority`/`RiskLevel` filtering, sorting, and
  badges are already fully shipped in `ReviewQueuePanel.tsx` (the
  `review-queue-severity` project, merged #411). Idle items are already
  hardcoded `PriorityLow`. The only missing piece is *sectioning* by
  priority tier — a frontend layout decision, not a data-availability gap.
- **ux.md**: maps this directly onto GitHub's Participating/Watching split
  — idle "ready for next task" is not an attention item at all, "much like
  GitHub doesn't put your own passive Watching activity above @mentions."
- **build-vs-buy.md / SubStatusChip**: `web-app/src/components/sessions/SubStatusChip.tsx`
  already has a fully-built `SubStatus.IDLE` case ("● Idle", correct
  `aria-label`), and `SessionRow.tsx:352` already explicitly suppresses it
  from the Sessions list today — the chip this ADR asks for already exists
  and is one boolean term away from showing.

## Decision

**Idle items are filtered out of the Review Queue's rendered list and every
count derived from the same array, entirely on the frontend.** The backend
`Determine()` function (`session/review_queue_determiner.go`) is
**unchanged** — it continues to produce `ReasonIdle` `ReviewItem`s exactly as
it does today, including the idle ack-suppression fix from Epic 1.2. The
frontend applies one new predicate, `isReviewQueueVisible` (`reason !==
"idle"`), once, at the point `useReviewQueue.ts` exposes `items` to every
consumer — so `ReviewQueuePanel`, `ReviewQueueNavBadge`, and any other reader
of the same array agree by construction.

The existing `SubStatusChip`'s `SubStatus.IDLE` case is un-suppressed in
`SessionRow.tsx`, giving the Sessions list its chip with zero new component
work.

## Alternatives Considered

- **Remove `ReasonIdle` from `Determine()` entirely (backend change).**
  Rejected: `Determine()` is a shared pure function used by
  `ReviewQueuePoller.checkSession` *and* `StartupScanner.Scan`
  (per its own doc comment, `review_queue_determiner.go:104-106,320-321`).
  Changing what it emits risks regressing `TestReviewQueue*`
  (explicit Constraint) for a change the frontend-only approach achieves
  with strictly less risk and the same user-visible outcome.
- **Keep idle in the queue as a lowest, visually-collapsed tier** (the
  literal alternative Open Question 3 posed). Rejected: requirements.md's
  own Scope item 6 text already commits to "rather than occupying
  review-queue slots" — a collapsed-but-present tier still occupies a slot,
  just a quieter one, and doesn't match the GitHub-analogy reasoning in
  ux.md as cleanly as full removal from this specific list.
- **Build a new "session status" component for the Sessions-list chip.**
  Rejected: `SubStatusChip.tsx` already renders the correct chip with the
  correct `aria-label`; the only reason it doesn't show today is one
  explicit filter term in `SessionRow.tsx`.

## Consequences

- The review-queue nav badge count (`ReviewQueueNavBadge.tsx`) drops from
  counting all reasons to counting all-but-idle — this is the intended
  fix for the "107 review-queue items" trust-eroding raw count named in
  requirements.md's Problem Statement, not a side effect to work around.
- Any *future* consumer of `useReviewQueue()`'s `items` automatically
  inherits the idle-exclusion by construction (single filter point), rather
  than needing to remember to apply it — directly avoiding the
  count-vs-list mismatch bug class `StuckItemsSection`'s own code comment
  warns about (features.md §1).
- `session/review_queue_determiner.go`'s idle ack-suppression fix (Epic 1.2)
  remains meaningful even after this ADR: `Determine()` is still called by
  `StartupScanner.Scan` and any other consumer that may care about the raw
  idle signal independent of what the Review Queue *page* chooses to
  display; the ack-suppression correctness fix and the frontend display
  decision are orthogonal and both land.
