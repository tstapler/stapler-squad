# Implementation Plan: TMux Session Robustness

Status: Ready for Implementation
Created: 2026-04-16
Requirements: `project_plans/tmux-session-robustness/requirements.md`
Research: `project_plans/tmux-session-robustness/research/`
ADRs: `project_plans/tmux-session-robustness/decisions/ADR-001-exit-detection-strategy.md`, `ADR-002-zombie-reconciliation.md`, `ADR-003-session-api-interface.md`

---

## Overview

Fix silent session exits and zombie session state in the tmux session manager. When a session's
program (e.g. `claude`) exits inside tmux, the system currently leaves `IsStarted=true` and
`Status=Running` with no log evidence of why the session stopped. This causes the review queue to
poll a dead session indefinitely and the operator to find nothing useful in the session log.

The fix is a three-layer model: reactive event detection (primary, ~ms latency), scanner-EOF
fallback (secondary, same goroutine), and periodic reconciliation (tertiary, catches edge cases
missed by the first two). All three layers converge on a single `onExit` callback path, guarded
by `sync.Once` to prevent double-transition.

Three independent phases ship in order of impact:

- Phase 1 — Exit Detection: wire `%exit` and PTY EOF signals to the Instance state machine
- Phase 2 — Zombie Reconciliation: add `startMu`, debounce the health checker, batch list-sessions
- Phase 3 — Clean API: consumer-owned `SessionStreamer` interface in the server layer

---

## Dependency Visualization

```
Phase 1                         Phase 2                         Phase 3
Exit Detection                  Zombie Reconciliation           Clean API
─────────────────────────────   ─────────────────────────────   ─────────────────────────────
1a. onExit callback +           2a. startMu per Instance        3a. SessionStreamer interface
    %exit wiring                2b. Health checker debounce +   3b. LifecycleListener hooks
1b. onExit wired from               server-down guard
    Instance.start()            2c. ListAllSessions batch
1c. OnEOF from streamLoop
    wired to Instance

  no deps between 1a/1b/1c       2a must precede 2b             3a/3b independent of Phase 1/2
  (all three in same PR)         2c independent                  but Phase 1 onExit callbacks
                                                                 become Phase 3 hook fire points

Phase 1 ──────────────────────────────────> Phase 3 (1's onExit wires become 3's hook calls)
Phase 2a (independent) ──────────> Phase 2b
Phase 2c (independent) — can ship separately after 2a/2b are stable
```

Phases 1 and 2a can proceed in parallel. Phase 2b depends on 2a (`startMu` must be in place
before health checker debounce is useful). Phase 3 is architecturally independent but uses the
`onExit` fire points established in Phase 1 as the authoritative `EventExited` dispatch points.

---

## Phase 1: Exit Detection

**Priority**: P0 (fixes the observed production failure)
**Risk**: Low — additive callbacks with `sync.Once` guard; no state machine restructuring

### Context

Three files need changes. The goal is to make every exit path call the same `onExit` callback,
regardless of whether the exit was detected via control mode `%exit`, scanner EOF, or PTY EOF.
`sync.Once` prevents double-transition when two paths fire within the same millisecond.

The `intentionalStop` guard distinguishes operator-initiated `StopControlMode()` calls (not a
crash) from unexpected exits (session death). The flag must be an `atomic.Bool` because
`StopControlMode()` writes it while `readControlModeOutput()` reads it from separate goroutines.

The `started` flag reset in `streamLoop` (task 1c) fixes a secondary bug: after an EOF-driven
exit, `rs.started` remains `true`, causing the next `Start()` call to return "already started."

### Tasks

#### Task 1.1: Add `onExit` callback and `intentionalStop` guard to `TmuxSession`

**File**: `session/tmux/control_mode.go`
**File**: `session/tmux/tmux.go` (struct definition)

Add fields to `TmuxSession` struct:

```go
// session/tmux/tmux.go — inside TmuxSession struct
onExit          func(reason string)
onExitOnce      sync.Once
intentionalStop atomic.Bool
```

Note: `atomic.Bool` requires no import additions; `sync/atomic` is already imported via `sync`.
`atomic.Bool` was added in Go 1.19. Verify Go module version allows it — if not, use
`uint32` with `atomic.StoreUint32`/`atomic.LoadUint32`.

In `processControlModeLine`, replace the `case "%exit":` log-only handler:

```go
case "%exit":
    log.InfoLog.Printf("Control mode received %%exit for session '%s'", t.sanitizedName)
    if !t.intentionalStop.Load() {
        t.onExitOnce.Do(func() {
            if t.onExit != nil {
                t.onExit("control-mode-%exit")
            }
        })
    }
```

Add `%session-closed` handling in the same switch (additive case, same PR cost):

```go
case "%session-closed":
    // %session-closed $SESSION_ID  — fires when a named session is destroyed
    log.InfoLog.Printf("Control mode session-closed for '%s': %s", t.sanitizedName, strings.Join(fields[1:], " "))
    if !t.intentionalStop.Load() {
        t.onExitOnce.Do(func() {
            if t.onExit != nil {
                t.onExit("session-closed")
            }
        })
    }
```

In `readControlModeOutput()`, after `t.controlModeExited = true` (after the scanner loop exits),
fire the same callback as a scanner-EOF fallback. This covers cases where `%exit` was not
received (e.g., control mode process killed externally):

```go
// After: t.controlModeExited = true
// After the controlModeSubMu.Lock block that closes subscriber channels
if !t.intentionalStop.Load() {
    t.onExitOnce.Do(func() {
        if t.onExit != nil {
            t.onExit("control-mode-pipe-closed")
        }
    })
}
```

In `StopControlMode()`, set the intentional stop flag at the very top before any other work:

```go
func (t *TmuxSession) StopControlMode() error {
    t.intentionalStop.Store(true)
    // ... rest of existing implementation unchanged
```

**Acceptance criteria**:
- GIVEN a tmux session that exits unexpectedly
- WHEN `processControlModeLine` receives `%exit`
- THEN `onExit` is called exactly once with reason `"control-mode-%exit"`
- AND if `StopControlMode()` was called before the session died, `onExit` is NOT called

- GIVEN the control mode scanner loop exits (pipe closed without `%exit`)
- WHEN `readControlModeOutput` reaches the end of the scanner loop
- THEN `onExit` is called exactly once with reason `"control-mode-pipe-closed"`

- GIVEN `StopControlMode()` is called by the operator
- WHEN the process exits cleanly as a result
- THEN `onExit` is NOT fired (intentionalStop guard)

#### Task 1.2: Wire `onExit` from `Instance.start()` into `TmuxSession`

**File**: `session/instance.go`

After `i.tmuxManager.SetSession(session)` in `initTmuxSession()` (or immediately after the
`transitionTo(Running)` call succeeds in `start()`), wire the callback:

```go
// Wire exit callback — fires when control mode detects the session died.
// The callback acquires stateMutex, so it must not be called while stateMutex is held.
tmuxSession := i.tmuxManager.GetTmuxSession()
if tmuxSession != nil {
    tmuxSession.onExit = func(reason string) {
        log.ForSession(i.Title).Warning("Session exited via control mode: %s", reason)
        i.stateMutex.Lock()
        defer i.stateMutex.Unlock()
        if transErr := i.transitionTo(Stopped); transErr != nil {
            // transitionTo returns an error for illegal transitions (e.g., already Stopped).
            // This is expected if Phase 1c (PTY EOF callback) fired first.
            log.DebugLog.Printf("transitionTo(Stopped) from onExit for '%s': %v (may be duplicate)", i.Title, transErr)
        }
        i.started = false
    }
}
```

**Verify**: Confirm `transitionTo(Stopped)` is a legal transition from `Running` and `Ready` in
the FSM table at `instance.go:733`. If `Running → Stopped` is not defined, add it. The existing
`Kill()` path likely already handles this transition; check there for precedent.

**Acceptance criteria**:
- GIVEN a session in `Running` state
- WHEN the control mode `onExit` callback fires
- THEN `instance.Status` transitions to `Stopped`
- AND `instance.started` is set to `false`
- AND the session-scoped log (`log.ForSession`) records the exit reason

#### Task 1.3: Add `OnEOF` callback to `ResponseStream` and wire to `Instance`

**File**: `session/response_stream.go`

Add `OnEOF func()` field to `ResponseStream` struct (alongside existing `onOutput func()`).

In `streamLoop`, at all three exit-via-EOF paths (the `io.EOF` case at line ~149 and the
"file already closed / bad file descriptor / input/output error" case at line ~166), call the
callback before returning, and reset `rs.started`:

```go
// At each EOF exit path:
rs.mu.Lock()
rs.started = false
rs.mu.Unlock()
if rs.OnEOF != nil {
    rs.OnEOF()
}
rs.closeAllSubscribers()
return
```

The `rs.started = false` reset under `rs.mu` fixes pitfall 6b (subsequent `Start()` fails with
"already started" after an EOF-driven exit without going through `Stop()`).

**File**: `session/instance.go`

In the controller start path (where `StartController()` is called), after creating the
`ResponseStream`, wire the callback:

```go
responseStream.OnEOF = func() {
    log.ForSession(i.Title).Warning("Session exited (PTY EOF)")
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    if transErr := i.transitionTo(Stopped); transErr != nil {
        log.DebugLog.Printf("transitionTo(Stopped) from OnEOF for '%s': %v (may be duplicate)", i.Title, transErr)
    }
    i.started = false
}
```

**Acceptance criteria**:
- GIVEN `streamLoop` detects PTY EOF
- WHEN `OnEOF` is wired
- THEN `instance.Status` transitions to `Stopped`
- AND `instance.started` is set to `false`
- AND `rs.started` is reset to `false`
- AND a subsequent `responseStream.Start()` call succeeds (no "already started" error)
- AND the session-scoped log records "Session exited (PTY EOF)"

---

## Phase 2: Zombie Reconciliation

**Priority**: P1 (reduces zombie detection latency and prevents double-start)
**Risk**: Low-Medium — `startMu` is additive; health checker changes require careful testing

### Context

Phase 1 eliminates the common case (event-driven exit detection). Phase 2 hardens the fallback
path (health checker catches what Phase 1 misses). The double-start race documented in
`findings-pitfalls.md` pitfall 5 is the highest-severity remaining issue: without `startMu`,
the health checker and the server restore path can simultaneously call `Start()`, produce two
`streamLoop` goroutines on the same PTY, and race to close subscribers on exit.

The 2-cycle debounce for zombie detection adds ~2-4s of detection latency beyond what Phase 1
provides, but it eliminates false-positive zombie detection during the 50-200ms tmux server
restart window. The latency is well within the ≤10s requirement.

### Tasks

#### Task 2.1: Add `startMu sync.Mutex` to `Instance`

**File**: `session/instance.go`

Add `startMu sync.Mutex` to the `Instance` struct. At the top of the `start()` function body
(before `i.initTmuxSession()`), acquire the lock and re-check `i.started` inside the lock:

```go
func (i *Instance) start(firstTimeSetup bool, setupCleanup bool, cleanup *tmux.CleanupFunc) error {
    i.startMu.Lock()
    defer i.startMu.Unlock()

    // Double-checked locking: re-verify started state after acquiring the mutex.
    // A concurrent Start() call may have completed while we waited for the lock.
    if i.started && !firstTimeSetup {
        log.DebugLog.Printf("start() called for already-started instance '%s', skipping", i.Title)
        return nil
    }

    log.InfoLog.Printf("Starting instance '%s' (firstTimeSetup: %v)", i.Title, firstTimeSetup)
    // ... rest of existing implementation
```

The re-check uses `!firstTimeSetup` because `firstTimeSetup=true` indicates a deliberate
creation call that should proceed regardless.

**Acceptance criteria**:
- GIVEN two concurrent calls to `Start(false)` on the same instance
- WHEN the first call completes while the second is waiting on `startMu`
- THEN the second call returns nil without starting a second tmux session or PTY
- AND only one `streamLoop` goroutine exists per session

#### Task 2.2: Add server-down guard and 2-cycle debounce to health checker

**File**: `session/health.go`

Add a `failureCounts map[string]int` field to `SessionHealthChecker` to track consecutive
failures per instance title. Add a `failureCountsMu sync.Mutex` to protect the map.

In `checkSingleSession`, before calling `instance.Start(false)` for zombie recovery:

1. Check whether the tmux server itself is down. Use the existing server-level check pattern.
   If the server is down, skip this instance — the server recovery path handles bulk recovery:

```go
// Guard: skip individual session recovery if the tmux server is not running.
// The server-level recoverFromServerFailure() path handles bulk recovery.
if isServerDown() {
    result.Actions = append(result.Actions, "Skipped recovery: tmux server is down")
    return result
}
```

2. Require 2 consecutive `TmuxAlive() == false` results before triggering recovery:

```go
h.failureCountsMu.Lock()
h.failureCounts[instance.Title]++
count := h.failureCounts[instance.Title]
h.failureCountsMu.Unlock()

if count < 2 {
    result.Issues = append(result.Issues, fmt.Sprintf(
        "Instance looks unhealthy (consecutive failures: %d/2), deferring recovery", count))
    return result
}

// Two consecutive failures confirmed — proceed with recovery.
h.failureCountsMu.Lock()
h.failureCounts[instance.Title] = 0
h.failureCountsMu.Unlock()
```

Reset the counter on successful health checks:

```go
// In the "healthy" branch (TmuxAlive() == true):
h.failureCountsMu.Lock()
h.failureCounts[instance.Title] = 0
h.failureCountsMu.Unlock()
```

The `isServerDown()` helper can call the existing `checkServerNotRunning()` function or run
a lightweight `tmux list-sessions` and check for "no server running" in the error output.
Check `session/tmux/tmux.go` for the existing `IsServerRunning`-equivalent logic to reuse.

**Acceptance criteria**:
- GIVEN the tmux server restarts and all `has-session` calls return "no server running"
- WHEN `checkSingleSession` runs during the restart window
- THEN no `Start()` call is made (server-down guard)
- AND the individual failure counter is not incremented during server-down state

- GIVEN a session that is genuinely dead (tmux session absent)
- WHEN `checkSingleSession` runs twice consecutively
- THEN recovery is attempted on the second cycle (debounce)
- AND the log includes "consecutive failures: 2/2"

#### Task 2.3: Add `ListAllSessions` batch function

**File**: `session/tmux/tmux.go` (new function, package-level)

Add a package-level function that replaces N per-session `DoesSessionExist()` subprocess calls
with a single `tmux list-sessions` invocation:

```go
// ListAllSessions returns the set of all currently live tmux session names.
// Uses serverSocket for isolation if non-empty (same -L flag semantics as TmuxSession).
// Does NOT go through the per-session existence cache — intended for bulk reconciliation.
func ListAllSessions(serverSocket string) (map[string]bool, error) {
    var args []string
    if serverSocket != "" {
        args = append(args, "-L", serverSocket)
    }
    args = append(args, "list-sessions", "-F", "#{session_name}")

    out, err := exec.Command("tmux", args...).Output()
    if err != nil {
        // "no server running" is a common error — not fatal for reconciliation
        if strings.Contains(err.Error(), "no server running") ||
            strings.Contains(err.Error(), "error connecting to") {
            return nil, nil // Return nil map, nil error — caller treats as "server down"
        }
        return nil, fmt.Errorf("ListAllSessions: %w", err)
    }

    sessions := make(map[string]bool)
    for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
        if name != "" {
            sessions[name] = true
        }
    }
    return sessions, nil
}
```

Update `CheckAllSessions()` in `health.go` to call `ListAllSessions` once per cycle and pass
the result set to `checkSingleSession` as an additional parameter, replacing the per-instance
`TmuxAlive()` call with a set lookup.

**Acceptance criteria**:
- GIVEN 10 running sessions
- WHEN `CheckAllSessions` runs
- THEN exactly one `tmux list-sessions` subprocess is spawned per health check cycle
- AND the result is equivalent to calling `DoesSessionExist()` per session

---

## Phase 3: Clean API Surface

**Priority**: P2 (reduces coupling, prevents future regressions)
**Risk**: Low — consumer-owned interface pattern; `*Instance` satisfies the interface implicitly

### Context

`server/services/connectrpc_websocket.go` at line 453 calls `instance.GetTmuxSession()` and
then calls `tmuxSession.StartControlMode()` directly. This bypasses the `Instance` API and
re-exposes the concrete `TmuxSession` type. The fix is a consumer-owned `SessionStreamer`
interface defined in the server layer, implemented by `*Instance` via delegation methods.

The `LifecycleListener` interface gives the review queue and the server layer a typed way to
subscribe to session lifecycle events (`EventStarted`, `EventExited`, `EventRestarted`) without
polling `Instance.Status`.

### Tasks

#### Task 3.1: Consumer-owned `SessionStreamer` interface

**File**: `server/services/session_streamer.go` (new file, ~30 lines)

```go
package services

// SessionStreamer is the interface the WebSocket streaming handler requires.
// Defined in the consumer package (server/services) to prevent import cycles
// and to keep the interface minimal — only what this package legitimately needs.
type SessionStreamer interface {
    StartControlMode() error
    StopControlMode() error
    SubscribeControlModeUpdates() (string, <-chan []byte)
    UnsubscribeControlModeUpdates(id string)
}
```

**File**: `session/instance.go`

Add delegation methods that implement `SessionStreamer` on `*Instance`:

```go
// StartControlMode starts the control mode stream on the underlying tmux session.
func (i *Instance) StartControlMode() error {
    ts := i.tmuxManager.GetTmuxSession()
    if ts == nil {
        return fmt.Errorf("no tmux session available for instance '%s'", i.Title)
    }
    return ts.StartControlMode()
}

// StopControlMode stops the control mode stream.
func (i *Instance) StopControlMode() error {
    ts := i.tmuxManager.GetTmuxSession()
    if ts == nil {
        return nil
    }
    return ts.StopControlMode()
}

// SubscribeControlModeUpdates registers a new subscriber for real-time terminal output.
func (i *Instance) SubscribeControlModeUpdates() (string, <-chan []byte) {
    ts := i.tmuxManager.GetTmuxSession()
    if ts == nil {
        ch := make(chan []byte)
        close(ch)
        return "", ch
    }
    id, ch := ts.SubscribeToControlModeUpdates()
    return id, ch
}

// UnsubscribeControlModeUpdates removes a subscriber by ID.
func (i *Instance) UnsubscribeControlModeUpdates(id string) {
    ts := i.tmuxManager.GetTmuxSession()
    if ts == nil {
        return
    }
    ts.UnsubscribeFromControlModeUpdates(id)
}
```

**File**: `server/services/connectrpc_websocket.go`

Replace the direct `GetTmuxSession()` access. Find the call site at ~line 453 and accept the
`SessionStreamer` interface instead of calling `instance.GetTmuxSession()`:

```go
// Before (direct concrete access):
tmuxSession := instance.GetTmuxSession()
if err := tmuxSession.StartControlMode(); err != nil { ... }
subID, ch := tmuxSession.SubscribeToControlModeUpdates()

// After (interface-based):
var streamer services.SessionStreamer = instance  // *Instance implements SessionStreamer
if err := streamer.StartControlMode(); err != nil { ... }
subID, ch := streamer.SubscribeControlModeUpdates()
```

The `*Instance` type satisfies `SessionStreamer` implicitly. No cast or registration needed.

**Acceptance criteria**:
- GIVEN `connectrpc_websocket.go` is compiled
- THEN it does not import `session/tmux` or call `GetTmuxSession()`
- AND all existing streaming behaviour is preserved

#### Task 3.2: `LifecycleListener` interface and `RegisterLifecycleListener`

**File**: `session/instance.go`

Define the interface and event type:

```go
// LifecycleEvent indicates what happened to a session.
type LifecycleEvent int

const (
    EventStarted  LifecycleEvent = iota
    EventExited                  // Session program exited (clean or crash)
    EventRestarted               // Session was restarted after exit
)

// LifecycleListener receives session lifecycle events.
// OnLifecycleEvent is called from a separate goroutine (fire-and-forget).
// Implementations must not block.
type LifecycleListener interface {
    OnLifecycleEvent(instance *Instance, event LifecycleEvent)
}
```

Add to `Instance` struct:

```go
lifecycleListeners   []LifecycleListener
lifecycleListenersMu sync.RWMutex
```

Add methods:

```go
// RegisterLifecycleListener adds a listener that receives session lifecycle events.
// Safe to call from any goroutine.
func (i *Instance) RegisterLifecycleListener(l LifecycleListener) {
    i.lifecycleListenersMu.Lock()
    defer i.lifecycleListenersMu.Unlock()
    i.lifecycleListeners = append(i.lifecycleListeners, l)
}

// fireLifecycleEvent dispatches an event to all registered listeners.
// Each listener is called in its own goroutine to prevent blocking the caller.
func (i *Instance) fireLifecycleEvent(event LifecycleEvent) {
    i.lifecycleListenersMu.RLock()
    listeners := make([]LifecycleListener, len(i.lifecycleListeners))
    copy(listeners, i.lifecycleListeners)
    i.lifecycleListenersMu.RUnlock()

    for _, l := range listeners {
        l := l // capture loop variable
        go l.OnLifecycleEvent(i, event)
    }
}
```

Update the `onExit` callbacks from Phase 1 (tasks 1.2 and 1.3) to call
`i.fireLifecycleEvent(EventExited)` before returning. Update the `start()` success path
(after `i.started = true`) to call `i.fireLifecycleEvent(EventStarted)`.

**Acceptance criteria**:
- GIVEN a listener is registered on an instance
- WHEN the session exits (via control mode or PTY EOF)
- THEN `OnLifecycleEvent(instance, EventExited)` is called in a separate goroutine
- AND the caller (onExit callback) returns without blocking on the listener

---

## Known Issues

### Potential Bugs Identified During Planning

#### Concurrency Risk: `onExit` fires while `stateMutex` is held by caller [SEVERITY: High]

**Description**: If `onExit` is called from a context where `stateMutex` is already held by
the same goroutine, the `i.stateMutex.Lock()` inside the callback will deadlock.

**Mitigation**:
- Ensure `onExit` is only called from contexts where `stateMutex` is NOT held
- In `processControlModeLine`, the callback fires from the control mode reader goroutine — which
  does not hold `stateMutex`. This is safe.
- In `streamLoop`, the callback fires from the stream goroutine — which does not hold
  `stateMutex`. This is safe.
- Add a code comment in each `onExit` wiring site: "// Must not be called while stateMutex is held."

**Files Affected**: `session/instance.go`

#### Concurrency Risk: `onExitOnce` reset for session restart [SEVERITY: Medium]

**Description**: `sync.Once` fires exactly once per `TmuxSession` object lifetime. After a
session exits and is restarted, a new `TmuxSession` is created via `initTmuxSession()`. The
old `TmuxSession` (with the exhausted `onExitOnce`) is discarded. If the code path
reuses an existing `TmuxSession` object for a restarted session, the second exit will never
fire `onExit`.

**Mitigation**:
- Verify that `initTmuxSession()` always creates a fresh `TmuxSession` on restart. Review
  the `i.tmuxManager.HasSession()` guard — if `HasSession()` returns `true` for an existing
  (but dead) session object, the `onExitOnce` is exhausted.
- Add a `ResetExitOnce()` method to `TmuxSession` that resets both `onExitOnce` and
  `intentionalStop` to their zero values. Call it in the `Instance` restart path.

**Files Affected**: `session/tmux/tmux.go`, `session/instance.go`

#### Integration Risk: `transitionTo(Stopped)` illegal from certain states [SEVERITY: Medium]

**Description**: The FSM table in `instance.go:733` may not define `Running → Stopped` or
`Ready → Stopped` as legal transitions. If `transitionTo` returns an error, the `onExit`
callback silently fails to update status.

**Mitigation**:
- Read the FSM transition table before implementing tasks 1.2 and 1.3
- If `Running → Stopped` is not legal, add it. The `Kill()` path likely already uses a
  direct status write — use it as precedent for the correct approach
- The callback already logs the `transitionTo` error at DEBUG level, so failures are visible

**Files Affected**: `session/instance.go`

#### Concurrency Risk: `streamLoop` `started` flag reset races with `Stop()` [SEVERITY: Low]

**Description**: Task 1.3 resets `rs.started = false` inside `streamLoop` on EOF. `Stop()`
also sets `rs.started = false`. If both paths execute concurrently (Stop() is called just as
EOF fires), both write under `rs.mu` — this is safe (idempotent writes), but `closeAllSubscribers()`
may be called twice if `Stop()` also calls it. Check whether `closeAllSubscribers()` is
idempotent (closing an already-closed channel panics in Go).

**Mitigation**:
- Make `closeAllSubscribers()` idempotent: check whether each channel is already closed
  before closing it, or use a `closeOnce sync.Once` guard per subscriber
- Alternatively, set `rs.started = false` first in `streamLoop`, then check in `Stop()` and
  skip `closeAllSubscribers()` if `started` is already false

**Files Affected**: `session/response_stream.go`

#### Resource Risk: `attachCmd` orphan on `startMu`-blocked goroutine [SEVERITY: Low]

**Description**: With `startMu` in place (task 2.1), a second concurrent `Start()` call waits
on the mutex. If the first `Start()` call creates a tmux session and attaches a PTY, then the
session dies before the mutex is released, the waiting goroutine will proceed into a start
sequence for a dead session. `RestoreWithWorkDir` will create a new PTY attachment, but the
old `attachCmd` may not have been cleaned up.

**Mitigation**:
- This is caught by the double-checked locking pattern: after acquiring `startMu`, re-check
  `i.started`. If Phase 1 correctly set `i.started = false` after the exit, the waiting goroutine
  will proceed to start a fresh session — which is the desired behavior.
- The key dependency: Phase 1 must set `i.started = false` reliably before Phase 2's double-check
  is meaningful. Ship Phase 1 before Phase 2.

**Files Affected**: `session/instance.go`

#### Integration Risk: `isServerDown()` helper does not exist yet [SEVERITY: Medium]

**Description**: Task 2.2 references `isServerDown()`, which is not a named function in the
current health.go. The equivalent logic exists in `tmux.go` (the `recoveryMu + recoveryInFlight`
path uses it). An incorrect implementation of `isServerDown()` could suppress legitimate zombie
recovery.

**Mitigation**:
- Before implementing 2.2, grep for the existing server-down check pattern in `session/tmux/tmux.go`
  and `session/health.go`. Reuse the same logic rather than reimplementing it.
- The simplest correct implementation: run `tmux list-sessions` and check whether the error
  output contains "no server running" or "error connecting to".

**Files Affected**: `session/health.go`, `session/tmux/tmux.go`

---

## Verification Checklist

Before opening a PR for each phase, verify:

**Phase 1**:
- [ ] `STAPLER_SQUAD_USE_CONTROL_MODE=false ./stapler-squad` still works (PTY OnEOF path covers it)
- [ ] `make test` passes with no new failures
- [ ] Session exit is logged to `~/.stapler-squad/logs/` session-scoped log file
- [ ] UI shows session as `Stopped` within 1s of program exit (not after next health check)
- [ ] `StopControlMode()` called by operator does NOT trigger the `onExit` callback
- [ ] Restarting a session after exit starts cleanly (no "already started" error)

**Phase 2**:
- [ ] Concurrent `Start()` calls do not produce two `streamLoop` goroutines (check pprof)
- [ ] Health checker does not trigger recovery during a tmux server restart
- [ ] Health checker triggers recovery within 2 cycles (~2× health check interval) of a genuine zombie
- [ ] `make test` passes; health checker tests verify debounce and server-down guard

**Phase 3**:
- [ ] `server/services/connectrpc_websocket.go` does not import `session/tmux`
- [ ] `go vet ./...` passes (interface satisfaction verified at compile time)
- [ ] Lifecycle listener registered by review queue receives `EventExited` after session death
- [ ] `make lint` passes

---

## Pre-Implementation Verification Required

These questions must be answered by reading code before starting each phase:

**Before Phase 1**:
1. Does `transitionTo(Stopped)` accept `Running → Stopped` and `Ready → Stopped`? Read
   `session/instance.go` around line 733 for the FSM table.
2. Does `initTmuxSession()` create a new `TmuxSession` on restart, or reuse an existing one?
   Check the `i.tmuxManager.HasSession()` guard.
3. Where exactly is `StartController()` called in `instance.go`? This is where `responseStream.OnEOF`
   must be wired. Read `session/instance.go` for the `StartController` call site.

**Before Phase 2**:
4. What is the existing server-down check pattern? Grep for `"no server running"` and
   `checkServerNotRunning` in `session/tmux/tmux.go`.
5. What is the health check interval passed to `ScheduledHealthCheck`? This determines the
   actual debounce latency (2 × interval).

**Before Phase 3**:
6. Does `connectrpc_websocket.go` import any other `session/tmux` symbols beyond `GetTmuxSession()`?
   Run: `grep -n "tmux\." server/services/connectrpc_websocket.go`

---

## Out of Scope (Confirmed by Research)

- Exit code capture (`%pane-died`, `remain-on-exit` polling) — deferred; no requirement for it
- Full FSM audit — mechanical audit of all `i.Status = X` writes in `instance.go`; deferred
- Protobuf / web UI changes — `Stopped` is already a legal serialised status
- Batch `list-sessions` can be deferred if Phase 2a/2b ship first and are stable
- New external Go dependencies — none added in any phase

---

## Related Files

| File | Phase | Change Type |
|------|-------|-------------|
| `session/tmux/tmux.go` | Phase 1 | Add fields: `onExit`, `onExitOnce`, `intentionalStop` |
| `session/tmux/control_mode.go` | Phase 1 | Wire `%exit`, `%session-closed`, scanner-EOF to `onExit` |
| `session/response_stream.go` | Phase 1 | Add `OnEOF` field; reset `started` on EOF exit |
| `session/instance.go` | Phase 1, 2, 3 | Wire `onExit`/`OnEOF`; add `startMu`; add `LifecycleListener`; add `SessionStreamer` delegation methods |
| `session/health.go` | Phase 2 | Add `failureCounts`; add server-down guard; add debounce |
| `session/tmux/tmux.go` | Phase 2 | Add `ListAllSessions()` package-level function |
| `server/services/session_streamer.go` | Phase 3 | New file: `SessionStreamer` interface |
| `server/services/connectrpc_websocket.go` | Phase 3 | Replace `GetTmuxSession()` with `SessionStreamer` |
