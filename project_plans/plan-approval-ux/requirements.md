# Requirements: Plan Approval UX

**Status**: Draft | **Phase**: 1 — Ideation complete (non-interactive; generated from backlog item)
**Created**: 2026-08-01
**Source**: Backlog item `54d154dd-ec80-4782-b350-6642b89390e4` — "Plan approval ux sucks"

## Problem Statement

The backlog item's SDD pipeline mode produces a plan (via `sdd:3-plan` / triage) that a
user is meant to review and approve before implementation proceeds. The approval
mechanism exists in the backend (`planApproved` field, `ApprovePlanRequest`/
`ApprovePlanResponse` RPC, `StuckReasonPlanNotApproved` gate in
`DequeueNextQueuedItems`) but the UX around it is incomplete in three specific ways
confirmed by reading the current implementation:

1. **Approval state is hard to notice.** `ActionsSection.tsx` only shows an "Approve
   Plan" button when `item.planArtifactsPath && !item.planApproved`; once approved, the
   button disappears with no persistent "Approved" indicator elsewhere in the item
   detail view. There is no timeline/history entry recording who/when approved it
   beyond whatever generic status event plumbing already exists.

2. **The gate barely gates anything.** `planApprovalStaleness` / `StuckReasonPlanNotApproved`
   in `session/backlog_lifecycle.go` only blocks `DequeueNextQueuedItems` — i.e., it
   only matters for items sitting in the `queued` status. An item that is not going
   through the queue (or where `skipPlanning` is true) proceeds regardless of
   `planApproved`. This makes the field feel decorative rather than a real gate, which
   matches the reporter's confusion about "why it exists separately."

3. **No reject-with-reason.** `ApprovePlanRequest` (proto `backlog.proto`) takes only
   `item_id` — there is no `RejectPlan`/`RequestPlanChanges` RPC and no field for
   feedback text. Contrast with `TriggerTriageRequest.feedback`, which already supports
   "refine this triage result" — that pattern exists for triage but was never extended
   to plan review specifically.

4. **No way to visualize or annotate the plan.** `PlanArtifactsSection.tsx` renders only
   the plan artifacts *path* as inert text behind a collapsible header — it does not
   render the plan document's content, does not support inline/line-by-line comments,
   and provides no affordance to send structured feedback back to the AI pipeline.

**Who has this problem**: The solo developer (Tyler) driving backlog items through the
SDD pipeline mode, who currently cannot tell at a glance whether a plan was approved,
cannot block low-quality plans outside the queued-item path, and cannot give the
AI targeted feedback on a specific part of a plan without leaving the app (e.g. editing
the markdown file by hand or writing a prose comment that loses line context).

## Success Criteria (initial — refined by research/plan phases)

1. **Visible approval state**: The backlog item detail view shows an unambiguous,
   persistent indicator of plan approval status (not just a button that vanishes) —
   distinguishing "no plan yet", "plan pending review", "plan approved", and (new)
   "changes requested".
2. **Consistent gate**: Plan-approval-required behavior is either applied uniformly
   across all paths that spawn implementation work (not just the queued-item dequeue
   path), or the scope is intentionally narrowed and documented so the gate's purpose
   is unambiguous to the user.
3. **Reject / request-changes with reason**: A user can decline a plan and supply
   free-text (and ideally line-referenced) feedback that is surfaced to the next
   triage/plan run, mirroring the existing `TriggerTriageRequest.feedback` refinement
   pattern.
4. **Plan content is viewable in-app**: The plan document(s) produced by the SDD
   pipeline (`plan.md`, `requirements.md`, etc.) render as formatted content in the
   backlog item detail view, not just a filesystem path.
5. **Line-level feedback capability**: The user has some mechanism — inline comments,
   selected-text annotations, or an equivalent — to attach feedback to a specific part
   of the plan rather than only a single global comment box.

## Scope

### Must Have (MoSCoW)
- Persistent, glanceable plan-approval status indicator in the backlog item detail UI
- A reject/"request changes" action distinct from silent inaction, carrying a reason
  that flows into the next triage/plan iteration
- In-app rendering of the plan markdown content (not just the path)
- Research phase to determine feasibility/design for line-by-line or section-level
  feedback (may land as a follow-up if scope is large — plan phase will size this)

### Should Have
- Approval/rejection history visible in the item's status/progress timeline
- Making the plan-approval gate apply consistently regardless of queued vs. direct path
  (or an explicit documented decision not to)

### Out of Scope
- Building a full generic rich-text/CRDT collaborative editor
- Changing the underlying SDD skill phase files themselves (`sdd:2-research` /
  `sdd:3-plan` / `sdd:4-validate`) beyond what's needed to consume structured feedback
- Multi-user / concurrent-reviewer approval workflows (this is a solo-developer tool)

## Constraints

- **Tech stack**: Go backend (ConnectRPC + ent ORM), React/Next.js frontend
  (vanilla-extract CSS per `.claude/rules/css-architecture.md`) — no new dependencies
  unless justified in research (e.g. a markdown renderer, if one isn't already a
  dependency)
- **Existing infrastructure to build on**: `planApproved` field (`session/ent/schema/backlog_item.go`),
  `ApprovePlanRequest`/`Response` RPC (`proto/session/v1/backlog.proto`),
  `PlanArtifactsSection.tsx`, `ActionsSection.tsx`, `TriggerTriageRequest.feedback`
  refinement pattern, `StuckReasonPlanNotApproved` gate (`session/backlog_lifecycle.go`)
- **Registries**: any new RPC or frontend feature must update
  `docs/registry/features/` per `.claude/rules/feature-registry.md`
- **Backward compatibility**: existing `planApproved`/`ApprovePlan` callers must keep
  working; this is additive UX, not a breaking schema change unless research finds
  otherwise

## Context

### Existing Work (confirmed by direct code inspection, not assumption)
- `session/domain/backlog.go`: `ErrPlanRequired`, `ErrPlanArtifactsRequired`,
  `PlanArtifactsPath` field
- `session/backlog_lifecycle.go`: `planApprovalStaleness = 5 * time.Minute`,
  `StuckReasonPlanNotApproved`, used only in `DequeueNextQueuedItems`
- `proto/session/v1/backlog.proto`: `ApprovePlanRequest { string item_id }`,
  `ApprovePlanResponse { BacklogItem item }`, and separately
  `TriggerTriageRequest { string item_id; string feedback }` — the feedback-refinement
  pattern already exists one level up (triage) but not at plan-approval level
- `web-app/src/components/backlog/detail/ActionsSection.tsx`: conditionally renders
  "Approve Plan" button (`data-testid="backlog-action-approve-plan"`) only pre-approval
- `web-app/src/components/backlog/detail/PlanArtifactsSection.tsx`: collapsible section
  showing only `planArtifactsPath` as text, empty render when path is absent
- `tests/e2e/plan-gate.spec.ts`: existing e2e coverage of the current (narrow) gate
  behavior — must not regress

### Stakeholders
- **Primary**: Tyler (solo developer), sole user/reviewer of backlog-driven plans

## Open Questions — resolved by Phase 2 research

- **Reset on regeneration?** Resolved: architecture.md found a genuine bug — `TriggerTriage(feedback)`
  regeneration never clears `PlanApproved`/`PlanApprovedAt` today. Fix: reuse the same event to
  reset approval state, and model `RejectPlan` as triggering the existing `TriggerTriage` feedback
  path (mirrors the precedent, per ux.md and architecture.md) rather than being purely archival.
- **Which artifacts render in-app?** Resolved: `plan.md` is the primary target (existing
  `PlanArtifactsSection.tsx` already anchors on it; `GetFileContent` can't serve it since it's
  scoped to a live session workspace, so a new RPC is needed — see architecture.md/stack.md).
  Other artifacts (`requirements.md`, `research/*.md`) are the same rendering path and can reuse
  the new RPC/component if scope allows, but are not required for v1.
- **Line-level feedback required for v1?** Resolved: not required. ux.md and build-vs-buy.md agree
  it's a precision enhancement, not what makes the gate trustworthy — first thing to cut if plan
  phase needs to reduce scope. Custom heading/paragraph-anchor mechanism (not a new library) is
  the build approach if pursued.

## Research Dimensions Needed
- [ ] **Stack** — What markdown-rendering capability already exists in `web-app`
  (check for an existing renderer used elsewhere, e.g. session output or notes
  rendering) before adding a new dependency
- [ ] **Features** — How similar tools (GitHub PR review, Notion comments, other
  AI-pipeline review UIs) implement line-level/section feedback, scaled down for a
  solo-developer single-file markdown review case
- [ ] **Architecture** — Where does rejection feedback need to flow to reach the next
  `sdd:2-research`/`sdd:3-plan` invocation? Does this reuse `TriggerTriageRequest.feedback`
  or need a new field/RPC? How does the review-queue/stuck-item machinery
  (`StuckReasonPlanNotApproved`) need to change if the gate scope changes?
  Cross-reference `.claude/rules/session-creation-registry.md`-style touchpoint mapping
  if new RPC fields or session states are introduced.
- [ ] **Pitfalls** — Race between plan regeneration (new triage feedback run) and a
  stale rendered plan already open in the UI; what happens to line-referenced comments
  when the plan document is regenerated and line numbers shift; interaction with
  `skipPlanning` and `skipReviewGate` flags
