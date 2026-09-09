# ADR-001: `LivenessEngine` as a Sibling Interface, Not a `WorkflowEngine` Extension

**Status**: Accepted
**Date**: 2026-09-03
**Deciders**: Tyler Stapler (via SDD Phase 3 planning, `backlog-custom-workflow-stages`)
**Related**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md` (ADR-013)

---

## Context

The motivating bug (`docs/tasks/backlog-feature-improvement.md`, 2026-09-03 update) is that the
"idea" stage's headless-triage staleness threshold (`maxHeadlessTriageSessionStaleness`, 35m) and
call budget (`triageCallBudget`, 30m) are flat, pipeline-mode-blind Go constants — sdd-mode triage
needs a larger budget than default-mode triage, and the mismatch reliably times out sdd-mode items
(12 parked in `STUCK_REASON_ORPHANED_TRIAGE` as of this writing). `session/workflow_engine.go`'s
`WorkflowEngine` interface (`CanTransition`, `ValidateGates`, `AllowedTransitions`) has no liveness
concept at all — Feasibility Risks flagged this as "designed for status-transition legality only...
extending it needs its own design pass, not a mechanical 4th method."

`research/architecture.md` §3a surveyed the two options: add a liveness method directly to
`WorkflowEngine`, or introduce a new sibling interface — the same shape decision this codebase
already made once for `PipelineEngine` vs. `WorkflowEngine` (see `session/pipeline_engine.go`'s
package doc comment, which explicitly argues disjoint consumer sets and disjoint reasons to change
justify keeping the two separate rather than coupling them).

## Decision

Introduce `session.LivenessEngine` as a new, independent interface:

```go
type LivenessEngine interface {
    LivenessFor(stage BacklogStatus, mode PipelineMode) (LivenessDefinition, error)
}
```

`DefaultLivenessEngine` (in-memory, reproduces every hardcoded constant) and `CachingLivenessEngine`
(DB-backed, cached, fail-closed) are its two implementations, held as an independent field on
`BacklogService` and `BacklogLifecycleListener` — constructed and wired the same way `pipelineEngine`
already is, never as a method of `WorkflowEngine`.

`WorkflowEngine` itself is touched only by the separate gate decision (ADR-002) — no liveness method
is added to it.

## Rationale

- **Disjoint consumers**: `LivenessEngine` is consulted by the periodic `reconcile*` stuck-detection
  sweeps (`session/backlog_lifecycle_stale.go`, `backlog_lifecycle_triage.go`, and — per Story 1.4.3 —
  `reconcileBouncingItems` in `session/backlog_lifecycle.go`) and by `TriggerTriage`'s call-budget
  selection (`server/services/backlog_service_triage.go`).
  `WorkflowEngine.CanTransition`/`ValidateGates` are consulted by the synchronous
  `TransitionBacklogItemStatus` request path. Different call sites, different triggering clocks (a
  ~60s periodic tick vs. a synchronous RPC).
- **Independent evolution**: nothing in `CanTransition` needs a liveness value; nothing in liveness
  resolution needs to know whether a transition is legal. `research/architecture.md` §3a confirms no
  cross-calls are needed in either direction.
- **Precedent already validated once**: this is the exact argument that already justified keeping
  `PipelineEngine` separate from `WorkflowEngine`. Reapplying it here, rather than re-deriving a
  different answer, keeps the codebase's interface-boundary reasoning consistent.

## Alternatives Considered

- **Add a 4th method to `WorkflowEngine`** — rejected: would couple two disjoint consumer sets and
  contradict the already-established `PipelineEngine`/`WorkflowEngine` separation precedent for no
  benefit; flagged explicitly as the wrong move by Feasibility Risks' own framing ("not a mechanical
  4th method").
- **A single `PolicyEngine` facade wrapping `WorkflowEngine`+`PipelineEngine`+`LivenessEngine`** —
  rejected: no independent consumer needs the facade; every existing and new caller already knows
  which specific interface it needs. A facade here would be exactly the interface-pollution
  `.claude/rules/interface-pollution-checklist.md` warns against — added indirection with no
  disjoint-consumer justification.

## Consequences

**Positive**: `LivenessEngine` can evolve (new `LivenessKind` variants, a future finer-grained axis)
without touching `WorkflowEngine`'s already-stable, well-tested transition-legality code.
`DefaultLivenessEngine` gives a provable zero-regression baseline (Milestone 1's characterization
tests assert against it directly).

**Negative**: one more field to wire through `server/dependencies.go`, `BacklogService`, and
`BacklogLifecycleListener` — matches the existing `pipelineEngine` wiring cost exactly, so no new
wiring *pattern* is introduced, just one more instance of the existing one.

**Neutral**: `LivenessEngine` takes a `PipelineMode` as a read-only input parameter but never calls
into `PipelineEngine` — it only needs the mode's identity (a string) for its sparse-override lookup
key, not any content-resolution behavior, keeping the two interfaces from developing an implicit
coupling through shared internal calls.

---

See `docs/adr/013-workflow-engine-replaces-valid-transitions.md` for the original `WorkflowEngine`
seam this ADR extends without modifying, and `ADR-002-configured-workflow-engine-and-gates.md` for
the sibling decision on how transition gates *do* extend `WorkflowEngine` (a materially different
answer than this ADR's for liveness, for a materially different reason).
