# Terminal Robustness — Requirements

**Goal**: Deliver an iTerm2-grade browser terminal experience: faithful scrollback, corruption-free resize, and first-class mobile touch support.

---

## Context

Stapler Squad runs Claude Code sessions in tmux inside a browser terminal (xterm.js + ConnectRPC streaming). The current implementation suffers from four interconnected problems that break the "native terminal" illusion:

### Problem 1 — Resize/Reflow Corruption

**What happens**: When the browser pane or window resizes, tmux reflows wrapped lines at the new width. The server captures a new snapshot (`capture-pane`) but the snapshot may mix old-width and new-width content if captured before tmux finishes reflowing. xterm.js then renders this mixed state, producing corruption (lines bleeding, wrong line breaks, doubled content).

**Root causes identified**:
- Adaptive debounce (10 ms for first 3 resizes, 250 ms after) triggers early captures before tmux quiescence.
- The ±1 nudge hack forces SIGWINCH but does not guarantee tmux has finished re-rendering before the next capture.
- The frontend `fitAddon.fit()` is called twice (initial + 100 ms delayed), which fires two resize events on some browsers, doubling resize churn.
- `localStorage`-cached cell dimensions can produce the wrong initial fit on the next mount, leading to a spurious resize event.

### Problem 2 — Scrollback Buffer Corruption

**What happens**: Scrolling up in the browser shows current terminal content repeated rather than older history.

**Root causes identified**:
- xterm.js `scrollback` is set to **0** (intentional: the comment says "tmux handles scrollback to avoid duplication"), but the client never fetches server-side history when the user scrolls up.
- `handleScrollbackReceived()` in `TerminalOutput.tsx` **rejects** historical scrollback requests with a metadata guard.
- The initial payload sends only 50 lines (`lines: 50` in the stream handshake). The server's `ScrollbackManager` has a 10,000-line circular buffer, but the client never requests deeper history.
- When xterm.js hits its scrollback=0 ceiling, its viewport wraps around, displaying the current screen contents at position 0—creating the "current content repeated" artifact.

### Problem 3 — Mobile Touch: Select & Copy

**What happens**: On iOS/Android, users cannot select text or invoke the system copy sheet.

**Root causes identified**:
- Selection requires a long-press (400 ms) that dispatches a synthetic `mousedown`. This works only when xterm.js mouse tracking is `none`; Claude Code sets `vt200` tracking which intercepts the event.
- After selection the "copy" affordance (toolbar button or system sheet) is absent—there is no floating "Copy" button.
- The private API `terminal._core._renderService.dimensions.css.cell.height` used for line-height calculation is unreliable and breaks across xterm.js minor versions.

### Problem 4 — Mobile Touch: Tap to Position Cursor

**What happens**: A single tap does not reliably position the cursor in the terminal, making it hard to interact with CLI prompts.

**Root causes identified**:
- Single tap dispatches `mousedown`+`mouseup` without accounting for mouse tracking mode.
- When xterm.js is in `vt200` (or higher) mouse tracking, the synthetic events are not translated to the correct escape sequences and forwarded to the PTY.
- `useTouchScroll.ts` and `useMobileTerminalGestures.ts` conflict: both handle `touchmove`, and `useTouchScroll` may absorb scroll events before the gesture hook can decide whether the touch is a tap or scroll.

---

## Requirements

### R1 — Corruption-Free Terminal Resize

**R1.1** After any resize event, the server MUST wait for tmux quiescence (no tmux output for ≥100 ms) before capturing and sending a new snapshot to the client.

**R1.2** The frontend MUST debounce resize events to at most one resize per 150 ms (collapse all intermediate sizes; send only the final settled size).

**R1.3** The frontend MUST NOT call `fitAddon.fit()` more than once per resize event cycle. The secondary 100 ms delayed fit MUST be removed.

**R1.4** After a resize, the terminal MUST display a non-blocking visual indicator (dimmed overlay or spinner) while waiting for the post-resize snapshot, then clear it when the snapshot arrives.

**R1.5** The backend resize handler MUST coalesce rapid resize events (same cols/rows within 50 ms) to avoid issuing redundant PTY `ioctl` calls.

**R1.6** Cached cell dimensions in `localStorage` MUST be validated against the current font size and font family before use; stale entries MUST be ignored (and replaced) rather than applied.

---

### R2 — Faithful Scrollback

**R2.1** xterm.js `scrollback` MUST be set to a minimum of 5,000 lines (not 0).

**R2.2** On initial connection, the server MUST send the most recent N lines of scrollback (N configurable, default 500) as an initial payload written into xterm.js before live streaming begins.

**R2.3** When the user scrolls to within 200 lines of the top of xterm.js's scrollback buffer, the frontend MUST request an additional batch of older lines from the server.

**R2.4** The server MUST expose a ConnectRPC endpoint (or extend the existing stream handshake) to return scrollback by line-range or byte-range from the `ScrollbackManager`.

**R2.5** Scrollback content received from the server MUST be written into xterm.js via `terminal.write()` at the current scrollback position without clearing the visible screen.

**R2.6** The initial handshake `lines: 50` parameter MUST be increased to match R2.2 (500 lines).

**R2.7** The `handleScrollbackReceived()` rejection of historical scrollback MUST be removed; historical scrollback writing MUST be enabled.

---

### R3 — Mobile Text Selection & Copy

**R3.1** A floating "Copy" button MUST appear whenever xterm.js reports a non-empty selection (via the `onSelectionChange` event), positioned relative to the selection end point.

**R3.2** Tapping the "Copy" button MUST invoke `navigator.clipboard.writeText(terminal.getSelection())` and display a brief "Copied" toast.

**R3.3** Long-press selection (400 ms) MUST work regardless of xterm.js mouse tracking mode. When tracking mode is not `none`, the selection MUST be performed via `terminal.select()` API rather than synthetic mouse events.

**R3.4** The private `terminal._core._renderService...` API call for cell height MUST be replaced with `terminal.options.fontSize * terminal.options.lineHeight` (or the xterm.js public `dimensions` API if available in the installed version).

**R3.5** After selection begins, `touchmove` MUST extend the selection using `terminal.select(startCol, startRow, length)` with coordinates computed from the touch position and cell dimensions.

---

### R4 — Mobile Tap to Position Cursor

**R4.1** A single tap on the terminal MUST translate to a mouse click escape sequence forwarded to the PTY when xterm.js mouse tracking is `vt200` or higher.

**R4.2** When mouse tracking is `none`, a single tap MUST focus the terminal (show the on-screen keyboard) without sending mouse sequences to the PTY.

**R4.3** `useTouchScroll.ts` and `useMobileTerminalGestures.ts` MUST be merged or clearly de-conflicted so that scroll, tap, and long-press are mutually exclusive gesture recognizer states.

**R4.4** The tap-to-cursor escape sequence MUST use the correct coordinates derived from the tapped pixel position and xterm.js cell dimensions, using the public API (not private internals).

---

### R5 — Performance & Quality Bar

**R5.1** Terminal output MUST render at ≥30 FPS during normal Claude Code output bursts (measured as xterm.js `write()` calls per animation frame).

**R5.2** Resize → stable-display round trip MUST complete in ≤800 ms on a localhost connection.

**R5.3** Initial scrollback load (500 lines) MUST complete in ≤300 ms on a localhost connection.

**R5.4** No regressions in existing desktop keyboard shortcuts, search, or WebGL rendering.

---

## Out of Scope

- Ligature rendering (requires `@xterm/addon-canvas` and a ligature font; separate ADR)
- Multi-pane split terminal view
- Terminal recording/playback (separate feature)
- Non-tmux PTY mode improvements (raw mode is legacy; not the focus)

---

## Success Criteria

1. A user resizing the browser window sees no corrupted or doubled text at any point.
2. A user scrolling up 500+ lines in a long-running Claude Code session sees correct historical output, not the current screen.
3. A user on iOS Safari can long-press to select text and tap "Copy" to copy it to the system clipboard.
4. A user on iOS Safari can tap the command prompt area and type immediately.
5. All existing desktop tests (`make ci`) pass with no regressions.
