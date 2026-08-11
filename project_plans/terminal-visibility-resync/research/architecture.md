# Architecture research: terminal visibility/focus resync placement

Research question: given the existing layering (`TerminalOutput.tsx` →
`useTerminalStream.ts` → `useTerminalFlowControl.ts` → `TerminalStreamManager.ts`
→ `XtermTerminal.tsx`), where should the new visibility/focus resync logic live,
and what's the right integration-test target for the full-repaint claim?

All line numbers below are against the state of the tree at research time
(worktree `stapler-squad-terminal-visibility-resync_18c570feb1481981`).

## 1. State ownership at each layer

| Layer | File | State mechanism | What it owns |
|---|---|---|---|
| Component | `web-app/src/components/sessions/TerminalOutput.tsx` | `useState` (UI-facing) + many `useRef` (timers, mount flags, imperative handles) | `showReconnectButton`, `connectionAttempts`, `isWaitingForStableSize`, `isLoadingInitialContent`, `showReconnectBanner` (all local `useState`, lines 81-100); `streamManagerRef` (the `TerminalStreamManager` instance), `xtermRef` (the `XtermTerminalHandle`), `lastResizeRef`, `terminalStateRef`, etc. |
| Hook | `web-app/src/lib/hooks/useTerminalStream.ts` | `useState` for connection lifecycle + refs mirroring it | `isConnected`, `error`, `scrollbackLoaded`, `terminalState` (typed state machine), `isHardFailed` — all `useState` (lines 88-93). Refs: `isConnectedRef` (mirror of `isConnected`, synced via effect at 118-120), `shouldReconnectRef`, `isDisconnectingRef`, `terminalBackoffRef`, `reconnectTimerRef`, `connectRef` (stable ref to latest `connect` closure for listeners), `terminalDebounceTimerRef`. |
| Sub-hook | `web-app/src/lib/hooks/useTerminalFlowControl.ts` | **Refs only — no React state at all** | `isResyncingRef`, `waitingForPaneResponseRef`, `lastResyncTimeRef`, `lastResizeTimeRef`, `pendingResizeTimerRef`, `dimensionSyncRef` (lines 41-46). Exposes `requestFullResync`, `markResyncComplete`, `markPaneResponseReceived`, and two **ref-getters** `getIsResyncingRef()` / `getWaitingForPaneResponseRef()` (lines 316-317) rather than the ref objects directly. |
| Imperative class | `web-app/src/lib/terminal/TerminalStreamManager.ts` | Private class fields, no React | `writeBuffer`, `pendingWrites`, `watermark`, `isPaused`, `writeQueue`, `isWritingInitialContent`/`pendingLiveWrites` (write-lock for scrollback bootstrap), an `EscapeSequenceParser` instance, a `RedrawThrottler` instance. Lives inside `streamManagerRef` (owned by `TerminalOutput.tsx`), constructed lazily by `getOrCreateStreamManager()`. |
| React wrapper | `web-app/src/components/sessions/XtermTerminal.tsx` | Refs for the real `Terminal`/`FitAddon`/`SerializeAddon`; `useState` only for a few cosmetic concerns | Exposes `XtermTerminalHandle` via `useImperativeHandle` (line 874): `terminal`, `serializeAddon`, `write/writeln/clear/focus/fit/search*`. Owns the mount-time `fitAddon.fit()` (~442) and the `ResizeObserver`-driven fit (~724-820) — two of the three protected triggers. |

Key structural fact: **`useTerminalFlowControl` already establishes the pattern
of crossing a hook boundary via ref-getters, not raw refs or prop-drilled
setters.** `useTerminalStream.ts`'s own `disconnect()` already reaches into
`flowControl.getIsResyncingRef()` (line 358, used at 365) to decide whether to
delay disconnect during an in-flight resync. Any new code that needs
`isResyncingRef`/`waitingForPaneResponseRef` should use the same getter
pattern, not add new plumbing.

**`useTerminalStream.ts`'s returned `TerminalStreamResult` (lines 54-71,
built at 457-473) currently does NOT expose `requestFullResync`,
`markResyncComplete`, or `markPaneResponseReceived`.** Confirmed by reading
the return statement — only `sendInput, resize, connect, disconnect,
scrollbackLoaded, requestScrollback, sendFlowControl, startRecording,
stopRecording, terminalState, isHardFailed, handleManualReconnect` are
returned. This exactly matches requirements.md's claim; wiring these three
through is in-scope item 3 and a hard prerequisite for anything below.

## 2. Where the new visibilitychange/focus listener belongs

**Recommendation: `TerminalOutput.tsx`, not `useTerminalStream.ts`.**

Reasoning:

- `showReconnectButton` is local `useState` that exists **only** in
  `TerminalOutput.tsx` (line 82). `useTerminalStream.ts` has no visibility
  into it and no channel to set it except a new callback prop threaded down
  — which would create a second, parallel path to the same piece of state
  alongside the existing `isConnected`-transition effect that already sets it
  (lines 667-733, specifically `setShowReconnectButton(true)` inside the
  `wasConnected && !isConnected` branch at ~719 and `setShowReconnectButton(false)`
  at ~678 on reconnect). Two effects independently setting the same `useState`
  from different files is a correctness hazard (race on which one "wins",
  harder to reason about) that placement in the same file avoids entirely.
- The existing `NEXT_PUBLIC_RECONNECT_V2` listener in `useTerminalStream.ts`
  (lines 420-446) is explicitly listed as **out of scope / must not change**,
  and it solves a narrower, disjoint problem: reconnect-when-disconnected
  only, gated behind a flag, with no concept of "connected but stale" at all
  (it can't call `requestFullResync` because that's not threaded through the
  hook's return value, and the hook's internals never call `flowControl.*`
  functions on their own initiative — see below). Its placement is not
  strong evidence for symmetry: it was written before the resync primitives
  existed in scope, addresses a strict subset of the new requirement, and the
  new requirement's connected-branch is unrelated to what it does.
- Every `flowControl.*` capability — `sendInput`, `resize`,
  `requestScrollback`, `sendFlowControl`, and (once wired through) 
  `requestFullResync`/`markResyncComplete`/`markPaneResponseReceived` — is
  presently *called from* `TerminalOutput.tsx`, never from inside
  `useTerminalStream.ts` itself. `useTerminalStream.ts`'s own logic only
  reads `flowControl.getIsResyncingRef()` for its `disconnect()` guard; it
  never *initiates* a flow-control action. Adding a resync-initiating
  listener inside `useTerminalStream.ts` would be the first exception to that
  one-directional flow (hook exposes capabilities → component decides when to
  invoke them), whereas adding it in `TerminalOutput.tsx` is a natural
  continuation of the existing pattern — it already owns exactly this class
  of responsibility ("on a connection-relevant transition, decide what to do
  next") in the connection-state effect at line 667.
- The stall watchdog's fallback (`disconnect().then(() => connect())`) needs
  both `disconnect` and `connect`, which `TerminalOutput.tsx` already holds
  as destructured values from `useTerminalStream()` (line 432) — zero new
  plumbing required if the listener lives there.

Net effect on `useTerminalStream.ts`: no changes to its existing
visibility/online effect (satisfies "leave as-is"); it only needs to grow its
return object by three fields (`requestFullResync`, `markResyncComplete`,
`markPaneResponseReceived`) sourced directly from `flowControl` — a pure
addition, matching in-scope item 3 and not touching the protected lines.

## 3. Detecting "resync completed" for the stall watchdog

`handleOutput` (`TerminalOutput.tsx` ~409) and the `output` case in
`useTerminalStream.ts` (~244-268) are the single funnel for **all** terminal
bytes — both ordinary live-streamed output and the `CurrentPaneRequest`
resync response arrive as the identical `msg.data.case === "output"` shape
with no discriminator. Confirmed server-side: both the resize-triggered
resync (`connectrpc_websocket.go:1409` `fullContent := clearAndHome +
content`) and plain streamed content (`:1204`, same pattern) are wrapped in
an identical `TerminalData_Output{ Data: []byte(...) }` — no request ID, no
type tag, no correlation field on the proto message.

Given that, **a ref-based "pending resync, treat next output as completion"
flag is architecturally sound** — and it's not a new pattern being
introduced, it's the same one `useTerminalFlowControl` already uses for
itself (`waitingForPaneResponseRef` / `isResyncingRef`, refs rather than
state, precisely because there's no strongly-typed completion signal to key
off of). The resize path already relies on this exact assumption today:
`resize()` sends the resize RPC, then 100ms later pushes its own
`currentPaneRequest` (lines 219-237 of `useTerminalFlowControl.ts`) and
simply trusts that the *next* output message is that pane's response. That
trust is well-founded architecturally because delivery is over a single
`for await (const msg of stream)` loop over one ordered WebSocket-backed
proto stream (`useTerminalStream.ts` line 211) — messages cannot be
reordered relative to each other on that connection, so "next output after I
requested a resync" is a sound proxy for "this is the resync response," not
a hack.

Implementation shape that follows from this: in `handleOutput`, immediately
after `manager.write(output)`, check the (now-exposed) resync-pending ref; if
set, call `markPaneResponseReceived()`/`markResyncComplete()` and clear the
local `RESYNC_STALL_TIMEOUT_MS` watchdog timer. One caveat worth a code
comment: this makes "resync complete" a heuristic ("the very next output of
any kind completes it") that is only safe *because* of the single-ordered-
stream guarantee inherited from `useTerminalStream`'s connection model — it
is not independently re-verified by content inspection (e.g. checking for
the `clearAndHome` prefix), so if that ordering invariant were ever broken
(e.g. a future multiplexed/out-of-order transport) this detection would
silently degrade. Not a blocker for this fix, but worth flagging in the
implementation comment for future maintainers.

## 4. Integration test target for AC7 (full-repaint proof)

**`writeInitialContent()` is the wrong target — it is not the code path the
resync feature uses.** Per requirements.md's own traced call chain, the
`CurrentPaneRequest` response is a plain `output` message that flows through
`TerminalOutput.handleOutput` → `TerminalStreamManager.write()`. Confirmed
by reading `TerminalOutput.tsx`: `handleOutput` (~409-421) calls
`manager.write(output)`, full stop — `writeInitialContent()` is called only
from `handleScrollbackReceived` (line 378) for the one-time initial
scrollback bootstrap on session mount, a completely different trigger with
different semantics: it does an explicit JS-level `terminal.clear()` call
(line 260 of `TerminalStreamManager.ts`) and holds the `isWritingInitialContent`
write-lock that defers concurrently-arriving live writes — none of which the
resync flow exercises or needs.

The actual "reset" primitive in the resync flow is **not** anything inside
`TerminalStreamManager` — it's embedded in the ANSI payload itself and
performed by xterm.js's own VT parser. Server-side, `ansiSnapshotPrefix =
ansiDECSTR + ansiEraseScreen + ansiCursorHome` (DECSTR soft-reset, ED2
erase-entire-screen, CUP cursor-home; `connectrpc_websocket.go:126-129`) is
prepended to the fresh pane content for *every* `CurrentPaneRequest`
response, including the resize-triggered one (`:1409`) and the dedicated
resync one. Client-side, that whole string is handed to
`manager.write(clearAndHome + content)`, which routes through
`RedrawThrottler.process()` → `EscapeSequenceParser.processChunk()` →
`terminal.write()` (`TerminalStreamManager.ts` lines 228-248,
360-396) — i.e. plain `xterm.js` interpreting DECSTR/ED2/CUP in its own
buffer state machine is what performs the repaint. `TerminalStreamManager`
calls neither `terminal.clear()` nor `this.escapeParser.reset()` anywhere on
this path. `escapeParser.reset()` is called only from `cleanup()` (unmount /
disconnect teardown, line 544) and, on inspection of
`EscapeSequenceParser.ts` (lines 63-68), does something unrelated regardless
— it only discards a buffered *partial* multi-chunk escape sequence
(`this.partialSequence = ""`); it never touches xterm's screen buffer.

So requirements.md's AC7 parenthetical — "a reset (equivalent:
`TerminalStreamManager`'s escape-parser reset / buffer clear)" — is
imprecise: neither of those two `TerminalStreamManager` mechanisms
(`escapeParser.reset()`, `writeInitialContent()`'s `terminal.clear()`) is
actually invoked by the resync flow. The "reset" is a single `write()` call
carrying a payload that begins with a screen-clearing ANSI sequence, not two
separate method calls.

**Correct integration test target:** call `TerminalStreamManager.write()` —
not `writeInitialContent()` — against a real, non-mocked `@xterm/xterm`
`Terminal` instance:
1. Pre-seed the terminal with "stale/corrupted" content (e.g.
   `manager.write(...)` a partial/garbled TUI redraw fragment, simulating
   what a backgrounded tab's coalesced/dropped deltas would leave behind).
2. Call `manager.write(clearAndHome + freshPaneContent)` using the *exact*
   payload shape the server sends (`DECSTR + ED2 + CUP` prefix, matching
   `ansiSnapshotPrefix`), not a synthetic simplified clear sequence.
3. Assert via xterm's real buffer API (`terminal.buffer.active.getLine(n)
   .translateToString()`) that the rendered lines equal only the fresh
   content, with no leftover glyphs from step 1 — proving xterm.js's ANSI
   engine performed a genuine full repaint. This directly satisfies AC7's
   "not just mocked call-count assertions" requirement, because it observes
   the actual rendered buffer rather than asserting that some reset method
   was called N times.

One thing worth flagging explicitly in the test's own comments: because
there is no first-party "reset" primitive to unit test (no
`StateApplicator`, and `TerminalStreamManager` doesn't call
`terminal.clear()`/`escapeParser.reset()` on this path), this test is
inherently pinning **xterm.js's** interpretation of DECSTR/ED2/CUP, not
stapler-squad's own logic. That's exactly what AC7 asks for ("integration
test... `TerminalStreamManager` + xterm `Terminal`"), but it does mean a
future `@xterm/xterm` upgrade that changes ANSI-reset semantics could break
this test's premise even with zero changes to this repo's code — that's a
legitimate and different risk profile from a first-party unit test, and
should be called out as such rather than treated as a flaky/spurious
failure if it ever trips.

## Summary of file/line references for implementation

- `web-app/src/lib/hooks/useTerminalStream.ts:54-71,457-473` — add
  `requestFullResync`, `markResyncComplete`, `markPaneResponseReceived` to
  `TerminalStreamResult` and the return object, sourced from `flowControl`.
  Do not touch the `NEXT_PUBLIC_RECONNECT_V2` listener at lines 420-446.
- `web-app/src/components/sessions/TerminalOutput.tsx` — new
  `visibilitychange`/`focus` listener lives here, near the existing
  connection-state effect (lines 667-733) that already owns
  `showReconnectButton`. Stall watchdog uses `disconnect`/`connect` already
  destructured at line 432. `handleOutput` (~409-421) gets the
  resync-completion check.
- `web-app/src/lib/hooks/useDebounce.ts:31-56` — root-cause fix: replace
  `useState<NodeJS.Timeout | null>` timer id with `useRef`, wrap
  `debouncedCallback` in `useCallback` so it's referentially stable. Currently
  unused anywhere in the codebase (confirmed via repo-wide search) — this fix
  is a pure prerequisite with no existing call sites to break.
- `web-app/src/lib/terminal/TerminalStreamManager.ts:228-248` (`write()`) —
  integration test target for AC7, not `writeInitialContent()`
  (lines 257-277).
- `web-app/src/components/sessions/XtermTerminal.tsx:442` (mount fit),
  `~724-820` (ResizeObserver fit) — confirmed as the two of three protected
  triggers living in this file; the third (`handleManualReconnect`) is in
  `TerminalOutput.tsx`. None of these need touching for this fix.
