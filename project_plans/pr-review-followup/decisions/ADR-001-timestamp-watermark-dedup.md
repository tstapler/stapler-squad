# ADR-001: Per-Item Timestamp Watermark for PR-Feedback Dedup

**Status**: Accepted
**Date**: 2026-08-02
**Project**: pr-review-followup

## Context

`ReconcilePRPending` (`session/backlog_lifecycle.go:3850`) needs a way to detect
"substantive `COMMENTED`-state review or plain PR comment feedback that hasn't been
addressed by a fix session yet" without re-spawning a fix session on every ~60s tick
for feedback that was already responded to. GitHub does not clear `COMMENTED`
reviews or plain comments on push the way it can clear `CHANGES_REQUESTED` reviews
(via the "dismiss stale approvals" branch-protection setting) — so, unlike the three
existing triggers (`CIFailing`, `HasBlockingReviews`, `HasConflicts`), this signal is
not self-clearing. Some durable, content-aware dedup marker is required.

Two research passes on this project disagreed on the shape of that marker:

- **build-vs-buy.md §3** and **pitfalls.md §1** independently argued for an
  **ID-based** dedup key: track the specific GitHub review/comment IDs a fix session
  has already responded to, reasoning that (a) comparing this app's local clock
  against GitHub's introduces clock-skew risk, and (b) a human amending/re-requesting
  an already-actioned review doesn't produce a new ID, so ID-based tracking correctly
  distinguishes "same feedback, re-read" from "genuinely new feedback."
- **architecture.md §1c** argued for a **timestamp watermark**: a single nullable
  `time.Time` column, reasoning that an item has at most one open PR at a time, so a
  scalar high-water mark is sufficient and avoids the complexity of persisting and
  pruning an ID set.

requirements.md's Open Questions section explicitly flagged this as a contradiction
this plan must resolve, not silently pick a side on.

## Decision

**Timestamp watermark.** Add a single nullable `time.Time` field,
`PrFeedbackAddressedAt`, to `BacklogItem` (ent schema: `pr_feedback_addressed_at`).
`ReconcilePRPending` computes:

```go
hasNewFeedback := prStatus.HasReviewFeedback &&
    (item.PrFeedbackAddressedAt == nil || prStatus.LatestFeedbackAt.After(*item.PrFeedbackAddressedAt))
```

where `LatestFeedbackAt` is the newest `submittedAt`/`createdAt` across all
substantive `COMMENTED` reviews and plain comments captured by `GetPRStatus`'s
existing single `gh pr view` call. The watermark advances to `LatestFeedbackAt` only
once `remediatePRFixWithBackoffGate` confirms a fix was actually dispatched
(`attempted=true, fixErr=nil`).

## Why the Clock-Skew Objection Doesn't Apply Here

build-vs-buy.md's clock-skew concern is a real risk for a *general* pattern: "compare
a timestamp this app records locally against a timestamp a remote system assigned."
That comparison mixes two clocks and is exactly the kind of thing that breaks under
NTP drift, container clock weirdness, or a slow-starting reconciler tick.

That is not what this comparison does. Both sides of `.After()` here —
`prStatus.LatestFeedbackAt` and the stored `item.PrFeedbackAddressedAt` — are
**GitHub-assigned** timestamps: `LatestFeedbackAt` is parsed directly from GitHub's
own `submittedAt`/`createdAt` fields, and `PrFeedbackAddressedAt` is *set* from a
prior tick's `LatestFeedbackAt` (never from `time.Now()`). This app's wall clock
never enters the comparison. Two timestamps issued by the same clock (GitHub's) are
monotonically ordered relative to each other by construction — the clock-skew
scenario the ID-based argument was defending against (this app's clock vs. GitHub's
clock) simply isn't present in this design.

## Why "At Most One Open PR Per Item" Makes a Scalar Sufficient

The ID-based approach's core value is handling *multiple independent things that
need independent dedup state* — e.g., N items each with their own PR, or one item
across multiple concurrent PRs. Neither applies here: `item.PrNumber`/`item.PrURL`
are wholesale-replaced (not appended-to) whenever a PR closes without merging
(`backlog_lifecycle.go:3985-4044`), so at any instant there is exactly one PR whose
feedback timeline this watermark needs to track. "Has a fix been dispatched covering
everything up to timestamp T" is a single comparison against a single scalar; an ID
set would need to check set membership per review/comment, prune entries for a PR
that's since closed, and handle "does this ID exist across a PR replacement"
edge cases the domain doesn't actually produce.

## Consequences

- **Coarser grain, accepted**: if 10 comments land in one tick and a fix session
  addresses only some of them, the watermark still advances past all 10 (they all
  have a `createdAt` at or before `LatestFeedbackAt` at dispatch time). A stray
  unaddressed comment among a batch won't independently re-trigger. This is a known,
  accepted limitation (see plan.md's Unresolved Questions) — the mitigating factor is
  that `fixCtx`/`FeedbackText` includes every comment's full body in the single fix
  session's context, so a competent fix session addresses the batch together, not
  piecemeal.
- **No unbounded growth**: a single nullable column, same operational shape as
  `shipped_snapshot_at`/`plan_approved_at`/`queued_at` already on this schema — no
  pruning job, no JSON blob to encode/decode.
- **Reopening a resolved-but-parked stuck row inherits its prior attempt count**: if
  `MarkStuck` reopens a previously-resolved `pr_needs_fix` row for genuinely new
  feedback, and that row had *previously* parked (5 attempts exhausted on an
  unrelated CI flap) before resolving, the reopened row has no automated retries
  left. This is a pre-existing property of the shared-reason/row design (see
  architecture.md §3), not something this ADR's watermark choice introduces or
  worsens — noted here for completeness, not as a consequence of choosing timestamp
  over ID-based dedup.

## Alternatives Rejected

See plan.md's "Step 0.5 — Alternatives Considered" and Pattern Decisions table for
the full three-way comparison (timestamp watermark vs. ID-based set vs. GraphQL
`reviewThreads.isResolved` suppression). Summary: ID-based dedup was rejected because
its two motivating problems (clock skew, multi-PR-per-item collisions) don't exist in
this domain; GraphQL-based suppression was rejected as a heavier, partial-coverage
mechanism (roughly doubles API call volume, and top-level comments have no
`isResolved` concept at all) reserved as a documented future lever if the
timestamp-watermark's coarser grain proves insufficient in practice.
