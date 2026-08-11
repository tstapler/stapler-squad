# ADR-001: Terminal Instance Pool with CSS Visibility Toggling

**Status**: Accepted
**Date**: 2026-04-09
**Deciders**: Tyler Stapler
**Relates to**: Terminal Jank Elimination (Story 3)

## Context

Every session switch in Stapler Squad's web UI destroys and recreates the xterm.js `Terminal` instance, its WebGL rendering context, and its WebSocket connection. This causes 500-1200ms of jank: the user sees a loading spinner while the new terminal initializes, connects, receives a snapshot from `tmux capture-pane`, and replays it. The root cause is that `TerminalOutput` is re-mounted on every `sessionId` prop change, coupling the terminal lifecycle to React component mounting.

Three approaches were evaluated:

1. **CSS visibility pool**: Keep N terminal instances alive in a React context, toggle `visibility: hidden` for inactive terminals. WebSocket connections stay open. Session switch becomes a CSS property change.

2. **Reconnect with visible-screen snapshot**: Keep the mount/unmount model but optimize the reconnect path. Send only the visible viewport from `tmux capture-pane` instead of replaying scrollback. Reduce the fixed 250ms sleep with a quiescence detector.

3. **Detach/reattach DOM elements**: Follow VS Code's pattern of `detachFromElement()` / `attachToElement()` to move the xterm.js canvas between containers without destroying the `Terminal` object.

## Decision

We chose Option 1: a CSS visibility pool with LRU eviction, capped at 8 terminal instances. Each pooled terminal keeps its xterm.js `Terminal` instance alive, its WebSocket connected, and its buffer accumulating output. Inactive terminals use `visibility: hidden` with `pointer-events: none`.

The pool is rendered as a React portal into `document.body` to survive navigation within the React tree. A `TerminalPoolProvider` context manages allocation, activation, and eviction.

Option 2 (optimized reconnect) is also implemented as the fallback path for cold starts and evicted sessions, but it is not the primary session-switch mechanism.

Option 3 was rejected because xterm.js 5.x/6.x does not expose stable `detachFromElement()` / `attachToElement()` APIs in its public surface. VS Code accesses internal APIs (`xterm.raw`) that are not available to external consumers. Reimplementing this would require forking xterm.js or depending on private API surface that breaks across versions.

## Consequences

### Positive

- Warm session switches are instantaneous (~0ms): CSS visibility change only, no network roundtrip, no terminal re-initialization.
- Terminal state is perfectly preserved: scroll position, cursor position, buffer content, alternate screen state.
- WebSocket connections stay open, so live output continues accumulating in background terminals. No output is lost during a switch.
- Eliminates all session-switch race conditions (stale output, pending disconnect, writeInitialContent mid-switch) because terminals are never destroyed on switch.

### Negative

- Memory overhead: each pooled terminal holds ~1MB JS heap (5000-line scrollback) + ~16MB VRAM (WebGL context). At pool size 8: ~8MB heap + ~128MB VRAM.
- WebGL context limit: browsers cap at ~16 contexts per page. Pool size of 8 leaves margin, but integrated GPUs with lower limits may trigger context eviction. Mitigation: subscribe to `webglAddon.onContextLoss` and fall back to DOM renderer.
- Complexity: the React component architecture becomes more complex. `TerminalOutput` no longer owns its terminal instance; it delegates to the pool context. The portal-based rendering requires careful z-index and focus management.
- Server resource cost: N goroutines per connected client (one per pooled session), each with an open WebSocket. At 8 sessions: ~16 goroutines + ~640KB in WebSocket buffers. This is negligible for a localhost application.

### Neutral

- LRU eviction means sessions outside the pool (9th+ most recently accessed) still experience cold-start latency. This is mitigated by the quiescence detector (ADR-003) which reduces cold-start time to ~50-100ms.
- The `@xterm/addon-serialize` package becomes a dependency (needed for potential future snapshot/restore on eviction).

## Patterns Applied

- **Object Pool Pattern** (GoF): reuse expensive-to-create objects (xterm.js Terminal instances) rather than creating and destroying them on each use.
- **Portal Pattern** (React): render pool entries into `document.body` to decouple their lifecycle from the component tree.
- **LRU Cache**: evict least-recently-accessed entries when the pool reaches capacity, balancing memory usage against switch latency.
- **VS Code Terminal Architecture** (prior art): VS Code keeps all terminal instances alive with `display: none` toggling. We use `visibility: hidden` instead because `display: none` causes `FitAddon.proposeDimensions()` to return `undefined` (zero container dimensions).
