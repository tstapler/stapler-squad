# Research: Architecture — backlog-agent-communication

**Date**: 2026-07-23

## Current Pipeline Architecture (as-built)

```
idea → refining → ready → queued → in_progress → review → pr_pending → done → archived
                                        │             │
                                   (work session)  (review session)
```

State machine: `session/domain/backlog.go` — `validTransitions` map +
`TransitionGuard` (business-rule guards, e.g. `ErrACRequired`, `ErrPlanRequired`,
`ErrVerdictRequired`). This is a pure, DB-independent, table-driven state machine —
new dimensions of this project should prefer *additive fields* on existing
statuses/entities over new top-level `BacklogStatus` values, matching the
requirements.md scope constraint.

**Orchestration layer**: `session.BacklogLifecycleListener`
(`session/backlog_lifecycle.go`, ~2600+ lines) is the central event-driven
coordinator — reacts to `onSessionStarted`/`onSessionExited` lifecycle events, and
owns the two flows most relevant to this project:
- `handleReviewSessionExited` — the review→next-step decision point (pain point B
  lives here: FAIL/PARTIAL/UNVERIFIABLE goes straight to
  `autoReopenWithBackoffGate` → `AutoReopenAfterFailedReview`, with **no branch that
  gives the implementer a way to contest the verdict before rework is triggered**).
- `shipViaAgentOrFallback` / `pushAndCreatePR` — the PR-creation point (pain point A
  lives here: PR URL/number are cached onto the `BacklogItem` row here, and BUG-040
  found two independent ways this write can silently not happen or happen out of
  order).

**Remediation/backoff layer**: `session/backlog_remediation.go` — `RemediationDue`
is the single shared gate every *automated* remediation action (reopen-after-fail,
PR-fix respawn, stale-work respawn) must pass before acting, backed by a
`BacklogStuckState` row per `(item_id, reason)` with a 5-step exponential backoff
(30m → 2h → 8h → 24h → 72h) and a hard cap (`MaxRemediationAttempts = 5`) after
which the row "parks" and a one-time notification fires. **Key architectural fact
for dimension 3 ("ask for help")**: this system is entirely **reconciler-initiated**
— a periodic sweep (`ReconcileStuck`) *detects* a stuck shape and *then* gates its
own automated retry. It has no concept of an **agent-initiated** signal ("I,
mid-session, am declaring myself stuck right now") — see Pitfalls research for why
this matters.

**Stuck-item taxonomy**: `session/domain/backlog.go`'s `StuckReason` enum (11
values as of 2026-07-23) — each reason has a detector (periodic sweep function), a
`BacklogStuckState` row lifecycle (`MarkStuck`/`ResolveStuck`/`selfHealStuck`), and
a `/unfinished` UI card (`web-app/src/components/backlog-stuck/`). This is the
**existing durable, human-visible "something needs attention" surface** — dimension
2 (infra-bug reporting) and dimension 3 (ask-for-help) should be evaluated against
reusing this exact plumbing (new `StuckReason` values +
`MarkStuck`/`RemediationDue`) rather than building a parallel system, per the
explicit "compose, don't duplicate" constraint in requirements.md.

**Pipeline content layer**: `session.PipelineEngine` /
`PipelineModeRepository` (`session/pipeline_engine.go`,
`project_plans/backlog-configurable-pipeline/`) is a separate, orthogonal axis —
it resolves *which prompt/skill content* runs per stage (already snapshotted onto
`ItemSession.pipeline_mode_snapshot(_hash)`). This project's scope
(communication/escalation/reporting tooling) is independent of which pipeline mode
is active; new MCP tools this project adds should be available regardless of
`PipelineMode`, not gated by it, unless a specific reason emerges in planning.

## MCP Tool Call Graph (agent → orchestrator today)

```
triage session ──submit_triage_result──► BacklogService (sets AC, plan)
work session    ──report_progress──────► AC criterion status + BacklogProgressNote (append-only)
work session    ──request_review───────► status→review, spawns review session
review session  ──submit_review_verdict► ReviewVerdictData (per-criterion + summary)
                                          │
                        BacklogLifecycleListener.handleReviewSessionExited
                                          │
                        ┌─────────────────┴─────────────────┐
                     FAIL/PARTIAL/UNVERIFIABLE            PASS
                     autoReopenWithBackoffGate       shipViaAgentOrFallback
                     (rework, no dispute path)        (push + create PR)
```

Every arrow above is **one-directional and upward** (agent → orchestrator/DB). There
is currently no arrow representing: (a) review session → work session structured
handoff, (b) work session → review session forward context beyond
`verification_notes` (freeform text) and the diff itself, (c) any agent →
human-or-Master-agent lateral escalation channel that isn't "wait for the periodic
stuck sweep to eventually notice."

## Integration Points for the Four Dimensions

1. **Structured handoff (forward + backward)**
   - Forward (work → review): `request_review`'s `verification_notes` field
     (freeform, ≤4000 chars) is the only existing forward-context channel beyond the
     diff. A structured extension (e.g. a typed "what I changed and why" JSON blob,
     or explicit fields for "files touched", "known limitations", "areas needing
     extra scrutiny") would extend `request_review`'s args and
     `ItemSession.verification_notes` (or a new column) — additive, no new entity
     required per `BuildReviewPrompt`'s existing consumption of
     `verification_notes`.
   - Backward (review → work, structured findings): `submit_review_verdict`'s
     `CriterionVerdict.Evidence` is currently the only structured-ish per-criterion
     data; `summary` is a single freeform string with no taxonomy. A structured
     findings list (severity, category, file:line, suggested fix) mirrors this
     repo's own review-agent output conventions (`code-review` skill's
     `[BLOCKER]/[CRITICAL]/[MAJOR]/[NIT]` labels) — reusing that vocabulary instead
     of inventing a new one is a concrete, low-risk design choice worth flagging in
     planning.

2. **Infra-bug reporting**
   - No existing tool distinguishes "my assigned work is broken" from "the
     orchestrator/tooling is broken." The natural home is either (a) a new MCP tool
     (`report_infra_issue` or similar) that writes to a new lightweight
     append-only table (mirroring `BacklogProgressNote`'s shape but scoped
     globally, not per-item) and fires a `Notifier` call at a distinct priority, or
     (b) a documented *convention* for reusing an existing generic mechanism.
     Today's closest generic mechanism is the six BUG-04x docs themselves — all
     filed manually by an agent editing `docs/bugs/open/*.md` directly, with no MCP
     tool involved at all. That manual convention already works today (proven by
     BUG-040 through BUG-045 all being filed this way) — planning should explicitly
     decide whether a new MCP tool materially improves on "agent writes a bug doc
     and mentions it," or whether the gap is really about *low-friction discovery*
     (a human has to notice a new file in `docs/bugs/open/`) rather than *capture*.

3. **Ask for help / escalation**
   - No existing "agent declares itself stuck, right now, mid-session" tool. The
     closest existing primitive is `set_session_goal` with `status: "blocked"`
     (`server/mcp/tools_goal.go`) — this already exists and is visible via
     `get_session_goal`/session listing, but is **not** wired into the
     `StuckReason`/`/unfinished` durable surface, and nothing currently reads
     `status: "blocked"` and surfaces it distinctly to a human as "needs help" vs.
     "just hasn't updated its goal in a while." This is a strong candidate reuse
     point: dimension 3 may be substantially "wire `blocked` goal status into a new
     `StuckReason` (e.g. `StuckReasonAgentRequestedHelp`) detector + a richer
     `set_session_goal` payload (a reason/question field)" rather than a wholly new
     tool — see Open Question in requirements.md; plan phase must decide explicitly.
   - "Central Master agent": no existing always-on orchestrating session exists in
     this codebase. The closest structural analogue is `headless.Pool`
     (spawns short-lived headless sessions on demand — used for review sessions) —
     an "ask for help" escalation could plausibly spawn a short-lived
     "triage-the-escalation" headless session (reusing `headless.Pool`) rather than
     requiring a persistent "Master agent" process, which would be new
     infrastructure and cuts against the low-operational-overhead constraint.

4. **Human-in-the-loop preservation**
   - Every existing human touchpoint funnels through exactly two surfaces:
     `Notifier.Notify` (ephemeral push/toast) and `StuckReason` +
     `/unfinished` (durable, actionable card). Any new capability from dimensions
     1–3 should terminate in one or both of these, not a third bespoke surface,
     unless research/planning finds a concrete reason the existing two are
     insufficient (e.g. a dispute needs a *response* channel back into the
     pipeline, which neither existing surface supports today — `/unfinished`'s
     actions are all "retry/reset/snooze", not "provide adjudication input").

## Related In-Flight Work (flag, don't block on)

- `project_plans/backlog-configurable-pipeline/` — orthogonal (workflow content
  selection), per requirements.md's explicit scope note.
- A separately-dispatched "SDD as default backlog pipeline" task (per this task's
  brief) — also orthogonal, same reasoning.
