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

Four sites send the browser a full-screen snapshot: the hub's `sendHubInitialSnapshot`, the legacy path's
`captureAndSendInitialSnapshot`, `sendPostResizeSnapshot` and `handleCurrentPaneRequest` (client resync). The hub also
broadcasts a snapshot after a negotiated resize (`applyNegotiatedSize`) and serves `AttachSubscriber`'s catch-up.

## Decision

1. **Capability, not a type assertion.** `session.RedrawCapturer` (backend side, `process_capabilities.go`) is satisfied by
   `TymuxBackend`; `Instance.CapturePaneRedraw` returns `(streamhub.FullStateSnapshot, ok, err)` with `ok == false` when
   the backend has no redraw. `streamhub.FullStateSnapshotter` is the optional seam the hub and the handler assert with
   comma-ok, so tmux, shell tabs and every other controller are untouched.
2. **Sent as-is.** The redraw resets state, positions the cursor and arms pending wrap itself, so none of the legacy
   prefix, CRLF rewrite or cursor sync is applied on top. Its alt-screen flag is stamped on the frame and recorded on the
   `Instance` (`SetAltScreenActiveBootstrap`) so later input routing agrees.
3. **Fresh `CapturePane` per attach, never cached.** A cached redraw would hand a later attacher the modes and cursor of
   whenever it was taken. The capture is one RPC to the local daemon, bounded by `fullStateCaptureTimeout` (3 s).
4. **Fallback.** No capability, `ok == false` (daemon older than v1.9.0, whose snapshots have `Modes == nil`), or a failed
   capture falls back to the unchanged legacy text path, so a connection always gets a paint.

## Rejected

- **Reuse ADR-003's attach-stream priming snapshot.** It is primed once per standing attach stream with a
  sequence-based skip (tymux repo ADR-003), not per browser connection, so a second tab attaching later has no priming
  snapshot to reuse.
- **Cache the redraw like the text snapshot.** Stale modes/cursor, as above.
- **Apply the legacy prefix to the redraw.** The prefix's erase+home would clear the alt-screen/scroll-region state the
  redraw has just restored.

## Consequences

- A second `CapturePane` RPC per attach/resize/resync on tymux; the text snapshot cache is bypassed for those sessions.
- On a failed or declined redraw `captureAndSendInitialSnapshot` can poll twice (redraw, then text) before giving up on a
  session that never starts: up to about 10 s of spinner instead of 5 s.
- Resync via `handleCurrentPaneRequest` now also echoes `AltScreenActive` for redraw frames.

## Verification

Unit tests per layer (`session/tymux/session_test.go`, `session/streamhub/full_state_test.go`,
`server/services/connectrpc_websocket_fullstate_test.go`) cover capability, verbatim delivery, no caching, and each
fallback. The redraw bytes themselves are covered by the existing xterm.js fidelity suite.
