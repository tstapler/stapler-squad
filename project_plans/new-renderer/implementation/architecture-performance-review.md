# Architecture & Performance Review: Terminal Streaming Pipeline

**Reviewer**: Senior Systems Architect  
**Date**: 2026-06-24  
**Branch**: `stapler-squad-new-renderer`  
**Scope**: Full pipeline, PTY → CircularBuffer → ResponseStream → ConnectRPC WebSocket → xterm.js  

---

## 1. Current Architecture Map

### Data Flow Diagram

```
┌───────────────────────────────────────────────────────────────────────────────────────┐
│                              MANAGED SESSION PATH (control mode)                      │
│                                                                                       │
│  [Claude Code process]                                                                │
│       │ raw PTY bytes (VT100/ANSI)                                                    │
│       ▼                                                                               │
│  session/pty_access.go  (os.File PTY fd)                                              │
│       │                                                                               │
│       ▼                                                                               │
│  session/response_stream.go  ResponseStream.streamLoop()                              │
│       │ pty.Read(readBuf[4096])   ← 4 KB read buffer, single goroutine              │
│       │ escapeParser.Parse(chunk, sessionSeq)   ← Stage 1 analytics (passthrough)    │
│       │ ptyAccess.buffer.Write(chunk)           ← CircularBuffer write (O(n)!)       │
│       │ broadcast(ResponseChunk)  ← fan-out; drops on full channel (default!)        │
│       │                                                                               │
│       ▼                                                                               │
│  session/pty_subscriber.go  memPTYSubscriber                                          │
│       │ pushCh (buffered chan, 1024 entries)                                          │
│       │ drain() goroutine: coalesces → ch (64 entries)                               │
│       │                                                                               │
│       ▼                                                                               │
│  server/services/connectrpc_websocket.go  streamViaControlMode()                      │
│       │ 32-frame coalescing batch                                                     │
│       │ escapeParser.ParseStage2(buf, offset)   ← Stage 2 analytics tap              │
│       │ sendData(buf) → proto.Marshal(TerminalData{Output: buf})                     │
│       │                                                                               │
│       ▼                                                                               │
│  ConnectRPC WebSocket (binary protobuf envelopes)                                     │
│                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────────────────────────────────┐
│                         EXTERNAL SESSION PATH (capture-pane polling)                  │
│                                                                                       │
│  tmux control mode / capture-pane (-p -e -J)  ← FULL SCREEN snapshot, NOT raw bytes │
│       │ 50ms debounce after %output event (control mode)                              │
│       │ OR 500ms ticker (fallback polling)                                            │
│       │ Full string comparison for change detection (O(n) per update!)               │
│       │                                                                               │
│       ▼                                                                               │
│  server/services/connectrpc_websocket.go  streamViaTmuxCapturePane()                  │
│       │ formatSnapshotForClient():                                                    │
│       │   sanitizeInitialContent() ← strips cursor/screen-clear/private-mode codes   │
│       │   prepareSnapshotContent() ← \n→\r\n normalization                           │
│       │   ansiSnapshotPrefix prepended (DECSTR + ED2 + CUP)                          │
│       │ FULL SNAPSHOT sent on every detected change                                   │
│       │                                                                               │
│       ▼                                                                               │
│  ConnectRPC WebSocket (binary protobuf envelopes)                                     │
│                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────────────────────────────────┐
│                                  BROWSER / FRONTEND                                   │
│                                                                                       │
│  web-app/src/lib/hooks/useTerminalStream.ts                                           │
│       │ ConnectRPC client, bidirectional streaming                                    │
│       │ TextDecoder.decode(rawData, { stream: true })  ← live path                   │
│       │ TextDecoder.decode(chunk.data)  ← scrollback path (MISSING stream: true!)    │
│       │                                                                               │
│       ▼                                                                               │
│  web-app/src/lib/terminal/TerminalStreamManager.ts                                    │
│       │ Write-lock (isWritingInitialContent) prevents live/history interleaving       │
│       │ RedrawThrottler.process(output)  ← detects /^\x1b\[\d+A/, caps at 10 FPS    │
│       │                                                                               │
│       ▼                                                                               │
│  web-app/src/lib/terminal/EscapeSequenceParser.ts                                     │
│       │ ED3 filter: \x1b[2J\x1b[3J → \x1b[2J                                        │
│       │ Partial sequence buffering (20-char lookback)                                 │
│       │                                                                               │
│       ▼                                                                               │
│  TerminalStreamManager.handleProcessedOutput()                                        │
│       │ Mode-transition refresh (?1049l, ?47l, ?2026l, ?25h)                         │
│       │ Watermark flow control: HIGH=100KB pause, LOW=10KB resume                    │
│       │ Chunked writes: 16 KB slices with requestAnimationFrame yields               │
│       │                                                                               │
│       ▼                                                                               │
│  xterm.js Terminal.write() (@xterm/xterm v6)                                         │
│                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

### Key Subsystems Summary

| Subsystem | File | Role |
|---|---|---|
| PTY reader | `session/response_stream.go:175` | 4 KB reads, fan-out broadcast |
| History store | `session/circular_buffer.go` | 10 MB in-memory ring; O(n) write loop |
| Snapshot generator | `server/services/connectrpc_websocket.go:1033` | Capture-pane polling, full-screen snapshots |
| Control mode handler | `server/services/connectrpc_websocket.go:480` | Raw delta streaming with 32-frame coalesce |
| Input encoding | `server/services/connectrpc_websocket.go:1447` | Per-byte hex subprocess per keystroke |
| Write pipeline | `web-app/src/lib/terminal/TerminalStreamManager.ts` | Watermarks, chunking, redraw throttle |
| Sequence safety | `web-app/src/lib/terminal/EscapeSequenceParser.ts` | Partial-seq buffering, ED3 filter |
| Connection hook | `web-app/src/lib/hooks/useTerminalStream.ts` | ConnectRPC client, state machine |

---

## 2. Performance Analysis

### 2.1 Hot Path Throughput

**Control mode path (managed sessions):**

The PTY read loop in `response_stream.go:181` uses a 4 KB read buffer. At the burst rates typical of Claude Code (build output, code generation: 50–500 KB/s), this means 12–125 syscalls per second and 12–125 `broadcast()` invocations per second. Each broadcast iterates all subscribers and does a non-blocking channel send. This is acceptable for 1–3 subscribers but degrades proportionally with more.

The 32-frame coalescing batch in `connectrpc_websocket.go:745` amortizes WebSocket frame overhead. At 4 KB per read × 32 frames, the maximum coalesce batch is ~128 KB per WebSocket send. This is a reasonable boundary.

**Capture-pane path (external/fallback):**

Full-screen snapshots over tmux capture-pane are the dominant cost. A typical 80×24 terminal with color output is 8–20 KB of text plus ANSI codes. At the 50ms debounce, the maximum throughput is 20 snapshots/second × 20 KB = 400 KB/s sent over WebSocket, even if the actual PTY delta is one character. This is functionally an O(screen_size) operation regardless of how much actually changed.

### 2.2 Memory Pressure

Each session allocates a 10 MB `CircularBuffer`. With 10 concurrent sessions, that is 100 MB. The `CircularBuffer.Write()` loop at `circular_buffer.go:73` is a byte-by-byte copy via `range data`. For a 4 KB PTY read, this executes 4096 iterations in the hot path. No use of `copy()` or bulk operations.

The per-subscriber channel buffer at `response_stream.go:108` is 10,000 `ResponseChunk` entries. Each `ResponseChunk` contains a heap-allocated `[]byte`. At burst rates, this can hold up to several hundred MB of pending data per subscriber, unbounded by back-pressure.

The `StateApplicator.previousLines` map at `StateApplicator.ts:37` holds a decoded string per terminal row. For a 200-row terminal this is small, but it is reallocated on every resize (via `previousLines.clear()`).

### 2.3 Latency Budget

| Stage | Typical latency | Notes |
|---|---|---|
| PTY → CircularBuffer write | ~0.1ms | Dominated by O(n) byte loop |
| PTY → broadcast() | ~0.05ms | Non-blocking channel send |
| capture-pane subprocess | 5–50ms | Process fork, tmux IPC, 5s timeout |
| 50ms debounce (control mode) | 50ms fixed | Adds latency floor even for single keystroke response |
| 32-frame coalescing | 0–few ms | Bounded by PTY read rate |
| WebSocket frame transmission | network RTT | Typically <5ms LAN |
| TextDecoder + onOutput callback | <1ms | Pure JS |
| RedrawThrottler 100ms window | 0–100ms | Adds latency for full-redraw frames |
| xterm.js write() callback | 1–16ms | Async internal buffer, RAF-aligned |

The most impactful latency adds are: 50ms debounce on the server side, and 100ms redraw throttle on the client side. For interactive Claude Code usage (typing, waiting for response), neither matters. For TUI applications with smooth animation, both create visible artifacts.

### 2.4 Input Performance

`sendInputToTmux` at `connectrpc_websocket.go:1447` encodes each byte as hex and spawns a subprocess (`tmux send-keys`) per keystroke. Pasting 100 bytes triggers 100 `exec.Command` calls, each with process fork overhead (~2ms on macOS). A 100-character paste takes ~200ms at the Go level before tmux sees any input. This is the single worst per-operation cost in the system.

---

## 3. Flexibility Analysis

### 3.1 Streaming Mode Architecture

The codebase supports five streaming modes (`raw`, `raw-compressed`, `state`, `hybrid`, `ssp`) via `streamingMode` parameter in `useTerminalStream.ts:51`. However, the Go backend has hardcoded two paths (`streamViaControlMode`, `streamViaTmuxCapturePane`) that do not cleanly align with the frontend mode strings. The `state`, `hybrid`, and `ssp` modes on the frontend reference `StateApplicator`, `DeltaApplicator`, and `EchoOverlay` classes that have server-side counterparts in `server/terminal/state.go` and `server/terminal/delta.go`, but the wiring from the server handler to these subsystems is incomplete or not the default path.

The result is a system that is nominally multi-mode but operationally single-mode (capture-pane raw).

### 3.2 Proto Extensibility

The `TerminalData` oneof in `proto/session/v1/session.proto` already carries `TerminalState`, `TerminalDiff`, `ResizeQuiescence`, `ScrollbackRequest`, `ScrollbackResponse`, and `ShellStatusUpdate`. This is well-designed for forward extension. Adding a new message type does not break existing clients because unrecognized oneof cases are silently ignored by the generated code.

The `CurrentPaneRequest` message carries `streaming_mode` as a string, which allows the client to signal its capability without a proto change.

### 3.3 Analytics System

The two-stage escape analytics system (`pkg/analytics/`) is architecturally complete but operationally dormant. Stage 1 is wired in `response_stream.go:279` with `enabled: false`. Stage 2 is wired in `connectrpc_websocket.go:765` behind `IsEnabled()`. The `MangleCorrelator` for comparing Stage 1 vs Stage 2 sequences is implemented. No configuration toggle or UI exists to activate it. Adding a feature flag or admin endpoint to enable analytics without recompiling is low effort and high value for production debugging.

### 3.4 Observability

Logging uses structured slog-compatible calls throughout the Go code. The frontend has extensive `console.log` instrumentation gated on `localStorage.getItem("debug-terminal") === "true"`. This is practical for development but provides no production telemetry. There is no metrics endpoint, no trace ID propagation through the WebSocket session, and no dashboard for tracking snapshot sizes, coalesce batch depths, or subscriber channel fill levels.

---

## 4. Identified Weaknesses

### CRITICAL: C1 — Per-byte input encoding (file:line `connectrpc_websocket.go:1447`)

Every character the user types spawns a `tmux send-keys` subprocess. The encoding loop calls `exec.Command("tmux", "send-keys", "-t", ..., hexByte)` for each byte. A 100-character paste makes 100 process forks. This produces 100–200ms of latency for non-trivial pastes and has been observed to cause input loss under rapid typing. The fix is to batch the full input string into a single subprocess call with the entire input in one hex-encoded argument, or to use the tmux control-mode stdin pipe for input delivery.

### CRITICAL: C2 — Full-screen snapshot on every change (file:line `connectrpc_websocket.go:1169`)

The `streamViaTmuxCapturePane` path sends the complete terminal screen (80×24 text + ANSI codes, typically 8–20 KB) on every detected change, regardless of how many cells actually changed. A single cursor blink or clock update triggers transmission of the full screen. This makes the default streaming path 10×–100× less efficient than delta streaming. At 20 updates/second during active Claude Code usage, this generates 160–400 KB/s of WebSocket traffic for what should be a few bytes.

### HIGH: H1 — O(n) byte-by-byte CircularBuffer write loop (file:line `circular_buffer.go:73`)

The `Write()` method comments "This is an O(1) operation" but implements a byte-by-byte `for _, b := range data` loop. For a 4 KB PTY read, this executes 4096 iterations. For a 64 KB burst (during code generation), it executes 65536 iterations. The fix is a two-segment `copy()` to handle the wrap-around, reducing to O(1) allocation and two bulk memcpy operations.

The correct implementation:
```go
// First segment: from head to end of buffer (or end of data, whichever is smaller)
space := cb.size - cb.head
first := min(len(data), space)
copy(cb.data[cb.head:], data[:first])
// Second segment (wraps): remaining data from beginning of buffer
if first < len(data) {
    copy(cb.data, data[first:])
}
```

### HIGH: H2 — Silent data drop under backpressure (file:line `response_stream.go:299`)

When a subscriber's channel buffer (10,000 entries) fills, the broadcast drops data silently with only a warn log. There is no backpressure propagated to the PTY reader to slow down. The subscriber never knows data was dropped, so it cannot request a resync. For the WebSocket handler, a dropped chunk means the frontend terminal shows a gap. Under high-output sessions (large file cat, npm install log), this is a practical occurrence.

The upstream chain has no flow control signal from the WebSocket write path back to `broadcast()`. The frontend does implement a watermark flow control (`TerminalStreamManager.ts:363`) that sends a `paused: true` signal to the server, but the server-side handling of this signal in `streamViaControlMode` (if it exists) was not found to be wired to `broadcast()` backpressure.

### HIGH: H3 — `EnableDiskFallback` is a non-functional stub (file:line `circular_buffer.go:179`)

`EnableDiskFallback` creates a temp file but `Write()` never writes to it. The 10 MB buffer is a hard cap with silent data loss on overflow. Long-running sessions that produce more than 10 MB of output (any substantial build or code generation task) lose their earliest history. The stub comment says "future implementation" but callers may rely on the method succeeding to believe overflow protection is active.

### HIGH: H4 — Scrollback chunk TextDecoder missing `{ stream: true }` (file:line `useTerminalStream.ts:308`)

The shared `textDecoderRef.current` is used with `{ stream: true }` on the live output path (line 271) but without it on the scrollback chunk path (line 308). When scrollback chunks are decoded, each call to `decode()` without `stream: true` finalizes the decoder state, flushing any buffered incomplete multi-byte sequence. If a live output chunk arrived just before a scrollback decode, the scrollback call will emit U+FFFD for the dangling bytes from the live path and reset decoder state — corrupting both the scrollback display and the next live output chunk.

### HIGH: H5 — `sanitizeInitialContent` strips private-mode sequences needed by new renderer (file:line `connectrpc_websocket.go:104`)

The `rePositionCodes` regex strips any sequence matching `\x1b\[\?\d+[hl]`. This matches bracketed paste mode (`ESC[?2004h`), alternate screen (`ESC[?1049h`), mouse tracking (`ESC[?1000h`, `ESC[?1006h`), and focus events (`ESC[?1004h`). Claude Code's new renderer likely relies on some of these being present in the initial snapshot replay to set up terminal state. Stripping them from the snapshot means the live stream receives output formatted for an alternate screen but xterm.js is in normal screen mode, causing layout corruption.

### MEDIUM: M1 — O(n) string comparison for capture-pane change detection (file:line `external_tmux_streamer.go:414`)

`checkForUpdates()` compares `content != s.lastContent` where both are full terminal screen strings (8–20 KB). A hash comparison (FNV-1a or xxHash of the raw bytes) would be O(1) instead of O(n) and would avoid the string allocation required for the comparison.

### MEDIUM: M2 — Per-consumer goroutine fan-out in `notifyConsumers` (file:line `external_tmux_streamer.go:449`)

For each change detection, `notifyConsumers` spawns a new goroutine per consumer. If multiple consumers are registered and changes arrive rapidly (control mode burst), goroutine creation can become a source of latency. The goroutines are not bounded. A worker pool with 1–2 goroutines per consumer would be safer.

### MEDIUM: M3 — 20-character lookback in `EscapeSequenceParser` too short for new renderer (file:line `EscapeSequenceParser.ts:83`)

The partial escape sequence detection scans only the last 20 characters for a `\x1b` byte. DCS sequences (`\x1bP...ST`), APC sequences (`\x1b_...ST`), and OSC sequences with long parameters (e.g., `\x1b]8;;https://very-long-url.example.com\x1b\\`) can exceed 20 characters. If the `\x1b` that opens the sequence falls outside the 20-char window, the parser passes an incomplete sequence to xterm.js. The new Claude Code renderer is more likely to emit longer sequences (OSC hyperlinks, DCS passthrough for 256-color profile embedding) than the old one.

### MEDIUM: M4 — `RedrawThrottler` drops intermediate frames (file:line `TerminalStreamManager.ts:65`)

The throttler detects "full redraw" by `/^\x1b\[\d+A/.test(chunk)` (cursor-up at start of chunk). If more than one full-redraw frame arrives within the 100ms window, all but the last are dropped. If Claude Code's animation frames carry incremental state (each frame adds a line, rather than repeating the full state), dropping intermediate frames loses that state. The detection pattern also triggers on any cursor-up at the start of a chunk, which includes normal interactive TUI output that is not a full redraw.

### MEDIUM: M5 — `sanitizeUTF8Bytes()` strips SO/SI and other legitimate control bytes (file:line researched in `server/terminal/state.go`)

The sanitizer in the SSP state generation path replaces any control byte outside `\t`, `\n`, `\r`, `\x07`, `\x08` with a space. This includes `\x0e` (SO, shift-out character set) and `\x0f` (SI, shift-in), used by terminal programs for alternate character sets, as well as `\x00` through `\x06`, `\x10` through `\x1a`. While rare in Claude Code output, this sanitization is applied unconditionally and silently.

### LOW: L1 — `GetAll()` contiguous detection has an incorrect branch condition (file:line `circular_buffer.go:132`)

```go
if !cb.wrapped || (cb.head == 0 && cb.count == cb.size) {
```
The second condition `cb.head == 0 && cb.count == cb.size` is checked but then treated as the non-wrapped path with `copy(result, cb.data)` (copies all `cb.size` bytes). However, when `head == 0` and the buffer is full, the data was written by wrapping — the oldest byte is at `tail`, not index 0. The copy should still use the two-segment approach. This is a bug that silently returns data starting at index 0 instead of at `tail`, incorrectly ordering history in full-buffer scenarios.

### LOW: L2 — 3× SIGWINCH resize nudge with 100ms delays (file:line `connectrpc_websocket.go:1346`)

The resize handler sends three SIGWINCH signals with 100ms delays between them to work around a pty resize race. This adds a guaranteed 200ms of delay after every terminal resize event. The underlying issue is that some programs do not respond to the first SIGWINCH if the pty dimension has not been updated yet by the time the signal arrives. The correct fix is to call `pty.Setsize()` before sending SIGWINCH, not to retry with delays.

### LOW: L3 — `bytes.Split(output, "\n")` in SSP state/delta generators splits OSC sequences (researched in `server/terminal/delta.go:249`, `server/terminal/state.go:181`)

Both SSP generators use `bytes.Split(output, []byte("\n"))` to split terminal output into lines. OSC sequences (OSC 8 hyperlinks, OSC 52 clipboard, OSC 133 shell integration markers) can legitimately contain `\n` bytes within their parameter strings. Splitting on bare `\n` truncates these sequences.

---

## 5. Redesign Proposal

### 5.1 Transport Layer Optimization: Delta Streaming for All Paths

**Current state**: Both control-mode and capture-pane paths send raw bytes or full snapshots respectively. The SSP/state/delta infrastructure exists but is not the default.

**Proposed redesign**: Adopt capture-pane-triggered delta generation as the primary path for all sessions, using the already-implemented `server/terminal/delta.go` and `server/terminal/state.go` infrastructure.

The pipeline becomes:

```
%output event → tmux capture-pane → TerminalStateManager → diff(prev, curr) → TerminalDiff proto
```

The `DeltaApplicator.ts` already handles receiving and applying these diffs. This reduces per-update WebSocket traffic from 8–20 KB (full screen) to 100–2000 bytes (changed cells only) for typical Claude Code usage.

For control-mode sessions (raw PTY access), the same delta approach applies: buffer raw PTY bytes in a virtual terminal emulator (e.g., `creack/pty` + a VT100 state machine), capture the screen state after each read, diff against previous, and send only the delta.

**Migration boundary**: The `streamingMode` proto field on `CurrentPaneRequest` is already wired. The server can select the delta path based on client capability without a flag change.

### 5.2 Circular Buffer: O(1) Bulk Write with Correct Contiguous Copy

Replace the byte-by-byte `for _, b := range data` loop with a two-segment `copy()`:

```go
func (cb *CircularBuffer) Write(data []byte) (int, error) {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    originalLen := len(data)
    cb.totalBytesWritten += int64(originalLen)
    
    if originalLen == 0 {
        return 0, nil
    }
    
    // If data exceeds buffer size, keep only the last cb.size bytes
    if originalLen > cb.size {
        data = data[originalLen-cb.size:]
        copy(cb.data, data)
        cb.head = 0
        cb.tail = 0
        cb.count = cb.size
        cb.wrapped = true
        return originalLen, nil
    }
    
    // Segment 1: from head to end of backing array
    space1 := cb.size - cb.head
    n1 := min(len(data), space1)
    copy(cb.data[cb.head:], data[:n1])
    
    // Segment 2 (if wrap-around needed)
    if n1 < len(data) {
        copy(cb.data, data[n1:])
    }
    
    // Update head
    newHead := (cb.head + len(data)) % cb.size
    
    // Update tail and count
    if cb.count+len(data) > cb.size {
        overflow := cb.count + len(data) - cb.size
        cb.tail = (cb.tail + overflow) % cb.size
        cb.count = cb.size
        cb.wrapped = true
    } else {
        cb.count += len(data)
    }
    cb.head = newHead
    
    return originalLen, nil
}
```

Also fix the `GetAll()` contiguous detection to use `cb.tail` as the start position when `head == 0 && count == size`.

### 5.3 Input Batching: Single Subprocess Per Input Event

Replace the per-byte hex encoding with a whole-string submission:

```go
func sendInputToTmux(sessionName string, data []byte) error {
    // Convert entire input to hex string for tmux send-keys
    var hexStr strings.Builder
    for i, b := range data {
        if i > 0 {
            hexStr.WriteByte(' ')
        }
        fmt.Fprintf(&hexStr, "0x%02x", b)
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    cmd := safeexec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName,
        "-l", hexStr.String())
    return cmd.Run()
}
```

For control-mode sessions, input can be sent directly through the control-mode stdin pipe without any subprocess at all, further reducing latency.

### 5.4 Backpressure and Flow Control: Bilateral Signaling

**Current state**: Frontend sends `paused: true` flow control messages but the Go server does not use them to throttle the PTY reader. The subscriber channel drops silently.

**Proposed**: Wire the flow control signal through to the PTY reader:

1. The WebSocket handler receives a `FlowControl{paused: true}` message.
2. It sets a `flowControlPaused` atomic bool on the PTY subscriber.
3. The `ResponseStream.streamLoop()` checks this flag and applies `time.Sleep(10ms)` in the read loop to create backpressure.
4. When `FlowControl{paused: false}` arrives, the sleep is removed.

This creates a feedback loop: frontend watermark → server slows PTY reads → PTY buffer fills → process-level flow control via OS PTY semantics.

For the immediate channel drop issue, change the default `broadcast()` behavior: instead of `default: drop`, use a short timeout with exponential backoff, and only drop after a configurable grace period (default 100ms). Log the drop with the subscriber ID and session so it is actionable.

### 5.5 EscapeSequenceParser and Transformation Points

Three changes are required to support the new Claude Code renderer:

**A. Extend partial sequence lookback to 256 characters**

Replace `scanLength = Math.min(20, data.length)` with `Math.min(256, data.length)`. This handles OSC hyperlinks, DCS passthrough, and other long sequences without regression on common short sequences.

**B. Add DCS, APC, PM, SOS sequence detection**

Current code handles CSI (`\x1b[`), OSC (`\x1b]`), and simple two-byte escapes. Add:
- DCS: `\x1bP...ST` (terminates with `\x1b\\`)
- APC: `\x1b_...ST`
- PM: `\x1b^...ST`
- SOS: `\x1bX...ST`

These terminate with the String Terminator (`\x1b\\` or `\x07`), same as OSC.

**C. Narrow `rePositionCodes` to preserve private-mode flags**

Change `server/services/connectrpc_websocket.go:71` from:
```go
`|\x1b\[\?\d+[hl]`  // strips ALL private modes including bracketed paste, mouse
```
to only strip alternate screen:
```go
`|\x1b\[\?1049[hl]` // only alternate screen enable/disable
`|\x1b\[\?47[hl]`   // old alternate screen
```

Preserve `ESC[?2004h` (bracketed paste), `ESC[?25l/h` (cursor hide/show), and mouse tracking modes. These are required for correct initial state when the new renderer expects them to be set.

**D. Fix scrollback TextDecoder contamination**

In `useTerminalStream.ts`, replace the shared `textDecoderRef` for scrollback chunks with a dedicated decoder:

```typescript
const scrollbackDecoderRef = useRef(new TextDecoder());

// In scrollbackResponse handler:
for (const chunk of msg.data.value.chunks) {
    const text = scrollbackDecoderRef.current.decode(chunk.data, { stream: true });
    chunks.push(text);
}
// Final flush
scrollbackDecoderRef.current.decode(new Uint8Array(0));
```

---

## 6. Migration Path

### Phase 1: Bug Fixes (1–2 days, no protocol changes)

These fixes address the immediate new-renderer regression and have no compatibility risk:

1. **Fix scrollback TextDecoder** (`useTerminalStream.ts:308`): Add a dedicated decoder for scrollback chunks. Unit test: verify that a multi-byte UTF-8 character split across two consecutive scrollback chunks decodes correctly.

2. **Fix `GetAll()` contiguous detection** (`circular_buffer.go:132`): Correct the branch to use `cb.tail` as the copy origin when the buffer is exactly full. Unit test: fill buffer to exactly `size` bytes, call `GetAll()`, verify the returned byte sequence is in written order.

3. **Narrow `rePositionCodes` regex** (`connectrpc_websocket.go:71`): Preserve bracketed paste, cursor hide/show, and mouse tracking in initial snapshots. Test: connect with new renderer, confirm initial display is correctly formatted.

4. **Extend `EscapeSequenceParser` lookback to 256** (`EscapeSequenceParser.ts:83`): No functional change for sequences < 20 chars. Test: unit test with a 100-char OSC sequence split at byte 85.

5. **Add `{ stream: true }` to all `StateApplicator.decode()` calls** (`StateApplicator.ts`): Critical for the SSP streaming mode path. All three `textDecoder.decode()` calls need the flag.

### Phase 2: Performance Fixes (3–5 days, Go-only changes)

These fixes address hot-path performance with backward-compatible changes:

1. **O(1) CircularBuffer write** (`circular_buffer.go:49`): Replace byte loop with two-segment `copy()`. Run `go test ./session/...` to verify existing tests pass. Benchmark before/after with `go test -bench=BenchmarkCircularBufferWrite`.

2. **Input batching** (`connectrpc_websocket.go:1447`): Send all input bytes in one subprocess call. Test: paste 500 characters, measure round-trip time before/after. Expected improvement: 100× reduction in subprocess spawns.

3. **Full-text to hash change detection** (`external_tmux_streamer.go:414`): Replace `content != s.lastContent` with FNV-1a hash comparison. No behavioral change; pure performance improvement.

4. **Resize SIGWINCH fix** (`connectrpc_websocket.go:1346`): Call `pty.Setsize()` before SIGWINCH, remove triple-send with delays. Test with tmux resize event; verify terminal reflow is correct on first signal.

### Phase 3: Architecture Migration (2–4 weeks, requires protocol awareness)

These changes improve the fundamental transport model:

1. **Delta streaming as default**: Implement a `TerminalStateManager` that wraps the capture-pane loop and maintains prev/curr state. Wire `server/terminal/delta.go` into the default capture-pane path. The frontend `DeltaApplicator.ts` is already implemented and tested.

2. **Control-mode input via stdin pipe**: For managed sessions, send input directly through the control-mode process stdin instead of via subprocess. This eliminates the subprocess entirely for the primary session type.

3. **Backpressure wiring**: Connect the frontend `FlowControl` proto messages to the Go PTY reader via an atomic pause flag. This prevents data loss under burst conditions.

4. **Analytics activation**: Add a configuration field and HTTP admin endpoint to enable escape analytics at runtime. This enables production diagnosis of future renderer regressions without requiring a redeploy.

### Compatibility Matrix

| Phase | Breaking change? | Rollback risk |
|---|---|---|
| Phase 1 bug fixes | No | Low — pure fixes |
| Phase 2 performance | No | Low — behavioral equivalence |
| Phase 3 delta streaming | Yes (protocol) | Medium — requires coordinated frontend+backend deploy |
| Phase 3 input pipe | No | Low — fallback to subprocess if pipe fails |
| Phase 3 backpressure | No | Low — feature-flaggable |
| Phase 3 analytics | No | Low — off by default |

### Test Checkpoints

Before each phase, run:
```bash
make build && make test  # Go tests
cd web-app && npx jest --no-coverage  # Frontend unit tests
make quick-check  # Build + test + lint combined
```

After Phase 3 delta streaming is deployed:
```bash
# Start test server and run e2e
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
cd tests/e2e && npm test
```

Target metrics post-migration:
- Capture-pane WebSocket traffic: < 2 KB per update (down from 8–20 KB)
- Input paste latency: < 5ms for 100 characters (down from ~200ms)
- CircularBuffer write throughput: > 500 MB/s (up from ~50 MB/s)
- No data drops on subscriber channels under normal Claude Code load

---

## Executive Summary

- **Root cause of new-renderer regression**: Three concurrent issues explain the escape code corruption: (1) `sanitizeInitialContent()` strips private-mode sequences (`ESC[?...h/l`) needed by Claude Code's new renderer during initial snapshot replay, (2) the `EscapeSequenceParser` 20-character lookback misses the opening `ESC` of long OSC/DCS sequences emitted by the new renderer, and (3) the shared `TextDecoder` instance for scrollback chunks lacks `{ stream: true }`, contaminating decoder state for the live output path and producing U+FFFD artifacts. Fixing these three items (all in Phase 1, zero protocol changes) is the highest-priority action.

- **Structural performance bottleneck**: The `streamViaTmuxCapturePane` path — which is the default for all sessions — transmits a full screen snapshot (8–20 KB) on every detected change. At 20 changes/second during active usage, this is 160–400 KB/s for what should be 100–500 bytes of deltas. The `sendInputToTmux` function compounds this by spawning one subprocess per character, making paste operations ~200ms for 100 characters. The `CircularBuffer.Write()` byte loop adds unnecessary O(n) CPU cost in the hot path. These three issues together define the performance ceiling of the current architecture.

- **Recommended trajectory**: Fix the three regression bugs immediately (Phase 1, 1–2 days). Apply the Go-only performance fixes as a batch (Phase 2, 3–5 days). Plan the delta streaming migration as a tracked feature (Phase 3, 2–4 weeks) using the already-implemented `DeltaApplicator.ts` and `server/terminal/delta.go` infrastructure. The codebase already contains all the pieces of a best-in-class terminal streaming system; the primary work is wiring them together as the default path and fixing the bugs that prevent the existing infrastructure from working correctly with the new renderer.
