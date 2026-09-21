# ADR-003: Reconciliation Audit Trail Is a Metadata Stamp on the Original Notification Record, Not a New Record Type

**Status**: Accepted
**Date**: 2026-09-04
**Project**: notification-revamp

## Context

Requirements.md mandates a visible, non-silent "Auto-resolved by rule:
`<name>`" record whenever rule-reconciliation auto-resolves a previously
pending approval (Observability Requirements; per house memory
`feedback_document_ai_decisions_in_edge_cases`, AI-driven state changes must
never be silent). Two design questions need resolving:

1. **What record carries this audit trail** — a new record type, or an
   annotation on something that already exists?
2. **How does the record survive the referenced rule being edited or
   deleted later** — a live reference or a snapshot?

`features.md` cites this repo's own precedent directly on point:
`backlog-session-lifecycle-ux`'s ADR-001 (`RespawnEvent`) chose "a loose
string reference, not a hard FK" specifically so a historical audit record
survives the referenced entity's deletion — the decision that established
"scoped, historical audit rows can't be a live pointer to a mutable entity"
as the working precedent in this codebase.

Separately, `architecture.md` and `pitfalls.md` both independently observed
that `ApprovalService.ResolveApproval` already stamps
`metadata["approval_decision"]` on the *original* notification record
(the `APPROVAL_NEEDED`-type record, whose ID equals the approval's ID by the
existing convention documented in `ApprovalService.ResolveApproval`'s own
comment) as a side effect of every resolution, live or otherwise — and the
Notifications page's `AutoHandledSection` already filters purely on
`notificationType === "auto_approved"`.

## Decision

1. **No new notification record type or notification-type enum value.**
   Reconciliation resolves through `ApprovalService.ResolveApprovalReconciled`,
   which calls the existing `ResolveApproval` path (so the original
   `APPROVAL_NEEDED` record already gets its normal `approval_decision`
   stamp, `MarkRead`, and event-bus broadcast for free), then adds exactly
   two new metadata keys to that same record: `classifier_rule_name` (the
   rule's display name, captured **as a string value at resolution time** —
   not a rule ID lookup performed at render time) and `reconciled: "true"`.
2. **The audit trail is a snapshot, not a live reference.** `classifier_rule_name`
   is copied verbatim from `ClassificationResult.RuleName` at the moment of
   resolution. If the rule is later renamed or deleted, this string is
   unaffected — matching `RespawnEvent`'s "loose reference, not a hard FK"
   precedent exactly, for the identical reason: the decision already
   happened and the session already proceeded, so there is no live behavior
   to keep in sync, only a historical label to keep readable.
3. **The frontend routes reconciled records to `AutoHandledSection` via
   metadata, not notification type**: `metadata.reconciled === "true"`
   is added as an alternate inclusion condition alongside the existing
   `notificationType === "auto_approved"` check, and as an alternate
   *exclusion* condition on the main/informational feed. One record serves
   both the race-condition lookup (`GetByID` in `ResolveApproval`'s
   not-found branch, Task 2.1.1c) and the Auto-handled display — no
   duplicate record is written for the same event.
4. **Structured log line at resolution time, independent of the metadata
   stamp**: `log.Info` with `rule_id`, `rule_name`, `approval_id`,
   `session_id`, `before_decision: "escalate"`, `after_decision`, per the
   Observability Requirements — this is the durable, greppable audit trail;
   the metadata stamp is the UI-visible one.

## Alternatives Considered

- **A new `RECONCILED` notification type / a separate `AppendReconciledApproval`
  record**, mirroring `AppendAutoApproved`'s shape. Rejected: it would
  duplicate data for the same event (the original escalation record already
  exists and is already correctly threaded through `AutoHandledSection`'s
  existing type filter once that filter also checks the new metadata key),
  and it would give the mid-review-race fix (Epic 2.3) nothing to look up by
  approval ID — the whole point of reusing the original record is that its
  ID *is* the approval ID by existing convention.
- **A live FK/rule-ID lookup at render time** (fetch the current rule by
  `RuleID` when displaying the audit note). Rejected per the `RespawnEvent`
  precedent: a deleted rule would render as blank/"unknown rule," and a
  reused rule ID (however unlikely) would render actively wrong information
  for a decision that already happened and cannot be undone by editing the
  rule further.
- **Route reconciliation events through `AnalyticsStore.RecordFromResult`
  as a new "via" dimension** (features.md's "unstated needs" observation
  that a future rule-impact view will want this) as this project's audit
  mechanism. Deferred, not rejected outright: this is a real, cheap-to-add
  future enhancement (tag `via: "reconciliation"` vs `via: "live"` in the
  existing analytics call), but it's a *query surface* for later, not the
  *audit-trail requirement* this ADR resolves now — building it prematurely
  here would be scope creep the Rabbit Holes section warns against for a
  requirement that's already satisfied by the metadata stamp + structured
  log above.

## Consequences

- Zero new persistence surface, zero new proto message — a pure metadata
  addition to an existing record shape (`NotificationRecord.Metadata
  map[string]string` already exists).
- `AutoHandledSection`'s rendering code needs one small addition
  (distinguish "auto-decided live" from "auto-resolved by rule after the
  fact" in copy — Task 2.3.2b) but no new prop shape.
- If a future "show me everything this rule has ever auto-resolved" view is
  built (features.md's flagged unstated need), it can query
  `NotificationHistoryStore` for `metadata.reconciled == "true" &&
  metadata.classifier_rule_name == X` without any schema change made here.
