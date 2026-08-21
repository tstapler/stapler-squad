# PTY Fan-Out Multiplexer: Architecture Research

**Date**: 2026-06-13
**Status**: Research / Design

---

## Overview

`GetPTYReader()` returns `t.ptmx *os.File` directly (via `TmuxSession.GetPTY()`). PTY reads are **destructive**: each `Read()` call consumes bytes from the kernel FIFO. Two goroutines calling `Read()` on the same fd race to consume output; whichever goroutine wins gets the bytes, and the other sees nothing.

Currently there are **three concurrent consumers** of the same PTY fd:

| Consumer | Location | How it reads |
|---|---|---|
| `ResponseStream.streamLoop()` | `session/response_stream.go:207` | `pty.Read(readBuf)` — direct fd access via `PTYAccess.pty` |
| `StreamTerminal` RPC goroutine | `server/services/session_service.go:1753` | `ptyFile.Read(buf)` — own `*os.File` from `GetPTYReader()` |
| `TerminalWebSocketHandler` | `server/services/terminal_websocket.go:117` | `ptyReader.Read(buf)` — own `*os.File` from `GetPTYReader()` |

`ResponseStream.streamLoop()` also writes received bytes into `PTYAccess.buffer` (a `CircularBuffer`), but `StreamTerminal` and the WebSocket handler bypass that buffer entirely and read from the raw fd.

---

## Existing Infrastructure

### CircularBuffer (`session/circular_buffer.go`)

A thread-safe, in-memory circular buffer (default 10 MB, though `ClaudeController` allocates 256 KB) with:
- `Write([]byte) (int, error)` — O(1) append, overwrites oldest on overflow
- `GetAll() []byte` — full linearized snapshot
- `GetRecent(n int) []byte` — last N bytes
- `TotalBytesWritten() int64` — monotonic write counter (can serve as a byte-position cursor)
- `WriteTo(io.Writer)` — streaming export

The buffer already tracks `totalBytesWritten` which is exactly what a cursor-based subscription model needs.

### PTYAccess (`session/pty_access.go`)

A thin wrapper over `*os.File` with:
- `Read()` / `Write()` delegating directly to `p.pty.Read(buf)` / `p.pty.Write(data)` — **no multiplexing**
- `GetBuffer()` / `GetRecentOutput(n int)` — delegates to `CircularBuffer`
- `UpdatePTY(*os.File)` — hot-swap for detach/reattach
- No background goroutine; every `Read()` call goes straight to the OS

`PTYAccess` does **not** have a fan-out read loop. Each call to `p.pty.Read()` is a raw system call.

### PTYSubscriber (`session/pty_subscriber.go`)

Already exists as a fully-built fan-out primitive (added recently, build-tag `!windows`):
- `PTYSubscriber` interface: `Push([]byte) error`, `Chan() <-chan []byte`, `Close()`
- `memPTYSubscriber` implementation: two-stage pipeline (push channel → drain goroutine → consumer channel)
- Coalesces small chunks in the drain goroutine to reduce channel overhead
- `maxPushBufEntries = 1024` (~4 MB at 4 KB/chunk) before `ErrSubscriberFull`

This is the **missing link**: `PTYSubscriber` exists as a subscriber abstraction but there is no multiplexer goroutine that reads the PTY and calls `Push()` on registered subscribers.

### mux.Multiplexer (`session/mux/multiplexer.go`)

The external SSQ-MUX multiplexer (for IntelliJ/VS Code) already solves this same problem at the OS level: a single goroutine (`forwardPTYOutput`) reads `m.ptmx` and calls `broadcastToClients(msg)`. However, this is for external sessions attaching to a separate tmux attach-session process. It is **not** reusable for the internal `TmuxSession.ptmx` fan-out.

### ResponseStream (`session/response_stream.go`)

`streamLoop()` is already the de-facto primary reader of `PTYAccess.pty`. It:
1. Reads from `ptyAccess.pty` (raw fd) with a 100ms deadline
2. Writes to `ptyAccess.buffer` (CircularBuffer)
3. Calls `rs.onOutput()` (activity tracking + status check signal)
4. Broadcasts `ResponseChunk` to `rs.subscribers` map

So `ResponseStream` is already doing fan-out to its own subscriber set. The problem is that `StreamTerminal` and the WebSocket handler do **not** go through `ResponseStream` — they call `GetPTYReader()` and read the fd directly.

---

## The Exact Race

```
TmuxSession.ptmx (*os.File)
       │
       ├── ResponseStream.streamLoop() ← reads via PTYAccess.pty.Read()
       │       writes to CircularBuffer, broadcasts to ResponseChunk subscribers
       │
       ├── StreamTerminal goroutine ← reads via ptyFile.Read()  [DESTRUCTIVE RACE]
       │       streams to ConnectRPC client
       │
       └── TerminalWebSocketHandler ← reads via ptyReader.Read() [DESTRUCTIVE RACE]
               streams to WebSocket client
```

Bytes consumed by `StreamTerminal` are never seen by `ResponseStream` (and thus not written to `CircularBuffer`, not triggering `onOutput`, not available to `ClaudeController`). The race is non-deterministic and data-loss is proportional to how fast each consumer reads.

---

## Option A: Channel Fan-Out (Multiplexer Goroutine)

**Design**: Add a single goroutine inside `PTYAccess` (or a new `PTYMultiplexer` struct) that owns the PTY read loop. It reads from `ptmx` and `Push()`es into each registered `PTYSubscriber`. Existing consumers subscribe via `Subscribe()` and read from their channel.

```
PTYMultiplexer
  ptmx *os.File
  subscribers map[string]PTYSubscriber  ← protected by RWMutex
  │
  └── readLoop() goroutine
        reads ptmx → Push() to each subscriber
```

Consumers become:
- `ResponseStream.streamLoop()` reads from `subscriber.Chan()` instead of `ptyAccess.pty.Read()`
- `StreamTerminal` creates a subscriber, reads from `subscriber.Chan()`
- `WebSocketHandler` creates a subscriber, reads from `subscriber.Chan()`

**Pros**:
- Exactly models the existing `mux.Multiplexer.forwardPTYOutput()` pattern already in this codebase
- `PTYSubscriber` is already implemented and tested (`session/pty_subscriber.go`)
- No OS-level trickery; pure Go
- Late subscribers can receive a replay of the `CircularBuffer` contents on join (history catch-up)
- Backpressure is per-subscriber: slow subscriber gets `ErrSubscriberFull` and is evicted; others are unaffected
- `PTYAccess.UpdatePTY()` (for detach/reattach) just needs to restart the read goroutine on the new fd

**Cons**:
- Adds one goroutine per session (bounded, low memory)
- Requires changing `ResponseStream.streamLoop()` to read from a channel instead of calling `pty.Read()` directly (moderate refactor)
- `StreamTerminal` must create/destroy a subscriber per RPC call (but it already manages lifetimes via `context.WithCancel`)
- The 100ms `SetReadDeadline` used in `streamLoop()` must move to the multiplexer's read loop

---

## Option B: Ring Buffer + Read Cursor

**Design**: The multiplexer writes to a shared append-only ring buffer (`CircularBuffer` already exists). Each subscriber holds a monotonic byte-offset cursor (`int64`). Reading is a poll/wait against `CircularBuffer.TotalBytesWritten()`.

```
PTYMultiplexer
  ptmx *os.File
  ring *CircularBuffer         ← already implemented
  totalWritten int64           ← already on CircularBuffer
  cond *sync.Cond              ← broadcast on every Write
  │
  └── readLoop() goroutine
        reads ptmx → ring.Write() → cond.Broadcast()

Subscriber:
  cursor int64                 ← starts at ring.TotalBytesWritten() at subscribe time
  next() []byte                ← cond.Wait() until totalWritten > cursor, then read
```

**Pros**:
- Zero per-subscriber channel allocation; all subscribers share one buffer
- Constant memory for N subscribers (only one copy of data in ring, N cursors)
- New subscribers can read full history from ring immediately (cursor = 0, or cursor = oldest offset)
- No `ErrSubscriberFull` / slow-subscriber eviction: slow subscriber simply falls behind; if it falls off the ring end, it detects the gap via `cursor < ring.tail` offset
- No drain goroutines; simpler lifecycle

**Cons**:
- Requires adding `Cond`-based notification to `CircularBuffer` (it currently has no signal mechanism)
- Subscribers must poll in a tight loop or use `sync.Cond.Wait()` — cannot `select` on a Cond; this breaks the idiomatic Go `select` pattern that `ResponseStream` currently uses with `ctx.Done()`
- `sync.Cond` + `select` interop requires a bridge goroutine or `context.AfterFunc` — adds complexity
- Slow subscriber that falls off the ring loses data silently (or detects it via offset comparison, but has no recovery path other than "reconnect and miss")
- Current `CircularBuffer` overwrites old data — there is no offset-to-position mapping, making "read from cursor" non-trivial to implement without restructuring `CircularBuffer`

---

## Option C: Pipe Tee (OS-Level Byte Duplication)

**Design**: For each subscriber, create an `os.Pipe()` pair. A single multiplexer goroutine reads `ptmx` and writes to N write-ends using `io.MultiWriter`. Each subscriber reads from their own pipe's read-end.

```
ptmx *os.File
  └── readLoop() goroutine
        reads ptmx
        io.MultiWriter → [pipeW1, pipeW2, pipeW3]

Subscriber 1: pipeR1  (standard *os.File, SetReadDeadline works)
Subscriber 2: pipeR2
Subscriber 3: pipeR3
```

**Pros**:
- Subscribers receive `*os.File` — identical API to `GetPTYReader()` today; `StreamTerminal` code is literally unchanged
- `SetReadDeadline` works on pipe fds the same as PTY fds
- Kernel-level buffering: the OS pipe buffer (64 KB on Linux) absorbs bursts
- No Go channel overhead; bytes transferred via kernel

**Cons**:
- Each subscriber consumes 2 file descriptors (a pipe pair); fd pressure per session scales with subscriber count
- `io.MultiWriter` is synchronous and sequential: `pipeW.Write()` blocks if the pipe buffer is full — a slow subscriber stalls the entire multiplexer loop, including fast subscribers
- Workaround for blocking: non-blocking writes to pipes, but detecting `EAGAIN` and skipping/dropping for a slow subscriber re-introduces the slow-subscriber problem without backpressure signaling
- Pipe buffers are not circular and do not support history replay; late subscribers see only future bytes
- `io.TeeReader` only handles 2 consumers (tee into one extra reader); N>2 requires manual fan-out anyway
- Requires fd cleanup on subscriber disconnect; leaking pipe fds is a real risk

---

## Comparison Matrix

| Criterion | A: Channel Fan-Out | B: Ring Buffer + Cursor | C: Pipe Tee |
|---|---|---|---|
| Correctness (no data loss to fast subscribers) | Yes | Yes | Yes (if non-blocking writes) |
| Slow subscriber isolation | Yes (ErrSubscriberFull → evict) | Yes (falls behind silently) | No (blocks multiplexer) |
| History replay for late subscribers | Yes (via CircularBuffer) | Yes (native, via cursor=0) | No |
| Go select compatibility | Yes (channel) | No (Cond.Wait) | Yes (os.File) |
| fd pressure | None | None | 2 per subscriber |
| API change to StreamTerminal | Moderate (channel read) | Moderate (cursor poll) | Minimal (same *os.File) |
| Requires existing code changes | PTYAccess + ResponseStream | CircularBuffer + new type | None (if *os.File returned) |
| Implementation complexity | Low | Medium | Medium |
| Aligns with existing PTYSubscriber | Directly | Indirectly | No |

---

## Recommendation: Option A (Channel Fan-Out)

Option A is the recommended approach because:

1. **PTYSubscriber already exists and is tested.** `session/pty_subscriber.go` implements exactly the `Push()` / `Chan()` abstraction needed. The infrastructure is built; only the goroutine that calls `Push()` is missing.

2. **The pattern is already proven in this codebase.** `mux.Multiplexer.forwardPTYOutput()` does this exact read-and-broadcast to N clients. Option A reuses the same pattern at the internal session layer.

3. **Option C's blocking write problem is serious.** A single slow WebSocket client would pause PTY reads for the entire session, starving `ClaudeController`'s status detection and response streaming. Option A evicts the slow subscriber via `ErrSubscriberFull` and continues serving others.

4. **Option B's Cond/select impedance mismatch is real.** Every existing Go consumer uses `select` with `ctx.Done()` for cancellation. Wrapping `sync.Cond.Wait()` in a select-compatible bridge negates Option B's complexity advantage over Option A.

5. **Option A gives the best slow-subscriber story.** When `memPTYSubscriber.pushCh` fills (`maxPushBufEntries = 1024` chunks ≈ 4 MB), `Push()` returns `ErrSubscriberFull`. The multiplexer can log a warning, close the subscriber, and force the consumer to reconnect — identical to how `mux.Multiplexer.broadcastToClients()` uses a 100ms write deadline and ignores send errors.

---

## Migration Strategy

### Step 1: Introduce PTYMultiplexer

Create `session/pty_multiplexer.go` (build-tag `!windows` to match `pty_subscriber.go`):

```go
type PTYMultiplexer struct {
    mu          sync.RWMutex
    pty         *os.File
    subscribers map[string]PTYSubscriber
    buf         *CircularBuffer  // existing buffer, kept for history replay
    ctx         context.Context
    cancel      context.CancelFunc
    onOutput    func()           // forwarded to ResponseStream callbacks
}

func (m *PTYMultiplexer) Subscribe(id string) (PTYSubscriber, error) { ... }
func (m *PTYMultiplexer) Unsubscribe(id string) { ... }
func (m *PTYMultiplexer) UpdatePTY(f *os.File) { /* restart readLoop */ }
func (m *PTYMultiplexer) readLoop() {
    buf := make([]byte, 4096)
    for {
        m.pty.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        n, err := m.pty.Read(buf)
        // handle EOF, timeout, errors identically to ResponseStream.streamLoop()
        if n > 0 {
            m.buf.Write(buf[:n])          // write to CircularBuffer (history)
            if m.onOutput != nil { m.onOutput() }
            m.fanOut(buf[:n])             // push to all subscribers
        }
    }
}
```

### Step 2: Refactor PTYAccess

`PTYAccess` gains a `*PTYMultiplexer` field. The `Read()` method becomes a `Subscribe()` call (for backward compat, or removed). `NewPTYAccess()` starts `PTYMultiplexer.readLoop()`.

### Step 3: Refactor ResponseStream

`ResponseStream.streamLoop()` replaces:
```go
n, err := pty.Read(readBuf)
```
with:
```go
data := <-sub.Chan()
```

The `onOutput` callback, buffer write, and broadcast all move into `PTYMultiplexer.readLoop()` (for the buffer write and `onOutput`) while the ResponseChunk broadcast stays in `ResponseStream` as a subscriber consumer.

### Step 4: Fix StreamTerminal

`StreamTerminal` replaces:
```go
ptyFile, err := instance.GetPTYReader()
// ...
n, readErr := ptyFile.Read(buf)
```
with:
```go
sub, err := instance.SubscribePTY(streamCtx, "stream-terminal-"+sessionID)
defer sub.Close()
// ...
data := <-sub.Chan()
```

`Instance.SubscribePTY()` is a thin delegation to `PTYAccess`'s multiplexer.

### Step 5: Fix TerminalWebSocketHandler

Same pattern as Step 4.

### Transparent Compatibility

Consumers that do not need to change:
- `CommandExecutor` — writes to PTY via `PTYAccess.Write()` (unaffected; writes go to `ptmx.Write()` directly)
- `ratelimit.PTYConsumer` — reads via `PTYAccess.GetRecentOutput()` (reads from CircularBuffer, which is now written by the multiplexer's read loop; unaffected)
- `IdleDetector` — driven by `onOutput` callback, which moves into the multiplexer; unaffected
- `ClaudeController.statusCheckCh` signal — currently triggered from `ResponseStream.SetOnOutput()`, which is satisfied by the multiplexer's `onOutput`

The `GetPTYReader()` method on `Instance` can remain temporarily for callers that are not yet migrated, but it should return a subscriber's `*os.File` shim or be deprecated in favor of `SubscribePTY()`.

### Rollout Risk

- The one goroutine-per-session overhead is bounded and trivial (Go goroutines are ~8 KB stack)
- The read loop logic is identical to what `ResponseStream.streamLoop()` does today, so behavior is preserved
- Existing `PTYSubscriber` tests cover the `Push`/`Chan`/`Close` semantics
- No proto or frontend changes required
