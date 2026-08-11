# Requirements: Review Queue Severity Levels (P0/P1/P2)

Source: backlog item `8bd3f70e-5fe2-49e9-8a39-1968d4842598`, migrated from
`TylerStaplerAtFanatics/stapler-squad#40` (created 2026-04-09).

## Problem

The manual approval review queue (pending hook approvals surfaced in
`ReviewQueuePanel.tsx`, backed by `PendingApproval` / `ListPendingApprovals`)
treats every escalated item as equal priority. As session count grows, a
low-risk test-file edit sits in the same FIFO position as a high-risk
`rm -rf` or force-push request, forcing humans to open every item to find
the dangerous one.

## Pre-implementation code audit (what already exists)

This is materially less greenfield than the original issue implies — most of
"sources of severity" already exist upstream in the classifier and are simply
not threaded through to the live queue, its wire format, or the UI:

1. **`pkg/classifier.RiskLevel`** (`RiskLow`/`RiskMedium`/`RiskHigh`/`RiskCritical`)
   already exists and is populated on every `ClassificationResult`, including
   pattern-derived seed rules matching the issue's own examples (`rm`, `git
   push --force` → `RiskCritical`). See `pkg/classifier/classifier.go:16-24`
   and the seed rule table (`RiskCritical`/`RiskHigh` entries ~line 760-990).
2. **`classifier.Rule.RiskLevel`** is already persisted per approval rule via
   the ent `ApprovalRule` schema and exposed over the wire on
   `ApprovalRuleProto.risk_level` (`proto/session/v1/types.proto:1084`) and
   `SuggestedRuleProto.risk_level` (`:1455`) — the rules-management UI can
   already show/edit risk per rule.
3. **`ClassificationAnalytics`** (ent schema, written via
   `AnalyticsStore.RecordFromResult`) already stores `RiskLevel` per recorded
   decision.
4. **The gap**: `server/services/approval_handler.go`'s `createApproval`
   path (~line 384-437) computes `escalation classifier.ClassificationResult`
   (which has `.RiskLevel`) but only copies `EscalationReason` and
   `EscalationCategory` onto the `PendingApproval` struct
   (`server/services/approval_store.go:20-45`) — `RiskLevel` is dropped on
   the floor before it ever reaches the live queue item.
5. `PendingApprovalProto` (`proto/session/v1/types.proto:1034-1060`, backing
   `ListPendingApprovalsResponse`) has no risk/severity field at all — even
   if the Go struct carried it, the wire type can't carry it to the browser.
6. **Correction (found during Phase 2 research, see `research/features.md` and
   `research/architecture.md`)**: there are **two independent frontend
   consumers**, not one, and neither has a severity badge/sort/filter today:
   - `ApprovalCard.tsx` / `ApprovalDrawer.tsx` read `PendingApprovalProto`
     directly via `ListPendingApprovals` (`web-app/src/lib/approvalsApi.ts`).
   - `ReviewQueuePanel.tsx` never touches that proto — it reads flattened
     string keys off `ReviewItem.Metadata`, populated by the enrichment
     block in `session/review_queue_poller.go:820-862` (the same place
     `escalation_reason`/`escalation_reason_category` are set today from
     `ApprovalStore.GetApprovalMetadataBySession`). This is the panel most
     users actually triage from day-to-day.
   Both paths currently drop `RiskLevel` and must be threaded independently.
   Also, `ApprovalRulesPanel.tsx` already receives `riskLevel` per rule but
   never renders it (`web-app/src/components/sessions/ApprovalRulesPanel.tsx:253`)
   — a smaller, related gap worth closing in the same pass.
7. `GetApprovalAnalyticsResponse` (`proto/session/v1/session.proto:1429`) has
   no risk-level breakdown — only a daily bucket summary.
8. Separately, `session/queue/queue.go` has its own, unrelated `Priority`
   enum (`PriorityUrgent`/`High`/`Medium`/`Low`) for session-attention
   `ReviewItem`s (idle/error/approval-pending/etc — the *session* triage
   queue). This is a distinct concept from per-*approval-request* risk and
   must not be confused or merged with it; the two priority notions may
   eventually want a shared visual vocabulary but are computed from
   different inputs (session state vs. classified tool-use risk).

Net effect: rule-derived and pattern-derived severity computation is a
**wire-through + UI** problem, not a new classification engine. Agent-
self-reported severity (source #3 in the original issue) has no existing
analog and is new work.

## In Scope

1. Map `classifier.RiskLevel` (Low/Medium/High/Critical) to the issue's
   requested P0/P1/P2 scale for display purposes (4→3 level collapse
   decision to be made in planning — see open question below).
2. Thread `RiskLevel` from `classifier.ClassificationResult` through
   `PendingApproval` (Go), `PendingApprovalProto` (proto + regen), and the
   `ListPendingApprovals` RPC response.
3. Severity badge on each `ReviewQueuePanel` item, colour-coded.
4. Default sort by severity (highest risk first) within the queue.
5. Filter control to show/hide by severity level.
6. Approval analytics breakdown by severity — extend
   `GetApprovalAnalyticsResponse` (backed by existing per-decision
   `ClassificationAnalytics.RiskLevel` data, no new storage needed) and
   render it in the analytics UI.
7. Persist `RiskLevel` on `PersistedApproval` (disk JSON) so severity
   survives a server restart for orphaned approvals — currently
   `EscalationReason`/`EscalationCategory` are persisted but `RiskLevel`
   would need the same treatment once added.

## Out of Scope (for this pass — flag as follow-up if research disagrees)

- Agent-reported severity via a structured self-report format — no existing
  hook/tool contract for an agent to declare its own risk level; needs its
  own design (new tool_input field? separate MCP call?) and is the one
  genuinely new capability the original issue asks for. Included as a
  research question, not assumed in-scope for the initial implementation.
- Changing `session/queue/queue.go`'s session-attention `Priority` enum —
  out of scope; that queue answers "which session needs a human," not "how
  risky is this specific approval request."
- Automatic escalation routing to a specific person/role (Gastown's
  Deacon/Mayor pattern) — no equivalent concept (roles/assignees) exists in
  this codebase; not building it here.

## Acceptance Criteria (draft — refined further in plan.md / validate)

1. Every `PendingApproval` created via the escalation path carries the
   classifier's `RiskLevel` at creation time (rule-derived and
   pattern-derived sources already flow through the classifier — no new
   pattern-matching logic needed).
2. `ListPendingApprovals` RPC response includes severity for each item.
3. `ReviewQueuePanel` renders a colour-coded severity badge per item and
   sorts the queue by severity by default (highest risk first, ties broken
   by existing created_at/age ordering).
4. User can filter the review queue by severity level.
5. `GetApprovalAnalyticsResponse` includes an approval-count breakdown by
   severity level, rendered in the analytics UI.
6. Severity survives a server restart (orphaned/persisted approvals retain
   their severity).
7. Existing approval flow (approve/deny, expiry, auto-approval, secret scan
   auto-deny) is unaffected — this is additive metadata, not a behavior
   change to the classifier's Decision output.
8. **(Added post-validate — consistency-check CONCERN)** `ApprovalCard`/
   `ApprovalDrawer` (Path A, the other live consumer of `PendingApprovalProto`
   alongside `ReviewQueuePanel`) render a severity badge and sort pending
   approvals severity-first, mirroring AC3 for Path B — plan.md's Epic 5 was
   already building this; this AC formalizes it as in-scope rather than
   leaving it implied only by the narrative gap analysis above.
9. **(Added post-validate — consistency-check CONCERN)** `ApprovalRulesPanel`
   renders the already-stored `riskLevel` per rule (a pre-existing gap: the
   field was already wired through `upsertRule` but never displayed) —
   plan.md's Epic 7 was already building this; formalized here as in-scope.

## Success Metric and Risky Assumption (added post-triad — PM lens)

- **Success metric**: median time-to-first-action on Critical-severity queue items should
  decrease relative to today's chronological-order baseline (no current instrumentation
  captures this — add a log-timestamp delta between item-created and first
  approve/deny/dismiss, bucketed by severity, as a lightweight follow-up; not blocking this
  feature's initial ship since the UX acceptance criteria already give a testable proxy:
  "highest-severity item visible in 1 glance, 0 clicks").
- **Named risky assumption**: `classifier.RiskLevel` (rule-derived + pattern-derived) is
  trustworthy enough to drive *human attention ordering*, not just rule-editing display and
  analytics counts (its only two uses today). If the seed rule table under- or
  over-classifies common commands, severity-first sort could bury a genuinely dangerous but
  unmatched action below a falsely-Critical one — mitigated by, but not eliminated by, the
  "unrecorded sorts as High" fail-safe (ADR-001), which only covers *un*classified items, not
  *mis*classified ones. Flagged, not resolved, in this pass — misclassification-rate tuning is
  the existing classifier's problem, unchanged by this feature.

## Open Questions for Research/Planning — resolved in Phase 2 research

- **Resolved**: keep the existing 4-level `classifier.RiskLevel`
  (Low/Medium/High/Critical) as the canonical scale; do not remap to P0/P1/P2
  (which inverts direction — P0 = most severe vs. `RiskLevel`'s ascending
  iota — and is lossy). Label UI badges with the plain RiskLevel words,
  consistent with GitHub Dependabot's 4-tier convention and this repo's own
  already-wired-but-unrendered `ApprovalRuleProto.risk_level` field. See
  `research/ux.md` and `research/build-vs-buy.md`.
- **Still open, deferred out of initial scope**: agent-self-reported
  severity has no existing analog in this codebase and needs its own design
  (new tool_input convention or separate report path). Classifier-side risk
  already covers the issue's stated examples (`rm`, force push), so this is
  not blocking. Track as a follow-up item, not part of this implementation.
- **Resolved with caution flagged**: severity sort should NOT be a hard
  primary key by default — `research/pitfalls.md` found `ReviewQueuePanel.tsx`
  already has a "snapshot-on-enter" pattern that stabilizes set membership
  but not sort order; a hard severity-first sort risks reordering rows a
  user is mid-click on. Plan should default to severity as a secondary sort
  key (existing age/expiry ordering preserved as primary) or gate re-sorting
  to explicit user action/new-item boundaries, not every render.
