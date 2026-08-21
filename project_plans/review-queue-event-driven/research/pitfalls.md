# Pitfalls and Failure Modes — Event-Driven Review Queue

_Research date: 2026-05-09_

---

## 1. Goroutine Lifecycle: Callback Fires After Controller Is Stopped

### What the code does

`ClaudeController.Stop()` calls `cc.cancel()` and then `cc.responseStream.Stop()`. The
`streamLoop` goroutine notices `rs.ctx.Done()` and exits. `onOutput` is called **inside**
`streamLoop`, without holding `cc.mu`. `Stop()` **does** hold `cc.mu.Lock()` for its entire
duration, but `onOutput` runs in a separate goroutine and is not serialized with `Stop()`.

### Race window

```
goroutine A (streamLoop)          goroutine B (caller)
  reads PTY data (n > 0)
                                    cc.Stop() acquires cc.mu.Lock()
  calls rs.onOutput()               [blocked: goroutine A still in streamLoop]
    → idleDetector.RecordActivity() [safe: has its own mu]
    → rateLimitHandler.NotifyOutput()
  ← returns
                                    cc.mu acquired; cancel(); responseStream.Stop()
```

The existing `onOutput` closure (`RecordActivity` + `rateLimitHandler.NotifyOutput`) is safe
against this ordering because both targets guard themselves with their own mutexes.

**However**, for the proposed `StatusChangeListener`:

- If the listener is a **plain closure** set via `cc.SetStatusChangeListener(fn)` and `fn`
  calls `GetCurrentStatus()`, then `GetCurrentStatus()` tries to acquire `cc.mu.RLock()`.
- `Stop()` holds `cc.mu.Lock()`. If `onOutput` fires and calls `GetCurrentStatus()` while
  `Stop()` is waiting (or already holding the write-lock), the read-lock attempt blocks until
  `Stop()` finishes — and `Stop()` sets `cc.instance = nil` **before** releasing the lock
  (it only does `cc.ctx = nil; cc.cancel = nil`, not `cc.instance = nil`). So after
  `Stop()` completes, `GetCurrentStatus()` will call `cc.instance.Preview()` on a now-stopped
  controller. That is not a nil-pointer crash (instance is not nil-ed), but it may return
  stale or error data and fire a spurious listener call.
- More importantly: if `GetCurrentStatus()` is called **from inside onOutput without
  re-entering cc.mu** (i.e., the proposed implementation skips the lock when called inline),
  there is no locking protection for `cc.statusCache` (a plain struct, not atomic).

**Nil-pointer risk rating: LOW for existing fields; MEDIUM for any new fields that are
zero-initialized after Stop().** Specifically, if `cc.idleDetector` or `cc.statusDetector`
is set to nil during Stop() (they currently are not), an `onOutput` callback that reads them
would panic.

**Recommendation:**
- `onOutput` must check a `stopped` atomic flag before calling `GetCurrentStatus()` or the
  listener, or the listener must be cleared to nil synchronously with `cc.cancel()`.
- If `GetCurrentStatus()` is called from `onOutput`, it must NOT re-acquire `cc.mu`
  (deadlock risk with Stop()); instead cache state in a separate lock or call an internal
  unprotected helper.

---

## 2. OnOutput Fires on Every PTY Write — High-Frequency Call Risk

### Measured call rate

The `streamLoop` reads in 4 KB chunks (`readBuf := make([]byte, 4096)`). During active
Claude sessions (code generation, streaming output) the PTY can emit many KB per second.
Assuming 20–50 KB/s output, `onOutput` fires at **5–12 calls/second** in a busy session.
During rapid builds or test output, bursts can reach 50–100 calls/second (multiple reads per
100 ms deadline window).

### Current cost of onOutput (existing)

`RecordActivity()` acquires `idleDetector.mu.Lock()` and does a time comparison — O(1),
sub-microsecond, safe at high frequency. `rateLimitHandler.NotifyOutput()` sends on a
buffered channel; if the channel is full the send is dropped. Total cost: ~100–500 ns.

### Proposed additional work: GetCurrentStatus() in onOutput

`GetCurrentStatus()` is explicitly documented as cache-optimized:
- Acquires `cc.mu.RLock()` (read-lock, shared with other readers).
- Calls `cc.instance.Preview()` only on cache miss — **but preview is a tmux
  `capture-pane` subprocess call**, not a buffer read. For controller-managed sessions,
  `cc.instance` is backed by an `Instance` with a `ClaudeController`, and `Preview()`
  calls `cc.ptyAccess.GetBuffer()` (fast, no subprocess) **unless** the instance has no
  controller — which cannot happen inside ClaudeController itself.

Looking at `GetCurrentStatus()` (line 488):
```go
content, err := cc.instance.Preview()
```

`cc.instance` is an `InstanceContext`. For production wiring, `instance` is a `*session.Instance`.
`Instance.Preview()` ultimately calls `tmux capture-pane` (a subprocess). This means:

**On a cache HIT** (same tail hash): cost is `RLock + hash compare + RUnlock` — ~200 ns.
**On a cache MISS**: cost is `RLock + capture-pane subprocess (~10–50 ms) + detection + RUnlock`.

The cache only hits when content has not changed since the last call. During active output
(which is exactly when `onOutput` fires most), the buffer changes on every read, so the hash
changes on **every call** and every call spawns a subprocess.

**This is a critical performance flaw.** At 50 calls/second, each triggering a 10 ms
`capture-pane` process, the system would spawn 50 processes/second, each taking 10 ms — well
beyond what a host system can sustain. This will cause fork exhaustion, especially on macOS
(kern.maxprocperuid).

### The existing `minActivityInterval` debounce

`RecordActivity()` has a `minActivityInterval = 500 ms` debounce — but **this only applies to
`RecordActivity()` itself**, not to `onOutput` firing. The `onOutput` closure is called on
every PTY read. Adding `GetCurrentStatus()` into `onOutput` without its own gate means status
detection runs at PTY read frequency, not at the debounced 500 ms rate.

**Recommendation:**
- Do NOT call `GetCurrentStatus()` (and therefore do NOT call `Preview()`) from inside
  `onOutput`. Instead, use the circular buffer's content directly (already in memory) via
  `cc.ptyAccess.GetBuffer()` or `GetRecentOutput()`, bypassing `Preview()`.
- Gate status detection calls with a timestamp check (e.g., only run if `time.Since(lastCheck) > 250ms`).
- Alternatively, post to a deduplicated work channel (capacity 1, drop if full) and run
  detection in a separate goroutine, achieving natural rate-limiting.

---

## 3. StatusChangeListener Set After Controller Starts — Race on First Event

### The proposed pattern

```go
cc := NewClaudeController(instance)
cc.Start(ctx)
// ... some time passes ...
cc.SetStatusChangeListener(fn)  // set AFTER Start()
```

The requirements state "InstanceStatusManager wires the controller's StatusChangeListener to
the ReactiveQueueManager **at session creation time**" (R1.5), implying `SetStatusChangeListener`
is called before `Start()`. But the requirements also say "Must be called before Start()" is
the design intent for `SetOnEOFCallback` — if `SetStatusChangeListener` has the same
constraint, it must be enforced by documentation or a startup check.

### Race condition

`SetOnOutput` (the model for this pattern) has **no lock** protecting the write:
```go
func (rs *ResponseStream) SetOnOutput(fn func()) {
    rs.onOutput = fn  // plain assignment, no mutex
}
```

The streamLoop goroutine reads `rs.onOutput` without a lock (line 220). If `SetStatusChangeListener`
follows the same pattern, there is a **data race** if the listener is set after `Start()` —
the streamLoop goroutine may read a partially-written function pointer.

More importantly: if `Start()` is called before the listener is wired, the controller may
detect and emit status events with no listener registered, silently losing the first status
transition (e.g., `Unknown → NeedsApproval`) that would have triggered an immediate queue update.

**Recommendation:**
- Require `SetStatusChangeListener` to be called before `Start()` (document and add a
  startup assertion).
- If setting after `Start()` must be supported, protect the listener field with `cc.mu` on
  both write (exclusive) and read (shared). Note that `onOutput` currently reads `rs.onOutput`
  without any lock; this is a pre-existing race that needs fixing regardless.
- To avoid missing the first event, `InstanceStatusManager` should call
  `SetStatusChangeListener` as part of `RegisterController`, which must happen before
  `Start()` in the session creation flow.

---

## 4. Timer-Per-Session for Idle Detection — Goroutine Leak Risk

### Proposed design (R2)

R2 calls for a per-session idle timer that fires when `IdleStateTimeout` is reached. If this
is implemented as a `time.AfterFunc` or `time.NewTimer` goroutine per controller, each session
holds a live goroutine until the session is stopped.

### Current cleanup story

`ClaudeController.Stop()` calls `cc.cancel()`, which cancels the controller's context. If the
idle timer goroutine is not wired to `cc.ctx`, it will **not** stop when the session stops.

The existing `idleDetector` has no goroutines of its own — it is purely reactive (called when
PTY data arrives or the poller ticks). Adding a timer goroutine changes this.

### Specific leak scenarios

1. **Session deleted while timer is pending**: if the timer fires after `Stop()` and the
   listener calls `ReactiveQueueManager.CheckSession(inst)`, the instance may have been
   removed from the poller's list, causing `FindInstance` to return nil — that path is already
   safely handled in `handleUserInteraction`, so not a panic risk, but the goroutine continues
   running until the timer fires.

2. **Timer reset thrashing**: `RecordActivity()` is debounced at 500 ms. If the timer reset
   is triggered from `RecordActivity()`, then during a 60-second continuous stream, the timer
   is reset at most every 500 ms — 120 resets, each one stopping and re-creating a
   `time.Timer`. This is acceptable overhead but must use `timer.Reset()` correctly (drain
   the channel first per Go docs).

3. **Multiple timers per session**: if `RecordActivity()` is called concurrently (it is: it
   acquires `idleDetector.mu`), and the timer reset is not atomic with the lock, two goroutines
   could both observe "timer should be reset" and create a second timer without stopping the
   first.

**Recommendation:**
- Store the timer as a field on `IdleDetector` or `ClaudeController`, protected by the
  existing mutex. Use a single `time.Timer` with `Stop()` + `Reset()` rather than
  `time.AfterFunc` (which spawns a new goroutine per firing).
- In `ClaudeController.Stop()`, explicitly stop the idle timer with `timer.Stop()`.
- The timer's callback should post to a buffered channel (capacity 1) consumed by a goroutine
  already managed under `cc.ctx`, so it terminates with the controller.

---

## 5. minActivityInterval Debounce — Scope and Actual Call Rate

### What it debounces

`RecordActivity()` internally debounces writes to `lastActivity`. The guard is:
```go
if id.timeNow().Sub(id.lastActivity) < minActivityInterval {
    return  // no-op
}
id.lastActivity = id.timeNow()
```

This means `lastActivity` advances at most once per 500 ms, which limits how often the
poller's content cache is invalidated (the cache key is `lastActivity`).

### What it does NOT debounce

`onOutput` itself is called on every PTY read — potentially thousands of times per second
during streaming output. The debounce inside `RecordActivity()` prevents `lastActivity` from
changing more than 2×/second, but it does NOT prevent `onOutput` from being called. Any work
placed directly in `onOutput` beyond `RecordActivity()` runs at full PTY frequency.

### Consequence for the proposed design

If the implementation adds `GetCurrentStatus()` or the `StatusChangeListener` call directly
in `onOutput`, those run at PTY frequency (unbounded). The 500 ms debounce does not protect
them. A separate gate must be added:

```go
// In onOutput callback (proposed, not yet implemented):
cc.idleDetector.RecordActivity()             // debounced to 500ms internally
cc.maybeCheckStatus()                        // must add its own gate
```

Where `maybeCheckStatus` checks `time.Since(cc.lastStatusCheck) > 250ms` before running
detection.

---

## 6. Thread Safety: GetCurrentStatus() Called From onOutput

### Lock nesting analysis

`streamLoop` runs without holding any `ClaudeController` locks. The `onOutput` callback is
called at line 220-221 of `response_stream.go`, with no lock held from `ResponseStream` at
that call site (the `exitTail` update at line 204 acquires `rs.mu.Lock()` and releases it at
line 210, before the `onOutput` call).

`GetCurrentStatus()` acquires `cc.mu.RLock()` (line 481 of `claude_controller.go`). The
`cc.mu` is a `deadlock.RWMutex` (from `github.com/linkdata/deadlock`), which adds deadlock
detection instrumentation.

**There is no locking conflict** between `onOutput → GetCurrentStatus() → cc.mu.RLock()` and
the normal read paths (e.g., `GetIdleState()`, `GetRateLimitState()`).

**There IS a conflict** with `Stop()`, which acquires `cc.mu.Lock()` (exclusive). If
`GetCurrentStatus()` is in flight (holding `cc.mu.RLock()`) when `Stop()` is called,
`Stop()` will block until `GetCurrentStatus()` returns — which is correct behavior, not a
deadlock.

**The deadlock risk** arises if the `StatusChangeListener` callback itself acquires `cc.mu`
in any form (e.g., by calling another controller method). Since the listener is called while
`cc.mu.RLock()` is held (inside `GetCurrentStatus()`), any re-entrant acquisition of `cc.mu`
(even a read-lock on many mutex implementations) would deadlock. `deadlock.RWMutex` is not
reentrant.

**Existing statusCache safety**: `cc.statusCache` is written at line 514 while `cc.mu.RLock()`
is held. This is a **write through a read-lock**, which is not safe if multiple goroutines
hold the read-lock concurrently (both could enter the cache-miss branch and write simultaneously,
causing a data race on the struct fields). In practice, `onOutput` is called from a single
goroutine (`streamLoop`), so there is at most one concurrent caller of `GetCurrentStatus()`
from that path. But if `GetCurrentStatus()` is also called from the poller goroutines
(concurrently), the statusCache write under RLock is a race.

**Recommendation:**
- Either call the `StatusChangeListener` *outside* `cc.mu` (acquire the lock only to read
  the cache, then release before invoking the callback), or use a write-lock for the
  cache-miss branch.
- Explicitly document that the `StatusChangeListener` must not call back into `ClaudeController`
  methods that acquire `cc.mu`.
- Fix the existing `cc.statusCache` write-under-RLock data race by upgrading to a write-lock
  on cache miss (promote from RLock to Lock, which requires releasing and re-acquiring in Go's
  `sync.RWMutex`/`deadlock.RWMutex` — use a separate cache mutex instead).

---

## Summary Table

| # | Pitfall | Severity | Mitigation |
|---|---------|----------|------------|
| 1 | Callback fires after Stop() — stale reads, not nil-panic | Medium | Atomic `stopped` flag; clear listener in Stop() |
| 2 | GetCurrentStatus() in onOutput → subprocess per PTY read → fork exhaustion | **Critical** | Use buffer directly; gate with separate timestamp; deduplicated work channel |
| 3 | Listener set after Start() — data race + first event lost | Medium | Enforce before-Start(); protect field with cc.mu |
| 4 | Per-session idle timer goroutine not cleaned up in Stop() | Medium | Single time.Timer on controller, stopped in Stop() |
| 5 | minActivityInterval only debounces RecordActivity, not onOutput | High | Add explicit gate in onOutput for any new work |
| 6 | GetCurrentStatus() writes statusCache under RLock — data race if concurrent callers | Medium | Separate cache mutex or promote to write-lock on miss; call listener outside lock |
