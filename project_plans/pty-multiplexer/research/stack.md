# PTY Read Path Stack Research

## Overview

The stapler-squad PTY read path has two distinct streaming architectures that run concurrently for managed sessions:

1. **Control-mode path** (primary, managed sessions): `tmux -C` attach → `SubscribeControlModeUpdates()` channel → WebSocket. Does NOT call `GetPTYReader()`. This is the happy path for the browser UI.
2. **PTY direct-read path** (legacy/external sessions and internal controller): `GetPTYReader()` returns `*os.File` → caller reads bytes directly via `ptyFile.Read()`. Used by `StreamTerminal` (ConnectRPC BidiStream), the legacy `terminal_websocket.go` handler, `ResponseStream.streamLoop()` (the internal fan-out broadcaster), and `main.go` debug tooling.

There is a **critical design tension**: `StreamTerminal` and `ResponseStream` both call `ptyFile.Read()` on the same `*os.File` simultaneously if both are active for the same session. A PTY file descriptor is a character device — each `read(2)` syscall consumes bytes from the kernel ring buffer. Two concurrent readers will race and each sees only a subset of the output. This is the core problem any PTY multiplexer must solve.

---

## PTY Read Path Components

### 1. `session/pty_access.go` — `PTYAccess`

The synchronization wrapper around a `*os.File` PTY.

```
PTYAccess {
    mu          sync.RWMutex
    sessionName string
    pty         *os.File     // raw PTY file descriptor
    buffer      *CircularBuffer
    closed      bool
}
```

Key methods:
- `Write(data []byte)` — acquires write lock, calls `p.pty.Write()`.
- `Read(buf []byte)` — acquires **read** lock (RLock, not Lock), calls `p.pty.Read()`. Multiple concurrent `Read()` callers are therefore permitted by the lock — there is no serialization. This is intentional design for concurrent read scenarios but means byte-level races when two goroutines both call `PTYAccess.Read()`.
- `GetBuffer() []byte` — returns a copy of the circular buffer's full contents.
- `GetRecentOutput(n int) []byte` — returns the last `n` bytes from the circular buffer.
- `UpdatePTY(*os.File)` — replaces the underlying file descriptor (e.g., after detach/reattach).
- `Close()` — marks closed but does NOT close the underlying file; tmux owns the fd lifecycle.

`PTYAccess` does NOT start any goroutines. It is a passive holder. The buffer is populated externally by `ResponseStream.streamLoop()`.

---

### 2. `session/response_stream.go` — `ResponseStream` (the canonical PTY reader goroutine)

**This is the single goroutine that owns PTY consumption for managed sessions.**

`ResponseStream` holds a `*PTYAccess`. On `Start(ctx)`, it launches a single background goroutine `streamLoop()`:

```
streamLoop():
  for {
    pty := ptyAccess.pty          // read-locked snapshot
    pty.SetReadDeadline(now + 100ms)
    n, err := pty.Read(readBuf)   // DIRECT pty.Read on the *os.File
    if n > 0:
      exitTail rolling update
      if onOutput != nil: onOutput()   // notifies IdleDetector
      sessionSeq = buffer.TotalBytesWritten()
      escapeParser.Parse(chunk.Data, sessionSeq)
      buffer.Write(chunk.Data)    // ← THIS is what populates PTYAccess.buffer
      broadcast(chunk)            // fan-out to all Subscriber channels
  }
```

**Buffer population**: `ResponseStream.streamLoop()` is the sole writer to `PTYAccess.buffer`. The circular buffer is NOT written from anywhere else. Consumers of the buffer (`GetRecentOutput`, `GetBuffer`) only read.

**Fan-out**: `ResponseStream` maintains a `subscribers map[string]*Subscriber`. Each subscriber gets a buffered channel (`bufferSize = 10000` chunks). `broadcast()` sends to every subscriber channel non-blocking (drops on full with a log warning).

---

### 3. `session/claude_controller.go` — `ClaudeController`

Orchestrates the per-session controller. On `Start(ctx)`:

1. Calls `instance.GetPTYReader()` → gets `*os.File`.
2. Creates `CircularBuffer(256KB)`.
3. Creates `PTYAccess(sessionName, ptyReader, buffer)`.
4. Creates `ratelimit.PTYConsumer(ptyAccess, rateLimitManager)` — polls buffer via `GetRecentOutput`.
5. Creates `ResponseStream(sessionName, ptyAccess)` — the actual PTY reader goroutine.
6. Creates `StatusDetector`, `IdleDetector(sessionName, ptyAccess)`.

`ClaudeController` itself does NOT read the PTY directly. It delegates all PTY reading to `ResponseStream`.

`GetRecentOutput(n int)` on the controller reads from `ptyAccess.GetRecentOutput(n)` which reads from the circular buffer. This is the source for `Preview()` (no subprocess call needed when controller is active).

---

### 4. `session/instance_tmux.go` — `Instance.GetPTYReader()`

```go
func (i *Instance) GetPTYReader() (*os.File, error) {
    i.stateMutex.RLock()
    defer i.stateMutex.RUnlock()
    if !i.started { return nil, fmt.Errorf("session not started") }
    return i.pm().GetPTY()
}
```

Delegates to `ProcessManager.GetPTY()` which returns the raw PTY `*os.File` from the underlying tmux attach process. This is the root of the PTY file descriptor.

`WriteToPTY(data []byte)` delegates to `i.pm().SendKeys(string(data))` — sends keys through tmux, NOT through the PTY fd directly.

`ResizePTY(cols, rows int)` delegates to `i.pm().SetWindowSize(cols, rows)`.

---

### 5. `session/instance_terminal.go` — `Instance.Preview()`

```go
func (i *Instance) Preview() (string, error) {
    if ctrl := i.GetController(); ctrl != nil {
        raw := ctrl.GetRecentOutput(0)  // reads from circular buffer, no PTY read
        return string(raw), nil
    }
    // Fallback: capture-pane subprocess
    content, err := i.pm().CapturePaneContent()
    ...
}
```

`Preview()` does NOT call `GetPTYReader()` or read the PTY directly. When the `ClaudeController` is active, it reads from the in-memory circular buffer. When no controller, it falls back to `tmux capture-pane`.

---

### 6. `server/services/session_service.go` — `StreamTerminal` handler (lines 1643–1900)

The ConnectRPC BidiStream handler for the new terminal protocol.

```go
ptyFile, err := instance.GetPTYReader()  // line 1698
// ...
// Goroutine 1 (output): reads PTY directly
n, readErr := ptyFile.Read(buf)          // line 1753
// sends deltas via TerminalState (MOSH-style)

// Goroutine 2 (input): receives from client, calls instance.WriteToPTY()
// Goroutine 2 (resize): calls instance.ResizePTY()
// Goroutine 2 (flow control): pause/resume via pauseCh
```

**This handler reads `ptyFile` directly**, bypassing `PTYAccess` and `ResponseStream`. If `ResponseStream` is also running (as it will be for any managed session with a controller), they compete for bytes from the same kernel PTY ring buffer.

---

### 7. `server/services/terminal_websocket.go` — legacy WebSocket handler

```go
ptyReader, err := instance.GetPTYReader()  // line 80
// ...
n, err := ptyReader.Read(buf)              // line 117, in goroutine
```

Same pattern as `StreamTerminal` — reads the PTY `*os.File` directly in a goroutine. Legacy handler, but still in use for non-control-mode paths.

---

### 8. `server/services/connectrpc_websocket.go` — `streamViaControlMode`

Does NOT call `GetPTYReader()`. Uses:
- `streamer.StartControlMode()` / `streamer.StopControlMode()`
- `streamer.SubscribeControlModeUpdates()` → `<-chan []byte`

Output is received from the control-mode channel (tmux `-C` protocol), NOT from the PTY file descriptor. This is the architecturally clean path and does not compete with `ResponseStream`.

---

### 9. `session/instance_shells.go` — shell drain loop

```go
_, readErr := pty.Read(buf)  // line 200
```

Used in `watchShellExit` to drain the PTY of a shell subprocess until EOF. This `pty` is a separate PTY created for the shell subprocess (not the main session PTY), so it does not conflict with the session-level readers.

---

### 10. `main.go` — debug/diagnostic tooling (line 438)

```go
ptyReader, err := inst.GetPTYReader()
```

Used only in the CLI debug command that prints PTY file descriptor info. Not part of the production serving path.

---

## Callers Map

| File | Line | What calls | Context |
|---|---|---|---|
| `session/claude_controller.go` | 147 | `cc.instance.GetPTYReader()` | `ClaudeController.Start()` — gets fd to pass to `PTYAccess` |
| `session/detection/ratelimit/integration.go` | 148,153 | `pc.buffer.GetRecentOutput(4096)` | `PTYConsumer.pollLoop()` — reads from circular buffer, NOT PTY fd |
| `session/response_stream.go` | 207 | `pty.Read(readBuf)` | `streamLoop()` goroutine — the canonical PTY reader |
| `server/services/session_service.go` | 1698 | `instance.GetPTYReader()` | `StreamTerminal` handler setup |
| `server/services/session_service.go` | 1753 | `ptyFile.Read(buf)` | `StreamTerminal` output goroutine — DIRECT PTY read |
| `server/services/terminal_websocket.go` | 80 | `instance.GetPTYReader()` | Legacy WebSocket handler setup |
| `server/services/terminal_websocket.go` | 117 | `ptyReader.Read(buf)` | Legacy WebSocket output goroutine — DIRECT PTY read |
| `session/instance_tmux.go` | 240 | `i.pm().GetPTY()` | `GetPTYReader()` implementation |
| `session/instance_shells.go` | 200 | `pty.Read(buf)` | `watchShellExit` drain loop (separate shell PTY, not session PTY) |
| `main.go` | 438 | `inst.GetPTYReader()` | Debug CLI command (not production) |
| `testutil/expect.go` | 191, 357 | `s.pty.Read(buf)` | Test harness PTY reader |
| `tuitest/integration/claude_squad/keyboard_test.go` | 102 | `c.pty.Read(buffer)` | Integration test |

---

## Buffer Population Analysis

### Who populates `PTYAccess.buffer`?

`PTYAccess.buffer` (`*CircularBuffer`) is populated **exclusively** by `ResponseStream.streamLoop()` at line 284:

```go
if rs.ptyAccess.buffer != nil {
    rs.ptyAccess.buffer.Write(chunk.Data)
}
```

This write happens after the `pty.Read()` call in the background goroutine, immediately before `broadcast()`.

### Is there a separate goroutine for reading vs. writing to the buffer?

**Yes — there is exactly one goroutine** (`streamLoop`) that both reads from the PTY fd and writes to the buffer. The same goroutine that calls `pty.Read(readBuf)` also calls `buffer.Write(chunk.Data)`. There is no producer/consumer split within the buffer write path.

The buffer is then read by many consumers, all from separate goroutines:
- `PTYAccess.GetRecentOutput(n)` → called by `ratelimit.PTYConsumer.pollLoop()` (its own goroutine)
- `PTYAccess.GetBuffer()` → called on demand (e.g., from controller status detection)
- `ctrl.GetRecentOutput(0)` → called by `Instance.Preview()` (any caller's goroutine)
- `IdleDetector` via its own internal mechanisms

All buffer reads go through `PTYAccess.mu.RLock()`, providing safe concurrent read access. Writes in `streamLoop` also need to go through a lock — but looking at the code, `buffer.Write()` is called directly without acquiring `ptyAccess.mu`. The `CircularBuffer` itself must have its own internal synchronization.

---

## Key Findings

### Finding 1: Three simultaneous PTY readers are possible for a managed session

For a session with a running `ClaudeController`:
- `ResponseStream.streamLoop()` reads the PTY fd continuously (background goroutine)
- `StreamTerminal` Goroutine 1 also calls `ptyFile.Read()` directly when a web client connects via ConnectRPC BidiStream
- Legacy `terminal_websocket.go` Goroutine 1 also calls `ptyReader.Read()` if the legacy path is active

All three read from the same `*os.File` fd. The kernel TTY discipline delivers each byte to exactly one `read(2)` call. Bytes are split non-deterministically between readers.

### Finding 2: `streamViaControlMode` is isolated from PTY fd races

The control-mode WebSocket path (`connectrpc_websocket.go:streamViaControlMode`) does NOT call `GetPTYReader()`. It uses tmux's `-C` (control mode) protocol over a separate subprocess stdin/stdout pair. Its output channel (`SubscribeControlModeUpdates`) is entirely separate from the PTY fd. This path is safe for multiplexing.

### Finding 3: `PTYAccess.Read()` uses `RLock` — concurrent direct reads are not serialized

`PTYAccess.Read()` acquires `p.mu.RLock()`, which allows multiple goroutines to call it simultaneously. Each call goes to `p.pty.Read(buf)`. This means the lock does NOT prevent the race — it only prevents reads from running concurrently with `UpdatePTY` or `Close`. The design intentionally does not serialize PTY reads.

### Finding 4: The circular buffer has a single writer and multiple readers

`buffer.Write()` is called only from `ResponseStream.streamLoop()`. All `GetRecentOutput()` / `GetBuffer()` calls are reads. There is no second writer — the buffer is a "write once per PTY read, read many" store. Consumers of the buffer (rate limit detector, idle detector, Preview) always see a snapshot of what `streamLoop` has consumed so far.

### Finding 5: `StreamTerminal` bypasses the circular buffer entirely

`StreamTerminal` reads bytes from `ptyFile.Read()` and processes them through `TerminalState` for MOSH-style delta encoding before sending to the client. These bytes never touch `PTYAccess.buffer`. If `ResponseStream` and `StreamTerminal` race on the PTY fd, the bytes that `StreamTerminal` wins are NOT recorded in the buffer, creating gaps in the history visible to `Preview()`, status detection, and rate limit detection.

### Finding 6: `WriteToPTY` goes through tmux sendkeys, not the PTY fd

`Instance.WriteToPTY()` delegates to `i.pm().SendKeys(string(data))` which sends a `tmux send-keys` command. It does NOT write to the PTY file descriptor. This means input and output paths are fully decoupled at the process level — input goes via tmux control, output comes via PTY read.

### Architectural Implication for PTY Multiplexer

A PTY multiplexer must ensure that `ResponseStream.streamLoop()` is the **single reader** of the PTY fd for managed sessions. `StreamTerminal` and legacy WebSocket handlers should consume from `ResponseStream`'s fan-out subscriber channels, NOT call `GetPTYReader()` directly. The control-mode path already does this correctly (via tmux `-C`). The direct-PTY paths need to be converted to subscribe to `ResponseStream` channels instead.
