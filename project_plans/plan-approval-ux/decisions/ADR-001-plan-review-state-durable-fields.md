# ADR-001: Plan-Review State Modeled as Durable Fields, Not a Status-Event or Enum

**Status**: Accepted
**Date**: 2026-08-01
**Context**: Plan Approval UX feature — how to persist "changes requested" state
(and the reason) alongside the existing `plan_approved`/`plan_approved_at` bool
pair, and how a 4-5-state UI status indicator (no plan / pending review /
approved / changes requested / skipped) is derived.

## Decision

Add two new nullable fields to `BacklogItem`, following the exact convention
already used for `plan_approved`/`plan_approved_at`
(`session/ent/schema/backlog_item.go:55-59`):

```go
field.String("plan_rejection_reason").
    Optional().
    Comment("Free-text reason from the most recent RejectPlan call. Cleared on ApprovePlan, on the next TriggerTriage completion (fresh or feedback-driven), and on any backward transition to idea/refining — mirrors the existing plan_approved reset convention."),
field.Time("plan_rejected_at").
    Optional().
    Nillable(),
```

`RejectPlan` sets both fields. `ApprovePlan` and the `TriggerTriage` async
completion write both clear `plan_rejection_reason` (`""`) — clearing the
timestamp is optional/best-effort, matching the existing precedent that
`plan_approved_at` itself is never explicitly cleared when `plan_approved`
flips back to `false` (`backlog_service_lifecycle.go:596-601`).

The 5-state UI indicator (`no_plan` / `pending_review` / `approved` /
`changes_requested` / `skipped`) is a **pure derivation**, computed from
existing/new fields with no new persisted state field of its own:

```
skipped           ⟺ item.skipPlanning === true
changes_requested ⟺ !skipped && item.planRejectionReason !== ""
approved          ⟺ !skipped && !changes_requested && item.planApproved === true
pending_review    ⟺ !skipped && !changes_requested && !approved && item.planArtifactsPath !== ""
no_plan           ⟺ none of the above
```

Precedence matters: `skipped` is checked first (a categorically different
meaning than "no plan yet" per pitfalls.md §6), then `changes_requested`
(since clearing `plan_rejection_reason` on approve/regenerate means its mere
presence is sufficient — no timestamp-ordering comparison against
`plan_approved_at` is ever needed).

## Alternatives Considered

1. **Reuse `BacklogStatusEvent` (append-only status-transition log) as the
   history/reason record** (architecture.md §6b, "Design 1: ephemeral
   reason"). Rejected: `BacklogStatusEvent` rows represent a real
   `from_status → to_status` transition (`recordStatusEvent`,
   `session/ent_repository_backlog.go:45`), gated by the `validTransitions`
   whitelist (`session/backlog.go:194-196`) inside
   `TransitionBacklogItemStatus`. A plan rejection does not change the
   item's status (it stays in `ready`/`queued`) — recording it as a
   same-status pseudo-transition (`ready → ready`) would either require
   bypassing the transition FSM entirely (calling the internal
   `recordStatusEvent` helper directly, outside `TransitionBacklogItemStatus`)
   or extending `validTransitions` with self-loops that don't represent real
   state changes. Either option breaks the implicit invariant every other
   caller of `status_events` relies on — that `from_status != to_status`
   always — and risks corrupting "time in status" style queries that assume
   each row is a genuine transition. A full per-event history table
   (mirroring `BacklogProgressNote`) was also considered and rejected for v1
   as disproportionate to a "Should Have" requirement (see plan.md's Unresolved
   Questions — deferred, not designed away).

2. **A single `plan_status` enum-as-string field** (`"none"|"pending"|
   "approved"|"changes_requested"`), replacing `plan_approved` outright.
   Rejected: `plan_approved bool` is a public field on `ApprovePlanResponse`
   and consumed directly by the spawn-gate boolean checks at
   `backlog_service_triage.go:438,656` (`!item.PlanApproved`). Replacing it
   with a string enum is a breaking change to every existing caller
   (violates the requirements.md constraint: *"existing `planApproved`/
   `ApprovePlan` callers must keep working"*) and would require touching the
   gate-check call sites as a forced side effect of an unrelated UX feature.
   The derived-status approach gets the same 5-state UI without touching the
   gate's own boolean contract at all.

3. **A `changes_requested bool` field paired with a nullable timestamp**,
   structurally identical to `plan_approved`/`plan_approved_at` but without a
   reason string (reason lives elsewhere, e.g. appended to `notes`). Rejected:
   this is strictly worse than the chosen design for the same schema cost — a
   `string` field doubles as both the boolean signal (`"" ` = not rejected,
   `text` = rejected) and the reason storage, so no extra boolean column is
   needed at all.

## Consequences

- Two new nullable columns, additive only — no migration of existing rows,
  no change to `ApprovePlanRequest`'s wire shape (`item_id` only, unchanged).
- `RejectPlan`, `ApprovePlan`, the `TriggerTriage` completion write, and the
  existing backward-transition reset block
  (`backlog_service_lifecycle.go:595-606`) all become the four write sites
  that must agree on clearing `plan_rejection_reason` — enumerated explicitly
  in plan.md's Epic 1/2/3 tasks so none is missed.
- The derivation function (`derivePlanReviewStatus`) is the single source of
  truth for the 5-state model — both `PlanVerdictBox` (status card) and
  `ActionsSection` (button visibility) call the same helper rather than each
  re-deriving state from raw fields, avoiding drift between the two surfaces.
- **Deferred, not solved**: a full audit trail of *every* past rejection
  (not just the most recent) is out of scope for this pass — `plan_rejection_reason`
  is last-write-wins, same as `plan_approved`/`plan_approved_at` already are.
  If multi-entry history becomes a hard requirement later, it is an additive
  follow-up (a `BacklogPlanReviewEvent` entity modeled on `BacklogProgressNote`),
  not a rework of this decision.

## Risks

- If a future feature needs to distinguish "rejected, never resubmitted" from
  "rejected, then a *different* unrelated update cleared the reason
  accidentally," the last-write-wins design can't answer that — mitigated by
  scoping this explicitly as a known, accepted limitation (see plan.md
  Unresolved Questions) rather than an oversight.
