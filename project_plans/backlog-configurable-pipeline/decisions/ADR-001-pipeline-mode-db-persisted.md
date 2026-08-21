# ADR-001: PipelineMode is DB-Persisted and User-Definable at Runtime

**Status**: Accepted, contingent on the Phase 0 spike (see `implementation/plan.md`'s Risk Control
section) — this decision proceeds to Phase 1 implementation only if the pre-Phase-1 hand-authored
spike shows meaningfully lower friction than editing Go source directly. If the spike fails its
binary gate, this ADR's decision is reversed and the DB-persisted approach is not built.
**Date**: 2026-07-15
**Deciders**: repo owner (single-operator tool), recorded via `backlog-configurable-pipeline` SDD project

## Context

The backlog feature's pipeline (triage → plan → implement → review → merge) is currently fixed:
every item gets the identical slash-command set (`session/backlog_commands.go:20`,
`WriteSlashCommands`) and the identical fixed prompt-building functions
(`BuildHeadlessTriagePrompt`, `BuildHeadlessReviewPrompt`, `BuildSessionInitialPrompt`). The only
per-item variation is three independent boolean short-circuits (`SkipReviewGate`, `SkipPlanning`,
`AutoSpawnSession`) that skip a stage — none let a user substitute a different skill/command set.

Phase 2 research (`project_plans/backlog-configurable-pipeline/research/*.md`) evaluated how a new
`PipelineEngine` seam should be backed. It initially leaned toward a small, closed, code-defined
registry (`map[string]PipelineEngine`, mirrored on `session/backlog_plugin.go`'s
`PluginRegistry`) for two concrete reasons:

1. **Security**: content that flows into LLM prompts and into files written by
   `WriteSlashCommands` is a meaningfully larger injection surface when it is DB-editable at
   runtime than when it is a closed, PR-reviewed set of Go functions.
2. **Precedent**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md` explicitly
   considered and rejected "load config from DB on every call" (its "Alt B") for the sibling
   `WorkflowEngine` concept — no caching strategy, O(n) DB reads on a hot path — and shipped only
   a static `DefaultWorkflowEngine`, deferring DB-persisted custom config to a never-built
   "Phase 2."

## Decision

**`PipelineMode` definitions are persisted in a new ent-backed database table and are creatable,
editable, and deletable by the operator at runtime through the UI — not a closed Go-code
registry.**

This reverses Phase 2 research's initial lean. The reversal was made deliberately, during Phase 3
planning kickoff, for the following reasons:

1. **This is a single-operator internal tool.** The person who would define a pipeline mode
   through the UI is the same person who could otherwise edit `WriteSlashCommands`'s Go source and
   redeploy via `make install-service`. There is no untrusted third party the operator needs
   protection from — the multi-tenant injection threat model the initial research applied does not
   hold here. The residual risk is **self-inflicted breakage** (a malformed mode definition
   producing a broken prompt or an invalid slash-command file), which is a correctness/robustness
   concern, not a security boundary.
2. **Live, shipped precedent already exists in this exact codebase.** `session.Workflow` /
   `WorkflowRepository` (`session/workflow_repository.go`, `session/ent_workflow_repository.go`)
   is already a DB-persisted, slug-addressed, user-creatable named preset driving the omnibar's
   `@workflow-slug` detector today. "Runtime-configurable named presets" is proven, shipped
   infrastructure in a sibling subsystem of this same repo, not a novel bet.
3. **The feature's stated goal is explicitly about reducing engineering involvement.** A
   code-registry still requires a PR and a deploy for every new mode. That only partially satisfies
   "a user can say 'use X for this item'" if "user" is assumed to mean "an engineer who can ship a
   PR" — which the requirements' Success Metrics explicitly reject ("Adding a new pipeline mode
   requires no engineering involvement... not a code deploy").

## What This Does Not Reopen

- `WorkflowEngine` / `docs/adr/013-workflow-engine-replaces-valid-transitions.md`'s Phase 2
  (`ConfiguredWorkflowEngine`, custom **states**) stays out of scope, unchanged. This decision is
  scoped entirely to `PipelineEngine` (which skills/prompts run *within* a status) — a different
  seam from `WorkflowEngine` (which status transitions are *legal*). The two are not merged; see
  `session/pipeline_engine.go` (new) vs. `session/workflow_engine.go` (unchanged) in the
  implementation plan.
- This is **not** "arbitrary free-text appended to every prompt." Mode definitions remain
  structured DB rows — a slug, name, description, enabled flag, and a small fixed set of typed
  content-template columns (one per slash-command file, one per prompt) — not a single
  unstructured text box. Structure is what keeps this "configuration," not "code injection," even
  under the relaxed single-operator threat model above. See the Migration Plan in
  `implementation/plan.md` for the exact column list.
- Content-template fields support only fixed-placeholder string substitution (e.g. `{{item_id}}`),
  never a general-purpose templating DSL and never string-interpolation into a shell/command-line
  context — only into prompt/markdown-file contexts. This boundary is a hard rule regardless of
  the relaxed trust model and is enforced at the point templates are rendered
  (`session/pipeline_engine.go`, `renderTemplate`).

## Consequences

**New engineering cost this decision introduces** (absent in the rejected code-registry
alternative):

1. An ent schema + migration for a `PipelineMode` table (`session/ent/schema/pipeline_mode.go`),
   mirroring `session.Workflow`'s slug/name/description/enabled shape.
2. **A caching strategy `WorkflowRepository` does not provide.** Confirmed by direct inspection:
   every `WorkflowRepository` method (`GetBySlug`, `ListEnabled`, etc.) does a direct, uncached ent
   query — acceptable for the omnibar (human keystrokes, low frequency) but not acceptable for
   `PipelineEngine`, whose consumers (`WriteSlashCommands`, `TriggerTriage`,
   `ReviewGateRunner.Run`) are hot paths. This ADR requires an explicit in-process cache
   (`session/pipeline_engine.go`, `pipelineModeCache`, an `atomic.Pointer[map[string]...]`
   copy-on-write structure) invalidated on every write RPC, rather than a query-per-call pattern.
   Reads (`Get`) stay fully lock-free via the atomic pointer load. Writes (`Invalidate`) are
   additionally serialized behind a `sync.Mutex` guarding only the DB-read + pointer-swap sequence
   — added during Phase 3 planning's adversarial review after it found that two concurrent
   `Invalidate` calls (e.g. a double-submitted edit) could otherwise race such that a slower/older
   read's `Store` lands after a faster/newer one, leaving the cache stuck on stale data (a
   lost-update, not a torn read — `atomic.Pointer` alone does not prevent it). See
   `implementation/plan.md`'s Pattern Decisions table and Story 1.3.2/Task 1.3.2b/1.3.2d.
3. A CRUD management UI surface (`/settings/pipeline-modes`) beyond the item-level selector alone.

**Tripwire on the relaxed threat model**: the "no untrusted third party" premise above holds only
while stapler-squad is a single-operator instance. If this tool is ever deployed for concurrent
multi-user access (not just multi-repo/multi-remote for one operator — this repo already has a
dual-remote setup per `CLAUDE.md`, which is a different thing), `PipelineMode` CRUD must gain
per-user ownership/authorization before that deployment — one user's mode content currently drives
another user's triage/review LLM calls and file writes with no access control and no audit trail
beyond a Warn-level log line. Re-evaluate this ADR's threat model explicitly at that point; do not
carry the relaxed assumption forward silently. (Flagged by Phase 4's pre-mortem, P3 — unlikely
under current usage, catastrophic if it silently applies to a context it was never evaluated
against.)

**Mitigation for the self-inflicted-breakage risk accepted above**: an item whose stored
`pipeline_mode` slug fails to resolve (deleted mode, or a slug that was never valid) falls back to
the built-in default pipeline behavior and logs at Warn — never a silent no-op, never a crash. The
empty-string mode (`PipelineModeDefault = ""`) additionally bypasses the cache/repository entirely
and calls the pre-existing hardcoded functions directly, so the overwhelmingly common case (items
that never touch this field) has zero new DB/cache dependency in its call path. See
`implementation/plan.md` Phase 1, Epic 1.3.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Closed Go-code registry (`map[string]PipelineEngine`), mirroring `PluginRegistry` | Requires a PR + deploy per new mode — fails the stated goal of no-engineering-involvement for defining a new pipeline. Phase 2 research's initial recommendation; reversed by this ADR. |
| Free-text "custom instructions" field appended to every prompt | No structure to validate or display ("what ran"); fails predictably-on-error worse than typed fields; rejected in requirements.md even under the relaxed single-operator threat model. |
| Raw string slash-command name picked by the user (e.g. type `/sdd:full` into a text field) | No validation surface, silent-no-op-on-typo risk, and doesn't compose with `WriteSlashCommands`'s fixed-set generation without effectively becoming a registry lookup anyway. |
| DB-persisted, but query-per-call like `WorkflowRepository` (no cache) | Violates the NFR that pipeline-mode resolution must not add an uncached synchronous DB read to hot paths (`TriggerTriage`, `WriteSlashCommands`, `ReviewGateRunner.Run`); directly the "Alt B" pattern ADR-013 already rejected once for the sibling `WorkflowEngine`. |

## References

- `project_plans/backlog-configurable-pipeline/requirements.md` — "Runtime Configurability
  Decision (resolved 2026-07-15)"
- `project_plans/backlog-configurable-pipeline/research/build-vs-buy.md`
- `project_plans/backlog-configurable-pipeline/research/pitfalls.md` §2–3
- `docs/adr/013-workflow-engine-replaces-valid-transitions.md`
- `session/workflow_repository.go`, `session/ent_workflow_repository.go` (`session.Workflow`)
- `project_plans/backlog-configurable-pipeline/implementation/plan.md` (this decision's
  implementation)
