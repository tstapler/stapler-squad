# ADR-001: Controller Status Events via Coalescing Channel + Background Goroutine

**Status:** Accepted
**Date:** 2026-05-09
**Authors:** Claude Code Assistant

## Context

The review queue relies on a 2-second polling loop (`ReviewQueuePoller`) to discover that sessions need attention. The `ClaudeController` observes every PTY write in real time via its `OnOutput` callback, but nothing connects that signal to the queue — status detection only happens when an external caller polls `GetCurrentStatus()`, which itself may invoke `tmux capture-pane` (a subprocess) on cache miss.

The goal is to emit a status-change event from the controller to the `ReactiveQueueManager` the moment the detected status changes, without:

1. Calling subprocesses from inside the PTY goroutine (fork exhaustion risk — up to 50 `capture-pane` processes/second at PTY write frequency)
2. Holding `ClaudeController` locks when invoking the listener (deadlock / re-entrancy risk)
3. Importing `server/` packages from `session/` (circular import — R4.1)
4. Losing the first status event if the listener is wired after `Start()` is called

## Decision

**Use a capacity-1 coalescing channel (`statusCheckCh chan struct{}`) plus a dedicated background goroutine per controller, wired before `Start()` is called.**

### Mechanism

The `OnOutput` closure (already fires on every PTY write) sends a non-blocking signal:

```go
// Inside the SetOnOutput closure in ClaudeController.Start():
select {
case cc.statusCheckCh <- struct{}{}:
default: // channel already has a pending signal; drop duplicate
}
```

A background goroutine started with `cc.ctx` drains the channel and runs status detection:

```go
go func() {
    for {
        select {
        case <-cc.ctx.Done():
            return
        case <-cc.statusCheckCh:
            newStatus, _ := cc.GetCurrentStatus() // uses hash cache; no lock held when calling listener
            cc.mu.Lock()
            changed := newStatus != cc.lastEmittedStatus
            if changed {
                cc.lastEmittedStatus = newStatus
            }
            listener := cc.statusChangeListener
            cc.mu.Unlock()
            if changed && listener != nil {
                listener(newStatus, cc.sessionName)
            }
        }
    }
}()
```

### Wiring constraint: register before Start()

`SetStatusChangeListener(fn)` MUST be called before `cc.Start()`. The wiring point is `server/dependencies.go` step 7 (after `inst.StartController()` — the same pattern already used for `SetRateLimitCallbacks`). `InstanceStatusManager.RegisterController()` forwards the listener at controller creation time, satisfying R1.5.

The `StatusChangeListener` type is defined in `session/` as a plain Go `func`:

```go
type StatusChangeListener func(newStatus detection.DetectedStatus, sessionName string)
```

`server/` code implements this func type and passes it in; no `server/` import enters `session/`.

## Options Considered

### Option A — Inline in OnOutput (rejected)

Call `GetCurrentStatus()` directly inside the `OnOutput` closure.

**Problem:** During active Claude output, `OnOutput` fires at 5–100 calls/second. `GetCurrentStatus()` calls `instance.Preview()` on a cache miss, which invokes `tmux capture-pane` (a subprocess, ~10–50 ms). At 50 calls/second this spawns 50 processes/second — well beyond `kern.maxprocperuid` on macOS. Even with a timestamp gate (e.g., `time.Since(lastCheck) > 250ms`), the detection logic and lock acquisition run in the hot PTY path. The hash cache only helps when content is unchanged; during active output the hash changes every call. **Critical performance flaw** — ruled out.

### Option B — EventBus from session package (rejected)

Introduce a pub-sub `EventBus` in `session/` that `ClaudeController` publishes to and `server/` subscribes to.

**Problem:** Any `EventBus` interface generic enough to carry typed events must be defined in a shared package. Defining it in `session/` and importing it from `server/` is safe, but `server/` packages that currently subscribe to events (e.g., `ReactiveQueueManager`) already have direct references to `session` types, creating tight coupling. More critically, the `EventBus` pattern encourages `session/` to grow an event taxonomy that leaks server-layer concepts upward. The callback-func approach (Option C) achieves the same decoupling with zero new abstractions and is the pattern already established by `SetRateLimitCallbacks`. **Unnecessary complexity** — ruled out.

### Option C — Timer-only polling at reduced interval (rejected)

Keep the polling architecture but reduce `PollInterval` from 2s to 250ms.

**Problem:** Linear scaling with session count. At 250ms interval with 20 sessions, the poller runs `checkSession()` 80 times/second, each potentially calling `capture-pane`. Offers no latency improvement proportional to a true event-driven approach; still misses status changes that occur within the poll window. **Does not meet R5.3** (approval prompts within 1 second). **Ruled out** as primary mechanism; poller is retained only as a 30s safety net (R3.1).

## Consequences

### Benefits

- **Latency**: Approval prompts surface within ~1 PTY read cycle (tens of milliseconds) rather than up to 2 seconds. Satisfies R5.3.
- **No subprocess spawning in hot path**: The `OnOutput` closure only does a non-blocking channel send — O(1), no allocation. Status detection runs in the background goroutine at the natural rate dictated by how often `statusCheckCh` drains.
- **Natural coalescing**: A capacity-1 channel deduplicated rapid signals for free. A burst of 100 PTY writes during code generation produces at most 1 pending status check, not 100.
- **Import graph safety**: `session/` defines only a plain `func` type. No `server/` package enters `session/`. Satisfies R4.1–R4.3.
- **Lock safety**: `cc.mu` is held only briefly to read/write `lastEmittedStatus` and `statusChangeListener`. The listener is called outside the lock. No re-entrancy hazard, no deadlock with `Stop()`.

### Goroutine lifecycle

The background goroutine is started in `Start()` and terminates when `cc.ctx` is cancelled (by `Stop()`). `Stop()` already calls `cc.cancel()`, so no additional cleanup is required. The goroutine does not hold any resources — `statusCheckCh` is a value-type channel that will be garbage collected after the goroutine exits.

The goroutine count per session is +1 over the current baseline. At 100 concurrent sessions this is 100 additional goroutines, each sleeping on a `select` — negligible memory and CPU overhead.

### "Register before Start()" constraint

This constraint prevents the first status transition (e.g., `Unknown → NeedsApproval` if Claude immediately presents an approval prompt) from being silently lost. It is enforced by:

1. **Documentation**: `SetStatusChangeListener` godoc states the constraint.
2. **Startup assertion**: `Start()` logs a warning (non-fatal) if `statusChangeListener == nil`, to catch misconfigured sessions in development.
3. **Wiring order in `server/dependencies.go`**: listener is attached in step 7 immediately before `inst.StartController()` calls `controller.Start()`.

If the listener is nil at event time (e.g., set after `Start()` in a test), the background goroutine skips the call — no panic, no lost queue entry (the safety-net poller will catch it within 30s).

### Poller retained as safety net

The `ReviewQueuePoller` is retained at a 30s reconciliation interval for: staleness detection (2-minute threshold requires time-based logic), uncommitted-changes checks (git I/O), external/non-controller sessions, and tmux reconciliation. For controller-managed sessions the poller's fast-path check is optional and may be skipped once confidence in the event-driven path is established (R3.2).

## Compliance

### Implementation checklist

- [ ] Add `statusCheckCh chan struct{}` field to `ClaudeController` (buffered, capacity 1)
- [ ] Add `statusChangeListener StatusChangeListener` and `lastEmittedStatus detection.DetectedStatus` fields
- [ ] Extend `SetOnOutput` closure in `Start()` with the non-blocking channel send
- [ ] Start background goroutine in `Start()` before `responseStream.Start()`
- [ ] Stop goroutine via `cc.ctx` cancellation in `Stop()` (no extra cleanup needed)
- [ ] Add `SetStatusChangeListener(fn StatusChangeListener)` setter with `cc.mu` protection
- [ ] Wire listener in `server/dependencies.go` step 7, before `inst.StartController()`
- [ ] Add startup assertion/log in `Start()` if listener is nil
- [ ] Update `TestReviewQueue*` and `TestReviewQueuePoller*` tests to pass (R5.4)

### Detection of regressions

```bash
# Verify no new circular imports
go build ./...

# Verify no goroutine leaks
go test -race ./server/services/... ./session/...

# Verify queue latency (approval prompt within 1s)
go test -run TestReviewQueueApprovalLatency ./server/services/...
```

## References

- Requirements: `project_plans/review-queue-event-driven/requirements.md` (R1, R4)
- Pitfalls: `project_plans/review-queue-event-driven/research/pitfalls.md` (pitfalls 1–6)
- Architecture: `project_plans/review-queue-event-driven/research/architecture.md` (Q2, Q4, Q5)
- Existing pattern: `SetRateLimitCallbacks` in `server/dependencies.go`
- Related: [ADR-006: Async Event Loop Patterns](../../../../docs/adr/006-async-event-loop-patterns.md), [ADR-011: Prefer Lock-Free Concurrency](../../../../docs/adr/011-prefer-lock-free-concurrency.md)
