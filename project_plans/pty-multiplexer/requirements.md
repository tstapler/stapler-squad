# Requirements: PTY Multiplexer Subsystem

## Confirmed Bug (Root Cause)

`GetPTYReader()` in `session/instance_tmux.go` returns the single `t.ptmx` `*os.File` handle.
Two independent consumers call `Read()` on this same file descriptor simultaneously:

1. **`ClaudeController`** (via `PTYAccess.Read()` in `session/pty_access.go:63`) — reads bytes
   into the 256KB circular buffer used by status detection, idle detection, and `Preview()`.
2. **`StreamTerminal`** (in `server/services/session_service.go:1753`) — reads bytes to send
   to the browser WebSocket client.

PTY `Read()` is destructive: each byte can only be consumed once. Whichever goroutine wins the
`Read()` race gets the data; the other gets nothing. When the browser is open, the `❯` prompt
bytes are frequently consumed by the streaming goroutine before the controller sees them.
Result: `GetDetectedStatus()` never returns `StatusIdle`, the driver times out after 30 seconds,
and the initial workflow prompt fires blind — missing or arriving before Claude's readline is
ready.

## Problem Statement

The codebase currently assumes at most ONE reader of PTY output at a time. This assumption is
violated whenever a user opens a session in the browser while the session driver is running.
There is no fan-out mechanism: the same `*os.File` handle is handed to multiple callers who
each believe they are the sole consumer.

## Goals

1. Replace the single-reader PTY model with a **broadcast multiplexer** that delivers every
   byte to ALL registered consumers — no byte is ever lost to a competing reader.
2. All existing consumers (`ClaudeController`, `StreamTerminal`, any future subscribers) read
   from their own independent byte stream without interfering with each other.
3. The multiplexer is the single goroutine that reads from `t.ptmx`; subscribers receive
   copies via channels or ring-buffer cursors.
4. `StreamTerminal` and the controller's status/idle detection work correctly regardless of
   whether zero, one, or many browser clients are connected simultaneously.

## Functional Requirements

### FR-1: Single PTY Reader
- **FR-1a**: Exactly one goroutine reads from `t.ptmx` at any time (the multiplexer loop).
- **FR-1b**: `PTYAccess.Read()` is removed or repurposed — direct `t.ptmx` reads outside the
  multiplexer are forbidden.
- **FR-1c**: The multiplexer starts when the PTY is created (`RestoreWithWorkDir`) and stops
  when the PTY is closed.

### FR-2: Fan-Out Delivery
- **FR-2a**: Every byte read from `t.ptmx` is delivered to ALL active subscribers.
- **FR-2b**: A slow or temporarily-blocked subscriber (e.g. a browser client with backpressure)
  does not block other subscribers or the multiplexer read loop.
- **FR-2c**: When a subscriber falls too far behind (configurable watermark), it receives an
  explicit "overrun" signal and can resync from the circular buffer snapshot rather than
  silently losing bytes.

### FR-3: Subscriber Interface
- **FR-3a**: Any component can call `mux.Subscribe()` to receive a `<-chan []byte` (or equivalent
  cursor into a shared ring buffer) and `mux.Unsubscribe()` when done.
- **FR-3b**: The controller subscribes once at startup and unsubscribes on stop.
- **FR-3c**: `StreamTerminal` subscribes per-connection and unsubscribes when the browser
  disconnects.
- **FR-3d**: Subscribing or unsubscribing is safe from any goroutine, including under concurrent
  reads.
- **FR-3e**: `GetPTYReader()` is either removed or restricted — callers that formerly used it
  for reading must use `Subscribe()` instead. (Writing to the PTY is unaffected.)

### FR-4: Circular Buffer Integration
- **FR-4a**: The multiplexer writes every byte into the existing 256KB circular buffer (or a
  promoted one owned by the multiplexer, not `PTYAccess`).
- **FR-4b**: New subscribers receive a configurable initial snapshot of recent history from the
  circular buffer before receiving live deltas — enabling `StreamTerminal` to send the current
  screen state on connect without a separate `CapturePaneContent` call.
- **FR-4c**: `Preview()` reads from the circular buffer as before (no behavioral change).

### FR-5: Backward Compatibility
- **FR-5a**: `ClaudeController`, `StatusDetector`, `IdleDetector`, `RateLimitConsumer`,
  `ResponseStream` — all components that previously called `PTYAccess.Read()` — continue to
  work without API changes at their call sites, even if the underlying implementation changes.
- **FR-5b**: Sessions without a controller (external/attached sessions) fall back to
  `CapturePaneContent` for `Preview()` as they do today.
- **FR-5c**: `WriteToPTY` / `SendKeys` are unaffected — the multiplexer is read-only.
- **FR-5d**: `streamViaControlMode` (the WebSocket/control-mode path used by most sessions) is
  unaffected unless it also uses `GetPTYReader()`.

### FR-6: Lifecycle Safety
- **FR-6a**: If `t.ptmx` is replaced (e.g. `RestoreWithWorkDir` called again after EIO), the
  multiplexer stops, all subscribers are notified via channel close or error, and a new
  multiplexer starts on the new PTY. Subscribers reconnect automatically.
- **FR-6b**: When the multiplexer goroutine exits (EIO, PTY closed), all subscriber channels
  are drained and closed — no goroutine leaks.
- **FR-6c**: Multiple simultaneous browser clients for the same session each get independent
  subscriber channels and receive the same bytes.

## Non-Goals

- This refactor does NOT change how `WriteToPTY` / `SendKeys` work.
- This does NOT change the `streamViaControlMode` WebSocket path unless it already uses
  `GetPTYReader()` for reads.
- This does NOT change the session driver's prompt detection logic beyond fixing the data
  starvation it currently experiences.
- No changes to the proto/API surface.
- No changes to the tmux backend (`TmuxSession`), except wiring the multiplexer in.

## Relevant Files

| File | Role |
|---|---|
| `session/tmux/tmux.go` | `TmuxSession.GetPTY()` — returns `t.ptmx` (the single fd) |
| `session/instance_tmux.go` | `Instance.GetPTYReader()` — calls `GetPTY()`, used by controller + StreamTerminal |
| `session/pty_access.go` | `PTYAccess` — wraps `*os.File`, calls `p.pty.Read()` directly |
| `session/claude_controller.go` | Calls `GetPTYReader()` at line 147; creates `PTYAccess` |
| `server/services/session_service.go` | `StreamTerminal` calls `GetPTYReader()` at line 1698, reads at line 1753 |
| `session/instance_terminal.go` | `Preview()` — uses `ctrl.GetRecentOutput(0)` or `CapturePaneContent` |

## Acceptance Criteria

1. With a browser client connected and actively streaming, `ClaudeController` always detects
   `StatusIdle` within 5 seconds of Claude showing the `❯` prompt.
2. The initial workflow prompt is injected with `claudeAtPrompt:true` (not `timedOut:true`)
   even when a user has the session open in the browser.
3. Two simultaneous browser clients on the same session both see identical output.
4. `make quick-check` passes (no new test failures beyond pre-existing).
5. No goroutine leaks when a browser client disconnects mid-stream (verified via `pprof`
   goroutine dump or test).
6. `Preview()` returns correct terminal content regardless of whether 0, 1, or N browser
   clients are connected.
