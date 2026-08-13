# Research: Architecture for Terminal Jank Elimination

## Summary

The root cause of session-switch jank is that `TerminalOutput` is re-mounted on every session change, destroying and recreating the xterm.js `Terminal` instance and its WebSocket connection, then replaying scrollback into a blank screen. The fix is a terminal instance pool backed by CSS visibility toggling that keeps N instances alive across switches, combined with a visible-screen-only reconnect protocol that delivers only the live pane snapshot (not full scrollback) on background-to-foreground transitions. Lazy scrollback loading above the viewport can be added as a separate, decoupled concern once the core pool is in place.

---

## Current Architecture Analysis

### What happens on every session switch

```
1. Parent (SessionDetail) renders new sessionId prop
2. TerminalOutput mounts fresh:
   - isLoadingInitialContent = true → loading overlay shown
   - streamManagerRef.current = null
   - hasInitiatedConnectionRef = false
3. XtermTerminal mounts:
   - new Terminal(...) created (WebGL context, canvas, ResizeObserver)
   - double-RAF + 100ms timeout before fitAddon.fit()
   - onResize fires → TerminalOutput.handleTerminalResize
4. handleTerminalResize triggers connect(cols, rows) after 50ms stability timeout
5. useTerminalStream.connect():
   - new AbortController + MessageQueue
   - sends CurrentPaneRequest { lines: 50, targetCols, targetRows }
   - awaits first server message → setIsConnected(true)
6. server streamViaControlMode():
   - sends resize (±1 nudge) to tmux — waits 50ms + 200ms for SIGWINCH
   - runs tmux capture-pane -e (tmux execs shell command, reads pane buffer)
   - sends clear+home + sanitized snapshot as first Output message
7. handleScrollbackReceived → manager.writeInitialContent():
   - terminal.clear()
   - chunked write of snapshot (16KB chunks)
   - multiple scrollToBottom() calls (0ms, 10ms, 100ms, 500ms)
8. setIsLoadingInitialContent(false) → loading overlay hides
```

### Key bottlenecks (with measured latencies)

| Phase | Bottleneck | Typical cost |
|---|---|---|
| xterm init | WebGL context + double-RAF | 16–32ms + ~100ms |
| Size stability wait | 50ms debounce timer | 50ms |
| WebSocket connect | TCP handshake + ConnectRPC handshake | 5–20ms |
| tmux resize + SIGWINCH wait | Hard-coded 50ms + 200ms sleep in Go | 250ms |
| capture-pane exec | Shell invocation + pane read | 20–80ms |
| writeInitialContent | clear + chunked write | 50–200ms |
| scrollToBottom delays | 4× setTimeout | up to 500ms |
| **Total** | | **500–1200ms** |

### The structural problem

`TerminalOutput` couples three independent concerns into a single component lifecycle:
1. xterm.js `Terminal` instance (canvas, WebGL, DOM)
2. WebSocket connection + streaming state
3. Loading/error UI overlay

Unmounting destroys all three together. Even with cached dimensions (which eliminates the 50ms debounce), the tmux resize+sleep (250ms) and writeInitialContent (up to 200ms) remain because they are inherent to the reconnect path, not to the DOM teardown.

---

## Option A: Terminal Instance Pool (CSS hide/show)

### Design

A `TerminalPoolProvider` React context manages a fixed pool of N `Terminal` instances. Each instance is mounted once into a hidden `div` container and never unmounted. On session switch, the previously active terminal is CSS-hidden and the newly selected one is CSS-shown.

```
TerminalPoolProvider (React Context)
  ├── pool: Map<sessionId, PoolEntry>
  ├── maxSize: 5 (LRU eviction)
  └── hidden container div (position: absolute, visibility: hidden, pointer-events: none)
      ├── TerminalSlot[session-A]  ← currently visible
      ├── TerminalSlot[session-B]  ← hidden
      └── TerminalSlot[session-C]  ← hidden

PoolEntry {
  sessionId: string
  terminal: Terminal           // xterm.js instance, never disposed
  streamManager: TerminalStreamManager
  connection: { isConnected, connect, disconnect, ... }  // from useTerminalStream
  lastAccessed: number         // for LRU eviction
  state: 'fresh' | 'connecting' | 'live' | 'backgrounded'
}
```

**Activation flow on session switch (A→B):**
1. `pool.setActive(sessionB)` called
2. `session-A` slot: `visibility: hidden` (or `display: none`, see CSS section)
3. `session-B` slot: already mounted; `visibility: visible`
4. `session-B` connection is already live → user sees terminal instantly
5. No reconnect, no clear, no scrollToBottom needed

**Creation flow (first view of session-C):**
1. Allocate slot in hidden container
2. Mount `XtermTerminal` in hidden div → full init sequence happens invisibly
3. Connect WebSocket, receive snapshot, write initial content
4. When user navigates to session-C: just unhide → instant display

**LRU eviction:**
- Pool capped at N=5 (configurable; each Terminal is ~2–8MB with WebGL)
- On eviction: disconnect WebSocket, dispose Terminal, remove from pool
- Next visit to evicted session restarts the full init sequence (first-visit cost, not switch cost)

### CSS hide/show strategy

Three options, each with different tradeoffs:

| Strategy | Mechanism | ResizeObserver fires? | WebGL context preserved? | Cursor/focus safe? |
|---|---|---|---|---|
| `display: none` | Layout removed | No (container has 0 size) | Yes | Yes (no focus) |
| `visibility: hidden` | Layout kept, not rendered | Yes (has dimensions) | Yes | Yes |
| `position: absolute; left: -9999px` | Off-screen layout | Yes (has dimensions) | Yes | Problematic (focus) |

**Recommendation: `visibility: hidden` with `pointer-events: none`**

- Preserves layout dimensions → `FitAddon.proposeDimensions()` returns correct values when unhiding
- ResizeObserver fires correctly when container resizes while hidden
- No focus leakage (pointer-events: none)
- WebGL context is never lost (no GPU context limit issues since context is never released)

Key risk with `display: none`: xterm.js `FitAddon` measures container dimensions to calculate cols/rows. If the container is `display: none`, dimensions are 0×0 and the terminal will resize to 0 columns on unhide. Mitigation: store last known dimensions and call `fit()` only after the unhide frame.

**Portal rendering:** The hidden container should be rendered via a React portal into `document.body` (not inside the session detail panel) to prevent the pool from being unmounted when the user navigates away from any session. This is the most important structural change.

### Implementation sketch

```tsx
// TerminalPool.tsx
const TerminalPoolContext = createContext<TerminalPoolAPI>(null!);

export function TerminalPoolProvider({ children, maxSize = 5 }) {
  const poolRef = useRef<Map<string, PoolEntry>>(new Map());
  const [activeId, setActiveId] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const activate = useCallback((sessionId: string, baseUrl: string) => {
    if (!poolRef.current.has(sessionId)) {
      // LRU eviction if at capacity
      if (poolRef.current.size >= maxSize) evictLRU(poolRef.current);
      // Create new slot (mounts XtermTerminal + starts connection)
      allocateSlot(poolRef.current, sessionId, baseUrl, containerRef.current!);
    }
    poolRef.current.get(sessionId)!.lastAccessed = Date.now();
    setActiveId(sessionId);
  }, [maxSize]);

  return (
    <TerminalPoolContext.Provider value={{ activate, pool: poolRef.current, activeId }}>
      {children}
      {createPortal(
        <div ref={containerRef} style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, pointerEvents: 'none' }}>
          {Array.from(poolRef.current.entries()).map(([id, entry]) => (
            <div
              key={id}
              style={{
                position: 'absolute', inset: 0,
                visibility: id === activeId ? 'visible' : 'hidden',
                pointerEvents: id === activeId ? 'auto' : 'none',
              }}
            >
              <TerminalSlot entry={entry} />
            </div>
          ))}
        </div>,
        document.body
      )}
    </TerminalPoolContext.Provider>
  );
}
```

### Tradeoffs

| Pro | Con |
|---|---|
| Zero-latency switch for warm sessions | Pool overhead: N×(WebGL context + WebSocket) always alive |
| Terminal state preserved perfectly | More complex component architecture (portal + context) |
| No tmux resize needed on switch | Max 5 sessions "warm" (configurable) |
| Scroll position preserved exactly | CSS z-index management required |
| Solves cursor/corruption by never resetting | FitAddon dimension edge case on unhide |

---

## Option B: Reconnect with Visible-Screen Snapshot

### Design

Keep the mount/unmount model but eliminate all artificial delays. On switch, immediately connect and request only the **visible screen** from tmux (rows×cols cells), not scrollback. The server sends the snapshot within one round trip; the client writes it without clear+replay jank.

**Protocol changes needed:**

**Client (`useTerminalStream.ts`):**
- `CurrentPaneRequest` already has `targetCols`, `targetRows`, and `lines` fields
- Change `lines` default from 50 to the actual terminal rows (e.g., 40–50 for a typical window)
- Remove the `scrollbackLines: 1000` parameter from `TerminalOutput` — it is unused for the initial connect path but confuses intent

**Server (`streamViaControlMode` in `connectrpc_websocket.go`):**

The current server does:
```go
// Resize tmux (±1 nudge) → sleep 50ms → sleep 200ms → capture-pane → send snapshot
```

The 250ms sleep is necessary because tmux needs time to relay SIGWINCH to the process and the process (Claude TUI) needs time to redraw at the new size. This delay cannot be eliminated for size-change cases. However, it can be eliminated when the terminal size has not changed:

```go
// Proposed: skip nudge+sleep if dimensions match current tmux pane size
currentCols, currentRows := instance.GetPaneSize()
if targetCols == currentCols && targetRows == currentRows {
    // No resize needed — capture immediately
    initialContent, _ = instance.CapturePaneContentRaw()
} else {
    // Size changed — nudge + sleep + capture
    // ... existing logic ...
}
```

Additionally, `CapturePaneContentRaw()` invokes `tmux capture-pane` as a subprocess. This can be pre-warmed: the server can maintain a per-session "last snapshot" cache, refreshed whenever the tmux control mode sends an `%output` event. When the client connects, the server can serve the cached snapshot immediately instead of invoking `tmux capture-pane`.

**Visible-screen-only protocol:**
```
Client connects → sends CurrentPaneRequest { targetCols, targetRows, lines: 0 (full visible) }
Server:
  1. Check if resize needed (compare targetCols/targetRows to current pane size)
  2. If no resize: serve cached snapshot immediately (0ms) or run capture-pane (~20ms)
  3. If resize: nudge + sleep + capture (250ms, same as today)
  4. Send Output message with clear+home + sanitized snapshot
  5. Subscribe to control mode updates, forward incrementally
Client: writes snapshot directly (no clear, no chunked replay jank)
```

**Why this is better than today:**
- Eliminates the 50ms stability timer (use cached dimensions)
- Eliminates `writeInitialContent`'s progressive chunking (snapshot is small — one visible screen is ~80×40 = 3200 chars + ANSI)
- Eliminates the 0/10/100/500ms scrollToBottom cascade (single scrollToBottom after one small write)

### Tradeoffs

| Pro | Con |
|---|---|
| Lower memory footprint (no idle WebSockets/terminals) | Still 250ms on first view after resize |
| Simple architecture (no pool, no portal) | Loses scroll position (always reset to bottom) |
| Handles unlimited sessions | Loses terminal content history above viewport |
| Easier to reason about connection state | Still a visible transition (even if faster) |

---

## Option C: Hybrid (Pool for recent + snapshot for cold sessions)

### Design

Combine options A and B: pool the last N (e.g., 3–5) recently-viewed sessions in warm state, serve snapshot-reconnect for sessions outside the pool.

```
Session switch to session-X:
  if pool.has(session-X):
    → instant CSS show (Option A path, ~0ms)
  else if pool.size < maxSize:
    → allocate slot, connect, receive snapshot, add to pool
  else:
    → evict LRU from pool
    → snapshot-reconnect for session-X (Option B path, ~50–300ms)
    → add session-X to pool as warm
```

**Pool lifecycle states:**

```
         [allocate]          [socket connected, snapshot received]
fresh ──────────────→ connecting ──────────────────────────────→ live
                                                                    │
                                   [CSS hidden, socket stays open]  │
                               ←─────────────────────────────────── │
                              backgrounded                          │
                                   │                                │
                         [CSS shown]│                               │
                               ──────────────────────────────────→  │
                                                               re-foregrounded
                                                              (instant, no reconnect)
                                   │
                         [LRU evicted]
                               ↓
                           [disposed]
                          (terminal.dispose(), WebSocket closed)
```

**WebSocket for backgrounded sessions:** Connections stay open for all pool members. This is the correct tradeoff because:
- Each open WebSocket (with no activity) costs ~2KB/s in keepalive pings (gorilla/websocket default)
- At N=5 sessions that is ~10KB/s — negligible
- The tmux control mode subscription on the server side is a goroutine waiting on a channel — ~8KB stack per session
- Closing and reopening costs 250ms+ (resize path) — far more expensive than keeping alive

**Server-side "backgrounded" optimization:**
When a client sends a resize message to a backgrounded session (it should not, but if it does), the server should not send SIGWINCH. Only the active/visible session should receive resize signals. The pool architecture naturally prevents this because hidden terminals do not trigger ResizeObserver events (with `visibility: hidden`).

---

## Lazy Scrollback Loading Design

This is decoupled from the pool architecture and can be added after the pool is implemented.

### Problem

xterm.js has a fixed-size `scrollback` buffer (currently 5000 lines in `XtermTerminal`). When the user scrolls to the top of the visible viewport, there may be older history in the server-side `ScrollbackManager` circular buffer that is not loaded.

### Design

**Trigger:** xterm.js `onScroll` event fires when the viewport scrolls. When `terminal.buffer.active.viewportY === 0` (user has scrolled to the top of the xterm buffer), trigger a scrollback request.

**Request:** `ScrollbackRequest { from_sequence: oldestLoadedSequence - 1, limit: 200 }` sent via the existing `requestScrollback()` in `useTerminalStream`.

**Response handling:**
The server already has `ScrollbackResponse { chunks, hasMore, oldestSequence, newestSequence, totalLines }`. The client needs to prepend the received chunks to the xterm buffer.

**Prepend challenge:** xterm.js does not support prepending to the scrollback buffer. Data can only be written at the current bottom. The standard pattern is:
1. Save current viewport scroll position (lines from bottom: `buffer.normal.length - buffer.active.viewportY - terminal.rows`)
2. Call `terminal.clear()` (clears scrollback and resets position)
3. Write old scrollback chunks first
4. Write current buffer content (re-snapshot from server or cached locally)
5. Restore scroll position

This is disruptive and jank-inducing if done naively. **Better approach:** xterm.js `IBufferNamespace.normal` is read-only but xterm does support `Terminal.write()` with arbitrary positioning. For historical prepend, the least-disruptive approach is:

1. Do NOT call `terminal.clear()`
2. Use xterm.js `ITerminalOptions.scrollback` to temporarily double the buffer (this appends to `normal` buffer)
3. Write old chunks at the beginning via DEC save/restore cursor sequences to reposition before writing
4. This is fragile; a more robust option is a **virtual scrollback layer**: keep old scrollback in a separate JS array and render it into a DOM `<pre>` element above the xterm canvas, scrolled in sync

**Recommended pattern: two-zone scrollback**
- Zone 1 (DOM `<pre>`): historical scrollback before the current xterm buffer; rendered as styled HTML; responds to wheel events
- Zone 2 (xterm.js canvas): live terminal with recent N lines + current screen
- A custom scroll container controls which zone is visible; when Zone 1 is scrolled to bottom, Zone 2 takes over

**API changes needed:**

Server (`session/scrollback/manager.go`):
- `GetScrollback(sessionID, fromSeq, limit)` already exists — no change needed
- The `ScrollbackResponse` proto is already defined — no change needed

Client (`useTerminalStream.ts`):
- `requestScrollback(fromSequence, limit)` already exists
- `onScrollbackReceived` callback already delivered via hook
- `TerminalOutput.tsx` currently ignores responses with `metadata` (historical scrollback) — line 158-162: "Reject historical scrollback requests"
- This rejection needs to be reversed; the callback needs to prepend to the two-zone view

**Component changes:**
```
TerminalOutput
  └── ScrollbackContainer (new)
        ├── HistoricalScrollback (new DOM component, renders old chunks)
        └── XtermTerminal (existing, only current N lines)
```

---

## WebSocket Lifecycle Recommendation

**Recommendation: Keep connections open for all pool members (up to N=5).**

### Analysis

**Cost of a backgrounded open WebSocket:**
- Go server: 1 goroutine per streaming direction (2 goroutines per session), ~8KB stack each = ~16KB per session
- Control mode tmux subscription: 1 goroutine blocked on channel read
- Network: gorilla/websocket default ping interval 60s; ~100 bytes/ping = ~1.7 bytes/sec
- At 5 sessions: ~80KB memory, ~9 bytes/sec

**Cost of reconnect on switch:**
- Best case (no resize needed): ~50–100ms (WebSocket handshake + snapshot)
- With resize: ~300–500ms

**Break-even:** For a user actively switching between sessions, keeping 5 connections open costs ~80KB memory. A single reconnect takes 300ms of subjective latency. Memory is not the bottleneck; latency is.

**Exception: sessions outside the pool.** Sessions evicted from the pool should have their WebSocket disconnected to avoid unbounded connection growth. The pool eviction path already handles `disconnect()`.

---

## Recommended Architecture

**Pursue Option C (Hybrid Pool + Snapshot)**, implemented in two phases:

### Phase 1: Terminal Instance Pool (high impact, moderate complexity)

1. Create `TerminalPoolProvider` as a React context/portal that manages 5 terminal slots
2. Each slot holds an `XtermTerminal` + `useTerminalStream` connection, never unmounted
3. CSS `visibility: hidden` + `pointer-events: none` for inactive slots
4. Portal into `document.body` to survive navigation changes
5. LRU eviction at N=5 with full cleanup (dispose terminal, disconnect WebSocket)
6. `TerminalOutput` becomes a thin wrapper that calls `pool.activate(sessionId)` instead of managing its own terminal instance
7. Remove the loading overlay entirely for warm sessions; keep it only for cold (first-view) sessions

This alone eliminates the session-switch jank for the last 5 sessions — which covers 99% of switching behavior in practice.

### Phase 2: Faster Cold Reconnect (lower impact, requires Go changes)

1. Server: cache per-session visible screen snapshot (updated on each control-mode `%output` event)
2. Server: skip resize+sleep when `targetCols == currentCols && targetRows == currentRows`
3. Client: remove the 50ms stability timer entirely (use cached dimensions from `TerminalDimensionCache`)
4. Client: remove the 4× `scrollToBottom` cascade — single call after snapshot write

This reduces cold-reconnect time from ~500ms to ~50–100ms for warm-cache hits.

### Phase 3: Lazy Scrollback (independent, deferred)

Implement the two-zone scrollback pattern as a separate component after Phase 1 is stable. This does not block jank elimination.

---

## References

- xterm.js FitAddon behavior with hidden containers: https://github.com/xtermjs/xterm.js/issues/3564
- xterm.js flow control guide (watermark pattern already implemented): https://xtermjs.org/docs/guides/flowcontrol/
- Mosh paper (source of SSP/diff approach already in the proto): https://mosh.org/mosh-paper.pdf
- tmux control mode documentation: https://github.com/tmux/tmux/wiki/Control-Mode
- React portal pattern for persistent UI: https://react.dev/reference/react-dom/createPortal
- WebGL context limits (browser enforces 16 contexts per page): https://developer.mozilla.org/en-US/docs/Web/API/WebGL_API/By_example/Detect_WebGL
