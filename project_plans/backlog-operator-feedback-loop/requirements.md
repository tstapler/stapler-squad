# Requirements: backlog-operator-feedback-loop

**Complexity: 4** (spans proto, Go backend, three React surfaces; includes a genuinely open
state-machine design question for plan review; recommended entry point is `/sdd:full` per the
originating backlog item).

Source: backlog item `b337cbb6-6758-4210-bc41-97668d248698` — "Close the operator feedback
loop: answer triage questions, request plan revisions, and steer backlog sessions in place."
This file was generated non-interactively from that item's description/ACs (no ideation
interview — no user present in this session).

## Problem statement

The backlog pipeline (idea → triage → ready → in_progress → review → done) has three review
points where a human should be able to correct the agent, but currently cannot do so without
leaving the view, retyping context by hand, or having no affordance at all:

1. **Triage clarifying questions render, then dead-end.** The triage agent can ask a question
   via a `suggestions` entry with `rationale: "question"`. The frontend renders it in a
   dedicated "Triage Questions" section, but that section is read-only — no per-question answer
   field, no "answer and re-triage" action. The operator must manually retype the answer into an
   unrelated generic "Refine triage with feedback" box.
2. **`steer_session` is unreachable from backlog item context.** The MCP tool and its UI
   (`SessionActionsOverflow`'s Steer dialog) exist and work from general session surfaces
   (`SessionRow.tsx`, `SessionCard.tsx`, `PaneHeader.tsx`), but the backlog item detail view's
   `SessionsSection.tsx` renders linked sessions as plain `<a>` links with no Steer affordance.
3. **`ApprovePlan` has no reject / request-changes counterpart.** `ApprovePlan` is one-way
   (sets `PlanApproved=true` + timestamp). There is no `RejectPlan`/`RequestChanges`/
   `PlanRevis*` RPC, handler, or UI anywhere in the codebase (verified by full-tree grep,
   zero matches).

Gaps 1 and 2 are UI-wiring problems connecting existing backend primitives. Gap 3 needs new
backend surface (RPC + status transition) in addition to UI.

## Baseline (what users do today without this)

- **Gap 1**: Operator reads the question in the Triage Questions section, then manually
  retypes/paraphrases an answer into the separate "Refine triage with feedback" textarea,
  losing the question↔answer linkage entirely.
- **Gap 2**: Operator must navigate away from the backlog item detail view to the general
  session list, locate the same session there (not guaranteed to be surfaced while headless),
  and steer it from there.
- **Gap 3**: A plan reviewer who disagrees with a plan has no structured rejection path. The
  only lever is the generic triage `feedback` field, which is scoped to triage refinement, not
  framed as plan review, and isn't offered as an action next to Approve.

## Success metric

Every acceptance criterion below (1–8) is independently demonstrable via a Playwright e2e test
per `.claude/rules/e2e-test-conventions.md`, and the new/changed RPCs and components are
registered per `.claude/rules/feature-registry.md`. Qualitatively: an operator can answer a
triage question, request plan changes, or steer a backlog-linked session, all from the item
detail view they're already reading, without retyping context by hand.

## Appetite / scope note

Originating item recommends `/sdd:full` (not `/sdd:quick`) because: (a) the plan-review
state-machine design (open question 1) and the question-persistence design (open question 2)
are real architecture decisions warranting an ADR before implementation, and (b) the work spans
proto, Go handlers, and three React surfaces — exceeds a single-context-window `/sdd:quick`
scope.

## Acceptance criteria (from item; authoritative until Phase 3/4 refine them)

1. From the Triage Questions section of a backlog item, an operator can submit an answer to a
   **specific** rendered question without retyping the question text, and that answer is
   delivered as feedback for that item's next triage run.
2. Submitting an answer to a question triggers (or explicitly queues) a re-triage of that item
   using the existing feedback-driven re-triage path — no new triage mechanism is introduced.
3. A backlog item whose plan is awaiting review presents **both** an Approve action and a
   Request Changes action in the same place.
4. Request Changes requires the reviewer to supply the requested change text, and that text
   reaches the agent responsible for revising the plan — an empty rejection is not possible.
5. Request Changes moves the item to a state distinguishable from both "plan approved" and
   "plan never reviewed," and that state is visible in the item detail view.
6. A running triage, review, or work session attached to a backlog item can be steered from the
   backlog item detail view, without navigating to the general session list.
7. Steering from the backlog item view uses the existing `steer_session` path — no parallel
   steering implementation.
8. Each of criteria 1, 3, and 6 has a Playwright e2e test in `tests/e2e/` following
   `.claude/rules/e2e-test-conventions.md`, and the new/changed RPCs and components are
   registered per `.claude/rules/feature-registry.md`.

## Out of scope

- Redesigning the triage prompt or triage agent logic — this connects existing outputs to
  existing inputs; agent decisions are unchanged.
- Multi-turn chat-style conversation UI. A single structured answer/request per interaction is
  sufficient; threaded back-and-forth is a separate product question.
- Steering affordances for all session types site-wide. Scope is backlog-linked sessions in the
  item detail view; the general session list's Steer dialog is unchanged.
- Any change to `TriggerTriageRequest.feedback` semantics or `BuildHeadlessRetriagePrompt`
  behavior — Gap 1 consumes this path as-is.
- Notification/alerting when an agent asks a question (badges, notifications) — reasonable
  follow-up, not required for this ticket.

## Known key files (from item research — verify against current HEAD during Phase 2/3)

- `proto/session/v1/backlog.proto` — `TriggerTriageRequest.feedback` (:454-457), `ApprovePlan`
  RPC (:464-468), "question" marker doc comment (:41)
- `server/services/backlog_service_triage.go:2295-2482` — `BuildHeadlessRetriagePrompt` /
  retriage handling
- `server/services/backlog_service_lifecycle.go:742-786` — `ApprovePlan` handler
  (`+api: backlog:approve-plan`)
- `server/mcp/tools_backlog.go:1921-1941` — `submit_triage_result` question convention
- `server/mcp/tools_terminal.go:138-149,627-674` — `steer_session` MCP tool
- `web-app/src/components/backlog/TriageReviewPanel.tsx:48-142` — existing feedback refine form
- `web-app/src/components/backlog/TriageDiffSection.tsx:20-21,87-97` — read-only Triage
  Questions section
- `web-app/src/components/sessions/SessionActionsOverflow.tsx` — Steer dialog (lines 58,
  120-121, 146, 158, 432-434)
- `web-app/src/components/backlog/detail/SessionsSection.tsx` — backlog item's session list
  (plain `<a>` links, no overflow menu)
- `web-app/src/components/backlog/BacklogItemDetail.tsx:578` — `ApprovePlan` UI call site
- `web-app/src/lib/hooks/useBacklogService.ts:589,822` — `triggerTriage(id, feedback)`

## Feasibility risks / rabbit holes (carried from item; Phase 2 pitfalls research to expand)

- Whether a headless, prompt-driven triage/review session reads and acts on a mid-run
  `steer_session` call the same way an interactive session does is unverified (open question 3).
- Question-answer persistence (open question 2) risks scope creep into a full Q&A state model
  if not bounded — default to stateless (current triage `suggestions` shape) unless research
  shows a cheap way to track resolved/unresolved.
- Gap 3's RPC/state-machine design (open question 1) is the primary architecture risk and the
  reason this is Complexity 4, not 2.

## Open questions (preserved verbatim from item for Phase 2/3 to resolve)

1. Should Request Changes reuse the existing `feedback` field and status machinery, or does it
   need its own RPC (`RequestPlanChanges`) and backlog status? Reuse is cheaper; a dedicated RPC
   is more honest about state and gives a real "changes requested" status. Drives most of Gap
   3's effort.
2. Should answered questions carry persistent resolved/unresolved state on the item? Adds a
   persistence surface and a re-triage reconciliation question (does a new triage run's question
   list supersede answered ones?).
3. Does steering a headless, prompt-driven triage or review session behave the same as steering
   an interactive session? Needs verifying before criterion 6 is claimed done.
4. Ship as one issue or split into sub-issues once planning starts? Recommendation from item:
   keep as one ticket through research and planning, split at the plan stage only if Gap 3's
   state-machine design is large enough to sequence separately.

## Observability requirements

Not called out as a hard requirement in the source item. Phase 3 plan should note whether
existing backlog-item audit logging/notification patterns (see `feedback_document_ai_decisions_in_edge_cases.md`
memory: "self-heal/auto-close actions should post a visible comment + notify()") extend
naturally to Request Changes and answered questions, since both are operator-authored state
changes an operator would expect to see logged.
