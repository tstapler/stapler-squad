# Bug: Orphaned-triage stuck-state context discards the classified EndReason, always showing a generic message

**Status**: Fixed
**Priority**: Medium
**Fixed in**: this branch (2026-08-05)

## Symptoms

Every `orphaned_triage` stuck-state notification and its `backlog_stuck_states.context`
column read the identical generic string regardless of what actually killed the
underlying triage session — `timeout`, `process_error`, `claude_not_found`, or anything
else all rendered as the same "triage session %s ended without moving the item out of
idea" message. An operator (or any future automated remediation keyed off failure
category) had no way to tell a transient LLM refusal apart from a missing `claude`
binary from the stuck-item view alone.

Confirmed live in the running instance's DB: a real `orphaned_triage` row for item
`cfb91f0e-1a1a-473e-9530-be1214a47c87` had
`context = "triage session headless-triage-9f13d8b9-9d32-428e-b8bc-e24a34cfcda3 ended
without moving the item out of idea"` — no hint of *why* it ended, even though the
underlying `ItemSession.end_reason` column already had a classified value.

## Root Cause

`TriggerTriage` (`server/services/backlog_service_triage.go`) already classifies every
failed headless triage call via `classifyHeadlessCallError` (line ~2055) into one of
`timeout` / `shutdown` / `process_error` / `claude_not_found` / `other`, and persists it
onto `ItemSession.EndReason` via `UpdateItemSessionEndedWithReason` (line ~2288).

`reconcileOrphanedTriageItems` (`session/backlog_lifecycle.go`, ~line 2536) already
*reads* that same `EndReason` field — it uses it to detect the `"shutdown"` carve-out a
few lines above the bug (the graceful-shutdown respawn branch). But the two "already
ended" shapes below that carve-out (shape 2: ended while still in `idea`; shape 3: ended
while `queued` with no usable plan) built their `reasonDetail` strings with a hardcoded
template that never referenced `latestTriage.EndReason` at all — a pure data-plumbing
gap, not a missing classification. The data was already loaded into the same
`ItemSessionSummary` struct the shutdown check reads from; it was simply never
interpolated into the two other message templates a few lines down.

## Fix

**`session/backlog_lifecycle.go`**:
- Added `triageEndReasonOrUnknown(endReason string) string`, a small helper that
  returns the persisted `EndReason` verbatim, or the literal `"unknown"` when it's empty
  (a session ended via the plain `UpdateItemSessionEnded` path with no
  `classifyHeadlessCallError` bucket ever recorded — e.g. a legacy row).
- Shape 2's `reasonDetail` now reads `"triage session %s ended (%s) without moving the
  item out of idea"`, interpolating `triageEndReasonOrUnknown(latestTriage.EndReason)`.
- Shape 3's `reasonDetail` gets the identical treatment: `"triage session %s ended (%s)
  with no usable plan while item was gated on plan approval (status=%s)"`.

No new query was needed — `EndReason` was already present on the `ItemSessionSummary`
this function already had in hand.

## Regression Tests

`session/backlog_lifecycle_stuck_test.go`:
- `TestReconcileOrphanedTriageItems_should_surfaceEndReasonInContext_When_TriageSessionEndedWithClassifiedError` —
  ends a triage session with `EndReason = "process_error"` via
  `UpdateItemSessionEndedWithReason`, then asserts the resulting
  `backlog_stuck_states.context` contains `"process_error"` and does **not** contain the
  old generic-only phrase verbatim.
- `TestReconcileOrphanedTriageItems_should_fallBackToUnknown_When_EndReasonNeverClassified` —
  ends a session via the plain `UpdateItemSessionEnded` (no `EndReason` ever recorded)
  and asserts the context falls back to the literal `"unknown"` rather than an empty
  parenthetical.

Both pass; full `go test ./session ./server/services` suite (all pre-existing tests,
including every other `reconcileOrphanedTriageItems` shape) remains green, and `make
build && make lint` are clean.

## Phase D — Reflect

**Classification**: Integration Gap — two internal call sites within the same
reconciliation pass (the shutdown check and the reasonDetail builders a few lines
below it) read from the same struct, but only one of them was wired up to use the field
it already had. The upstream classification logic (`classifyHeadlessCallError`) and its
persistence (`UpdateItemSessionEndedWithReason`) were both already correct; the gap was
purely in the surface-facing string construction never consuming data that was already
in scope.

**Earliest enforcement point**: A unit test asserting on the `context` string content
(the regression test added above) is close to the earliest achievable level — this is a
string-formatting omission, not a type-level or compile-time-checkable defect (Go's type
system has no way to require "every field on this struct must appear in some output
string"). No lint rule would generically flag "a field read three lines up isn't reused
three lines down." The test is the right level.

**Recurring shape identified — "diagnostic data captured upstream but the
reasonDetail/context string ignores it before reaching the operator."** This is the
*second* confirmed instance of this shape in `session/backlog_lifecycle.go`, not the
first:

- **`reconcileOrphanedTriageItems`** (this bug): reads `latestTriage.EndReason` for the
  shutdown carve-out, but never threaded it into shapes 2/3's `reasonDetail` — fixed
  here.
- **`reconcileBouncingItems`** (~line 3108): calls
  `er.GetMostRecentReviewVerdictForItem(ctx, item.ID)`, which returns the actual most
  recent `ReviewOutcome` (e.g. `Fail`/`Partial`/`Unverifiable`), but only ever reduces it
  to a `hasPass bool` gate. Both the `MarkStuck` `reasonDetail`
  (`"bounced in_progress<->review %d times in the last %s with no PASS verdict"`, line
  ~3193) and the operator-facing notification body (line ~3213) say only "no PASS
  verdict" — never *which* non-passing verdict actually kept recurring. **Not fixed in
  this PR** — it is a distinct call site with its own message-format tradeoffs (a bounce
  can span several review cycles with different verdicts each time, so "the most recent
  verdict" needs its own design decision about which value to surface and how), and
  fixing it would expand this PR's blast radius beyond the EndReason-threading fix this
  bug was scoped to. Filed as `docs/bugs/open/BUG-060-bouncing-items-reason-detail-discards-review-verdict.md`
  so it isn't silently re-discovered as new.

Checked but **not** additional instances of this shape: `reconcileStaleWorkSessions` and
the zombie-session branch of `reconcileStuckReviewItems` only ever read from
still-open (`EndedAt == nil`) sessions, so there is no "ended" diagnostic field to
discard yet. `reconcileOrphanedAgentPRs` and `markPRReadyUnmerged`'s reasonDetail
strings are built from data with no richer upstream classification to lose (a boolean
merged/PR-ready state, not a categorized failure). `autonomous_stuck` (set in
`server/services/autonomous_orchestration_service.go`) already interpolates
`outcome.Reason` into its `MarkStuck` call — it does **not** have this gap.

Given two confirmed instances in the same file with the same shape, a shared helper
(e.g. a `reasonDetailWithClassification(base string, classification string) string`
used at every `MarkStuck` call site that has a categorized upstream signal available)
would close this class rather than requiring a third bug report before someone
proposes it — worth doing the next time either this function or
`reconcileBouncingItems` is touched, rather than as a standalone refactor PR with no
adjacent behavior change to justify it.
