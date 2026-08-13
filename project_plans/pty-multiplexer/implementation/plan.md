# PTY Multiplexer Implementation Plan

**Feature**: Fix PTY multi-reader data race by routing `StreamTerminal` and `TerminalWebSocketHandler` through `ResponseStream`'s existing fan-out subscriber API.

**Approach**: The narrowest correct fix. `ResponseStream.streamLoop()` is already the sole intended PTY reader and already has a working `Subscribe()` / `Unsubscribe()` / `broadcast()` fan-out mechanism. The broken callers (`StreamTerminal`, `TerminalWebSocketHandler`) call `GetPTYReader()` and then `Read()` on the raw `*os.File`, stealing bytes before `streamLoop()` can write them to the `CircularBuffer` or broadcast them to detectors.

The fix is to make those two callers subscribe to `ResponseStream` instead of reading the PTY fd directly. No new goroutine, no new type, no `PTYMultiplexer` struct required. The existing `PTYSubscriber` abstraction is not needed for this fix (it solves a different layering problem).

---

## Architecture Decision Record (ADR-implicit)

**Decision**: Use `ResponseStream.Subscribe()` / `Unsubscribe()` as the fan-out delivery mechanism for `StreamTerminal` and `TerminalWebSocketHandler`, rather than building a new `PTYMultiplexer`.

**Rationale**:
- `ResponseStream` already does exactly what a multiplexer would do: single goroutine reads PTY, broadcasts `ResponseChunk` to N subscriber channels, non-blocking sends (drops on full, logs warning), `sync.RWMutex`-guarded map updated on subscribe/unsubscribe.
- `StreamTerminal` needs `[]byte` data; `ResponseChunk.Data` is `[]byte`. The subscriber receives `<-chan ResponseChunk` — a direct fit.
- `ResponseStream` already handles EIO, `RestoreWithWorkDir` PTY replacement (through `PTYAccess.UpdatePTY()`), and subscriber channel close-on-EOF via `closeAllSubscribers()`.
- The `TerminalState.ProcessOutput()` call in `StreamTerminal` consumes `[]byte` and can be fed from `ResponseChunk.Data` without change.
- Flow control (`pauseCh` / XTerm backpressure): under the new design, pausing delivery does NOT pause the PTY read loop — the subscriber channel fills and drops start after the 10,000-chunk buffer (configurable). This correctly isolates backpressure to one client and avoids stalling `ClaudeController`. The XTerm `pauseCh` logic is retained within the handler for input-side gating.

**What is NOT in scope**:
- Building `PTYMultiplexer` in `session/pty_multiplexer.go` — over-engineered for the current three callers.
- Changing `PTYAccess.Read()` — it stays as-is; only `streamLoop()` calls it.
- Changing `ResponseStream.streamLoop()` — it stays as-is.
- Changing `PTYSubscriber` / `memPTYSubscriber` — not used in this fix.
- Proto or frontend changes.

---

## Epic / Story / Task Breakdown

### Epic 1: Fix the Broken Direct-Read Callers (2 stories, 5 tasks)

#### Story 1.1: Fix `StreamTerminal` in `server/services/session_service.go`

**File**: `server/services/session_service.go`
**Broken lines**: 1698 (`instance.GetPTYReader()`), 1753 (`ptyFile.Read(buf)`)

##### Task 1.1.1: Add `SubscribeToResponseStream` accessor on `Instance`

**Why**: `StreamTerminal` needs a path from `*session.Instance` to the underlying `ResponseStream.Subscribe()`. Today the path is `instance.GetController().Subscribe()`, but `GetController()` can return `nil` for sessions without a controller. The handler must handle the nil case.

**File**: `session/instance_controller.go`
**What to add**: A new method after `GetController()` at line 141:

```go
// SubscribeToResponseStream subscribes to the session's ResponseStream and returns
// a read channel for ResponseChunks. Returns (nil, nil) if no controller (and thus
// no response stream) is active for this session.
func (i *Instance) SubscribeToResponseStream(subscriberID string) (<-chan ResponseChunk, error) {
    ctrl := i.GetController()
    if ctrl == nil {
        return nil, fmt.Errorf("no controller active for session '%s'", i.Title)
    }
    return ctrl.Subscribe(subscriberID)
}

// UnsubscribeFromResponseStream removes a subscriber from the session's ResponseStream.
func (i *Instance) UnsubscribeFromResponseStream(subscriberID string) error {
    ctrl := i.GetController()
    if ctrl == nil {
        return nil
    }
    return ctrl.Unsubscribe(subscriberID)
}
```

**Verify**: `go build ./session/...` passes.
**Test**: None new required for this accessor (covered by Story 1.3 integration test).

##### Task 1.1.2: Replace `GetPTYReader()` + `ptyFile.Read()` in `StreamTerminal` output goroutine

**File**: `server/services/session_service.go`
**Lines to change**:

Remove lines 1697–1702 (the `GetPTYReader()` call and its error check):
```go
// Get PTY for reading terminal output
ptyFile, err := instance.GetPTYReader()
if err != nil {
    log.Error("[StreamSession] failed to get PTY reader", "session", instance.Title, "err", err)
    return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PTY reader: %w", err))
}
```

Replace with:
```go
// Subscribe to the ResponseStream for fan-out PTY output delivery.
// This avoids reading the PTY fd directly, which would race with ResponseStream.streamLoop().
subscriberID := fmt.Sprintf("stream-terminal-%s-%d", initialMsg.SessionId, time.Now().UnixNano())
responseCh, err := instance.SubscribeToResponseStream(subscriberID)
if err != nil {
    log.Error("[StreamTerminal] failed to subscribe to response stream", "session", instance.Title, "err", err)
    return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to subscribe to response stream: %w", err))
}
defer func() {
    _ = instance.UnsubscribeFromResponseStream(subscriberID)
}()
```

In the output goroutine (was lines 1720–1798), replace the inner `ptyFile.Read(buf)` block:

**Remove** (lines 1728–1797):
```go
buf := make([]byte, 32*1024)
for {
    // Block until unpaused rather than spinning.
    if ptyPaused {
        select {
        case <-streamCtx.Done():
            return
        case ptyPaused = <-pauseCh:
            ...
        }
        continue
    }

    select {
    case <-streamCtx.Done():
        return
    case paused := <-pauseCh:
        ptyPaused = paused
        ...
    default:
        n, readErr := ptyFile.Read(buf)
        if n > 0 { ... }
        if readErr != nil { ... }
    }
}
```

**Replace with**:
```go
for {
    if ptyPaused {
        select {
        case <-streamCtx.Done():
            return
        case ptyPaused = <-pauseCh:
            if !ptyPaused {
                log.Info("[FlowControl] PTY reading RESUMED", "session", initialMsg.SessionId)
            }
        }
        continue
    }

    select {
    case <-streamCtx.Done():
        return
    case paused := <-pauseCh:
        ptyPaused = paused
        if paused {
            log.Info("[FlowControl] PTY reading PAUSED", "session", initialMsg.SessionId)
        }
    case chunk, ok := <-responseCh:
        if !ok {
            // ResponseStream closed (session exited or controller stopped).
            // NOTE: ResponseChunk.Error exists but is never set by streamLoop — fatal
            // PTY errors close the channel without emitting an error chunk. The !ok
            // path is the only EOF signal; there is no chunk.Error to check.
            return
        }
        if len(chunk.Data) == 0 {
            continue
        }
        // Update terminal activity timestamps with the output content
        instance.UpdateTerminalTimestamps(string(chunk.Data), true)

        // Process PTY output through terminal state
        if processErr := terminalState.ProcessOutput(chunk.Data); processErr != nil {
            log.Warn("failed to process terminal output", "err", processErr)
            outputMsg := &sessionv1.TerminalData{
                SessionId: initialMsg.SessionId,
                Data: &sessionv1.TerminalData_Output{
                    Output: &sessionv1.TerminalOutput{
                        Data: chunk.Data,
                    },
                },
            }
            if sendErr := stream.Send(outputMsg); sendErr != nil {
                errCh <- fmt.Errorf("failed to send output: %w", sendErr)
                return
            }
            continue
        }

        stateMsg := terminalState.GenerateState()
        stateMsg.SessionId = initialMsg.SessionId
        if sendErr := stream.Send(stateMsg); sendErr != nil {
            errCh <- fmt.Errorf("failed to send state: %w", sendErr)
            return
        }
    }
}
```

**Key differences**:
- No more `ptyFile.Read()` — reads from `<-responseCh` (the `ResponseStream` fan-out channel).
- Flow control (`pauseCh`) is preserved — when `ptyPaused`, the `select` blocks until unpaused. The `responseCh` fills during pause; drops start after 10,000 chunks (the `ResponseStream.bufferSize` default). This is acceptable: the browser client's XTerm buffer handles short pauses; long pauses will see some drops, but this is the same trade-off as the existing `broadcast()` drop policy.
- Channel close (`!ok`) is the new EOF signal.

**Verify**: `go build ./server/services/...` passes.
**Test**: `go test ./server/services/... -run TestStreamTerminal` (if tests exist). Manual verification: open a session in the browser, confirm `ClaudeController` still detects `StatusIdle`.

##### Task 1.1.3: Handle the no-controller fallback for `StreamTerminal`

**Context**: `SubscribeToResponseStream` returns an error if no controller is active. Not all sessions have a controller (e.g., external/attached sessions). The `StreamTerminal` handler must handle this case.

**Decision**: If there is no controller (and therefore no `ResponseStream`), fall back to the original `GetPTYReader()` / `ptyFile.Read()` path. This preserves backward compatibility for external sessions while fixing the race for managed sessions.

**File**: `server/services/session_service.go`

Replace the subscribe block (Task 1.1.2) with:
```go
var responseCh <-chan ResponseChunk
var ptyFile *os.File
subscriberID := fmt.Sprintf("stream-terminal-%s-%d", initialMsg.SessionId, time.Now().UnixNano())

ch, subErr := instance.SubscribeToResponseStream(subscriberID)
if subErr != nil {
    // No active controller — fall back to direct PTY read for external/unmanaged sessions.
    log.Info("[StreamTerminal] no response stream active, falling back to direct PTY read", "session", instance.Title)
    ptyFile, err = instance.GetPTYReader()
    if err != nil {
        log.Error("[StreamSession] failed to get PTY reader", "session", instance.Title, "err", err)
        return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PTY reader: %w", err))
    }
} else {
    responseCh = ch
    defer func() {
        _ = instance.UnsubscribeFromResponseStream(subscriberID)
    }()
}
```

In the output goroutine, branch on which mode is active:
```go
if responseCh != nil {
    // Managed session: read from ResponseStream subscriber channel
    // ... (channel-select loop from Task 1.1.2)
} else {
    // External/unmanaged session: read PTY directly (no concurrent controller)
    buf := make([]byte, 32*1024)
    // ... (original ptyFile.Read loop, unchanged)
}
```

This ensures zero regression for external sessions.

**Verify**: `go build ./server/services/...` passes.

#### Story 1.2: Fix `TerminalWebSocketHandler` in `server/services/terminal_websocket.go`

**File**: `server/services/terminal_websocket.go`
**Broken lines**: 80 (`instance.GetPTYReader()`), 117 (`ptyReader.Read(buf)`)

##### Task 1.2.1: Apply the same subscriber pattern to `TerminalWebSocketHandler`

**What to change**: Apply the same subscriber pattern as Tasks 1.1.2–1.1.3.

**Additional fix required**: The existing `terminal_websocket.go` has a pre-existing panic risk: Goroutine 1 executes `defer close(done)` when it exits, but Goroutine 2 does `done <- struct{}{}` (a blocking send) on errors, which panics if Goroutine 1 already closed `done`. The new subscriber path makes this more likely to trigger because `responseCh` closes reliably on session exit, immediately triggering the `!ok` return in Goroutine 1. Fix this as part of this task by converting `done` to a context-cancel pattern or making `done` a buffered channel of size 1.

Remove lines 79–85 (the `GetPTYReader()` call):
```go
ptyReader, err := instance.GetPTYReader()
if err != nil {
    log.Error("failed to get PTY reader", "err", err)
    _ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
    return
}
```

Replace with:
```go
var responseCh <-chan ResponseChunk
var ptyReader *os.File
subscriberID := fmt.Sprintf("ws-terminal-%s-%d", sessionID, time.Now().UnixNano())

ch, subErr := instance.SubscribeToResponseStream(subscriberID)
if subErr != nil {
    log.Info("[TerminalWebSocket] no response stream active, falling back to direct PTY read", "session", sessionID)
    ptyReader, err = instance.GetPTYReader()
    if err != nil {
        log.Error("failed to get PTY reader", "err", err)
        _ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
        return
    }
} else {
    responseCh = ch
    defer func() {
        _ = instance.UnsubscribeFromResponseStream(subscriberID)
    }()
}
```

In Goroutine 1 (the output goroutine, lines 105–135), replace the `ptyReader.Read(buf)` loop:

```go
// Fix the pre-existing done-channel panic: make done buffered so G2's send
// never blocks if G1 has already closed. Replace:
//   done := make(chan struct{})
// with:
//   done := make(chan struct{}, 1)
// This prevents the panic without changing the signaling semantics.

wg.Add(1)
go func() {
    defer wg.Done()
    defer close(done)

    if responseCh != nil {
        for {
            select {
            case <-done:
                return
            case chunk, ok := <-responseCh:
                // NOTE: ResponseChunk.Error is never set by streamLoop — do NOT
                // add a chunk.Error check here; !ok is the only EOF signal.
                if !ok {
                    return
                }
                if len(chunk.Data) == 0 {
                    continue
                }
                if err := conn.WriteMessage(websocket.BinaryMessage, chunk.Data); err != nil {
                    log.Error("error writing to WebSocket", "err", err)
                    return
                }
            }
        }
    } else {
        // Fallback: direct PTY read for unmanaged sessions
        buf := make([]byte, 1024)
        for {
            select {
            case <-done:
                return
            default:
                n, err := ptyReader.Read(buf)
                if err != nil {
                    if err != io.EOF {
                        log.Error("error reading from PTY", "err", err)
                        _ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("PTY error: %v", err)))
                    }
                    return
                }
                if n > 0 {
                    if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
                        log.Error("error writing to WebSocket", "err", err)
                        return
                    }
                }
            }
        }
    }
}()
```

**Verify**: `go build ./server/services/...` passes.

---

### Epic 2: Guard the ResponseStream Subscriber Channel Close Race (1 story, 1 task)

#### Story 2.1: Audit `ResponseStream.Unsubscribe()` for send-after-close risk

**Context**: `ResponseStream.broadcast()` iterates `rs.subscribers` under `RLock` and sends to `sub.Ch`. `Unsubscribe()` closes `sub.Ch` under `Lock`. Since `Lock` is exclusive and `RLock` is shared, a close under `Lock` cannot race with a send under `RLock` — the existing mutex semantics are correct.

**However**: The new callers (`StreamTerminal`, `TerminalWebSocketHandler`) read from the subscriber channel in a goroutine that may exit at any time. When the goroutine exits, it calls `defer instance.UnsubscribeFromResponseStream(subscriberID)`. This is a `Lock` call on `ResponseStream.mu`. The `defer` fires after the goroutine's channel receive loop exits, which is safe — by the time the deferred unsubscribe runs, the goroutine is no longer reading from the channel.

##### Task 2.1.1: Add nil-guard to `Unsubscribe` for already-closed subscribers

**File**: `session/response_stream.go`

The current `Unsubscribe()` at line 332 returns an error if the ID is not found. The `defer` in `StreamTerminal` discards this error, which is fine. However, if `closeAllSubscribers()` fires (on PTY EOF) before the handler's defer runs, the deferred `Unsubscribe()` will return "subscriber not found" (because `closeAllSubscribers()` already removed it). The discarded error is harmless, but it generates a log line.

**Change** (defensive, low-risk): The discard via `_ = instance.UnsubscribeFromResponseStream(...)` in the new code already handles this. No code change needed — just a note that "subscriber not found" errors from deferred cleanup are expected and safe.

**Verify**: Review only — no code change in this task.

---

### Epic 3: Verify EIO / `RestoreWithWorkDir` Continuity (1 story, 1 task)

#### Story 3.1: Confirm `PTYAccess.UpdatePTY()` propagates to active `StreamTerminal` subscribers

**Context**: When `RestoreWithWorkDir` replaces `t.ptmx`, it calls `PTYAccess.UpdatePTY(newPTY)`. This updates the `*os.File` pointer inside `PTYAccess`. The `ResponseStream.streamLoop()` goroutine then:
1. Gets EIO from the old ptmx.
2. On EIO, calls `closeAllSubscribers()` and calls `rs.OnEOF()`.
3. Stops.

After `UpdatePTY()`, `streamLoop()` is **not** automatically restarted. This is the existing behavior and is already a known limitation — `RestoreWithWorkDir` would need to also call `responseStream.Start()`.

**Investigation required** (not a new bug we are introducing, but a pre-existing gap): Check whether `RestoreWithWorkDir` or the calling code in `instance_cold_restore.go` restarts the response stream after PTY replacement.

##### Task 3.1.1: Audit `RestoreWithWorkDir` for response stream restart

**File to read**: `session/tmux/tmux.go` around line 790 (the `RestoreWithWorkDir` method), then follow callers.

**Check**:
1. Does `RestoreWithWorkDir` notify `PTYAccess` or `ResponseStream` that the PTY was replaced?
2. If not, after `closePTYAndAttachCmd()` + new ptmx creation, is `responseStream.Start()` called?

**Expected finding**: This gap is pre-existing. Our fix does not make it worse — if `ResponseStream` was not restarted before our fix, `StreamTerminal` was also broken (using the old fd). After our fix, `StreamTerminal` correctly observes the channel close (when `closeAllSubscribers()` fires) and terminates gracefully, which is at least as good as the current behavior.

**Action if gap exists**: File a follow-up issue; do not block this PR on it. Document in the PR description.

**Verify**: Read `session/tmux/tmux.go:RestoreWithWorkDir` and any callers. Confirm behavior is not regressed by this change.

---

### Epic 4: Testing (1 story, 3 tasks)

#### Story 4.1: Test coverage for the refactored callers

##### Task 4.1.1: Write a unit test for `StreamTerminal` with an active `ResponseStream`

**File**: `server/services/session_service_stream_terminal_test.go` (new file, or add to existing test file)

**Pattern**: Use an in-memory `ResponseStream` backed by a pipe-based `PTYAccess`. Subscribe via `instance.SubscribeToResponseStream()`. Verify that bytes written to the pipe appear in the `stream.Send()` output.

**Test names** (for registry tracking):
- `TestStreamTerminal_ReceivesFromResponseStream_WhenControllerActive`
- `TestStreamTerminal_FallsBackToDirectRead_WhenNoController`

**File to consult**: `server/services/session_service_test.go` for existing mock patterns.

**What the test must verify**:
1. When `ClaudeController` is active, `StreamTerminal` receives bytes via `ResponseStream` fan-out — NOT via direct PTY read.
2. When no controller is active, `StreamTerminal` falls back to direct PTY read without error.
3. When the `ResponseStream` channel closes (simulating session exit), the `StreamTerminal` handler exits cleanly without goroutine leak.

##### Task 4.1.2: Write a concurrent-reader regression test

**File**: `session/response_stream_test.go` (existing)

Add a test that verifies no bytes are lost when two subscribers read simultaneously from one `ResponseStream`:
- Name: `TestResponseStream_TwoSubscribersReceiveAllBytes`
- Setup: `PTYAccess` backed by `os.Pipe()`, two subscribers, write N chunks to pipe.
- Assert: Both subscribers receive all N chunks with identical content.

This test would have caught the original race if it had existed.

##### Task 4.1.3: Verify `make quick-check` passes

**Command**: `make quick-check` from repo root.
**Expected**: All tests pass, no lint errors.

If any existing test broke due to the changes (e.g., tests that mock `instance.GetPTYReader()` and verify it's called), update the mock expectations to reflect the new call pattern.

---

## Migration Notes

### What changes

| Component | Before | After |
|---|---|---|
| `StreamTerminal` output goroutine | Calls `GetPTYReader()` → `ptyFile.Read(buf)` | Calls `SubscribeToResponseStream()` → reads `<-responseCh` |
| `TerminalWebSocketHandler` output goroutine | Calls `GetPTYReader()` → `ptyReader.Read(buf)` | Calls `SubscribeToResponseStream()` → reads `<-responseCh` |
| `ResponseStream.broadcast()` | Sends to `CommandExecutor` subscribers only | Also sends to `StreamTerminal` and `TerminalWebSocketHandler` subscribers |
| `ClaudeController` PTY read | `streamLoop()` gets all bytes | No change — `streamLoop()` was already the intended sole reader; now it actually is |

### What does NOT change

- `ResponseStream.streamLoop()` — the PTY read loop is unchanged.
- `PTYAccess.Read()` — still calls `p.pty.Read()` directly; still used by `streamLoop()`.
- `PTYSubscriber` / `memPTYSubscriber` — not used in this fix.
- `GetPTYReader()` on `Instance` — kept for the fallback path and for tests.
- `streamViaControlMode` — unaffected; never used `GetPTYReader()`.
- `ClaudeController.Start()` — unchanged; it creates `PTYAccess` and `ResponseStream` as before.
- `CircularBuffer` population — unchanged; `streamLoop()` still writes all bytes.
- `Preview()`, `GetCurrentStatus()`, `GetIdleState()` — unchanged; read from `CircularBuffer`.
- Proto/API surface — no changes.
- Frontend — no changes.

### Backward compatibility

Sessions without an active `ClaudeController` (external/attached sessions, sessions where the controller was not started, or unmanaged sessions) use the `GetPTYReader()` fallback path. They continue to work exactly as before. The race condition does not exist for these sessions because only one goroutine reads the PTY.

### Flow control semantics change

**Before**: `StreamTerminal` holding `pauseCh = true` stopped all PTY reading for `StreamTerminal`'s fd copy. The `ResponseStream.streamLoop()` continued reading its own fd copy (but in practice they raced, so this didn't work correctly anyway).

**After**: `StreamTerminal` holding `pauseCh = true` stops consuming from `responseCh`. The `ResponseStream.streamLoop()` continues reading and broadcasting. The `responseCh` channel fills at 10,000 chunks (the default `bufferSize`), then drops start. For typical pause durations (< 1s), the 10,000-chunk buffer is sufficient (10,000 × 4KB = ~40MB buffer — far more than a 1-second pause would generate). For extended pauses, drops are acceptable because the MOSH-style `GenerateState()` sends a full terminal snapshot on every call, not a delta — so the client will see a correct screen on next receive regardless of what was dropped.

---

## Acceptance Criteria Checklist

- [ ] AC-1: With browser client connected and actively streaming, `ClaudeController` detects `StatusIdle` within 5 seconds of `❯` prompt appearing (no PTY byte theft by `StreamTerminal`).
- [ ] AC-2: Initial workflow prompt fires with `claudeAtPrompt: true`, not `timedOut: true`.
- [ ] AC-3: Two simultaneous browser clients on the same session (two `StreamTerminal` or WebSocket connections) both receive identical bytes from their respective `ResponseStream` subscriber channels.
- [ ] AC-4: `make quick-check` passes with no new test failures.
- [ ] AC-5: No goroutine leaks when a browser client disconnects mid-stream (verified via `pprof` goroutine snapshot before and after disconnect).
- [ ] AC-6: `Preview()` returns correct content with 0, 1, or N clients connected.

---

## Task Execution Order

Tasks can be executed sequentially in story order. No parallel dependencies.

1. Task 1.1.1 — Add `SubscribeToResponseStream` / `UnsubscribeFromResponseStream` to `Instance`
2. Task 1.1.2 — Replace `StreamTerminal` output goroutine (managed session path)
3. Task 1.1.3 — Add fallback for external sessions in `StreamTerminal`
4. Task 1.2.1 — Apply same pattern to `TerminalWebSocketHandler`
5. Task 2.1.1 — Audit (no code change expected)
6. Task 3.1.1 — Audit `RestoreWithWorkDir` (read-only)
7. Task 4.1.2 — Add `TestResponseStream_TwoSubscribersReceiveAllBytes` regression test
8. Task 4.1.1 — Add `StreamTerminal` unit tests
9. Task 4.1.3 — `make quick-check`

**Estimated size**: ~150 lines changed / added across 3–4 files + test file. No new files required beyond the test file.

---

## Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| `StreamTerminal` flow-control pause causes subscriber channel to fill and drop | Medium | 10,000-chunk `bufferSize` covers short pauses; MOSH-style `GenerateState()` sends full snapshots (verified in `terminal_state.go:672–704`), so drops don't corrupt terminal state |
| `Unsubscribe` returns "not found" after `closeAllSubscribers()` fires | Low | Defer discards the error (`_ = ...`); safe |
| External/unmanaged sessions regress | Low | Fallback path to `GetPTYReader()` preserved when `SubscribeToResponseStream()` returns error |
| `RestoreWithWorkDir` does not restart `ResponseStream` | Medium | Pre-existing gap; not introduced by this fix. `StreamTerminal` now exits cleanly on channel close (better than current silent byte loss). File follow-up. |
| `responseCh` send-on-closed channel panic | None | `broadcast()` under `RLock` vs. `closeAllSubscribers()` under `Lock` — mutex exclusion prevents race. Deferred unsubscribe can only fire after goroutine exits (no concurrent send). |
| WebSocket `done` channel send-after-close panic (pre-existing, amplified) | Medium | Pre-existing bug in `terminal_websocket.go`: G1 closes `done` on exit; G2 blocks on `done <- struct{}{}`. New subscriber path makes G1 exit more reliably on session end. Fix required in Task 1.2.1 — convert to context-cancel or use buffered `done`. |
| TOCTOU in fallback: controller starts between nil-check and `GetPTYReader()` | Low | Narrow window (milliseconds); only affects the race we're fixing, not external sessions which this PR doesn't touch. Acknowledge in PR description; fix if observed in production. |
| Lock order `cc.mu → rs.mu` (via `SubscribeToResponseStream` → `Subscribe`) | Low | Consistent with existing `Stop()` path; `streamLoop` never acquires `cc.mu`. `deadlock.RWMutex` will catch any future inversion at runtime. Document order in code comment. |
| `ResponseChunk.Error` dead-code branch | Low | `streamLoop` never sets this field; fatal errors close the channel. Remove the `chunk.Error` check in implementation to avoid misleading future readers. |

---

## Files Changed Summary

| File | Type | What Changes |
|---|---|---|
| `session/instance_controller.go` | Edit | Add `SubscribeToResponseStream()` and `UnsubscribeFromResponseStream()` |
| `server/services/session_service.go` | Edit | Replace `GetPTYReader()` + `ptyFile.Read()` in `StreamTerminal` with subscriber pattern |
| `server/services/terminal_websocket.go` | Edit | Replace `GetPTYReader()` + `ptyReader.Read()` in `TerminalWebSocketHandler` with subscriber pattern |
| `session/response_stream_test.go` | Edit | Add `TestResponseStream_TwoSubscribersReceiveAllBytes` |
| `server/services/session_service_test.go` (or new file) | Edit/New | Add `TestStreamTerminal_*` tests |

Total: 3 production files edited, 1–2 test files touched.
