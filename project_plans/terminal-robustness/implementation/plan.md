# Terminal Robustness — Implementation Plan

**Requirements source**: `project_plans/terminal-robustness/requirements.md`
**Research sources**: `project_plans/terminal-robustness/research/`
**Target branch**: `stapler-squad-terminal-rbust`

---

## Agent Assignment Model

Each Epic maps to one implementation agent. Epics 1–3 have no inter-epic dependencies and can be worked in parallel. Epic 4 (Frontend Overlay + Integration) depends on Epics 1 and 2 (consumes the new proto variant and the scrollback write-lock). Epic 3 (Mobile) is fully independent of Epics 1, 2, and 4.

```
Epic 1 (Resize Quiescence) ──────────────────────────────────┐
Epic 2 (Scrollback E2E) ─────────────────────────────────────┤─▶ Epic 4 (Frontend Integration)
Epic 3 (Mobile Gestures) ──── standalone, no downstream deps ┘
```

---

## Epic 1: Resize Quiescence

**Covers**: R1.1, R1.2, R1.3, R1.5, R1.6
**No inter-epic dependencies.**
**Agent runs**: Go backend + TypeScript frontend changes for resize path only.

### Story 1.1: Backend resize coalescing and quiescence wait

#### Task 1.1.1 — Add 50 ms duplicate-resize guard in resize goroutine

- **File**: `server/services/connectrpc_websocket.go`
- **Lines**: ~657–673 (the `go func()` that drains `resizeCh`)
- **Change**: Inside the `case r := <-resizeCh:` block, before calling `instance.SetWindowSize`, add a `lastAppliedResize` struct tracking `{cols, rows int; t time.Time}`. If `r.cols == last.cols && r.rows == last.rows && time.Since(last.t) < 50*time.Millisecond`, skip the `SetWindowSize` call and log at Debug level. Update `lastAppliedResize` on every non-skipped call.
- **Why**: R1.5 — prevents redundant PTY ioctls when rapid window-drag events produce the same dimensions multiple times within 50 ms; avoids triggering unnecessary tmux reflows.
- **Test**: Go unit test `TestResizeCoalescing` in `server/services/connectrpc_websocket_test.go` — send two identical resize messages within 10 ms; assert `SetWindowSize` is called exactly once. Send two different sizes within 10 ms; assert called twice.

#### Task 1.1.2 — Persist quiescence subscription for streaming lifetime

- **File**: `server/services/connectrpc_websocket.go`
- **Lines**: ~518 (`streamer.UnsubscribeControlModeUpdates(quiescenceSubID)`) and ~570–579 (new subscription setup)
- **Change**: Remove the `streamer.UnsubscribeControlModeUpdates(quiescenceSubID)` call at line 518. Repurpose `quiescenceCh` (already a `<-chan []byte` from the handshake subscription) so it remains live for the stream duration. Pass `quiescenceCh` into the resize goroutine (Task 1.1.1 scope) as a new parameter. After `SetWindowSize` succeeds, call `waitForQuiescence(quiescenceCh, 300*time.Millisecond, 100*time.Millisecond)` before returning from the resize handler block.
- **Why**: R1.1 — the resize goroutine currently returns immediately after `SetWindowSize`; tmux reflow can take 100–400 ms; without quiescence, the next capture-pane sees partially reflowed content producing line corruption.
- **Test**: Integration test `TestResizeQuiescence` — resize a session with >1000 lines of scrollback; assert that the post-resize snapshot byte count matches the expected column width (no old-width lines present).

#### Task 1.1.3 — Send post-resize snapshot after quiescence

- **File**: `server/services/connectrpc_websocket.go`
- **Lines**: ~667–673 (resize goroutine, after `waitForQuiescence`)
- **Change**: After `waitForQuiescence` returns, call `instance.CapturePaneContentRaw()` and send the result as a `TerminalData_Output` message on the stream using the same `sendData` helper already defined in the output goroutine. Extract `sendData` to a closure accessible from both goroutines (or use a channel to request a send from the output goroutine).
- **Why**: Without sending a post-resize snapshot the client display remains stale at the old layout until the next natural PTY output event; this is the primary visible symptom of resize corruption.
- **Test**: Manual smoke test — resize browser window; verify terminal redraws cleanly within 800 ms (R5.2). Automated: assert that a `TerminalData_Output` message is received by the test client within 400 ms of sending a resize message when the session is idle.

#### Task 1.1.4 — Add `ResizeQuiescence` proto variant

- **File**: `proto/session/v1/events.proto`
- **Lines**: The `TerminalData` oneof block (currently ends at field 15 `ssp_negotiation`)
- **Change**: Add new message definition:
  ```protobuf
  message ResizeQuiescence {
    bool resizing = 1;
    int32 cols = 2;
    int32 rows = 3;
  }
  ```
  Add to `TerminalData` oneof: `ResizeQuiescence resize_quiescence = 16;`
  Run `make generate-proto` after the change.
- **Why**: Clients need a signal to show/hide the resizing overlay (R1.4); a dedicated proto variant is cleaner than overloading existing fields and enables typed dispatch in the frontend.
- **Test**: `make build` succeeds (proto compilation). Proto round-trip test: marshal a `TerminalData` with `ResizeQuiescence{resizing: true, cols: 220, rows: 50}` and unmarshal; assert all fields equal.

#### Task 1.1.5 — Emit ResizeQuiescence(true) before wait, ResizeQuiescence(false) after snapshot

- **File**: `server/services/connectrpc_websocket.go`
- **Lines**: resize goroutine body (Task 1.1.1–1.1.3 scope)
- **Change**: Before calling `waitForQuiescence`, marshal and send a `TerminalData` with `ResizeQuiescence{resizing: true, cols: r.cols, rows: r.rows}`. After sending the post-resize snapshot (Task 1.1.3), send `ResizeQuiescence{resizing: false, cols: r.cols, rows: r.rows}`.
- **Why**: R1.4 — enables the frontend to show a non-blocking overlay during the quiescence wait and clear it once the stable snapshot arrives.
- **Test**: Client-side integration test — assert the message sequence for a resize is: `ResizeQuiescence(true)` → `TerminalData_Output` (snapshot) → `ResizeQuiescence(false)`.

### Story 1.2: Frontend resize debounce fix

#### Task 1.2.1 — Remove secondary 100 ms delayed fit()

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: 238–244
- **Change**: Delete the `setTimeout(() => { ... fitAddon.fit() ... }, 100)` block entirely. The double-rAF block already on lines 200–246 provides sufficient layout stability; the extra 100 ms setTimeout causes a second resize event with no benefit.
- **Why**: R1.3 — the secondary fit fires `terminal.onResize` a second time, triggering a second server resize RPC and a second capture-pane cycle; this is the root cause of double-resize corruption on mount.
- **Test**: Jest test `XtermTerminal.test.tsx` — mount the component; assert `fitAddon.fit` is called exactly once during initialization (spy on `FitAddon.prototype.fit`).

#### Task 1.2.2 — Replace adaptive debounce with flat 150 ms

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: 288–313 (ResizeObserver callback, `resizeCount` and `debounceDelay` variables)
- **Change**: Delete the `resizeCount` variable and its increment. Delete `const debounceDelay = resizeCount <= 3 ? 10 : 250;`. Replace with `const debounceDelay = 150;`. Remove the now-unused `resizeCount` variable.
- **Why**: R1.2 — the 10 ms fast-path debounce fires before a single animation frame (~16 ms), causing the ResizeObserver to trigger fit and server resize before tmux can process the previous SIGWINCH; 150 ms ensures tmux has stabilized.
- **Test**: Jest test — simulate 5 rapid ResizeObserver callbacks within 100 ms; assert `fitAddon.fit` is called exactly once (the debounced call), not 5 times.

#### Task 1.2.3 — Validate localStorage cell dimensions cache against current font

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx` (or wherever `loadTerminalConfig` reads localStorage)
- **Lines**: Look up `loadTerminalConfig()` usage, approximately lines 55–95 and the cache read path
- **Change**: When reading cached cell dimensions from localStorage, compare stored `fontSize` and `fontFamily` keys against the current component props. If either differs, delete the cached `cellWidth`/`cellHeight` entries and proceed without pre-sizing. Add a comment: "Stale cache from different font config causes wrong initial fit (R1.6)."
- **Why**: R1.6 — stale cell dimension cache from a previous font configuration produces an incorrect initial `fit()` measurement, causing the first resize to report wrong cols/rows, which sends a malformed resize to the server.
- **Test**: Jest test — store a cache entry with `fontSize: 12` then render the component with `fontSize: 14`; assert the cached cell dimensions are not applied (spy on `fitAddon.proposeDimensions`).

---

## Epic 2: Scrollback End-to-End

**Covers**: R2.1, R2.2, R2.3, R2.4, R2.5, R2.6, R2.7
**No inter-epic dependencies.** (Epic 4 reads the write-lock added here, but can be developed against a stub.)
**Agent runs**: Go backend + TypeScript frontend.

### Story 2.1: Wire ScrollbackRequest to WebSocket handler

#### Task 2.1.1 — Add ScrollbackRequest dispatch case in input goroutine

- **File**: `server/services/connectrpc_websocket.go`
- **Lines**: ~762–763 (after the `if resize := incomingData.GetResize()` block, before the closing comment)
- **Change**: Add:
  ```go
  if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil {
      entries, err := h.scrollbackManager.GetScrollback(sessionID, scrollbackReq.FromSequence, int(scrollbackReq.Limit))
      if err != nil {
          log.WarningLog.Printf("[streamViaControlMode] ScrollbackRequest failed for '%s': %v", sessionID, err)
          continue
      }
      chunks := make([]*sessionv1.ScrollbackChunk, 0, len(entries))
      for _, e := range entries {
          chunks = append(chunks, &sessionv1.ScrollbackChunk{Data: e.Data})
      }
      hasMore := scrollbackReq.Limit > 0 && len(entries) == int(scrollbackReq.Limit)
      resp := &sessionv1.TerminalData{
          SessionId: sessionID,
          Data: &sessionv1.TerminalData_ScrollbackResponse{
              ScrollbackResponse: &sessionv1.ScrollbackResponse{
                  Chunks:  chunks,
                  HasMore: hasMore,
              },
          },
      }
      if respBytes, merr := proto.Marshal(resp); merr == nil {
          _ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, respBytes))
      }
  }
  ```
- **Why**: R2.4 — `ScrollbackRequest`/`ScrollbackResponse` messages are defined in the proto but the server never handles `GetScrollbackRequest()` in the input loop; this is the single missing wire-up that blocks all scrollback-on-demand.
- **Test**: Go integration test `TestScrollbackOnDemand` — send a `ScrollbackRequest{FromSequence: 0, Limit: 100}` over WebSocket; assert a `ScrollbackResponse` with ≥1 chunk is received.

#### Task 2.1.2 — Send initial ScrollbackResponse after handshake snapshot

- **File**: `server/services/connectrpc_websocket.go`
- **Lines**: ~562–568 (after the initial `TerminalData_Output` is sent, before the streaming subscription starts)
- **Change**: After the initial snapshot is sent (line ~558), call `h.scrollbackManager.GetScrollback(sessionID, 0, initialScrollbackLines)` where `initialScrollbackLines` comes from `currentPaneReq.ScrollbackLines` (or defaults to 500). Marshal and send as a `TerminalData_ScrollbackResponse` message. The client receives this immediately after the initial screen snapshot and writes it to the pre-open scrollback buffer.
- **Why**: R2.2 — the initial connection currently sends only the current pane snapshot (50–1000 lines); the server's 10,000-line circular buffer is never consulted; users see no scrollback history on connect.
- **Test**: Integration test — connect to a session with >500 lines of history; assert the client receives a `ScrollbackResponse` message within 300 ms of connecting (R5.3).

#### Task 2.1.3 — Increase handshake `scrollbackLines` default from 50 to 500

- **File**: `server/services/connectrpc_websocket.go` (handshake parsing, ~line 450–490) and `web-app/src/lib/hooks/useTerminalStream.ts` (the `scrollbackLines` parameter in the stream handshake message)
- **Lines**: Handshake construction site; `TerminalOutput.tsx` line 338
- **Change**: In `useTerminalStream` call at `TerminalOutput.tsx` line 338: change `scrollbackLines: 1000` to `scrollbackLines: 500`. In the Go handler, update the default value guard for `currentPaneReq.ScrollbackLines` from 50 to 500.
- **Why**: R2.6 — the existing `lines: 50` (or `1000` in the TypeScript call) parameter is inconsistent; R2.2 specifies 500 as the standard initial payload.
- **Test**: Assert the handshake proto message sent by the frontend contains `scrollback_lines: 500`.

### Story 2.2: xterm.js scrollback buffer and client-side wiring

#### Task 2.2.1 — Set xterm.js scrollback to 5000

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: ~154–164 (Terminal constructor options)
- **Change**: Change the `scrollback` option. Currently `scrollback` is passed as a prop (default `0` based on the prop definition). Change the prop default or the Terminal constructor to use `scrollback ?? 5000`. Specifically: in the `Terminal({..., scrollback, ...})` call, replace `scrollback` with `scrollback ?? 5000`. Ensure the prop type allows undefined with a comment: "Default 5000: xterm.js default of 1000 is insufficient for session history (R2.1)."
- **Why**: R2.1 — xterm.js `scrollback: 0` completely disables the scrollback buffer; any lines pushed off the viewport are immediately discarded; this is the root cause of the "current content repeated" artifact when scrolling up.
- **Test**: Jest test — render `XtermTerminal` with no `scrollback` prop; assert `Terminal` is constructed with `scrollback: 5000` (spy on `Terminal` constructor).

#### Task 2.2.2 — Remove handleScrollbackReceived metadata guard

- **File**: `web-app/src/components/sessions/TerminalOutput.tsx`
- **Lines**: 290–297 (the `if (metadata) { ... return; }` block)
- **Change**: Delete the entire `if (metadata)` block including the log line and `return`. Allow all scrollback — both initial and historical — to proceed to `manager.writeInitialContent(scrollback)`.
- **Why**: R2.7 — the metadata guard unconditionally discards all historical scrollback responses with the comment "auto-load disabled"; this prevents the feature from working at all once it is wired up server-side.
- **Test**: Jest test — call `handleScrollbackReceived` with a non-empty scrollback string and a `metadata` object; assert `manager.writeInitialContent` is called (not early-returned).

#### Task 2.2.3 — Add write-lock flag to TerminalStreamManager

- **File**: `web-app/src/lib/terminal/TerminalStreamManager.ts`
- **Lines**: ~91–137 (class fields and constructor)
- **Change**: Add two fields:
  ```ts
  private isWritingInitialContent: boolean = false;
  private pendingLiveWrites: string[] = [];
  ```
  In `write()` (line ~212), add at the top of the method:
  ```ts
  if (this.isWritingInitialContent) {
    this.pendingLiveWrites.push(output);
    return;
  }
  ```
  In `writeInitialContent()` (line ~233), wrap:
  ```ts
  async writeInitialContent(content: string): Promise<void> {
    this.isWritingInitialContent = true;
    try {
      this.terminal.clear();
      await this.enqueueWrite(content);
      this.terminal.scrollToBottom();
      setTimeout(() => this.terminal.scrollToBottom(), 10);
      setTimeout(() => this.terminal.scrollToBottom(), 100);
      setTimeout(() => this.terminal.scrollToBottom(), 500);
    } finally {
      this.isWritingInitialContent = false;
      const pending = this.pendingLiveWrites.splice(0);
      for (const chunk of pending) {
        this.write(chunk);
      }
    }
  }
  ```
- **Why**: Pitfall #1 and #8 — without the write-lock, live `output` messages that arrive while `writeInitialContent` is still processing its async `enqueueWrite` are written to xterm.js before the queued scrollback chunks, producing out-of-order display (history appearing below live output).
- **Test**: Jest test `TerminalStreamManager.test.ts` — call `writeInitialContent('history')` and, before it resolves, call `write('live')` three times; assert xterm.js `write()` is called with `'history'` before any `'live'` chunk.

#### Task 2.2.4 — Add cleanup for debug monitor references

- **File**: `web-app/src/lib/terminal/TerminalStreamManager.ts`
- **Lines**: `cleanup()` method
- **Change**: In the `cleanup()` method, add:
  ```ts
  this.originalWrite = null;
  this.originalRefresh = null;
  ```
- **Why**: Pitfall #7 — when the terminal is disposed while the manager is still alive, `originalWrite` and `originalRefresh` hold references to the closed terminal object, preventing GC. This is a memory leak in long-lived SPA sessions.
- **Test**: Jest test — create a manager, install debug monitor, call `cleanup()`, assert `manager['originalWrite']` is null.

### Story 2.3: Scroll-near-top triggers server-side history fetch

#### Task 2.3.1 — DOM scroll listener to detect near-top-of-buffer

- **File**: `web-app/src/components/sessions/TerminalOutput.tsx` (or a new `useScrollbackPaging.ts` hook)
- **Lines**: New hook or new effect block in `TerminalOutput.tsx`
- **Change**: After the terminal is open and the initial scrollback is loaded, attach a DOM `scroll` event listener to `terminal.element` (the xterm.js container element, `terminal.element` is public). In the handler:
  ```ts
  const viewportY = terminal.buffer.active.viewportY;
  if (viewportY < 200 && !isFetchingScrollback && hasMoreScrollback) {
    setIsFetchingScrollback(true);
    requestScrollback(oldestSequenceReceived, 500); // sends ScrollbackRequest over WebSocket
  }
  ```
  Do NOT use `terminal.onScroll` — per research (xterm.js issues #3201, #3864) it only fires on buffer writes, not user scroll gestures. Use the DOM `scroll` event on `terminal.element`.
- **Why**: R2.3 — when the user scrolls near the top of the local xterm.js buffer, older server-side history must be fetched; without this trigger there is no "load more" mechanism.
- **Test**: Jest/RTL test — render `TerminalOutput` with a mocked terminal that has `viewportY = 100`; fire a DOM scroll event on the terminal element; assert `requestScrollback` is called with the correct `fromSequence`.

#### Task 2.3.2 — Handle ScrollbackResponse for paged (non-initial) history

- **File**: `web-app/src/components/sessions/TerminalOutput.tsx`
- **Lines**: `handleScrollbackReceived` callback (~line 289)
- **Change**: Extend `handleScrollbackReceived` to distinguish initial vs. paged responses. The server's `ScrollbackResponse` includes `has_more` and `oldest_sequence`. When this is a paged response (not the initial load):
  1. Update `hasMoreScrollback` state from `response.hasMore`.
  2. Update `oldestSequenceReceived` state from `response.oldestSequence`.
  3. Call `manager.prependScrollbackBatch(content)` — a new method on `TerminalStreamManager` (see Task 2.3.3).
  4. Set `isFetchingScrollback = false`.

  Use a ref or state variable `isInitialScrollbackDone` to distinguish the two paths: before the first scrollback is written it is the initial load; subsequent calls are paged.
- **Why**: R2.3 / R2.5 — paged history must be written differently from initial history; initial history clears the buffer first; paged history must preserve the existing buffer and prepend (or use the serialize-clear-rewrite pattern).
- **Test**: Jest test — call `handleScrollbackReceived` twice: first with no prior scrollback (initial), then again (paged); assert `writeInitialContent` is called for the first, `prependScrollbackBatch` for the second.

#### Task 2.3.3 — Add prependScrollbackBatch to TerminalStreamManager

- **File**: `web-app/src/lib/terminal/TerminalStreamManager.ts`
- **Lines**: After `writeInitialContent` method (~line 243)
- **Change**: Add method:
  ```ts
  async prependScrollbackBatch(content: string): Promise<void> {
    // Serialize current buffer state
    // (requires SerializeAddon to be injected or accessible)
    // Pattern: serialize → clear → write history → write saved → scroll to bottom
    // If SerializeAddon is not available, fall back to reconnect-with-larger-window approach.
    this.isWritingInitialContent = true;
    try {
      // Option: use terminal.write() to prepend by clearing and rewriting.
      // This loses selection state but preserves visual content.
      const serialized = this.serializeAddon?.serialize() ?? '';
      this.terminal.clear();
      await this.enqueueWrite(content);
      if (serialized) {
        await this.enqueueWrite(serialized);
      }
      this.terminal.scrollToBottom();
    } finally {
      this.isWritingInitialContent = false;
      const pending = this.pendingLiveWrites.splice(0);
      for (const chunk of pending) {
        this.write(chunk);
      }
    }
  }
  ```
  Add a `serializeAddon?: ISerializeAddon` field and a setter `setSerializeAddon(addon: ISerializeAddon)`. In `XtermTerminal.tsx`, after loading `@xterm/addon-serialize`, call `manager.setSerializeAddon(serializeAddon)`.
- **Why**: R2.5 — xterm.js has no prepend-buffer API; the serialize-clear-rewrite pattern is the only viable approach for inserting older history above existing content (per research/stack.md §3).
- **Test**: Jest test — call `prependScrollbackBatch('old-history')` on a manager whose terminal already has content; assert terminal `write()` is called with `'old-history'` before the serialized current content.

---

## Epic 3: Unified Mobile Gesture Recognizer

**Covers**: R3.1, R3.2, R3.3, R3.4, R3.5, R4.1, R4.2, R4.3, R4.4
**No inter-epic dependencies.**
**Agent runs**: TypeScript frontend only.

### Story 3.1: Merge touch hooks into unified gesture state machine

#### Task 3.1.1 — Create useTerminalGestures.ts

- **File**: `web-app/src/lib/hooks/useTerminalGestures.ts` (new file)
- **Lines**: New file, ~250 lines
- **Change**: Create a new hook implementing the five-state gesture machine (IDLE → PENDING → SCROLLING | SELECTING | TAPPING → IDLE). The hook:
  - Accepts `{ containerRef, terminal, onSendData }` (terminal is the xterm.js `Terminal` instance; `onSendData` sends bytes to PTY).
  - Registers a single `touchstart` listener on `containerRef` with `{ passive: false }`.
  - On `touchstart` (1 finger): transition IDLE → PENDING; record `startX, startY, startTime`; start 400 ms long-press timer; record `startCol, startRow` from pixel-to-cell conversion.
  - On `touchstart` (>1 finger): cancel any in-progress gesture (transition to IDLE; cancel long-press timer).
  - On `touchmove`: if state is PENDING and `|dy| > 8px` → SCROLLING (cancel timer). If SCROLLING → scroll terminal. If SELECTING → extend selection. Always `e.preventDefault()` when handling.
  - On `touchend`: if PENDING and `|dy| < 8px` and `elapsed < 400ms` → TAPPING. If SCROLLING → IDLE. If SELECTING → IDLE (dispatch mouseup). If TAPPING: dispatch tap action.
  - On `touchcancel` / multi-touch: → IDLE unconditionally.
  - Returns cleanup disposer.
- **Why**: R4.3 — the two existing hooks (`useTouchScroll` and `useMobileTerminalGestures`) both handle `touchmove` on the same element with incompatible delta-tracking strategies, causing double-scroll and preventing selection during scroll.
- **Test**: Jest test `useTerminalGestures.test.ts` — simulate `touchstart` + short-move `touchmove` + `touchend`; assert state transitions IDLE→PENDING→SCROLLING→IDLE. Simulate `touchstart` + 400 ms wait + `touchend`; assert IDLE→PENDING→SELECTING→IDLE.

#### Task 3.1.2 — Replace private API cell height with public calculation

- **File**: `web-app/src/lib/hooks/useTerminalGestures.ts` (new hook from Task 3.1.1)
- **Lines**: Cell dimension helper function inside the hook
- **Change**: Define:
  ```ts
  function getCellDimensions(terminal: Terminal): { cellH: number; cellW: number } {
    const el = terminal.element;
    if (el && terminal.rows > 0 && terminal.cols > 0) {
      return {
        cellH: el.clientHeight / terminal.rows,
        cellW: el.clientWidth / terminal.cols,
      };
    }
    // Fallback: use font metrics (less accurate but never undefined)
    const fontSize = terminal.options.fontSize ?? 14;
    const lineHeight = (terminal.options.lineHeight as number | undefined) ?? 1.0;
    return {
      cellH: fontSize * lineHeight,
      cellW: fontSize * 0.6,
    };
  }
  ```
  Use this helper everywhere in the hook instead of any `_core._renderService` access.
- **Why**: R3.4 / R4.4 — `terminal._core._renderService.dimensions.css.cell.height` is a private API that may be `undefined` until after the first render frame in xterm.js 6.x; using `element.clientHeight / rows` is public, always available post-`open()`, and accurate.
- **Test**: Jest test — with a mocked terminal where `element.clientHeight = 480` and `rows = 20`; assert `getCellDimensions` returns `{ cellH: 24, cellW: ... }`.

#### Task 3.1.3 — Implement mouse-tracking-aware mouse tracking check

- **File**: `web-app/src/lib/hooks/useTerminalGestures.ts`
- **Lines**: Any reference to mouse tracking mode in the hook
- **Change**: Use `terminal.modes.mouseTrackingMode` (public API, stable in xterm.js 6.0.0) instead of any prop ref or `getMouseTracking()` callback. Define a helper:
  ```ts
  const isMouseTracking = () => terminal.modes.mouseTrackingMode !== 'none';
  ```
  Call this at gesture action time (not at registration time) so it reflects the runtime PTY-driven mode.
- **Why**: R3.3 / R4.1 — the existing `getMouseTracking()` reads from a prop/config that reflects the configured mode, not the actual PTY-controlled runtime mode; Claude Code sets `vt200` via escape sequences at runtime regardless of the prop value.
- **Test**: Jest test — mock `terminal.modes.mouseTrackingMode = 'vt200'`; assert `isMouseTracking()` returns `true`. Mock `terminal.modes.mouseTrackingMode = 'none'`; assert returns `false`.

#### Task 3.1.4 — Implement SCROLLING action with correct delta tracking

- **File**: `web-app/src/lib/hooks/useTerminalGestures.ts`
- **Lines**: `touchmove` handler, SCROLLING state branch
- **Change**: Track `lastY` (updated each `touchmove`). In SCROLLING:
  ```ts
  const dy = touch.clientY - lastY;
  lastY = touch.clientY;
  const { cellH } = getCellDimensions(terminal);
  const lines = Math.round(-dy / cellH);
  if (lines !== 0) terminal.scrollLines(lines);
  e.preventDefault();
  ```
  Register `touchmove` with `{ passive: false }` to allow `preventDefault()`.
- **Why**: The current `useTouchScroll` uses cumulative delta from `startY` (causing overshooting) while `useMobileTerminalGestures` uses per-event delta — only one approach should exist; per-event delta with public cell height is correct.
- **Test**: Jest test — simulate three `touchmove` events with dy=24px each, with `cellH = 24`; assert `terminal.scrollLines(-1)` is called three times (not three different values from accumulated delta).

#### Task 3.1.5 — Implement SELECTING action (both tracking modes)

- **File**: `web-app/src/lib/hooks/useTerminalGestures.ts`
- **Lines**: Long-press timer handler and `touchmove` SELECTING branch
- **Change**: On long-press timer fire (state transition PENDING → SELECTING):
  - Always: call `hapticFeedback()` if available (`navigator.vibrate?.(10)`).
  - If `!isMouseTracking()`: dispatch synthetic `mousedown` to `.xterm-screen` at `(startX, startY)`.
  - If `isMouseTracking()`: record `startCol, startRow` from `getCellDimensions`; call `terminal.select(startCol, startRow, 1)` to begin selection via public API.
  
  On `touchmove` in SELECTING state:
  - If `!isMouseTracking()`: dispatch synthetic `mousemove` to `.xterm-screen`.
  - If `isMouseTracking()`: compute `currentCol, currentRow`; call `terminal.select(startCol, startRow, (currentRow - startRow) * terminal.cols + (currentCol - startCol + 1))`.
  
  On `touchend` from SELECTING: dispatch `mouseup` if not-tracking; call `terminal.getSelection()` if tracking.
- **Why**: R3.3 / R3.5 — existing code returns early when `mouseTracking !== 'none'`, leaving selection completely broken during Claude Code sessions; `terminal.select()` bypasses mouse tracking mode and works in all modes.
- **Test**: Jest test — simulate long-press with `terminal.modes.mouseTrackingMode = 'vt200'`; assert `terminal.select` is called and `dispatchEvent(mousedown)` is NOT called.

#### Task 3.1.6 — Implement TAPPING action (focus + mouse escape sequence)

- **File**: `web-app/src/lib/hooks/useTerminalGestures.ts`
- **Lines**: `touchend` TAPPING dispatch handler
- **Change**: On TAPPING:
  - If `!isMouseTracking()`: call `terminal.focus()` — shows on-screen keyboard (R4.2).
  - If `isMouseTracking()` (vt200 or higher):
    ```ts
    const { cellH, cellW } = getCellDimensions(terminal);
    const canvasRect = terminal.element!.getBoundingClientRect();
    const col = Math.floor((tapX - canvasRect.left) / cellW) + 1; // 1-based
    const row = Math.floor((tapY - canvasRect.top) / cellH) + 1;  // 1-based
    // X10 mouse encoding: \x1b[M + button(32=left-press) + col+32 + row+32
    const press  = `\x1b[M${String.fromCharCode(32, col + 32, row + 32)}`;
    const release = `\x1b[M${String.fromCharCode(35, col + 32, row + 32)}`; // button 35 = release
    onSendData(press + release);
    terminal.focus();
    ```
- **Why**: R4.1 — a single tap must position the cursor in CLI prompts when mouse tracking is active; without synthesizing the ANSI mouse escape sequences the tap is silently dropped.
- **Test**: Jest test — simulate tap with `terminal.modes.mouseTrackingMode = 'vt200'` at pixel `(100, 50)` with `cellH = 25, cellW = 10`; assert `onSendData` is called with the correct `\x1b[M...` sequence.

#### Task 3.1.7 — Remove useTouchScroll and useMobileTerminalGestures from XtermTerminal

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: ~115 (`useTouchScroll` call), ~136–141 (`useMobileTerminalGestures` call)
- **Change**: Delete both hook calls and their associated imports. Add `useTerminalGestures({ containerRef, terminal: terminalRef.current, onSendData: onDataRef.current })` in their place. The `useTerminalGestures` hook handles its own cleanup via the returned disposer.
- **Why**: R4.3 — the two hooks must be replaced, not just de-conflicted; having both registered simultaneously is the root cause of the double-scroll and selection-during-scroll failure.
- **Test**: Jest test — render `XtermTerminal`; assert `useTouchScroll` and `useMobileTerminalGestures` are not called (can be verified by checking that the no-longer-imported modules are absent from the component).

### Story 3.2: Floating Copy button

#### Task 3.2.1 — Add onSelectionChange listener and Copy button state

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: ~263–268 (existing `selectionDisposable` / `onSelectionChange` handler)
- **Change**: Replace the existing `onSelectionChange` handler (which incorrectly calls `clipboard.writeText` from a non-gesture event):
  ```ts
  const [copyButtonPos, setCopyButtonPos] = useState<{ x: number; y: number } | null>(null);
  
  const selectionDisposable = terminal.onSelectionChange(() => {
    const text = terminal.getSelection();
    if (text && text.length > 0) {
      const pos = terminal.getSelectionPosition();
      if (pos && terminal.element) {
        const rect = terminal.element.getBoundingClientRect();
        const { cellH, cellW } = getCellDimensions(terminal);
        // Position button near selection end
        setCopyButtonPos({
          x: rect.left + pos.end.x * cellW,
          y: rect.top + pos.end.y * cellH - 40, // 40px above selection end
        });
      }
    } else {
      setCopyButtonPos(null);
    }
  });
  ```
  Move `setCopyButtonPos` state out of the effect (into the component body) so it persists across re-renders.
- **Why**: R3.1 — a floating Copy button must appear when xterm.js has a non-empty selection; `onSelectionChange` is the correct event but must only set state (not invoke clipboard), as iOS does not recognize `onSelectionChange` as a user gesture.
- **Test**: Jest/RTL test — fire `terminal.onSelectionChange` with `getSelection()` returning `'hello'`; assert the Copy button element is in the document.

#### Task 3.2.2 — Render floating Copy button and wire clipboard write

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: JSX return block
- **Change**: Add to the JSX:
  ```tsx
  {copyButtonPos && (
    <button
      className={styles.floatingCopyButton}
      style={{ position: 'fixed', left: copyButtonPos.x, top: copyButtonPos.y, zIndex: 9999 }}
      onPointerDown={(e) => {
        // Synchronous within user gesture — safe for iOS clipboard
        const text = terminal.getSelection(); // synchronous
        if (navigator.clipboard?.writeText) {
          navigator.clipboard.writeText(text).catch(() => {});
        } else {
          // Fallback: execCommand
          const el = document.createElement('textarea');
          el.value = text;
          document.body.appendChild(el);
          el.select();
          document.execCommand('copy');
          document.body.removeChild(el);
        }
        setCopyButtonPos(null);
        // Show brief "Copied" toast
        setShowCopiedToast(true);
        setTimeout(() => setShowCopiedToast(false), 1500);
        e.preventDefault();
      }}
    >
      Copy
    </button>
  )}
  {showCopiedToast && (
    <div className={styles.copiedToast}>Copied</div>
  )}
  ```
  Add `floatingCopyButton` and `copiedToast` styles to `XtermTerminal.css.ts` (vanilla-extract, per CSS architecture rules).
- **Why**: R3.2 — the Copy button must call `clipboard.writeText` synchronously inside a `pointerDown` (user gesture) handler, not from `onSelectionChange` which is not recognized as a gesture by iOS Safari.
- **Test**: Jest/RTL test — when `copyButtonPos` is non-null, click the Copy button; assert `navigator.clipboard.writeText` is called with the mocked `getSelection()` return value.

---

## Epic 4: Frontend Overlay + Integration

**Covers**: R1.4, R5.1–R5.4
**Depends on**: Epic 1 (ResizeQuiescence proto variant), Epic 2 (write-lock in TerminalStreamManager)
**Agent runs**: TypeScript frontend only.

### Story 4.1: Terminal state machine

#### Task 4.1.1 — Add terminal state enum to useTerminalStream

- **File**: `web-app/src/lib/hooks/useTerminalStream.ts`
- **Lines**: Return type and internal state declarations
- **Change**: Add:
  ```ts
  export type TerminalState =
    | 'DISCONNECTED'
    | 'CONNECTING'
    | 'LOADING'
    | 'STABLE'
    | 'RESIZING'
    | 'FETCHING_SCROLLBACK';
  ```
  Add `terminalState: TerminalState` to the hook's return value. Internal transitions:
  - → `CONNECTING` on `connect()` call.
  - → `LOADING` on WebSocket open + first message sent.
  - → `STABLE` on initial snapshot written (first `TerminalData_Output` processed).
  - → `RESIZING` on `ResizeQuiescence{resizing: true}` received.
  - → `STABLE` on `ResizeQuiescence{resizing: false}` received.
  - → `FETCHING_SCROLLBACK` when `requestScrollback` is called.
  - → `STABLE` after `ScrollbackResponse` is processed.
  - → `DISCONNECTED` on WebSocket close.
- **Why**: R1.4 — the overlay must be driven by a typed state machine rather than ad-hoc booleans; `RESIZING` is now a distinct state driven by server messages rather than client-side guesswork.
- **Test**: Jest test `useTerminalStream.test.ts` — simulate receiving `ResizeQuiescence{resizing: true}`; assert `terminalState === 'RESIZING'`. Simulate `ResizeQuiescence{resizing: false}`; assert `terminalState === 'STABLE'`.

#### Task 4.1.2 — Handle ResizeQuiescence message in client stream loop

- **File**: `web-app/src/lib/hooks/useTerminalStream.ts`
- **Lines**: Message dispatch block (where `getOutput()`, `getScrollbackResponse()` etc. are handled)
- **Change**: Add a dispatch case for `getResizeQuiescence()`:
  ```ts
  const rq = msg.getResizeQuiescence?.();
  if (rq) {
    if (rq.resizing) {
      setTerminalState('RESIZING');
    } else {
      setTerminalState('STABLE');
    }
    return; // no further processing
  }
  ```
  Note: `getResizeQuiescence` is generated by `make generate-proto` after Epic 1 Task 1.1.4.
- **Why**: Without dispatching on the new proto variant, the client never transitions to/from `RESIZING` state, the overlay never appears, and R1.4 is not satisfied.
- **Test**: Jest test — inject a `TerminalData` with `ResizeQuiescence{resizing: true}` into the mock WebSocket; assert `terminalState` transitions to `'RESIZING'`.

### Story 4.2: Resize overlay

#### Task 4.2.1 — Add resizing overlay to TerminalOutput

- **File**: `web-app/src/components/sessions/TerminalOutput.tsx`
- **Lines**: JSX return block (find where the terminal container div is rendered)
- **Change**: Wrap the xterm container with a `position: relative` parent. Add:
  ```tsx
  {terminalState === 'RESIZING' && (
    <div
      className={styles.resizingOverlay}
      aria-label="Terminal resizing"
      aria-live="polite"
    >
      <span className={styles.resizingSpinner} />
    </div>
  )}
  ```
  Thread `terminalState` from `useTerminalStream` return value to this JSX. Add `resizingOverlay` and `resizingSpinner` styles to `TerminalOutput.css.ts` (vanilla-extract). The overlay should be: `position: absolute; inset: 0; background: rgba(0,0,0,0.3); display: flex; align-items: center; justify-content: center; pointer-events: none;` (non-blocking).
- **Why**: R1.4 — the user must see a visual indicator (dimmed overlay) while the server is waiting for tmux quiescence; without it, the terminal appears frozen with corrupted content for up to 300 ms.
- **Test**: Jest/RTL test — render `TerminalOutput` with `terminalState = 'RESIZING'`; assert the overlay element is in the document with the correct `aria-label`. With `terminalState = 'STABLE'`; assert overlay is absent.

#### Task 4.2.2 — Queue incoming output during RESIZING state

- **File**: `web-app/src/components/sessions/TerminalOutput.tsx`
- **Lines**: `handleOutput` callback (~line 310)
- **Change**: In `handleOutput`, check `terminalState`. If `terminalState === 'RESIZING'`, push to a `pendingOutputDuringResize` ref (array of strings) instead of calling `manager.write(output)`. When `terminalState` transitions from `RESIZING` → `STABLE`, flush `pendingOutputDuringResize` through `manager.write()` in order.
- **Why**: Pitfall #2 (Race 3) — a snapshot arriving while the client is mid-resize writes bytes at the old column width; subsequent `fit()` renders them incorrectly. Queuing output during resize prevents stale bytes from being written before the post-resize snapshot.
- **Test**: Jest test — simulate `terminalState = 'RESIZING'`; call `handleOutput('live bytes')`; assert `manager.write` is NOT called. Then transition to `STABLE`; assert `manager.write('live bytes')` is called.

---

## Cross-Cutting Tasks (no epic, required before merge)

### Task X.1 — Remove stale `mouseTracking` option set in XtermTerminal

- **File**: `web-app/src/components/sessions/XtermTerminal.tsx`
- **Lines**: ~127–129 and ~163 (`mouseTracking` in Terminal constructor options)
- **Change**: Remove `mouseTracking` from the `Terminal({...})` options object. The `mouseTracking` option is not a valid `ITerminalOptions` field in xterm.js 6.0.0; setting it is a no-op. Remove the `mouseTracking` prop from the component's prop type if it is only used for this no-op assignment. Add a comment: "Mouse tracking mode is set at runtime by PTY escape sequences and read via terminal.modes.mouseTrackingMode."
- **Why**: Removes misleading dead code that implies mouse tracking is configurable via prop when it is actually PTY-controlled at runtime (per stack.md §2.6 and §6).
- **Test**: TypeScript compilation succeeds with `mouseTracking` removed from options (`make build`).

### Task X.2 — Feature registry update

- **Files**: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`
- **Change**: Add entries for:
  - `scrollback:on-demand` (backend RPC wired in Task 2.1.1)
  - `resize:quiescence` (backend behavior + proto field from Epic 1)
  - `mobile:gesture-recognizer` (frontend feature, `useTerminalGestures` hook)
  - `mobile:copy-button` (frontend feature, floating Copy button)
  Set `tested: false` initially; update to `true` as tests are added in the respective epics.
- **Why**: Per `.claude/rules/feature-registry.md` — every feature change must update the registry; failing to do so causes `coverage-gaps.json` to grow and CI to flag it.
- **Test**: `make registry-diff` shows no unexpected gaps.

### Task X.3 — Run make ci after all epics land

- **Command**: `make ci`
- **Change**: No file change — validation gate.
- **Why**: R5.4 — no regressions in existing desktop keyboard shortcuts, search, or WebGL rendering.
- **Test**: `make ci` exits 0.

---

## Implementation Sequence

```
Week 1 (parallel):
  Agent A: Epic 1 (Tasks 1.1.1 → 1.1.5 → 1.2.1 → 1.2.2 → 1.2.3)
  Agent B: Epic 2 (Tasks 2.1.1 → 2.1.2 → 2.1.3 → 2.2.1 → 2.2.2 → 2.2.3 → 2.3.1 → 2.3.2 → 2.3.3)
  Agent C: Epic 3 (Tasks 3.1.1 → 3.1.2 → 3.1.3 → 3.1.4 → 3.1.5 → 3.1.6 → 3.1.7 → 3.2.1 → 3.2.2)

Week 2:
  Agent D: Epic 4 (after Epic 1 and 2 are merged; Tasks 4.1.1 → 4.1.2 → 4.2.1 → 4.2.2)
  All agents: Task X.1, X.2, X.3
```

---

## Flagged Technology Decisions (for ADR Agent)

The following decisions require ADRs before or during implementation:

1. **ADR-010: Scrollback paging strategy — serialize-clear-rewrite vs. reconnect-with-larger-window**
   - Decision: Task 2.3.3 uses `SerializeAddon` to serialize current buffer, clear, write older history, write saved state. Alternative: when user scrolls to top, disconnect and reconnect with a larger `scrollbackLines` value in the handshake, avoiding the prepend problem entirely.
   - Tradeoff: serialize-clear-rewrite preserves the live stream but disrupts selection state and is CPU-intensive for large buffers; reconnect is simpler but causes a visible flicker and re-sends the handshake.
   - Recommendation to document: which approach is chosen and why.

2. **ADR-011: ResizeQuiescence proto field placement — new oneof variant vs. reuse existing field**
   - Decision: Task 1.1.4 adds `resize_quiescence = 16` to the `TerminalData` oneof. Alternative: encode resize state as a flag in `TerminalResize` message.
   - Tradeoff: new oneof variant is type-safe and unambiguous; reusing `TerminalResize` requires clients to distinguish "resize command" from "resize state notification" by checking a new flag.
   - Recommendation to document: new variant is cleaner; field 16 is confirmed free.

3. **ADR-012: Mobile gesture event model — TouchEvent vs. PointerEvent**
   - Decision: Task 3.1.1 uses `TouchEvent` (`touchstart`, `touchmove`, `touchend`). Alternative: `PointerEvent` (`pointerdown`, `pointermove`, `pointerup`) which has better cross-device support and avoids Touch object recycling issues.
   - Tradeoff: `PointerEvent` fires `pointercancel` on iOS when a scroll gesture is detected, complicating the long-press state machine; `TouchEvent` is more predictable for `touchcancel` semantics. The existing codebase already uses `TouchEvent` exclusively.
   - Recommendation to document: stick with `TouchEvent` for this iteration; revisit if stylus support is needed.

4. **ADR-013: iOS clipboard — synchronous gesture requirement**
   - Decision: Task 3.2.2 uses `onPointerDown` (not `onClick`) for the Copy button to ensure the clipboard call is within the synchronous user gesture stack on iOS Safari.
   - Tradeoff: `pointerDown` fires before `click`; if the user lifts their finger outside the button, the clipboard write has already happened without a visible "Copied" confirmation. `onClick` is safer for confirmation UX but may fail on iOS Safari if async work precedes `writeText`.
   - Recommendation to document: `onPointerDown` with synchronous `getSelection()` + immediate `writeText()` call, per pitfalls.md §4.

5. **ADR-014: Quiescence detection strategy — fixed 100 ms wait vs. polling capture-pane checksums**
   - Decision: Task 1.1.2 uses `waitForQuiescence(quiescenceCh, 300ms, 100ms)` — the existing quiescence helper that monitors control-mode output silence.
   - Alternative: poll `capture-pane`, checksum the output, and compare successive captures until stable (per features.md §4).
   - Tradeoff: control-mode silence detection is already implemented in the codebase and has been validated at handshake time; polling capture-pane adds latency and extra subprocess calls. The control-mode approach is preferred.
   - Recommendation to document: reuse existing `waitForQuiescence` with 100 ms quiet window; document the 300 ms total timeout as the performance bound for R5.2.
