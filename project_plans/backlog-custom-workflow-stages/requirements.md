# Requirements: backlog-custom-workflow-stages

**Date**: 2026-09-03
**Type**: feature addition (system design — touches data model, proto, backend orchestration, UI)
**Complexity**: 4 — cross-cutting change (state machine, per-stage liveness detection, and UI) with
Large appetite, per the user's explicit sizing decision below.

## Problem Statement

The backlog's workflow (`BacklogStatus`: idea → refining → ready → queued → in_progress → review →
pr_pending → done/archived) is a fixed, hardcoded 9-state machine — `session/backlog.go`'s
`validTransitions` map and `TransitionGuard`, wrapped (unchanged) by `DefaultWorkflowEngine`
(`session/workflow_engine.go`, per `docs/adr/013-workflow-engine-replaces-valid-transitions.md`).
Adding any new state today means editing the transition map, the transition guard, the proto enum,
`BacklogLifecycleListener`, and `useBacklogService.ts`'s frontend transition table — eight files for
one state, per ADR-013's own framing. ADR-013 explicitly proposed a Phase 2 (`ConfiguredWorkflowEngine`,
DB-persisted custom states) to fix this but it was **never implemented** — `DefaultWorkflowEngine`
remains the only implementation today.

Separately, nothing in the system lets a stage declare its own **liveness/progress semantics** —
what "still running" vs. "stalled" vs. "dead" means for work happening inside that stage, and how
long that work is expected to take. Every stage's staleness/timeout threshold is a hardcoded Go
constant, and different stages' thresholds must be *manually kept in sync* with each other across
files with no compiler or test enforcement of the relationship.

**Concrete, currently-live evidence this is a real (not hypothetical) gap**, root-caused this
session (`docs/tasks/backlog-feature-improvement.md`, "Update — 2026-09-03"): the "sdd" pipeline
mode's triage prompt runs a much heavier unattended workload (`sdd:2-research` → `sdd:3-plan`
→ `sdd:4-validate`, with subagent dispatch) than the default triage prompt, but both are bounded by
the exact same flat, pipeline-mode-blind `triageCallBudget` (30 min,
`server/services/backlog_service_triage.go:434`), and the exact same flat 35-minute
`maxHeadlessTriageSessionStaleness` (`session/backlog_lifecycle_triage.go:64`) — a second, separately
hardcoded constant in a different package that a code comment says "MUST stay strictly greater than"
the first, with no enforcement beyond that comment (see BUG-055). Result: sdd-mode triage reliably
times out, and as of this writing **12 live backlog items are parked in `STUCK_REASON_ORPHANED_TRIAGE`**
because every one of their 5 automated retry attempts failed identically — the retry loop
(`retryOrphanedTriageWithBackoffGate`) has no way to know the timeout itself, not the retry, was the
problem.

This is scoped as the general fix, not a one-off patch to those two constants: **add custom,
user-definable top-level workflow stages (states) with arbitrary transitions between them, per
ADR-013 Phase 2's original intent, where each stage carries its own liveness/progress definition**
(expected duration, staleness threshold, what counts as progress) — so a new custom stage, or a
pipeline mode with different timing needs inside an existing stage, is a configuration change, not a
new hardcoded constant pair someone has to remember to keep in sync.

**A second, related gap in the same hardcoded fixed-workflow design**: today's `WorkflowEngine.ValidateGates`
and `TransitionGuard` (`session/backlog.go`) already express *some* transition gates — AC-required
before `done`, a review verdict of PASS before `review`→`pr_pending`, `SkipReviewGate`/`PlanApproved`
bool overrides — but they are hardcoded per-transition Go logic tied to the fixed 9-state machine.
A user-defined custom transition has no way to require an equivalent check without a code change.
This project also generalizes *that* — which human-review, automated-review, structural, or
custom/pluggable checks must pass before a given transition (built-in or custom) is allowed — as a
configurable property of the transition, alongside its liveness definition.

## Actors for Transition Gates

- **Human reviewer**: approves/rejects a transition explicitly (generalizes today's "Approve Plan" /
  "Override → Done" / manual "Submit Review" actions).
- **Automated reviewer**: a headless LLM review call (via the existing `PipelineEngine`-selected
  review prompt) whose verdict (PASS-equivalent) gates the transition (generalizes today's
  `review_gate.go` PASS/FAIL/UNVERIFIABLE flow) — this is itself a piece of work with its own
  liveness/timeout envelope, so the gate model and the liveness model from the Problem Statement
  above are not independent; see Rabbit Holes.
- **Structural/mechanical check**: a precondition evaluated directly against item state — "all
  acceptance criteria done," "a PR exists and CI is green," "no open BLOCKER findings" (generalizes
  today's AC-required and PR-mergeability checks).
- **Custom/pluggable check**: an extensibility point for a check type not enumerated above (e.g. a
  user-defined script or skill invocation whose exit/verdict gates the transition) — open-ended by
  the user's explicit choice to include this, not a closed enum; see Rabbit Holes for the scope risk
  this carries.

## Baseline

Today: `BacklogStatus` is a fixed Go-level enum; adding a state requires code changes across 8 files
per ADR-013. Liveness/staleness for stuck-detection is one hardcoded duration constant per
`StuckReason` (`session/domain/backlog.go`'s enum, ~12 reasons), scattered across
`server/services/backlog_service_triage.go`, `session/backlog_lifecycle*.go`, and
`session/backlog_remediation.go`, with no per-pipeline-mode or per-stage variation and no structural
mechanism keeping related constants (e.g. a call budget and the staleness sweep that must exceed it)
in sync — confirmed broken in exactly this way for the sdd/triage pair above. A user cannot define a
new workflow stage, or adjust how long a stage is allowed to run, without an engineer editing Go
source and shipping a deploy.

## Users / Consumers

- The backlog UI operator (single operator, per `project_plans/backlog-configurable-pipeline/`'s
  established threat model) defining/editing custom stages and transitions, and their liveness
  parameters, through a management UI.
- Backend orchestration code that currently hardcodes the state machine and per-`StuckReason`
  thresholds: `session/workflow_engine.go` (`WorkflowEngine`/`DefaultWorkflowEngine`),
  `session/backlog.go` (`validTransitions`/`TransitionGuard`), `session/backlog_lifecycle*.go` (the
  `reconcile*` stuck-detection sweeps), `session/backlog_remediation.go` (the shared remediation
  backoff gate), `server/services/backlog_service_triage.go` (`triageCallBudget` and its siblings).
- `session.PipelineEngine` (already implemented by `project_plans/backlog-configurable-pipeline/`) —
  this project is a sibling/consumer relationship, not a merge: `PipelineEngine` decides *what*
  skills/prompts run within a given stage; this project decides *which stages exist, how they
  connect, and how long work inside them is expected to take*. Phase 2 research must read that
  project's requirements.md and ADR-001 in full before designing anything here, to avoid re-litigating
  its "Runtime Configurability Decision" or duplicating its caching-layer design.
- `web-app/src/components/backlog/BacklogBoard.tsx` (`COLUMNS`, currently a hardcoded 9-entry array)
  and `useBacklogService.ts`'s frontend transition table — both need to render whatever stage set is
  actually configured, not a fixed list.

## Success Metrics

- A user can define a new top-level workflow stage (name, description, allowed
  incoming/outgoing transitions, and its liveness/progress definition) through a UI, without a code
  change or redeploy, and immediately use it on a backlog item — mirroring
  `backlog-configurable-pipeline`'s decisive success metric for `PipelineEngine` modes, applied here
  to stages/transitions instead.
- Each stage's liveness/progress definition (expected duration, staleness threshold) is queryable
  and enforced by the stuck-detection sweeps and remediation backoff gate — not a second hardcoded
  constant someone has to remember to update in step.
- **Milestone 1 (pulled forward — see Risk Control)**: the concrete motivating bug is fixed as an
  instance of the general model — the "idea" stage's liveness definition varies correctly by
  pipeline mode (sdd vs. default), so sdd-mode triage stops reliably timing out, verified against
  the 12 currently-parked items (`306bbc57` and its 11 siblings) recovering on their next remediation
  attempt once this ships and the relevant config is set, with no manual per-item intervention beyond
  what the existing cold-retry heartbeat (BUG-083) already provides.
- Existing items and the existing 9-state workflow behave exactly as today with zero explicit
  configuration (the shipped default stage set reproduces current behavior) — zero regression,
  mirroring `backlog-configurable-pipeline`'s identical guarantee for `PipelineEngine`.
- `BacklogBoard.tsx` and the item detail page render whatever stage set is actually configured
  (including any user-defined custom stages), not a fixed list.
- A user can attach one or more gates (human approval, automated review verdict, structural check,
  or a custom/pluggable check) to a custom transition through the same UI, and the transition is
  refused until every attached gate is satisfied — verified by defining a custom transition with a
  human-approval gate and confirming `WorkflowEngine`/`ConfiguredWorkflowEngine` refuses it pre-approval
  and allows it post-approval, with no code change between the two.
- The item detail UI shows, for any pending transition, which gate(s) are blocking it and who/what
  can satisfy each one — closing the same "what's blocking this" visibility gap the original
  `backlog-feature-improvement` audit found for the fixed workflow.

## Appetite

**Large (3–6 weeks)** — explicit user decision: full ADR-013 Phase 2 scope end-to-end (DB
schema/migration, `ConfiguredWorkflowEngine`, per-stage liveness, and a management UI for
defining/editing custom stages and transitions, mirroring `backlog-configurable-pipeline`'s
pipeline-mode CRUD UI), not a backend-only cut.

**Milestone sequencing (explicit user decision)**: the per-stage liveness/timeout piece — which
subsumes the motivating sdd-triage-timeout bug — is pulled forward as an early, independently
shippable milestone (Milestone 1) inside this project, ahead of the full custom-stage/transition
definition UI (Milestone 2+). Phase 3 planning must structure the plan so Milestone 1 can ship and
deploy on its own. This is safe to sequence this way because the 12 currently-parked items are not
silently stuck in the interim — `docs/bugs/fixed/BUG-083` already gives every parked `StuckReason`
row a 7-day cold-retry heartbeat, so they will pick up Milestone 1's fix automatically once it
deploys, without needing a synchronized "ship the whole project, then manually reset 12 items" step.

## Constraints

- Must reuse the narrow-interface + deep-copy-on-construct pattern already established by
  `session/workflow_engine.go` (`WorkflowEngine`) — extending it (per ADR-013's own Phase 2 framing:
  "Phase 2 custom states are a drop-in: swap `DefaultWorkflowEngine` with `ConfiguredWorkflowEngine`
  via the same interface") rather than replacing or duplicating the seam.
- Must NOT duplicate or re-litigate `session.PipelineEngine` (`project_plans/backlog-configurable-pipeline/`,
  already shipped) — that project's "Runtime Configurability Decision" (DB-persisted, single-operator
  threat model, structured-not-freetext content, explicit cache in front of the mode table) is settled
  precedent this project should follow for its own DB-persisted stage/transition config, not
  re-derive from scratch. Read that project's requirements.md, ADR-001, and `research/pitfalls.md`
  before Phase 3 planning.
- `.claude/rules/interface-pollution-checklist.md` — no speculative interfaces, no Java/Spring-shaped
  layering; `WorkflowEngine`'s existing narrow-interface style is the template.
- Any new per-item/per-stage field follows the existing `BacklogItemData`/`BacklogItemUpdate` pattern
  in `session/repository.go` (plain struct fields, `*T` for optional partial-update semantics) — see
  `backlog-configurable-pipeline`'s own cited bucket-[2] finding that non-optional proto3 bools
  already caused a silent flag-clobbering bug once; a new field must not repeat that mistake.
- Any new session-creation-adjacent surface must satisfy `docs/reference/session-creation-registry.md`'s
  7-touchpoint checklist if it behaves like a session-creation mode; must satisfy
  `docs/reference/feature-registry.md` regardless.
- `ent` schema changes require the `--feature sql/upsert` regeneration flag exactly as documented in
  this repo's `CLAUDE.md` (`session/ent/generate.go`) — a known footgun if skipped.
- BUG-055's invariant (a stage's staleness-sweep threshold must stay strictly greater than its own
  call/work budget, with real margin, or the sweep and the work's own natural
  completion/timeout race) must be an enforced property of the new per-stage liveness model — not
  something Phase 3 planning re-derives as "two numbers a human keeps in sync," which is the exact
  design that just failed live.

## Non-functional Requirements

- **Performance SLO**: resolving a stage's transition legality and liveness definition must not add
  an *uncached* synchronous DB read to hot paths (`TransitionBacklogItemStatus`, the periodic
  `reconcile*` stuck-detection sweeps, `RemediationDue`) — same caching mandate
  `backlog-configurable-pipeline` already established for `PipelineEngine`; Phase 3 planning should
  evaluate reusing that same cache infrastructure rather than building a second one.
- **Scalability**: not applicable — single-operator internal tool, no multi-tenant concerns (same
  posture as the sibling `PipelineEngine` project).
- **Security classification**: internal, single-operator — same relaxed-but-still-structured threat
  model as `backlog-configurable-pipeline`'s Runtime Configurability Decision (structural integrity
  against self-inflicted misconfiguration, not access control against an untrusted third party).
- **Data residency**: not applicable.

## Scope

### In Scope

- A DB-persisted, user-definable set of custom top-level workflow stages (states) and the legal
  transitions between them — `ConfiguredWorkflowEngine` per ADR-013 Phase 2, satisfying the existing
  `WorkflowEngine` interface (or an intentionally-evolved version of it, if Phase 3 planning finds the
  current 3-method interface insufficient for liveness — to be decided in planning, not here).
- A configurable transition-gate model generalizing today's `ValidateGates`/`TransitionGuard`:
  each transition (built-in or custom) can require one or more gates — human approval, automated
  review verdict, structural/mechanical check, or a custom/pluggable check (per the Actors for
  Transition Gates section above) — evaluated before the transition is allowed, with the item
  detail UI surfacing which gate(s) are outstanding.
- A per-stage (and, where the motivating bug requires it, per-stage-×-pipeline-mode) liveness/progress
  definition: expected duration, staleness/timeout threshold, and what counts as "still running" vs.
  "stalled" vs. "dead" for work inside that stage — consulted by the stuck-detection sweeps
  (`session/backlog_lifecycle*.go`'s `reconcile*` functions) and the remediation backoff gate
  (`session/backlog_remediation.go`) instead of each reading its own hardcoded constant.
- Migrating the existing hardcoded per-`StuckReason` thresholds (`maxWorkSessionStaleness`,
  `maxHeadlessTriageSessionStaleness`, `triageCallBudget`, and siblings) onto the new model, including
  the motivating sdd/triage pair, as Milestone 1.
- ent schema + migration for the new stage/transition/liveness config tables, proto changes, and
  RPCs mirroring `WorkflowRepository`'s (or `backlog-configurable-pipeline`'s pipeline-mode
  repository's) CRUD surface.
- UI: a management surface for defining/editing/enabling/disabling custom stages, their transitions,
  and their liveness parameters (Milestone 2+, per the appetite decision), plus updating
  `BacklogBoard.tsx`'s `COLUMNS` and `useBacklogService.ts`'s transition table to render whatever
  stage set is actually configured instead of a fixed list.
- A new or updated ADR: either implement ADR-013 Phase 2 as originally proposed and mark it
  Accepted/Implemented, or record where this project's design diverges from that proposal and why.

### Out of Scope

- Re-litigating or duplicating `session.PipelineEngine` (which skills/prompts run within a stage) —
  already shipped by `backlog-configurable-pipeline`; this project only decides which stages exist,
  how they connect, and their liveness, consuming `PipelineEngine` unchanged where relevant (e.g. a
  stage's liveness definition may vary by which pipeline mode is active within it, as the motivating
  bug requires, but this project does not change how pipeline modes themselves are selected or
  authored).
- Multi-tenant / multi-user permissions for who can define or use which stage — remains a
  single-operator tool.
- `session/autonomous_driver.go`'s hardcoded orchestration prompt/signals — a related but separate
  concern already flagged out-of-scope by the sibling `PipelineEngine` project; not reopened here.
- Reworking the *content* of individual stages' work (what an agent actually does inside a stage) —
  this project is about the state machine and liveness envelope around that work, not the work
  itself.

## Rabbit Holes

- **The relationship between "a stage's liveness definition" and "a pipeline mode's expected
  workload" is the exact axis the motivating bug lives on** — sdd-mode triage needs a different
  budget than default-mode triage *within the same "idea" stage*. Phase 3 planning must explicitly
  decide whether liveness is a property of (stage) alone, (stage × pipeline-mode), or configurable at
  an even finer grain — do not let this get decided implicitly by whatever's easiest to schema first,
  since choosing "stage alone" would fail to fix the motivating bug at all.
- **BUG-055's race** (a staleness sweep tombstoning a call that hasn't actually timed out yet) is a
  real, previously-hit failure mode, not a theoretical edge case — any new liveness model must make
  "sweep threshold > work budget, with margin" a property the model itself enforces or derives (e.g.
  compute the sweep threshold as budget-plus-margin rather than storing both as independent config
  values), not two independently-editable numbers a UI lets a user set inconsistently.
- **Migrating existing `StuckReason` thresholds onto the new model risks silently changing today's
  live remediation timing** (backoff schedule, cold-retry heartbeat from BUG-083) if the migration
  isn't a careful value-for-value port. Phase 3 planning must treat "existing stuck-detection behavior
  is bit-for-bit unchanged for the default/built-in stages" as a hard regression gate, verified before
  any custom-stage work is considered done.
- **A management UI for custom stages/transitions is real, substantial UI work** (a graph/transition
  editor is a different shape than `backlog-configurable-pipeline`'s flat mode-selector dropdown) —
  budget real design time in Phase 3; don't assume it's a small extension of the existing pipeline-mode
  settings page.
- **The "custom/pluggable check" gate type is open-ended by the user's explicit choice, and it is not
  independent of the liveness model above** — a custom check that shells out to a script or invokes a
  skill is itself a piece of work that can hang, crash, or run long, which means it needs the exact
  same liveness/timeout envelope this project is already building for stages. Do not design the gate
  model and the liveness model as two unrelated features that happen to ship together; a
  custom/pluggable gate check should be expressible using the same liveness primitives, not a third,
  bespoke timeout mechanism. Phase 3 planning must also bound *how* a custom check executes (sandboxing,
  what it can access, how its verdict is reported back) — this is the single largest scope-blowout risk
  in the whole project if left unbounded; consider constraining Milestone 2's custom-check support to a
  narrow, well-defined interface (e.g. "invoke this named skill/slash-command, treat exit 0 / a specific
  report_progress-style call as pass") rather than arbitrary code execution.
- **Automated-review-verdict gates already have a real implementation to reuse, not invent**:
  `session/review_gate.go`'s PASS/FAIL/UNVERIFIABLE flow is the existing automated-reviewer shape.
  Generalizing it for custom transitions should extend/parameterize that existing mechanism, not build
  a parallel one.

## Alternatives Considered

- **Patch only the two specific constants the motivating bug hit** (give `triageCallBudget` a
  per-pipeline-mode override, bump `maxHeadlessTriageSessionStaleness` to match) — rejected as the
  long-term approach per the user's explicit choice this session: this is the Nth time a
  stage/session-timing constant has needed a one-off fix (BUG-055 already documents the exact
  sweep-vs-budget race this bug is a second instance of), and ADR-013 Phase 2 already named the
  correct general fix over a year ago without anyone building it. A third one-off patch would leave
  the next custom-stage timing mismatch to be independently rediscovered.
- **Fold this into `backlog-configurable-pipeline` as a follow-up phase instead of a new project** —
  rejected: that project's requirements.md explicitly scoped `ConfiguredWorkflowEngine`/custom states
  out ("a related but separate initiative; do not implement it as part of this project"), and its
  `PipelineEngine` seam is a different concept (skill/prompt selection, not state machine shape) from
  what this project needs to build. Keeping them separate matches that project's own stated boundary.

## Feasibility Risks

- `WorkflowEngine`'s current 3-method interface (`CanTransition`, `ValidateGates`,
  `AllowedTransitions`) was designed for status-transition legality only, with no liveness concept at
  all — extending it to carry per-stage liveness data is unproven and needs its own design pass in
  Phase 3, not a mechanical addition of a 4th method.
- Same caching-layer risk `backlog-configurable-pipeline` already flagged for `PipelineEngine`:
  `WorkflowRepository` (the closest existing DB-persisted/slug-addressed precedent) has no caching
  layer at all. Reuse whatever caching design that sibling project landed on if it fits, rather than
  inventing a second one.
- Risk that "liveness is a property of (stage × pipeline-mode)" (see Rabbit Holes) turns out to need a
  third axis once Phase 2 research surveys every current `StuckReason` — e.g. `stale_work`'s
  liveness concept (an active work session reporting no progress) is shaped differently from
  `orphaned_triage`'s (a headless call that either finishes or doesn't). The model may need to
  support more than one liveness *shape*, not just one shape with configurable numbers. Phase 2
  research must survey all ~12 existing `StuckReason`s' actual liveness semantics before Phase 3
  designs the schema, not just the one (`orphaned_triage`) the motivating bug hit.
- `ent` schema changes require the `--feature sql/upsert` regeneration flag — low risk, well
  documented in this repo's `CLAUDE.md`, but a known footgun if skipped.

## Observability Requirements

Log which stage/liveness definition was resolved for a given item at each stage transition and at
each stuck-detection sweep decision, at Info level, following the existing
`[BacklogLifecycle]`/`[TriggerTriage]` log-prefix convention — needed to debug "why did this item's
staleness threshold not match what I configured" without new tooling. Log cache misses/refreshes for
any new stage/liveness config cache at Debug level. Log at Warn (not silent fallback) whenever a
stage or liveness identifier fails to resolve, falling back to the default/built-in behavior rather
than crashing or silently using zero/infinite thresholds — mirrors the "fail closed and loud on
unrecognized value" lesson already applied elsewhere in this subsystem. No new metrics/alerts
required; single-operator tool, no oncall rotation.

## Risk Control

The default/built-in stage set (idea → refining → ready → queued → in_progress → review →
pr_pending → done/archived) and its migrated-over liveness thresholds must be **behaviorally
identical to today's hardcoded behavior** for every item that doesn't opt into a custom stage — land
this as its own reviewable commit, verified via the existing stuck-detection regression test suite,
before any custom-stage or UI work is considered for that milestone. This is the concrete mechanism
for "zero regression for the current single-pipeline behavior" above.

**Milestone 1 (per the Appetite section's explicit sequencing decision)**: the per-stage-×-pipeline-mode
liveness fix for the "idea"/triage case ships and can be deployed independently of Milestone 2+ (the
custom-stage-definition UI), so live relief for the 12 parked items does not wait on the full project.
An unresolvable/malformed custom stage or liveness definition on a live item must fail closed to the
default built-in behavior with a loud Warn log (per Observability above), never a silent no-op or a
crash — same risk-control mechanism the sibling `PipelineEngine` project already established for its
own DB-mutable config.

## Open Questions

*(resolved by Phase 2 research — `research/architecture.md`, `research/features.md`,
`research/pitfalls.md`, `research/ux.md` — unless marked otherwise)*

- ~~Exactly how does a stage's liveness definition vary by pipeline mode?~~ **RESOLVED**: liveness
  is not one shape — the `StuckReason` survey found at least 3-4 genuinely distinct detection
  mechanisms (duration-budget-plus-margin for headless calls, heartbeat-staleness for live tmux
  sessions, cycle-frequency for `bouncing`, and by-design-indefinite gates with no timeout at all).
  `LivenessDefinition` should be a tagged union keyed by (stage) with a sparse (stage ×
  pipeline-mode) override — not one flat `{expected_duration, staleness_threshold}` schema.
  `backlog_remediation.go`'s retry-backoff schedule is a separate, orthogonal axis and should stay
  global/unconfigured, not folded into per-stage liveness config.
- ~~Should `WorkflowEngine` grow a liveness method, or a sibling interface?~~ **RESOLVED**: a new
  sibling `LivenessEngine` interface (mirrors the `PipelineEngine`/`WorkflowEngine` separation).
  Gates, by contrast, should extend `WorkflowEngine` with one new method (e.g. `PendingGates`)
  rather than a parallel interface, since they're the same question `ValidateGates` already answers.
  Gate *initiation* (spawning a review, creating an approval affordance) stays outside both
  interfaces as orchestration, matching `review_gate.go`'s existing spawn/record split.
- ~~Does adding custom stages require changes to `ReviewGateRunner`/`AutonomousDriver`?~~
  **RESOLVED, with a correction to this doc's own assumption**: `review_gate.go` and
  `autonomous_driver.go` have **zero** literal `BacklogStatus` branches. The real anchoring lives in
  `session/backlog_lifecycle_review.go`, `autonomous_orchestration_service.go`'s
  `onAutonomousDriverComplete` (hardcodes `toStatus = BacklogStatusReady`), and
  `server/mcp/tools_backlog.go`'s status whitelists. **New scope boundary this surfaced**: a custom
  stage becomes a full graph citizen for transitions/liveness but does **not** automatically inherit
  review-gate/MCP-tool/stuck-detection behavior — that must be attached explicitly via the gate
  model. Phase 3 planning must state this boundary explicitly in the plan, not leave it implicit.
- Should the existing `StuckReason` enum be replaced by stage-derived reasons, or kept separate?
  **Still open** — research leaned toward keeping liveness classification as a sibling layer
  (consistent with keeping remediation-backoff global/separate, above) but did not explicitly settle
  `StuckReason`'s fate. Phase 3 planning must decide before touching `session/backlog_remediation.go`.
- ~~Should transition gates extend `WorkflowEngine` or live in a sibling interface?~~ **RESOLVED**
  — see the `LivenessEngine`/`PendingGates` resolution above.
- What does "human approval" mean for a custom transition with no existing UI action? **Substantially
  answered** by UX research: gates render as a GitHub-branch-protection-style checklist (not a
  canvas), so a human-approval gate generates a generic approve/reject affordance in that checklist
  automatically. Exact mechanics (button copy, who's notified) still for Phase 3 to detail.
- For an automated-review-verdict gate on a custom transition, which review prompt runs and how does
  the verdict map? **Substantially answered**: `review_gate.go` is already `PipelineEngine`-aware
  (calls `InteractiveReviewPromptFor`), and the hardcoding is narrower than assumed — one call site
  (`session/backlog_lifecycle.go:798`, gated on `toStatus == BacklogStatusReview`). `Run`'s inline
  guards (worktree-identity, branch-drift, empty-diff checks) assume a diff/PR is involved, though,
  and are **not** reusable verbatim for a non-code-shipping custom transition like idea→ready — Phase
  3 must design around this, not assume full reuse.

**New items surfaced by Phase 2 research, not originally anticipated by this doc:**

- **Pre-existing landmine, must be fixed as part of this project (pitfalls.md)**: two call sites
  already bypass the injected `WorkflowEngine` by calling `session.CanTransitionBacklog` directly —
  `backlog_service_lifecycle.go:801` (`OverrideVerdict`) and `backlog_service_sync.go:121`
  (`AttachSessionToItem`) — flagged as harmless *today* by `docs/tasks/backlog-feature-improvement.md`
  (~line 1413) but would silently let a custom transition's gates be bypassed on day one once
  `ConfiguredWorkflowEngine` ships. Phase 3's plan must include re-routing both call sites through the
  engine as an in-scope task, not a follow-up.
- **Migration risk is lower than assumed (architecture.md)**: `BacklogStuckState` rows key off a
  plain unvalidated `reason` string with no FK to any stage config, so the 12 live parked items need
  zero data migration — only a characterization test proving Milestone 1's migrated threshold values
  are bit-for-bit identical to today's hardcoded constants (per this doc's existing Risk Control
  section).
- **Fail-closed needs to point in opposite directions for liveness vs. gates (pitfalls.md)**: a safe
  default *duration* for unresolvable liveness config, but the transition must *block* (not
  proceed) on an unresolvable gate — there's no safe default "pass" state for an arbitrary custom
  gate. This doc's Observability/Risk Control sections should be read with that asymmetry in mind;
  Phase 3's plan must state both directions explicitly.
- **No new backend dependency; one likely new frontend dependency (stack.md, build-vs-buy.md)**:
  `qmuntal/stateless` and similar OSS state-machine libraries don't model liveness or typed gates, so
  they'd save ~40 lines of already-correct transition-legality code while leaving 100% of the actual
  new work to hand-build — not worth adopting. `session/pipeline_engine.go`'s existing
  `pipelineModeCache` (atomic-pointer, copy-on-write, stdlib-only) should be reused verbatim for the
  new engine's cache. On the frontend, no graph/node-edge library exists in `web-app/package.json`
  today — but UX research recommends skipping a graph canvas entirely (a11y risk, disproportionate to
  scale) in favor of extending `PipelineModeForm.tsx`'s existing list-based CRUD pattern, which avoids
  needing one.
- **Naming collision (ux.md)**: "Workflow" is already taken twice in the UI (`web-app/src/app/workflows/`
  — cron-scheduled automation routines — and `WorkflowHistorySection.tsx`, the status-transition audit
  trail). `ConfiguredWorkflowEngine` must surface in the UI as "Stages"/"Pipeline Stages," not
  "Workflow(s)."
