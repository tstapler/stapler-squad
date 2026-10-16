# Architecture Review: backlog-custom-workflow-stages
**Date**: 2026-09-03
**Verdict**: CONCERNS (superseded 2026-09-03 — all 4 concerns below were subsequently addressed in
plan.md during Phase 4's Product Triad Review repair loop: duplicate fallback ownership, gate-config
validation timing, the StageSlug/BacklogStatus type-boundary decision, and the graph-validate/persist
transaction. See plan.md's Domain Glossary, Epic 2.1's Decision subsection, and Epic 2.7 for the
resulting design. Left this file's original findings unedited below as the historical record.)

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (confirmed via
file check). No constitution-derived hard constraints apply — skipping to the three-lens review.

## Phase 1 / Phase 2 Independence Check

Cross-referenced every file listed under Phase 1 (Epics 1.1–1.6: `session/liveness_definition.go`,
`session/liveness_engine.go`, `session/liveness_cache.go`, `session/liveness_repository.go`,
`session/ent_liveness_repository.go`, `session/ent/schema/stage_liveness_definition.go`,
`server/services/backlog_service_liveness.go`, `server/dependencies.go` (liveness wiring),
`session/backlog_lifecycle_triage.go`, `session/backlog_lifecycle.go` (liveness field only),
`server/services/backlog_service_triage.go`, `server/services/backlog_service.go`,
`session/backlog_remediation_test.go`) against every Phase 2 type/file (`ConfiguredWorkflowEngine`,
`GateStatus`/`GateKind` (`session/gate_status.go`), `backlog_stage`/`stage_transition`/
`transition_gate`/`gate_satisfaction_record` ent schemas, `session/graph_validator.go`). **No Phase 1
file references any Phase 2 type or file** — the claim holds structurally. The only shared file is
`proto/session/v1/backlog.proto`, edited by both phases for unrelated message additions (Liveness
CRUD in Phase 1; Stage/Transition/Gate CRUD in Phase 2) — a rebase-sequencing note, not a dependency
(see Nitpicks).

One forward-compatibility gap is real, however: `LivenessEngine.LivenessFor(stage BacklogStatus, mode
PipelineMode)` (ADR-001) and `WorkflowEngine.PendingGates(item, to BacklogStatus)` (ADR-002) are typed
to the closed, 9-value `BacklogStatus` type, but Epic 2.4.4c plans to resolve liveness for a *custom*
stage/gate through this same `LivenessEngine.LivenessFor` call. See Concern #3 below — this doesn't
violate "Phase 1 has zero Phase 2 dependency" (the arrow still points the right way), but it is an
unaddressed completeness gap in an already-`Accepted` interface signature that Phase 2 will hit at
Epic 2.3/2.4.

## Blockers

None found.

## Concerns

- [ ] **Story 1.1.2 / Story 1.3.1 — the `(stage, mode) → (stage, nil)` sparse-override fallback is
  specified twice, at two different layers, with no stated single owner.** Story 1.1.2's acceptance
  criteria states *the repository* falls back: "*When* read back via `GetByStageAndMode("idea", "sdd")`
  and no `("idea","sdd")` row exists, *Then* the repository falls back to the `("idea", nil)` row (see
  Story 1.3.1)." Story 1.3.1's acceptance criteria independently states *the cache* does this same
  fallback: "`livenessCache.Get("idea", "sdd")`... returns a miss when no `("idea","sdd")` row and no
  `("idea", nil)` row exist." If both are implemented as written, the mode-less-fallback logic exists
  in two places — exactly the duplicated-logic smell `code-architecture-best-practices`' Reuse Check
  flags as the highest-value thing to catch before it ships twice. **Remediation**: designate one owner
  before Task 1.1.2a/1.3.1c are implemented — recommend `livenessCache.Get` (matching
  `pipelineModeCache`'s precedent of doing resolution at the cache/engine layer, keeping
  `EntLivenessRepository.GetByStageAndMode` a dumb exact-match query) — and strike the fallback claim
  from Story 1.1.2's acceptance criteria.

- [ ] **Story 2.4.1–2.4.4 — `transition_gates.config` is persisted and consumed as opaque JSON with no
  typed parse-at-boundary step.** The Migration Plan defines `config (JSON — kind-specific fields)` on
  `transition_gates`, but no task in Epic 2.4 (human-approval, structural, automated-review,
  custom-check — Stories 2.4.1–2.4.4) or Epic 2.7 (the CRUD RPC handlers) introduces a single typed
  `GateConfig` parse step. As written, each of the four gate-kind evaluators plus the UI plus
  `InvokeCustomGateCheck` will independently read/validate this JSON blob against its own ad hoc
  expectations — the "validation logic repeated across service methods" primitive-obsession smell the
  `type-driven-design` lens flags directly, and structurally the same category of risk `LivenessDefinition`
  was explicitly designed to avoid one layer up (Pattern Decisions: "Category error... a flat schema"
  rejected for liveness, but the equivalent problem is left unaddressed for gate config). **Remediation**:
  add one task under Epic 2.4 (or 2.7.2) defining a `GateConfig` Go sum type — `HumanApprovalConfig`,
  `AutomatedReviewConfig{PipelineMode, RequiresDiff}`, `StructuralConfig{CheckID}`,
  `CustomCheckConfig{SkillID}` — plus `ParseGateConfig(kind GateKind, raw json.RawMessage) (GateConfig,
  error)` called once at the RPC-handler/repository boundary (Epic 2.7.2c), so `PendingGates`'s
  dispatch and `InvokeCustomGateCheck` consume an already-typed value, never raw JSON.

- [ ] **Domain Glossary — `StageSlug`'s definition contradicts this codebase's own newtype convention
  and is underspecified relative to `BacklogStatus`.** The glossary calls `StageSlug` "a Go string type
  alias documented as 'never a raw string.'" Verified via `session/domain/backlog.go:13` and
  `session/pipeline_engine.go:36`: the two closest precedents, `BacklogStatus` and `PipelineMode`, are
  both **defined types** (`type BacklogStatus string`, `type PipelineMode string`), never `type X =
  string` aliases — the `type-driven-design` skill explicitly calls a real alias "no protection," which
  is presumably not the intent here, just imprecise wording. Separately and more substantively:
  `LivenessEngine.LivenessFor` and `WorkflowEngine.PendingGates` are typed to `BacklogStatus` (ADR-001,
  ADR-002 — already `Accepted`), a type whose doc comment and 9 named constants describe the fixed
  built-in lifecycle, yet Epic 2.4.4c plans to resolve liveness for a *custom* stage/gate through this
  identical call. The plan never states whether `StageSlug` and `BacklogStatus` become the same
  underlying type going forward, or how the boundary between "the closed built-in enum" and "any
  addressable stage, built-in or custom" is drawn at the `LivenessEngine`/`WorkflowEngine` signature
  level. **Remediation**: fix the glossary wording to "defined type, not alias" and, before Epic 2.2/2.3
  implementation starts, state explicitly in the plan (or a short ADR addendum) whether `BacklogStatus`
  becomes a documented subset of `StageSlug`'s value space, or whether `LivenessFor`/`PendingGates`
  widen their parameter type to `StageSlug` in Phase 2 — this is a design decision affecting two
  already-`Accepted` interface signatures and should not be improvised inside Epic 2.3/2.4's
  implementation tasks.

- [ ] **Epic 2.6/2.7 — no transaction wraps graph validation and transition persistence, leaving a
  TOCTOU race in the exact invariant Epic 2.6 exists to guarantee.** Task 2.7.2c says the handler
  "invok[es] `graph_validator.go` on every mutating call," but nothing in Epic 2.6 or 2.7 specifies
  wrapping "validate the graph" and "persist the new transition" in one DB transaction. Two concurrent
  `CreateStageTransition` calls could each validate against the pre-mutation graph and then both commit,
  producing a graph with an unreachable stage or a gate-free-cycle that passed validation only because
  of the race — defeating Epic 2.6's whole purpose. Given the single-operator threat model this is
  low-likelihood, but it is a real correctness gap in a soundness-critical validator, not a
  hypothetical. **Remediation**: wrap validate+persist in a single ent transaction (`WithTx`) in Task
  2.7.2c's handler implementation; add one adversarial test alongside 2.6.1d exercising two
  interleaved mutating calls.

## Nitpicks

- `LivenessDefinition`'s validating constructor (Story 1.1.1) is a good fit with this codebase's
  documented house convention (`session/domain/backlog.go`'s `StuckReason` comment: "validated at the
  boundary via `IsValid`, not a truly-unrepresentable sum type") — no need to introduce a GoF-style
  interface sum type here, that would be inconsistent with precedent. But add one explicit acceptance
  criterion to Story 1.3.1 requiring `EntLivenessRepository`'s row→struct mapping route through the same
  validation path used by `NewLivenessDefinition` (not a bare struct literal built directly from ent
  columns) — otherwise a row that reached the DB some other way (a manual SQL fix, a future migration)
  could re-enter the system unvalidated on the read path even though the write path is guarded.
- `proto/session/v1/backlog.proto` is edited by both Phase 1 (Task 1.3.2a) and Phase 2 (Tasks 2.6.2b,
  2.7.1a, 2.7.2a). This is a shared-file rebase/merge-conflict risk across the two milestones' PRs, not
  an architectural coupling — just worth sequencing awareness when both phases are in flight
  concurrently.
