# ADR-005: New `onCurrentPaneRequest` Callback for Control-Mode Mid-Stream Resync

**Date**: 2026-08-13
**Status**: Accepted
**Project**: terminal-resync-reliability

## Context

Two structurally distinct server-side streaming paths exist:
`streamViaTmuxCapturePane` (`server/services/connectrpc_websocket.go:1634`), which already
handles a mid-stream `CurrentPaneRequest` including the stale-dimension slow path
(`:1930-2094`), and `streamViaControlMode`/`streamShellViaControlMode`
(`:579`, `:1083`), which share `runInputReadLoop` (`:1488-1497`) — a callback-dispatch loop
with `onInput`/`onResize`/`onScrollbackRequest` but confirmed to have **no**
`onCurrentPaneRequest` callback today. Control-mode sessions currently have no path to
answer a mid-stream resync request with a correlated `resync_id` or a stale-dimension skip
(AC2, AC3) at all.

Client-asserted `stale_dimensions` (rather than a server-side timing heuristic) is also
decided here: `requirements.md`'s own Rabbit Holes section states there is no reliable
server-side signal for "this tab was just backgrounded," so the boolean must originate
client-side from `document.visibilityState` at request-build time.

## Decision

Add a new `onCurrentPaneRequest func(*sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error)`
parameter to `runInputReadLoop`, dispatched from the same message-type switch that already
dispatches `onScrollbackRequest` for `ScrollbackRequest` frames. Both `streamViaControlMode`
and `streamShellViaControlMode` pass a real implementation backed by a shared helper
function (factored out of `streamViaTmuxCapturePane`'s existing dimension-check +
response-construction logic) so behavior is identical across both streaming paths, not
copy-pasted. `CurrentPaneRequest.stale_dimensions` is set by the client, never inferred
server-side.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Duplicate `streamViaTmuxCapturePane`'s ~120-line mid-stream handler inline inside `streamViaControlMode` | Two independent copies of dimension-check/slow-path/response-construction logic that must be kept behaviorally identical forever — the exact fix-flaky/duplicated-logic risk this repo's rules warn against |
| Route control-mode sessions through the capture-pane polling path for resync only | Mixes two streaming strategies within a single session's lifetime, introducing its own coordination bugs (which path "owns" the pane at a given moment) for no benefit over adding one callback |
| Server-side timing heuristic for `stale_dimensions` | `requirements.md`'s Rabbit Holes section explicitly notes no reliable server-side signal exists for this; a client-asserted boolean is unambiguous and cheap to compute at the one call site that already knows `document.visibilityState` |

## Consequences

- `runInputReadLoop`'s signature grows by one parameter; both existing call sites
  (`streamViaControlMode`, `streamShellViaControlMode`) must be updated together
  (Task 3.2.2.2).
- The shared helper (Task 3.2.1.2) becomes the single source of truth for
  dimension-check/stale-dimension-skip/response-construction logic, used by three call
  sites (capture-pane path, control-mode path, shell control-mode path) instead of one.
- A test asserting `onCurrentPaneRequest` is dispatched exactly once per `CurrentPaneRequest`
  frame (Task 3.2.2.3) guards against the callback silently not firing on either control-mode
  variant.
