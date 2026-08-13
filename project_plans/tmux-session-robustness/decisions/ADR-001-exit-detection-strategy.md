# ADR-001: Three-Layer Exit Detection Strategy

Status: Accepted
Date: 2026-04-16
Deciders: Tyler Stapler

---

## Context

When a session's program (e.g. `claude`) exits inside tmux, stapler-squad currently has no
mechanism to propagate that event to the `Instance` state machine. `instance.started` stays
`true` and `instance.Status` stays `Running`. The UI shows the session alive; the review queue
keeps polling it.

Three signal paths already exist in the codebase but none reach the state machine:

1. `processControlModeLine` in `control_mode.go` handles `%exit` with a log statement and the
   comment "let the caller handle cleanup." No caller handles it.
2. `ResponseStream.streamLoop` in `response_stream.go` detects PTY EOF (line ~149) and logs to
   `ForSession()` but calls only `closeAllSubscribers()` before returning.
3. `SessionHealthChecker.checkSingleSession` in `health.go` calls `TmuxAlive()` and triggers
   recovery, but has no debounce and no server-down guard.

On tmux 3.6a (the version installed on the target machine), `tmux -C attach-session` exits when
the session it is attached to dies, which causes both `%exit` and scanner EOF to fire. These are
reliable signals for the common case.

## Decision

Use a three-layer detection model where all layers converge on a single `onExit` callback
protected by `sync.Once`:

**Layer 1 — Reactive control mode event (primary, ~ms latency)**
Wire `%exit` and `%session-closed` notifications in `processControlModeLine` to an `onExit func(reason string)`
field on `TmuxSession`. Fire via `onExitOnce.Do` to prevent double-transition.

**Layer 2 — Scanner EOF fallback (secondary, same goroutine)**
After the scanner loop in `readControlModeOutput` exits, fire `onExit` via `onExitOnce.Do`
with reason `"control-mode-pipe-closed"`. This covers cases where `%exit` was not received
(e.g., control mode process killed externally before sending `%exit`).

**Layer 3 — PTY EOF via `ResponseStream.OnEOF` (tertiary, milliseconds)**
Add `OnEOF func()` to `ResponseStream`. Wire it from `Instance.start()` to call
`transitionTo(Stopped)` and set `i.started = false`. This covers sessions where control mode
is disabled (`STAPLER_SQUAD_USE_CONTROL_MODE=false`) or never started successfully.

**Intentional-stop guard**
Add `intentionalStop atomic.Bool` to `TmuxSession`. Set it at the top of `StopControlMode()`
before any other work. Gate all `onExitOnce.Do` calls behind `!t.intentionalStop.Load()` to
prevent false exit events when the operator intentionally stops control mode.

The `onExit` callback on `TmuxSession` is set by `Instance.start()` after a successful
`transitionTo(Running)`. It acquires `stateMutex`, calls `transitionTo(Stopped)`, and sets
`i.started = false`.

## Alternatives Considered

**Option A (chosen): Targeted propagation of existing signals**
Wire `%exit` and PTY EOF to callbacks. No new infrastructure, no new Go primitives.
Fits the existing `onServerRecovered` callback convention already in `tmux.go`.

**Option B: Direct tmux unix socket protocol**
Implement the `imsg` binary protocol over `/tmp/tmux-<UID>/default` directly.
Rejected: undocumented binary format, no Go library, version-sensitive. The control mode text
protocol is the documented API for this use case.

**Option C: Replace tmux with custom process supervisor**
Build a Go daemon per session (like `claude-mux`). Rejected: loses `capture-pane`, `send-keys`,
keepalive architecture, and all tmux introspection. The exit detection problem is a wiring gap,
not a transport gap.

**Option D: Full FSM with transition callbacks**
Route all `i.Status = X` assignments through `transitionTo()` and fire `TransitionHook`
callbacks. Rejected for this iteration: `instance.go` is ~2,400 lines with status writes
scattered throughout. A mechanical audit is high-risk. Deferred to a follow-on refactor session.

## Consequences

Positive:
- Session exit is detectable within milliseconds of the program dying
- The three layers provide defence-in-depth: any one layer alone is sufficient for the common case
- `sync.Once` guarantees exactly one `transitionTo(Stopped)` call regardless of which layer fires first
- No new external dependencies
- `STAPLER_SQUAD_USE_CONTROL_MODE=false` mode remains functional (layer 3 covers it)

Negative / Accepted risks:
- `onExitOnce` is exhausted after the first exit; a restarted session must get a fresh `TmuxSession`
  object. Verify that `initTmuxSession()` always creates a new object on restart.
- `transitionTo(Stopped)` must be a legal FSM transition from `Running` and `Ready`. Verify
  the FSM table before implementation.
- `intentionalStop atomic.Bool` requires Go 1.19+. Confirm module version; fall back to
  `atomic.StoreUint32/LoadUint32` on `uint32` if needed.
- Exit codes are not captured in this iteration. `%pane-died` parsing deferred to follow-on.
