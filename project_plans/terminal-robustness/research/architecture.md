# Terminal Robustness — Architecture Research

## 1. Scrollback Architecture

### Current Implementation

`ScrollbackManager` (`session/scrollback/manager.go`) maintains per-session `CircularBuffer`s (default 10,000 lines) backed by compressed JSONL files on disk (zstd by default). Key methods:

- `GetRecentLines(sessionID, n)` — returns last N entries from memory buffer or disk.
- `GetScrollback(sessionID, fromSeq, limit)` — returns entries starting at `fromSeq`, merging disk and memory. This is the natural on-demand fetch API.
- `GetRecentBytes(sessionID, bytes)` — currently hard-caps in-memory fetch at 500 lines (ADR-001 comment: "Phase 1 limit; Phase 2 will lift via server-side streaming approach").
- `CurrentSequence(sessionID)` — returns highest sequence number written, used for checkpoint tracking.

`CircularBuffer` (`buffer.go`) stores `ScrollbackEntry{Timestamp, Data []byte, Sequence uint64}` with monotonically increasing sequence numbers. `GetLastN(n)` and `GetRange(fromSeq, limit)` are O(n) reads under a read-lock.

`FileScrollbackStorage` (`storage.go`) uses line-delimited JSON appended to `~/.stapler-squad/sessions/<id>/scrollback.jsonl.zst`. `ReadTail` byte-seeks the compressed file directly — this is incorrect for zstd (compressed format is not seekable by byte offset), so `ReadTail` returns raw compressed bytes, not valid terminal data. **This is a latent bug affecting deep-history reads.**

### API Changes Needed for Scrollback-on-Demand

The scrollback-on-demand path exists in proto already (`ScrollbackRequest`/`ScrollbackResponse` in `events.proto`), but the server never handles `ScrollbackRequest` messages in the WebSocket input loop (`streamViaControlMode`). The handler at line ~750 only dispatches on `GetInput()`, `GetResize()`, not `GetScrollbackRequest()`.

**Required backend changes:**

1. Add a `case incomingData.GetScrollbackRequest() != nil:` branch in the WebSocket read goroutine (after resize handling, ~line 762 in `connectrpc_websocket.go`).
2. Implement a handler that calls `h.scrollbackManager.GetScrollback(sessionID, req.FromSequence, int(req.Limit))` and sends back a `ScrollbackResponse` with chunks populated from the entries' `.Data` fields.
3. Fix `GetRecentBytes` / `ReadTail` to actually decode the compressed file rather than seeking raw bytes.
4. Increase the in-memory cap from 500 to at least 5000 lines (or remove it entirely, relying on `GetScrollback`'s `limit` parameter).

**Proto changes needed:**

The existing `ScrollbackRequest` and `ScrollbackResponse` messages in `events.proto` are already sufficient:

```protobuf
message ScrollbackRequest {
  uint64 from_sequence = 1;
  int32 limit = 2;
}
message ScrollbackResponse {
  repeated ScrollbackChunk chunks = 1;
  bool has_more = 2;
  uint64 total_lines = 3;
  uint64 oldest_sequence = 4;
  uint64 newest_sequence = 5;
}
```

No new proto messages are needed for scrollback — only the server-side handler and the frontend consumer need to be wired up.

**Initial payload change:** The handshake (`CurrentPaneRequest`) sends only the pane snapshot (current visible screen). The server should also send a `ScrollbackResponse` immediately after the initial pane snapshot, containing the most recent 500 lines. This requires adding a second message write in `streamViaControlMode` after the initial `TerminalOutput` send (~line 558).

---

## 2. Resize Quiescence

### Current Strategy

In `streamViaControlMode` (`connectrpc_websocket.go`, lines ~480-518):

**Handshake resize (initial connection):**
1. Server starts a separate control-mode subscription for quiescence detection before any resize.
2. Issues a `±1 nudge` (resize to `cols-1` then back to `cols`) to guarantee SIGWINCH fires even when tmux is already at target size.
3. Calls `waitForQuiescence(quiescenceCh, 500ms timeout, 50ms quietFor)` — waits until no control-mode output arrives for 50 ms, or 500 ms total.
4. Then captures pane content and sends as initial snapshot.

**Streaming resize (post-connection, lines ~657-762):**
- Resize events from the client enter a `resizeCh` channel of capacity 1.
- A dedicated goroutine drains the channel and calls `instance.SetWindowSize(cols, rows)` — no quiescence wait.
- Rapid events are coalesced: new resize replaces old if the goroutine is busy.

**`SetWindowSize` in `session/tmux/tmux.go` (lines ~1372-1407):**
- Calls `updateWindowSize` (PTY ioctl) then `resize-window` via tmux control mode or subprocess.
- Stores `lastKnownCols`/`lastKnownRows` as atomics.
- **No quiescence wait** — returns immediately after issuing the resize command.

### What Needs to Change

Per R1.1/R1.2/R1.5:

1. **Post-resize quiescence in streaming path**: After `SetWindowSize` returns, the goroutine must wait for a new quiescence signal (100 ms of no output) before the server's next capture. The streaming resize goroutine needs access to the control-mode update channel. Currently the quiescence subscription is set up only for the handshake and then unsubscribed. The subscription must persist for the lifetime of the stream.

2. **Server-to-client "resizing" signal**: After applying the resize and before capturing, the server should send a new proto message to tell the client to show the resizing overlay (R1.4). After the post-resize capture is complete and sent, the client clears the overlay.

   **New proto field needed** — add a `ResizeQuiescence` message to `TerminalData.data` oneof:
   ```protobuf
   message ResizeQuiescence {
     // True = resize in progress (show overlay), False = stable (hide overlay)
     bool resizing = 1;
     int32 cols = 2;
     int32 rows = 3;
   }
   ```
   Add `resize_quiescence = 16;` to the `TerminalData` oneof in `events.proto`.

3. **Backend resize coalescing (R1.5)**: The current `resizeCh` channel with capacity 1 already coalesces at the channel level. Add a 50 ms timestamp guard in the resize goroutine: if the same (cols, rows) pair was applied within the last 50 ms, skip the ioctl call entirely.

4. **Frontend debounce fix (R1.2/R1.3)**: The `ResizeObserver` callback in `XtermTerminal.tsx` (lines ~311-332) uses an adaptive debounce: 10 ms for first 3 resizes, then 250 ms. Replace with a flat 150 ms debounce. Remove the secondary `setTimeout(..., 100)` fit at lines 239-244 — the double-RAF inside the ResizeObserver callback is already sufficient.

5. **localStorage cell dimension cache (R1.6)**: XtermTerminal reads `scrollback` from localStorage but does not cache raw cell dimensions. However, `loadTerminalConfig()` returns `scrollbackLines`, `fontSize`, and `fontFamily`. Add a validation step: if stored `fontSize` or `fontFamily` differs from current values, evict and recompute.

---

## 3. Proto Streaming Structure — Summary of New Messages Needed

### Existing messages that are sufficient (no change needed):
- `ScrollbackRequest` / `ScrollbackResponse` / `ScrollbackChunk` — already defined in `events.proto`.
- `TerminalResize` — already defined.
- `FlowControl` — already defined.

### New message needed:
```protobuf
// ResizeQuiescence signals terminal resize state to the client.
// Server sends resizing=true immediately when resize is applied,
// then resizing=false after quiescence is detected and new snapshot is sent.
message ResizeQuiescence {
  bool resizing = 1;   // true=overlay on, false=overlay off
  int32 cols = 2;
  int32 rows = 3;
}
```

Add to `TerminalData` oneof:
```protobuf
ResizeQuiescence resize_quiescence = 16;
```

The `TerminalData` oneof currently ends at field 15 (`ssp_negotiation`). Field 16 is free.

---

## 4. Frontend Terminal State Machine

The terminal component needs to distinguish these states to drive the UI correctly:

```
States:
  DISCONNECTED   — no WebSocket connection
  CONNECTING     — WebSocket opened, handshake message sent, awaiting response
  LOADING        — initial snapshot in transit (show spinner over blank terminal)
  STABLE         — streaming live output, no pending operations
  RESIZING       — resize sent to server; overlay dimmed; awaiting post-resize snapshot
  FETCHING_SCROLLBACK — client requested older history; xterm.js will prepend it

Transitions:
  DISCONNECTED   → CONNECTING        on connect()
  CONNECTING     → LOADING           on WebSocket open + handshake sent
  LOADING        → STABLE            on initial snapshot written to xterm.js
  STABLE         → RESIZING          on ResizeQuiescence(resizing=true) received
  RESIZING       → STABLE            on ResizeQuiescence(resizing=false) + new snapshot written
  STABLE         → FETCHING_SCROLLBACK  when user scrolls within 200 lines of top (R2.3)
  FETCHING_SCROLLBACK → STABLE       on ScrollbackResponse written to xterm.js
  Any            → DISCONNECTED      on WebSocket close or error
```

**Implementation notes:**
- `useTerminalStream` hook (`web-app/src/lib/hooks/useTerminalStream.ts`) should expose a `terminalState` value from this enum instead of the current boolean `isConnected`.
- The `RESIZING` overlay (R1.4) should be a semi-transparent div layered over the xterm.js canvas, driven by `terminalState === 'RESIZING'`.
- `handleScrollbackReceived` in `TerminalOutput.tsx` (lines ~290-307) currently **rejects** scrollback with metadata (historical requests). Remove this guard — all scrollback, whether initial or historical, should call `manager.writeInitialContent()` or a new `manager.prependScrollback()` method that writes before the current viewport without clearing it.
- Set `xterm.js scrollback` to 5000 lines (R2.1). This requires changing the default in `XtermTerminal.tsx` from `scrollbackProp ?? config?.scrollbackLines ?? 0` to `?? 5000`.
- The `scrollbackLines: 1000` parameter in `useTerminalStream` call (TerminalOutput.tsx line 338) controls the initial handshake request; increase to 500 per R2.2. Rename to `initialScrollbackLines: 500` for clarity.

---

## 5. Mobile Gesture State Machine

### Conflict Analysis

Both `useTouchScroll.ts` and `useMobileTerminalGestures.ts` register independent `touchstart` + `touchmove` listeners on the same container element. Both call `terminal.scrollLines()` from `touchmove` based on vertical delta. Both call `event.preventDefault()` when handling scroll. Since both handlers run on the same `touchmove` event, they can double-scroll (each calculating delta from different baseline `startY` values).

`useTouchScroll` is registered in `XtermTerminal.tsx` (line 115) while `useMobileTerminalGestures` is also registered (line 136). Both attach to `containerRef`.

`useTouchScroll` uses a cumulative delta (updates `touchStartY` to current position each move). `useMobileTerminalGestures` uses total displacement from `touchState.startY` to decide if scrolling vs selecting, then `lastY` delta for the scroll amount. These are mutually incompatible approaches.

### Unified Gesture State Machine

Replace both hooks with a single `useTerminalGestures` hook implementing:

```
States:
  IDLE             — finger not touching
  PENDING          — touchstart received; intent unknown; long-press timer started
  SCROLLING        — finger moved >8px vertically within first 400ms; timer cancelled
  SELECTING        — long-press timer fired (400ms without movement)
  TAPPING          — touchend within 400ms with <8px movement (tap gesture)

Transitions:
  IDLE        → PENDING     on touchstart (1 finger)
  PENDING     → SCROLLING   on touchmove with |dy| > 8px
  PENDING     → SELECTING   on long-press timer fires (400ms, no scroll)
  PENDING     → TAPPING     on touchend with |dy| < 8px within 400ms
  SCROLLING   → IDLE        on touchend
  SELECTING   → IDLE        on touchend (dispatch mouseup to xterm-screen)
  TAPPING     → IDLE        (dispatch tap action: focus or mouse-click escape)
  Any         → IDLE        on touchcancel or multi-touch (>1 finger)

Actions per state:
  SCROLLING:
    - Compute dy = currentY - lastY (update lastY each move)
    - cellH = terminal.options.fontSize * (terminal.options.lineHeight ?? 1.2)
    - lines = Math.round(-dy / cellH)
    - terminal.scrollLines(lines) if lines !== 0
    - e.preventDefault()

  SELECTING (when mouseTracking === 'none'):
    - On enter: dispatchEvent(mousedown) to .xterm-screen
    - On touchmove: terminal.select(startCol, startRow, length) via public API
    - On exit: dispatchEvent(mouseup) to .xterm-screen

  SELECTING (when mouseTracking !== 'none'):
    - Use terminal.select() API directly for rectangle selection

  TAPPING (when mouseTracking === 'none'):
    - terminal.focus() to show on-screen keyboard (R4.2)

  TAPPING (when mouseTracking is vt200 or higher):
    - Compute col = Math.floor((tapX - canvasLeft) / cellW)
    - Compute row = Math.floor((tapY - canvasTop) / cellH)
    - Send ANSI mouse click escape sequence to PTY (R4.1)
    - Use \x1b[M + button + col+32 + row+32 encoding for X10 mouse reporting
```

**Cell dimension calculation (R3.4/R4.4):**
Replace all uses of `terminal._core._renderService.dimensions.css.cell.height` with:
```ts
const fontSize = terminal.options.fontSize ?? 14;
const lineHeight = (terminal.options as any).lineHeight ?? 1.0;
const cellH = fontSize * lineHeight;
const cellW = (terminal as any).dimensions?.css?.cell?.width ?? fontSize * 0.6;
```
If `terminal.dimensions` (public API, available in xterm.js >= 5.3) is present, prefer it.

**Copy button (R3.1/R3.2):**
Add `onSelectionChange` listener in `XtermTerminal.tsx` that sets a React state `selectionActive: boolean`. When true, render a floating `<button>Copy</button>` positioned via `getBoundingClientRect()` of the xterm canvas. On press: `navigator.clipboard.writeText(terminal.getSelection())` + show brief "Copied" toast.

---

## Key Findings Summary

1. **Scrollback server-side handler is completely unwired**: `ScrollbackRequest`/`ScrollbackResponse` proto messages exist but the WebSocket input goroutine never dispatches on them. The fix is a single `case` branch + a call to `scrollbackManager.GetScrollback()`. The `xterm.js scrollback=0` setting and `handleScrollbackReceived` metadata guard together completely block all scrollback from working.

2. **Resize quiescence is only applied at handshake, not during streaming**: The streaming resize path (`resizeCh` goroutine) calls `SetWindowSize` and returns immediately with no quiescence wait. Post-resize captures that race against tmux reflow are the root cause of corruption. The fix requires keeping the control-mode quiescence subscription alive for the stream lifetime and waiting 100 ms of silence before each post-resize capture. One new proto variant (`ResizeQuiescence`) signals the client to show/hide the resizing overlay.

3. **Both touch scroll hooks conflict and must be unified**: `useTouchScroll` and `useMobileTerminalGestures` both handle `touchmove` on the same element with incompatible delta-tracking strategies, causing double-scroll. A single `useTerminalGestures` hook with a five-state machine (IDLE/PENDING/SCROLLING/SELECTING/TAPPING) eliminates the conflict, fixes the private-API cell-height lookup, enables tap-to-cursor via ANSI escape sequences, and enables selection in both `none` and `vt200` mouse-tracking modes.
