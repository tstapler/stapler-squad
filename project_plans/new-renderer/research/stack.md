# Stack Research: new-renderer

**Date**: 2026-06-24
**Researcher**: Claude Code
**Branch**: stapler-squad-new-renderer

---

## 1. Runtime Versions

| Component | Version | File |
|---|---|---|
| Go | 1.25.0 | `go.mod` |
| `@xterm/xterm` | ^6.0.0 | `web-app/package.json` |
| `@xterm/addon-fit` | ^0.11.0 | `web-app/package.json` |
| `@xterm/addon-webgl` | ^0.18.0 | `web-app/package.json` |
| `@xterm/addon-search` | ^0.15.0 | `web-app/package.json` |
| `@xterm/addon-serialize` | ^0.13.0 | `web-app/package.json` |
| `@xterm/addon-web-links` | ^0.11.0 | `web-app/package.json` |
| `connectrpc.com/connect` | v1.19.0 | `go.mod` |
| `github.com/gorilla/websocket` | v1.5.3 | `go.mod` |
| `github.com/creack/pty` | v1.1.24 | `go.mod` |

---

## 2. Terminal Rendering Pipeline

```
[Claude Code process]
     ↓  PTY bytes
[tmux session]
     ↓  capture-pane polling (streamViaTmuxCapturePane)
[connectrpc_websocket.go]
     ↓  protobuf TerminalData.output (binary)
[ConnectRPC / gorilla/websocket]
     ↓  WebSocket stream
[useTerminalStream.ts]
     ↓  TextDecoder.decode(data, {stream: true})
[TerminalStreamManager.ts]
     ↓  chunked writes (16 KB)
[EscapeSequenceParser.ts]
     ↓  filtered / boundary-safe chunks
[xterm.js Terminal.write()]
```

The default streaming path for ALL sessions is `streamViaTmuxCapturePane` in
`server/services/connectrpc_websocket.go`. The legacy direct-PTY path
(`streamViaControlMode`) is disabled by default
(`STAPLER_SQUAD_USE_CONTROL_MODE=false`).

---

## 3. Key Files Per Stage

### Stage A: Go transport — `server/services/connectrpc_websocket.go` (1542 lines)

Primary file for terminal streaming. Key sections:

| Line range | What it does |
|---|---|
| ~71 | `rePositionCodes` regex (see Section 5) |
| ~85 | `ansiSnapshotPrefix` = DECSTR + ED2 + CUP constants |
| ~104 | `sanitizeInitialContent()` — applies rePositionCodes regex |
| ~116 | `prepareSnapshotContent()` — normalises `\n` → `\r\n` |
| ~147 | `formatSnapshotForClient()` — builds DECSTR+ED2+CUP + content + cursor |
| ~480 | `streamViaControlMode()` — legacy path, control-mode only |
| ~1033 | `streamViaTmuxCapturePane()` — **primary path for all sessions** |

### Stage B: PTY subscriber — `session/pty_subscriber.go`

`memPTYSubscriber` — coalescing ring channel, cap 1024 (~4 MB). Pure
passthrough: does NOT strip or modify bytes.

### Stage C: Analytics tap — `session/response_stream.go`

Calls `rs.escapeParser.Parse(chunk.Data, sessionSeq)` (line 279).
The parser is created but `enabled: false` by default — analytics is wired
but dormant.

### Stage D: React hook — `web-app/src/lib/hooks/useTerminalStream.ts`

Handles ConnectRPC bidirectional stream. For raw `output` messages:
`TextDecoder.decode(rawData, { stream: true })` → calls `onOutput(text)`.
Terminal state machine: `DISCONNECTED → CONNECTING → LOADING → STABLE`.

### Stage E: Write buffering — `web-app/src/lib/terminal/TerminalStreamManager.ts`

- Write buffer with HIGH (100 KB) / LOW (10 KB) watermarks
- Chunks writes at 16 KB to prevent UI freeze
- `RedrawThrottler`: throttles full-screen redraws to 10 FPS (100 ms window)
  — **drops intermediate frames** when Claude's animation emits frequent
  full-screen redraws

### Stage F: Sequence safety — `web-app/src/lib/terminal/EscapeSequenceParser.ts`

Prevents ANSI sequences from being split across xterm.js `write()` calls.
Also contains the **ED3 filter** (see Section 5).

### Stage G: xterm.js — `web-app/src/components/sessions/XtermTerminal.tsx`

React wrapper. Loads `WebglAddon` with canvas fallback. Key options:
`allowProposedApi: true`, `scrollback: 5000`. Exposes `XtermTerminalHandle`
ref with `write()`, `writeln()`, `fit()`, `serializeAddon`.

---

## 4. Analytics System (terminal-analytics, status: IMPLEMENTED / DORMANT)

All infrastructure exists in `pkg/analytics/`:

| File | Contents |
|---|---|
| `escape_code_parser.go` | Full ANSI parser (CSI, OSC, DCS, PM, APC, SOS, Charset, Simple) |
| `escape_code_store.go` | In-memory singleton store (`GetGlobalStore()`); `SetEnabled()` |
| `escape_event_writer.go` | `EscapeEventWriter` interface; `EscapeEventRecord` struct |
| `mangle_correlator.go` | Ring-buffer correlator for Stage 1/2 sequence matching |

Wiring status:
- **Stage 1** (PTY → transport): `session/response_stream.go` line 279 calls
  `escapeParser.Parse()` — wired, but `enabled: false`
- **Stage 2** (transport → client): instrumentation exists in parser, not yet
  connected to the capture-pane streaming path
- **SQLite persistence**: not hooked up; in-memory store only

The `session/claude_controller.go` `GetEscapeParser()` method and
`session/instance_controller.go` `GetEscapeParser()` accessor provide handles
for enabling the parser at runtime.

---

## 5. Stripping / Transformation Points (Ordered by Pipeline Stage)

### 5.1 `sanitizeInitialContent()` — Go, snapshots only

**File**: `server/services/connectrpc_websocket.go` ~line 104
**What it strips** (via `rePositionCodes` regex):
```go
var rePositionCodes = regexp.MustCompile(
    `\x1b\[\d;?\d[Hf]`   +  // Absolute cursor: ESC[H, ESC[n;mH
    `|\x1b\[\d*J`         +  // Screen clear: ESC[J, ESC[1J, ESC[2J, ESC[3J
    `|\x1b\[\?\d+[hl]`   +  // Private mode: ESC[?1049h (alt screen), ESC[?25l
    `|\x1b[78]`           +  // DEC save/restore cursor
    `|\x1b\[[su]`,           // CSI save/restore cursor
)
```
**Scope**: Applied only to initial snapshot content, NOT live streaming output.
**Risk**: If the new Claude renderer emits private-mode sequences
(e.g. `ESC[?2004h` — bracketed paste mode) in the snapshot, they are stripped.
The regex `\x1b\[\?\d+[hl]` is broad and would match any `ESC[?NNNh/l`.

### 5.2 ED2+ED3 filter — TypeScript, all live output

**File**: `web-app/src/lib/terminal/EscapeSequenceParser.ts` line 39
```typescript
const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```
**What it strips**: Removes ED3 (`ESC[3J`, erase scrollback) when immediately
preceded by ED2 (`ESC[2J`, erase visible screen).
**Risk**: If the new Claude renderer emits `ED2+ED3` as a combined "clear
everything" reset, the scrollback-erase portion is silently dropped. This
would cause xterm.js to retain stale scrollback that the renderer expected
to be gone, leading to duplicate or corrupted history.

### 5.3 RedrawThrottler — TypeScript, all live output

**File**: `web-app/src/lib/terminal/TerminalStreamManager.ts`
**What it drops**: Intermediate full-screen redraws within a 100 ms window.
Detection pattern: `/^\x1b\[\d+A/` (cursor-up at start of chunk).
**Risk**: If the new Claude renderer generates animation frames at >10 FPS
(e.g. spinner animations, progress bars), intermediate frames are discarded
and only the most recent frame in each 100 ms window is written. If those
intermediate frames carry state (e.g. incremental line writes), the final
state rendered may be incorrect.

### 5.4 `server/terminal/state.go` and `delta.go` — Go, state/delta modes only

Both files contain their own `stripANSIBytes()` functions for visible
character counting. These are used only in `state` and `hybrid` streaming
modes, not the default `raw` / capture-pane path.

### 5.5 `server/mcp/ansi.go` `stripANSI()` — intentional, MCP path only

**File**: `server/mcp/ansi.go`
**What it strips**: ALL ANSI escape sequences from bytes.
Used only for MCP tool output display, never for the terminal display path.
Must not be changed (confirmed constraint in requirements).

---

## 6. Snapshot vs. Live Output: Critical Distinction

The pipeline has two distinct data paths with different transformation rules:

**Snapshot path** (initial terminal state on connect):
- Goes through `sanitizeInitialContent()` + `formatSnapshotForClient()`
- Prepended with `DECSTR + ED2 + CUP` prefix to reset terminal state
- Has `rePositionCodes` applied (strips cursor positioning, screen clears, etc.)
- Normalises `\n` → `\r\n`

**Live streaming path** (ongoing output):
- Goes through `streamViaTmuxCapturePane()` polling loop
- Does NOT go through `sanitizeInitialContent()` or `rePositionCodes`
- Passes through `EscapeSequenceParser.ts` (ED2+ED3 filter + split prevention)
- Subject to `RedrawThrottler`

If the new Claude renderer changed how it handles the initial render vs.
incremental updates, the snapshot path's aggressive stripping could be the
cause of broken initial display state.

---

## 7. Streaming Modes Available

The `streamingMode` parameter in `useTerminalStream.ts` supports:
`"raw"` (default), `"raw-compressed"` (LZMA), `"state"`, `"hybrid"`, `"ssp"`.

The default `raw` mode uses the capture-pane path. The `ssp` mode (Stapler
Streaming Protocol) adds predictive echo for latency masking.

---

## 8. What "New Renderer" Likely Refers To

Based on the requirements and codebase context, the "new renderer" is a change
in Claude Code itself (the external `claude` CLI binary), not in stapler-squad.
Claude Code v0.2+ introduced a new terminal UI framework that:
- May emit more aggressive full-screen redraws (triggering `RedrawThrottler`)
- May use `ED2+ED3` combined sequences for screen reset
- May rely on `ESC[?2004h` (bracketed paste mode) or other private modes in
  initial setup that get stripped by `rePositionCodes` in snapshots
- May use alternate screen (`ESC[?1049h`/`1049l`) more aggressively, which
  `rePositionCodes` strips from snapshots

The new renderer is observable via the tmux capture-pane output; changes to
Claude Code's output format are transparent to the PTY layer.

---

## 9. Analytics Activation Path

To activate the escape code analytics system (enabling diagnosis):

1. Call `escapeParser.SetEnabled(true)` on the parser returned by
   `instance.GetEscapeParser()` — enables Stage 1 (PTY→transport)
2. Connect Stage 2 tap in `streamViaTmuxCapturePane()` to call
   `escapeParser.ParseStage2()` after each capture-pane read
3. Hook up an `EscapeEventWriter` to the `EscapeCodeStore` to persist events
4. Use `MangleCorrelator.RecordStage1()` / `RecordStage2()` to identify
   sequences present at Stage 1 but absent at Stage 2 (= stripping point)

This pattern is already fully implemented — it just needs to be switched on
via configuration or a debug flag.
