# Requirements: plan-feedback

**Date**: 2026-09-14
**Type**: feature addition (existing product — backlog item lifecycle)
**Complexity**: 2 — focused feature, Small/Medium appetite

## Problem Statement

Once a backlog item's session has moved past the triage/planning stage —
`in_progress`, `review`, `pr_pending`, or `done` — there is no way for the
operator to send it back for re-planning while explaining *what* should
change about the plan.

Concrete, code-verified gaps (not hypothetical):

1. **The one reachable "send back" affordance is a bare transition with zero
   feedback capture.** `BacklogItemDetail.tsx`'s `send_back_ready` case
   (rendered as "↩ Back to Ready" in `ActionsSection.tsx`, visible from
   `in_progress`/`review`/`pr_pending`/`done` per `itemActions.ts`'s
   `CAN_SEND_BACK_READY`) calls `transitionStatus(item.id, "ready")` with no
   reason/feedback argument at all.
2. **`send_back_refining` is dead code.** `BacklogItemDetail.tsx` has a
   `case "send_back_refining"` and a toast message ("Sent back to
   refining.") for it, but no button anywhere adds `"send_back_refining"`
   to the actions set (`itemActions.ts` defines no
   `CAN_SEND_BACK_REFINING`) — it can never fire.
3. **Transitioning to `refining` today destroys context instead of carrying
   it.** `TransitionBacklogItemStatus` (`backlog_service_lifecycle.go:811-827`)
   resets `PlanApproved`, wipes `PlanArtifactsPath` to `""`, and clears
   `PlanRejectionReason` to `""` whenever the target is `idea` or `refining`
   — there is no field this handler writes the operator's new feedback
   into. `TransitionBacklogItemStatus` also accepts an `overrideReason`
   param, but that only becomes an audit-trail progress note
   (`backlog_service_lifecycle.go:794-800`) — it is never threaded into any
   field a subsequent triage run reads.
4. **`TriggerTriage`'s status guard requires `idea` or `ready`**
   (`backlog_service_trigger_triage.go:183-187`) — it explicitly rejects
   `refining` as a starting status. So even a UI that captured feedback and
   set status to `refining` could not immediately kick off a retriage from
   there without either changing that guard or routing through a different
   status.
5. **The one place feedback *does* reach a retriage today only covers the
   pre-implementation case.** `RejectPlan` + the "Regenerate Plan with This
   Feedback" button (`PlanVerdictBox.tsx`, ADR-002 in
   `project_plans/plan-approval-ux/`) lets an operator reject an
   *unapproved* plan with a reason and separately trigger
   `triggerTriage(id, reason)` — but this flow only applies while the item
   sits at `ready` with `PlanApproved=false`. It does nothing for an item
   that already started work, is in review, or already shipped and needs
   to be redone differently.

## Baseline (what users do today without this)

An operator who watches an in-progress or already-reviewed session go the
wrong direction has no in-view way to say why. They must either: retype
feedback into an unrelated box elsewhere in the UI (the same "retype into a
generic box" complaint `backlog-operator-feedback-loop`/PR #457 already
fixed once, for a different surface), manually re-trigger triage via MCP
tools outside the web UI, or abandon the item's progress and start a new
one — losing whatever the original session already established.

## Users / Consumers

Same as `backlog-operator-feedback-loop`'s established framing: a single
operator running their own `stapler-squad` instance, reviewing their own
backlog items. Not a multi-operator review tool.

## Success Metrics

From the backlog item detail view of an item in `in_progress`, `review`,
`pr_pending`, or `done`, an operator can type free-text feedback describing
what the next triage/plan pass should change, submit once, and that
feedback is verifiably used by the resulting triage/plan run (visible in
the new `triage_result`/plan diff) — without leaving the item detail view
or retyping into an unrelated box. The dead `send_back_refining` code path
is either wired up correctly or removed.

## Appetite

Small (1–2 days). This reuses existing, already-tested machinery
(`TriggerTriage`'s `feedback` parameter, its `BuildHeadlessChatRetriagePrompt`
/`BuildHeadlessRetriagePrompt` prior-result-aware prompt builders, and the
`transitionStatus`/`RejectPlan` RPC patterns) rather than introducing new
backend primitives — see Rabbit Holes for the one design question that
could push this to Medium.

## Constraints

- Must reuse the existing `TriggerTriage(item_id, feedback)` retriage path
  — no new triage/LLM-invocation code path, per the precedent set by
  ADR-002 (`project_plans/plan-approval-ux/decisions/ADR-002-reject-plan-manual-retrigger.md`).
  That ADR explicitly rejected auto-invoking triage as a side effect of a
  status-only write, for reasons (shared precondition/guard sequence,
  in-flight/orphan-session guards) that apply here too.
- Feedback text needs a length cap consistent with the existing
  `maxRejectReasonLength = 10000` (`backlog_service_lifecycle.go:892`).

## Non-functional Requirements

- **Performance SLO**: not applicable — this is a low-frequency operator
  action, not a hot path.
- **Scalability**: not applicable.
- **Security classification**: internal (single-operator tool, same
  classification as the rest of the backlog surface).
- **Data residency**: no special requirements.

## Scope

### In Scope

- A feedback text box reachable from the backlog item detail view for
  items in `in_progress`, `review`, `pr_pending`, or `done`, where the
  operator describes what the next triage/plan pass should change.
- Submitting that feedback sends the item back toward planning **and**
  delivers the feedback to the triage agent's next run — a single
  operator action, not a two-click "reject, then separately regenerate"
  flow (per the user's explicit direction — this differs from ADR-002's
  two-click precedent, which this project should note as a deliberate,
  justified deviation rather than an oversight, in its own plan/ADR).
- Fixing or removing the dead `send_back_refining` code path so it no
  longer silently does nothing.
- Ending (stopping the tmux session and marking it ended) any live
  work/review session still running against an item at the moment it's sent
  back to `ready` — so the superseded session doesn't keep running
  concurrently with the new triage/plan the send-back kicks off, and so it
  doesn't silently block the next work-session spawn
  (`hasActiveWorkSession`). This is distinct from "steering" a live session
  (see Out of Scope below); it's necessary given the orphaned-session/OOM-
  shape risk pitfalls research identified, and its origin is that research
  finding, not an originally-stated requirement — implemented via
  `stopLiveWorkSessions`, extracted from `forceResetItem`'s existing
  teardown loop (plan.md Epics 1.1-1.2).

### Out of Scope

- Steering a still-*live*, currently-running tmux session in place
  (`steer_session`) as part of this flow. This project only covers backlog
  items whose current work/review session has already produced something
  reviewable — not interrupting a session mid-flight. (If the operator
  wants that, `steer_session` already exists as a separate, already-shipped
  mechanism.)
  - **"Steering" vs. "stopping," clarified:** this exclusion is about
    *redirecting* a live session's instructions in place while it keeps
    running — that stays out of scope. It does not exclude *stopping*
    (ending) a live session as part of sending its item back: this project
    does add that, as the In Scope bullet above states, because a send-back
    supersedes whatever plan the live session was executing against, and
    leaving it running unattended is the orphaned-session/OOM-shape risk
    pitfalls research flagged — a different action from steering, aimed at
    a different problem.
- Multi-operator concurrency/locking on the send-back action (same
  single-operator assumption `backlog-operator-feedback-loop` already
  documented and deferred).
- Changing the pre-implementation `RejectPlan`/"Regenerate Plan with This
  Feedback" flow at `ready` — that flow already works and is untouched by
  this project.

## Rabbit Holes

- **Target status for "send back": `refining` vs `ready`.** `refining` is
  the status the dead code names, but transitioning there today wipes
  `PlanArtifactsPath` (losing the prior plan as retriage context) and isn't
  a valid `TriggerTriage` starting status. `ready` preserves the prior plan
  (so the retriage prompt can revise it via `priorResult`, matching "change
  the plan" rather than "start over") and is already a valid `TriggerTriage`
  precondition — no guard change needed. This is a real architecture
  decision for Phase 3, not resolved here.
- **Where the feedback text is persisted.** `PlanRejectionReason` is the
  closest existing field, but it's explicitly wiped by the idea/refining
  reset block (`backlog_service_lifecycle.go:816`) and is conceptually
  scoped to "why I rejected this plan," not "what to do differently" from a
  later lifecycle stage. Decide whether to reuse it, repurpose it, or (only
  if truly warranted) add a new field.
- **Combined one-click action vs. the ADR-002 precedent.** The user
  explicitly wants one action (type feedback, submit, done) rather than
  ADR-002's deliberate two-click separation. Confirm this is compatible with
  `TriggerTriage`'s in-flight/orphan-session/concurrency-semaphore guards
  firing synchronously in the same request, or whether it still needs to be
  two RPC calls made back-to-back from one UI submit (functionally one
  click, structurally two calls) — that's an implementation detail, not a
  UX change, as long as it resolves in one user-visible step.

## Alternatives Considered

- Auto-triggering retriage as a side effect inside
  `TransitionBacklogItemStatus` itself — rejected for the same reasons
  ADR-002 rejected it for `RejectPlan` (duplicates `TriggerTriage`'s
  precondition/guard sequence or forces a refactor of that handler, out of
  proportion to this feature).
- Reusing `steer_session` to inject feedback into the still-running
  session instead of restarting triage — rejected as out of scope; this
  project targets items whose session has already produced a reviewable
  result, not live redirection.

## Feasibility Risks

- If `TriggerTriage`'s guard needs to change to accept `refining` as a
  starting status (only if Phase 3 picks `refining` over `ready` as the
  target), that guard change needs its own scrutiny — it currently exists
  specifically to keep triage's precondition simple.
- Existing e2e tests may assert today's dead-end behavior for
  `send_back_refining` or the bare `send_back_ready` transition; these
  need updating, not just new tests added.

## Open Questions

- Confirmed by the user: target is "send back to refining" with a feedback
  box for what the triage agent should change. Phase 3 planning still needs
  to resolve the `refining`-vs-`ready` mechanism question above — the
  user's intent (revise the existing plan, not discard it) argues for
  `ready` under the hood even though the user's own words said "refining";
  this should be raised explicitly during planning, not silently decided.
