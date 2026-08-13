# Implementation Plan: Scrollback History Delivery to WebSocket Clients

Status: Ready for Implementation
Created: 2026-04-17
Updated: 2026-04-21
ADRs: `project_plans/stapler-squad/decisions/ADR-001..003`

## Session Progress (2026-04-21)

**Test coverage added** to `server/services/connectrpc_websocket_test.go`:
- `sanitizeInitialContent()` — 5 tests verifying positioning codes stripped, SGR colors preserved
- `waitForQuiescence()` — 4 tests covering quiet period, timeout, channel close, timer reset
- `getOrRefreshSnapshot()` + `markSnapshotDirty()` — 6 tests covering cache miss/hit/dirty/error/concurrent
- `isAllowedOrigin()` — 5 tests covering localhost, HTTPS, HTTP-non-localhost, malformed

**Remaining test gap:** `streamViaControlMode()` and `streamViaTmuxCapturePane()` still have zero integration tests.
To cover them: implement a `mockSessionStreamer` satisfying the 4-method `SessionStreamer` interface
(`server/services/session_streamer.go`) and test the full connect sequence.

**Not yet started:** Stories 1–4 from this plan (the actual scrollback history injection).

---

## Epic Overview

When a client connects to a terminal stream, they see only the current visible pane (~24-50 lines from `CapturePaneContentRaw()`). This plan wires the existing `CapturePaneContentWithOptions()` method in `session/tmux/tmux.go` into the WebSocket connect path so that the last N lines of tmux scrollback history are sent to the client before the current pane snapshot. The user sees history in their xterm.js scrollback buffer and can scroll up to read it.

### What exists today

- `CapturePaneContentWithOptions(start, end string)` in `session/tmux/tmux.go` — runs `tmux capture-pane -p -e -J -S <start> -E <end> -t <session>`, can capture arbitrary line ranges including history
- `CircularBuffer` and `ScrollbackManager` in `session/scrollback/` — stores raw PTY bytes, NOT used for this feature (see ADR-001)
- `scrollbackManager *scrollback.ScrollbackManager` field in `ConnectRPCWebSocketHandler` — wired but unused for client delivery
- `ScrollbackRequest` / `ScrollbackResponse` protobuf messages — already defined in `events.proto`, not used by current server handlers
- `requestScrollback` in `useTerminalStream.ts` — already implemented on the client side
- `onScrollbackReceived` callback in `useTerminalStream.ts` — partially wired for the deprecated `currentPaneResponse` case

### What is missing

- The call to `CapturePaneContentWithOptions()` at connect time in `streamViaControlMode()` and `streamViaTmuxCapturePane()`
- Configuration for how many lines to deliver
- The server-side handler for `ScrollbackRequest` in the current `connectrpc_websocket.go` (removed in refactor, present in `.bak2` files)

### Architecture Decision Records

- ADR-001: Use tmux capture-pane (not CircularBuffer raw bytes) — `project_plans/stapler-squad/decisions/ADR-001-scrollback-source-tmux-vs-circular-buffer.md`
- ADR-002: Push history at connect, before current pane snapshot, using `TerminalOutput` framing — `project_plans/stapler-squad/decisions/ADR-002-scrollback-delivery-timing-and-framing.md`
- ADR-003: Default 1000 lines, max 5000, configurable via `STAPLER_SQUAD_SCROLLBACK_LINES` env var — `project_plans/stapler-squad/decisions/ADR-003-scrollback-history-line-limit-configuration.md`

---

## Dependency Visualization

```
Story 1 (P0)                 Story 2 (P1)                Story 3 (P2)
Configuration +              Control mode path:          Capture-pane polling path:
instance.GetScrollbackHistory  streamViaControlMode()    streamViaTmuxCapturePane()
helper method                history inject              history inject

  |                             |                          |
  |  no deps                    |  depends on Story 1      |  depends on Story 1
  v                             v                          v
[Story 1] ──────────────> [Story 2] ──────────────> [Story 3]

Story 4 (P3, independent, deferred)
On-demand scrollback request handler
  |
  |  independent of Stories 1-3 (uses same helper)
  v
[Story 4]
```

Stories 2 and 3 can proceed in parallel after Story 1. Story 4 is independent but lower priority since push-at-connect covers 95% of user needs.

---

## ANSI/Terminal Safety Analysis

This is the highest-risk area. Summary of findings:

### Why tmux capture-pane output is safe to replay

`tmux capture-pane -S -N` with `-J` (join wrapped lines) outputs the terminal history as a series of complete, newline-terminated text lines with ANSI color/attribute codes preserved but with **cursor-positioning codes already removed by tmux**. Specifically:

- ESC[H, ESC[n;mH (cursor absolute position) — stripped by tmux for history
- ESC[2J, ESC[3J (clear screen) — stripped by tmux for history
- ESC[?1049h/l (alternate screen switch) — stripped by tmux for history
- ESC[nm (SGR colors/attributes) — preserved

This is precisely the set of "context-dependent" codes that the existing `sanitizeInitialContent()` / `rePositionCodes` regex in `connectrpc_websocket.go` strips from raw PTY captures. tmux has already done this work for history lines — **do not run sanitizeInitialContent() on history output from tmux**. Applying the sanitizer to tmux history would strip nothing extra (tmux already removed those codes) but adds unnecessary processing overhead.

### The correct message send order

```
(1) Resize tmux + quiescence wait     — existing code, unchanged
(2) Capture history lines             — NEW: CapturePaneContentWithOptions("-1000", "-1")
(3) Send history as TerminalOutput    — NEW: if len(history) > 0, send bytes + reset sequence
(4) Capture current pane snapshot     — existing getOrRefreshSnapshot()
(5) Send current pane as TerminalOutput — existing code, unchanged
(6) Start live streaming goroutines   — existing code, unchanged
```

The `ESC[H` (cursor home) at the start of the current pane snapshot (step 5) repositions the cursor to row 1, col 1 of the viewport. History lines (step 3) were already written and xterm.js moved them into scrollback as the viewport filled. The user sees the current pane state in the viewport and can scroll up to see history.

### Separator between history and live content

After sending history bytes, send `\x1b[0m\r\n` before the current pane snapshot. This:
- Resets ANSI attributes (prevents a bold/colored last line of history bleeding into the pane content)
- Issues CRLF so the snapshot starts at column 0

The current pane snapshot already begins with `\x1b[H` which repositions the cursor regardless, so this separator is belt-and-suspenders defensive coding.

### TUI app safety (vim, Claude Code interactive menus)

The concern is that replaying history might corrupt the live TUI's rendering state. It does not, for this reason: history is sent **before** the current pane snapshot and **before** live streaming begins. By the time the live TUI's output arrives, the xterm.js terminal has processed the history lines (which are safe, static, non-positioning content) and the current pane snapshot (`ESC[H` + sanitized content) has reset the viewport to its correct state.

The live TUI's own `ESC[2J`/`ESC[H` sequences on the next render cycle will clear and re-draw the viewport. History lines are now safely above the viewport in xterm.js's scrollback buffer — the live TUI cannot see or affect them.

### What can go wrong (and mitigations)

1. **tmux history-limit is too low**: If the tmux session was created with a low `history-limit` (default 2000), requesting 1000 lines returns only what tmux has. The response is simply shorter — no error, just less history. Document that users should add `set -g history-limit 50000` to `~/.tmux.conf`.

2. **Large session with dense output**: A session that ran `git log --stat --oneline` for a large repo could have 5000 lines of dense text. At the 1000-line default cap, only the last 1000 lines are sent. This is correct behavior.

3. **Race between history capture and live output**: Between step (2) and step (6), the tmux session may write more output. History (captured at step 2) and live output (arriving in step 6) may briefly overlap for the last few lines. xterm.js will display the overlap as duplicated lines in the scrollback, then the live output continues correctly. This is acceptable and consistent with how any terminal emulator handles reconnection. The overlap is at most a few lines and only at the history/live boundary.

4. **Paused sessions**: `CapturePaneContentWithOptions()` calls tmux, which is unavailable for paused sessions. The function will return an error. The handler must check for error and skip history delivery silently — a paused session showing the current pane snapshot (from scrollback file) is already degraded; absence of history is acceptable. Implement in Story 1 as an explicit fallback: `if err != nil { log skip history }`.

5. **External sessions**: External sessions (discovered via ssq-mux) use `streamViaTmuxCapturePane()` and call `instance.CapturePaneContent()` which delegates to the tmux manager. History capture for external sessions uses `CapturePaneContentWithOptions()` on the external tmux session name. This should work as long as the external session is a regular tmux session.

---

## Story 1: Configuration, Helper Method, and Fallback Logic

**Priority**: P0 (required by Stories 2 and 3)
**Files**: `server/services/connectrpc_websocket.go`, `session/instance.go`

### Task 1.1: Add scrollbackHistoryLines to ConnectRPCWebSocketHandler

**File**: `server/services/connectrpc_websocket.go`

Add `scrollbackHistoryLines int` field to `ConnectRPCWebSocketHandler`. Update `NewConnectRPCWebSocketHandler` to accept `historyLines int`. Add validation: clamp 0 → 1000 (default), negative → 0 (disabled), >5000 → 5000 (cap).

```go
// In ConnectRPCWebSocketHandler struct:
scrollbackHistoryLines int // Lines of tmux history to send on connect (0=disabled, default=1000)

// In NewConnectRPCWebSocketHandler:
func NewConnectRPCWebSocketHandler(
    sessionService *SessionService,
    scrollbackManager *scrollback.ScrollbackManager,
    tmuxStreamerManager *session.ExternalTmuxStreamerManager,
    streamingMode string,
    historyLines int,  // NEW parameter
) *ConnectRPCWebSocketHandler {
    if historyLines < 0 {
        historyLines = 0
    } else if historyLines == 0 {
        historyLines = 1000 // default
    } else if historyLines > 5000 {
        historyLines = 5000 // cap
    }
    // ...
    scrollbackHistoryLines: historyLines,
```

**Acceptance criteria**:
- GIVEN historyLines=0, WHEN handler is created, THEN scrollbackHistoryLines is 1000
- GIVEN historyLines=-1, WHEN handler is created, THEN scrollbackHistoryLines is 0
- GIVEN historyLines=9999, WHEN handler is created, THEN scrollbackHistoryLines is 5000
- GIVEN historyLines=500, WHEN handler is created, THEN scrollbackHistoryLines is 500

### Task 1.2: Add GetScrollbackHistory to session.Instance

**File**: `session/instance.go`

Add a method that calls `tmuxManager.CapturePaneContentWithOptions()` with history-oriented parameters. This centralizes the tmux call and makes the history capture testable independently.

```go
// GetScrollbackHistory returns the last N lines of tmux scrollback history.
// Returns empty string with nil error if the session is paused or history is unavailable.
// The returned string contains ANSI color codes but no cursor-positioning sequences
// (tmux strips those from history output).
func (i *Instance) GetScrollbackHistory(lines int) (string, error) {
    if lines <= 0 {
        return "", nil
    }
    startLine := fmt.Sprintf("-%d", lines)
    content, err := i.tmuxManager.CapturePaneContentWithOptions(startLine, "-1")
    if err != nil {
        return "", err
    }
    return content, nil
}
```

Note: `CapturePaneContentWithOptions()` with `-E -1` captures the line **before** the current visible pane (where `-1` is the last history line). This prevents the history output from duplicating the current pane snapshot.

**Acceptance criteria**:
- GIVEN a live session, WHEN GetScrollbackHistory(100) is called, THEN it returns up to 100 lines of history
- GIVEN a paused session (tmux session not present), WHEN GetScrollbackHistory(100) is called, THEN it returns an error
- GIVEN lines=0, WHEN GetScrollbackHistory(0) is called, THEN it returns ("", nil) immediately without a tmux call

### Task 1.3: Update handler constructor callers

**Files**: Wherever `NewConnectRPCWebSocketHandler` is called (likely `server/server.go` or similar wiring code)

Read `STAPLER_SQUAD_SCROLLBACK_LINES` environment variable and pass as `historyLines`. Default to 0 (triggers the 1000-line default in the constructor).

```go
scrollbackLines := 0 // triggers default of 1000
if val := os.Getenv("STAPLER_SQUAD_SCROLLBACK_LINES"); val != "" {
    if n, err := strconv.Atoi(val); err == nil {
        scrollbackLines = n
    }
}
handler := services.NewConnectRPCWebSocketHandler(
    sessionService, scrollbackManager, tmuxStreamerManager, streamingMode, scrollbackLines,
)
```

**Acceptance criteria**:
- GIVEN STAPLER_SQUAD_SCROLLBACK_LINES=500, WHEN server starts, THEN handler uses 500 history lines
- GIVEN STAPLER_SQUAD_SCROLLBACK_LINES unset, WHEN server starts, THEN handler uses 1000 history lines
- GIVEN STAPLER_SQUAD_SCROLLBACK_LINES=0, WHEN server starts, THEN handler delivers no history (disabled)

---

## Story 2: History Injection in streamViaControlMode

**Priority**: P1
**Depends on**: Story 1
**Files**: `server/services/connectrpc_websocket.go`

This is the primary streaming path for managed sessions. It handles the quiescence-based snapshot sequence.

### Task 2.1: Inject history capture between quiescence and snapshot

**File**: `server/services/connectrpc_websocket.go`, function `streamViaControlMode()`

Insert history capture and delivery after `streamer.UnsubscribeControlModeUpdates(quiescenceSubID)` and before `getOrRefreshSnapshot()`. The exact location is line ~508 in the current file.

```go
// After quiescence detection (line ~508):
streamer.UnsubscribeControlModeUpdates(quiescenceSubID)

// NEW: Capture and send scrollback history
// Must occur AFTER resize/quiescence (so tmux is at correct dimensions)
// and BEFORE current pane snapshot (so history appears above live content in xterm.js scrollback)
if h.scrollbackHistoryLines > 0 {
    historyContent, histErr := instance.GetScrollbackHistory(h.scrollbackHistoryLines)
    if histErr != nil {
        log.InfoLog.Printf("[streamViaControlMode] History capture failed for '%s' (session may be paused): %v",
            sessionID, histErr)
        // Proceed without history — not fatal
    } else if len(strings.TrimSpace(historyContent)) > 0 {
        // Append reset+CRLF separator before current pane snapshot
        historyBytes := []byte(historyContent + "\x1b[0m\r\n")
        historyData := &sessionv1.TerminalData{
            SessionId: sessionID,
            Data: &sessionv1.TerminalData_Output{
                Output: &sessionv1.TerminalOutput{Data: historyBytes},
            },
        }
        if histBytes, err := proto.Marshal(historyData); err == nil {
            envelope := protocol.CreateEnvelope(0, histBytes)
            if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
                log.ErrorLog.Printf("[streamViaControlMode] Failed to send history for '%s': %v", sessionID, err)
                // Non-fatal: current pane snapshot and live streaming still proceed
            } else {
                log.InfoLog.Printf("[streamViaControlMode] Sent %d bytes of history for session '%s'",
                    len(historyBytes), sessionID)
            }
        }
    } else {
        log.InfoLog.Printf("[streamViaControlMode] History empty for session '%s', skipping", sessionID)
    }
}

// EXISTING: Now capture and send current pane snapshot
initialContent, err := h.getOrRefreshSnapshot(sessionID, func() (string, error) {
    return instance.CapturePaneContentRaw()
})
```

**Acceptance criteria**:
- GIVEN a session with 500 lines of history, WHEN a client connects, THEN the client receives history bytes before the current pane snapshot
- GIVEN a session with no history (brand new session), WHEN a client connects, THEN no history message is sent and the current pane snapshot is sent normally
- GIVEN GetScrollbackHistory fails (paused session), WHEN a client connects, THEN the error is logged and the current pane snapshot is sent normally without crashing
- GIVEN scrollbackHistoryLines=0, WHEN a client connects, THEN no history capture is attempted

---

## Story 3: History Injection in streamViaTmuxCapturePane

**Priority**: P1 (parallel with Story 2)
**Depends on**: Story 1
**Files**: `server/services/connectrpc_websocket.go`

This is the fallback path for managed sessions when `STAPLER_SQUAD_USE_CONTROL_MODE=false` and for external sessions.

### Task 3.1: Inject history capture in the capture-pane polling path

**File**: `server/services/connectrpc_websocket.go`, function `streamViaTmuxCapturePane()`

Insert history capture after the resize/redraw wait and before the initial content send (line ~797 in current file).

```go
// After managed session resize wait (~line 795):
if instance.IsManaged {
    // ... existing resize code ...
    time.Sleep(200 * time.Millisecond)
}

// NEW: Capture and send history
// Same pattern as streamViaControlMode Story 2
if h.scrollbackHistoryLines > 0 {
    historyContent, histErr := instance.GetScrollbackHistory(h.scrollbackHistoryLines)
    if histErr != nil {
        log.InfoLog.Printf("[streamViaTmuxCapture] History capture failed for '%s': %v", sessionID, histErr)
    } else if len(strings.TrimSpace(historyContent)) > 0 {
        historyBytes := []byte(historyContent + "\x1b[0m\r\n")
        historyData := &sessionv1.TerminalData{
            SessionId: sessionID,
            Data: &sessionv1.TerminalData_Output{
                Output: &sessionv1.TerminalOutput{Data: historyBytes},
            },
        }
        if histBytes, err := proto.Marshal(historyData); err == nil {
            envelope := protocol.CreateEnvelope(0, histBytes)
            if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
                log.ErrorLog.Printf("[streamViaTmuxCapture] Failed to send history for '%s': %v", sessionID, err)
            } else {
                log.InfoLog.Printf("[streamViaTmuxCapture] Sent %d bytes of history for session '%s'",
                    len(historyBytes), sessionID)
            }
        }
    }
}

// EXISTING: Send initial content
var initialContent string
if instance.IsManaged {
    // ...
```

**Acceptance criteria**: Same as Story 2 Task 2.1 but for the capture-pane path.

---

## Story 4: On-Demand Scrollback Request Handler (Deferred)

**Priority**: P3 (independent, deferred)
**Depends on**: Story 1 (for `GetScrollbackHistory`)

The protobuf `ScrollbackRequest` / `ScrollbackResponse` messages are already defined. The client `requestScrollback()` hook in `useTerminalStream.ts` already sends `ScrollbackRequest`. The `.bak2` file shows a prior implementation. This story restores the server-side handler for clients that want to fetch additional history after initial connect (e.g., a "Load more history" button in the UI).

### Task 4.1: Add ScrollbackRequest handler in streamViaControlMode input loop

**File**: `server/services/connectrpc_websocket.go`

In the "Goroutine 2: Read from WebSocket" section of `streamViaControlMode()`, add a case for `ScrollbackRequest` alongside the existing input/resize handlers:

```go
// Handle scrollback request
if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil {
    limit := int(scrollbackReq.Limit)
    if limit <= 0 || limit > 5000 {
        limit = 1000
    }
    histContent, histErr := instance.GetScrollbackHistory(limit)
    if histErr != nil {
        log.WarningLog.Printf("[streamViaControlMode] On-demand scrollback failed for '%s': %v", sessionID, histErr)
        continue
    }
    lines := strings.Split(strings.TrimRight(histContent, "\n"), "\n")
    chunks := make([]*sessionv1.ScrollbackChunk, 0, len(lines))
    for i, line := range lines {
        chunks = append(chunks, &sessionv1.ScrollbackChunk{
            Data:        []byte(line + "\n"),
            Sequence:    scrollbackReq.FromSequence + uint64(i),
            TimestampMs: time.Now().UnixMilli(),
        })
    }
    resp := &sessionv1.TerminalData{
        SessionId: sessionID,
        Data: &sessionv1.TerminalData_ScrollbackResponse{
            ScrollbackResponse: &sessionv1.ScrollbackResponse{
                Chunks:         chunks,
                HasMore:        false,
                TotalLines:     uint64(len(chunks)),
                OldestSequence: scrollbackReq.FromSequence,
                NewestSequence: scrollbackReq.FromSequence + uint64(len(chunks)) - 1,
            },
        },
    }
    if respBytes, err := proto.Marshal(resp); err == nil {
        stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, respBytes)) //nolint:errcheck
    }
}
```

**Acceptance criteria**:
- GIVEN client sends ScrollbackRequest{limit: 200}, WHEN handler processes it, THEN a ScrollbackResponse is sent with up to 200 lines of tmux history
- GIVEN limit exceeds 5000, WHEN handler processes request, THEN limit is capped at 5000

---

## Known Issues

### Potential Bug: tmux -E -1 may include visible pane lines

**Severity**: Medium
**Description**: The `CapturePaneContentWithOptions` uses `-E -1` to stop before the current visible pane. The exact semantics of `-E -1` in tmux are "the line just before the visible pane". In practice this means history lines and current visible lines may slightly overlap for 1-2 lines at the boundary. The current pane snapshot then sends `ESC[H` + the full visible pane, so any duplicate lines appear in scrollback (above viewport) and are harmless to the live TUI.

**Mitigation**: This is a known tmux behavior and is acceptable. If desired, a future improvement could trim trailing blank lines from history before sending. Document the expected 1-2 line overlap in code comments.

**Files Likely Affected**:
- `session/instance.go` — `GetScrollbackHistory()`

### Potential Bug: Large history payload blocks connection completion

**Severity**: Medium
**Description**: For sessions with 5000 lines of dense output (e.g., verbose test run), the history payload can reach 400-600KB. `stream.WriteMessage()` is synchronous (blocked by the WebSocket write mutex). If the WebSocket connection is slow (e.g., remote access over a VPN), this write blocks the goroutine for several seconds before live streaming begins.

**Mitigation**: The 5000-line maximum cap limits payload to ~600KB. For a 1Mbps connection, this is <5 seconds. For typical localhost/LAN usage, <50ms. Acceptable for the initial implementation. A future improvement: stream history in chunks so live output can interleave. Document the behavior.

**Files Likely Affected**:
- `server/services/connectrpc_websocket.go` — `streamViaControlMode()`, `streamViaTmuxCapturePane()`

### Potential Bug: History from wrong dimensions

**Severity**: Low
**Description**: History is captured after the resize+quiescence wait, so it should reflect the correct width. However, history lines are **pre-rendered** by tmux at whatever width they were originally written. A session that ran at 120 columns will have 120-column wrapped history lines even if the current client is 80 columns wide. tmux's `-J` flag joins wrapped lines, so a 120-column line displayed in an 80-column xterm.js will wrap visually but be semantically correct.

**Mitigation**: This is inherent to terminal history — you cannot reflow lines that were already wrapped by the process. Acceptable. Document in code comments.

**Files Likely Affected**:
- `session/instance.go` — `GetScrollbackHistory()` — add doc comment

### Potential Bug: sanitizeInitialContent applied to history (developer error risk)

**Severity**: Medium
**Description**: A developer implementing Stories 2-3 might apply `sanitizeInitialContent()` to history output by analogy with the current pane snapshot path. This would be incorrect (tmux already stripped positioning codes from history) and would cause no visible problem — but it would be dead code hiding a misunderstanding.

**Mitigation**: Add a clear comment in `GetScrollbackHistory()` and at the call site:
```go
// NOTE: Do NOT apply sanitizeInitialContent() to history output.
// tmux strip cursor-positioning codes from history automatically.
// sanitizeInitialContent() is only needed for raw PTY captures (current pane).
```

**Prevention**: Code review checklist should verify this.

### Potential Bug: Concurrent connects to same session duplicate history

**Severity**: Low
**Description**: If two clients connect to the same session simultaneously, both receive the history bytes independently. This is correct behavior — each client has its own WebSocket connection and its own xterm.js instance. There is no shared state issue.

**Mitigation**: None needed. Documented as intentional.

### Potential Bug: History from external sessions with non-standard tmux names

**Severity**: Low
**Description**: External sessions (ssq-mux) use the external tmux session name from `ExternalMetadata.TmuxSessionName`. `GetScrollbackHistory()` on `instance.go` calls `tmuxManager.CapturePaneContentWithOptions()` which uses `t.sanitizedName` (the internal tmux name). For external sessions, `sanitizedName` may not match `ExternalMetadata.TmuxSessionName`.

**Mitigation**: Inspect how `CapturePaneContent()` vs `GetContent()` is used for external sessions in `streamViaTmuxCapturePane()`. External sessions use `streamer.GetContent()` (the external tmux streamer), not `instance.CapturePaneContent()`. For Story 3, external session history should be fetched via `CapturePaneContentWithOptions` on the external tmux session name, not through `instance.GetScrollbackHistory()`. Add a branch in Story 3:

```go
if instance.ExternalMetadata != nil && instance.ExternalMetadata.TmuxSessionName != "" {
    // Use external tmux session name for history capture
    historyContent, histErr = instance.GetScrollbackHistoryForTmux(
        instance.ExternalMetadata.TmuxSessionName, h.scrollbackHistoryLines)
} else {
    historyContent, histErr = instance.GetScrollbackHistory(h.scrollbackHistoryLines)
}
```

This may require adding `GetScrollbackHistoryForTmux(tmuxName string, lines int) (string, error)` to instance or exposing the tmux manager's `CapturePaneContentWithOptions` via an interface.

**Files Likely Affected**:
- `session/instance.go` — review which method routes to which tmux name

---

## Context Preparation Guide

### For Story 1 (Configuration + Helper)

Files to read before starting:
1. `/Users/tylerstapler/IdeaProjects/stapler-squad/server/services/connectrpc_websocket.go` — lines 88-117 (struct definition and constructor)
2. `/Users/tylerstapler/IdeaProjects/stapler-squad/session/tmux/tmux.go` — search for `CapturePaneContentWithOptions` (around line 1516-1530)
3. `/Users/tylerstapler/IdeaProjects/stapler-squad/session/instance.go` — search for existing delegation methods like `CapturePaneContentRaw` to understand the pattern
4. Wherever `NewConnectRPCWebSocketHandler` is called (grep the repo)

Files to check after implementation:
- `go build .` must succeed
- `go test ./server/services` must pass

### For Story 2 (streamViaControlMode history injection)

Files to read before starting:
1. `/Users/tylerstapler/IdeaProjects/stapler-squad/server/services/connectrpc_websocket.go` — `streamViaControlMode()` function (~lines 410-732), specifically lines 507-555 (the quiescence-to-snapshot transition)
2. ADR-001, ADR-002 for rationale
3. Story 1 implementation (the `GetScrollbackHistory` method added to `session/instance.go`)

The exact injection point is immediately after `streamer.UnsubscribeControlModeUpdates(quiescenceSubID)` and before the `getOrRefreshSnapshot` call.

### For Story 3 (streamViaTmuxCapturePane history injection)

Files to read before starting:
1. `/Users/tylerstapler/IdeaProjects/stapler-squad/server/services/connectrpc_websocket.go` — `streamViaTmuxCapturePane()` function (~lines 741-1222), specifically lines 795-840 (resize wait and initial content send)
2. Known Issues section: "external sessions with non-standard tmux names" — verify whether this affects the implementation

### For Story 4 (on-demand ScrollbackRequest handler)

Files to read before starting:
1. `/Users/tylerstapler/IdeaProjects/stapler-squad/server/services/connectrpc_websocket.go.bak2` — lines 496-568 (the old implementation to reference)
2. `/Users/tylerstapler/IdeaProjects/stapler-squad/proto/session/v1/events.proto` — ScrollbackRequest and ScrollbackResponse message definitions
3. `/Users/tylerstapler/IdeaProjects/stapler-squad/web-app/src/lib/hooks/useTerminalStream.ts` — `requestScrollback` function and `scrollbackResponse` handler to understand the client contract

---

## Testing Strategy

### Unit tests (Story 1)

- `TestNewConnectRPCWebSocketHandler_HistoryLinesClamping` in `connectrpc_websocket_test.go`
  - historyLines=0 → 1000
  - historyLines=-5 → 0 (disabled)
  - historyLines=9999 → 5000

### Integration tests (Stories 2-3)

The existing `connectrpc_websocket_test.go` mocks `instance` via the `SessionStreamer` interface. Extend with:
- `TestStreamViaControlMode_SendsHistoryBeforeSnapshot`
  - Mock `instance.GetScrollbackHistory()` to return "line1\nline2\nline3"
  - Connect a test WebSocket client
  - Assert first received `TerminalOutput` message contains "line1\nline2\nline3"
  - Assert second received `TerminalOutput` message contains the current pane snapshot
- `TestStreamViaControlMode_HandlesHistoryError`
  - Mock `GetScrollbackHistory()` to return error
  - Assert connection completes normally and current pane snapshot is sent

### Manual verification

1. Start a session, run a multi-screen command (`git log --oneline | head -200`)
2. Close the terminal tab in the web UI
3. Reopen the terminal tab
4. Verify you can scroll up and see the previous command output
5. Verify the live terminal state is correct (no garbled output, TUI renders correctly)
