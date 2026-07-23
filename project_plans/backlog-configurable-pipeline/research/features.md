# Research: Feature Landscape for Per-Item Configurable Pipeline

## 1. Analogous "N modes, one selected" patterns already in this codebase

### 1a. `DetectorRegistry` (`web-app/src/lib/omnibar/detector.ts`)
A priority-sorted list of `Detector` implementations (`GitHubPRDetector` priority 10 ... `SessionSearchDetector`
priority 200 as catch-all fallback). Each detector implements `{ name, priority, detect() }`; the registry sorts by
priority ascending and the first non-null `detect()` result wins (`detector.ts:310-316`). This is a **stateless,
compile-time, ordered-strategy** pattern — good prior art for "first matching mode wins," but it answers *auto
selection from input*, not *explicit user choice of mode*, so it's a partial analog only.

### 1b. `OmnibarAction` discriminated union (`web-app/src/lib/omnibar/actions/types.ts` + `dispatch.ts`)
A closed `type OmnibarAction = {type:"navigate_session";...} | {type:"create_session";...} | ...` union with an
exhaustive `switch (action.type)` in `dispatch.ts`. Missing a case is a **compile error** — this is the actual
architectural guard the requirements should borrow for `PipelineEngine` mode dispatch if modes are a closed,
code-defined set (`"default" | "quick" | "full"`). This directly supports Open Question 1's "Go-code-defined
registry of named modes" framing over full DB-runtime-configurability, *if* the mode set stays small and rarely
changes.

### 1c. `session.Workflow` — a DB-persisted "named preset" already exists (previously unknown to requirements)
`session/workflow_repository.go` defines `WorkflowRepository` (`Create/Update/Delete/GetBySlug/ListAll/ListEnabled`)
and `WorkflowCreateInput`/`WorkflowUpdateInput` with fields: `Slug, Name, Description, Command, TargetDirectory,
InputTemplate, SessionType, Model, AgentType, CronExpression, CronEnabled, KeepSessions, ArchiveAfterHours`. This is
the omnibar's "saved/scheduled workflow" feature (`@workflow-slug` detector, priority 25, dynamically registered —
see `session-creation-registry.md`'s touchpoint table). It is **exactly** the shape of thing Open Question 1 is
asking whether to build: a DB-persisted, slug-addressed, user-creatable named preset that maps to a concrete session
configuration (command + session type + model + agent type). It is a strong existing precedent that this repo
already has infrastructure for "named mode → concrete config" that is runtime/DB-configurable, not just a Go-code
registry — worth citing directly when answering Open Question 1.

**Naming collision risk**: `session.Workflow` (this DB-backed omnibar preset entity) and `session.WorkflowEngine`
(ADR-013's backlog state-transition policy interface, `session/workflow_engine.go`) are unrelated concepts that
share the "workflow" name. Requirements language ("pipeline mode might legitimately want to add/skip a status")
already risks conflating these three: `PipelineEngine` (new), `WorkflowEngine` (backlog status transitions),
`Workflow` (omnibar saved command presets). Recommend the plan phase pick a name for the new type that doesn't
overload "workflow" further — e.g. `PipelineEngine`/`PipelineMode` as already used in the requirements doc, kept
strictly distinct from both existing types.

### 1d. `WorkflowEngine` (ADR-013) — closest prior art for a pluggable policy engine
`session/workflow_engine.go` (64 lines) defines a narrow interface:
```go
type WorkflowEngine interface {
    CanTransition(from, to BacklogStatus) bool
    ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error
    AllowedTransitions(from BacklogStatus) []BacklogStatus
}
```
`DefaultWorkflowEngine` wraps the pre-existing `validTransitions` map + `TransitionGuard` function 1:1, reproducing
current behavior exactly, and is injected once at server startup (`server/dependencies.go`). This is the **template
to copy** for `PipelineEngine`/`DefaultPipelineEngine`, per requirements' own instruction. Key details worth
preserving from the ADR:
- The interface is deliberately narrow — only methods the actual consumers need (`BacklogService`,
  `BacklogLifecycleListener`), not a kitchen-sink interface.
- ADR-013 explicitly deferred a `ConfiguredWorkflowEngine` (DB-persisted, custom states) to "Phase 2" and shipped
  only `DefaultWorkflowEngine` in "Phase 1," keeping `validTransitions`/`TransitionGuard` as-is underneath rather
  than deleting them — i.e., the wrap-don't-replace strategy. This is the same strategy the requirements document
  proposes for `PipelineEngine`, and it succeeded once already in this codebase — reuse the "keep the old
  functions, wrap them in Phase 1, only replace in an explicit Phase 2" sequencing.
- ADR-013's "Alt B" (load config on every transition call → O(n) DB read per list op) was explicitly rejected due to
  a caching pitfall found in research. If `PipelineEngine` becomes DB-persisted per Open Question 1, the plan phase
  must address this same caching concern (mode lookups happen inside `WriteSlashCommands`, prompt builders, and the
  review-gate runner — all potentially hot paths per backlog item).

### 1e. No "profile"/"preset"/"template" concept found for pipelines specifically
Beyond `session.Workflow` (1c, an omnibar/session-creation concept, not a backlog-pipeline concept), grep across
`session/*.go` and `server/services/*.go` found no existing profile/preset/template abstraction scoped to backlog
pipeline stages. `PipelineEngine` would be new territory for the backlog subsystem specifically.

## 2. What `/sdd:quick` and `/sdd:full` actually differ by today

Read `~/dotfiles/.claude/skills/sdd/skills/quick/SKILL.md` and `.../full/SKILL.md` directly (both exist and are
accessible). The difference is **not** "same 7 phases, different subset toggled on/off." It's structurally different:

- **`sdd:quick`** (86 lines): a single linear 6-step flow — clarify → read before touching → plan inline (no file
  written) → implement → verify (tests must show green output before any completion claim) → output summary. It
  explicitly has **no artifacts, no phase gates, no fresh-session requirement**. Scoped for "bug fixes (1-3 files),"
  "small well-scoped tasks," explicitly excluding "new services, multi-epic features, >5 files, anything requiring
  architecture decisions."
- **`sdd:full`** (149 lines): a **pure orchestrator** that never duplicates phase logic — each of its 7 named
  phases (`1-ideate` → `2-research` → `3-plan` → `4-validate` → checkpoint-commit → `5-implement` → `6-verify` →
  `7-ship`) is executed by reading and following the corresponding standalone phase file
  (`.claude/commands/sdd/N-*.md`). It writes durable artifacts to `project_plans/<project>/` at every phase, uses
  parallel `Agent` dispatch for research/planning/verification, requires a **fresh session before Phase 5**, and has
  explicit phase gates (user confirmation between phases, "if pre-mortem/triad review fails, halt").

**Direct answer to Open Question 3** ("different fixed command set per mode, or different subset of skills within
the same command set?"): the real precedent is **a different fixed command/skill set entirely**, not a shared
skeleton with stages toggled. `sdd:quick` doesn't have "phases" at all in the `sdd:full` sense — it's a fundamentally
different, shorter control flow, not `sdd:full` with phases 1-4 skipped. If `PipelineEngine` modes are meant to
mirror this precedent, a "quick" pipeline mode for a backlog item should mean *substituting a different, self-
contained prompt/skill-set* for that item's triage/implement/review stages — not flipping subset flags within one
shared prompt template. This has a direct consequence for the "Rabbit Hole" the requirements flagged about
`BuildHeadlessTriagePrompt`/`BuildReviewPrompt` risking a full templating engine: the `sdd:quick` vs `sdd:full`
precedent suggests **mode-specific whole-prompt variants** (a small closed set, like `DefaultPipelineEngine` +
1-2 named alternatives producing entirely different prompt bodies) are more faithful to existing practice than a
single parameterized template with conditional sections — which avoids the templating-engine rabbit hole by keeping
each mode's prompt construction as its own simple function, mirroring `BuildReviewPrompt` vs
`BuildHeadlessReviewPrompt` already being separate functions rather than one parameterized one.

## 3. Edge cases and failure modes a configurable-pipeline design must handle

- **Mode deleted/renamed after items already reference it**: `session.Workflow` (1c) is the only existing precedent
  for a deletable named preset in this codebase, and its `WorkflowRepository.Delete` has no visible referential
  integrity check against past usage (not verified in depth — flag for the planning phase). If `PipelineEngine`
  modes are a small Go-code-defined registry (`"default","quick","full"`) rather than DB rows, deletion/rename is a
  code-deploy event, not a runtime one — recommend treating unknown mode strings as a **hard fallback to
  `"default"` with a logged warning**, not an error, so historical items with a mode value no longer present in the
  registry don't hard-fail `WriteSlashCommands` or prompt construction (which run on every session resume, not just
  creation).
- **Mode vs. `SkipReviewGate`/`SkipPlanning`/`AutoSpawnSession` composition**: today these three booleans are
  independent, ad hoc AND-conditions scattered across three files with no central coordination:
  - `backlog_lifecycle.go:395` — `toStatus := BacklogStatusReview; if item.SkipReviewGate { toStatus =
    BacklogStatusDone }`
  - `backlog_lifecycle.go:419` — `toStatus == BacklogStatusReview && !item.SkipReviewGate && (...)"` gates whether a
    review-gate session is spawned
  - `review_gate.go:60-61` — a second, independent short-circuit on `SkipReviewGate`
  - `backlog_service_triage.go:216` — `!isReopen && !item.SkipPlanning && !item.PlanApproved && !req.Msg.Autonomous`
    gates whether planning is required

  Nothing today enforces mutual consistency between these flags — an item could already have contradictory-looking
  states (e.g. `SkipPlanning=true` with a status that implies planning happened). Adding `PipelineMode` as a fourth,
  independent axis compounds this: a `mode="full"` item with `SkipPlanning=true` is a real possible contradiction the
  design must resolve. Two options, both viable given precedent:
  1. **Mode subsumes the booleans** — `PipelineEngine` for a given mode determines `SkipReviewGate`/`SkipPlanning`
     internally, and the three DB boolean fields become either read-only derived views or are deprecated in favor of
     `AllowedTransitions`-style engine queries (mirrors ADR-013's `AllowedTransitions` eliminating the frontend's
     duplicate `STATUS_TRANSITIONS` table — same "engine is the single source of truth" pattern).
  2. **Mode composes with the booleans** — mode selects the skill/command set; the three booleans remain independent
     stage on/off switches layered on top. Simpler to ship incrementally (matches "Start with `DefaultPipelineEngine`
     reproducing today's behavior exactly" instruction) but perpetuates the existing scattered-condition pattern
     ADR-013 was written to move away from.

  Recommend the plan phase make this an explicit ADR decision point — the requirements' own Open Question 2
  ("immutable at creation or changeable per-transition, e.g. escalate quick→full after failed review") interacts
  directly with this: if mode is changeable mid-flight, option 2 (compose) is materially simpler to reason about
  than option 1 (subsume), since subsuming would require re-deriving/rewriting the boolean fields on every mode
  change.

- **`autonomous_driver.go` being out of scope makes the seam cosmetic**: confirmed real. `autonomous_driver.go:336-
  341` contains `autonomousSystemPrompt` (a package-level `const` string) and `buildOrchestrationPrompt`, which are
  the actual runtime orchestration logic for autonomous-mode items — entirely hardcoded, no seam, and explicitly out
  of scope per the requirements. Since `WriteSlashCommands` only writes **files** the agent *may* read
  (`.claude/commands/backlog/*.md`), and the actual autonomous orchestration loop (`autonomous_driver.go`) doesn't
  consult those files at all (it drives via `NEXT_MESSAGE`/`DONE` signals against session output, not slash
  commands), a `PipelineEngine` mode selection for an *autonomous* item would indeed be cosmetic — it would change
  which markdown files exist in the worktree without changing what the orchestrator LLM actually does. This
  confirms the risk the requirements author already flagged and suggests the plan phase should scope
  `PipelineEngine`'s real behavioral effect to **non-autonomous** (human/headless-triage-driven) items only, and
  explicitly document that autonomous-mode items get mode selection as a no-op/cosmetic label until
  `autonomous_driver.go` is addressed in a future initiative.

## 4. Unstated needs: preview/dry-run visibility

No existing dry-run, preview, or "what will run" concept exists anywhere in `session/` or `server/services/`
(confirmed via grep — the only "preview" hits are unrelated log-truncation variables in `backlog_triage.go`). This
is a genuine gap relative to the stated goal: a user choosing `/sdd:full` for an item is making a resource/time
trade-off (7 phases with fresh-session gates vs. one linear pass), and per the requirements' own scope item ("UI:
... read-only 'what ran' surface in `BacklogItemDetail.tsx`") there is already a planned *retrospective* surface
("what ran"). There is no planned *prospective* surface ("what will run if I pick this mode").

Given `DefaultPipelineEngine` will need to expose enough structure to drive `WriteSlashCommands` deterministically
per mode, exposing that same structure as a **static, no-network-call preview** (e.g. "quick mode skips: planning
gate, review gate; runs: triage → implement → ship") is low-incremental-cost if `PipelineEngine` is a small
Go-code-defined registry (per section 1b/1c reasoning) — it's just rendering the mode's static metadata, not an
actual dry-run execution. This is worth a line item in the plan's scope, distinct from the "what ran" (retrospective,
already in scope) — call it "what will run" (prospective, currently unstated). Recommend flagging this as a
low-cost scope addition rather than a new dry-run *execution* concept (which would risk the ADR-013 Alt-B-style
scope creep the requirements' Rabbit Holes section is trying to avoid).

## Key files referenced
- `web-app/src/lib/omnibar/detector.ts` (`DetectorRegistry`, priority-sorted strategy list)
- `web-app/src/lib/omnibar/actions/types.ts`, `dispatch.ts` (`OmnibarAction` exhaustive union)
- `session/workflow_engine.go`, `docs/adr/013-workflow-engine-replaces-valid-transitions.md` (closest prior art)
- `session/workflow_repository.go`, `session/ent_workflow_repository.go` (`session.Workflow` DB-persisted preset —
  previously unknown analog, naming-collision risk with `WorkflowEngine`)
- `~/dotfiles/.claude/skills/sdd/skills/quick/SKILL.md`, `.../full/SKILL.md` (answers Open Question 3)
- `session/backlog_commands.go` (`WriteSlashCommands`, `backlogCommandsDir`)
- `session/backlog_lifecycle.go:395,419,464`, `session/review_gate.go:60-61`,
  `server/services/backlog_service_triage.go:216` (existing scattered boolean-flag composition)
- `session/autonomous_driver.go:320-341` (`autonomousSystemPrompt`, `buildOrchestrationPrompt` — confirmed
  cosmetic-seam risk)
- `session/backlog_context.go`, `session/backlog_review.go`, `session/backlog_triage.go` (existing separate
  prompt-builder functions per stage/mode — precedent against a shared templating engine)
- `proto/session/v1/backlog.proto:117,157,196`, `session/ent/schema/backlog_item.go:43` (`auto_spawn_session` field
  pattern to mirror for new proto/schema fields)
