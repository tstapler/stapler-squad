# Feature Research: Review Queue Event-Driven Architecture

## Research Questions & Findings

---

### Q1: The existing OnEOF callback pattern — what does it look like and can we use the same pattern for StatusChange?

**Pattern:**
```go
// ClaudeController field (claude_controller.go:72)
onEOFCallback func()

// Setter (claude_controller.go:83)
func (cc *ClaudeController) SetOnEOFCallback(fn func()) {
    cc.onEOFCallback = fn
}

// Wired at Start() time (claude_controller.go:231)
if cc.onEOFCallback != nil {
    cc.responseStream.OnEOF = cc.onEOFCallback
}
```

**Caller (instance_controller.go:54):**
```go
controller.SetOnEOFCallback(func() {
    // transition instance state
    // fire lifecycle event
})
```

**Conclusion:** The pattern is a plain `func()` field + a `Set*` setter called *before* `Start()`. The same pattern is directly applicable for `StatusChangeListener`:

```go
// New field on ClaudeController
onStatusChange func(status detection.DetectedStatus, desc string)

// New setter
func (cc *ClaudeController) SetStatusChangeListener(fn func(detection.DetectedStatus, string)) {
    cc.onStatusChange = fn
}
```

The listener would be called inside the `SetOnOutput` callback (inside `responseStream.streamLoop`), after hash-checking and status detection, when `newStatus != cc.statusCache.status`. This satisfies R1.1–R1.4.

**Goroutine context:** The `onEOFCallback` fires from the `responseStream.streamLoop` goroutine. The `StatusChangeListener` would also fire from the same goroutine (via `SetOnOutput`). The callback must not block or acquire locks owned by the stream loop — the same constraint as `onEOFCallback`.

---

### Q2: How does ReactiveQueueManager currently call CheckSession() — what does it pass and from which goroutine?

**Current calls (review_queue_manager.go:202):**
```go
// Called from processEvents() goroutine, which processes the EventBus channel
rqm.poller.CheckSession(inst)
```

**`CheckSession` implementation (review_queue_poller.go:1165):**
```go
func (rqp *ReviewQueuePoller) CheckSession(inst *Instance) {
    rqp.checkSession(inst, batchPaneActivity(""))
}
```

It calls `batchPaneActivity("")` (a subprocess call to `tmux list-panes`) inline on the calling goroutine. The full `checkSession` logic then runs synchronously: status detection, content fetch, queue add/remove decision.

**Implication for the new path:** When a `StatusChangeListener` fires from the `responseStream.streamLoop` goroutine, it must **not** call `CheckSession` directly on that goroutine (would block PTY reads). It should either:
1. Signal the `activityCh` (non-blocking `chan struct{}{1}` send) to wake the poll loop immediately, or
2. Dispatch to a dedicated goroutine/channel inside `ReactiveQueueManager`.

The `signalActivity()` helper (review_queue_manager.go:163) already implements the non-blocking signal:
```go
func (rqm *ReactiveQueueManager) signalActivity() {
    select {
    case rqm.activityCh <- struct{}{}:
    default:
    }
}
```

A new `handleStatusChange` method on `ReactiveQueueManager` should call `CheckSession` from its own `processEvents` goroutine, not inline in the callback.

---

### Q3: Is there a pattern for debounced/coalesced callbacks already in the codebase?

**Yes — two patterns exist:**

**Pattern A: Buffered channel coalescing (ratelimit/integration.go:100–112)**
```go
notifyCh: make(chan struct{}, 1),  // buffer=1 coalesces bursts

func (pc *PTYConsumer) NotifyOutput() {
    select {
    case pc.notifyCh <- struct{}{}:
    default:  // already pending, drop
    }
}
```
The poll loop reads from `notifyCh` and processes the *current* buffer content, not individual events. This means N rapid notifications collapse to 1 processing call. This is precisely the debounce model needed.

**Pattern B: activityCh signal channel (review_queue_poller.go:172–178)**
```go
// Buffered channel(1) — snaps poll loop to fast interval
func (rqp *ReviewQueuePoller) SetActivityChannel(ch <-chan struct{}) {
    rqp.activityCh = ch
}
```
Used by `ReactiveQueueManager.signalActivity()` to wake the poller immediately.

**Conclusion:** For StatusChange events from PTY output, the `notifyCh` pattern (buffered 1-slot channel) is the right model. The `ClaudeController` would send to a `statusChangeCh chan struct{}{1}`, and a background goroutine in `ReactiveQueueManager` would drain it and call `CheckSession`. This collapses burst output (e.g., Claude streaming a response) into a single check.

Alternatively, the `StatusChangeListener` can simply call `signalActivity()` directly (since both are non-blocking), which already wakes the poll loop from its slow-poll backoff. The decision depends on whether a dedicated `handleStatusChange` path is needed for correctness (e.g., to pass session context through).

---

### Q4: How does `ratelimit.PTYConsumer.NotifyOutput()` work — is it a relevant pattern for debouncing?

**Implementation (ratelimit/integration.go:107–112):**
```go
// notifyCh has buffer=1 — extra signals are silently dropped
func (pc *PTYConsumer) NotifyOutput() {
    select {
    case pc.notifyCh <- struct{}{}:
    default:
    }
}
```

**Poll loop consumes it (ratelimit/integration.go:147–151):**
```go
case <-pc.notifyCh:
    data := pc.buffer.GetRecentOutput(4096)
    if len(data) > 0 {
        pc.manager.ProcessOutput(data)
    }
```

**How it's wired (claude_controller.go:221–225):**
```go
cc.responseStream.SetOnOutput(func() {
    cc.idleDetector.RecordActivity()
    if cc.rateLimitHandler != nil {
        cc.rateLimitHandler.NotifyOutput()  // ← debounce notify
    }
})
```

**Relevance:** This is highly relevant. The `StatusChangeListener` path can follow the *exact same model*:
- Add `statusChangeCh chan struct{}` (buffer=1) to `ClaudeController`
- In `SetOnOutput` callback, after status detection detects a change, do a non-blocking send to `statusChangeCh`
- A goroutine inside `ReactiveQueueManager` (or a callback wired via `SetStatusChangeListener`) drains `statusChangeCh` and calls `CheckSession`

This gives <1s latency (fires on next PTY read after the status line appears) with zero-overhead burst coalescing. The existing `minActivityInterval = 500ms` in `idleDetector` provides natural debounce for idle events.

---

### Q5: What is `InstanceStatusManager.GetController()` — is it the right place to register StatusChangeListener callbacks?

**Implementation (instance_status.go:52–57):**
```go
func (ism *InstanceStatusManager) GetController(instanceTitle string) (*ClaudeController, bool) {
    ism.mu.RLock()
    defer ism.mu.RUnlock()
    controller, exists := ism.controllers[instanceTitle]
    return controller, exists
}
```

**Current usages:** `GetController` on `InstanceStatusManager` is NOT called anywhere in `server/` today. Controllers are registered via `ism.RegisterController(title, controller)` from `ControllerManager.RegisterController`, which is called from `instance_controller.go:83` inside `StartController()`.

**Where callbacks should be registered:** The right place is `instance_controller.go`'s `StartController()`, *after* `controller.Start()` succeeds and *before* `RegisterController` is called. This mirrors the `SetOnEOFCallback` pattern exactly:

```go
// instance_controller.go (inside StartController, after controller.Start())
controller.SetOnEOFCallback(func() { ... })         // existing
controller.SetStatusChangeListener(func(s, d) { ... }) // new

// Register with status manager (stores controller)
i.controllerManager.RegisterController(i.Title, controller)
```

However, to avoid circular imports (R4.1), the `StatusChangeListener` function cannot be a closure defined in `instance_controller.go` that calls into `server/`. Instead:
- `Instance.SetStatusChangeCallback(fn func(detection.DetectedStatus, string))` stores the callback as an `Instance` field (same pattern as `onRateLimitDetected`)
- The `server/` layer calls `inst.SetStatusChangeCallback(...)` after creating the instance (same wiring point as `SetRateLimitCallbacks`)
- `StartController()` then reads `i.onStatusChange` and wires it into the controller (same as `wireRateLimitCallbacks`)

This preserves the no-circular-import constraint and reuses the proven `SetRateLimitCallbacks` / `wireRateLimitCallbacks` pattern verbatim.

**Key call site in server:** `server/dependencies.go:549` creates `ReactiveQueueManager`. The wiring of `StatusChangeCallback` would happen either in the `BuildRuntimeDeps` goroutine (step 7, after `StartController`) or via a new `ReactiveQueueManager` method called during startup. The `InstanceStatusManager.GetController()` method itself is not the registration point — it's a lookup utility that could be used later for health checks.

---

## Summary of Design Decisions

| Question | Conclusion |
|---|---|
| StatusChange callback shape | `func(detection.DetectedStatus, string)` plain func, set via `SetStatusChangeListener` before `Start()` |
| Where to fire it | Inside `SetOnOutput` callback after hash-cache check detects `newStatus != lastEmitted` |
| Debounce mechanism | Buffered `chan struct{}{1}` + non-blocking send (mirror of `NotifyOutput`) |
| Who calls CheckSession | `ReactiveQueueManager.processEvents` goroutine, not the PTY goroutine |
| Registration point | `instance_controller.go:StartController()` via `wireRateLimitCallbacks` pattern; server wires via `inst.SetStatusChangeCallback()` |
| Circular import avoidance | `Instance` holds `onStatusChange func` field; wired from `server/` before controller start, same as `onRateLimitDetected` |
