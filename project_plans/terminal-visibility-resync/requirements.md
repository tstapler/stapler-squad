# Requirements: Terminal visibility/focus resync

Source: backlog item `7728f6df-268a-4578-9066-c300ff69269b` (imported from GitHub issue #154).

## Problem

Backgrounding a browser tab throttles the WebSocket stream; the tmux control-mode
deltas coalesce/drop while hidden. On return, xterm.js's buffer has silently
diverged from the real tmux pane. The next partial repaint (e.g. a TUI redrawing
its option picker) paints over stale cells → visible corruption. The underlying
session is fine; only the client-side render is desynced. Today the only paths
that force a full clean repaint are: (1) initial mount fit, (2) the manual
"Reconnect" button (only rendered while disconnected), (3) `ResizeObserver`-driven
`FitAddon.fit()` (which also triggers a full pane recapture — see below). There is
no `document.visibilitychange` / `window focus` handler anywhere in the terminal
path.

## Codebase reality check (the original issue text is stale)

The issue's suggested fix references `StateApplicator.applyState()` and
`useTerminalSnapshot` as the "existing full-redraw primitive." **Neither exists
for this purpose in the current codebase**:
- `useTerminalSnapshot.ts` exists but is a *session-card preview* hook (fetches
  last-N-lines HTML for the session list), unrelated to the live terminal pane.
- There is no `StateApplicator` class anywhere in the repo.

The actual real full-resync primitive already exists but is **not wired up**:
- `useTerminalFlowControl.requestFullResync(urgent)` (`web-app/src/lib/hooks/useTerminalFlowControl.ts:72`)
  sends a `CurrentPaneRequest` proto message over the stream.
- The server (`server/services/connectrpc_websocket.go:1339-1454`) handles
  `CurrentPaneRequest` by force-capturing fresh tmux pane content and replying
  with a plain `TerminalData_Output` message whose payload is
  `clearAndHome + content` (an ANSI clear-screen+home sequence followed by the
  full pane).
- On the client this arrives via the ordinary `msg.data.case === "output"`
  branch in `useTerminalStream.ts` → `onOutput` → `TerminalOutput.handleOutput`
  → `TerminalStreamManager.write()`, which renders it (the leading clear+home
  produces the full clean repaint).
- This is exactly what the existing resize path already does:
  `useTerminalFlowControl.resize()` sends the resize RPC, then 100ms later sends
  its own `CurrentPaneRequest` — i.e. **every resize already does a full
  resync**, which is why nudging the window size is a known user workaround.
- `requestFullResync`, `markResyncComplete`, `markPaneResponseReceived`, and the
  `isResyncingRef`/`waitingForPaneResponseRef` tracking refs are fully
  implemented in `useTerminalFlowControl.ts` but **`useTerminalStream.ts` never
  exposes `requestFullResync`/`markResyncComplete`/`markPaneResponseReceived` in
  its returned object**, and nothing in `TerminalOutput.tsx` calls them. This is
  dead/unwired code, not a hypothetical.
- `useTerminalStream.ts` already has an *unrelated* `visibilitychange`/`online`
  listener (line ~420-446), gated behind `NEXT_PUBLIC_RECONNECT_V2`, but it only
  reconnects when **disconnected**. It does nothing when the socket is still
  connected but the render buffer has silently diverged — which is the actual
  bug being fixed here.

Given this, the fix is implemented against the real primitives:
`requestFullResync(true)` (full resync while connected) and `connect()` /
`disconnect()` (recovery while disconnected or stalled), not against
nonexistent `StateApplicator`/`resetApplicatorSequence` names. Acceptance
criteria below map the issue's named-function language onto the real
equivalents; test assertions target the real call sites.

## In scope

1. A debounced (~300ms) `visibilitychange`→`visible` + `window focus` handler
   in the terminal connection path (`TerminalOutput.tsx` / `useTerminalStream.ts`)
   that:
   - If connected: triggers exactly one full resync (`requestFullResync(true)`)
     per debounce window, even if both events fire in the same tick.
   - If disconnected: calls `connect()` directly and calls
     `setShowReconnectButton(true)` (covers silent/graceful disconnects where
     `error === null`, which the existing 5s-timeout reconnect-button path
     would otherwise miss or delay).
   - If a resync is requested while connected but no output arrives within
     `RESYNC_STALL_TIMEOUT_MS = 4000ms`, a watchdog clears the pending resync
     state and runs `disconnect().then(() => connect())`.
   - Never moves keyboard focus.
2. Root-cause fix of `useDebouncedCallback` in `useDebounce.ts`: replace the
   `useState`-backed timer id with a `useRef`, and return a memoized (stable)
   callback. The current implementation double-fires when two calls land in
   the same JS tick, because `setTimeoutId` is async — the second call's
   `if (timeoutId)` check reads the still-stale value from before the first
   call's `setTimeoutId`, so it never clears the first timer.
3. Wiring `requestFullResync`/`markResyncComplete`/`markPaneResponseReceived`
   through `useTerminalStream`'s return value so `TerminalOutput` can call
   them, and marking resync complete when the pane-refresh output is received
   after a pending resync (needed for the watchdog to know a resync
   succeeded).

## Out of scope / must not change

- The three pre-existing full-resync/refit triggers: mount-time `fitAddon.fit()`
  in `XtermTerminal.tsx`, the manual Reconnect button's `handleManualReconnect`
  in `TerminalOutput.tsx`, and the `ResizeObserver`-driven fit path in
  `XtermTerminal.tsx`. These must be left byte-for-byte unmodified (AC6);
  verify with a scoped `git diff` against `XtermTerminal.tsx` before shipping.
- `useTerminalStream.ts`'s existing `NEXT_PUBLIC_RECONNECT_V2`
  visibility/online reconnect-when-disconnected listener — left as-is; the new
  logic is additive and only needs to act when that path doesn't (i.e. when
  connected, or as a documented fallback when it's flag-disabled).
- No new dependency; the fix reuses existing hooks/state machinery.
- Scope exception (added during plan validation): one comment-only line in
  `server/services/connectrpc_websocket.go`, cross-referencing the `clearAndHome`
  ANSI constant the AC7 integration test pins against, so a future change to
  that server-side constant doesn't silently desync from the test. No
  behavioral change to any backend file.

## Acceptance criteria (from backlog item, indices match `/backlog/done-N`)

0. Manual repro: returning to a backgrounded tab whose pane was mid-TUI-redraw
   shows a clean terminal with no manual action. Verified by manual repro,
   recorded in the PR description.
1. Regression test: a simulated `visibilitychange`→`'visible'` transition and a
   `window focus` event, even back-to-back in the same tick, trigger exactly
   one full-resync call pair (real equivalent:
   `requestFullResync(true)`, paired with marking/clearing the pending-resync
   state) per ~300ms debounce window.
2. `useDebouncedCallback` fixed at the root (useRef timer id, memoized return)
   with a dedicated same-tick regression test.
3. The resync never steals focus — automated test focuses a sibling input and
   asserts `document.activeElement` is unchanged across the resync.
4. Qualifying transition while `isConnected === false`: fallback calls
   `connect()` directly and calls `setShowReconnectButton(true)` — not merely a
   `connectionAttempts` text change — so a silent/graceful disconnect
   (`error === null`) still triggers real reconnection.
5. Qualifying transition while `isConnected === true` but resync stalls (no
   state response within `RESYNC_STALL_TIMEOUT_MS = 4000ms`): watchdog
   force-clears pending resync flags and runs
   `disconnect().then(() => connect())`. Regression tests for both the
   stall-fires and completes-in-time cases.
6. None of the three pre-existing triggers listed in "Out of scope" are
   modified — confirmed via scoped `git diff` review.
7. An integration test exercises the *real* (non-mocked) full-resync repaint
   path — `TerminalStreamManager` + xterm `Terminal` — to prove that a reset
   (equivalent: `TerminalStreamManager`'s escape-parser reset / buffer clear)
   followed by writing fresh full-pane content (the `clearAndHome + content`
   payload the server sends for `CurrentPaneRequest`) produces a true full
   repaint against a scripted stale/corrupted-buffer scenario — not just
   mocked call-count assertions.
