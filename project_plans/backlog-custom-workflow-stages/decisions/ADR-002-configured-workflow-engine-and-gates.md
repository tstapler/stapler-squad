# ADR-002: `ConfiguredWorkflowEngine` Implements ADR-013 Phase 2; Gates Extend `WorkflowEngine` via `PendingGates`

**Status**: Accepted
**Date**: 2026-09-03
**Deciders**: Tyler Stapler (via SDD Phase 3 planning, `backlog-custom-workflow-stages`)
**Related**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md` (ADR-013, Phase 2 proposal
this ADR implements), `ADR-001-liveness-engine-sibling-interface.md` (the sibling decision this ADR
deliberately does *not* repeat for gates)

---

## Context

ADR-013 proposed but never implemented `ConfiguredWorkflowEngine`: "Phase 2 custom states are a
drop-in: swap `DefaultWorkflowEngine` with `ConfiguredWorkflowEngine` via the same interface." This
project implements that proposal, adding DB-persisted custom stages and transitions
(`session/ent/schema/backlog_stage.go`, `stage_transition.go`).

Separately, requirements.md's Scope adds a **generalized transition-gate model**: any transition
(built-in or custom) can require one or more gates (human approval, automated review, structural
check, custom/pluggable check) — generalizing today's hardcoded `TransitionGuard` switch
(`session/domain/backlog.go:541-614`). This is new scope beyond ADR-013's original proposal, which
said nothing about gates as a separate configurable concern.

`research/architecture.md` §3b considered whether gates should be a sibling interface (matching
`LivenessEngine`'s pattern, ADR-001) or an extension of `WorkflowEngine` itself, and found the
opposite answer from ADR-001's: gates and `CanTransition`/`ValidateGates` share the same consumers
and the same triggering clock (a transition-attempt), so segregating them would be interface
pollution, not interface segregation.

## Decision

1. `ConfiguredWorkflowEngine` implements `WorkflowEngine` unchanged (`CanTransition`,
   `AllowedTransitions`), backed by DB-loaded `StageDefinition`/`TransitionDefinition` rows, with a
   copy-on-write cache mirroring `pipelineModeCache`. Zero call-site changes are required anywhere
   that already programs against the `WorkflowEngine` interface rather than the concrete
   `DefaultWorkflowEngine` type.
2. `WorkflowEngine` gains exactly one new method:

```go
type WorkflowEngine interface {
    CanTransition(from, to BacklogStatus) bool
    PendingGates(item BacklogItemTransitionInput, to BacklogStatus) ([]GateStatus, error)
    ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error
    AllowedTransitions(from BacklogStatus) []BacklogStatus
}
```

   `ValidateGates` becomes `len(unsatisfied PendingGates(...)) == 0` — a thin wrapper, not a
   parallel implementation. `DefaultWorkflowEngine`'s `PendingGates` returns a `GateStatus` derived
   directly from today's `TransitionGuard` switch (one structural gate per existing guard clause),
   so the built-in 9-state machine's gate behavior is unchanged.
3. Gate *initiation* (spawning a review session, creating a pending-approval UI affordance) stays
   outside both `WorkflowEngine` and `LivenessEngine`, in the same orchestration layer that already
   does this today (`ReviewGateRunner`, `server/services/backlog_service_lifecycle.go`'s transition
   handler) — no fourth interface is added for this.

## Rationale

- **Gates are the same question `ValidateGates` already answers**, just from a dynamic rule source
  instead of a hardcoded switch — this is squarely `ConfiguredWorkflowEngine`'s job, not a new
  concern needing a new interface boundary.
- **The one real gap is structured output, not a new question**: `ValidateGates` returns only
  `error`, with no per-gate "which one, who can satisfy it" data — exactly what Success Metrics
  requires for the item-detail UI. `PendingGates` closes that gap with the minimum necessary
  addition (+1 method), not a redesign.
- **`ConfiguredWorkflowEngine`'s built-in-stage output must be byte-for-byte identical to
  `DefaultWorkflowEngine`'s** (Risk Control's zero-regression requirement) — verified by a dedicated
  test in Epic 2.3, comparing every `(from,to)` pair in `domain.ValidTransitions()`.

## Alternatives Considered

- **A sibling `GateEngine` interface, mirroring `LivenessEngine`'s pattern** — rejected, for the
  opposite reason ADR-001 accepted a sibling for liveness: gates and `CanTransition`/`ValidateGates`
  have the *same* consumers and the *same* triggering clock. Splitting them would fail the
  disjoint-consumer test that justified `LivenessEngine`'s separation in the first place —
  inconsistent application of the same principle would be worse than either extreme applied
  uniformly.
- **A fourth interface for gate initiation (spawning reviews/approval UI)** — rejected: no evidence
  of an independent consumer for this beyond the existing orchestration call sites
  (`ReviewGateRunner`, the transition handler); adding one now would be speculative, not
  demand-driven.
- **Leave `ValidateGates`'s signature unchanged and bolt structured gate data onto a separate,
  unrelated query** — rejected: would let `PendingGates` and `ValidateGates` drift out of sync
  (two independent implementations of "is this transition gated" instead of one source of truth with
  a thin wrapper).

## Consequences

**Positive**: `WorkflowEngine` stays at 4 methods (not 3 + a second interface); every existing
`ValidateGates` call site (`TransitionBacklogItemStatus`, `GuardedTransitionAllowed`) needs zero
changes; the item-detail "what's blocking this" UI (Epic 2.10) gets exactly the data it needs from
one new method.

**Negative**: `DefaultWorkflowEngine.PendingGates` must express every existing `TransitionGuard`
branch as an equivalent structural `GateStatus` — real, non-mechanical translation work (Epic 2.3),
not a trivial pass-through.

**Neutral**: this ADR also implements ADR-013 Phase 2 as originally proposed — `ConfiguredWorkflowEngine`
is a drop-in `WorkflowEngine` implementation exactly as ADR-013 anticipated — while extending its
scope with the gate model ADR-013 never specified. ADR-013's own status is updated to `Accepted` with
a cross-link to this ADR (Epic 2.11).

---

See `ADR-001-liveness-engine-sibling-interface.md` for the sibling-interface decision this ADR
deliberately does not repeat, and `ADR-003-custom-gate-check-execution-bound.md` for how the
`GateKindCustom` gate type is bounded to avoid becoming an arbitrary-code-execution surface.
