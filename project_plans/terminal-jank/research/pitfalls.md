# Research: Pitfalls for Terminal Jank Elimination

Status: Complete | Phase: 2 - Research
Created: 2026-04-09

## Summary

The three most dangerous risks are: (1) FitAddon returns `undefined` dimensions when a terminal container is hidden (display:none or zero-size), breaking resize logic if fit is called on a non-visible terminal; (2) browsers enforce a hard cap of roughly 16 WebGL contexts per page, and xterm.js WebglAddon exposes an `onContextLoss` event but does NOT automatically fall back to the canvas renderer — the application must handle this or all rendering stops; (3) tmux `capture-pane` cannot capture alternate-screen content (when Claude's TUI is active), so the "visible screen" snapshot used on reconnect will be the normal-screen fallback output, not the live TUI state — this means the visible-screen reconnect approach requires a forced TUI redraw before capture.

---

## Pitfall 1: xterm.js Hidden Terminal Issues

### What Breaks

**FitAddon.proposeDimensions() returns `undefined` when container is hidden.**

From the xterm.js source (`addon-fit/src/FitAddon.ts`), `proposeDimensions()` returns `undefined` when:
- `dims.css.cell.width === 0 || dims.css.cell.height === 0` (zero cell dimensions)
- Terminal element has no parent element

When a container has `display:none`, the browser reports all CSS dimensions as 0. FitAddon computes cols/rows by dividing container width/height by cell width/height. If the container reports 0 dimensions, the calculation produces NaN or falls into the `undefined` guard. Calling `fitAddon.fit()` on a hidden terminal does nothing or sets 80×24 default.

**Confirmed issue (xterm.js #4338):** `proposeDimensions()` returned `{ cols: NaN, rows: NaN }` when `_renderService.dimensions.actualCellWidth` was `undefined`. This happens when the renderer isn't fully initialized — which can occur when the terminal mounts while hidden.

**ResizeObserver fires with zero contentRect when container is hidden.** If a persistent terminal is CSS-hidden (`display:none` or `visibility:hidden`), ResizeObserver fires with width=0, height=0. The current `XtermTerminal.tsx` code guards against this: `if (widthChanged || heightChanged)` with a `> 1` pixel threshold, so zero-dimension notifications are suppressed. However, when the terminal is *shown again*, ResizeObserver will fire with the correct size — this is the correct trigger point to call `fit()`.

**WebGL rendering continues into a hidden canvas.** Writing to a hidden terminal works correctly at the buffer level. The WebGL renderer continues painting to an off-screen canvas. This is safe, though it wastes GPU cycles.

**Terminal.open() must not be called on a hidden container.** The xterm.js `terminal.open(element)` call triggers the initial renderer setup, which measures cell dimensions. If the element has zero dimensions at this point, the renderer may initialize with incorrect cell metrics. This is a one-time issue at creation, not on every hide/show cycle.

### Mitigations

- Never call `terminal.open()` on a hidden container. Create the DOM container first, ensure it has real dimensions (even if off-screen via `position:absolute; visibility:hidden`), then call `open()`.
- Never call `fitAddon.fit()` while hidden. Defer fit until `ResizeObserver` fires after the element becomes visible.
- Use `visibility:hidden` (not `display:none`) for hidden terminals if you want ResizeObserver and layout to keep working. With `visibility:hidden`, the element still occupies space and has real dimensions; with `display:none`, it is removed from layout entirely.
- Alternative: keep all persistent terminals in a positioned absolutely off-screen container (e.g., `position:absolute; left:-9999px`). This ensures dimensions are always valid.

---

## Pitfall 2: WebGL Context Limits

### Browser Limits

Chrome and Firefox enforce a per-page WebGL context limit, typically **16 simultaneous contexts**. Older Intel integrated graphics (igfx) can be as low as 8. When the limit is exceeded, `getContext('webgl2')` returns `null` and the existing oldest context is often lost (the browser evicts the oldest to make room).

**xterm.js issue #4379** (open as of 2026): "Browsers strictly limit the number of active WebGL contexts per page. Terminal7's multiplexer often needs dozens of panes — that's 25 panes across sessions with no browser supporting that many contexts."

The proposed solution in that issue: use a single shared WebGL context with `gl.scissor` + `gl.viewport` to render all terminals. This is a fundamental architectural choice, not a simple fix.

### How the Current Code Handles WebGL Failure

In `XtermTerminal.tsx`:

```typescript
try {
  const webglAddon = new WebglAddon();
  terminal.loadAddon(webglAddon);
  console.log("[XtermTerminal] WebGL renderer enabled");
} catch (e) {
  console.warn("[XtermTerminal] WebGL not available, using canvas fallback:", e);
}
```

The try/catch handles the case where WebGL is not available at init time. However, **it does not subscribe to `webglAddon.onContextLoss`**, which fires after the context is later evicted by the browser. When the browser evicts an existing context (rather than refusing the initial creation), the catch block never fires — `loadAddon` succeeds, but later the context silently dies.

From the WebglAddon source: `onContextLoss` is emitted after a 3-second timeout if `webglcontextrestored` does not fire. There is no automatic fallback to the canvas renderer. The terminal goes blank (white rectangle with a frown icon in Chrome) and stops rendering.

### Memory Impact per WebGL Instance

Each WebGL context allocates:
- GPU-backed texture atlas for glyphs (typically 2048×2048px, ~16MB VRAM per terminal)
- Canvas framebuffer (proportional to terminal pixel dimensions)
- Shared uniform buffers

At 10 sessions with WebGL: ~160MB VRAM minimum, potentially causing context loss on machines with integrated graphics.

### Mitigations

- Subscribe to `webglAddon.onContextLoss` and fall back to the DOM renderer (available in xterm.js 5+):
  ```typescript
  webglAddon.onContextLoss(() => {
    webglAddon.dispose();
    // terminal automatically falls back to canvas/DOM renderer
  });
  ```
- With persistent instances (the goal of this feature), 10+ sessions each holding a WebGL context is a hard constraint problem. Options:
  1. Use the DOM renderer for background (non-visible) terminals; switch to WebGL only for the active terminal.
  2. Implement a WebGL context pool (single context, scissor rendering) — significant effort, tracked as xterm.js enhancement.
  3. Cap persistent instances at a safe number (e.g., 8) and use an LRU eviction strategy for the rest.

---

## Pitfall 3: Memory Footprint of Persistent Instances

### What Each xterm.js Instance Holds

Per terminal instance at default configuration (`scrollback: 5000` as set in `TerminalOutput.tsx`):

| Component | Memory |
|-----------|--------|
| Scrollback buffer (5000 lines × ~200 bytes avg) | ~1MB JavaScript heap |
| Active viewport buffer | ~20-80KB |
| WebGL glyph texture atlas | ~16MB VRAM |
| Canvas elements (WebGL canvas + overlay canvas) | proportional to pixel size |
| Event listeners and disposables | negligible |
| TerminalStreamManager write queue | variable (up to HIGH_WATERMARK = 100KB) |

**At 10 sessions:** ~10MB JS heap + ~160MB VRAM (WebGL) = significant on low-end machines.
**At 20 sessions:** ~20MB JS heap + ~320MB VRAM — will trigger WebGL context loss on most machines.

The existing `XtermTerminal.tsx` sets `scrollback={5000}`. With 5000 lines at average 200 bytes/line including ANSI codes, that is 1MB per terminal in the xterm.js internal buffer alone.

### LRU Eviction Strategy

For a pool of persistent terminals, an LRU eviction policy is necessary:
- Keep the last N most-recently-viewed terminals alive (N = safe WebGL context count - 2, typically 6-10).
- Evicted terminals: dispose the xterm.js instance (freeing VRAM), retain the session's scrollback in the backend scrollback manager.
- On re-activation of an evicted session: create a new xterm.js instance (incurs the ~100ms init cost, but only for cold sessions).

The existing `ScrollbackManager` in `session/scrollback/manager.go` already retains scrollback (up to 10,000 lines, 10MB per session) on the backend. This is the correct architecture — the JS memory limit drives eviction, not the backend.

---

## Pitfall 4: Rapid-Switch Race Conditions

### Known Races in Current Code

**Race 1: Pending disconnect/connect overlap (`TerminalOutput.tsx` lines 446-470)**

The current session switch flow:
1. `sessionId` prop changes
2. Effect fires: `setIsLoadingInitialContent(true)`, resets state
3. If `isConnected`, calls `disconnect()` (async, up to 1s timeout)
4. Sets `pendingConnectAfterDisconnectRef = true`
5. When `isConnected` transitions to `false`, fires `connect()` for new session

If the user switches sessions again before the first disconnect completes, `pendingConnectAfterDisconnectRef` is overwritten. The `hasInitiatedConnectionRef` check prevents double-connects, but the prior disconnect may still be in-flight against the wrong session's WebSocket.

**Race 2: WebSocket messages arriving after session switch**

When a session is disconnected, the AbortController is aborted after a 1-second timeout (`useTerminalStream.ts` line 320). During this window, output from the old session can still arrive via the stream loop (the `for await` loop only terminates when the abort propagates). These outputs go to `onOutput`, which calls `TerminalStreamManager.write()`, which writes to the *current* terminal (which may now be bound to a different session).

**The TerminalStreamManager is not session-bound.** It holds a direct reference to the `terminal` (xterm.js instance). If `streamManagerRef.current` is reset and a new manager is created (current code does this on session switch via `streamManagerRef.current = null`), but the old stream's output callback still holds the old manager via closure, writes go to the old (now-reset) manager. The old manager's `cleanup()` is called, so `terminal.write()` may be called on a disposed terminal — this throws in xterm.js and is silently swallowed.

**Race 3: ResizeObserver callback after session switch**

The `ResizeObserver` in `XtermTerminal.tsx` is tied to the xterm.js component lifecycle, not the session. If `TerminalOutput` unmounts and remounts (current behavior), the observer is correctly torn down. But with persistent terminals, a hidden terminal's ResizeObserver remains active. If a layout shift affects the hidden terminal's container, the observer fires, triggers `fit()`, and sends a resize to the backend — which may not be the active session.

**Race 4: `writeInitialContent` async chunked write after session switch**

`TerminalStreamManager.writeInitialContent()` is async and uses chunked writes with `requestAnimationFrame` yields. If the user switches away during the chunked write, the writes continue to the now-background terminal. This is not harmful if terminals are persistent (writes just update the background terminal's buffer), but the `setIsLoadingInitialContent(false)` callback fires via `setOnFirstOutput` — if that callback captures React state setters from the wrong component instance, it can corrupt loading state.

### Mitigations

- Tag each WebSocket stream with a sequence/nonce. Discard output whose nonce doesn't match the current active connection.
- Separate the `onOutput` callback from the terminal reference: route output via a per-session queue, not via a shared callback that captures the current terminal.
- For `writeInitialContent`: add a cancellation token. If the session switch happens mid-write, abort remaining chunks.
- For ResizeObserver on persistent terminals: gate the resize backend call on "is this session currently active?", not just "is the terminal connected?".

---

## Pitfall 5: Cursor/Alternate-Screen State Reconstruction

### The Fundamental Problem

`tmux capture-pane` **cannot capture alternate-screen content**.

From the tmux man page: *"If the alternate screen is used, history is not accessible."* The `capture-pane` command outputs the normal-screen buffer when the pane is in alternate-screen mode. For Claude Code (which uses the alternate screen for its TUI via `\x1b[?1049h`), `capture-pane` returns either a blank screen or the last normal-screen content, not the live TUI.

The current code in `connectrpc_websocket.go` forces a `±1 nudge` resize before capturing to trigger a TUI redraw:

```go
// Resize tmux to match client dimensions BEFORE capturing.
// We use a ±1 nudge to guarantee SIGWINCH even if tmux is already at the target size.
if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil {
    targetCols := int(*currentPaneReq.TargetCols)
    // ...resize to targetCols-1, then targetCols...
}
```

This forces the TUI to redraw its alternate screen. The PTY output from that redraw flows through the scrollback manager. But `capture-pane` is then called immediately after the nudge — the redraw may not have completed before the capture. The current code has a `time.Sleep` but the timing is inherently racy.

### What Gets Lost

After a `capture-pane` snapshot is replayed into a fresh xterm.js terminal:

1. **Cursor position**: The current code strips absolute cursor positioning codes (`ESC[n;mH`) via `sanitizeInitialContent()`. This means cursor position is reset to home (1,1) on every reconnect. The user sees the cursor jump to top-left rather than its last known position.

2. **Alternate screen buffer state**: xterm.js enters alternate screen mode when it receives `\x1b[?1049h`. If the capture-pane snapshot was taken while the TUI was in normal-screen mode (because the TUI had not yet redrawn), the client xterm.js terminal is in normal-screen mode. When live streaming resumes, the first TUI redraw sends `\x1b[?1049h` followed by the full screen contents — this works correctly, but there will be a flash of normal-screen content between the snapshot and the live redraw.

3. **Scroll region**: CSI `\x1b[n;mr` (DECSTBM) sets scroll region. Not preserved in capture-pane output. If the TUI uses scroll regions and the reconnect snapshot doesn't include the set-scroll-region sequence, the initial rendering is wrong until the next full redraw.

4. **Colors and SGR attributes**: Preserved correctly by `-e` flag in `capture-pane`.

### Mitigations for Persistent Terminals

With persistent terminals (no reconnect on switch), cursor and alternate screen state are preserved in the xterm.js instance's buffer — this is precisely why persistent instances are better. The cursor/alternate-screen problem is a reconnect problem, not a persistent-instance problem. If visible-screen-only reconnect is implemented, the ±1 nudge approach needs a reliable completion signal (e.g., wait for PTY output after resize, up to 500ms).

---

## Pitfall 6: tmux capture-pane Limitations

### What capture-pane Misses

| Feature | What capture-pane Does |
|---------|----------------------|
| Alternate screen content | Returns normal screen buffer; alternate screen content is not accessible |
| Cursor position | Emitted as absolute `ESC[n;mH` sequences (stripped by current sanitizeInitialContent) |
| Scrollback history | Only accessible with `-S -` flag (which goes back to history start); by default only captures visible screen |
| Wrapped line handling | `-J` flag joins wrapped lines, losing precise cursor-positioning codes; without `-J`, wrapped lines include tmux's continuation markers |
| Mouse state | Not captured |
| Color depth/palette | Only SGR sequences preserved; actual xterm color assignments not captured |
| Clipboard | Not captured |

### The -J Flag Trade-off

The current `CapturePaneContent()` uses `-J` (join wrapped lines) + `-e` (escape sequences). The comment in `CapturePaneContentRaw()` explains the trade-off:

```go
// NO -J: Preserve wrapped lines with their original ANSI positioning codes
// The -J flag (join wrapped lines) strips cursor positioning codes, breaking TUI rendering.
```

With `-J`: wrapped long lines are joined, preserving readable text but losing cursor-positioning ESC sequences.
Without `-J`: cursor-positioning codes are preserved, but tmux adds a special continuation marker for wrapped lines that can confuse terminal emulators.

Neither option perfectly reconstructs the original PTY stream.

### Race: Capture Between Resize and Redraw

The `±1 nudge` pattern:
1. Resize to `targetCols-1` (triggers SIGWINCH to pane process)
2. Sleep 50ms
3. Resize to `targetCols` (second SIGWINCH)
4. Sleep 50-100ms
5. `capture-pane`

The race: Claude's TUI redraw in response to SIGWINCH is not guaranteed to complete in 50-100ms. If the TUI is mid-computation (generating code, etc.), the SIGWINCH may be queued and processed later. The capture then shows a half-redrawn TUI or the pre-nudge state.

There is no reliable synchronization mechanism in tmux to wait for "pane has processed SIGWINCH and written its response." The only approach is heuristic timeouts or monitoring PTY output activity.

### Known Bugs

**SGR attribute leakage**: If the pane's last output ended mid-SGR sequence (partial sequence), `capture-pane -e` may output malformed sequences. The `EscapeSequenceParser` in the frontend handles partial sequences at chunk boundaries but not at the content-start boundary (fresh terminal).

**UTF-8 sanitization**: The current code calls `sanitizeUTF8String(output)` on capture-pane output. Invalid UTF-8 bytes in the pane (e.g., from binary program output) are replaced, which can visually corrupt content where they occur. This is unavoidable given the terminal's byte-stream nature.

---

## Pitfall 7: Claude TUI Redraw Corruption

### The RedrawThrottler Design and Its Failure Modes

`TerminalStreamManager.ts` contains `RedrawThrottler`, which coalesces rapid full-screen redraws at a 10 FPS cap (100ms window). The detection heuristic:

```typescript
const isFullRedraw = /^\x1b\[\d+A/.test(chunk);
```

This matches only chunks that *start with* a cursor-up sequence (`ESC[nA`). This is correct for Claude's TUI pattern (it redraws by moving cursor up N rows and rewriting). However:

**Failure Mode 1: False negatives when redraws span multiple WebSocket messages.**

If Claude's redraw sequence is split across two WebSocket messages (e.g., `\x1b[24A` in one message and the screen content in the next), the detection fires on the first message but the second arrives without the cursor-up prefix. The throttler calls `flushPending()` on the non-redraw second chunk, which writes the pending redraw immediately, then also writes the current chunk — resulting in the screen being written twice in rapid succession.

**Failure Mode 2: Throttler drops the last redraw before a mode transition.**

Scenario: Claude exits its TUI (`\x1b[?1049l` — alternate screen exit). The final redraw arrives, is throttled (queued in `pendingRedraw`). The alternate-screen exit sequence arrives in the *next* chunk, which doesn't match the full-redraw regex, so `flushPending()` is called — the final TUI state is written, then the alternate-screen exit. This should be correct, but the 100ms throttle window means the final TUI state is delayed by up to 100ms before the exit is visible.

**Failure Mode 3: EscapeSequenceParser scans only the last 20 bytes.**

```typescript
const scanLength = Math.min(20, data.length);
```

ANSI escape sequences are almost always under 20 bytes. But tmux/VTE OSC sequences for window title (`\x1b]0;title\x07`) can be longer. If a window title sequence exceeds 20 bytes and spans a chunk boundary, the parser does not detect the partial sequence and passes the truncated sequence to xterm.js. xterm.js's internal parser handles incomplete sequences by buffering them — but the double-incomplete scenario (our partial escape *plus* xterm.js's buffering) can produce unexpected state.

**Failure Mode 4: RedrawThrottler `pendingRedraw` is overwritten without flushing.**

```typescript
this.pendingRedraw = chunk;
if (!this.throttleTimer) {
  this.throttleTimer = setTimeout(() => {
    this.flushPending();
  }, this.throttleMs);
}
```

If 3 rapid full-redraws arrive: the first queues a timer, the second and third overwrite `pendingRedraw`. When the timer fires, only the third redraw is written. This is intentional — it's the throttling behavior. But if the second redraw contained a structural difference (e.g., Claude switched modes between redraw 2 and 3), the intermediate state is lost. For most UI output this is fine; for streaming code that requires every intermediate state it is not.

**Failure Mode 5: `cleanup()` called mid-write during session switch.**

`TerminalStreamManager.cleanup()` calls `redrawThrottler.cleanup()`, which flushes `pendingRedraw`. But the flush calls `handleProcessedOutput()`, which may call `enqueueWrite()`, which starts an async Promise chain. If the terminal is being disposed concurrently, the async write callbacks fire on a disposed terminal, throwing errors that are silently ignored.

### Current Issues with EscapeSequenceParser

The `EscapeSequenceParser.isCompleteEscapeSequence()` has a coverage gap: it handles CSI (`ESC[`), OSC (`ESC]`), simple 2-char escapes, and C1 codes, but does NOT handle:
- DCS sequences (`ESC P ... ST`) — used by some terminal multiplexers
- PM/APC sequences (`ESC ^ ... ST`, `ESC _ ... ST`) — used by Sixel graphics and some SSH implementations
- `ESC \` (String Terminator) as a standalone sequence

If these appear in the stream (unlikely for Claude but possible from other programs), the parser will pass them through without buffering at boundaries, potentially causing partial-sequence writes.

---

## Risk Matrix

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| FitAddon returns undefined on hidden terminal init | High (certain if `open()` called while hidden) | High (wrong terminal dimensions, mis-sized PTY) | Never call `open()` while hidden; use `visibility:hidden` or off-screen container |
| WebGL context limit exceeded (10+ sessions) | Medium (certain above ~16 sessions; likely above 8 on integrated graphics) | High (silent blank terminal, no fallback) | Subscribe to `onContextLoss`, use DOM renderer for background terminals |
| Memory growth (persistent instances at 20+ sessions) | Medium | Medium (page slowdown, potential OOM on mobile) | LRU eviction pool; evict to backend scrollback, not discard |
| Old session WebSocket output written to new session terminal | Medium (depends on disconnect timing) | Medium (garbled output visible briefly) | Session-stamp all output; discard stale session output |
| capture-pane returns wrong content during alternate screen | High (always when Claude TUI is active) | High (blank or stale snapshot on reconnect) | Force TUI redraw + wait for PTY settle before capture; use PTY stream for visible-screen restore instead of capture-pane |
| ±1 nudge race (capture before redraw completes) | Medium | Medium (stale snapshot, corrects on next output) | Add PTY activity monitoring; wait for output quiescence after resize |
| RedrawThrottler drops intermediate state | Low (by design, usually acceptable) | Low (cosmetic only in most cases) | No change needed; document as known behavior |
| EscapeSequenceParser misses DCS/APC sequences | Low (not used by Claude) | Low (partial sequence write, xterm.js buffers internally) | Low priority; expand parser if DCS sequences observed |
| ResizeObserver fires on background terminal, sends spurious resize | Medium (when using persistent CSS-hidden terminals) | Medium (PTY gets wrong size when not active) | Gate backend resize calls on "session is active" predicate |
| Rapid switch race: writeInitialContent continues mid-switch | Medium | Low (background terminal gets extra writes, harmless if persistent) | Add cancellation token to writeInitialContent; avoid clearing terminal mid-switch |

---

## References

- xterm.js FitAddon source: `addon-fit/src/FitAddon.ts` — `proposeDimensions()` returns `undefined` on zero cell dimensions
- xterm.js WebglAddon source: `addon-webgl/src/WebglAddon.ts` — `onContextLoss` event, no auto-fallback
- xterm.js WebglRenderer source: `addon-webgl/src/WebglRenderer.ts` — 3-second context restoration timeout before `onContextLoss` fires
- xterm.js issue #4379: [Support dozens terminals on single page](https://github.com/xtermjs/xterm.js/issues/4379) — WebGL context limits, scissor rendering proposal
- xterm.js issue #4338: [Fit dimensions NaN](https://github.com/xtermjs/xterm.js/issues/4338) — FitAddon NaN when render service dimensions undefined (fixed in 5.1.0)
- xterm.js issue #4841: [FitAddon resizes incorrectly](https://github.com/xtermjs/xterm.js/issues/4841) — fit leaves unused space in some browsers
- xterm.js issue #4804: [WebGL context lost](https://github.com/xtermjs/xterm.js/issues/4804) — context loss from GPU driver failure; `webglcontextrestored` fires but `RequestRedrawViewport` does not take effect
- tmux man page: `capture-pane` — "If the alternate screen is used, history is not accessible"
- Project codebase: `/server/services/connectrpc_websocket.go` — `sanitizeInitialContent()`, `±1 nudge`, cursor-positioning strip regex
- Project codebase: `/web-app/src/lib/terminal/TerminalStreamManager.ts` — `RedrawThrottler`, `EscapeSequenceParser` usage
- Project codebase: `/web-app/src/components/sessions/XtermTerminal.tsx` — WebGL try/catch without `onContextLoss` subscription, `scrollback={5000}`
- Project codebase: `/web-app/src/components/sessions/TerminalOutput.tsx` — session switch flow, `pendingConnectAfterDisconnectRef` pattern

---

## Addendum: Claude Code Issue #36582 + CLAUDE_CODE_NO_FLICKER Analysis

*Added 2026-04-09 from anthropics/claude-code issues*

### Root Cause of Scroll-to-Top Jank (Confirmed by Community)

Claude Code's TUI sends `\x1b[2J\x1b[3J` (ED2 = clear screen + ED3 = erase scrollback) during every streaming repaint. The ED3 sequence (`\x1b[3J`) resets xterm.js's `viewportY` to 0, which is what causes the scroll-to-top jump. Confirmed by logging xterm's `onScroll` event with stack traces — every viewport jump traces back to `eraseInDisplay` in xterm's `InputHandler.ts`.

**Immediate fix available** — strip ED3 when paired with ED2, so standalone `clear` still works:
```js
const filtered = data.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
terminal.write(filtered);
```

Our `EscapeSequenceParser` does NOT currently do this. This filter should be added to `TerminalStreamManager.write()` or `EscapeSequenceParser.processChunk()`.

**Upgrading from 5.x to `@xterm/xterm` 5.5.0** reduced frequency but didn't eliminate it. Full elimination requires either the ED3 filter or xterm.js 6.0.0 with native DEC 2026 support.

### CLAUDE_CODE_NO_FLICKER=1

Introduced in Claude Code v2.1.89 as "flicker-free alt-screen rendering with virtualized scrollback". **Do not rely on this.** Status as of 2026-04-09:

- Enabled **by default** in v2.1.89 (no opt-in required)
- **Destroys terminal scrollback** — re-renders entire conversation when output exceeds screen height, so only ~2 pages of scrollback survive (issue #41965)
- Numerous open bugs: broken table copy (#44893), broken arrow nav (#44538), broken text selection in iTerm2 (#43767), garbled Japanese text (#42406), broken Ctrl+J in tmux (#42821), broken external editor (#42606)
- Workaround: `CLAUDE_CODE_NO_FLICKER=0` to disable

**Implication for Stapler Squad**: Do NOT assume `CLAUDE_CODE_NO_FLICKER` is set or reliable. Our terminal must handle Claude's raw TUI output including the ED2+ED3 pattern without depending on Claude to fix its output.

### Issue #37283 — DECSET 2026 Request

Community has filed a request for Claude Code to emit `\x1b[?2026h`/`\x1b[?2026l` (synchronized output) around render cycles. Status: open, no Anthropic response. This confirms: Claude Code does NOT reliably emit DEC mode 2026 sequences today. Our xterm.js upgrade to 6.0.0 adds native support for when it does, but the ED3 filter is the immediate fix.

### Action Items from This Analysis

1. **Immediate**: Add `\x1b[2J\x1b[3J` → `\x1b[2J` filter to `EscapeSequenceParser` or `TerminalStreamManager`
2. **Short-term**: Upgrade to xterm.js 6.0.0 for native DEC 2026 + alt-buffer scroll fixes (#5411, #5390, #5127)
3. **Do not rely on**: `CLAUDE_CODE_NO_FLICKER` env var — too buggy and actively destroys scrollback
