# Implementation Plan: Event-Driven Review Queue

_Generated: 2026-05-09_

## Overview

Replace the 2-second polling loop as the primary detection mechanism for session status changes.
`ClaudeController` will emit status-change events via a callback when PTY content changes, routed
through `Instance` to `ReactiveQueueManager.OnControllerStatusChange()` using the same
`wireRateLimitCallbacks` pattern already in production.

---

## Pitfall Mitigations (required before any task is implemented)

| Pitfall | Mitigation built into tasks below |
|---------|-----------------------------------|
| P2 — GetCurrentStatus() in onOutput → subprocess per PTY write (CRITICAL) | Use a capacity-1 `statusCheckCh chan struct{}` + background goroutine in the controller. The `onOutput` closure does a non-blocking send; the goroutine drains and calls `GetCurrentStatus()` at most once per pending signal. This is identical to the `ratelimit.PTYConsumer.NotifyOutput()` / `notifyCh` pattern. |
| P6 — StatusChangeListener called while `cc.mu.RLock()` held → re-entrancy deadlock | Goroutine reads the cache under `RLock`, stores result locally, releases the lock, then calls the listener outside `cc.mu`. |
| P4 — Per-session idle timer goroutine leaks if not stopped | Timer is stored on the controller. `Stop()` calls `timer.Stop()`. The timer callback posts to a capacity-1 channel consumed by the same background goroutine already managing `statusCheckCh`, which exits when `cc.ctx` is cancelled. |
| P3 — Listener registered after Start() → first event lost | `SetStatusChangeListener` MUST be called before `Start()`. `instance_controller.go:StartController()` calls the setter before `controller.Start()` (mirroring `SetOnEOFCallback` ordering). |
| P1 — Callback fires after Stop() | The background goroutine exits when `cc.ctx` is cancelled. The goroutine checks `ctx.Done()` before calling the listener. |
| P5 — minActivityInterval only debounces RecordActivity, not onOutput | The `statusCheckCh` channel (capacity 1) collapses bursts; no inline work beyond a non-blocking channel send in `onOutput`. |

---

## Epic 1 — ClaudeController Status-Change Event Emission

**Scope:** `session/` package only. Zero server-layer imports. Satisfies R1, R4.

**Priority:** Highest

---

### Story 1.1 — Add status-change callback infrastructure to ClaudeController

**Requirement(s):** R1.1, R1.3, R1.4, R4.1

#### Task 1.1.1 — Add new fields and type to `claude_controller.go`

**File:** `session/claude_controller.go`

**Change:** Add the following to the `ClaudeController` struct and declare the `StatusChangeListener` type.

```go
// StatusChangeListener is called when the controller detects a terminal status transition.
// It is always invoked outside cc.mu and from the controller's own background goroutine.
// Implementations must not call back into any ClaudeController method that acquires cc.mu.
type StatusChangeListener func(newStatus detection.DetectedStatus, sessionName string)

// New fields on ClaudeController struct:
statusChangeListener  StatusChangeListener    // registered before Start()
lastEmittedStatus     detection.DetectedStatus
statusCheckCh         chan struct{}            // capacity 1; non-blocking send from onOutput
```

The `statusCheckCh` channel is initialized in `NewClaudeController` to `make(chan struct{}, 1)`.

**Why:** The capacity-1 channel collapses burst PTY output into at most one pending check,
mirroring `ratelimit.PTYConsumer.notifyCh`. No subprocess is spawned in `onOutput`.
Satisfies R1.4 (no server imports) and eliminates Pitfall 2 (P2).

---

#### Task 1.1.2 — Add `SetStatusChangeListener` setter to `claude_controller.go`

**File:** `session/claude_controller.go`

**Change:**

```go
// SetStatusChangeListener registers fn to be called on every terminal status change.
// Must be called before Start(). fn is invoked outside cc.mu; it must not call
// any ClaudeController method that acquires cc.mu.
func (cc *ClaudeController) SetStatusChangeListener(fn StatusChangeListener) {
    cc.statusChangeListener = fn  // plain assignment: must be called before Start()
}
```

**Why:** Mirrors the existing `SetOnEOFCallback` pattern (field assignment, no lock, before-Start
contract). Satisfies R1.1, R1.4.

---

#### Task 1.1.3 — Extend the `SetOnOutput` closure in `Start()` to signal the check channel

**File:** `session/claude_controller.go`, `Start()` method (~line 221)

**Change:** Replace the current `SetOnOutput` closure:

```go
// BEFORE:
cc.responseStream.SetOnOutput(func() {
    cc.idleDetector.RecordActivity()
    if cc.rateLimitHandler != nil {
        cc.rateLimitHandler.NotifyOutput()
    }
})

// AFTER:
cc.responseStream.SetOnOutput(func() {
    cc.idleDetector.RecordActivity()
    if cc.rateLimitHandler != nil {
        cc.rateLimitHandler.NotifyOutput()
    }
    // Signal the status-check goroutine; non-blocking drop if already pending.
    if cc.statusChangeListener != nil {
        select {
        case cc.statusCheckCh <- struct{}{}:
        default:
        }
    }
})
```

**Why:** The `onOutput` closure is already called on every PTY read at PTY frequency (P2).
The non-blocking send costs ~10ns (no allocation, no subprocess). All expensive work moves
to the background goroutine started in Task 1.1.4. Satisfies R1.2.

---

#### Task 1.1.4 — Start the status-check background goroutine in `Start()`

**File:** `session/claude_controller.go`, `Start()` method (after `responseStream.Start`)

**Change:** Add a background goroutine that drains `statusCheckCh`:

```go
// Start status-change detection goroutine (exits when cc.ctx is cancelled).
if cc.statusChangeListener != nil {
    go cc.runStatusChangeLoop(cc.ctx)
}
```

And add the method:

```go
// runStatusChangeLoop drains statusCheckCh and fires the StatusChangeListener on
// actual status transitions. Runs as a single goroutine so GetCurrentStatus() is
// never called concurrently from this path. Exits when ctx is cancelled.
func (cc *ClaudeController) runStatusChangeLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-cc.statusCheckCh:
            // Read status under RLock, then release before calling listener (P6).
            newStatus, _ := cc.GetCurrentStatus()
            cc.mu.Lock()
            if newStatus == cc.lastEmittedStatus {
                cc.mu.Unlock()
                continue
            }
            cc.lastEmittedStatus = newStatus
            listener := cc.statusChangeListener
            cc.mu.Unlock()

            // Call listener outside cc.mu (P6 mitigation).
            if listener != nil {
                listener(newStatus, cc.sessionName)
            }
        }
    }
}
```

**Why:** The goroutine exits via `ctx.Done()` when `Stop()` calls `cc.cancel()`, preventing
goroutine leaks (P1, P4). The listener is called outside `cc.mu` preventing re-entrancy
deadlocks (P6). Dedup (`newStatus == cc.lastEmittedStatus`) satisfies R1.3.

---

#### Task 1.1.5 — Ensure `statusCheckCh` is initialized in `NewClaudeController`

**File:** `session/claude_controller.go`, `NewClaudeController` return literal

**Change:** Add `statusCheckCh: make(chan struct{}, 1)` to the struct literal in
`NewClaudeController`.

**Why:** Prevents nil-channel panic if `onOutput` sends before `Start()` wires the goroutine.
A send to a nil channel blocks forever; capacity-1 buffer allows the send to succeed.

---

### Story 1.2 — Wire StatusChangeListener in `instance_controller.go`

**Requirement(s):** R1.5, R4.2

#### Task 1.2.1 — Add `onStatusChange` field and `SetStatusChangeCallback` to `Instance`

**File:** `session/instance.go`

**Change:** Mirror the `onRateLimitDetected` pattern. Add to the `Instance` struct (adjacent to
`onRateLimitDetected`):

```go
onStatusChange   func(newStatus detection.DetectedStatus, sessionName string)
onStatusChangeMu sync.RWMutex
```

Add setter (adjacent to `SetRateLimitCallbacks`):

```go
// SetStatusChangeCallback registers a server-layer callback for controller status changes.
// Safe to call before or after the controller is started; callback is wired at controller
// start time via wireStatusChangeCallback.
func (i *Instance) SetStatusChangeCallback(fn func(detection.DetectedStatus, string)) {
    i.onStatusChangeMu.Lock()
    i.onStatusChange = fn
    i.onStatusChangeMu.Unlock()
    i.wireStatusChangeCallback(i.GetController())
}
```

**Why:** Keeps the server-layer callback stored on `Instance` (same as `onRateLimitDetected`),
so `instance_controller.go` can read it without importing `server/`. Satisfies R4.1.

---

#### Task 1.2.2 — Add `wireStatusChangeCallback` to `instance_controller.go`

**File:** `session/instance_controller.go`

**Change:** Add (adjacent to `wireRateLimitCallbacks`):

```go
// wireStatusChangeCallback wires the instance-level status-change callback to the
// ClaudeController. Called both from SetStatusChangeCallback and from StartController.
func (i *Instance) wireStatusChangeCallback(ctrl *ClaudeController) {
    if ctrl == nil {
        return
    }
    i.onStatusChangeMu.RLock()
    fn := i.onStatusChange
    i.onStatusChangeMu.RUnlock()
    if fn == nil {
        return
    }
    ctrl.SetStatusChangeListener(fn)
}
```

**Why:** Mirrors `wireRateLimitCallbacks` exactly. Handles both orderings: server calls
`SetStatusChangeCallback` before or after `StartController`. Satisfies R1.5.

---

#### Task 1.2.3 — Call `SetStatusChangeListener` before `controller.Start()` in `StartController`

**File:** `session/instance_controller.go`, `StartController()` method

**Change:** Insert before `controller.Start(context.Background())`:

```go
// Wire status-change listener if the server layer has already registered one.
// This must run before Start() so no events are lost (P3 mitigation).
i.wireStatusChangeCallback(controller)
```

**Why:** Ensures the listener is registered before `Start()` launches the PTY stream goroutine,
eliminating the first-event-loss race (P3). Mirrors `SetOnEOFCallback` placement.

Also call `i.wireStatusChangeCallback(controller)` at the end of `StartController` (after
`RegisterController`) only if the `i.onStatusChange` is non-nil and was set after the
controller started — the idempotent `wireStatusChangeCallback` handles this safely.

---

### Story 1.3 — Tests for Epic 1

**Requirement(s):** R5.4 (regressions must pass), R1.1–R1.5

#### Task 1.3.1 — Add unit tests in `session/claude_controller_test.go`

**File:** `session/claude_controller_test.go`

Add the following test functions using the existing `newControllerWithMock` helper:

- `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt`
  Verifies R1.1: listener fires when mock preview changes to approval content.
  Uses `cc.onOutputCallback()` trigger (a test-only helper that invokes the same
  non-blocking send as `SetOnOutput`).

- `TestClaudeController_StatusChangeCallback_SuppressedOnNoChange`
  Verifies R1.3: listener NOT called when status is identical across two signals.

- `TestClaudeController_StatusChangeCallback_UsesHashCache`
  Verifies R1.2: detection runs only on cache miss (content hash changed).

- `TestClaudeController_StatusChangeCallback_NotCalledAfterStop`
  Verifies P1 mitigation: no callback after `Stop()` even if `statusCheckCh` has a pending item.

**Pattern:** Use `sync.WaitGroup` or a buffered channel to capture async callback delivery.
Use `testutil/wait.FastWaitConfig()` for the timeout assertion.

---

#### Task 1.3.2 — Add integration test in `server/review_queue_manager_test.go`

**File:** `server/review_queue_manager_test.go`

Add `TestReactiveQueueManager_StatusChangeCallback_TriggersQueueUpdate`:
- Creates a `newReactiveQueueTestSetup(t)`
- Wires a fake `ClaudeController` (using `newControllerWithMock`) with a `StatusChangeListener`
  that calls `rqm.OnControllerStatusChange(inst)` directly
- Simulates an approval prompt appearing in the mock PTY content
- Asserts the review queue contains the instance within `wait.FastWaitConfig()` deadline
- Satisfies R1.5 end-to-end

---

## Epic 2 — Server-Layer Wiring

**Scope:** `server/services/session_service.go` and `server/review_queue_manager.go`.
No `session/` package changes. Satisfies R1.5, R3, R5.5.

**Priority:** High (depends on Epic 1 completion)

---

### Story 2.1 — Add `OnControllerStatusChange` to `ReactiveQueueManager`

**Requirement(s):** R1.5, R5.5

#### Task 2.1.1 — Add `OnControllerStatusChange` method to `review_queue_manager.go`

**File:** `server/review_queue_manager.go`

**Change:** Add a new exported method that can be called from the controller goroutine:

```go
// OnControllerStatusChange is called by the ClaudeController's status-change goroutine
// when it detects a terminal status transition. It dispatches a CheckSession call on the
// ReactiveQueueManager's processEvents goroutine via the statusChangeCh channel.
// Safe to call from any goroutine.
func (rqm *ReactiveQueueManager) OnControllerStatusChange(inst *session.Instance, newStatus detection.DetectedStatus) {
    // Signal poller to snap to fast interval (non-blocking, same as signalActivity).
    rqm.signalActivity()

    // Run an immediate check for this specific session.
    // CheckSession is synchronous; call it on a fresh goroutine to avoid blocking
    // the controller's status-change goroutine.
    go func() {
        select {
        case <-rqm.ctx.Done():
            return
        default:
        }
        rqm.poller.CheckSession(inst)
    }()
}
```

**Note on goroutine:** `CheckSession` calls `batchPaneActivity("")` (a subprocess). It must not
block the controller's background goroutine. A fire-and-forget goroutine per status change is
acceptable here because status changes are rare (approvals, completions) and the subprocess
finishes in <50ms. The `ctx.Done()` guard prevents work after manager shutdown (P1 analog).

**Alternative considered:** A dedicated `statusChangeCh chan *session.Instance` (capacity N)
consumed by `processEvents`. Rejected for complexity; the fire-and-forget goroutine is simpler
and correctness is unchanged since `CheckSession` is already idempotent.

**Why:** Keeps event routing logic in one place (`ReactiveQueueManager`). Satisfies R5.5
(existing `handleEvent` path is untouched).

---

#### Task 2.1.2 — Add `detection` import to `review_queue_manager.go`

**File:** `server/review_queue_manager.go`

**Change:** Add `"github.com/tstapler/stapler-squad/session/detection"` to imports.

**Why:** Required for the `detection.DetectedStatus` parameter type in `OnControllerStatusChange`.

---

### Story 2.2 — Wire status-change callbacks at instance creation

**Requirement(s):** R1.5, R4.2

#### Task 2.2.1 — Add `wireStatusChangeCallback` helper to `session_service.go`

**File:** `server/services/session_service.go`

**Change:** Add a method adjacent to `wireRateLimitCallbacks`:

```go
// wireStatusChangeCallback registers a ReactiveQueueManager callback on the instance
// so that ClaudeController status transitions immediately trigger a CheckSession call.
// Safe to call before or after the controller is started (mirrors wireRateLimitCallbacks).
func (s *SessionService) wireStatusChangeCallback(inst *session.Instance) {
    if inst == nil || s.reactiveQueueMgr == nil {
        return
    }
    rqm, ok := s.reactiveQueueMgr.(interface {
        OnControllerStatusChange(*session.Instance, detection.DetectedStatus)
    })
    if !ok {
        return
    }
    inst.SetStatusChangeCallback(func(newStatus detection.DetectedStatus, _ string) {
        rqm.OnControllerStatusChange(inst, newStatus)
    })
}
```

**Note on interface assertion:** `s.reactiveQueueMgr` is stored as `services.ReactiveQueueManager`
interface. Two options:
1. Add `OnControllerStatusChange` to the `services.ReactiveQueueManager` interface — preferred
   for testability; update the interface definition and the mock.
2. Use a local interface assertion as above — avoids interface churn.

**ADR flag:** Whether to extend the `services.ReactiveQueueManager` interface or use a local
type assertion should be captured in a decision record. Recommendation: extend the interface
(clean contract; mock generation is automated). See ADR flag at end of this document.

---

#### Task 2.2.2 — Call `wireStatusChangeCallback` at session creation

**File:** `server/services/session_service.go`

**Change:** At the two sites where `wireRateLimitCallbacks` is currently called (lines ~224
and ~708), add an adjacent call to `wireStatusChangeCallback`:

```go
s.wireRateLimitCallbacks(inst)
s.wireStatusChangeCallback(inst)   // NEW
```

**Why:** Ensures every instance (both loaded-from-storage and newly created) gets the callback
wired. Satisfies R1.5.

---

#### Task 2.2.3 — (Optional) Extend `services.ReactiveQueueManager` interface

**File:** `server/services/interfaces.go` (or wherever the interface is defined)

**Change:** Add:

```go
OnControllerStatusChange(inst *session.Instance, newStatus detection.DetectedStatus)
```

Update the mock implementation accordingly.

**Why:** Allows `wireStatusChangeCallback` to use the typed interface instead of a local
assertion, making the dependency explicit and mockable in tests.

---

### Story 2.3 — Adjust poller behavior for controller-managed sessions

**Requirement(s):** R3.1, R3.2, R3.3, R3.4

#### Task 2.3.1 — Increase `PollInterval` for controller-managed sessions

**File:** `session/review_queue_poller.go`

**Change:** In `checkSession` (or the fast-path poll loop), add a guard:

```go
// If the instance has an active controller, the event-driven path handles
// status changes; skip the fast-path poll and rely on the 30s reconciliation.
if inst.GetController() != nil {
    return
}
```

This guard applies only in the fast-path poll (i.e., polls at `PollInterval`, not
`ReconcileInterval`). The 30s reconciliation block continues to run for all sessions
(satisfying R3.2, R3.3).

**Note:** Verify that `SlowPollInterval` backoff logic (R3.4) is not inside the code path
being skipped. If it is, preserve the backoff signal by checking `inst.GetController() != nil`
only for the status-detection sub-section of `checkSession`, not the whole function.

**Why:** Removes redundant polling for controller-managed sessions. Satisfies R3.1, R3.2.
Non-controller sessions (external/attached) continue at the 2s scan rate (R3.3).

---

### Story 2.4 — Tests for Epic 2

**Requirement(s):** R5.4, R5.5, R3.2

#### Task 2.4.1 — Add `TestReviewQueuePoller_ControllerSessions_SkipFastPath`

**File:** `session/review_queue_poller_test.go`

Verifies R3.2: a session with an active controller does not trigger status detection in the
fast-path poll, but does run reconciliation.

#### Task 2.4.2 — Verify existing `TestReactiveQueueManager*` tests pass unchanged

Run `go test ./server/... -run TestReactiveQueueManager -race` and confirm zero failures.
Satisfies R5.4, R5.5.

---

## Epic 3 — Idle Timeout Event Emission (lower priority)

**Scope:** `session/detection/idle.go` + `session/claude_controller.go` + minor server wiring.
Satisfies R2.

**Priority:** Lower — implement after Epics 1 and 2 are merged and green.

---

### Story 3.1 — Add idle-timeout timer to `IdleDetector`

**Requirement(s):** R2.1, R2.2, R2.3, R2.4

#### Task 3.1.1 — Add timer fields and `SetOnTimeout` to `idle.go`

**File:** `session/detection/idle.go`

**Change:** Add to `IdleDetector` struct:

```go
onTimeout    func()       // called when idle threshold expires; set before first RecordActivity
timeoutTimer *time.Timer
timerMu      sync.Mutex   // separate from mu to avoid nesting with the state mutex
```

Add:

```go
// SetOnTimeout registers fn to be called once when the idle threshold expires without
// activity. The callback runs on a goroutine owned by the Go runtime (time.AfterFunc).
// It must not block. Reset by RecordActivity() on every activity event.
func (id *IdleDetector) SetOnTimeout(fn func()) {
    id.timerMu.Lock()
    id.onTimeout = fn
    id.timerMu.Unlock()
}

// StartIdleTimer arms the idle timer. Must be called once after SetOnTimeout.
// It is called by ClaudeController.Start() to avoid a goroutine leak on controllers
// that are created but never started.
func (id *IdleDetector) StartIdleTimer() {
    id.timerMu.Lock()
    defer id.timerMu.Unlock()
    if id.timeoutTimer != nil || id.onTimeout == nil {
        return
    }
    id.timeoutTimer = time.AfterFunc(id.config.IdleThreshold, func() {
        id.timerMu.Lock()
        fn := id.onTimeout
        id.timerMu.Unlock()
        if fn != nil {
            fn()
        }
    })
}

// StopIdleTimer disarms the timer. Called by ClaudeController.Stop().
func (id *IdleDetector) StopIdleTimer() {
    id.timerMu.Lock()
    defer id.timerMu.Unlock()
    if id.timeoutTimer != nil {
        id.timeoutTimer.Stop()
        id.timeoutTimer = nil
    }
}
```

**Why:** `time.AfterFunc` spawns a goroutine per firing (not per timer). A single persistent
`time.Timer` (which would require a drain goroutine) is more complex for this use case. Since
the timer fires at most once per idle threshold (5s default) and the callback is lightweight,
`time.AfterFunc` is acceptable. Satisfies R2.1–R2.4.

---

#### Task 3.1.2 — Reset timer in `RecordActivity()`

**File:** `session/detection/idle.go`, `RecordActivity()` method

**Change:** After updating `id.lastActivity`, add:

```go
// Reset idle timer under timerMu (separate from state mu to avoid nesting).
id.timerMu.Lock()
if id.timeoutTimer != nil {
    id.timeoutTimer.Reset(id.config.IdleThreshold)
}
id.timerMu.Unlock()
```

**Why:** The 500ms `minActivityInterval` debounce (already in `RecordActivity()`) gates how
often the timer is reset. At most 2 resets/second during active output — acceptable overhead.
Satisfies R2.2, R2.4.

---

#### Task 3.1.3 — Call `StartIdleTimer` and `StopIdleTimer` from `ClaudeController`

**File:** `session/claude_controller.go`

**Change:**

In `Start()`, after `cc.rateLimitHandler.Start()`:

```go
// Arm idle timer after listener is registered.
if cc.idleDetector != nil {
    cc.idleDetector.StartIdleTimer()
}
```

In `Stop()`, before `cc.responseStream.Stop()`:

```go
if cc.idleDetector != nil {
    cc.idleDetector.StopIdleTimer()
}
```

**Why:** Ensures the timer is stopped when the controller stops, preventing post-Stop callbacks
(P4 mitigation). Satisfies R2.1.

---

### Story 3.2 — Wire idle-timeout callback through Instance to ReactiveQueueManager

**Requirement(s):** R2.1, R4.1

#### Task 3.2.1 — Add `SetIdleTimeoutCallback` to `Instance`

**File:** `session/instance.go` and `session/instance_controller.go`

**Change:** Add `onIdleTimeout func()` field and `SetIdleTimeoutCallback` setter to `Instance`,
mirroring the `onStatusChange` field. In `wireStatusChangeCallback`, also call:

```go
ctrl.idleDetector.SetOnTimeout(func() {
    i.onIdleTimeout() // calls up to server layer
})
```

**Why:** Keeps the import boundary clean (R4.1). The timer fires into `Instance` which forwards
to the server layer via the registered func.

---

#### Task 3.2.2 — Wire `onIdleTimeout` callback in `session_service.go`

**File:** `server/services/session_service.go`

**Change:** Add `wireIdleTimeoutCallback` adjacent to `wireStatusChangeCallback`:

```go
func (s *SessionService) wireIdleTimeoutCallback(inst *session.Instance) {
    if inst == nil || s.reactiveQueueMgr == nil {
        return
    }
    inst.SetIdleTimeoutCallback(func() {
        s.reactiveQueueMgr.CheckSession(inst) // or OnControllerStatusChange variant
    })
}
```

Call it alongside `wireStatusChangeCallback` at the same two session-creation sites.

**Why:** Satisfies R2.1 (idle event delivered without waiting for poll cycle). Satisfies R2.3
(reuses existing `IdleDetector` infrastructure).

---

### Story 3.3 — Tests for Epic 3

**Requirement(s):** R2.1–R2.4, R5.4

#### Task 3.3.1 — Add `TestAdaptivePoller_IdleTimerFiresWithoutPoll`

**File:** `session/review_queue_poller_test.go` or new `session/idle_timer_test.go`

Verifies R2.1: simulate an idle threshold expiry without a poll cycle; assert `CheckSession`
is called via the timeout callback.

#### Task 3.3.2 — Add `TestIdleDetector_ResetOnRecordActivity`

**File:** `session/detection/idle_test.go`

Verifies R2.2: timer is reset when `RecordActivity()` is called within the threshold.

---

## Cross-Cutting Concerns

### Import guard

Add a compile-time assertion at the `session/` package boundary to enforce R4.1. This can be
a build-tag constrained file or a `//go:build` directive linting rule (if `golangci-lint` is
configured with `depguard`). A simpler approach: add a comment in `CLAUDE.md` and enforce via
`go list` in CI.

### Goroutine leak detection

No `goleak` is in use. For the background goroutine (Task 1.1.4), tests should call
`controller.Stop()` and then assert the goroutine exits within 1s using `runtime.NumGoroutine()`
comparison or a done channel. Use the existing `testutil.WaitForCondition` pattern.

### `GetCurrentStatus()` concurrent write under RLock (Pitfall 6)

The existing `statusCache` write happens under `cc.mu.RLock()` which is a latent data race
when the poller and the new goroutine call `GetCurrentStatus()` concurrently. A correct fix
is to use a separate `cacheMu sync.Mutex` for `statusCache` and `idleCache`, or promote the
miss branch to a write-lock by releasing and re-acquiring. **This fix is a pre-requisite for
Task 1.1.4 to be race-free.** Include as a sub-task of Story 1.1 or as a separate story:

#### Task 1.1.0 — Fix `statusCache` write-under-RLock data race (pre-requisite)

**File:** `session/claude_controller.go`

**Change:** Add `cacheMu sync.Mutex` to `ClaudeController`. Replace all writes to
`cc.statusCache` and `cc.idleCache` with `cc.cacheMu.Lock()` / `cc.cacheMu.Unlock()` sections,
while keeping `cc.mu.RLock()` for reading the `instance` and `sessionName` fields.

**Why:** Without this fix, `go test -race` will flag a data race when the new goroutine and
the poller goroutine both call `GetCurrentStatus()` concurrently and both miss the cache.
This is a correctness fix that must land before Task 1.1.4.

---

## ADR Flags

| Decision | Options | Recommendation |
|----------|---------|----------------|
| ADR-1: Extend `services.ReactiveQueueManager` interface vs. local type assertion for `OnControllerStatusChange` | (A) Add method to interface + update mock; (B) local `interface{ OnControllerStatusChange(...) }` assertion | Extend the interface (A). Clean contract, mock-safe, no reflect overhead. File as ADR in `decisions/`. |
| ADR-2: `time.AfterFunc` vs single `time.NewTimer` + drain goroutine for idle timer | (A) `time.AfterFunc` — simple, no drain goroutine; (B) `time.NewTimer` — goroutine needed to drain channel | `time.AfterFunc` (A) for Epic 3. Lower complexity; timer fires rarely. |
| ADR-3: `OnControllerStatusChange` calls `CheckSession` on a fire-and-forget goroutine vs. queuing to `processEvents` | (A) fire-and-forget goroutine per status change; (B) dedicated `statusChangeCh` consumed by `processEvents` | Fire-and-forget (A) for initial implementation. Revisit if status changes become high-frequency (unlikely: approvals are rare). |

---

## Summary

| Dimension | Count |
|-----------|-------|
| Epics | 3 |
| Stories | 9 (E1: 3, E2: 4, E3: 3, but E3.2 has sub-tasks inside Story 3.2) |
| Tasks | 21 (E1: 8 incl. pre-req 1.1.0; E2: 9; E3: 6; cross-cutting: not counted separately) |
| ADR flags | 3 |

### Dependency Order

```
Task 1.1.0 (cacheMu fix)
    ↓
Story 1.1 (cc fields + goroutine)
    ↓
Story 1.2 (instance wiring)    Story 2.1 (ReactiveQueueManager method)
    ↓                              ↓
Story 1.3 (session tests)     Story 2.2 + 2.3 (server wiring + poller)
                                   ↓
                              Story 2.4 (server tests)
                                   ↓
                     [Epic 3 begins here — independent after E1+E2 green]
```

### File Change Summary

| File | Epic | Change type |
|------|------|-------------|
| `session/claude_controller.go` | E1 | New fields, goroutine, setter, onOutput extension |
| `session/instance.go` | E1 | New callback field + setter |
| `session/instance_controller.go` | E1 | `wireStatusChangeCallback` + call before `Start()` |
| `session/claude_controller_test.go` | E1 | 4 new unit tests |
| `server/review_queue_manager.go` | E2 | `OnControllerStatusChange` method + import |
| `server/services/session_service.go` | E2 | `wireStatusChangeCallback` helper + 2 call sites |
| `server/services/interfaces.go` | E2 | `OnControllerStatusChange` on interface (if ADR-1=A) |
| `session/review_queue_poller.go` | E2 | Controller-session skip in fast-path poll |
| `session/review_queue_poller_test.go` | E2+E3 | 2 new tests |
| `server/review_queue_manager_test.go` | E2 | 1 new integration test |
| `session/detection/idle.go` | E3 | Timer fields, `SetOnTimeout`, `StartIdleTimer`, `StopIdleTimer`, `RecordActivity` reset |
| `session/detection/idle_test.go` | E3 | Timer reset test |

### Pattern Reuse Summary

| Pattern reused | Source | Used by |
|----------------|--------|---------|
| `SetOnEOFCallback` (field + setter, before-Start contract) | `claude_controller.go:83` | Task 1.1.2 |
| `notifyCh` capacity-1 channel burst coalescing | `ratelimit/integration.go:100` | Task 1.1.3 + 1.1.4 |
| `wireRateLimitCallbacks` (instance field → controller wiring) | `instance_controller.go:193` | Task 1.2.2 |
| `SetRateLimitCallbacks` (safe before-or-after-start setter) | `instance_controller.go:180` | Task 1.2.1 |
| `signalActivity()` non-blocking channel send | `review_queue_manager.go:163` | Task 2.1.1 |
| `handleUserInteraction` → `poller.CheckSession(inst)` dispatch | `review_queue_manager.go:202` | Task 2.1.1 |
| `wireRateLimitCallbacks` call in `session_service.go` | `session_service.go:224,708` | Task 2.2.2 |
