# Requirements: backlog-sdd-default-pipeline

**Date**: 2026-07-24
**Type**: feature addition (builds on an already-shipped platform: `project_plans/backlog-configurable-pipeline/`)
**Complexity**: 2 — the hard, high-complexity part (DB-persisted `PipelineMode` + cache + CRUD UI) already
shipped. This project only adds a mode definition and a default-selection mechanism on top of it.

Source: operator request — "make the SDD workflow the new default automation pipeline for the
backlog feature." The `sdd:1-ideate` interactive interview is skipped: this is an autonomous,
unattended background run (no user present to answer `AskUserQuestion` prompts), and the request
itself already answers problem/why in enough detail to write requirements directly — the same
judgment call `backlog-configurable-pipeline/requirements.md` made for the same reason, and the
same judgment call this item's own `sdd`-flavored triage/initial prompts (designed below) make for
every future autonomous backlog item.

## Problem Statement

Today a backlog item's default pipeline (`PipelineMode == ""`, `PipelineModeDefault`) is a single
flat work session: triage produces a lightweight ad hoc research/plan/validate pass
(`BuildHeadlessTriagePrompt`), then one work session implements everything and calls
`/backlog/review` once. There is no distinct, artifact-producing planning phase, no adversarial
plan review, and no structured multi-layer verify gate before review — the operator wants backlog
items to default to running through the repo's own SDD phases (research → plan → validate →
implement → verify) instead.

## Baseline (confirmed by direct inspection before writing this doc)

- The configurable-pipeline **platform** (`session/pipeline_engine.go`, `PipelineMode` ent schema,
  CRUD RPCs `server/services/backlog_service_pipeline_mode.go`, cache, `ValidatePipelineModeContent`)
  is fully implemented and merged (commits `37daaed8`..`ad01a131`). This project does not rebuild any
  of it.
- The audit note that `BacklogItemForm.tsx` didn't expose a pipeline-mode selector is **stale** —
  commit `54a34cc4` ("add pipeline-mode selector, management UI, and what-ran surface") already wired
  a `RadioGroup` selector (`pipelineMode` state, `listPipelineModes()`), the `/settings/pipeline-modes`
  management UI, and an item-detail "what ran" surface. No selector work is needed here.
- No `PipelineMode` row named `sdd` (or any row at all) exists yet — there is no seed mechanism for
  `PipelineMode` rows anywhere in the codebase today (confirmed: no `Seed`/`Bootstrap` call site in
  `session/workflow_repository.go`, `session/ent_workflow_repository.go`, or `server/dependencies.go`
  for either `Workflow` or `PipelineMode`). An "sdd" mode must be created by this project, not assumed
  to already exist.
- `CachingPipelineEngine`'s custom-mode branches (`TriagePromptFor`, `ReviewPromptFor`,
  `InteractiveReviewPromptFor`) render a mode's templates using only `itemPlaceholders(item)` (item_id,
  item_title, item_description, repo_path) plus `criteria_count` — **the `artifactAbsPath` parameter
  (triage) and the `diff`/`acSnapshot`/`verificationNotes`/`extras` parameters (review) are silently
  dropped for every non-default mode**, not just this one. This is a genuine, pre-existing platform
  gap (see Feasibility Risks below), not something introduced here.

## Users / Consumers

- The backlog operator (single-operator tool), who will see a new "SDD (Stapler-Driven Development)"
  entry in the existing pipeline-mode selector and a new toggle in `/settings/features`.
- The spawned headless triage session, the spawned interactive/autonomous work session, and the
  spawned review-gate session for any item whose `PipelineMode == "sdd"` — all three read their
  prompt content from the new `PipelineMode` row via the existing `PipelineEngine` seam.

## Success Metrics

- An "sdd" `PipelineMode` row exists after a normal server boot with no manual UI/DB step, and its
  content passes the same `ValidatePipelineModeContent` structural checks the CRUD RPCs enforce.
- Selecting "sdd" on a backlog item changes what actually gets spawned: the slash commands and the
  initial/triage/review prompts written for that item differ from the default's, and the initial
  prompt instructs the session to run the `sdd:2-research` → `sdd:3-plan` → `sdd:4-validate` →
  `sdd:6-verify` skills using its own tool access before reaching `/backlog/review` — not a
  cosmetic mode-name swap (the exact risk `backlog-configurable-pipeline/requirements.md`'s
  "Rabbit Holes" section flagged).
- New backlog items can be made to default to `PipelineMode == "sdd"` instead of `""`, gated behind
  an off-by-default feature flag so the operator opts in deliberately rather than every future item
  silently changing behavior the moment this ships.
- Existing items, and any item with an already-explicit `pipeline_mode` value (including explicit
  `""`), are completely unaffected — the default-selection logic only ever applies to a brand-new
  item at the moment of creation, never to an existing row.
- `BacklogItemForm.tsx` still lets an operator override the default back to the flat pipeline (or to
  any other mode) on any single item — the escape hatch the task explicitly required stays intact
  (it already existed; this project must not regress it).

## Scope

### In Scope

- A new DB-persisted `PipelineMode` row (slug `sdd`), seeded idempotently (create-if-missing, never
  overwrites an operator's later hand-edits) at server boot, using the existing `PipelineModeRepository`
  and `ValidatePipelineModeContent`.
- Content for all 9 template fields, designed around the existing placeholder allow-list
  (`item_id`, `item_title`, `item_description`, `repo_path`, `criteria_index`, `criteria_count`,
  `criteria_text`) and the existing structural-integrity validator (no shell metacharacters:
  backtick, `$(`, `;`, `|`, `&&`).
- A new, off-by-default feature flag (`backlog:sdd-default-pipeline`) using the existing
  `knownFeatureFlags`/`GetFeatureFlags`/`UpdateFeatureFlag` mechanism (no new RPC/proto needed — it's
  a plain data-only flag, no `FeatureController`).
- Frontend: `BacklogItemForm.tsx` pre-selects `sdd` as the initial `pipelineMode` value for a
  **new** item (never for editing an existing item) when the flag is on and an enabled `sdd` mode is
  present in `listPipelineModes()` — the user can still change it before submitting, same as any
  other field default.
- Backend defense-in-depth: `CreateBacklogItem` defaults `PipelineMode` to `sdd` when the flag is on
  **and** the request's `pipeline_mode` field is genuinely absent (`nil` pointer, not explicit `""`)
  — covers any future non-UI caller (the debug seed handler, scripts, etc.), not just the form.
- `/settings/features` gets a friendlier label for the new flag (existing generic page, one
  `FEATURE_META` entry).
- Tests: Go unit tests for the seed function (create-if-missing, idempotent, validates own content,
  survives an operator hand-edit), for `CreateBacklogItem`'s new default-resolution branch (flag
  on/off, field present/absent), and frontend tests for the form's pre-selection behavior
  (new item vs. edit, flag on/off, mode present/absent).

### Out of Scope (explicitly deferred, with reasons)

- **Flipping the default unconditionally** (no toggle) — rejected per the task's own stated
  allowance and the Risk Control precedent `backlog-configurable-pipeline/requirements.md` itself
  established ("land the seam as a no-op-by-construction commit... before adding any second mode").
  This is a live service with real concurrent items; the SDD mode is materially heavier
  (multi-phase, longer-running, more tool calls) than today's flat pipeline and has not yet been
  observed running for real. See "Alternatives Considered" below for the full reasoning.
- **Fixing the `artifactAbsPath`/diff/extras placeholder-drop gap in `CachingPipelineEngine`** — real,
  confirmed, and pre-existing for every custom mode (not sdd-specific), but fixing it means changing
  `pipeline_engine.go`'s render call sites and `recognizedPlaceholders`, a shared, heavily-documented
  seam with its own extensive test suite. Worked around here (SDD content designed to gather its own
  evidence via tool access rather than depend on the missing placeholders) rather than fixed, per the
  "don't over-build" instruction. Flagged as a follow-up story.
- **Making `WriteBacklogContextFile`'s `.backlog-context.md` fallback pipeline-mode-aware** — it always
  calls `BuildSessionInitialPrompt` directly regardless of pipeline mode (confirmed in
  `session/backlog_commands.go`), so an sdd-mode item's context-compaction-recovery fallback shows
  the default pipeline's task protocol, not the SDD one. Deferred: the fallback protocol (done-N /
  review / ship) is still procedurally compatible with the SDD mode's own slash commands, so this is
  a UX/content gap, not a correctness break.
- Any change to `PipelineEngine`/`WorkflowEngine` control-flow, phase-tracking state, or new DB
  schema — per the task's own steer, prompt content instructing the spawned session to run the SDD
  skills itself is sufficient; the existing seam does not need new phase-tracking machinery.
- Reworking `AutonomousDriver`'s hardcoded orchestration — out of scope in the parent project and
  still out of scope here.

## Alternatives Considered

- **Unconditional default flip (no flag)** — the task explicitly permitted this if no strong reason
  existed not to. Rejected here: (1) explicit constraint not to destabilize a live service with
  concurrent real items; (2) the SDD pipeline is a genuinely heavier, longer-running, more expensive
  runtime path than today's default, un-observed in production; (3) this exact repo's own prior
  practice for this exact seam (the parent project's Risk Control section) is to ship
  behavior-preserving first and prove a second mode varies behavior deliberately, not by surprise
  default. The flag makes this reversible in one click without a deploy if it goes wrong, which an
  unconditional flip would not.
- **A general "default pipeline mode" config field (any slug, not just sdd) instead of a boolean
  flag** — more flexible, but bigger surface (new RPC or reuse of an under-specified mechanism) for
  a single-operator tool that only needs one on/off decision today. The existing `knownFeatureFlags`
  boolean mechanism already exists, is tested, and is exactly proportioned to "opt in to the one new
  mode I just built." Rejected as unnecessary generality; noted as a natural follow-up if a second
  candidate default mode ever appears.

## Non-functional Requirements

- No proto changes, no ent schema/migration changes — every message and field this needs
  (`PipelineMode`, `FeatureFlag`, `CreateBacklogItemRequest.pipeline_mode`) already exists.
- Seeding must never fail server boot (matches `NewPipelineEngine`'s own non-fatal philosophy) — log
  and continue on any seed error.
- Seed content must pass `ValidatePipelineModeContent` — verified by a test, not just by inspection.

## Observability

Seeding logs at Info on successful creation, and reuses the existing `[PipelineEngine]` log prefix
convention. No new metrics needed (single-operator tool, matches parent project's posture).

## Risk Control

The seed is **create-if-missing only** — it never overwrites an existing `sdd` row, so an operator's
later hand-edit via `/settings/pipeline-modes` survives every subsequent server restart. The feature
flag defaults to **off**, so shipping this PR changes zero live-item behavior until the operator
explicitly flips it.
