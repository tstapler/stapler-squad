# ADR-012: Scrollback Delivery Strategy

## Status: Proposed

## Context

The current terminal implementation sets `xterm.js scrollback: 0` and never fetches server-side
history when the user scrolls up. The `ScrollbackManager` maintains a 10,000-line circular buffer
backed by compressed JSONL files on disk, but the client never receives any of it. When xterm.js
hits its scrollback=0 ceiling, its viewport wraps around and displays the current screen contents
at position 0 — producing the "current content repeated" artifact described in Problem 2.

The proto messages `ScrollbackRequest` and `ScrollbackResponse` already exist in `events.proto`
and are sufficient for the on-demand path. The WebSocket input goroutine (`streamViaControlMode`)
does not dispatch on `GetScrollbackRequest()` — that handler branch is simply missing.

Three delivery architectures were considered for satisfying R2 (faithful scrollback):

**Option A — Extend existing ConnectRPC stream (in-stream initial batch only)**: Send scrollback as
a new `ScrollbackResponse` message type in the existing bidirectional stream immediately after the
initial pane snapshot, before live output begins. No separate RPC. The client consumes it as part
of the normal stream message loop. Does not address the user scrolling beyond the initial batch.

**Option B — New ConnectRPC endpoint (on-demand only)**: Add a separate `GetScrollback` unary RPC.
Client calls it at connection time for the initial batch and again when the user scrolls near the
top. Avoids any changes to the existing stream message union. Requires two open connections during
the load window and introduces a timing gap where live output may arrive before the initial
scrollback RPC returns.

**Option C — In-stream initial batch + lazy on-demand via existing stream messages**: Send the
initial scrollback batch in-stream (same as A) to avoid the two-connection race. Serve older
batches on demand via the same stream's `ScrollbackRequest`/`ScrollbackResponse` exchange (already
defined in proto) when the user scrolls within 200 lines of the top of xterm.js's local buffer.
A write-lock flag on `TerminalStreamManager` (`isWritingInitialContent`) blocks live output from
being written to xterm.js until the initial batch is fully flushed, preventing the out-of-order
write hazard documented in the pitfalls research.

Key constraints from research:

- xterm.js has no prepend API. Historical lines must be written before live bytes to maintain
  correct visual order. The only safe window to write history is while the buffer is empty
  (initial connection).
- xterm.js `onScroll` does not fire on user scroll gestures (issues #3201, #3864). The near-top
  trigger must use a DOM `scroll` event listener checking `terminal.buffer.active.viewportY`.
- The existing `ScrollbackRequest`/`ScrollbackResponse` proto surface is already sufficient; no
  new proto messages are needed for the on-demand path.
- ConnectRPC over WebSocket provides TCP FIFO ordering at the transport layer, but the application-
  layer async write path can lose ordering. The `isWritingInitialContent` write-lock resolves this.
- For paged on-demand loads (scroll-to-top), xterm.js cannot prepend. The practical approach:
  serialize current buffer with `SerializeAddon`, `terminal.clear()`, write older batch, then
  restore the serialized state. Alternatively, pre-load a larger initial window (500 lines with
  `scrollback: 5000`) to reduce paging frequency for typical Claude Code sessions.

## Decision

Adopt **Option C**: in-stream initial scrollback batch + lazy on-demand pagination via the existing
`ScrollbackRequest`/`ScrollbackResponse` proto exchange.

Concrete changes:

1. Set `xterm.js scrollback: 5000` (R2.1). Change the default in `XtermTerminal.tsx` from
   `scrollbackProp ?? config?.scrollbackLines ?? 0` to `?? 5000`.

2. After the initial pane snapshot is sent, the server sends a `ScrollbackResponse` containing
   the most recent 500 lines from `ScrollbackManager.GetScrollback()` as a second message in the
   same `streamViaControlMode` stream (~line 558 in `connectrpc_websocket.go`). Update the
   handshake `lines` parameter from 50 to 500 (R2.6).

3. Remove the `handleScrollbackReceived()` metadata guard that rejects historical scrollback
   (R2.7). All scrollback responses, initial or paged, call `manager.writeInitialContent()` or a
   new `manager.prependScrollback()` path.

4. Add `isWritingInitialContent` write-lock to `TerminalStreamManager`. Live output arriving via
   `manager.write()` while this flag is set is buffered in `pendingLiveWrites` and flushed in
   order after `writeInitialContent` resolves (R2.5, pitfall #8 mitigation).

5. Add the missing `case incomingData.GetScrollbackRequest() != nil:` branch in the WebSocket
   read goroutine, calling `scrollbackManager.GetScrollback(sessionID, req.FromSequence,
   int(req.Limit))` and writing back a `ScrollbackResponse`.

6. On the client, monitor `terminal.buffer.active.viewportY` via a DOM `scroll` event listener.
   When `viewportY < 200`, send a `ScrollbackRequest` with `from_sequence` set to the oldest
   sequence already rendered (R2.3). For paging, use `SerializeAddon` serialize → `terminal.clear()`
   → write older batch → restore serialized state to work around the xterm.js append-only
   constraint.

## Consequences

**Positive:**
- No new proto messages or RPC endpoints required; the existing `ScrollbackRequest`/`ScrollbackResponse`
  surface is fully utilized.
- Single WebSocket connection handles both initial load and on-demand paging; no two-connection
  timing race (vs Option B).
- The `isWritingInitialContent` lock correctly serializes history and live output regardless of
  how fast the server sends live bytes.
- Users immediately see the last 500 lines of history on connect and can page back further
  without reconnecting.
- Initial scrollback load (500 lines) targets ≤300 ms on localhost (R5.3).

**Negative / trade-offs:**
- The `SerializeAddon` serialize → clear → restore pattern for deep-history paging is complex and
  resets xterm.js selection state. Acceptable given that paging beyond the initial 500-line window
  is an uncommon interaction.
- `FileScrollbackStorage.ReadTail` has a latent bug (seeks raw compressed bytes rather than
  decoding zstd). The fix (decode before seeking) is a prerequisite for reliable deep-history
  reads and must be included in the implementation.
- Server-side `GetRecentBytes` in-memory cap (currently 500 lines) must be raised to at least
  5000 lines or removed, relying on `GetScrollback`'s `limit` parameter instead.

## Alternatives Considered

**Option A (in-stream initial batch only)** was rejected because it leaves the user unable to
access history beyond the initial 500-line batch. With long-running Claude Code sessions
accumulating thousands of lines, the inability to page back is a significant UX gap against R2.3.

**Option B (separate unary RPC for all scrollback)** was rejected because it requires two
simultaneous connections during the initial load window. The unary response races with live output
arriving on the stream connection; mitigating this requires client-side synchronization that is
equivalent to, but more complex than, the in-stream write-lock approach already needed for Option C.
The new RPC would also leave `ScrollbackRequest`/`ScrollbackResponse` in proto as dead schema.
