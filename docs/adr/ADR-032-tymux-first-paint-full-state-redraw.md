# ADR-032: A tymux session's first paint is a full-state redraw, captured fresh per attach

Status: Proposed — 2026-10-29

## Context

A browser tab that opens on a running tymux session used to receive only a text paint: `CapturePaneContentRaw` joins the
snapshot's cell text and SGR, then `ansiSnapshotPrefix` + `prepareSnapshotContent` + `withCursorSync` wrap it. The new
terminal never saw the pane's modes (application cursor keys, bracketed paste, mouse), title, alt-screen flag, scroll
region, tab stops, charsets, cursor style/visibility or pending wrap, so a TUI that set them before the tab opened
(Claude Code, vim) rendered and responded wrongly until it redrew everything itself.

`session/tymux/redraw.go`'s `RenderSnapshotRedraw` already turns a `PaneSnapshot` into exactly that stream and is
round-trip tested against xterm.js (`session/tymux/testdata/fidelity`). Nothing on the browser path called it.

Six places send a browser a full-screen snapshot:

| Site | Path |
|---|---|
| `StreamHub.AttachSubscriber` catch-up (`sendCatchUpSnapshot`) | hub, non-browser transports only |
| `StreamHub.applyNegotiatedSize` broadcast | hub |
| `sendHubInitialSnapshot` | hub, browser |
| `captureAndSendInitialSnapshot` | control mode, browser |
| `sendPostResizeSnapshot` | control mode, browser |
| `handleCurrentPaneRequest` (client resync) | both |

## Decision

1. **Capability, not a type assertion.** `session.RedrawCapturer` (backend side, `process_capabilities.go`) is satisfied by
   `TymuxBackend`; `Instance.CapturePaneRedraw` returns `(streamhub.FullStateSnapshot, ok, err)`. `streamhub.FullStateSnapshotter`
   is the optional seam the hub and the handler assert with comma-ok, so tmux, shell tabs and every other controller are
   untouched.
2. **Sent as-is.** The redraw resets state, positions the cursor and arms pending wrap itself, so none of the legacy
   prefix, CRLF rewrite or cursor sync is applied on top. Its alt-screen flag is stamped on the frame and recorded on the
   `Instance` (`SetAltScreenActiveBootstrap`) so later input routing agrees.
3. **Fresh `CapturePane` per attach, and never in the text cache.** A cached redraw would hand a later attacher the modes and
   cursor of whenever it was taken, and a later failed capture would fall back to a stale redraw dressed as text. The hub's
   `lastSnapshot` holds text only. One capture is one RPC to the local daemon, bounded by `fullStateCaptureTimeout` (3 s).
4. **Fallback is decided by the backend.** `tymuxGRPCSession.CapturePaneRedraw` returns `ok=false` when `snap.Modes == nil`:
   a daemon older than v1.9.0 sends no terminal state, and `RenderSnapshotRedraw`'s no-state branch paints grid and cursor
   only, with none of the reset the legacy prefix performs. `ok=false`, a missing capability, or a failed capture all take
   the unchanged legacy text path, so a connection always gets a paint.
5. **One capture per browser attach on the hub path.** `SubscriberCapability.ManualFirstPaint` makes the hub skip the
   attach-time catch-up (it was captured, then discarded by the old `SuppressNextSend` hack, then captured again by the
   handler) and hold the subscriber's output. The handler then calls `StreamHub.FirstPaint`, which:
   - takes `resizeApplyMu`, so no resize can capture-and-broadcast between the paint's capture and its write;
   - drops frames broadcast before the paint begins (an older resize snapshot, or output the capture already contains);
   - runs the handler's capture-and-write, holding frames that arrive meanwhile (up to `maxPaintBacklog`, then the paint
     fails visibly) and having the writer send them right after the paint, before switching to the live queue.

   While every subscriber is still waiting for its first paint, `applyNegotiatedSize` applies the resize but skips its own
   capture, which the paint would discard. `WebSocketTransport.SuppressNextSend` is removed.
   **This applies to tmux-backed sessions too.** `attachHubSubscriber` sets `ManualFirstPaint` for every hub browser
   subscriber, whatever the backend. A tmux subscriber therefore gets no attach-time catch-up, its writer starts only after
   `FirstPaint`, frames queued before the paint are discarded, and `applyNegotiatedSize` skips its capture while every
   subscriber awaits its first paint. tmux has no `RedrawCapturer` (only `TymuxBackend` implements it, so
   `Instance.CapturePaneRedraw` returns `ok=false`); `FirstPaint`'s paint callback then runs the unchanged text path
   (`CapturePaneContentRaw` + `ansiSnapshotPrefix` + `prepareSnapshotContent` + `withCursorSync`), so what the tmux
   subscriber receives is the same bytes as before, from one capture instead of two, ahead of any live output.
6. **Timestamps follow the legacy rule.** Only the control-mode initial snapshot calls `UpdateTerminalTimestamps`; an attach
   to the hub, a resize, or a resync of an idle session must not reset `LastMeaningfulOutput` or fire `SessionEventOutput`.
7. **One first-paint wait budget.** `captureAndSendInitialSnapshot` computes a single deadline shared by the redraw attempt
   and the text fallback (5 s total), instead of letting each poll for 5 s.

## Initial scrollback (verified by reading the code)

`sendInitialScrollback` is called only from `streamViaControlMode`, after the initial snapshot, with the handler's own
`ScrollbackManager` replay of raw live output. The web client's `handleScrollbackReceived` →
`TerminalStreamManager.writeInitialContent` calls `terminal.clear()` before writing it, which would replace a redraw with an
older replay. The hub path (the default) sends no initial scrollback, and the client only requests scrollback itself when
`hasMoreScrollbackRef`/`hasMoreAppScrollbackRef` are set, which only a prior scrollback response sets, so nothing clears the
redraw at connect there. A client-initiated tymux request would return one screen of plain text
(`CapturePaneContentWithOptions` → `rowsToPlainText`), which is a separate gap: tymux has no history API.

Choice: when the control-mode first paint was a redraw, `streamViaControlMode` skips `sendInitialScrollback`. The wiring is a
one-line `if !fullState`; `captureAndSendInitialSnapshot`'s `fullState` result is unit-tested, the call in
`streamViaControlMode` is not, because exercising it needs a live control-mode session.

## Ordering and double-apply

The redraw is an independent `CapturePane` while live output keeps flowing. tymux's attach-stream priming snapshot (tymux
ADR-003) avoids double-apply with a sequence number, but `PaneSnapshot` has no sequence field, so that dedupe cannot apply
here; `session/tymux/resync.go` calls the same pattern a double-render race. This design is no worse than the text paint it
replaces and narrows the window where it cheaply can:

- **Hub path.** The subscriber is registered before the capture, but now cannot be written to until its paint is out
  (decision 5). Frames queued before the paint's start are dropped as already captured, so only output in flight in the pump
  (emitted by the daemon, not yet broadcast by the hub) can be applied twice. Resize snapshots can no longer land after an
  older paint.
- **Control-mode path.** Unchanged from the text paint: output frames are dropped while `forwardingReady` is false and during
  `resizeSettling`. Output that the daemon emitted before the capture and the pump delivers after `forwardingReady` flips can
  still be applied twice.

Follow-up (not in this change): add a sequence number to `PaneSnapshot` in tymux and drop already-reflected frames on the Go
side, as ADR-003 does for the attach stream. That closes the remaining window on both paths.

## Rejected

- **Reuse ADR-003's attach-stream priming snapshot.** It is primed once per standing attach stream with a sequence-based skip,
  not per browser connection, so a second tab attaching later has no priming snapshot to reuse.
- **Cache the redraw like the text snapshot.** Stale modes/cursor, as above.
- **Apply the legacy prefix to the redraw.** The prefix's erase+home would clear the alt-screen/scroll-region state the redraw
  has just restored.
- **A capture-order ticket checked before a direct write.** It cannot stop an older frame already queued in the subscriber's
  writer from landing after a newer direct write; holding the writer until the paint is out can.

## Consequences

- `FirstPaint` holds `resizeApplyMu` across one capture RPC and one socket write, so another subscriber's resize waits that
  long. The write carries a 5 s deadline (`firstPaintWriteTimeout`, `WriteMessageTimeout`), so a stalled client socket cannot
  hold the lock indefinitely; other connection writes still have no deadline.
- On the hub path, a redraw-capable session costs one capture per browser attach, down from two plus a resize capture.
  tmux sessions also lose the wasted attach-time capture. Non-browser transports keep the catch-up, which holds
  `scrollForwardMu` for its capture.
- While a subscriber awaits its `FirstPaint` it has no writer, so `trySend` drops frames as superseded rather than queueing
  them (a full queue would otherwise trip the 250 ms slow-subscriber eviction when `FirstPaint` is delayed behind
  `resizeApplyMu` or a slow capture). If `FirstPaint` is never called, the subscriber is evicted after
  `defaultFirstPaintMaxAwait` (30 s); the check shares a lock with `beginFirstPaint`, so it cannot evict mid-paint.
- **A failed capture paints blank, it does not end the connection.** `writeHubInitialSnapshot` retries the text capture for up
  to `hubCaptureRetryWindow` (1 s; the paint holds the resize lock), then paints an empty snapshot and lets live frames
  follow. Failing the connection instead made a session whose capture keeps failing (e.g. `ErrSessionNotStarted` mid-resume)
  reconnect-loop while its live output worked. Frames dropped before the paint are lost in that case, as they were before
  this ADR; frames held during the paint are still replayed in order.
- **Frames during the paint are held, not queued.** From `beginFirstPaint` until the writer has replayed them, frames go to a
  bounded side list, not the 100-deep outbound queue, so a long capture or a stalled socket cannot trip the slow-subscriber
  eviction. The writer sends the list before the queue and switches over under the same lock, so no frame falls between the
  two. More than `maxPaintBacklog` (4096) frames, or `maxPaintBacklogBytes` (16 MiB) held, fail the connection rather than
  leaving a gap. The byte bound exists because a batch unit is uncapped (a flush carries everything read in its 20 ms
  window), so a bulk `cat` during a paint of up to ~9 s (1 s capture retry, 3 s capture, 5 s write) could otherwise hold
  gigabytes per subscriber.
- **Replay honours the slow-subscriber grace.** Once the paint returns, the held list may grow by only one queue's worth
  (`drainLimit`) beyond what it held; further frames are dropped and arm the usual 250 ms eviction, which fires if the list is
  still at its limit and the writer has sent nothing from the held batch since the stall was seen. A socket that stalls
  during replay is therefore evicted, as one stalling on the live queue is, while a slow client still moving through one
  long batch is not (the list only shrinks at batch boundaries, so progress is counted per `Send`;
  `TestFirstPaint_should_NotEvictSubscriber_When_ReplayIsSlowButProgressing`).
- **Residual duplicate window.** A frame broadcast after `FirstPaint` starts but while the capture RPC is in flight (up to
  `fullStateCaptureTimeout`, 3 s) is held and replayed after the paint, and may already be in the capture, so it can be
  applied twice. No clean cutoff exists: the hub cannot observe when, relative to the pump's frames, the daemon took its
  snapshot, so any sequence cutoff would trade this duplicate for dropping a frame that is not in the capture. At-least-once
  was chosen over possible loss; `TestFirstPaint_should_DeliverFrameBroadcastDuringPaintAfterPaint` pins it and
  `TestFirstPaint_should_LeaveNoGap_When_FramesRaceTheCapture` checks that nothing after the capture is lost.
- A control-mode connection whose first paint is a redraw gets no initial scrollback.

## Verification

Unit tests per layer: `session/tymux/session_test.go` (including the no-`Modes` decline), `session/redraw_capture_test.go`
(backend and `Instance` wiring), `session/streamhub/full_state_test.go` (no catch-up capture, discard, ordering, resize
blocking, skip-capture, no stale cache), `server/services/connectrpc_websocket_fullstate_test.go` and
`connectrpc_websocket_firstpaint_test.go` (each handler site with and without the capability, timestamps, wait budget). One
end-to-end test, `TestFirstPaint_should_RestoreTerminalStateInXterm_EndToEnd`, replays the real `Instance` → hub → WebSocket
bytes into xterm.js and checks grid, title, alt screen, modes, scroll region and cursor.
