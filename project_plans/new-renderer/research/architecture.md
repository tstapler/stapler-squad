# Architecture Research: new-renderer (terminal rendering bug)

**Date**: 2026-06-24
**Status**: Research complete
**Problem**: Claude Code's new renderer introduced escape code corruption in the display pipeline

---

## 1. Complete Data Flow Map: PTY → xterm.js

The pipeline has **two distinct paths** depending on the `STAPLER_SQUAD_USE_CONTROL_MODE` env var:

### Path A: Control Mode (primary path — managed sessions, default)

```
Claude Code (PTY process)
    │ raw bytes, escape codes intact
    ▼
session/pty_access.go  (PTY file descriptor read via os.File)
    │
    ▼
session/response_stream.go  ResponseStream.streamLoop()
    │   pty.Read(readBuf) → 4096-byte chunks
    │   escapeParser.Parse(chunk.Data, sessionSeq)       ← analytics tap (passthrough, no modification)
    │   ptyAccess.buffer.Write(chunk.Data)               ← writes to CircularBuffer (10 MB in-mem)
    │   broadcast(ResponseChunk{Data: []byte})           ← fan-out to all subscribers
    │
    ▼
session/pty_subscriber.go  memPTYSubscriber
    │   Push(data []byte) → pushCh (buffered chan, 1024 entries)
    │   drain() goroutine coalesces pushCh → ch (consumer-facing chan, 64 entries)
    │
    ▼
session/tmux/pty.go  (SubscribeControlModeUpdates → updateChan chan []byte)
    │   [tmux control mode notifications: %output lines from tmux -C]
    │
    ▼
server/services/connectrpc_websocket.go  streamViaControlMode()
    │   for { select { case data := <-updateChan: ... } }
    │   Coalescing loop: buf = append(buf, data...) for up to 32 frames
    │   escapeParser.ParseStage2(buf, totalBytesWritten)  ← Stage 2 analytics tap
    │   sendData(buf) → proto.Marshal(TerminalData{Output: buf}) → WebSocket binary frame
    │
    ▼
WebSocket (gorilla/websocket binary frame)
    │   protocol.CreateEnvelope(0, dataBytes)
    │
    ▼
web-app/src/lib/hooks/useTerminalStream.ts  (ConnectRPC WebSocket client)
    │
    ▼
web-app/src/lib/terminal/TerminalStreamManager.ts  TerminalStreamManager.write(output)
    │   isWritingInitialContent check (write-lock; queues to pendingLiveWrites if locked)
    │   RedrawThrottler.process(output)                   ← POTENTIAL STRIPPING POINT
    │       Full-redraw detection: /^\x1b\[\d+A/.test(chunk)
    │       If full-redraw: deferred 100ms (coalesces to latest)
    │   EscapeSequenceParser.processChunk(result)         ← POTENTIAL STRIPPING/BUFFERING POINT
    │       ED3 filter: strips \x1b[3J when paired with \x1b[2J
    │       Partial-sequence buffering at chunk boundary
    │   handleProcessedOutput(safeOutput)
    │       Detect mode transitions for refresh (?1049l, ?47l, ?2026l, ?25h)
    │       enqueueWrite (chunked at 16 KB)
    │
    ▼
xterm.js Terminal.write(data)  (via @xterm/xterm)
```

### Path B: tmux Capture-Pane (legacy/external sessions, or when control mode disabled)

```
tmux capture-pane -p  (full terminal snapshot, NOT raw PTY bytes)
    │   prepareSnapshotContent(): sanitizeInitialContent() strips \x1b\[\d*;?\d*[Hf] etc.
    │   \n → \r\n normalization
    │   ansiSnapshotPrefix prepended (DECSTR + ED2 + CUP)
    │
    ▼
WebSocket → TerminalStreamManager.writeInitialContent() [each snapshot]
    │   terminal.clear()
    │   enqueueWrite(content)  ← goes through EscapeSequenceParser.processChunk
    │
    ▼
xterm.js
```

### Initial Snapshot (both paths on connect)

On every new WebSocket connection (control mode path), the server sends an initial snapshot:
```
instance.CapturePaneContentRaw() → prepareSnapshotContent() → formatSnapshotForClient()
    → ansiSnapshotPrefix + sanitizeInitialContent() + withCursorSync()
```

`sanitizeInitialContent` strips via regex `rePositionCodes`:
- `\x1b\[\d*;?\d*[Hf]` (absolute cursor positioning)
- `\x1b\[\d*J` (screen clears)
- `\x1b\[\?\d+[hl]` (private mode: alternate screen, cursor visibility)
- `\x1b[78]` (DEC save/restore cursor)
- `\x1b\[[su]` (CSI save/restore cursor)

---

## 2. All Points Where Bytes Can Be Modified/Stripped/Transformed

### Go Side (server)

| Location | File | What it does | Risk to display |
|---|---|---|---|
| `stripANSI` | `server/mcp/ansi.go` | Strips ALL escape codes | **NOT on display path** — only called from `tools_terminal.go` `readSessionOutput()` for MCP tool output |
| `sanitizeInitialContent` | `server/services/connectrpc_websocket.go` line ~104 | Strips positioning/screen-clear/private-mode codes via regex | **ONLY on initial snapshot** from capture-pane; does NOT affect streaming PTY bytes |
| `prepareSnapshotContent` | same, line ~116 | `\n` → `\r\n` normalization; strips rePositionCodes | **ONLY on initial snapshot** |
| `withCursorSync` | same, line ~131 | Appends a CUP escape to re-sync cursor position | additive, not stripping |
| `rePositionCodes` regex | line ~71 | Private mode flags, absolute cursor codes | Only applied to capture-pane snapshots, NOT live PTY bytes |
| `detectContentWidth / stripAnsiCodes` | line ~1526 | Local `ansiEscapeRe` regex strip | **ONLY for width measurement diagnostics**, result is discarded and not sent to client |
| Coalescing in `streamViaControlMode` | line ~738–757 | Batches up to 32 frames | No modification — only concatenation |
| `memPTYSubscriber.drain()` | `session/pty_subscriber.go` | Coalesces small chunks | No modification |
| `CircularBuffer.Write` | `session/circular_buffer.go` | Circular overwrite on overflow | No modification; pure byte storage |

### JavaScript Side (browser)

| Location | File | What it does | Risk |
|---|---|---|---|
| `EscapeSequenceParser.processChunk` | `web-app/src/lib/terminal/EscapeSequenceParser.ts` | **Strips ED3 (`\x1b[3J`) when paired with ED2** — filters `\x1b[2J\x1b[3J` → `\x1b[2J`; also buffers partial escape sequences at chunk boundaries | **HIGH RISK**: The new Claude Code renderer may emit sequences this parser misidentifies as partial or incorrectly strips |
| `RedrawThrottler.process` | `web-app/src/lib/terminal/TerminalStreamManager.ts` | Defers full-screen redraws detected by `/^\x1b\[\d+A/` — coalesces to 10 FPS | **HIGH RISK**: If Claude Code's new renderer emits frames that look like "full redraws" (start with cursor-up), they are deferred and only the latest is sent to xterm.js — older frames are dropped |
| `handleProcessedOutput` | same | Forces `terminal.refresh()` after detecting `?1049l, ?47l, ?2026l, ?25h` | Low risk — additive |
| `isWritingInitialContent` write-lock | same | Queues live writes during initial content load | Low risk if lock releases correctly |

### Critical Finding: `stripANSI` in `server/mcp/ansi.go` is NOT on the display path

The `stripANSI` function (`server/mcp/ansi.go`) is only called from:
- `server/mcp/tools_terminal.go` line 202–203: `readSessionOutput` MCP tool (strips for AI consumption)
- `server/mcp/tools_terminal.go` line 416: `waitForOutput` polling loop
- `server/mcp/tools_terminal.go` line 454: timeout fallback in same tool

It is **never called** in the WebSocket streaming path (`connectrpc_websocket.go`), `ResponseStream`, or any path that feeds xterm.js. The `stripANSI_` alias in `tools_terminal.go` confirms the separation.

---

## 3. Most Likely Root Causes for the New Renderer Bug

### Hypothesis 1: `EscapeSequenceParser` partial-sequence buffering breaks with new renderer output

The `EscapeSequenceParser.processChunk` has a 20-character lookback for partial sequence detection (`scanLength = Math.min(20, data.length)`). Claude Code's new renderer may emit longer escape sequences (e.g., DCS passthrough, longer OSC sequences, or PM/APC sequences) that exceed this 20-char scan window. If the lookback misses the opening `\x1b`, the partial sequence is not buffered and the first part is written alone to xterm.js — garbling the terminal.

Additionally, the ED3 filter:
```typescript
const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```
If the new renderer emits ED2+ED3 in two separate chunks (which is common with control-mode coalescing), the regex never matches and ED3 passes through intact — or if previously it wasn't emitting ED3 at all and now does, the filter may interact unexpectedly.

### Hypothesis 2: `RedrawThrottler` drops frames from the new renderer

The new renderer may change the pattern of frames emitted. If it emits frames that:
- Start with `\x1b[\d+A` (cursor-up) on every update (common in Ink/React-like TUI frameworks)
- Then the `RedrawThrottler` defers them all and only flushes the latest

At 10 FPS cap with 100ms throttle, if Claude Code emits 20 redraws in 100ms (normal for interactive TUI), only the last frame reaches xterm.js. If the last frame is a partial state (e.g., mid-animation frame), the terminal shows corrupted content.

### Hypothesis 3: Control-mode coalescing produces sequences that span the 20-char lookback boundary

The `memPTYSubscriber.drain()` coalesces PTY chunks, then `streamViaControlMode` coalesces up to 32 frames. This means `EscapeSequenceParser.processChunk` receives large concatenated buffers (potentially 10s of KB) where the new renderer's escape sequences may be split at a point that the 20-char scan misses.

### Hypothesis 4: `sanitizeInitialContent` strips sequences needed by the new renderer

The initial snapshot uses `rePositionCodes` to strip private-mode sequences (`\x1b\[\?\d+[hl]`). The new renderer may rely on private mode flags being sent during the initial snapshot replay to set up terminal state correctly (e.g., bracketed paste mode, focus events, mouse tracking). These are stripped by `sanitizeInitialContent`, leaving xterm.js in wrong state for the live stream.

---

## 4. Terminal-Analytics Instrumentation Points (from existing plan)

The `project_plans/terminal-analytics/implementation/plan.md` designs a two-stage analytics system that directly maps to the instrumentation needed here:

### Stage 1 Tap (already wired)
**File**: `session/response_stream.go` line ~278–280
```go
if rs.escapeParser != nil {
    rs.escapeParser.Parse(chunk.Data, sessionSeq)
}
```
This is **already in place**. The `EscapeCodeParser.Parse` is a passthrough observer — it does not modify `chunk.Data`. The `sessionSeq` is captured from `rs.ptyAccess.buffer.TotalBytesWritten()` before the write.

### Stage 2 Tap (already wired)
**File**: `server/services/connectrpc_websocket.go` lines ~765–769
```go
if escapeParser != nil && escapeParser.IsEnabled() {
    escapeParser.ParseStage2(buf, instance.GetTotalBytesWritten())
}
```
This captures the coalesced transport frame (`buf`) immediately before `sendData(buf)`.

### JavaScript-Side Tap (not yet wired)
The `EscapeSequenceParser` and `RedrawThrottler` in `TerminalStreamManager` have no instrumentation. Adding `console.log` or a callback hook before/after `processChunk` would expose what bytes are being dropped or buffered.

### Correlation: Stage 1 → Stage 2
The terminal-analytics plan designed `MangleCorrelator` to compare `(sessionID, sessionSeq, payloadHash)` between Stage 1 and Stage 2. The `totalBytesWritten` counter on `CircularBuffer` (monotonically increasing) is the stable correlation key.

---

## 5. Minimal Fix Strategy (Once Root Cause is Identified)

### For Hypothesis 1 (partial-sequence buffering):
- Extend `EscapeSequenceParser.findPartialEscapeAtEnd`'s `scanLength` from 20 to at least 256 characters to handle longer sequences from the new renderer
- Add support for DCS (`\x1b P`), PM (`\x1b ^`), and APC (`\x1b _`) sequence types which the current parser does not detect

### For Hypothesis 2 (RedrawThrottler dropping frames):
- The `RedrawThrottler` regex `/^\x1b\[\d+A/` may be over-aggressive with the new renderer
- Mitigation: disable the throttler entirely (set `throttleMs = 0` or remove the full-redraw detection) and observe if rendering improves
- Or: change the detection to only throttle when there are 3+ consecutive full-redraw frames within 16ms (one display frame)

### For Hypothesis 3 (coalescing boundary):
- In `streamViaControlMode`, track the raw byte position of each coalesced chunk so `EscapeSequenceParser` can receive per-PTY-read-aligned boundaries
- Or: ensure `EscapeSequenceParser` receives the `buf` in smaller, PTY-read-sized pieces

### For Hypothesis 4 (sanitizeInitialContent over-stripping):
- Log the exact sequences being stripped from the initial snapshot
- The new renderer may require `\x1b[?2004h` (bracketed paste), `\x1b[?1006h` (SGR mouse), or `\x1b[?1004h` (focus events) to reach xterm.js even in snapshot replay
- Narrow `rePositionCodes` to strip only absolute cursor positioning (`[Hf]`) and screen clears (`[J]`), not private mode flags

---

## 6. Recommended Instrumentation Injection Points

To identify the actual root cause, add logging at these points in order of likelihood:

1. **JavaScript: `EscapeSequenceParser.processChunk` entry/exit** — log `data.length`, `filtered.length`, `partial.length`, and any partial sequence that was buffered. This immediately shows if sequences are being dropped.

2. **JavaScript: `RedrawThrottler.process` entry** — log when a frame is deferred vs. passed through. Count dropped frames per second.

3. **Go: `connectrpc_websocket.go` `sendData` closure** — log `len(buf)` and hex-dump the first 64 bytes before sending. Compare with what `parseStage2` sees.

4. **Go: `session/response_stream.go` `broadcast`** — log `len(chunk.Data)` and first 32 bytes after `pty.Read`. Confirms bytes enter the pipeline unmodified.

The analytics system designed in `terminal-analytics/implementation/plan.md` provides the SQLite-backed, production-grade version of these taps. For the immediate bug fix, temporary `console.log` on the JS side and a structured log on the Go `sendData` path are the fastest path to root cause.

---

## 7. Files of Interest

| File | Role in pipeline |
|---|---|
| `session/response_stream.go` | PTY reader, Stage 1 escape analytics tap |
| `session/circular_buffer.go` | In-memory PTY history storage; `TotalBytesWritten()` is the `session_seq` counter |
| `session/pty_subscriber.go` | Fan-out subscriber + coalescing drain goroutine |
| `server/services/connectrpc_websocket.go` | WebSocket handler, Stage 2 tap, coalescing loop, `sanitizeInitialContent`, `rePositionCodes` |
| `server/mcp/ansi.go` | `stripANSI` — MCP-only, NOT on display path |
| `web-app/src/lib/terminal/EscapeSequenceParser.ts` | JS-side partial-sequence buffering + ED3 stripping |
| `web-app/src/lib/terminal/TerminalStreamManager.ts` | `RedrawThrottler` + write pipeline |
| `web-app/src/components/sessions/XtermTerminal.tsx` | xterm.js initialization and terminal options |
| `project_plans/terminal-analytics/implementation/plan.md` | Full analytics system design with Stage 1+2 tap architecture |
