# ADR-013: WorkflowEngine Interface Replaces `validTransitions` Map at Runtime

**Status**: Proposed
**Date**: 2026-05-19
**Deciders**: Tyler Stapler

---

## Context

`session/backlog.go` currently encodes the backlog state machine as a package-level `var validTransitions map[BacklogStatus]map[BacklogStatus]bool` and a free function `CanTransitionBacklog(from, to)`. `TransitionGuard` in the same file applies hard-coded business rule checks per transition pair.

This design works for a fixed six-state workflow but blocks two requirements:

1. **`refining` state (S1)** — adding it requires edits to the `validTransitions` map, `TransitionGuard`, `BacklogLifecycleListener`, the proto enum, and `useBacklogService.ts`. Eight files for one state.
2. **Custom states (S2/S3)** — user-defined states cannot be known at compile time, so the map cannot hold them.

The core issue is that transition logic is **baked into the package rather than injected**. The service layer has no seam to substitute a different rule source.

---

## Decision

Introduce a `WorkflowEngine` interface in `session/workflow_engine.go`. The interface is narrow — only the operations needed by `BacklogService` and `BacklogLifecycleListener`:

```go
// WorkflowEngine is the runtime policy for backlog state transitions.
// Implementations may be backed by the static default config (DefaultWorkflowEngine)
// or a DB-persisted WorkflowConfig (ConfiguredWorkflowEngine).
type WorkflowEngine interface {
    // CanTransition reports whether the from→to edge exists in the workflow graph.
    CanTransition(from, to BacklogStatus) bool

    // ValidateGates checks business-rule gates for the transition.
    // Returns nil when all gates pass, or a sentinel error (ErrACRequired etc.) when blocked.
    ValidateGates(input BacklogItemTransitionInput, to BacklogStatus) error

    // AllowedTransitions returns all target states reachable from from.
    // Used to drive UI button visibility without duplicating the map client-side.
    AllowedTransitions(from BacklogStatus) []BacklogStatus
}
```

`DefaultWorkflowEngine` wraps the existing `validTransitions` map and `TransitionGuard` function, preserving all current behavior. It is constructed once at server startup and injected into `BacklogService`.

`BacklogService.TransitionBacklogItemStatus` replaces its direct calls to `session.CanTransitionBacklog` and `session.TransitionGuard` with calls to the injected `WorkflowEngine`.

`BacklogLifecycleListener` receives a `WorkflowEngine` so that `onSessionExited` no longer hard-codes `BacklogStatusReview` as the post-work state — instead it calls `engine.AllowedTransitions(BacklogStatusInProgress)` and picks the first non-archived target (or the engine exposes a dedicated `PostWorkTransition` helper).

The `refining` state is added to `DefaultWorkflowEngine` in Phase 1 without modifying the interface. Custom-state support (Phase 2) introduces `ConfiguredWorkflowEngine` backed by a DB-loaded `WorkflowConfig`, transparently satisfying the same interface.

---

## Consequences

**Positive**:
- Adding `refining` (or any future built-in state) requires only `DefaultWorkflowEngine` changes — no ripple through the service layer.
- Phase 2 custom states are a drop-in: swap `DefaultWorkflowEngine` with `ConfiguredWorkflowEngine` via the same interface.
- `BacklogService` and `BacklogLifecycleListener` become testable with mock engines.
- `AllowedTransitions` eliminates the duplicate transition table on the frontend (`STATUS_TRANSITIONS` in `useBacklogService.ts`).

**Negative**:
- One additional interface layer to navigate during debugging.
- `DefaultWorkflowEngine` must be constructed and threaded through `server/dependencies.go`; adds ~10 lines of wiring.

**Neutral**:
- The existing `validTransitions` package-level var and `CanTransitionBacklog`/`TransitionGuard` free functions are **kept as-is** and wrapped by `DefaultWorkflowEngine`. They are not deleted until Phase 2 confirms the interface is stable (avoids a big-bang refactor).

---

## Alternatives Considered

**Alt A: Add `refining` directly to `validTransitions`** — unblocks S1 but leaves the hardcoded-enum problem unresolved. Defers the interface seam until Phase 2, when the migration is more disruptive.

**Alt B: Load WorkflowConfig on every `TransitionBacklogItemStatus` call** — avoids an interface but creates a per-call DB read (O(n) for list operations). Rejected due to the caching pitfall identified in research.

**Alt C: Store workflow as a proto message, not a Go interface** — over-engineers the seam; proto is the serialization layer, not the runtime policy object. The interface stays in Go.
