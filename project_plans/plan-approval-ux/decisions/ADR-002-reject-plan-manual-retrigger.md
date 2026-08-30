# ADR-002: RejectPlan Persists State Only — Regeneration Stays a Separate, Explicit Action

**Status**: Accepted
**Date**: 2026-08-01
**Context**: Plan Approval UX feature — architecture.md §3 and ux.md §2 pull in
different directions on whether rejecting a plan should automatically kick off
a new `TriggerTriage(feedback)` run or just record the rejection for the user
to act on separately. The task brief requires an explicit, justified choice.

## Decision

`RejectPlan(item_id, reason)` **only persists** rejection state
(`plan_rejection_reason`, `plan_rejected_at`) — it does not itself invoke
`TriggerTriage` or any LLM call. It returns immediately, like `ApprovePlan`.

To satisfy the "feedback should be actionable, not archival" mental model
(ux.md §2), the frontend closes the gap with a **one-click, visually distinct
follow-up action**: once an item is in the `changes_requested` state,
`PlanVerdictBox` renders a secondary button — "Regenerate Plan with This
Feedback" — that calls the existing, already-shipped, already-tested
`triggerTriage(item.id, item.planRejectionReason)` RPC
(`useBacklogService.ts:744-757`). This is a second, explicit button, not a
side effect of the reject submission itself — matching ux.md's own fallback
guidance: *"If research finds this needs to be a separate explicit action...
the UI must make the two steps... visually distinct — e.g. two buttons, not
one."*

## Alternatives Considered

1. **Auto-invoke `TriggerTriage(feedback: reason)` synchronously inside the
   `RejectPlan` handler** (architecture.md §3, option (ii)). Rejected as the
   *default* for this pass: `TriggerTriage`'s real work happens in a
   goroutine gated by `triageInFlight` (an in-memory `sync.Map` TOCTOU guard,
   `backlog_service_triage.go:1884-1893`), a concurrency semaphore
   (`triageSem`, 8 slots), a `triageCallBudget`-scoped context, and an
   orphan-session tombstoning pass (`tombstoneOrphanTriageSessions`) — all of
   which run *before* the actual LLM call. Folding `RejectPlan` into this
   path means either (a) duplicating that whole precondition/guard sequence
   inside a second handler (drift risk — two copies of orphan/in-flight
   logic silently diverging over time), or (b) extracting it into a shared
   internal function first, which is a real refactor of `TriggerTriage`'s
   existing, working, tested handler — out of proportion to what "Request
   Changes" needs to deliver for v1. Architecture.md itself flags this
   tradeoff and recommends the manual path as lower-risk.
2. **Auto-invoke, but only as a "fire and forget" call from the frontend**
   (i.e., the client calls `rejectPlan` then immediately calls `triggerTriage`
   itself, no user click in between). Rejected: this reintroduces exactly the
   "nothing visibly happened, but a 7-15 minute LLM call is now running"
   confusion ux.md warns against, and removes the user's ability to reject a
   plan *without* immediately spending an LLM call (e.g. rejecting several
   items' plans in a batch review pass, then triggering regeneration
   separately) — a workflow that will keep working with this pass, but not
   with the fully-automatic variants of options 1/2.

## Consequences

- `RejectPlan`'s handler is structurally simple and mirrors `ApprovePlan`
  almost exactly (same guard shape, same `session.BacklogItemUpdate`
  partial-update call) — no new async/goroutine/in-flight-guard machinery to
  write, test, or maintain.
- The regeneration path a rejected plan takes is **the same, already-tested**
  `TriggerTriage` code path a fresh or manually-refined triage uses — no new
  prompt-building or LLM-invocation logic is introduced by this feature at
  all.
- A user can reject a plan and walk away without immediately spending an LLM
  call; the reason is durably stored (per ADR-001) and the "Regenerate with
  This Feedback" CTA remains available on next visit, not just in the moment
  right after rejection.
- **Tradeoff accepted**: rejecting a plan takes two clicks to actually see a
  new plan (Reject → Regenerate), not one. This is the deliberate,
  documented cost of avoiding a `RejectPlan`/`TriggerTriage` code-path merge
  in this pass — revisit as a follow-up refactor (extracting `TriggerTriage`'s
  pre-goroutine guard sequence into a shared internal helper both RPCs could
  call) only if user friction from the two-click flow is confirmed in
  practice, not preemptively.
