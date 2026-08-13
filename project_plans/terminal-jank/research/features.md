# Research: Features Survey - Web Terminal Multi-Session Switching

Status: Complete | Phase: 2 - Research
Date: 2026-04-09

## Summary

All mature terminal applications — desktop and web — keep terminal instances alive while switching tabs; none destroy and recreate. ttyd and GoTTY are single-session tools that delegate multi-session management to tmux; they offer no native session-switching pattern. tmux implements window switching by setting a `CLIENT_ALLREDRAWFLAGS` flag that triggers a visible-screen-only redraw (no scrollback replay) to the client terminal. The xterm.js `@xterm/addon-serialize` addon plus the `SerializeAddon.serialize()` method provides the exact primitive Stapler Squad needs to snapshot and restore terminal state without a full scrollback replay.

---

## How ttyd handles multi-session

ttyd is a single-session web terminal: it starts one command process per connecting client. Each connection gets its own WebSocket and PTY. The client-side React/Preact component (`html/src/components/terminal/index.tsx`) creates one `Xterm` instance per mount, connects in `componentDidMount`, and disposes in `componentWillUnmount`. There is no concept of "switching" between sessions — each browser tab or URL path gets exactly one terminal.

For sharing one terminal across multiple browsers, the ttyd documentation explicitly says to use tmux or GNU Screen rather than building multi-session logic into ttyd itself.

**Key architectural choice:** ttyd exposes a `--max-clients` flag (default 0 = unlimited) and `--once` flag (exit after first disconnect), confirming it is designed for single-session use. The `--exit-no-conn` flag exits when all clients disconnect, reinforcing that "session" in ttyd means "process lifetime."

**Implication for Stapler Squad:** ttyd offers no prior art for multi-session switching. Its architecture is the wrong layer — it is more analogous to Stapler Squad's per-session WebSocket stream than to the session-switching UI.

---

## How Wetty/GoTTY handle switching

**GoTTY (yudai/gotty and sorenisanerd/gotty):** Same architecture as ttyd. GoTTY describes itself as "a WebSocket server that relays output TTY to clients." One process per connection. Multi-user sharing is explicitly delegated to tmux: "use terminal multiplexers sharing single process multiple clients." GoTTY offers no session switching UI.

**Wetty (butlerx/wetty):** Also a single-session per-connection architecture. Uses xterm.js + WebSocket. No session-switching concept — it is a web SSH/local shell terminal, not a multiplexer.

**Pattern across all web terminal tools:** All three tools (ttyd, GoTTY, Wetty) are single-session proxies. They assume the terminal multiplexer (tmux, screen) handles session management. None of them implement session switching at the frontend layer because their architecture has no concept of multiple simultaneous sessions per page.

**Implication for Stapler Squad:** Stapler Squad's problem is the one ttyd/GoTTY explicitly defer to tmux. The solution must be designed at Stapler Squad's own layer, not borrowed from these tools.

---

## How tmux implements window switching

This is the most directly applicable prior art. Tmux's window-switch sequence from source code analysis:

### Command flow

```
user runs: tmux select-window -t :2
  → cmd_select_window_exec() in cmd-select-window.c
  → session_select(s, wl->idx)        // updates s->curw to new window
  → server_redraw_session(s)           // in server-fn.c
  → iterates all clients of session
  → server_redraw_client(c)
  → c->flags |= CLIENT_ALLREDRAWFLAGS  // sets redraw flag
  → next event loop tick: server_client_check_redraw(c)
  → tty_redraw_pane() / tty_draw_pane() for each pane in the NEW window
```

### What gets sent to the client terminal

**Visible screen only.** The `CLIENT_ALLREDRAWFLAGS` triggers a full repaint of the current window's visible viewport (the `struct screen` — rows × cols of cells). This is the equivalent of `tmux capture-pane` without `-S` (start-line) or `-E` (end-line), which defaults to the visible screen only.

The `cmd-capture-pane.c` source confirms: `capture-pane` without `-S/-E` flags captures from line 0 of the visible screen to the last visible line (`gd->hsize + gd->sy - 1`). Passing `-S -` (start from the beginning of history) is what captures the full scrollback.

**No scrollback is sent during a window switch.** The tmux server sends only the visible rows × cols cells of the new window's current screen state. The client terminal (your local iTerm2 or Terminal.app) receives a sequence of escape codes that paint those cells. Scrollback history in the *new* window's buffer is not transmitted — it is only accessible if the user explicitly scrolls up in tmux's copy mode.

### screen.c: alternate screen handling

`screen_alternate_on()` / `screen_alternate_off()` in `screen.c` manage tmux's internal alternate-screen state. When a window uses the alternate screen (TUI apps like vim, Claude's TUI), tmux maintains a `saved_grid` that preserves the main screen content. On `select-window`, tmux sends the correct screen state (alternate or main) for the new window.

### Key insight

tmux achieves instantaneous window switching because:
1. Every window's screen state (`struct screen`) is always maintained in memory server-side
2. On switch, only the viewport (~2-4 KB of escape codes for a typical 80×24 terminal) is transmitted
3. No scrollback is read or transmitted during a switch

This is the "visible-screen-only restore" pattern that Stapler Squad's requirements call for.

---

## Visible-screen-only restore pattern

### tmux capture-pane for the visible viewport

The tmux command to get just the visible screen (no scrollback) is:

```bash
# Capture only the visible viewport (no -S or -E flags = visible screen only)
tmux capture-pane -t <pane-id> -p

# Get the visible screen WITH escape sequences (colors, cursor position):
tmux capture-pane -t <pane-id> -p -e

# After a resize, get the post-resize visible screen:
tmux resize-window -t <session> -x <cols> -y <rows>
tmux capture-pane -t <pane-id> -p -e
```

The `-S` flag specifies start line (negative = lines before visible, `- ` = beginning of history). Without `-S`, capture-pane only returns the visible viewport. This is exactly the "visible-screen snapshot" Stapler Squad's requirements describe.

### WaveTerm's serialize-based approach

WaveTerm (`wavetermdev/waveterm`) uses the `@xterm/addon-serialize` SerializeAddon in `termwrap.ts` to capture terminal state:

```typescript
processAndCacheData() // serializes via serializeAddon.serialize()
resyncController()    // sends size + pty offset back to backend
```

On reconnect, WaveTerm restores from the serialized snapshot rather than replaying the full PTY stream. The serialized format encodes all cell content + colors + cursor position as a sequence of escape codes that can be written directly into a new or existing xterm.js instance.

### VS Code's CSS display:none approach

VS Code (`microsoft/vscode`) keeps all terminal xterm.js instances alive permanently and toggles visibility with CSS `display: none`:

```typescript
// In terminalGroup.ts:
setVisible(visible: boolean): void {
    this._visible = visible;
    if (this._groupElement) {
        this._groupElement.style.display = visible ? '' : 'none';
    }
    this.terminalInstances.forEach(i => i.setVisible(visible));
}
```

The hidden terminal's xterm.js `Terminal` instance:
- Continues to receive PTY output (WebSocket stays open)
- xterm.js `RenderService` detects `display:none` via `IntersectionObserver` and sets `_isPaused = true`, skipping DOM renders
- Internal buffer state (all lines, cursor position) stays live and up to date
- When made visible, the `IntersectionObserver` fires, `_isPaused = false`, and xterm.js flushes any pending renders

This means VS Code tab switching is literally zero-cost: no snapshot, no replay, no WebSocket reconnect. The xterm.js buffer has been tracking all output the entire time.

### xterm.js IntersectionObserver behavior (from RenderService.ts)

```typescript
// xterm.js src/browser/services/RenderService.ts
private _handleIntersectionChange(entry: IntersectionObserverEntry): void {
    this._isPaused = entry.isIntersecting === undefined
        ? (entry.intersectionRatio === 0)
        : !entry.isIntersecting;
    // On becoming visible: flush paused resize task + refresh rows
}

refreshRows(start: number, end: number): void {
    if (this._isPaused) { return; }  // Skip render when hidden
    // ... normal render path
}
```

When the terminal container transitions from `display:none` to visible, `IntersectionObserver` fires, `_isPaused` becomes false, and xterm.js immediately renders the current buffer state. The user sees the terminal exactly as it was — no loading, no replay.

**Critical caveat for Stapler Squad:** If the xterm.js container is hidden via `display:none`, `ResizeObserver` does not fire (element has 0x0 dimensions). If the terminal was resized while hidden, the FitAddon will calculate wrong dimensions. VS Code works around this by calling `setVisible(true)` first, then triggering a resize.

---

## Terminal history infinite scroll approaches

### Desktop terminal approach (Alacritty, WezTerm, iTerm2)

Desktop terminals keep the entire scrollback buffer in memory as a circular buffer of `Line` objects (typically up to 10,000–100,000 lines). Rendering is lazy: only the visible viewport is rasterized. Scrolling is implemented by adjusting a viewport offset into the buffer — no data loading, just pointer arithmetic.

**Key property:** The full history is always in memory. There is no "lazy loading" because loading from disk would be too slow for interactive scrolling. iTerm2 and WezTerm use memory-mapped files for very large scrollbacks, but the visible lines are always hot in RAM.

### xterm.js scrollback buffer

xterm.js keeps scrollback in its internal `CircularList` buffer (in `src/common/CircularList.ts`). The `scrollback` option (default 1000) sets the maximum lines retained. Lines above the visible viewport are stored in the `normal` buffer; the active viewport is the current screen.

xterm.js does **not** implement lazy loading of scrollback from an external source. All content in the buffer was written via `terminal.write()` at some point. There is no "stream more history on scroll" primitive in xterm.js itself.

### The @xterm/addon-serialize approach for partial restore

The SerializeAddon supports `scrollback: number` option to serialize only the last N lines of scrollback:

```typescript
serializeAddon.serialize({ scrollback: 0 })  // visible viewport only
serializeAddon.serialize({ scrollback: 100 }) // viewport + last 100 lines
serializeAddon.serialize()                    // full buffer
```

This directly maps to a "visible-screen-only restore with lazy history" pattern:

1. On session switch: send only `serialize({ scrollback: 0 })` (visible screen)
2. Show terminal instantly (no loading spinner)
3. If user scrolls up past the visible viewport: request more history from backend
4. Write history above current viewport via `terminal.write()` as user scrolls

### Lazy scrollback on scroll: no existing web terminal implements this

No existing open-source web terminal (ttyd, GoTTY, Wetty, WaveTerm) implements true lazy-load scrollback where history is fetched on demand as the user scrolls. All existing tools either:
- Replay full history on connect (Stapler Squad's current approach)
- Keep the full buffer in memory always (VS Code, desktop terminals)
- Simply discard history (basic web terminals)

The pattern is **feasible** with xterm.js:
- xterm.js `onScroll` event fires when user scrolls
- `buffer.active.viewportY` gives current scroll position
- When `viewportY` reaches 0 (top of loaded content), inject older content via `terminal.write()`
- New content written to the top of the buffer would shift everything down (tested behavior in VS Code's xterm.js usage)

However, prepending to an xterm.js buffer is not a first-class operation. Content writes append to the bottom. To prepend history, you would need to serialize the current state, clear, write the history, then restore. This is the approach used by VS Code's terminal "load more history" feature.

### tmux scrollback as the authoritative source

Since Stapler Squad's sessions run in tmux, `tmux capture-pane -S - -E - -p -e` retrieves the full scrollback (potentially thousands of lines). For lazy loading:

1. Initial display: `tmux capture-pane -p -e` (visible screen only, ~2-4 KB)
2. User scrolls to top of loaded content: `tmux capture-pane -S <start> -E <end> -p -e` (chunk of history)
3. Inject above current viewport

This requires either the `tmux capture-pane` range parameters or Stapler Squad's existing scrollback buffer (which already supports `GetScrollback(sessionID, fromSeq, limit)`).

---

## Key Insights for Stapler Squad

### 1. The VS Code pattern is the right model

Keep xterm.js instances alive, hidden via `display:none`. xterm.js's own `RenderService` pauses rendering automatically via `IntersectionObserver` when the element is hidden — this is zero-overhead for hidden terminals. WebSocket stays connected. PTY output continues to be buffered. On switch: unhide, trigger a resize if needed.

This matches the requirement "No loading overlay on session switch" exactly.

### 2. tmux's window-switch protocol is the right reconnect model

When a session has no live xterm.js instance (first time viewed, or if keep-alive is not implemented), use `tmux capture-pane -p -e` to get only the visible screen. Do NOT replay the full scrollback. The Go backend's `ResizePane → capture-pane` sequence should:
1. Resize the tmux pane to match the client's terminal size
2. Run `capture-pane -p -e` (visible viewport only, with escape sequences)
3. Send that as `currentPaneResponse` — typically 2-4 KB, not thousands of lines

This replaces the current "replay last N lines of scrollback" approach.

### 3. @xterm/addon-serialize is the right snapshot primitive

For serializing visible state to restore on reconnect (e.g., if the WebSocket drops):

```typescript
import { SerializeAddon } from '@xterm/addon-serialize';
// serialize({ scrollback: 0 }) → visible screen only, ~2-4 KB
// serialize() → full scrollback, potentially large
```

WaveTerm already uses this pattern (`processAndCacheData()` + `resyncController()`).

### 4. Lazy scrollback is feasible but not essential for v1

The "infinite scroll" requirement can be deferred. V1 priority:
1. Keep xterm.js instances alive (`display:none`)
2. Switch to visible-screen-only restore on first view
3. Accept that scrollback history above the visible viewport requires a full load (acceptable since it's user-initiated)

Lazy loading scrollback (load-on-scroll) is a V2 improvement that requires more complex buffer manipulation.

### 5. xterm.js ResizeObserver caveat requires explicit handling

When unhiding a `display:none` terminal, call `setVisible(true)` first (make it visible in the DOM), then trigger FitAddon resize. Calling FitAddon.fit() on a hidden element returns wrong dimensions (0×0 or cached stale dimensions).

---

## References

- tmux `cmd-select-window.c`: https://github.com/tmux/tmux/blob/master/cmd-select-window.c
- tmux `server-fn.c` (redraw chain): https://github.com/tmux/tmux/blob/master/server-fn.c
- tmux `cmd-capture-pane.c` (visible vs. history): https://github.com/tmux/tmux/blob/master/cmd-capture-pane.c
- tmux `screen.c` (alternate screen, grid management): https://github.com/tmux/tmux/blob/master/screen.c
- VS Code `terminalGroup.ts` (setVisible + display:none): https://github.com/microsoft/vscode/blob/main/src/vs/workbench/contrib/terminal/browser/terminalGroup.ts
- xterm.js `RenderService.ts` (IntersectionObserver pause/resume): https://github.com/xtermjs/xterm.js/blob/master/src/browser/services/RenderService.ts
- xterm.js `@xterm/addon-serialize` (serialize visible screen): https://github.com/xtermjs/xterm.js/tree/master/addons/addon-serialize
- WaveTerm `termwrap.ts` (serialize + resync pattern): https://github.com/wavetermdev/waveterm/blob/main/frontend/app/view/term/termwrap.ts
- ttyd architecture: https://github.com/tsl0922/ttyd
- GoTTY architecture: https://github.com/sorenisanerd/gotty
- Wetty architecture: https://github.com/butlerx/wetty
- Tabby `tabBody.component.ts` (Angular CSS class active): https://github.com/Eugeny/tabby/blob/master/tabby-core/src/components/tabBody.component.ts
