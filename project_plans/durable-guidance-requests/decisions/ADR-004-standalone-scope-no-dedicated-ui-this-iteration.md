# ADR-004: Standalone-Scoped Requests Get No Dedicated UI Surface This Iteration

**Status**: Accepted (Auto Mode — no open questions)
**Date**: 2026-09-11

## Context

Requirements open question 5, and `research/features.md` §4's "genuinely net-new
for this codebase" finding: a guidance request with neither a backlog item nor a
session — a "standalone" scope — has no existing UI-surface precedent anywhere in
this codebase. Every durable, ent-backed construct found
(`BacklogActivityNote`, `BacklogProgressNote`, `ApprovalRule`, `SessionGoal`) is
anchored to something with its own detail view. The three UI surfaces named in the
requirement (backlog item detail, triage panel, session view) are all themselves
anchored to a backlog item or session — none of them can render a standalone
request.

## Decision

Standalone-scoped `GuidanceRequest`s are fully supported in the backend (Phase 1:
`item_id` and `session_uuid` are both optional/nullable fields on the ent schema;
`CreateGuidanceRequest` accepts neither being set; MCP tools and the RPC layer
place no requirement on either). They are **not** rendered in a dedicated new UI
view in this iteration. Instead, a standalone request's creation and answer both
flow through the existing durable notification pipeline
(`NotificationHistoryStore`, reusing `NOTIFICATION_TYPE_INPUT_REQUIRED` —
`research/pitfalls.md` §2) exactly like item/session-scoped requests, so a human
is still durably notified and can still answer via the RPC/MCP surface even
though no bespoke "standalone guidance inbox" page exists yet. A global
"guidance inbox" view (listing all pending requests regardless of scope,
independent of any item/session context) is named explicitly as deferred
follow-on work, not silently dropped.

## Reasoning

- **No existing anchor to build from.** Building a fourth, wholly new UI surface
  (a "guidance inbox") is materially more design and implementation work than
  wiring a form into the three already-existing detail views this feature must
  touch anyway, and the requirements' "at least three" UI-surface bar is already
  met by item/session/triage without it.
- **Every consumer the requirements actually name is scoped.** Automated triage
  is item-scoped by construction; the autonomous driver's asks are
  session-scoped by construction. Standalone is the requirement's "or standalone"
  clause covering a hypothetical future caller, not a named current one — the
  right amount of investment now is "don't block it in the data model," not
  "build a UI for a caller that doesn't exist yet."
- **The durable notification pipeline already reaches a human without a bespoke
  view.** `NOTIFICATION_TYPE_INPUT_REQUIRED` notifications are visible via
  existing notification UI regardless of scope, and `GetNotificationHistory`
  already guarantees these survive a restart (`research/pitfalls.md` §2) — so
  "durably notify" is satisfied for standalone requests even without a new page.
  A human can still answer through `AnswerGuidanceRequest`/`answer_guidance_request`
  (Phase 1) even with no bespoke form.

## Consequences

**Positive**: no fourth UI surface to design, build, and test in this iteration;
the ent schema/proto/RPC layer is still fully general (nothing about the backend
design special-cases scope), so a follow-on "guidance inbox" view is pure
frontend work reading already-shipped backend state — not a backend redesign.

**Negative**: until a follow-on inbox view ships, a standalone request is only
answerable through a raw RPC call, `curl`/`grpcurl`, or the MCP
`answer_guidance_request` tool — not click-through-able in the web UI. This is an
explicit, named gap, not a silent one.

## Alternatives Considered

**Build a minimal global guidance-inbox view now.** Rejected for this iteration:
no named consumer creates standalone requests yet (both integration points in
scope — triage, autonomous driver — are inherently scoped), so the added
surface area has no concrete use case to validate against, and requirements.md
explicitly permits deferring scope that "Phase 2/3 research finds" isn't needed
yet. Tracked as explicit follow-on work in this plan's closing notes.
