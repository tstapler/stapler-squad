# Adversarial Review: PTY Multiplexer Implementation Plan

Reviewed against source files as of 2026-06-13.

---

## Finding 1 — `ResponseChunk.Error` is never populated by `streamLoop` (CONCERN)

**Location**: Task 1.1.2 and Task 1.2.1, proposed channel-read loop

**Plan code**:
```go
case chunk, ok := <-responseCh:
    if !ok {
        return
    }
    if chunk.Error != nil {   // ← DEAD CODE
        errCh <- fmt.Errorf("PTY read error: %w", chunk.Error)
        return
    }
```

**Actual behavior**: `ResponseChunk.Error` exists (declared at `response_stream.go:21`) but is **never set anywhere** in `streamLoop`. When a PTY read error occurs, `streamLoop` either logs and continues (non-fatal errors), or returns without broadcasting any chunk at all (fatal errors: EOF, EIO, closed). The channel is simply closed via `closeAllSubscribers()`, which `!ok` already handles.

The `chunk.Error != nil` branch is permanently dead code. This will not cause a compile error or panic, but the error propagation path it is meant to implement simply never fires. A fatal PTY error is silently swallowed (the goroutine exits cleanly via `!ok`, but the `errCh` never receives the root-cause error).

**Recommendation**: Remove the `chunk.Error` branch or add a note that it is aspirational. If error propagation through the chunk is desired, `streamLoop` must be modified to emit an error chunk before closing — which is out of scope per the plan.

---

## Finding 2 — Pre-existing `done` channel panic is preserved (and undocumented) in Task 1.2.1 (CONCERN)

**Location**: Task 1.2.1, proposed Goroutine 1 + existing Goroutine 2 in `terminal_websocket.go`

**Existing bug (pre-plan)**:
- Goroutine 1 owns `done`: it executes `defer close(done)`.
- Goroutine 2 signals Goroutine 1 via `done <- struct{}{}` (a blocking send) on WebSocket close or read error.
- If Goroutine 1 exits first (PTY EOF, WebSocket write error), it closes `done`. Goroutine 2's subsequent `done <- struct{}{}` is a **send on closed channel → panic**.

**How the plan interacts**: The plan's proposed Goroutine 1 code (Task 1.2.1) retains `defer close(done)` and adds `case <-done:` in the subscriber select. Goroutine 2 is left unchanged and still does `done <- struct{}{}`. The plan does not acknowledge or fix this pre-existing race.

**Severity**: The plan does not introduce this bug — it was there before. But because the plan substantially rewrites Goroutine 1, it is the right moment to fix it. The panic path is: session ends → `responseCh` closes → G1's `!ok` fires → G1 returns → `defer close(done)` → G2 later calls `done <- struct{}{}` → panic. This is more likely to be triggered with the new code because the `!ok` path fires more reliably (PTY EOF now reaches subscribers correctly).

**Recommendation**: Fix by converting `done` to a context cancellation (`context.WithCancel`) or by making `done` a buffered channel of size 1 so that G2's send never blocks (and is dropped if G1 already exited). Mark as CONCERN now, but the plan should note the risk and include a fix for Goroutine 2 as part of Task 1.2.1.

---

## Finding 3 — Subscribe-before-Start is safe but plan's timing claim is imprecise (MINOR)

**Location**: Epic 1, "Whether `ResponseStream.Subscribe()` can be called when the stream is not started"

**Actual behavior**: `ResponseStream.Subscribe()` adds the subscriber to `rs.subscribers` under `rs.mu.Lock` and returns the channel. It does not check `rs.started`. However, `GetController()` / `controllerManager.GetController()` returns the controller only after `StartController()` calls `RegisterController()`, which itself is called only after `controller.Start()` returns successfully. And `Start()` calls `responseStream.Start()` before returning. So by the time `GetController()` can return non-nil, the `ResponseStream` is already started and `streamLoop` is running.

**Net result**: The proposed `SubscribeToResponseStream` accessor cannot race with "before Start" because the controller is not observable until after Start completes. The plan is correct in its conclusion but does not state this reasoning explicitly.

**No code change required.**

---

## Finding 4 — `deadlock.RWMutex` on `ResponseStream.mu`: Subscribe call chain acquires two locks in nested order (CONCERN)

**Location**: Task 1.1.1 proposed accessor; `ClaudeController.Subscribe` at `claude_controller.go:487–496`

**Lock acquisition chain** when `SubscribeToResponseStream` is called:
1. `Instance.GetController()` takes `i.stateMutex.RLock()` → releases → returns `ctrl`
2. `ctrl.Subscribe(subscriberID)` takes `cc.mu.RLock()` (held)
3. Inside, calls `cc.responseStream.Subscribe(subscriberID)` which takes `rs.mu.Lock()` (held while `cc.mu.RLock` is also held)

Lock order: `cc.mu` → `rs.mu`.

**In `streamLoop`**, the sequence is:
1. `rs.mu.Lock()` to update `exitTail` (lines 250–256)
2. `rs.mu.Unlock()`
3. `rs.broadcast(chunk)` takes `rs.mu.RLock()`

Neither `streamLoop` nor `broadcast` ever acquires `cc.mu`. So the reverse order (`rs.mu` → `cc.mu`) does not occur. The lock order is consistent and there is no deadlock.

`cc.mu.Stop()` acquires `cc.mu.Lock()` and then calls `cc.responseStream.Stop()` which acquires `rs.mu.Lock()` (to check/set `rs.started`) — same order: `cc.mu` → `rs.mu`. Still consistent.

**However**, `deadlock.RWMutex` (from `linkdata/deadlock`) tracks lock order at runtime and will fire if it ever observes `rs.mu → cc.mu`. The plan should confirm no code path holds `rs.mu` and then calls anything that acquires `cc.mu`. The `rs.onOutput` callback (called from `streamLoop` without any lock held) calls `cc.idleDetector.RecordActivity()` and sends to `cc.statusCheckCh` — neither acquires `cc.mu`. This is safe.

**Net result**: No deadlock exists with the proposed accessor. The nested lock order is consistent. The CONCERN is that the plan does not document this order, so future contributors might introduce a reversal.

---

## Finding 5 — Flow control claim about `GenerateState()` full snapshots is correct (CLEAN)

**Location**: Migration Notes, "Flow control semantics change"

**Plan claim**: "the MOSH-style `GenerateState()` sends a full terminal snapshot on every call, not a delta — so the client will see a correct screen on next receive regardless of what was dropped."

**Verified**: `session/terminal_state.go:GenerateState()` (line 672) builds `lines := make([]*sessionv1.TerminalLine, 0, ts.Rows)` from all `ts.Rows` grid rows and returns a `TerminalData_State` with the complete screen. This is indeed a full snapshot. Drop-during-pause does not corrupt terminal state.

**No issue.**

---

## Finding 6 — `ResponseStream.mu` is `deadlock.RWMutex`, not `sync.RWMutex` — plan's description is wrong (MINOR)

**Location**: ADR section: "sync.RWMutex-guarded map updated on subscribe/unsubscribe"

**Actual code** (`response_stream.go:3,40`):
```go
import "github.com/linkdata/deadlock"
mu           deadlock.RWMutex
```

The plan's ADR says "`sync.RWMutex`-guarded map" but the actual type is `deadlock.RWMutex`. This is a documentation error only. `deadlock.RWMutex` implements the same interface and is a drop-in replacement, so the mutex semantics described in the plan are correct. The distinction matters only for tooling (runtime deadlock detection).

---

## Finding 7 — Task 1.1.3 fallback creates a race condition for external sessions with a controller (CONCERN)

**Location**: Task 1.1.3, "Handle the no-controller fallback for `StreamTerminal`"

**Plan's fallback condition**: If `SubscribeToResponseStream` returns an error (i.e., `GetController()` returns nil), fall back to `GetPTYReader()` / `ptyFile.Read()`.

**The subtle issue**: The error from `SubscribeToResponseStream` occurs in two cases:
1. `GetController()` returns nil (no controller) — the fallback is correct.
2. `ctrl.Subscribe()` returns an error because the subscriber ID already exists. (Unlikely but possible if `time.Now().UnixNano()` collides or if the same session is subscribed twice.)

More importantly, the plan's logic is: "no controller → no race → safe to use GetPTYReader()". This reasoning is correct for external sessions that never had a controller. But there is a TOCTOU window: between the time `GetController()` returns nil and the time `GetPTYReader()` begins reading, `StartController()` could have been called (e.g., by a concurrent session start). This would create exactly the race the fix is trying to eliminate.

The window is narrow (milliseconds) but real. For production correctness, after `GetPTYReader()` succeeds, the code should re-check whether a controller now exists and fail fast if it does. In practice this race is unlikely to matter, but the plan should acknowledge it.

---

## Finding 8 — Goroutine 1 in WebSocket handler has no mechanism for Goroutine 2 to signal it when using `responseCh` (CONCERN)

**Location**: Task 1.2.1, proposed Goroutine 1 in `responseCh != nil` branch

**Plan code**:
```go
for {
    select {
    case <-done:
        return
    case chunk, ok := <-responseCh:
        if !ok { return }
        ...
    }
}
```

**Issue**: `done` is a `chan struct{}` that Goroutine 2 signals via `done <- struct{}{}`. This blocking send works correctly in this path because Goroutine 1's select has `case <-done:` and is no longer in a tight `default:` spin loop. G2 blocks until G1 picks it up.

**But there is an interaction with the pre-existing Goroutine 1 `defer close(done)`**: If `responseCh` closes (`!ok`) and G1 returns, G1 executes `defer close(done)`. If G2 is simultaneously blocked in `conn.ReadMessage()` and the connection closes, G2 will attempt `done <- struct{}{}` on an already-closed channel. See Finding 2 for the full analysis.

The new subscriber path makes this race **more likely to be triggered** than the old `ptyReader.Read` path, because `responseCh` closes reliably on session exit (whereas `ptyReader.Read` with EIO may behave differently per OS).

---

## Finding 9 — Task 4.1.2 test would compile but pipe writes must go to the PTY end correctly (MINOR)

**Location**: Task 4.1.2, `TestResponseStream_TwoSubscribersReceiveAllBytes`

**Plan says**: "Setup: `PTYAccess` backed by `os.Pipe()`, two subscribers, write N chunks to pipe."

**Actual `streamLoop` behavior**: `streamLoop` reads from `rs.ptyAccess.pty` (line 189 of `response_stream.go`: `pty := rs.ptyAccess.pty`). In a test using `os.Pipe()`, the test writes to `pw` (the write end) and `streamLoop` reads from `pr` (the read end), which is passed as the `*os.File` to `NewPTYAccess`. This is the correct approach and will compile and work.

**No issue** — test construction is viable. The MINOR note is that the test must close `pw` to trigger EOF and end the test cleanly; the plan doesn't mention this.

---

## Finding 10 — `SubscribeToResponseStream` proposed signature leaks `session` package type into callers (MINOR)

**Location**: Task 1.1.1, proposed accessor on `Instance`

**Plan code**:
```go
func (i *Instance) SubscribeToResponseStream(subscriberID string) (<-chan ResponseChunk, error) {
```

This is already in package `session` so it returns `session.ResponseChunk`. The callers (`StreamTerminal`, `TerminalWebSocketHandler`) are in package `services` and already import `session`. No issue — `ResponseChunk` is already exported and used elsewhere. The return type is fine.

---

## Finding 11 — `onOutput` callback invoked from `streamLoop` while subscribers channel may be filling (MINOR, pre-existing)

**Location**: `response_stream.go:266-268` — `rs.onOutput()` is called while no lock is held

The `onOutput` callback is set by `ClaudeController.Start()` and calls `cc.idleDetector.RecordActivity()`, `cc.rateLimitHandler.NotifyOutput()`, and sends to `cc.statusCheckCh`. None of these acquire `rs.mu`, so there is no deadlock risk. This is pre-existing and the plan correctly does not touch it.

---

## Finding 12 — Line numbers in plan are accurate for `StreamTerminal` (VERIFIED)

The plan states:
- Line 1698: `instance.GetPTYReader()` — **verified**: `session_service.go:1698` is exactly this call.
- Line 1753: `ptyFile.Read(buf)` — **verified**: `session_service.go:1753` is this call.
- WebSocket handler line 80: `instance.GetPTYReader()` — **verified**: `terminal_websocket.go:80`.
- WebSocket handler line 117: `ptyReader.Read(buf)` — **verified**: `terminal_websocket.go:117`.

The plan's line numbers are correct as of this review.

---

## Finding 13 — `GetController()` acquires `stateMutex.RLock`; `SubscribeToResponseStream` then acquires `cc.mu.RLock` (lock chain is fine but undocumented) (MINOR)

Full chain: `stateMutex.RLock` → `cc.mu.RLock` → `rs.mu.Lock`. This three-lock chain is consistent (same order everywhere) but is not documented in the plan. Future modifications should maintain this order.

---

## Summary Table

| # | Finding | Severity |
|---|---------|----------|
| 1 | `chunk.Error` branch is permanently dead code — `streamLoop` never populates `ResponseChunk.Error` | CONCERN |
| 2 | Pre-existing `done` channel send-after-close panic in WebSocket handler; plan preserves and may amplify it | CONCERN |
| 3 | Subscribe-before-Start is safe; plan's reasoning is correct but unexplained | MINOR |
| 4 | Lock order `cc.mu → rs.mu` is consistent; plan's "sync.RWMutex" description is wrong but safe | CONCERN (undocumented lock order) |
| 5 | `GenerateState()` full-snapshot claim verified correct | CLEAN |
| 6 | `ResponseStream.mu` is `deadlock.RWMutex` not `sync.RWMutex` — docs only | MINOR |
| 7 | TOCTOU window in fallback: controller could start between nil-check and `GetPTYReader()` call | CONCERN |
| 8 | `responseCh` close amplifies pre-existing done-channel panic (see Finding 2) | CONCERN |
| 9 | Test 4.1.2 will compile; pipe close needed for clean EOF | MINOR |
| 10 | `ResponseChunk` return type visibility — no issue | MINOR |
| 11 | `onOutput` callback from `streamLoop` — no lock held, no issue | MINOR |
| 12 | All plan line numbers are accurate | CLEAN |
| 13 | Three-lock chain undocumented | MINOR |

---

## Detailed Verdict on Critical Questions

**Does `ResponseChunk.Error` exist?** Yes (`response_stream.go:21`). But it is never set, making the plan's error-propagation branch dead code. Not a compile error — a silent logic gap.

**Can `Subscribe()` be called before `Start()`?** Technically yes (no guard in `Subscribe`), but the proposed accessor path through `GetController()` makes this impossible in practice because the controller is only registered after `Start()` returns.

**Is `ResponseStream.mu` a `deadlock.RWMutex`?** Yes. The plan calls it `sync.RWMutex`. The behavioral semantics are identical; the difference is runtime deadlock detection.

**Is the `done` channel close pattern a new problem?** No — it is pre-existing. But the plan's new subscriber path makes the panic more likely to trigger in practice (session exit now reliably closes `responseCh`, triggering `!ok`, triggering `close(done)`, leaving G2 with a stale `done <- struct{}{}`).

**Is `GenerateState()` a full snapshot?** Yes — verified in `terminal_state.go:672–704`.

**Is the fallback for external sessions safe?** Mostly, but there is a narrow TOCTOU window (Finding 7).

**Would Task 4.1.2 compile?** Yes, the pipe-based `PTYAccess` test pattern works.

---

**Verdict: CONCERNS**

No BLOCKERs found (the plan will compile, start, and function correctly). Four CONCERNs require attention before shipping:

1. The `chunk.Error` dead-code branch (Finding 1) should be removed or documented.
2. The pre-existing WebSocket `done` channel panic (Finding 2) is amplified by the new code and should be fixed in Task 1.2.1.
3. The TOCTOU fallback race (Finding 7) should be acknowledged in the risk register.
4. The lock-order chain `cc.mu → rs.mu` (Finding 4) should be documented to prevent future inversions.
