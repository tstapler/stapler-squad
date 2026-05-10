# Architecture Research: Event-Driven Review Queue

## Q1: PTY output → RecordActivity() → status cache call path

### Exact trace

```
PTY bytes arrive (tmux pane output)
  ↓
ResponseStream (session/response_stream.go)
  → reads from PTYAccess circular buffer
  → calls cc.responseStream.SetOnOutput callback (set in Start())

SetOnOutput closure (claude_controller.go:221–226):
  → cc.idleDetector.RecordActivity()         // debounced (500ms), updates lastActivity
  → cc.rateLimitHandler.NotifyOutput()       // rate limit detection

[No status cache update here — RecordActivity() only updates lastActivity timestamp]
```

### Status cache (statusCache / idleCache)

`statusCache` and `idleCache` are only updated when `GetCurrentStatus()` or `GetIdleState()` are explicitly called — neither is invoked from the `OnOutput` closure. Today the status cache path is:

```
External caller (e.g. ReviewQueuePoller.checkSession via statusManager.GetStatus())
  → InstanceStatusManager.GetStatus(inst)
    → controller.GetCurrentStatus()          // reads inst.Preview() (tmux capture-pane)
      → tailContent() → hashString()
      → if hash == statusCache.tailHash → return cached result (O(1))
      → else → statusDetector.DetectWithContextFromLines(lastNLines())
               → cc.statusCache = new entry

    → controller.GetIdleStateInfo()
      → GetIdleState()
        → reads inst.Preview()
        → tailContent() → hashString()
        → if hash == idleCache.tailHash → return cached state
        → else → idleDetector.DetectStateFromContent()
                 → cc.idleCache = new entry
```

**Key finding**: The `OnOutput` callback only touches `idleDetector.lastActivity`. It does NOT run status detection or update `statusCache`. Status detection only happens when `GetCurrentStatus()` / `GetIdleState()` are polled from outside the controller (currently only by the poller or tests).

---

## Q2: Where to inject StatusChangeListener — minimal changes, no circular imports

### Recommended injection point: `ClaudeController.SetOnOutput` closure in `Start()`

The cleanest place to inject a `StatusChangeListener` is **inside the existing `SetOnOutput` closure** (claude_controller.go line 221). This closure already fires on every PTY write. We extend it to:

1. Call `GetCurrentStatus()` (uses the existing hash cache — O(1) on cache hit)
2. Compare against a new `lastEmittedStatus` field on `ClaudeController`
3. If changed, call the registered `StatusChangeListener` func

```go
// New field on ClaudeController:
type StatusChangeListener func(newStatus detection.DetectedStatus, sessionName string)

// New fields on ClaudeController struct:
statusChangeListener  StatusChangeListener
lastEmittedStatus     detection.DetectedStatus

// New setter (no circular import — plain func type):
func (cc *ClaudeController) SetStatusChangeListener(fn StatusChangeListener) {
    cc.mu.Lock()
    cc.statusChangeListener = fn
    cc.mu.Unlock()
}
```

Extended `SetOnOutput` closure:
```go
cc.responseStream.SetOnOutput(func() {
    cc.idleDetector.RecordActivity()
    if cc.rateLimitHandler != nil {
        cc.rateLimitHandler.NotifyOutput()
    }
    // NEW: status-change event
    if cc.statusChangeListener != nil {
        newStatus, _ := cc.GetCurrentStatus()  // hash-cached, O(1) on hit
        cc.mu.Lock()
        if newStatus != cc.lastEmittedStatus {
            cc.lastEmittedStatus = newStatus
            listener := cc.statusChangeListener
            cc.mu.Unlock()
            listener(newStatus, cc.sessionName)
        } else {
            cc.mu.Unlock()
        }
    }
})
```

### Why this location, not InstanceStatusManager

- `ClaudeController` has the PTY event source (`OnOutput`) — it's the earliest detection point
- `InstanceStatusManager` is a passive registry; it doesn't receive PTY events
- Putting the listener on `ClaudeController` (a `session/` type) avoids importing `server/`
- The listener is a plain `func` — no interface, no import chain

### Import safety

`ClaudeController` already imports `session/detection` but **not** `server/`. The `StatusChangeListener` type is defined in `session/` as a plain func type, so `server/` code can implement it and pass it in via `SetStatusChangeListener()` without creating a cycle.

### Wiring point: `InstanceStatusManager.RegisterController()` or `instance_controller.go:StartController()`

`StartController()` (instance_controller.go:17) calls `controller.Start()` then `cm.RegisterController()`. The server layer (`BuildRuntimeDeps`) calls `inst.StartController()`. The correct wiring location is either:
- **`InstanceStatusManager.RegisterController()`** — wire the listener inside the manager itself (requires `InstanceStatusManager` to hold a callback factory), or
- **`server/dependencies.go` step 7** — after `inst.StartController()` succeeds, call `inst.GetController().SetStatusChangeListener(...)` to inject a closure that calls `reactiveQueueMgr.CheckSession(inst)`

The latter (step 7 callback wiring in `BuildRuntimeDeps`) is the pattern already used for rate-limit callbacks (`SetRateLimitCallbacks`) and is the cleanest separation.

---

## Q3: How IdleDetector.DetectState() is called today, and where to add idle-timeout callback

### Current call sites

`DetectState()` is marked `DEPRECATED` in idle.go (line 92). The preferred path is `DetectStateFromContent()` called from `ClaudeController.GetIdleState()` (claude_controller.go:704–727). The poller calls it indirectly:

```
ReviewQueuePoller.checkSession()
  → statusManager.GetStatus(inst)
    → controller.GetIdleStateInfo()
      → GetIdleState()
        → DetectStateFromContent(filtered content)   ← primary path
        → DetectState()                               ← fallback (Preview error)
```

**`RecordActivity()` on `IdleDetector`** is the only method called from `OnOutput`. It does not call `DetectState()` — it only updates `lastActivity`.

### Cleanest place to add idle-timeout callback

**Option A — timer inside `IdleDetector`** (matches requirements R2.1–R2.4):

Add a `time.Timer` to `IdleDetector` that is reset on every `RecordActivity()` call and fires after `IdleThreshold`. When it fires, call a registered `func()` callback:

```go
// IdleDetector additions:
onTimeout      func()           // fired once when timer expires
timeoutTimer   *time.Timer
timeoutMu      sync.Mutex

func (id *IdleDetector) SetOnTimeout(fn func()) { ... }

func (id *IdleDetector) RecordActivity() {
    // existing debounce logic ...
    id.timeoutMu.Lock()
    if id.timeoutTimer != nil {
        id.timeoutTimer.Reset(id.config.IdleThreshold)
    }
    id.timeoutMu.Unlock()
}
```

The timer fires `onTimeout()` on its own goroutine; the callback (provided by `server/`) calls `reactiveQueueMgr.CheckSession(inst)`.

**Option B — detect state change in `OnOutput`** (piggybacks on Q2 solution):

If `GetCurrentStatus()` is called in `OnOutput` anyway (Q2), then transitioning from `StatusActive → StatusIdle/StatusReady` already signals a potential idle condition. The `StatusChangeListener` can check whether the new status maps to `IdleStateTimeout` without a separate timer.

**Recommendation**: Option A (dedicated timer) matches R2.1–R2.4 exactly and avoids coupling idle-timeout detection to PTY write frequency. The 500ms `minActivityInterval` debounce already throttles timer resets, satisfying R2.4.

---

## Q4: InstanceStatusManager.GetStatus() — right intermediary or direct callbacks?

### What GetStatus() does

`GetStatus(inst *Instance)` (instance_status.go:73) is a synchronous aggregator:
1. Looks up the controller by `inst.Title`
2. Calls `controller.GetCurrentStatus()` — triggers tmux capture-pane + hash-cache lookup
3. Calls `controller.GetIdleStateInfo()`
4. Returns a value struct (`InstanceStatusInfo`)

It does **not** push events anywhere. It is a pull-based query, not a push-based emitter.

### Should callbacks go through InstanceStatusManager?

**No.** `InstanceStatusManager` is the wrong intermediary for push-based status change events:

- It has no goroutine, no channel, no subscriber list — it is purely a registry + query facade
- Making it emit events would require adding goroutine infrastructure (channels, locks) that duplicates what `ClaudeController` already has
- It would also require `InstanceStatusManager` to import an event type from `server/`, creating a cycle (R4.1)

**Correct design**: callbacks go **directly from `ClaudeController`** (via `StatusChangeListener`) to `ReactiveQueueManager`. `InstanceStatusManager` remains a read-only registry used by the poller and the status display path.

`InstanceStatusManager` still plays a role in **wiring**: `RegisterController()` is the natural place to attach the listener to a newly created controller (since it has the `controller` reference and is called from `StartController()`). But it does this as a forwarding action, not as an event broker.

---

## Q5: Existing debounce/rate-limit mechanism to reuse

### minActivityInterval (500ms) — `session/detection/idle.go:245`

```go
const minActivityInterval = 500 * time.Millisecond

func (id *IdleDetector) RecordActivity() {
    id.mu.Lock()
    defer id.mu.Unlock()
    if id.timeNow().Sub(id.lastActivity) < minActivityInterval {
        return // no-op: too soon since last update
    }
    id.lastActivity = id.timeNow()
}
```

This is the primary debounce gate for PTY-driven activity. It prevents `lastActivity` (and any derived timer resets) from thrashing at PTY write frequency. **Reuse this for status-change emission**: since `RecordActivity()` is already called from `OnOutput` and is debounced, calling `GetCurrentStatus()` inside `OnOutput` will at most run status detection once per 500ms on an active session (because `lastActivity` only changes every 500ms, and the hash cache suppresses re-detection when content hasn't changed).

### Hash cache in ClaudeController — O(1) on cache hit

`statusCache` (a `statusCacheEntry` struct with `tailHash uint64`) suppresses repeat status detection when PTY tail content is unchanged. This is already the primary rate-limit mechanism for `GetCurrentStatus()`. No additional debouncing is needed for the status-change listener — the hash cache acts as a natural gate.

### activityCh / signalActivity() — `server/review_queue_manager.go:163`

An existing `chan struct{}` (buffer 1) is used to snap the poll loop back to its fast interval. This is a coarse snap signal, not a per-event debouncer. **Reuse `signalActivity()`** alongside the new `StatusChangeListener` path: the listener calls `reactiveQueueMgr.CheckSession(inst)` directly (bypassing the poll loop) and optionally also calls `signalActivity()` to keep the poll loop interval fast while the session is active.

### Summary of mechanisms to reuse

| Mechanism | Location | Role in new design |
|---|---|---|
| `minActivityInterval` (500ms) | `detection/idle.go` | Throttles timer resets and `lastActivity` updates from PTY writes |
| `statusCache` (hash gate) | `claude_controller.go` | Suppresses `GetCurrentStatus()` re-computation on unchanged content |
| `activityCh` / `signalActivity()` | `review_queue_manager.go` | Snaps poll loop to fast interval; still useful alongside event path |
| Existing `CheckSession(inst)` export | `review_queue_poller.go:1165` | Immediate per-session check; reused by `StatusChangeListener` and idle timer callback |

---

## Summary

1. **PTY output → status cache**: The `OnOutput` closure (in `ClaudeController.Start()`) only calls `RecordActivity()` today — it does NOT run status detection. The status cache (`statusCache`) is only updated lazily when an external caller (poller/status manager) calls `GetCurrentStatus()`. Adding status detection in `OnOutput` is the correct event-driven extension point.

2. **Injection point with no circular imports**: Add `SetStatusChangeListener(fn StatusChangeListener)` to `ClaudeController`. Wire it from `server/dependencies.go` step 7 (after `inst.StartController()` succeeds) using the same pattern already used for `SetRateLimitCallbacks`. This keeps `session/` free of `server/` imports.

3. **Idle timeout callback**: Add a `time.Timer` inside `IdleDetector` (reset by `RecordActivity()`) that fires an `onTimeout` func when `IdleThreshold` expires without activity. The existing 500ms `minActivityInterval` debounce already throttles timer resets at the right frequency; no new debounce infrastructure is needed.
