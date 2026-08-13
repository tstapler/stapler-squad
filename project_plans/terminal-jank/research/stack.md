# Research: Stack Options for Terminal Jank Elimination

Status: Complete | Phase: 2 - Research
Created: 2026-04-09

## Summary

The dominant and correct approach is to keep xterm.js `Terminal` instances alive in a React context pool and toggle visibility with `visibility: hidden` (not `display: none`) when not active. VS Code uses a structural equivalent — it detaches and reattaches the xterm.js DOM wrapper element while keeping the `Terminal` object alive, preserving buffer and scroll state. WebGL contexts survive CSS visibility toggling but the browser hard cap of ~16 contexts per page means a pool capped at 6–8 sessions is required for safety. The alternative — reconnect with a visible-screen snapshot via `tmux capture-pane` — eliminates jank only for first-visit connections, not for session switches once a pool is in place. Infinite-scroll lazy loading of historical scrollback into xterm.js is achievable via the `scrollback.manager.go` API but requires custom virtual buffer logic not natively provided by xterm.js.

---

## Option 1: Keep-alive xterm.js instances (CSS hide/show)

### Mechanism

xterm.js `Terminal` instances are JavaScript objects. They are only destroyed when `.dispose()` is called. CSS visibility changes (`display: none` / `visibility: hidden`) do not affect the in-memory state of the `Terminal`, its internal scrollback buffer, or its cursor position. Writes to a hidden terminal's `.write()` method update the internal buffer regardless of visibility.

**What VS Code does (as confirmed by source code):**

VS Code keeps `Terminal` instances alive permanently and uses a `detachFromElement()` / `attachToElement()` pattern. The `_wrapperElement` (containing the xterm canvases) is removed from and re-inserted into the DOM without disposing the `Terminal` object. On reattach, VS Code calls `xterm.raw.open(element)` and `xterm.refresh()` to resynchronize with the new container. This is the production-proven reference implementation for multi-terminal management.

### Constraints

**1. terminal.open() must be called with a visible container.**

`terminal.open(element)` triggers the renderer initialization, which calls `charSizeService.measure()` to compute cell pixel dimensions. If the element is hidden (`display: none`) or has zero computed dimensions, cell measurement fails. Cell dimensions will be 0 or undefined, and all subsequent `FitAddon.proposeDimensions()` calls return the minimum (2 cols × 1 row) until the renderer is reinitialized. The solution is to call `open()` once in a temporarily-visible or off-screen-but-layout-visible container (e.g., `position: absolute; visibility: hidden; width: <real px>; height: <real px>`), then hide it.

**2. FitAddon fails on display:none containers.**

`FitAddon.proposeDimensions()` divides container pixel dimensions by cell pixel dimensions. With `display: none`, container reports 0×0 and the result is `undefined` or NaN. With `visibility: hidden`, layout is still computed and the container reports real dimensions — FitAddon works correctly. This is the primary reason to prefer `visibility: hidden` over `display: none` for background terminals.

**3. WebGL context survival.**

Browsers enforce a per-page WebGL context limit. Chrome and Firefox typically allow ~16 simultaneous contexts; older integrated graphics may allow as few as 8. xterm.js creates one WebGL2 context per `Terminal` instance (one per canvas). CSS visibility changes do **not** release the context — the context survives as long as the `Terminal` exists. This means 10 background sessions consume 10 WebGL contexts indefinitely.

Key fact from xterm.js `WebglRenderer.ts`: the renderer registers `webglcontextlost` and `webglcontextrestored` listeners. If the browser evicts a context to make room for a new one, it fires `webglcontextlost`. The xterm.js handler waits 3 seconds for `webglcontextrestored`; if not restored, it fires `onContextLoss` — but **does not automatically fall back to a canvas renderer**. The terminal goes blank. Application code must subscribe to `webglAddon.onContextLoss` and fall back explicitly.

Practical safety limit with WebGL: pool size of 6–8 terminals maximum.

**4. ResizeObserver with hidden terminals.**

With `visibility: hidden`, ResizeObserver fires normally (the element has real layout). With `display: none`, ResizeObserver fires with zero contentRect and the current `XtermTerminal.tsx` threshold guard (`> 1` pixel) suppresses it. When the terminal is shown again, a new ResizeObserver event fires with real dimensions — this is the correct moment to call `fitAddon.fit()`.

**5. Focus leakage.**

A `visibility: hidden` element still participates in tab order and can receive synthetic focus. Add `tabIndex={-1}` and `pointer-events: none` to background terminals.

### Viability

High. This is the correct primary approach. The detailed implementation design is in `architecture.md` (Option A: Terminal Instance Pool). The pitfall details are in `pitfalls.md`.

---

## Option 2: Reconnect with visible-screen snapshot (tmux capture-pane)

### What tmux capture-pane provides

`tmux capture-pane -p -e [-J] -t <session>` outputs the current pane content as text with ANSI/VT100 escape sequences preserved (`-e`). The `-J` flag joins soft-wrapped lines. The output represents the *normal-screen* visible viewport (rows×cols grid), not the scrollback history (accessed via `-S`/`-E` flags with negative line numbers).

The codebase already has two variants:
- `CapturePaneContent()`: uses `-J` flag (joins wrapped lines), used for status detection
- `CapturePaneContentRaw()`: no `-J` flag (preserves cursor positioning sequences), used in the visible-screen handshake

### Critical limitation: alternate screen

`tmux capture-pane` **cannot capture alternate-screen content**. From the tmux man page: *"If -a is given, the alternate screen is used, and the history is not accessible."* Without `-a`, output is always from the normal screen. Claude Code runs its TUI in alternate-screen mode (`\x1b[?1049h`). When Claude's TUI is active, `capture-pane` returns the last normal-screen content (typically a blank screen or a shell prompt), not the live TUI.

The current codebase works around this with a `±1 nudge` resize (`targetCols-1` then `targetCols`) before capture. The resize sends SIGWINCH to the process, which causes Claude's TUI to re-draw. But the redraw output goes to the PTY stream (not to `capture-pane`). The code then waits 250ms (hardcoded `time.Sleep`) and calls `capture-pane` hoping the redraw has replayed into the pane buffer. This is inherently racy: the 250ms delay may not be sufficient for complex renders, and there is no synchronization between the TUI redraw and the capture.

### What capture-pane does and does not preserve

| Terminal state | Preserved by capture-pane -e? |
|---|---|
| Visible text content | Yes |
| ANSI SGR colors and attributes | Yes (with -e) |
| Cursor absolute position (ESC[n;mH) | Yes (with -e, no -J) |
| Alternate screen content | No — returns normal screen content |
| Scroll region (DECSTBM) | No |
| Terminal mode flags (?1049h etc.) | No |
| Scrollback history (above viewport) | No (requires -S/-E with negative numbers) |

### Current code behavior

The `connectrpc_websocket.go` function `streamViaControlMode()` (line ~399) calls `CapturePaneContentRaw()` after resizing tmux to match the client's terminal dimensions. The Go-side code at lines 958–989 handles the resize-before-capture flow including multiple SIGWINCH signals and the 250ms sleep. This is sent to the client as the first `currentPaneResponse` message, which the client replays into a freshly cleared xterm.js terminal.

### Viability as a standalone approach

Medium-low. The visible-screen snapshot eliminates the need to replay full scrollback, but the alternate-screen problem means Claude Code sessions will often show stale or blank content on reconnect. With a persistent terminal pool (Option 1), this approach becomes secondary: it is only needed for the first time a session is viewed (cold start), or when a session has been evicted from the LRU pool. With the pool in place, the reconnect path is rarely triggered.

### Improvement opportunity

Use a completion signal instead of a fixed sleep: after sending SIGWINCH, read from the PTY until no new bytes arrive for 100ms (or use the scrollback manager's sequence counter to detect when new output has settled). This makes the capture timing adaptive rather than fixed.

---

## Option 3: WebSocket keep-alive vs reconnect

### Current approach

Each session switch disconnects the WebSocket for the previous session and connects a new one for the next session. The disconnect is async (up to 1-second timeout). This adds latency and creates the race conditions documented in `pitfalls.md` (Race 1 and Race 2).

### Keep-alive (always-open WebSockets)

With a terminal instance pool, each pooled session maintains a persistent WebSocket connection. The Go backend's `streamViaControlMode()` already keeps the stream open indefinitely (it loops reading PTY output and writing to the ConnectRPC stream). The client side uses `useTerminalStream.connect()` / `disconnect()` which are currently called on every session switch.

Persistent WebSockets per session:
- Server resource cost: one goroutine per session (already the case in `streamViaControlMode`), plus the ConnectRPC streaming overhead
- Network cost: idle sessions generate no traffic (PTY output only flows when the process writes)
- Memory cost: each WebSocket has a read/write buffer (~64KB typical); at 10 sessions that is ~640KB — negligible
- Timeout risk: idle WebSockets may be terminated by proxies or load balancers. The existing ping/pong or keepalive in ConnectRPC WebSocket transport should handle this, but a configurable keepalive interval may be needed.

### Reconnect-with-snapshot (current approach, optimized)

If keep-alive is not feasible (e.g., resource constraints at scale), a fast reconnect path can reduce jank to the time of one roundtrip:
1. Client sends `CurrentPaneRequest` with terminal dimensions
2. Server resizes tmux, sends SIGWINCH, waits for output quiescence
3. Server sends only the visible screen (not full scrollback)
4. Client writes snapshot to xterm.js without clearing (uses `\x1b[H` + write, not `terminal.clear()`)

Step 4 is the key optimization: avoid `terminal.clear()` before writing the snapshot. Clear + write causes the visible blank-then-content flash. Writing at cursor home position without clear overlays existing content, which is visually less disruptive.

### Tradeoff summary

| Approach | Switch latency | Server resources | Complexity |
|---|---|---|---|
| Keep-alive (pool) | ~0ms | N goroutines always | Moderate |
| Reconnect + snapshot | 300–600ms | Goroutines only while active | Low (current) |
| Reconnect + fast snapshot | 50–150ms | Goroutines only while active | Moderate |

**Recommendation:** Pair the terminal pool (Option 1) with keep-alive WebSockets. Persistent connections are the natural complement to persistent xterm.js instances. The reconnect path becomes a fallback for cold-start and evicted sessions only.

---

## Option 4: Infinite scroll for scrollback history

### What xterm.js provides natively

xterm.js maintains a scrollback buffer internally, configured via the `scrollback` option (currently 5000 lines in `TerminalOutput.tsx`). This buffer is held entirely in memory. xterm.js does not provide virtual scrollback — all lines in the buffer are in memory simultaneously.

There is no xterm.js API for lazy-loading scrollback content. The `terminal.write()` call appends to the end of the buffer; there is no API to prepend content above the current viewport.

**The `buffer.active` property** exposes read-only access to buffer lines but no write path. The internal buffer is a doubly-linked circular structure with no insertion-at-head support in the public API.

### Approaches to lazy scrollback

**1. Prepend via terminal.write + repositioning (fragile)**

Write historical content before existing content by sending a cursor-save + cursor-to-top + write + cursor-restore sequence. This is fragile: it changes the terminal's visual state, may disrupt running processes' output, and does not correctly handle wraparound or alternate screen.

**2. Separate scrollback viewer (recommended)**

Render historical scrollback in a separate component above the live terminal. Options:
- A virtualized list (react-virtuoso, react-window) rendering plain text lines from the backend scrollback API
- A read-only xterm.js instance initialized with historical content
- A custom canvas-based text renderer

The backend `ScrollbackManager.GetScrollback()` and `GetRecentLines()` APIs already return historical entries by sequence number. The `requestScrollback` function in `useTerminalStream.ts` sends a `ScrollbackRequest` upstream, and the response arrives as a `scrollbackResponse` message. The client currently ignores scrollback responses with metadata (`handleScrollbackReceived` returns early if `metadata` is present — see `TerminalOutput.tsx` lines 158–163).

**3. Large initial scrollback (current workaround)**

The `scrollback={5000}` in `TerminalOutput.tsx` and `scrollbackLines: 1000` in `useTerminalStream` are the current "load a lot upfront" approach. This works for most sessions but has the jank problem of replaying 1000 lines into xterm.js on every switch.

### Viability

Implementing true infinite scroll for xterm.js is non-trivial because xterm.js does not support it natively. The recommended approach is a separate panel below/above the terminal that loads historical scrollback from the backend on demand, using the existing `requestScrollback` + `scrollbackResponse` protocol that is already implemented but unused on the client side.

---

## Option 5: Alternatives to xterm.js

### hterm (Google)

Used in Chrome's built-in terminal (Chromebook SSH). Pure DOM-based renderer, no WebGL/canvas. Slower rendering but no WebGL context constraints. Multi-instance is viable without the context limit problem. Last meaningful update 2023. Community activity low. Migration cost from xterm.js: high (different API, different addon ecosystem).

### terminal.js (various)

No single dominant alternative with active maintenance. Most "terminal.js" libraries are wrappers around xterm.js or toy implementations without VT100 completeness.

### Custom canvas renderer

Building a custom multi-session terminal renderer on a single shared canvas with scissor regions would solve the WebGL context limit problem definitively. Implementation cost: very high (4–8 weeks). Not warranted given that the LRU pool + context limit guard is sufficient.

### Recommendation

Stay with xterm.js. It is the only production-grade web terminal library with full VT220/xterm compatibility, active maintenance, WebGL acceleration, and an extensive addon ecosystem. The WebGL context limit is manageable with an LRU pool of 6–8 terminals.

---

## Recommendation

**Primary approach:** Persistent terminal instance pool (Option 1) with `visibility: hidden` toggling, capped at 6–8 sessions (LRU eviction), with keep-alive WebSocket connections for pooled sessions (Option 3 keep-alive variant). This eliminates session-switch jank entirely for warm sessions with zero new technology dependencies.

**Secondary approach:** For cold-start (first view of a session) and for evicted sessions, use the existing visible-screen snapshot via `tmux capture-pane` (Option 2) but replace the fixed `250ms` sleep with an output-quiescence detector. This reduces cold-start latency from 500–1200ms to 100–300ms.

**Deferred:** Infinite scrollback lazy loading (Option 4) using the separate viewer + existing `requestScrollback` protocol. Implement after the core pool is stable.

**Not recommended:** Alternative terminal libraries (Option 5). Migration cost too high, gains marginal.

---

## References

- xterm.js `WebglRenderer.ts` (onContextLoss handler): `github.com/xtermjs/xterm.js/blob/master/addons/addon-webgl/src/WebglRenderer.ts`
- xterm.js `FitAddon.ts` (proposeDimensions behavior on hidden containers): `github.com/xtermjs/xterm.js/blob/5.5.0/addons/addon-fit/src/FitAddon.ts`
- VS Code terminal instance lifecycle (attachToElement/detachFromElement): `github.com/microsoft/vscode/blob/main/src/vs/workbench/contrib/terminal/browser/terminalInstance.ts`
- tmux capture-pane man page (alternate screen limitation): `man.openbsd.org/tmux.1#capture-pane`
- xterm.js issue on WebGL context limits (Terminal7 multiplexer): xterm.js #4379
- Existing codebase: `session/tmux/tmux.go` lines 1415–1449 (CapturePaneContent, CapturePaneContentRaw)
- Existing codebase: `server/services/connectrpc_websocket.go` lines 395–1014 (capture-pane + reconnect flow)
- Existing codebase: `web-app/src/components/sessions/XtermTerminal.tsx` (WebGL init and ResizeObserver)
- Existing codebase: `web-app/src/components/sessions/TerminalOutput.tsx` (session switch flow)
- Existing codebase: `web-app/src/lib/hooks/useTerminalStream.ts` (connect/disconnect lifecycle)
