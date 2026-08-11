# PTY Fan-Out Multiplexer: Risks and Edge Cases

## Overview

The proposed fix replaces two independent direct readers of `t.ptmx` (ClaudeController via `ResponseStream` and `StreamTerminal` via `GetPTYReader`) with a single multiplexer goroutine that fans bytes out to multiple subscribers. This document catalogs the risks that design introduces, grounded in the actual codebase at `session/`, `session/tmux/`, and `server/services/session_service.go`.

**Current read path (per reader):**
- `ClaudeController.Start()` → `instance.GetPTYReader()` → `PTYAccess` → `ResponseStream.streamLoop()` → calls `pty.Read(buf)` via `PTYAccess.Read()`
- `StreamTerminal` RPC handler → `instance.GetPTYReader()` → raw `ptyFile.Read(buf)` in a goroutine

**Proposed read path:**
- Single multiplexer goroutine reads `t.ptmx` → fans bytes into per-subscriber channels

---

## Race Conditions

### 1. ptmx replaced mid-stream: `RestoreWithWorkDir`

**Location:** `session/tmux/tmux.go:790` — `RestoreWithWorkDir()`

`RestoreWithWorkDir` unconditionally closes and replaces `t.ptmx` while the multiplexer goroutine may be blocked inside `t.ptmx.Read()`:

```go
// tmux.go:879-904
if t.ptmx != nil {
    _ = t.closePTYAndAttachCmd()   // closes t.ptmx
}
// ...creates new ptmx...
ptmx, attachCmd, err := t.ptyFactory.StartWithSize(t.buildAttachCommand(), ws)
// ...
t.ptmx = ptmx
```

`GetPTY()` returns the current `t.ptmx` pointer at the time of the call — it is not a live reference. The multiplexer goroutine holds a snapshot of the old `*os.File`. When `closePTYAndAttachCmd` closes the underlying file descriptor, the goroutine's `Read()` will unblock with EIO or `bad file descriptor`. The goroutine must detect this, discard the stale handle, and acquire the new `t.ptmx`.

**Risk:** The multiplexer goroutine sees EIO, stops, and never re-connects to the new ptmx. All subscribers go dark even though the session is alive. This is not a theoretical risk — `RestoreWithWorkDir` is called from `instance_cold_restore.go` and from the session lifecycle on tmux attach failures.

**Mitigation required:** The multiplexer must not hold a raw `*os.File` pointer. It needs a live accessor (e.g., a `func() *os.File` getter protected by the same mutex that guards `t.ptmx`) so it can re-acquire the new ptmx after EIO, or the multiplexer must register itself to receive a "ptmx replaced" notification.

### 2. Double-reader window during transition

During the time between `PTYAccess.UpdatePTY()` being called and the multiplexer goroutine detecting EIO and restarting, there is a window where neither reader is consuming from the new ptmx. Bytes written by tmux during this window are buffered in the kernel PTY ring. Depending on tmux's output rate, the kernel buffer (typically 4 KB on Linux) can fill up, causing tmux's write to block. This creates a brief but real stall in terminal output for the user.

---

## Goroutine Leaks

### 1. Slow subscriber blocks the multiplexer loop

The multiplexer will use a per-subscriber channel. If a subscriber's channel is full and the multiplexer uses a blocking send, the entire fan-out stalls: all other subscribers stop receiving data, the multiplexer goroutine is parked, and the PTY read buffer in the kernel grows until the terminal's application-level flow control kicks in.

**Current `ResponseStream` approach (reference):** `broadcast()` in `response_stream.go:294-307` uses a non-blocking `select` with a `default` case that drops and logs a warning:

```go
select {
case sub.Ch <- chunk:
    // Successfully sent
default:
    // Channel is full, log warning but don't block
    log.Warn("subscriber channel full, dropping chunk", ...)
}
```

The buffer per subscriber is 10,000 chunks (`NewResponseStream` default). If the multiplexer adopts blocking sends instead, a paused `StreamTerminal` client (see Backpressure section) would cause the multiplexer to park indefinitely.

**Risk:** goroutine leak if the subscriber goroutine exits without draining its channel while the multiplexer is stuck trying to send to it.

### 2. No subscriber means no reader

If all subscribers disconnect between when the multiplexer goroutine calls `Read()` and when it tries to fan out, the goroutine should continue draining the PTY to prevent kernel buffer saturation. Abandoning reads causes tmux to stall. The multiplexer must keep reading even with zero subscribers.

---

## EIO Propagation

### Current EIO handling in `ResponseStream.streamLoop()` (`response_stream.go:209-246`)

The existing code handles EIO explicitly:

```go
errMsg := err.Error()
if strings.Contains(errMsg, "file already closed") ||
    strings.Contains(errMsg, "bad file descriptor") ||
    strings.Contains(errMsg, "input/output error") {
    log.Info("session program exited (PTY closed)", "err", err)
    rs.closeAllSubscribers()
    rs.started = false
    if rs.OnEOF != nil {
        rs.OnEOF()
    }
    return
}
```

`io.EOF` is also caught and treated identically. The `OnEOF` callback is wired in `ClaudeController` to trigger session-exit processing.

### Multiplexer EIO risks

1. **EIO from ptmx close vs. EIO from session exit:** Both conditions produce EIO. The multiplexer must distinguish between:
   - Legitimate ptmx close due to `RestoreWithWorkDir` replacing the fd (should re-connect to new ptmx)
   - PTY closed because the tmux session's pane exited (should propagate EOF to all subscribers and stop)

   Currently `ResponseStream` treats EIO as terminal. A multiplexer that does the same will kill all subscriber channels when `RestoreWithWorkDir` closes the old ptmx, even though the session is not dead.

2. **EIO must propagate as an error chunk, not a channel close:** If the multiplexer closes subscriber channels on EIO, subscribers using `range ch` will exit. If EIO was a transient "ptmx replaced" event, those subscribers are now dead and won't reconnect. Propagation as an error chunk lets subscribers decide whether to reconnect.

3. **`StreamTerminal` EIO path (`session_service.go:1789-1795`):**
   ```go
   if readErr != nil {
       if readErr.Error() != "EOF" {
           errCh <- fmt.Errorf("PTY read error: %w", readErr)
       }
       return
   }
   ```
   This code terminates the StreamTerminal goroutine on EIO. If the multiplexer delivers EIO as a channel error chunk, the StreamTerminal handler must be updated to handle the new error-delivery path.

---

## Channel Safety

### Closing while sender is active

`ResponseStream.Unsubscribe()` (`response_stream.go:332-346`) closes the subscriber's channel under `rs.mu.Lock()`. The `broadcast()` method holds `rs.mu.RLock()` while sending. This means a `close(sub.Ch)` under write-lock cannot race with a send under read-lock — safe under the current design.

**Risk in a new multiplexer:** If the multiplexer goroutine holds a snapshot of subscriber channels (e.g., a copied slice) and a subscriber unsubscribes and its channel is closed while the multiplexer is iterating the snapshot, the multiplexer will panic with "send on closed channel."

**Pattern required:** Either:
- Non-blocking send with recover from panic (costly), or
- Subscriber channels are never closed by the receiver; instead the receiver signals via a separate done channel, and the multiplexer uses `select { case ch <- chunk: case <-sub.done: }`, or
- Re-read the subscriber map under RLock on every broadcast (current `ResponseStream` pattern — copy the lock-guarded map on each broadcast)

### `sync.Once` for channel close

The codebase does not currently use `sync.Once` to guard channel closes in this path. The `closeAllSubscribers()` method clears the map atomically under write-lock, which is sufficient for the current single-reader design. A multiplexer that fans to multiple writable channels must ensure each channel is closed exactly once.

---

## Backpressure

### `StreamTerminal` flow control (`session_service.go:1717-1797`)

`StreamTerminal` implements XTerm-style flow control via `pauseCh`:

```go
pauseCh := make(chan bool, 1)
var ptyPaused bool

// in PTY read goroutine:
if ptyPaused {
    select {
    case <-streamCtx.Done():
        return
    case ptyPaused = <-pauseCh:
        // wait until resumed
    }
    continue
}
```

When the client sends `FlowControl.Paused = true`, the goroutine stops calling `ptyFile.Read()`. In the current architecture, `StreamTerminal` holds its own fd from `GetPTYReader()`, so pausing it only stops that reader — `ResponseStream` continues reading.

**After the multiplexer is introduced:** If `StreamTerminal` becomes a subscriber channel and the multiplexer goroutine does not pause when `StreamTerminal`'s channel fills up, the multiplexer will either:
- Drop chunks for that subscriber (non-blocking send), losing terminal output the user expected to see after resuming, or
- Block on that subscriber's channel, stalling all other subscribers including `ClaudeController`/`ResponseStream`

The flow control signal (`pauseCh`) currently pauses the PTY read goroutine inside `StreamTerminal`. With a multiplexer, that signal must instead pause delivery to only the `StreamTerminal` subscriber channel, not the entire multiplexer. The multiplexer must continue draining the PTY while the `StreamTerminal` subscriber is paused.

**Consequence:** Pausing `StreamTerminal` will cause its subscriber channel to fill and chunks to be dropped, meaning the client misses terminal output between pause and resume. The XTerm flow control spec expects the server to stop sending data when paused, not to drop it. The multiplexer design breaks this contract unless the `StreamTerminal` subscriber channel is large enough to buffer all output during a pause window, or the multiplexer implements per-subscriber backpressure that suspends delivery to one subscriber without blocking others.

---

## Existing Test Coverage

### Tests that test `PTYAccess.Read()` directly

- `session/pty_access_test.go`: `TestPTYAccess_Read`, `TestPTYAccess_Write`, `TestPTYAccess_UpdatePTY`, `TestPTYAccess_Close` — these use `os.Pipe()` as a mock PTY and call `ptyAccess.Read()` directly. If `PTYAccess.Read()` is removed or changed to delegate to a multiplexer subscriber channel instead of `p.pty.Read()`, these tests will need to be rewritten. They currently assert that `ptyAccess.Read()` blocks until data arrives on the pipe.

### Tests that test `ResponseStream` with a real `PTYAccess`

- `session/response_stream_test.go`: `TestResponseStream_StartAndStop`, `TestResponseStream_Subscribe`, `TestResponseStream_Broadcast` — these construct a `PTYAccess` backed by `os.Pipe()` and verify that `streamLoop()` delivers bytes to subscribers. If `ResponseStream` is replaced by subscribing to the multiplexer, these tests lose their pipe-based setup.

### `ClaudeController` tests that assume PTY read calls

- `session/claude_controller_test.go`: Most tests skip PTY interaction (`t.Skip("Requires full instance initialization")`). The `mockInstance.GetPTYReader()` returns `(nil, error)` — so tests that use `mockInstance` do not exercise the PTY read path. These tests are not broken by the multiplexer change, but they also do not provide coverage for the multiplexer integration.

### `CommandExecutor` tests

- `session/command_executor_test.go`: Uses `PTYAccess` backed by a pipe. The executor subscribes to `ResponseStream` via `responseStream.Subscribe()`. If the read path changes, these tests may need to drive the multiplexer goroutine instead of writing directly to the pipe.

### `StreamTerminal` tests

- `server/services/connectrpc_websocket_test.go` and `server/services/session_service_*.go` tests do not test `StreamTerminal` directly with a real PTY. The WebSocket handler (`connectrpc_websocket.go`) intercepts `StreamTerminal` calls in browser clients — the `StreamTerminal` handler in `session_service.go` handles non-browser gRPC clients. There are no unit tests for the `StreamTerminal` PTY read goroutine's EIO behavior.

---

## Breaking Changes

If `PTYAccess.Read()` stops calling `p.pty.Read()` and instead reads from a multiplexer subscriber channel:

1. **`pty_access_test.go`** — All tests using `os.Pipe()` and asserting blocking `Read()` behavior break. The new `Read()` semantics would be "dequeue from channel" not "read from fd."

2. **`response_stream.go:streamLoop()`** — Currently calls `ptyAccess.mu.RLock()` to snapshot `ptyAccess.pty`, then calls `pty.SetReadDeadline()` + `pty.Read()`. If `PTYAccess.Read()` changes, the deadline-based timeout loop in `streamLoop()` no longer functions because `SetReadDeadline` only applies to file I/O, not channel reads.

3. **`ratelimit.NewPTYConsumer(cc.ptyAccess, ...)`** — The rate-limit handler reads from `PTYAccess`. If `PTYAccess.Read()` returns from a channel, the rate-limit consumer still works, but it must be wired as a subscriber to the multiplexer, not as a second `PTYAccess.Read()` caller, or it will not receive any bytes.

4. **`detection/idle.go`** — `IdleDetector` uses `PTYAccess` to detect activity. If `PTYAccess` becomes a channel consumer, idle detection continues to work, but only if the multiplexer delivers bytes to it.

5. **`StreamTerminal` `ptyFile.Read(buf)` call** — This calls `Read()` on the raw `*os.File` returned by `GetPTYReader()`, bypassing `PTYAccess` entirely. If the multiplexer is introduced, `GetPTYReader()` can no longer return the raw fd — it would need to return a channel or a `PTYAccess`-backed reader. This is a breaking API change for `StreamTerminal`.

---

## Summary Table

| Risk | Severity | File(s) | Notes |
|------|----------|---------|-------|
| `RestoreWithWorkDir` closes ptmx mid-read | High | `session/tmux/tmux.go:879` | Multiplexer must detect EIO and re-acquire new ptmx |
| Slow subscriber blocks multiplexer | High | `session/response_stream.go:294` | Must use non-blocking send (current `broadcast()` pattern) |
| EIO from ptmx replace vs. session exit | High | `session/tmux/tmux.go`, `response_stream.go:209` | Multiplexer cannot treat all EIO as terminal EOF |
| Close of subscriber channel while sender active | Medium | `session/response_stream.go:332` | Must guard with per-subscriber done channel or RLock pattern |
| StreamTerminal flow control breaks | Medium | `server/services/session_service.go:1717` | pauseCh pauses whole PTY now; must pause only one subscriber |
| Zero-subscriber PTY drain | Medium | `session/mux/multiplexer.go:562` | Must keep draining even with no subscribers |
| `PTYAccess.Read()` semantics change | High | `session/pty_access_test.go`, `response_stream_test.go` | All pipe-based tests break; deadline-based timeout in streamLoop() breaks |
| `ratelimit.PTYConsumer` not wired to mux | Medium | `session/claude_controller.go:158` | Rate-limit handler must be a subscriber, not a direct reader |
