# Research: Scrollback Architecture & Cold-Start API

Status: Complete | Phase: 2 - Research
Created: 2026-04-09

## Summary

The current scrollback architecture has two disconnected halves: a server-side `ScrollbackManager` with 10,000-entry circular buffer + zstd-compressed disk storage, and an xterm.js client configured with `scrollback: 0` (no client buffer). The server-side data is never used — the client silently ignores all `scrollbackResponse` messages. Cold-start latency (500–1200ms) is dominated by a hard-coded 250ms Go sleep waiting for tmux SIGWINCH. Three targeted API changes eliminate most of this: a per-session snapshot cache, an output-quiescence detector replacing the 250ms sleep, and a line-granular scrollback endpoint enabling real lazy loading.

---

## Current State Audit

### Server side (`session/scrollback/`)

```
ScrollbackConfig {
  MaxLines:        10000      // CircularBuffer cap
  MaxSizeBytes:    10MB       // disk storage cap (zstd level 3)
  FlushInterval:   5s         // how often dirty buffer flushes to disk
  StoragePath:     ~/.stapler-squad/sessions/
}
```

`ScrollbackManager` stores raw PTY write chunks as `ScrollbackEntry { Timestamp, Data []byte, Sequence uint64 }`. Critically: **entries are PTY write chunks, not lines**. One entry might be 3 bytes (`$ `) or 64KB (a file cat). 10,000 entries for a verbose Claude session could be anywhere from 500KB to 100MB of terminal data.

The proto has `ScrollbackRequest` / `ScrollbackResponse` and `requestScrollback()` on the client side — both fully wired — but the server never calls `GetRecentLines` / `GetScrollback` in the initial snapshot path, and the client at `TerminalOutput.tsx:158–163` explicitly discards any `scrollbackResponse` with metadata.

### Client side (`XtermTerminal.tsx:95`)

```ts
const scrollback = scrollbackProp ?? config?.scrollbackLines ?? 0;
```

xterm.js `Terminal({ scrollback: 0 })` means **zero lines above the viewport**. Users can only see what's currently on screen. If they scroll up in xterm.js, there is nothing there. The design intent was to rely on server-side scrollback via the proto protocol, but that half was never connected.

### Cold-start path (`connectrpc_websocket.go:streamViaControlMode`)

```
1. Parse handshake → extract targetCols, targetRows
2. ResizePTY(cols-1, rows)          → 50ms sleep
3. ResizePTY(cols, rows)            → 200ms sleep  ← dominant cost
4. CapturePaneContentRaw()          → 20–80ms (shell exec + tmux read)
5. Send "\x1b[2J\x1b[H" + snapshot → client writes to blank terminal
```

The 200ms sleep is a fixed worst-case guess for "how long until tmux TUI redraws after SIGWINCH". There is no feedback — the server sleeps regardless of whether the redraw takes 10ms or 800ms.

---

## Problem 1: Entry Granularity Mismatch

The `CircularBuffer` capped at 10,000 entries provides no meaningful line-count guarantee. For pagination in an infinite-scroll UI, the client needs to say "give me lines 200–400 above viewport" not "give me entries 5000–5200". Without line-count tracking, the client cannot implement reliable page offsets.

**Fix**: Add a `GetScrollbackLines(sessionID string, fromLine int, lineCount int) ([]byte, int, error)` method to `ScrollbackManager` that:
1. Reads raw bytes from the circular buffer + storage
2. Counts newlines to build a line index
3. Returns the requested line slice as raw terminal bytes
4. Returns `totalLinesAvailable` for the client to know when to stop paginating

This can be built on top of the existing `GetRecentBytes` / `GetScrollback` without changing the storage format.

---

## Problem 2: Cold-Start Latency (250ms Hard-coded Sleep)

The ±1 nudge + 200ms sleep is racy in two directions:
- **Too short**: fast-moving Claude output may still be mid-redraw at 200ms, so the snapshot captures a partial state
- **Too long**: idle sessions or simple shell prompts redraw in <10ms, wasting 190ms

**Fix: Output quiescence detector**

Replace the fixed sleep with a goroutine that reads from the control mode `%output` event stream and waits until no `%output` events have arrived for N milliseconds (e.g. 50ms). If the session is already idle, this fires immediately. If the TUI is actively redrawing, it waits until the redraw settles.

```go
func waitForQuiescence(updates <-chan []byte, timeout time.Duration, quietFor time.Duration) {
    deadline := time.After(timeout)       // 500ms absolute cap
    quiet := time.NewTimer(quietFor)      // 50ms no-output window
    for {
        select {
        case _, ok := <-updates:
            if !ok { return }
            quiet.Reset(quietFor)         // output arrived, reset quiet timer
        case <-quiet.C:
            return                        // 50ms of silence → TUI settled
        case <-deadline:
            return                        // hard cap, proceed anyway
        }
    }
}
```

Expected improvement: idle sessions go from 250ms to ~5ms. Active Claude sessions go from 250ms to ~50–100ms (actual redraw time).

---

## Problem 3: Per-Session Snapshot Cache

Every cold start runs `tmux capture-pane` via a shell exec (20–80ms). For sessions that haven't had output since the last connect, we're paying this cost unnecessarily.

**Fix**: Cache the last capture-pane result in memory, keyed by session ID. Invalidate on every `%output` event received from control mode. On connect:
- If cache is valid (no output since last capture): serve cached snapshot directly (~0ms)
- If cache is invalid (output since last capture): run capture-pane, update cache (~20–80ms)

```go
type snapshotCache struct {
    mu        sync.RWMutex
    snapshots map[string]*sessionSnapshot
}

type sessionSnapshot struct {
    content   string
    capturedAt time.Time
    dirty     bool  // true = output arrived since last capture
}
```

Control mode's `%output` handler calls `cache.markDirty(sessionID)`. The snapshot handler calls `cache.getOrRefresh(sessionID, captureFn)`.

---

## Problem 4: ED2+ED3 Scroll-to-Top on Cold Restore

The current cold-start path sends:
```
\x1b[2J\x1b[H + sanitized capture-pane content
```

`\x1b[2J` is ED2 (clear screen). Claude's TUI output also includes `\x1b[2J\x1b[3J` (ED2 + ED3 = erase scrollback). Both hit the same xterm.js `eraseInDisplay` path that resets `viewportY` to 0 — causing scroll-to-top for users scrolled up.

**Fix**: Replace `\x1b[2J\x1b[H` with `\x1b[H` alone (cursor to home without clearing). The terminal is fresh on first connect so there's nothing to clear. For reconnects after pool eviction, use `@xterm/addon-serialize` to serialize + clear + restore state directly in the client instead of doing server-side clear.

Additionally, add the ED3 filter to `EscapeSequenceParser.processChunk()`:
```ts
// Strip ED3 (erase scrollback) when paired with ED2 (clear screen)
// Claude's TUI emits \x1b[2J\x1b[3J on every redraw; ED3 resets viewportY to 0
data = data.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```

---

## Warm Session Scrollback (Pool Approach)

With keep-alive terminal instances, the scrollback question simplifies significantly:

- xterm.js `scrollback: 0` → change to `scrollback: 5000` for pooled instances
- The xterm.js in-memory buffer naturally accumulates history as Claude writes, exactly like a local terminal
- No server-side fetch needed for warm sessions at all
- `ScrollbackManager` remains as the backing store for cold sessions and history beyond 5000 lines

**Memory cost**: xterm.js `scrollback: 5000` at 220 cols ≈ 5000 × 220 chars ≈ ~1.1MB per session for line data. At pool size 8: ~9MB total. Acceptable.

---

## Lazy Scrollback Design (Deferred to Phase 3)

For history beyond the xterm.js buffer (cold sessions, long-running sessions):

**Client triggers**: When `terminal.buffer.active.viewportY === 0` (user scrolled to top of xterm buffer), request the next page of history.

**API call**: `requestScrollback(fromSequence, lineCount)` → `scrollbackResponse`

**Rendering**: The `@xterm/addon-serialize` pattern: serialize current terminal state, clear, prepend historical lines, restore serialized state. This is the only way to inject history "above" xterm's current content.

**Server API needed**:
```go
// New method on ScrollbackManager
func (m *ScrollbackManager) GetScrollbackByLines(
    sessionID string,
    offsetFromEnd int,  // how many lines from end to start
    lineCount int,      // how many lines to return
) (data []byte, totalLines int, err error)
```

Wire into the existing `scrollbackResponse` proto — no new proto fields needed. The `ScrollbackChunk.data` already carries raw bytes and `hasMore` + `totalLines` carry pagination state.

**Note**: Do not implement until Phase 3. The pool approach (Phase 1) eliminates the need for lazy loading for any session that has been viewed before.

---

## Recommended API Changes (Priority Order)

| Priority | Change | File | Expected gain |
|---|---|---|---|
| P0 | ED3 filter (`\x1b[2J\x1b[3J` → `\x1b[2J`) | `EscapeSequenceParser.ts` | Eliminates scroll-to-top |
| P1 | Output quiescence detector (replace 250ms sleep) | `connectrpc_websocket.go` | 200ms → 5–50ms cold start |
| P1 | Per-session snapshot cache (invalidate on %output) | `connectrpc_websocket.go` | 20–80ms → ~0ms for unchanged sessions |
| P1 | Remove `\x1b[2J` from cold-start prefix (use `\x1b[H` only) | `connectrpc_websocket.go` | Eliminates ED2 viewport reset |
| P2 | `scrollback: 5000` on pooled xterm.js instances | `XtermTerminal.tsx` | Full history for warm sessions |
| P3 | `GetScrollbackByLines(sessionID, offsetFromEnd, lineCount)` | `scrollback/manager.go` | Enables infinite scroll |
| P3 | Wire client `requestScrollback` → show history above viewport | `TerminalOutput.tsx` | Infinite scroll UX |

---

## References

- `server/services/connectrpc_websocket.go:streamViaControlMode` — cold-start path
- `session/scrollback/manager.go` — ScrollbackManager, GetRecentLines, GetScrollback
- `session/scrollback/buffer.go` — CircularBuffer (entry-granular, not line-granular)
- `web-app/src/components/sessions/TerminalOutput.tsx:158–163` — scrollbackResponse ignored
- `web-app/src/lib/terminal/EscapeSequenceParser.ts` — missing ED3 filter
- `web-app/src/components/sessions/XtermTerminal.tsx:95` — scrollback: 0
- anthropics/claude-code#36582 — ED2+ED3 root cause confirmation
