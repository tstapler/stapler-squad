# ADR-003: Consumer-Owned SessionStreamer Interface + LifecycleListener Pattern

Status: Accepted
Date: 2026-04-16
Deciders: Tyler Stapler

---

## Context

Two coupling problems exist in the server layer:

**Problem 1 — Server layer bypasses Instance API (findings-architecture.md)**
`server/services/connectrpc_websocket.go` at line ~453 calls `instance.GetTmuxSession()` to
reach `TmuxSession` directly, then calls `tmuxSession.StartControlMode()` and
`SubscribeToControlModeUpdates()`. This bypasses the `Instance` API and re-exposes the concrete
`TmuxSession` type that `TmuxProcessManager` was introduced to hide. Any future refactoring of
`TmuxSession` (e.g., changing the `SubscribeToControlModeUpdates` signature) will break the
server layer without compiler warning.

**Problem 2 — No lifecycle event surface (findings-architecture.md)**
The review queue and server layer have no way to react to session lifecycle events without
polling `Instance.Status` on a ticker. This means there is no clean path for the review queue
to clear a stale content cache entry when a session exits, or for the server layer to push a
status update to connected WebSocket clients when a session dies.

## Decision

### Consumer-owned `SessionStreamer` interface

Define a minimal interface in `server/services/session_streamer.go`:

```go
type SessionStreamer interface {
    StartControlMode() error
    StopControlMode() error
    SubscribeControlModeUpdates() (string, <-chan []byte)
    UnsubscribeControlModeUpdates(id string)
}
```

This interface is defined where it is consumed (the server layer), not where it is implemented
(`session` package). This is the Go-idiomatic "consumer-owned interface" pattern. It:
- Prevents import cycles (server layer does not import `session/tmux`)
- Keeps the interface minimal (only what the WebSocket handler legitimately needs)
- Enforces that new `TmuxSession` methods are not acquired by the server layer without an
  explicit interface update

`*Instance` implements `SessionStreamer` via four delegation methods that forward to
`i.tmuxManager.GetTmuxSession()`. These methods nil-guard the inner `TmuxSession` and return
safe defaults (nil error, pre-closed channel) when no session is available.

### `LifecycleListener` interface with fire-and-forget goroutine dispatch

Define a `LifecycleListener` interface on `Instance`:

```go
type LifecycleListener interface {
    OnLifecycleEvent(instance *Instance, event LifecycleEvent)
}
```

Events: `EventStarted`, `EventExited`, `EventRestarted`.

Listeners are registered via `RegisterLifecycleListener()`. The `fireLifecycleEvent()` helper
dispatches to all registered listeners in separate goroutines (fire-and-forget). This prevents
any slow listener from blocking `Instance.start()` or the `onExit` callback.

The `onExit` callbacks from Phase 1 (ADR-001) become the authoritative `EventExited` dispatch
points. The `start()` success path fires `EventStarted`. No separate event for `EventRestarted`
is needed in Phase 3 — callers that need to distinguish restart from first-start can compare
the instance's previous state.

## Alternatives Considered

**Alternative for streaming interface: extend `InstanceContext`**
The existing `InstanceContext` interface (used by `ClaudeController`) could be extended with
control mode methods. Not chosen: `InstanceContext` is defined in the `session` package and
serves a different consumer (the controller). Adding server-layer concerns to it would widen
the interface for all `InstanceContext` consumers. Consumer-owned interfaces are the better fit.

**Alternative for lifecycle events: channel-based event bus**
A package-level `chan LifecycleEvent` fan-out bus. Rejected: requires channel lifecycle
management (close on shutdown, drain or drop on slow consumer), adds backpressure decisions,
and is over-engineered for two consumers (review queue, server layer). The existing
`onServerRecovered` callback in `tmux.go` demonstrates that simple function fields are the
accepted convention in this codebase. `LifecycleListener` generalises that convention to
per-instance events.

**Alternative for lifecycle events: polling reconciler only**
Do not add a listener interface; have consumers poll `Instance.Status` on a ticker. Rejected:
polling adds detection latency proportional to the poll interval, which conflicts with the ≤10s
zombie detection requirement when review queue subscribers need immediate notification of exit.

**Full FSM with TransitionHook callbacks (Option D from synthesis.md)**
Model all state transitions through `transitionTo()` and fire hooks. Deferred: the mechanical
audit of ~2,400 lines in `instance.go` is high-risk and out of scope for this iteration. The
`LifecycleListener` pattern achieves the requirements without requiring a full FSM audit.

## Consequences

Positive:
- `connectrpc_websocket.go` no longer imports `session/tmux`; coupling is severed at the package boundary
- New `TmuxSession` methods are not silently accessible to the server layer
- The review queue and server layer have a typed event subscription mechanism
- Listeners run in separate goroutines; no listener can block session lifecycle operations
- Interface satisfaction is verified at compile time (Go implicit interface)

Negative / Accepted risks:
- Four delegation methods added to `instance.go` (small but increases the file size)
- `fireLifecycleEvent` spawns a goroutine per listener per event. With two listeners and one
  event (session exit), this is two goroutines — negligible. If many listeners are registered
  in the future, consider a worker pool. For the current use case, fire-and-forget is correct.
- `LifecycleListener` has no replay for late registrants. A listener registered after
  `EventStarted` misses the event. For the review queue, this is acceptable: it is registered
  at startup before any sessions start.
- The `SessionStreamer` interface must be kept in sync with the `TmuxSession` methods it wraps.
  If `SubscribeToControlModeUpdates` is renamed in `TmuxSession`, the delegation methods in
  `instance.go` must be updated, but the interface itself will not break (Go implicit interface
  means the breakage is caught at compile time in `connectrpc_websocket.go`).
