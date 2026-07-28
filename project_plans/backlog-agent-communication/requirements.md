# Requirements: backlog-agent-communication

**Date**: 2026-07-23
**Type**: feature addition (extends existing backlog automation pipeline)

## Problem Statement

stapler-squad's "backlog" feature drives idea→PR delivery through a chain of Claude
Code sessions (triage → plan → work → review → ship) coordinated via MCP tools
(`report_progress`, `request_review`, `submit_review_verdict`, `submit_triage_result`,
`get_backlog_item`, `set_session_goal`, `update_session_task`, and others — the full
current surface is `server/mcp/tools_backlog.go`, `tools_goal.go`, `tools_lifecycle.go`).
The stated long-term goal (`docs/tasks/backlog-feature-improvement.md`) is a fully
autonomous "software factory" that still preserves human-in-the-loop touchpoints at
every stage.

Today's tool surface is built almost entirely around a single agent reporting its own
status upward (progress, verdicts, triage results) to the orchestrator. Four
communication gaps stand between the current system and that goal:

1. Agents have no structured way to hand rich context forward to the *next* agent in
   the chain (e.g. work session → review session), or to receive structured findings
   back (e.g. a review's findings, beyond a prose verdict string).
2. Agents have no low-friction, distinct way to report that the orchestrator/tooling
   itself is broken (a stuck reconciler, a missing MCP capability, a confusing tool
   error) as opposed to "my assigned item failed." Six concrete examples of this class
   of problem were found and fixed/filed in this repo on 2026-07-23 alone
   (BUG-040 through BUG-045).
3. There is no "ask for help" escalation path for an agent that is genuinely stuck
   (not just facing a normal retryable failure) to reach the human operator or a
   central coordinating agent, distinct from the existing automatic stuck-item /
   remediation-backoff machinery (`session/backlog_remediation.go`,
   `StuckReason` in `session/domain/backlog.go`, the `/unfinished` UI).
4. Human-in-the-loop touchpoints must be preserved and strengthened at every stage of
   whatever is designed above — not centralized away into a fully automatic system
   with only a final PR-merge checkpoint.

Two concrete, already-reproduced pain points anchor this from being purely aspirational:

- **A. PR visibility loss**: PRs an agent creates sometimes don't show up as
  visible/linked on the backlog item. Related to (partially fixed) BUG-040
  (`pr_pending` item loses its PR reference) and open BUG-045 (review reads the wrong
  codebase state because the item's worktree was gone). Broader framing: the
  completion-reporting tools may not capture/surface enough structured data (PR URL,
  PR number, what changed, why) at the right points for a reviewer — human or AI — to
  reliably see it.
- **B. No verdict dispute path**: a work-session agent has no way to formally disagree
  with a review verdict it believes is wrong. A FAIL verdict just triggers
  `AutoReopenAfterFailedReview`/rework with no mechanism for the agent whose work was
  reviewed to say "I believe this verdict is incorrect, here's why" in a way a human
  will ever see. Repeated identical failures currently get parked with the *reviewer's*
  framing only, never the *implementer's* side of the disagreement.

## Users / Consumers

- **Primary**: the autonomous Claude Code sessions that make up the backlog pipeline
  (triage, plan, work, review, ship sessions) — they are both producers and consumers
  of the enriched communication/escalation tooling.
- **Primary human**: the sole human operator (Tyler), who is the target of any
  escalation, dispute-adjudication, or infra-bug-report surfacing designed here, via
  the web UI (`/unfinished`, backlog item detail page) and/or notifications.
- **Secondary/future**: a possible "central Master agent" (operator's own phrase) that
  could consume infra-bug reports or escalations instead of — or in addition to — the
  human, if a distinguished always-on orchestrating session/queue is designed.
- **Downstream**: the existing reconciliation/remediation system
  (`session/backlog_remediation.go`, `StuckReason` enum) is a consumer of whatever new
  signals are introduced, since new escalation/dispute states must compose with it
  rather than fork a second stuck-item taxonomy.

## Success Metrics

- Every one of the four dimensions has a concrete, named mechanism in the plan (tool,
  data field, UI surface, or explicit "reuses X, no new mechanism needed" decision) —
  not left as an open question.
- Pain point A: the plan specifies exactly which MCP tool(s)/fields capture PR
  URL/number/summary at creation time and how that data is guaranteed to reach the
  backlog item record and the review session's context, independent of worktree
  lifecycle timing (the root cause class behind BUG-040/BUG-045).
- Pain point B: the plan specifies a concrete "dispute a verdict" flow with a defined
  adjudicator (human, fresh re-reviewer, or other), a defined data shape for the
  dispute, and a defined UI surface where a human sees disputed verdicts — distinct
  from the silent auto-resolution that happens today.
- The plan explicitly maps each new capability against `StuckReason`/
  `backlog_remediation.go` and either (a) states it's a net-new `StuckReason` variant
  and why that's insufficient on its own, or (b) states the capability reuses existing
  machinery and describes the delta.
- Every new/changed MCP tool or UI surface has an explicit human-visibility point
  (what a human sees, where, and when) — nothing added here should reduce human
  observability into the pipeline versus today.

## Constraints

- No hard deadline. Planning-only for this task — phases 1–4 (ideate → research → plan
  → validate); implementation is explicitly out of scope and must not start.
- Must compose with, not duplicate or fork, the existing stuck-item/remediation-backoff
  system (`session/backlog_remediation.go`, `StuckReason`, `/unfinished` UI) — this is
  a hard design constraint stated directly by the operator.
- Must not assume or block on the outcome of a separate, concurrently-dispatched task
  in this same repo ("SDD as the default backlog pipeline" — which *workflow content*
  runs per item). This task is scoped to communication/escalation/reporting tooling
  around the pipeline, independent of which workflow content runs. Flag, but do not
  block on, any point where the two should eventually reconcile.
- Must follow this repo's existing conventions for MCP tool additions
  (`server/mcp/tools_backlog.go` and friends), ent schema changes
  (`--feature sql/upsert` generation rule), and the feature registry
  (`docs/registry/features/`) if new backend/frontend surface is proposed.
- Solo-operator project — designs should favor low operational overhead (no new
  infrastructure/services) over enterprise-scale generality.

## Scope

### In Scope

1. **Structured forward/backward context handoff between pipeline stages** — enriching
   MCP tools (or adding new ones) so a work-session agent can pass richer context to
   the review session that follows it, and a review agent's findings can be recorded
   in a structured, machine-readable way rather than only a prose verdict string.
2. **Infra/orchestrator-bug reporting path** — a distinct, low-friction way for any
   pipeline agent to report that the orchestrator/tooling itself (not their assigned
   item) is broken, including where that report lands and who/what consumes it.
3. **"Ask for help" escalation** — a dedicated path for a genuinely-stuck agent to
   request human (or "Master agent") help, explicitly designed against/alongside the
   existing `StuckReason`/remediation-backoff system, including what a "central Master
   agent" would concretely mean in this architecture if the design calls for one.
4. **Human-in-the-loop preservation/improvement** — for every mechanism proposed in
   1–3, an explicit design of the human touchpoint (where a human sees it, what action
   they can take, and how urgently).
5. **PR-visibility fix design** (pain point A) — structured PR metadata capture at the
   point a PR is created/reported, and its reliable propagation to the backlog item
   record and reviewer context, addressing the BUG-040/BUG-045 root-cause class.
6. **Verdict dispute path** (pain point B) — a way for an implementer agent to formally
   contest a FAIL verdict, a defined adjudication path, and defined UI visibility.
7. Explicit flags anywhere a proposed capability overlaps existing machinery (e.g.
   `StuckReason`, `backlog_remediation.go`, `PipelineEngine`/`PipelineModeRepository`
   from `session/pipeline_engine.go` and `project_plans/backlog-configurable-pipeline/`)
   so the plan composes with, rather than duplicates, what already exists.

### Out of Scope

- Any code implementation, MCP server changes, ent schema migrations, or UI changes —
  this task stops at planning artifacts (requirements → research → plan → validation)
  for human review before Phase 5.
- Redesigning *which* workflow content/skills run per backlog item (that's the
  separate, concurrently-dispatched "SDD as default pipeline" task) — this task covers
  communication/escalation/reporting tooling only, regardless of workflow content.
- Redesigning the core state machine of backlog item statuses
  (`idea → triage → queued → in_progress → review → pr_pending → shipped`, etc.)
  unless a specific new status is the only way to represent a dispute/escalation state
  — prefer additive fields/sub-states over new top-level statuses where possible.
- Multi-tenant / multi-operator considerations — this is a solo-operator system.
- Building a full "central Master agent" always-on service if research shows a
  lighter-weight mechanism (e.g. a queue + existing session-creation tooling) achieves
  the same human-reachability goal.

## Open Questions

- Is "ask for help" best modeled as a new `StuckReason` variant (reusing existing
  detection/backoff plumbing) with a manual-trigger MCP tool, or does it need a
  genuinely different code path because it's *agent-initiated* rather than
  *reconciler-detected*? (Research/plan phase must answer explicitly.)
- What does "a central Master agent" concretely mean in this architecture — a
  distinguished always-on orchestrating session, a special MCP-tool-triggered
  escalation queue that spawns a triage-like session on demand, or purely a human
  notification path with no agent intermediary? Operator used the phrase but left it
  open for investigation/design.
- Where should structured review findings/handoff context be persisted — new ent
  schema fields on `BacklogProgressNote`/`ItemSession`, a new entity, or a structured
  JSON blob field on an existing entity? (Research/plan phase to evaluate against
  existing schema in `session/ent/schema/`.)
- Should verdict disputes be adjudicated by a human by default, or by a fresh
  re-reviewer session with the dispute as context, with human involvement only on
  repeated disagreement? Operator posed both options; plan phase should recommend one
  with reasoning specific to this pipeline's failure modes (e.g. reviewer/implementer
  model bias risk if the same review criteria are just re-run).
- How does the eventual reconciliation with the concurrent "SDD as default pipeline"
  task happen — is it a later merge task, or should this plan leave explicit
  extension points now? Flag in the plan; do not resolve here.
