# Terminal Robustness — Feature Research

Research into how comparable web terminal and native mobile terminal applications handle scrollback, resize, and mobile UX.

---

## 1. Scrollback in Web Terminals

### How Comparable Projects Handle It

**GoTTY / ttyd (simple relay model)**
Both GoTTY and ttyd act as transparent PTY relays: they stream raw PTY bytes to the browser over WebSocket and rely entirely on xterm.js to maintain the scrollback buffer client-side. Neither project sends server-side history on reconnect; the client gets a fresh empty terminal on each connection. The `disableReconnect` client option in ttyd simply reconnects to a live PTY — no history is replayed.

**Wetty**
Wetty follows the same relay architecture (Node.js + WebSocket + SSH connection). No scrollback pre-population pattern is documented. On reconnect the user sees the current PTY screen only.

**VS Code Remote Terminal**
VS Code takes a different, more sophisticated approach: the Integrated Terminal runs across a split architecture (renderer process owns xterm.js, PTY host process owns node-pty). VS Code keeps a server-side replay buffer ("persistent terminal" feature). On reconnect, it replays the buffered output into xterm.js using `terminal.write()` calls in sequence before resuming live streaming. This is the canonical "pre-populate then stream" pattern.

### xterm.js Pre-population Pattern

The universally endorsed pattern for loading history into xterm.js is:

1. Set `scrollback: N` (minimum 5,000 lines) at terminal construction time.
2. Before opening the live stream, call `terminal.write(historicalChunk)` with the ANSI-encoded historical bytes from the server buffer. xterm.js will parse escape sequences, lay out the lines, and push them into the scrollback buffer. The cursor ends up at the correct position.
3. Only then attach the live data stream.

Limitations documented in xterm.js issues:
- xterm.js parses ANSI escape sequences on the client, so the server cannot efficiently send "raw line objects" — it must send the actual byte stream that produced the history.
- There is no public API to prepend lines above existing scrollback. `terminal.write()` always appends at the current cursor. The implication is that for initial population the terminal should be empty when history is written.
- Infinite scrollback (`scrollback: 99999999`) is technically possible but risks OOM; the maintainers recommend capping at a reasonable value and implementing lazy server-side loading instead.

### Lazy Load Near Top-of-Buffer

xterm.js exposes `terminal.buffer.active.viewportY` (public API, IBuffer interface). When `viewportY` approaches 0, the client is near the top of the local buffer and should request an older chunk from the server. However:
- `onScroll` only fires when new lines are introduced by writes, not on user scroll gestures (xterm.js issues #3201, #3864).
- The practical workaround is to wrap the terminal's scrollbar container with a native DOM `scroll` event listener and check `viewportY` there.
- There is no public API to insert lines at the top of an existing buffer (discussion #5150). The community workaround — copying buffer lines, resetting, and rewriting — causes selection to reset and is considered a hack. The recommended approach for deep-history is to design for "write all history first, then stream", rather than dynamic top-insertion.

**Practical recommendation**: Write the initial 500-line batch before starting the live stream (R2.2). For older batches, prepend to xterm.js before live data resumes (pause streaming, write history chunk, resume). This avoids the insert-at-top limitation.

---

## 2. Resize Handling

### The Core Race Condition

The PTY resize call (`TIOCSWINSZ` ioctl → SIGWINCH to child processes) is asynchronous. Data written by the child process under the old terminal width may still be in PTY kernel buffers when the resize is acknowledged. xterm.js issue #1914 documents this extensively:

> XOFF guarantees that pending data are already written and an ACK token ensures that the PTY has applied the new size. The ACK token could be inserted in the websocket stream and might contain the applied size.

This is called the "resize roundtrip" protocol. The full safe sequence is:
1. Frontend sends resize request (cols × rows) to server.
2. Server issues `TIOCSWINSZ` to PTY and `tmux resize-window -t <target> -x <cols> -y <rows>`.
3. Server inserts an ACK marker into the data stream back to the client.
4. Client waits for ACK before rendering the post-resize snapshot.

In practice, no open-source web terminal implements the full XOFF+ACK roundtrip. The universal fallback is debouncing + a fixed wait.

### ttyd Resize Approach

ttyd receives resize messages from xterm.js over WebSocket, decodes the `{columns, rows}` payload, and immediately calls `pty.Setsize()`. There is no quiescence detection or post-resize snapshot; ttyd relies on the PTY itself to re-emit the child's redrawn output. The known bug (terminal wider than window on first load with non-default font size, issue #415) results from no sync between the xterm.js fitAddon measurement and the actual PTY size at connection time.

### tmux Resize Timing and Hooks

tmux provides two relevant hooks:
- `client-resized`: fires when the tmux client is resized. This is before reflow is complete in some cases (issue #2995: "resizing slowly works but large steps do not").
- `window-resized`: fires after the window has been resized. According to tmux documentation, this fires after `client-resized`, making it a better candidate for post-resize capture, though no official guarantee that reflow is complete by this point.

Neither hook guarantees tmux's internal reflow is complete before the hook fires. The reflow of a large history (`window-history-limit` = 50,000 lines) can take seconds (tmux issue #4171).

**Recommended sequence for tmux + web terminal**:
1. Frontend debounces resize events to 150 ms (a single settled size per burst).
2. Frontend sends resize to server.
3. Server calls `ioctl(TIOCSWINSZ)` on PTY fd, then runs `tmux resize-window`.
4. Server waits for tmux output quiescence: poll `capture-pane` and compare checksums or wait for no-output interval of ≥100 ms.
5. Only after quiescence does server send the post-resize snapshot to the client.

The ±1 nudge hack (used in current stapler-squad implementation) forces a SIGWINCH but does not provide any quiescence guarantee. It should be replaced with the output-quiescence polling approach.

### VS Code Debounce Pattern

VS Code's terminal debounces `ResizeObserver` callbacks before calling `fitAddon.fit()`. The Kilo-Org cloud terminal project (GitHub issue #1195) documents:

> Wrapping the ResizeObserver callback and fitAddon.fit() calls with a debounce (150–200 ms) and only sending resize mutations after the debounce settles prevents resize storms during CSS transitions and ensures the PTY gets a single, final resize rather than dozens of intermediate ones.

xterm.js fit addon issues #3564 and #3584 document that calling `fit()` multiple times rapidly during CSS transitions produces erratic results. The fix is always exactly one `fit()` call after the resize has settled.

---

## 3. Mobile Terminal UX

### Blink Shell (Native iOS)

Blink Shell is the reference-grade native iOS terminal. Key UX patterns:
- **Text selection**: Tap and hold (long press) enters selection mode. The selection extends with drag gestures. The native iOS selection handles appear.
- **Copy**: Standard iOS "Copy" callout menu appears above the selection.
- **OSC 52**: Blink supports the OSC 52 terminal escape sequence (`\x1b]52;c;<base64>\x07`) over SSH connections, copying from remote programs into the iOS clipboard without any touch interaction.
- **Gestures**: Pinch to zoom changes font size. Swipe left/right switches between open connections.

### iSH and a-Shell (Emulated Linux on iOS)

Both are local shell apps, not web terminals. They use UIKit's native `UITextView` or custom text engine for input/output, giving them iOS-standard selection handles and copy menu for free. Not directly applicable to xterm.js in a web view.

### Web Terminals on Mobile (Wetty, ttyd)

Neither Wetty nor ttyd has documented first-class mobile support. The xterm.js issues tracker is the authoritative source:

- **Issue #1101** (opened 2017, still open): "Support mobile platforms" — xterm.js has very limited touch support; `CoreBrowserTerminal.ts` handles mouse and keyboard events with no dedicated touch event handling.
- **Issue #3727** (opened 2022): "Copy and paste do not work on touch devices" — it is impossible to select text by touching on Safari iOS across all xterm.js versions. The issue affects all renderers (WebGL, canvas, DOM). No upstream fix exists.
- **Issue #5377**: Limited touch support on mobile devices impacts terminal usability; labeled `help wanted`.

### Practical Mobile Copy Strategy

Since xterm.js has no native touch selection, the approaches used in production web terminals are:

1. **OSC 52 passthrough** (works for copy-from-terminal-app, not user-driven selection):
   - Register an OSC 52 handler in xterm.js: `terminal.parser.registerOscHandler(52, handler)`.
   - In tmux: `set -g allow-passthrough on` — required because modern apps wrap OSC 52 in DCS passthrough. Without this, the sequence is consumed by tmux and never reaches xterm.js.
   - Handler: decode base64 payload, call `navigator.clipboard.writeText(decoded)`.
   - Limitation: requires the program running in the terminal to emit OSC 52; does not enable user-initiated text selection.

2. **Custom long-press + floating Copy button** (user-driven selection):
   - Override `touchstart` (400 ms threshold) on the terminal container.
   - Use `terminal.select(col, row, length)` public API to mark a selection range.
   - Compute `col` and `row` from touch pixel coordinates divided by cell dimensions (`terminal.options.fontSize * terminal.options.lineHeight` for height; `terminal.options.fontSize * 0.6` or measured char width for width).
   - Listen to `terminal.onSelectionChange` — when selection is non-empty, show a floating "Copy" button absolutely positioned near the selection end.
   - Button tap: `navigator.clipboard.writeText(terminal.getSelection())` + dismiss.
   - When xterm.js mouse tracking mode is `vt200` or higher, synthetic `mousedown` events are forwarded to the PTY, breaking selection. The fix: use `terminal.select()` API directly (it bypasses mouse tracking mode entirely) instead of synthetic mouse events.

3. **Tap-to-cursor (R4.1)**:
   - Single `touchend` with no movement: convert pixel `(x, y)` → cell `(col, row)` using public dimensions, then send the ANSI mouse sequence `\x1b[M<btn><col+32><row+32>` to the PTY when mouse tracking mode is `vt200`+.
   - When tracking is `none`: just call `terminal.focus()`.
   - `useTouchScroll` and `useMobileTerminalGestures` must be merged into a single gesture recognizer state machine (states: idle, scrolling, long-press-selecting, tapping) to prevent event absorption conflicts.

---

## 4. tmux capture-pane Sequence After Resize

### Official Sequence (Best Practice)

No tmux documentation explicitly defines a post-resize capture-pane safe window. From tmux issue analysis and community practice:

```
resize PTY (TIOCSWINSZ)
  → tmux resize-window -t <session>:<window> -x <cols> -y <rows>
    → tmux internally: relayout panes, reflow history, send SIGWINCH to child processes
      → child processes redraw at new size
        → quiescence (no output for ~100 ms)
          → capture-pane -p -S - -E - (safe to call)
```

The only way to know reflow is complete is to observe that tmux is producing no further output. There is no hook or event that fires after reflow.

**Practical polling approach** (used by tools that need stable snapshots):
- After `resize-window`, run `capture-pane` in a polling loop checking for stability: compare consecutive captures with a short sleep (50 ms). When two consecutive captures are identical, reflow is done.
- Cap at a maximum of 5 iterations (250 ms total) to bound latency.

**Alternative: output-quiescence on the PTY fd**:
- Watch the PTY master fd with `select()/epoll` for read events.
- After the resize, wait until the fd has been idle (no new bytes) for ≥100 ms.
- Then `capture-pane`. This is more reliable than comparing capture outputs because it detects the end of child process redraws at the PTY level.

### tmux hook `window-resized`

The `window-resized` hook is the closest to a "resize complete" event:
```bash
set-hook -g window-resized "run-shell 'your-capture-script.sh'"
```
However, it fires when the window *size changes*, not when reflow is *complete*. For large scrollback buffers, reflow can take hundreds of milliseconds after the hook fires. Using `window-resized` as a trigger but then applying the output-quiescence poll inside the script is the recommended pattern.

---

## 5. xterm.js Scrollback Pre-population: Recommended Pattern

Based on research across xterm.js issues, VS Code implementation, and Zuul's log-streaming use case:

### Connection Handshake Protocol

```
Client                          Server
  |                               |
  |-- StreamTerminal (handshake)->|
  |   { lines: 500, ... }         |
  |                               |
  |<-- TerminalData (history) ----|  (batch: historical bytes)
  |   terminal.write(chunk)       |
  |   [scroll positioned]         |
  |                               |
  |<-- TerminalData (live stream)-|  (real-time PTY output)
  |   terminal.write(data)        |
```

### Key Implementation Points

1. **Set scrollback at construction**: `new Terminal({ scrollback: 5000 })`. Cannot be changed after construction without reset.

2. **Write history as raw bytes**: The server must send the ANSI-encoded byte stream (the same bytes that the PTY produced), not processed line strings. xterm.js will parse them correctly and reconstruct the visual state including colors, cursor position artifacts, etc.

3. **Write history before live stream**: Attach the live stream handler only after the history `write()` call completes (or its callback fires). This ensures the cursor is at the right position when live data arrives.

4. **Scroll to bottom after history**: Call `terminal.scrollToBottom()` after writing history so the user sees the current screen, not the top of the historical buffer.

5. **Near-top trigger for older batches**: Monitor `terminal.buffer.active.viewportY` via a DOM `scroll` event listener on the terminal element. When `viewportY < 200` (configurable threshold), pause live streaming temporarily, request an older batch from the server, prepend it (write before live data resumes), then resume.

6. **xterm.js `onScroll` unreliability**: Do not use `terminal.onScroll` for this purpose — it only fires on buffer writes, not user scroll events (issues #3201, #3864). Use DOM event instead.

---

## Summary of Key Sources

- [xterm.js issue #3727: Copy/paste broken on touch devices](https://github.com/xtermjs/xterm.js/issues/3727)
- [xterm.js issue #1101: Support mobile platforms](https://github.com/xtermjs/xterm.js/issues/1101)
- [xterm.js issue #1914: Terminal resize roundtrip (XOFF+ACK pattern)](https://github.com/xtermjs/xterm.js/issues/1914)
- [xterm.js discussion #5150: Insert lines from top (prepend limitation)](https://github.com/xtermjs/xterm.js/discussions/5150)
- [xterm.js issue #2181: Infinite history log on server](https://github.com/xtermjs/xterm.js/issues/2181)
- [xterm.js issue #3864: onScroll not fired on user scroll](https://github.com/xtermjs/xterm.js/issues/3864)
- [tmux issue #4003: Force tmux to adapt to new terminal size](https://github.com/tmux/tmux/issues/4003)
- [tmux issue #2995: client-resized hook strange resizing](https://github.com/tmux/tmux/issues/2995)
- [tmux issue #4171: Resizing panes is very slow](https://github.com/tmux/tmux/issues/4171)
- [tmux Hooks documentation](https://tmux-tmux.mintlify.app/configuration/hooks)
- [Kilo-Org cloud terminal issue #1195: Resize debounce pattern](https://github.com/Kilo-Org/cloud/issues/1195)
- [OSC 52 clipboard fix: xterm.js through tmux](https://max.nardit.com/articles/osc52-clipboard-xterm-tmux)
- [tmux issue #3192: OSC 52 passthrough](https://github.com/tmux/tmux/issues/3192)
- [Blink Shell docs](https://docs.blink.sh/)
- [xterm.js IBuffer API: baseY and viewportY](https://xtermjs.org/docs/api/terminal/interfaces/ibuffer/)
- [xterm.js Viewport and Scrolling (DeepWiki)](https://deepwiki.com/xtermjs/xterm.js/4.5-viewport-and-scrolling)
- [Zuul: Use xterm.js for live log streaming](https://opendev.org/zuul/zuul/commit/bb352a3559d75e816d1f9fd9a645fb41dee8ce10)
- [ttyd Client Options wiki](https://github.com/tsl0922/ttyd/wiki/Client-Options)
