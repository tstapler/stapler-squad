# Implementation Plan: Terminal Jank Elimination

Status: Stories 1 & 2 Complete — Story 3 Pending
Created: 2026-04-09
Updated: 2026-04-20
Requirements: `project_plans/terminal-jank/requirements.md`
Research: `project_plans/terminal-jank/research/`
ADRs: `project_plans/terminal-jank/decisions/ADR-001..003`

---

## Overview

Eliminate session-switch jank in Stapler Squad's web terminal by keeping xterm.js instances alive in a pool, replacing hard-coded server sleeps with event-driven quiescence detection, and filtering destructive escape sequences from Claude Code's TUI output. The current 500-1200ms latency on every session switch drops to ~0ms for warm sessions and ~50-100ms for cold starts.

## Dependency Visualization

```
Story 1 (P0)                Story 2 (P1)                Story 3 (P1)                Story 4 (P3)
ED3 filter +                Quiescence detector +       Terminal instance pool       Line-granular
remove clear prefix +       snapshot cache              (CSS visibility pool)        scrollback API +
xterm 6.0 upgrade                                                                   lazy loading
  |                           |                           |                           |
  |  no deps                  |  no deps                  |  depends on S1            |  independent
  |                           |                           |  (ED3 filter needed       |  (after S3 ships)
  v                           v                           |   for pooled terms)       v
[SHIP]                      [SHIP]                        v                         [DEFER]
                                                        [SHIP]

S1 ──────────────> S3
S2 (independent) ─> S3 (nice to have, not blocking)
S4 (independent, deferred)
```

Stories 1 and 2 can proceed in parallel. Story 3 depends on Story 1 (pooled terminals accumulate scrollback, so the ED3 filter must be in place). Story 4 is independent and deferred.

---

## ✅ Story 1: Immediate Fixes -- ED3 Filter + Remove Clear Prefix + xterm 6.0 Upgrade

**Priority**: P0 (fixes active user-facing bugs)
**Status**: ✅ COMPLETE (2026-04-20)
**Scope**: ~1 day
**Risk**: Low -- isolated changes, no architectural impact

### Context

Claude Code's TUI emits `\x1b[2J\x1b[3J` (ED2 + ED3) on every streaming repaint. ED3 (`\x1b[3J`) erases xterm.js scrollback and resets `viewportY` to 0, causing the scroll-to-top jump users see on every render cycle. Separately, the Go server prepends `\x1b[2J\x1b[H` (clear screen + cursor home) to every cold-start snapshot, which clears the terminal unnecessarily since the terminal is already blank on first connect.

### Tasks

#### ✅ Task 1.1: Add ED3 filter to EscapeSequenceParser

**Files**: `web-app/src/lib/terminal/EscapeSequenceParser.ts`
**Estimate**: 1 hour

Add a line to `processChunk()` that strips ED3 when paired with ED2:

```typescript
// In processChunk(), after prepending partial sequence:
const fullData = this.partialSequence + data;
this.partialSequence = "";

// Strip ED3 (erase scrollback) when paired with ED2 (clear screen).
// Claude Code emits \x1b[2J\x1b[3J on every TUI repaint; ED3 resets
// xterm.js viewportY to 0, causing scroll-to-top jank.
const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```

The filter must run before the partial-sequence detection (on `fullData`), because the ED3 sequence is always complete (never split across chunks -- it follows ED2 in the same write).

**Acceptance criteria**:
- GIVEN a terminal receiving Claude TUI output with `\x1b[2J\x1b[3J` pairs
- WHEN the output passes through EscapeSequenceParser
- THEN `\x1b[3J` is stripped and only `\x1b[2J` reaches xterm.js
- AND standalone `\x1b[3J` (without preceding ED2) is preserved (user-initiated clear)
- AND unit tests cover: paired ED2+ED3, standalone ED3, ED2 alone, multiple pairs in one chunk

#### ✅ Task 1.2: Remove clear-screen prefix from cold-start snapshot

**Files**: `server/services/connectrpc_websocket.go`
**Estimate**: 30 minutes

Change `clearAndHome := "\x1b[2J\x1b[H"` to `clearAndHome := "\x1b[H"` at all locations where this variable is assigned (lines ~411, ~676, and any others). The terminal is blank on first connect; clearing it is a no-op that adds jank. Cursor-home alone positions the cursor for the snapshot content.

Search for all occurrences:
```go
// BEFORE
clearAndHome := "\x1b[2J\x1b[H"

// AFTER
clearAndHome := "\x1b[H"
```

**Acceptance criteria**:
- GIVEN a fresh WebSocket connection to a session
- WHEN the server sends the initial snapshot
- THEN the snapshot is prefixed with `\x1b[H` only (no `\x1b[2J`)
- AND the terminal displays the snapshot content without a visible flash
- AND existing reconnect paths (resync, state restore) still function correctly

#### ✅ Task 1.3: Upgrade xterm.js to 6.0

**Files**: `web-app/package.json`, `web-app/src/components/sessions/XtermTerminal.tsx` (import adjustments if API changed)
**Estimate**: 2 hours

Upgrade all xterm packages:
```
@xterm/xterm: ^5.5.0 -> ^6.0.0
@xterm/addon-fit: match 6.x
@xterm/addon-web-links: match 6.x
@xterm/addon-webgl: match 6.x
@xterm/addon-search: match 6.x
@xterm/addon-serialize: match 6.x (add if not present)
```

xterm.js 6.0 includes:
- Native DEC mode 2026 (synchronized output) support
- Alt-buffer scroll fixes (#5411, #5390, #5127)
- Improved `RenderService` IntersectionObserver pause/resume (relevant for Story 3)

**Acceptance criteria**:
- GIVEN the upgraded xterm.js packages
- WHEN the web app builds and runs
- THEN all existing terminal functionality works (typing, scrolling, mouse events, WebGL rendering)
- AND `make restart-web` succeeds without build errors
- AND the `@xterm/addon-serialize` package is available for Story 3

### Integration Checkpoint (Story 1)

Run `make restart-web`, open two sessions in the web UI, and verify:
1. No scroll-to-top jump when Claude is streaming output
2. No visible screen flash on initial session load
3. Terminal content renders correctly with xterm 6.0
4. WebGL renderer still activates (check console for "[XtermTerminal] WebGL renderer enabled")

---

## ✅ Story 2: Cold-Start Latency Reduction -- Quiescence Detector + Snapshot Cache

**Priority**: P1
**Status**: ✅ COMPLETE (2026-04-20)
**Scope**: ~2 days
**Risk**: Medium -- Go-side concurrency, timing-sensitive behavior
**ADR**: `ADR-003-cold-start-quiescence.md`

### Context

The current cold-start path in `streamViaControlMode()` uses hard-coded `time.Sleep(50ms)` + `time.Sleep(200ms)` after the +/-1 nudge resize, waiting for Claude's TUI to redraw. This is racy in both directions: too slow for idle sessions (wastes 250ms), too fast for active sessions (captures partial redraws). Additionally, every connect runs `tmux capture-pane` even if the session has had no output since the last capture.

### Tasks

#### ✅ Task 2.1: Implement output quiescence detector

**Files**: `server/services/connectrpc_websocket.go` (new function + modify `streamViaControlMode`)
**Estimate**: 3 hours

Add a `waitForQuiescence` function that replaces the fixed sleeps:

```go
// waitForQuiescence blocks until no output has arrived for quietFor duration,
// or until the hard timeout is reached. Returns immediately if no output is
// flowing (idle session).
func waitForQuiescence(updates <-chan struct{}, timeout time.Duration, quietFor time.Duration) {
    deadline := time.After(timeout)
    quiet := time.NewTimer(quietFor)
    defer quiet.Stop()
    for {
        select {
        case _, ok := <-updates:
            if !ok {
                return
            }
            if !quiet.Stop() {
                select {
                case <-quiet.C:
                default:
                }
            }
            quiet.Reset(quietFor)
        case <-quiet.C:
            return
        case <-deadline:
            return
        }
    }
}
```

Wire it into `streamViaControlMode`:
1. Before the nudge resize, subscribe to the control mode output channel for this session
2. After the resize, call `waitForQuiescence(outputCh, 500*time.Millisecond, 50*time.Millisecond)`
3. Remove the `time.Sleep(50 * time.Millisecond)` and `time.Sleep(200 * time.Millisecond)` calls
4. Apply the same change to all other resize+sleep paths in the file (lines ~662-666, ~980-988)

**Acceptance criteria**:
- GIVEN an idle session with no active output
- WHEN a client connects and triggers the cold-start path
- THEN the quiescence detector returns within ~50ms (one quiet window)
- AND the total cold-start time (connect to first content) is under 150ms

- GIVEN an active session where Claude is streaming output
- WHEN a client connects
- THEN the quiescence detector waits until output settles (up to 500ms hard cap)
- AND the captured snapshot reflects the settled TUI state

#### ✅ Task 2.2: Implement per-session snapshot cache

**Files**: `server/services/connectrpc_websocket.go` (new type + modify handler struct)
**Estimate**: 2 hours

Add a snapshot cache to `ConnectRPCWebSocketHandler`:

```go
type sessionSnapshot struct {
    content    string
    capturedAt time.Time
    dirty      bool // true when %output has arrived since last capture
}

// Add to ConnectRPCWebSocketHandler struct:
snapshotCache   map[string]*sessionSnapshot
snapshotCacheMu sync.RWMutex
```

Cache lifecycle:
- On each `%output` event received from control mode: call `markDirty(sessionID)`
- On connect, before capture-pane: check `cache.get(sessionID)`:
  - If not dirty: serve cached content directly (skip capture-pane entirely)
  - If dirty or not cached: run capture-pane, store result, mark clean
- On session delete: remove from cache

**Acceptance criteria**:
- GIVEN a session with no output since the last client connected
- WHEN a new client connects to that session
- THEN the server serves the cached snapshot without running `tmux capture-pane`
- AND the response latency is under 10ms (no shell exec)

- GIVEN a session that has received output since the last capture
- WHEN a client connects
- THEN the server runs capture-pane, updates the cache, and serves fresh content

#### ✅ Task 2.3: Mark snapshots dirty on control mode output

**Files**: `server/services/connectrpc_websocket.go` (in the control mode output handler)
**Estimate**: 1 hour

In the existing control mode subscriber that processes `%output` events, add a call to `h.markSnapshotDirty(sessionID)` whenever output is received. This is the invalidation signal for Task 2.2.

**Acceptance criteria**:
- GIVEN a session producing output
- WHEN the control mode handler receives `%output` events
- THEN the snapshot cache for that session is marked dirty
- AND the next client connect triggers a fresh capture-pane

### Integration Checkpoint (Story 2)

1. Start two sessions, let them idle for 10 seconds
2. Switch between them -- cold start should complete in under 150ms (check `[TerminalMetrics]` console log)
3. Start Claude output in one session, immediately switch to it from another session
4. Verify the snapshot shows settled content (not a half-drawn TUI)
5. Check server logs for `[streamViaControlMode]` -- no more `200ms sleep` messages

---

## Story 3: Terminal Instance Pool -- Eliminate Switching Jank

**Priority**: P1
**Scope**: ~3 days
**Risk**: High -- new React architecture, WebGL context limits, race conditions
**ADR**: `ADR-001-terminal-instance-pool.md`, `ADR-002-xterm-upgrade-6.0.md`
**Depends on**: Story 1 (ED3 filter must be in place before pooled terminals accumulate scrollback)

### Context

Every session switch currently destroys and recreates the xterm.js `Terminal` instance, its WebSocket connection, and all associated state. The new approach keeps up to 8 terminal instances alive in a React context pool, toggling CSS `visibility: hidden` for inactive terminals. Warm session switches become a CSS property change (~0ms).

### Tasks

#### Task 3.1: Create TerminalPoolProvider context and portal

**Files**: `web-app/src/lib/terminal/TerminalPool.tsx` (new), `web-app/src/lib/terminal/TerminalPool.css.ts` (new)
**Estimate**: 4 hours

Create a React context that manages a pool of terminal slots:

```typescript
interface PoolEntry {
  sessionId: string;
  terminalRef: React.RefObject<XtermTerminalHandle>;
  streamManagerRef: React.MutableRefObject<TerminalStreamManager | null>;
  connectionState: {
    isConnected: boolean;
    connect: (cols?: number, rows?: number) => void;
    disconnect: () => void;
    // ... other useTerminalStream return values
  };
  lastAccessed: number;
  state: 'fresh' | 'connecting' | 'live' | 'backgrounded';
}

interface TerminalPoolAPI {
  activate: (sessionId: string, baseUrl: string) => void;
  deactivate: (sessionId: string) => void;
  getEntry: (sessionId: string) => PoolEntry | undefined;
  activeId: string | null;
}
```

Key design decisions:
- Portal into `document.body` to survive React tree navigation
- Each slot renders `XtermTerminal` + manages its own `useTerminalStream` connection
- CSS `visibility: hidden` + `pointer-events: none` for inactive slots
- `position: fixed; inset: 0` on pool container, `z-index` management for active slot
- LRU eviction at `maxSize = 8` (dispose terminal, disconnect WebSocket)

**Acceptance criteria**:
- GIVEN the TerminalPoolProvider wrapping the app
- WHEN `activate(sessionId)` is called for a new session
- THEN a new pool slot is created with an xterm.js instance
- AND the instance is mounted in a portal attached to document.body
- AND calling `activate` for a different session hides the current one via CSS

- GIVEN the pool is at capacity (8 entries)
- WHEN `activate` is called for a session not in the pool
- THEN the least-recently-accessed entry is evicted (terminal disposed, WebSocket disconnected)
- AND the new session takes its slot

#### Task 3.2: Add WebGL context loss handler

**Files**: `web-app/src/components/sessions/XtermTerminal.tsx`
**Estimate**: 1 hour

Subscribe to `webglAddon.onContextLoss` and fall back to the DOM renderer:

```typescript
try {
  const webglAddon = new WebglAddon();
  webglAddon.onContextLoss(() => {
    console.warn("[XtermTerminal] WebGL context lost, falling back to DOM renderer");
    try {
      webglAddon.dispose();
    } catch {
      // May already be disposed
    }
  });
  terminal.loadAddon(webglAddon);
  console.log("[XtermTerminal] WebGL renderer enabled");
} catch (e) {
  console.warn("[XtermTerminal] WebGL not available, using canvas fallback:", e);
}
```

This is critical for the pool: 8 terminals with WebGL = 8 WebGL contexts. Browsers cap at ~16; integrated GPUs may cap at 8. Without this handler, context eviction causes blank terminals with no recovery.

**Acceptance criteria**:
- GIVEN 8+ terminal instances in the pool
- WHEN the browser evicts a WebGL context
- THEN the affected terminal falls back to the DOM renderer
- AND terminal content remains visible (no blank white rectangle)

#### Task 3.3: Set scrollback to 5000 for pooled instances

**Files**: `web-app/src/components/sessions/XtermTerminal.tsx` or pool configuration
**Estimate**: 30 minutes

Pooled terminals keep their buffer alive across switches, so scrollback is meaningful. Set `scrollback: 5000` on the Terminal constructor for pooled instances. The current value passed from `TerminalOutput.tsx` is already 5000, but verify the pool passes it through.

**Acceptance criteria**:
- GIVEN a pooled terminal that has received 100+ lines of output
- WHEN the user scrolls up
- THEN previous output is visible in the scrollback buffer
- AND the buffer retains up to 5000 lines

#### Task 3.4: Integrate pool into TerminalOutput

**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/app/layout.tsx` (or equivalent app wrapper)
**Estimate**: 3 hours

Refactor `TerminalOutput` to use the pool instead of managing its own terminal lifecycle:

1. Wrap the app with `<TerminalPoolProvider maxSize={8}>`
2. In `TerminalOutput`, replace the direct `XtermTerminal` mount with `pool.activate(sessionId, baseUrl)`
3. Remove the terminal mount/unmount logic, the `xtermRef` management, and the `streamManagerRef` lifecycle
4. Keep the toolbar, loading overlay (for cold starts only), and mobile keyboard
5. The active pool entry's terminal DOM is positioned over `TerminalOutput`'s container area

The loading overlay is only shown for cold starts (session not in pool). For warm switches, the terminal is immediately visible.

**Acceptance criteria**:
- GIVEN session A is active and session B is in the pool (backgrounded)
- WHEN the user switches to session B
- THEN session B's terminal is visible immediately (no loading overlay, no spinner)
- AND session A's terminal is hidden but its WebSocket stays connected
- AND session A's scroll position and buffer content are preserved

- GIVEN a session not in the pool (cold start)
- WHEN the user switches to it
- THEN the loading overlay appears while the connection is established
- AND after the snapshot is received, the overlay hides and terminal content is visible

#### Task 3.5: Handle fit-on-show and focus management

**Files**: `web-app/src/lib/terminal/TerminalPool.tsx`
**Estimate**: 2 hours

When a terminal transitions from hidden to visible:
1. Call `requestAnimationFrame(() => fitAddon.fit())` to ensure correct dimensions
2. Call `terminal.focus()` to restore keyboard input
3. Guard against ResizeObserver firing on hidden terminals (only send resize to backend for the active session)
4. Add `tabIndex={-1}` and `pointer-events: none` to hidden terminal containers

```typescript
// On activate(sessionId):
const entry = pool.get(sessionId);
// Make visible
entry.containerEl.style.visibility = 'visible';
entry.containerEl.style.pointerEvents = 'auto';
// Fit after layout
requestAnimationFrame(() => {
  entry.fitAddon.fit();
  entry.terminal.focus();
});
```

**Acceptance criteria**:
- GIVEN a terminal that was hidden at 120x40 and the window has been resized to 160x50 while it was hidden
- WHEN the terminal is shown
- THEN `fitAddon.fit()` runs and the terminal resizes to 160x50
- AND the backend receives a resize message with the new dimensions
- AND the terminal has keyboard focus

#### Task 3.6: Rapid-switch race condition guard

**Files**: `web-app/src/lib/terminal/TerminalPool.tsx`
**Estimate**: 1 hour

Add a session ID guard to prevent stale WebSocket messages from writing to the wrong terminal:

1. Each pool entry tracks its `sessionId`
2. The `onOutput` callback checks `if (entry.sessionId !== this.sessionId) return` before writing
3. The `writeInitialContent` operation is cancellable -- if a different session is activated mid-write, the remaining chunks are dropped

**Acceptance criteria**:
- GIVEN session A is loading initial content (chunked write in progress)
- WHEN the user rapidly switches to session B then back to session A
- THEN session A's terminal does not contain garbled output from session B
- AND session B's initial content load starts fresh

### Integration Checkpoint (Story 3)

1. Open 5 sessions in the web UI
2. Switch between them rapidly (click through all 5 in under 3 seconds)
3. Verify: no loading spinners, no scroll-to-top, no blank terminals
4. Verify: each session shows its correct content with correct scroll position
5. Open a 9th session and verify the least-recently-used session is evicted from the pool
6. Switch back to the evicted session and verify it cold-starts correctly
7. Check the console for WebGL context loss warnings (should not appear with 8 or fewer sessions)
8. Resize the browser window while session B is active, then switch to session C -- verify C has correct dimensions

---

## Story 4: Line-Granular Scrollback API + Lazy Loading (Deferred)

**Priority**: P3 (deferred until after Story 3 ships)
**Scope**: ~2-3 days
**Risk**: Medium -- new API endpoint, xterm.js buffer manipulation
**Depends on**: Story 3 (pool must be stable first)

### Context

With pooled terminals holding 5000 lines of scrollback, most users will never need more history. But for long-running sessions, the existing `ScrollbackManager` holds up to 10,000 entries of raw PTY data on the server. The client currently ignores `scrollbackResponse` messages. This story wires up the existing proto for on-demand history loading.

### Tasks

#### Task 4.1: Implement GetScrollbackByLines on ScrollbackManager

**Files**: `session/scrollback/manager.go`, `session/scrollback/buffer.go`
**Estimate**: 3 hours

Add a line-granular API to the scrollback manager:

```go
func (m *ScrollbackManager) GetScrollbackByLines(
    sessionID string,
    offsetFromEnd int,
    lineCount int,
) (data []byte, totalLines int, err error)
```

Implementation:
1. Read raw entries from the circular buffer
2. Concatenate entry data and count `\n` characters to build a line index
3. Return the requested line range as raw terminal bytes
4. Return `totalLines` for the client to know pagination bounds

**Acceptance criteria**:
- GIVEN a session with 500 lines of scrollback
- WHEN `GetScrollbackByLines(id, 0, 50)` is called
- THEN the last 50 lines are returned as raw bytes
- AND `totalLines` is 500

#### Task 4.2: Wire scrollbackResponse handling in the client

**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/lib/hooks/useTerminalStream.ts`
**Estimate**: 3 hours

1. Remove the early return at `TerminalOutput.tsx:158-163` that discards `scrollbackResponse` with metadata
2. On `scrollbackResponse` with metadata: prepend historical content above the current viewport
3. Use `@xterm/addon-serialize` to snapshot current state, clear, write history + snapshot

Trigger: when `terminal.buffer.active.viewportY === 0` (user scrolled to top of loaded content), request the next page.

**Acceptance criteria**:
- GIVEN a terminal at the top of its 5000-line buffer with more history on the server
- WHEN the user scrolls to the top
- THEN a scrollback request is sent to the server
- AND the response is prepended above the current content
- AND the viewport scroll position is preserved (no jump)

#### Task 4.3: Add scroll-to-top trigger

**Files**: `web-app/src/lib/terminal/TerminalPool.tsx` or `TerminalOutput.tsx`
**Estimate**: 1 hour

Subscribe to xterm.js `onScroll` event. When `viewportY === 0` and there are more lines available (based on `totalLines` from last response), trigger `requestScrollback`.

**Acceptance criteria**:
- GIVEN the user is scrolling up through terminal history
- WHEN they reach the top of loaded content
- THEN a loading indicator appears briefly while history is fetched
- AND new content appears above the viewport seamlessly

---

## Known Issues

### Bug 1: WebGL Context Limit [SEVERITY: High]

**Description**: Browsers enforce a hard cap of ~16 WebGL contexts per page. Integrated GPUs may cap at 8. Each pooled terminal consumes one WebGL context. Pool of 8 leaves minimal margin.

**Mitigation**:
- Subscribe to `webglAddon.onContextLoss` (Task 3.2) and fall back to DOM renderer
- Pool size of 8 is conservative; users with integrated graphics may need a lower cap
- Consider auto-detecting GPU capability and adjusting pool size

**Files affected**: `XtermTerminal.tsx`, `TerminalPool.tsx`

**Prevention**: Task 3.2 implements the handler. Monitor for context loss events in production via console warnings.

### Bug 2: FitAddon on visibility:hidden Terminals [SEVERITY: Medium]

**Description**: `FitAddon.proposeDimensions()` works correctly with `visibility: hidden` (layout is computed), but calling `fit()` immediately after unhiding may use stale dimensions if the container resized while hidden.

**Mitigation**:
- Call `fit()` inside `requestAnimationFrame` after unhiding (Task 3.5)
- ResizeObserver fires with real dimensions on `visibility: hidden` containers, so dimensions are tracked even while hidden
- Never call `terminal.open()` on a hidden container

**Files affected**: `TerminalPool.tsx`, `XtermTerminal.tsx`

**Prevention**: Task 3.5 handles the show sequence.

### Bug 3: Rapid-Switch Race Condition [SEVERITY: Medium]

**Description**: If the user switches sessions faster than WebSocket messages arrive, old session output could be written to the newly-active terminal. In the pool model, each terminal has its own stream, so this is less likely -- but `writeInitialContent` chunked writes could still be in-flight for a cold-start terminal that gets backgrounded mid-load.

**Mitigation**:
- Each pool entry's `onOutput` callback is bound to its specific terminal instance, not a shared ref (Task 3.6)
- `writeInitialContent` checks a cancellation flag between chunks
- Session ID stamping on all WebSocket message handlers

**Files affected**: `TerminalPool.tsx`, `TerminalStreamManager.ts`

**Prevention**: Task 3.6 adds the guards.

### Bug 4: tmux capture-pane Alternate Screen Race [SEVERITY: Medium]

**Description**: `tmux capture-pane` cannot capture alternate-screen content. The +/-1 nudge forces a SIGWINCH redraw, but there is a race window between the redraw starting and `capture-pane` executing. The quiescence detector (Story 2) reduces this window but does not eliminate it -- the detector monitors control mode `%output` events, which may lag behind the actual PTY write.

**Mitigation**:
- Quiescence detector (Task 2.1) waits for output to settle before capturing
- 500ms hard cap prevents infinite waiting
- With the pool in place, this race only affects cold starts (first view or evicted sessions)
- Pooled terminals receive live output directly -- no capture-pane needed

**Files affected**: `connectrpc_websocket.go`

**Prevention**: The pool (Story 3) makes this race irrelevant for 99% of session switches.

### Bug 5: CLAUDE_CODE_NO_FLICKER Destroys Scrollback [SEVERITY: Low]

**Description**: Claude Code v2.1.89+ introduced `CLAUDE_CODE_NO_FLICKER` which re-renders the entire conversation when output exceeds screen height, destroying scrollback. Multiple bugs reported: broken table copy, broken arrow nav, garbled Japanese text.

**Mitigation**:
- Our ED3 filter (Task 1.1) works regardless of `CLAUDE_CODE_NO_FLICKER` state
- Do not set or depend on this env var in Stapler Squad
- The pool's 5000-line scrollback buffer provides the history that `CLAUDE_CODE_NO_FLICKER` destroys

**Files affected**: None (external dependency)

**Prevention**: Document in README that `CLAUDE_CODE_NO_FLICKER=0` may improve experience.

### Bug 6: Memory Growth with Large Pool [SEVERITY: Low]

**Description**: Each pooled terminal consumes ~1MB JS heap (5000-line scrollback) + ~16MB VRAM (WebGL texture atlas). At pool size 8: ~8MB heap + ~128MB VRAM.

**Mitigation**:
- LRU eviction at pool size 8 caps resource usage
- `terminal.dispose()` on eviction frees all resources
- Monitor memory usage via browser devtools Memory panel

**Files affected**: `TerminalPool.tsx`

**Prevention**: Pool size is configurable; defaults to 8 based on WebGL context limit analysis.

### Bug 7: Stale ResizeObserver on Hidden Terminals [SEVERITY: Low]

**Description**: Hidden terminals' ResizeObserver remains active. If a layout shift affects the hidden container, the observer fires and could send a spurious resize to the backend.

**Mitigation**:
- Gate backend resize calls on "session is currently active" predicate (Task 3.5)
- With `visibility: hidden`, the container maintains its dimensions, so spurious resize events are unlikely unless the browser window itself is resized

**Files affected**: `TerminalPool.tsx`, `XtermTerminal.tsx`

**Prevention**: Task 3.5 gates resize messages on active session check.

---

## Testing Strategy

### Unit Tests

| Component | Test Focus | Location |
|-----------|-----------|----------|
| EscapeSequenceParser | ED3 filter: paired, standalone, multiple in chunk, split across chunks | `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts` |
| waitForQuiescence | Immediate return on idle, wait-then-return on active, hard cap timeout | `server/services/connectrpc_websocket_test.go` |
| Snapshot cache | Get/set, dirty marking, clean serving, eviction on session delete | `server/services/connectrpc_websocket_test.go` |
| TerminalPool | LRU eviction, activate/deactivate, pool size limits | `web-app/src/lib/terminal/__tests__/TerminalPool.test.ts` |

### Integration Tests

| Scenario | Verification |
|----------|-------------|
| Session switch (warm) | Switch time < 50ms, no loading overlay, scroll position preserved |
| Session switch (cold) | Loading overlay appears, content loads within 200ms |
| Rapid switching (5 sessions in 3 seconds) | No garbled output, correct content on each terminal |
| Window resize while terminal hidden | Correct dimensions on show, backend receives resize |
| WebGL context loss simulation | DOM renderer fallback, terminal content visible |
| Quiescence detector accuracy | Idle sessions: < 100ms total; active sessions: snapshot reflects settled state |

### Manual QA Checklist

- [ ] Open 5+ sessions, switch between them, verify no jank
- [ ] Scroll up in a session, switch away, switch back -- scroll position preserved
- [ ] Start Claude Code in a session, let it stream output, verify no scroll-to-top
- [ ] Resize browser window while viewing session A, switch to session B -- B has correct dimensions
- [ ] Open 10+ sessions to trigger LRU eviction, verify evicted sessions cold-start correctly
- [ ] Check browser console for WebGL context loss warnings
- [ ] Check `[TerminalMetrics]` for cold-start times < 200ms
- [ ] Verify mobile keyboard toolbar still functions

---

## Rollout Plan

1. **Story 1** ships first (P0 bug fixes, no architectural risk)
2. **Story 2** ships second (server-side changes, testable independently)
3. **Story 3** ships third (depends on Story 1, largest scope)
4. **Story 4** deferred to a future release (independent, lower priority)

Each story can be merged independently. Story 3 is the only one with a hard dependency on Story 1.
