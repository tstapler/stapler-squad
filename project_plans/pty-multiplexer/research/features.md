# PTY Consumer Data Flow Research

**Bug**: `GetPTYReader()` returns the single `t.ptmx *os.File`. Both `ClaudeController`
and `StreamTerminal` call `Read()` on the same fd simultaneously. PTY reads are destructive —
bytes consumed by one reader are never seen by the other.

---

## Overview

There are **three distinct PTY-read sites** in this codebase, all consuming from the same
underlying `*os.File` returned by `GetPTYReader()`:

| Reader | File | Entry Point | Purpose |
|---|---|---|---|
| `ResponseStream.streamLoop` | `session/response_stream.go:207` | `ClaudeController.Start()` | Populates CircularBuffer; broadcasts to subscribers; feeds IdleDetector / RateLimitHandler |
| `StreamTerminal` output goroutine | `server/services/session_service.go:1753` | ConnectRPC bidi-stream handler | Renders PTY bytes into `TerminalState`; sends MOSH-style state to browser client |
| `TerminalWebSocketHandler` (legacy) | `server/services/terminal_websocket.go:117` | HTTP WebSocket `/ws` endpoint | Forwards raw bytes to legacy WebSocket connection |

`streamViaControlMode` (the primary web streaming path) does **not** call `GetPTYReader()`.
`Preview()` callers do **not** call `GetPTYReader()` — they read from `CircularBuffer`.

---

## ClaudeController Data Flow

**Entry point**: `ClaudeController.Start()` in `session/claude_controller.go:136`

```
ClaudeController.Start()
  ├── cc.instance.GetPTYReader()              → *os.File (the PTY fd)
  ├── NewCircularBuffer(256 * 1024)           → 256 KB in-memory ring buffer
  ├── NewPTYAccess(sessionName, ptyReader, buffer)
  │     └── PTYAccess wraps {pty *os.File, buffer *CircularBuffer}
  ├── NewResponseStream(sessionName, ptyAccess)
  │     └── Holds *PTYAccess reference
  ├── NewPTYConsumer(ptyAccess, ...)          → rate limit handler
  ├── NewIdleDetector(sessionName, ptyAccess) → idle state tracker
  └── responseStream.Start(ctx)
        └── go rs.streamLoop()               → background goroutine
```

**`ResponseStream.streamLoop()`** (`session/response_stream.go:172`):

```
for {
  pty.SetReadDeadline(now + 100ms)
  n, err := pty.Read(readBuf)          // ← DESTRUCTIVE READ from *os.File
  if n > 0 {
    rs.exitTail = append(...)          // rolling 2KB pre-exit buffer
    rs.ptyAccess.buffer.Write(chunk)   // write into CircularBuffer
    rs.escapeParser.Parse(chunk, seq)  // analytics pass-through
    rs.broadcast(chunk)                // fan-out to ResponseChunk channels
    rs.onOutput()                      // signals IdleDetector + RateLimitHandler
                                       //   + statusCheckCh (non-blocking)
  }
}
```

**Downstream from broadcast:**
- **IdleDetector** (`session/detection/`): `RecordActivity()` is called via `onOutput` hook; content-based idle detection reads from `CircularBuffer` via `GetBuffer()` / `GetRecentOutput()`
- **StatusDetector**: triggered by `statusCheckCh` signal; reads content from `instance.Preview()` which in turn calls `ctrl.GetRecentOutput(0)` → `ptyAccess.GetBuffer()` → `CircularBuffer.GetAll()`
- **RateLimitHandler**: receives `NotifyOutput()` via `onOutput` hook; reads buffer directly
- **ResponseStream subscribers**: `ResponseChunk` channels receive raw bytes; used by `CommandExecutor` for timeout/completion detection during `SendCommandImmediate`

**Buffer size**: CircularBuffer allocated at 256 KB (`session/claude_controller.go:153`), though `DefaultBufferSize` constant is 10 MB. Status detection uses only the last 4096 bytes (`statusDetectionTailBytes`) of whatever is in the buffer.

---

## StreamTerminal Data Flow

**Entry point**: `SessionService.StreamTerminal()` in `server/services/session_service.go:1643`

This is the ConnectRPC bidi-stream handler used by **non-WebSocket** gRPC clients.

```
StreamTerminal(ctx, stream)
  ├── instance.GetPTYReader()              → same *os.File as ClaudeController's reader
  ├── session.NewTerminalState(25, 80)     → in-handler ANSI state machine
  └── go (output goroutine):
        for {
          n, readErr := ptyFile.Read(buf)  // ← SECOND DESTRUCTIVE READ from same fd
          if n > 0 {
            instance.UpdateTerminalTimestamps(...)
            terminalState.ProcessOutput(buf[:n])  // render into 2D cell grid
            stateMsg := terminalState.GenerateState()  // MOSH-style snapshot
            stream.Send(stateMsg)          // send complete terminal state to client
          }
        }
```

`TerminalState` (`session/terminal_state.go`) is a self-contained ANSI terminal emulator
(80×25 default, resizable). `ProcessOutput` parses CSI/OSC/SGR escape sequences and updates
a 2D `[][]Cell` grid. `GenerateState()` serializes the full grid into a
`sessionv1.TerminalData_State` proto for MOSH-style synchronization.

**NOTE**: The comment at `session_service.go:1912` says the `CurrentPaneRequest` case
is "unexpected — WebSocket handler should intercept this." In practice, browser clients
route through the WebSocket path (`connectrpc_websocket.go`), not this handler. This
`StreamTerminal` handler may only be active for non-browser gRPC clients.

---

## streamViaControlMode Analysis

**File**: `server/services/connectrpc_websocket.go:472`

`streamViaControlMode` is the **primary browser streaming path** for managed sessions.
It does **not** call `GetPTYReader()` and does **not** do raw PTY reads.

```
streamViaControlMode(stream, instance, streamingMode)
  ├── instance.StartControlMode()          → starts tmux -C (control mode) subprocess
  ├── instance.CapturePaneContentRaw()     → initial snapshot via capture-pane subprocess
  ├── stream.WriteMessage(websocket, ...)  → sends initial snapshot bytes to browser
  ├── subscriberID, updateChan := streamer.SubscribeControlModeUpdates()
  └── go (output goroutine):
        for update := range updateChan {
          // updateChan receives parsed %output notifications from tmux control mode
          // No pty.Read() — data arrives via the tmux -C protocol channel
          sendData(update.Data)            → stream.WriteMessage(websocket, ...)
        }
```

The `updateChan` is populated by the tmux control mode parser which reads from the
**control mode subprocess's stdout**, not from the session PTY. This path is completely
decoupled from the `GetPTYReader()` file descriptor.

**Routing logic** (`connectrpc_websocket.go:440`):
- If `STAPLER_SQUAD_USE_CONTROL_MODE != "false"` **and** `instance.IsManaged` → `streamViaControlMode`
- Otherwise → `streamViaTmuxCapturePane` (polling, no PTY read)

Neither path uses `GetPTYReader()`. The raw PTY read in `StreamTerminal` is only reached
by non-browser gRPC connections.

---

## Preview() Callers

`Preview()` is defined in `session/instance_terminal.go:105`. It does **not** call
`GetPTYReader()` and does **not** read from the PTY fd.

```go
func (i *Instance) Preview() (string, error) {
    if ctrl := i.GetController(); ctrl != nil {
        raw := ctrl.GetRecentOutput(0)   // reads from CircularBuffer, not PTY fd
        return string(raw), nil
    }
    // fallback: capture-pane subprocess
    content, _ := i.pm().CapturePaneContent()
    return content, nil
}
```

**All callers are read-only from CircularBuffer or tmux capture-pane:**

| Caller | File | Purpose |
|---|---|---|
| `ClaudeController.GetCurrentStatus()` | `claude_controller.go:528` | Status detection (tail + hash cache) |
| `ClaudeController.GetIdleState()` | `claude_controller.go:751` | Idle state detection |
| `session_driver` (startup dialog watch) | `session_driver.go:188` | Answer trust-folder dialog |
| `session_driver` (read-back verify) | `session_driver.go:234, 267` | Confirm keystrokes were received |
| `session_driver` (approval watch) | `session_driver.go:549` | Watch for NeedsApproval prompt |
| `AutonomousDriver` (orchestration) | `autonomous_driver.go:156` | Build LLM prompt from current terminal |
| `AutonomousDriver` (completion) | `autonomous_driver.go:177` | Extract PR URL from final output |
| `ApprovalHandler` (LLM approval) | `approval_handler.go:322` | Supply terminal context to autonomous LLM reviewer |
| `SessionService.GetTerminalSnapshot` | `session_service.go:3261` | API: GetTerminalSnapshot RPC |

All of these read from the **CircularBuffer** (via `ctrl.GetRecentOutput(0)` → `buffer.GetAll()`)
when a ClaudeController is active. They never compete with PTY reads.

---

## PTYAccess Buffer Analysis

**File**: `session/pty_access.go`

`PTYAccess` is a thin wrapper that bundles a `*os.File` with a `*CircularBuffer`:

```go
type PTYAccess struct {
    mu          sync.RWMutex
    sessionName string
    pty         *os.File       // the raw PTY fd
    buffer      *CircularBuffer
    closed      bool
}
```

**Key methods:**

| Method | What it returns |
|---|---|
| `Read(buf)` | Delegates directly to `p.pty.Read(buf)` — destructive PTY read |
| `Write(data)` | Delegates to `p.pty.Write(data)` — writes input to PTY |
| `GetBuffer()` | Calls `p.buffer.GetAll()` — returns copy of entire CircularBuffer content |
| `GetRecentOutput(n)` | Calls `p.buffer.GetRecent(n)` — returns last n bytes from buffer |

`GetBuffer()` and `GetRecentOutput()` are non-destructive reads from the in-memory ring buffer.
They never touch the PTY fd.

**CircularBuffer** (`session/circular_buffer.go`):
- Allocated at **256 KB** for ClaudeController sessions (`claude_controller.go:153`)
- The `DefaultBufferSize` constant is 10 MB but is not used for controller sessions
- Thread-safe via `sync.RWMutex`
- Stores `totalBytesWritten` (monotonic counter) for sequence tracking
- `GetAll()` returns a full copy; no concurrent modification risk

The CircularBuffer is **write-once from ResponseStream.streamLoop**, then read-many by
status detectors and Preview() callers. The buffer is the single safe fan-out point for
PTY bytes.

---

## Key Findings

### 1. The Race Condition Is Real and Present

`ClaudeController.Start()` calls `GetPTYReader()` and then `ResponseStream.streamLoop()`
calls `pty.Read()` in a tight loop (100ms deadline). `StreamTerminal` also calls
`GetPTYReader()` and does a concurrent `ptyFile.Read()` with no deadline. Both read from
the **same `*os.File`** returned by `instance.GetPTYReader()`. Bytes consumed by one
goroutine are permanently lost to the other.

### 2. In Practice, streamViaControlMode Avoids the Race for Browser Clients

The routing logic in `connectrpc_websocket.go:440` sends managed sessions through
`streamViaControlMode`, which uses the tmux control mode channel — not `GetPTYReader()`.
The race only manifests when `StreamTerminal` is the active handler, which occurs for:
- Non-browser gRPC clients connecting directly to the ConnectRPC endpoint
- Sessions where `instance.IsManaged == false`
- Environments where `STAPLER_SQUAD_USE_CONTROL_MODE=false`

`TerminalWebSocketHandler` (legacy `/ws` endpoint) also calls `GetPTYReader()` and reads
the PTY directly, creating the same race with `ResponseStream` if both are active.

### 3. Preview() Is Already Safe — It Reads CircularBuffer

All callers of `Preview()` go through `ctrl.GetRecentOutput(0)` → `CircularBuffer.GetAll()`.
This is completely safe and non-destructive. Preview() is not a contributor to the bug.

### 4. CircularBuffer Is the Correct Fan-Out Point

`ResponseStream.streamLoop` is the sole **writer** to the CircularBuffer. All detectors,
`Preview()`, status checks, and idle tracking are **readers** from the CircularBuffer. The
correct fix is to make `StreamTerminal` (and `TerminalWebSocketHandler`) also read from the
CircularBuffer (via a subscription to `ResponseStream`), rather than calling `GetPTYReader()`
directly.

### 5. ResponseStream Already Has a Subscriber API

`ResponseStream.Subscribe(subscriberID)` returns `<-chan ResponseChunk`. This is the existing
fan-out mechanism used by `CommandExecutor`. `StreamTerminal` should subscribe to this channel
instead of reading the PTY fd directly. The `TerminalState.ProcessOutput()` call would then
operate on chunks broadcast from `ResponseStream` rather than bytes stolen from the PTY.

### 6. TerminalWebSocketHandler Is Likely Dead Code for TMux Sessions

The comment at `terminal_websocket.go:77` notes this is a legacy handler. The active path
for browser clients is `connectrpc_websocket.go`. However, it still registers a `GetPTYReader()`
read goroutine that would race with `ResponseStream` if reached.

---

## Proposed Fix Architecture (Summary)

```
PTY fd (*os.File)
    │
    └── ResponseStream.streamLoop (sole reader)
            │
            ├── CircularBuffer (ring buffer, 256 KB)
            │       ├── Preview() → GetCurrentStatus / GetIdleState / session_driver
            │       ├── RateLimitHandler
            │       └── CommandExecutor (via ResponseStream.Subscribe)
            │
            └── ResponseStream.broadcast()
                    ├── CommandExecutor subscriber channel
                    └── [NEW] StreamTerminal subscriber channel
                              └── TerminalState.ProcessOutput(chunk.Data)
                                      └── stream.Send(stateMsg)
```

The PTY fd is read by exactly one goroutine. All consumers fan out from either
the CircularBuffer (for snapshot/history access) or the `ResponseChunk` channel (for
real-time streaming).
