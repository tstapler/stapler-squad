# ADR-001: Send-Back-With-Feedback Targets `ready`, Not `refining`

**Status**: Accepted
**Date**: 2026-09-14
**Context**: `plan-feedback` requirements.md names the dead `send_back_refining`
code path (`BacklogItemDetail.tsx`'s `case "send_back_refining"` and its toast
message, never reachable because `itemActions.ts` defines no
`CAN_SEND_BACK_REFINING` set) and frames "refining vs. ready" as the one real
architecture decision this project needed to resolve before writing tasks
(requirements.md's Rabbit Holes; the user's own words said "send back to
refining," but requirements.md itself flags that the user's underlying intent —
revise the existing plan, not discard it — argues for `ready` instead, and
says so should be raised explicitly rather than silently decided).

## Decision

The send-back-with-feedback action transitions the item to **`ready`**, not
`refining`. `refining` is treated as out of scope for this feature; the dead
`send_back_refining` case and its toast entry are removed, not wired up.

## Evidence

- **`refining` is rejected outright by `TriggerTriage`'s status guard.**
  `server/services/backlog_service_trigger_triage.go:184-188`:
  ```go
  if item.Status != string(session.BacklogStatusIdea) && item.Status != string(session.BacklogStatusReady) {
      return nil, connect.NewError(connect.CodeFailedPrecondition, ...)
  }
  ```
  Targeting `refining` would require changing this guard — a change
  requirements.md's own Feasibility Risks section flags as needing its own
  scrutiny, since the guard exists specifically to keep `TriggerTriage`'s
  precondition simple. Targeting `ready` needs zero guard changes.
- **`ready` is `RejectPlan`'s exact post-condition.** `RejectPlan`
  (`server/services/backlog_service_lifecycle.go:928-934`) sets
  `PlanApproved: false` and `PlanRejectionReason: <reason>` with no status
  change — the state a `ready`-targeted send-back produces is byte-for-byte
  the same shape. That means `PlanVerdictBox`'s existing `changes_requested`
  card and "Regenerate Plan with This Feedback" button
  (`BacklogItemDetail.tsx:1629-1640`) render correctly the moment the item
  lands at `ready`, for free — including as a **failure-recovery path**: if
  the combined action's `triggerTriage` call fails after `transitionStatus`
  and `rejectPlan` already succeeded, the operator sees the same
  already-shipped "changes_requested" UI and can retry via its existing
  button, rather than facing a silent dead end with no recovery affordance.
- **`refining` would wipe `PlanArtifactsPath`, `ready` does not.** The
  idea/refining reset block (`backlog_service_lifecycle.go:813-827`) fires
  `if to == session.BacklogStatusIdea || to == session.BacklogStatusRefining`
  — not for `ready`. Under `refining`, `PlanArtifactsPath` would be blanked
  (a DB-only write — the `plan.md` file itself is never deleted anywhere in
  this codebase, confirmed by grep) until the next triage run repopulates it,
  and `ApprovePlan`'s file-on-disk precondition
  (`backlog_service_lifecycle.go:858`) would be unusable in the interim. This
  is a real, if recoverable, UX regression `ready` avoids entirely.
- **The retriage prompt's actual content input is unaffected either way.**
  `findPriorTriageResult` (`backlog_service_trigger_triage.go:731-744`) reads
  the `TriageResult` JSON blob off the most recent triage `ItemSession` row —
  not `item.PlanArtifactsPath` — so the "prior result" context a
  feedback-driven retriage revises from survives regardless of which target
  status is chosen. This means the choice is decided by guard/state-reuse
  ergonomics (above), not by a risk of losing retriage context under
  `refining` — that risk, while real for the *display* of the current plan,
  is weaker than the dead code's literal naming suggested.

## Alternatives Considered

1. **Target `refining`, matching the dead code's literal name.** Rejected:
   requires a `TriggerTriage` guard change (scrutiny risk, per Feasibility
   Risks), wipes `PlanArtifactsPath`/`PlanRejectionReason` via the existing
   idea/refining reset block, and gains no UI benefit — `refining`'s own
   `ItemActionabilityInput` case in `itemActions.ts` has "no status-specific
   primary action," so there is no rendering surface for plan/feedback state
   while an item sits there (unlike `ready`, which reuses `PlanVerdictBox`
   for free).
2. **Wire up `send_back_refining` as-is (bare transition, no feedback
   field).** Rejected outright — this is exactly the dead-end behavior
   requirements.md's success metric #2 asks to fix, not preserve.

## Consequences

- No `TriggerTriage` guard change — `ready` was already a valid starting
  status before this feature.
- The feedback text reuses the existing `PlanRejectionReason` field (see
  architecture.md §2); no new persisted field or proto message is
  introduced.
- The `review`/`pr_pending` → `ready` edge carries `ErrVerdictClearRequiredForReady`
  (`session/domain/backlog.go:644-657`) — send-back-with-feedback's
  `transitionStatus` call must always pass a non-empty `overrideReason` (the
  feedback text itself), which is harmless for `in_progress`/`done` sources
  (it becomes an audit-trail progress note,
  `backlog_service_lifecycle.go:791-800`) and required for `review`/
  `pr_pending` with a recorded PASS verdict.
- `refining` remains reachable only through its other existing entry points
  (e.g. `mark_ready`'s inverse, if any) — this feature does not add or remove
  any transition *to* `refining`; it only removes the one dead, unreachable
  code path that referenced it from `BacklogItemDetail.tsx`.
