# Research: existing reconnect/resync-on-visibility patterns + edge cases

## 1. Existing `document.hidden`/`visibilitychange` usages in the codebase

Grep for `document.hidden|visibilitychange|visibilityState` across `web-app/src` hits:
- `web-app/src/lib/hooks/useVcsStatus.ts`
- `web-app/src/lib/hooks/useSessionVcs.ts` (via shared polling helper)
- `web-app/src/lib/hooks/useReviewQueueNotifications.ts`
- `web-app/src/lib/hooks/useTerminalStream.ts` (the one true event-listener pattern)
- test files for the above

**`useVcsStatus.ts` (lines 99-105)** and **`useReviewQueueNotifications.ts` (line 125, 279)**: both only use `document.hidden` as a **gate check inside a `setInterval` polling callback**, e.g.:
```ts
const interval = setInterval(() => {
  if (!document.hidden) fetchVcs();
}, pollIntervalMs);
```
Neither registers a `visibilitychange` **event listener**. There is no debounce, no cleanup of a listener (nothing to clean up), and no "on becoming visible, do X" transition logic — they just skip a poll tick while hidden and pick back up on the next interval tick. **Not a reusable pattern for edge-triggered resync-on-visible** — it solves a different problem (don't waste polling requests while backgrounded), not "detect the visible transition and react once."

**Conclusion for Q1**: there is no existing "shared visibility hook" to mirror. The only real edge-triggered listener pattern in the repo is the one already inside `useTerminalStream.ts` itself (see Q4 below) — that is the pattern to mirror, not the VCS/review-queue polling gates.

## 2. `TerminalOutput.tsx` connection lifecycle — state and race conditions

Key state (`web-app/src/components/sessions/TerminalOutput.tsx`):
- `isConnected`, `error`, `terminalState` (`DISCONNECTED|CONNECTING|LOADING|STABLE|RESIZING|FETCHING_SCROLLBACK`) — all come from `useTerminalStream`.
- `connectionAttempts` (state), `showReconnectButton` (state), `isHardFailed` (from hook), `showReconnectBanner` (2s-delayed banner), `hasEverConnectedRef`, `hasInitiatedConnectionRef`, `pendingConnectAfterDisconnectRef`, `previousConnectionStateRef`.
- `isVisible?: boolean` prop — **pool-level** visibility (see Q5), unrelated to `document.visibilityState`.

Lifecycle hot spots (line refs from current tree):
- `useTerminalStream.ts:172` — `connect()` sets `terminalState = 'CONNECTING'` synchronously but `isConnected` only flips true on **first message received** (`useTerminalStream.ts:213`, inside the async stream loop) — there is a real window where `terminalState !== 'DISCONNECTED'` but `isConnected` is still `false`.
- `useTerminalStream.ts:221-229` — `resizeQuiescence` messages toggle `RESIZING`⇄`STABLE`.
- `useTerminalStream.ts:268` — **any** `output` message flips `LOADING`/`CONNECTING` → `STABLE`.
- `TerminalOutput.tsx:667-733` — on `isConnected` false→true, schedules a **250ms-delayed** post-connect `resize()` call (which itself triggers the resize path's own 100ms-delayed `CurrentPaneRequest`, per `useTerminalFlowControl.ts:220-237`).
- `TerminalOutput.tsx:711-722` — on true→false (when `NEXT_PUBLIC_RECONNECT_V2` is off), arms a 5s timer before `showReconnectButton(true)`.
- `TerminalOutput.tsx:927-992` — session-switch effect: resets `isLoadingInitialContent`, `hasInitiatedConnectionRef`, `connectionAttempts`, `showReconnectButton`, tears down `streamManagerRef`, and either connects immediately or sets `pendingConnectAfterDisconnectRef` (completed by the effect at `996-1008`) if a disconnect is still in flight. A 5s safety-timeout also exists for containers hidden at mount.
- `useTerminalFlowControl.ts:72-127` — `requestFullResync(urgent)`: **no-ops with a console.warn** (not a throw) if `!isConnectedRef.current` or `getTerminal()` returns null; internally throttles non-urgent calls to 1 per 2000ms via `lastResyncTimeRef`, bypassed when `urgent=true`.
- `useTerminalFlowControl.ts:196-253` — `resize()` has its own **200ms throttle-with-trailing-defer** and its own pending-timer ref (`pendingResizeTimerRef`), fully independent of any future resync-watchdog timer.

**Concrete races a naive visibility handler would hit**:
1. **Tab backgrounded during initial connect, before `isConnected` is ever true.** `terminalState` may be `CONNECTING`/`LOADING` with `isConnected === false`. A visibility handler that only branches on `isConnected` would take the "disconnected" fallback (`connect()` + `showReconnectButton(true)`) while a connect is already in flight — `connect()` in `useTerminalStream.ts:154` is idempotent-guarded (`if (isConnectedRef.current || !sessionId) return;`) so a second call while genuinely connecting is a no-op, but `showReconnectButton(true)` would incorrectly show a reconnect button during a still-succeeding first connect.
2. **Resync fired while a resize-triggered resync is already in flight.** The resize path already sends its own `CurrentPaneRequest` 100ms after every resize (`useTerminalFlowControl.ts:220-237`) and sets `waitingForPaneResponseRef`/`isResyncingRef`. A visibility-triggered `requestFullResync(true)` landing in that same window bypasses the throttle (urgent=true) and fires a second `CurrentPaneRequest` — two pane captures in flight, and whichever `output` arrives second "wins" as the terminal content (harmless in effect since both are full repaints of the same pane, but doubles server load and could race the watchdog's stall-timeout bookkeeping since there's only one `isResyncingRef`/`waitingForPaneResponseRef` pair for both).
3. **`terminalState === 'RESIZING'`.** Output is queued client-side during `RESIZING` (`TerminalOutput.tsx:407-464`, "Task 4.2.2"). If the visibility resync's `output` (clearAndHome+content) arrives while `RESIZING`, it gets queued rather than written immediately, and the watchdog's `RESYNC_STALL_TIMEOUT_MS` timer isn't aware of the queue — it could fire a spurious "resync stalled" disconnect/reconnect even though the response is sitting in the queue waiting to flush on `RESIZING→STABLE`.
4. **Multiple session switches while backgrounded.** `TerminalOutput.tsx:927` (session-switch effect) resets `hasInitiatedConnectionRef`/`showReconnectButton`/`connectionAttempts`/tears down `streamManagerRef` on every `sessionId` change — including switches that happen while the tab is hidden. A resync/watchdog timer armed for session A that fires after the component has already re-run this effect for session B would be operating on stale refs unless the resync code itself resets/cancels pending resync + watchdog timers in that same effect (the plan must add this cleanup explicitly — it's not covered by any existing cleanup path since `requestFullResync`/watchdog don't exist yet).
5. **No `output` arrives at all (true stall).** Because `isResyncingRef`/`waitingForPaneResponseRef` are refs (not React state) and nothing currently clears them (confirmed: zero call sites of `markResyncComplete`/`markPaneResponseReceived` in `TerminalOutput.tsx` or `useTerminalStream.ts` today — fully dead code), a resync request with no matching watchdog would leave `isResyncingRef.current === true` forever, silently blocking future resyncs (the AC5 stall-watchdog exists specifically to bound this).

## 3. `useDebouncedCallback` — other call sites and behavior-change risk

`useDebounce.ts` exports two hooks: `useDebounce` (value debounce, `useState`-based, **not** the buggy one) and `useDebouncedCallback` (the one with the bug).

Grep for `useDebouncedCallback` **call sites** across the whole repo (`web-app/src`, excluding its own definition) returns **zero matches**. Grep for imports of `useDebounce` from any other file finds only `useDebounce` (the value-debounce hook), used in:
- `web-app/src/app/logs/page.tsx:57` — `useDebounce(searchQuery, 300)`
- `web-app/src/lib/hooks/useHistoryFullTextSearch.ts:110` — `useDebounce(query, debounceMs)`

Both of those use `useDebounce`, not `useDebouncedCallback`. **`useDebouncedCallback` currently has no callers anywhere in the codebase** — it's dead code today. There is also no existing test file for `useDebounce.ts`.

**Conclusion for Q3**: fixing the `useRef`-vs-`useState` double-fire bug in `useDebouncedCallback` carries **zero regression risk to existing callers**, because there are none. This terminal-visibility-resync feature will be the *first* real consumer. (Still worth writing the dedicated same-tick regression test per AC2, both to prove the fix and to lock in behavior for the terminal caller and any future ones.)

## 4. Same-tick `visibilitychange` + `focus` dedup — existing pattern

`useTerminalStream.ts:420-446` (Story 3.1.3, gated behind `NEXT_PUBLIC_RECONNECT_V2`) is the one existing example of visibility-driven reconnect, and it **already demonstrates a dedup-capable shape**, though it only listens to `visibilitychange` + `online` (not `focus`):
```ts
const handleVisibilityOrOnline = (ev: Event) => {
  if (document.visibilityState !== "visible" && ev.type !== "online") return;
  if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
  terminalDebounceTimerRef.current = setTimeout(() => {
    terminalDebounceTimerRef.current = null;
    if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
      terminalBackoffRef.current.reset();
      connectRef.current();
    }
  }, 200);
};
document.addEventListener("visibilitychange", handleVisibilityOrOnline);
window.addEventListener("online", handleVisibilityOrOnline);
```
The reusable shape: **one shared handler function registered for multiple event types**, a **single ref-backed debounce timer** (clear-then-reschedule on every call, so N events collapsed into 1 tick still only fire once after the delay), and a **guard condition inside the timeout body** (re-checked at fire time, not at listener time) so state that changed during the debounce window is respected.

This pattern directly generalizes to `visibilitychange` + `window focus` both firing in the same real-browser tick when returning to a tab: register both event types on the same handler, single debounce timer ref → both events collapse to one fire. **No new dedup primitive is needed; the existing 3.1.3 listener already proves the shape works for exactly this multi-event-type coalescing problem.** The new AC1 listener should register `document.addEventListener("visibilitychange", ...)` and `window.addEventListener("focus", ...)` (not `"online"`) onto one handler using the fixed `useDebouncedCallback` (or the same manual-ref-timer style), with the visible/connected-vs-disconnected branching happening **inside the debounced callback body**, not at listener-registration time, exactly as the existing code re-checks `shouldReconnectRef.current`/`isConnectedRef.current` at fire time.

One correctness note: unlike the existing listener, `window focus` fires without carrying `document.visibilityState` info, and a `focus` event can also fire for reasons unrelated to backgrounding (e.g. clicking back into an already-visible window from an OS-level app switch on multi-window setups) — so the new handler's visible-guard should check `document.visibilityState === "visible"` inside the callback for both event types (not gate on `ev.type` the way the existing "online bypasses the visibility check" logic does), since for this feature (unlike reconnect-on-online) there's no case where firing while hidden is desired.

## 5. Multi-tab / session-pool `isVisible` prop vs document-level visibility — misfire risk

`SessionDetailView.tsx` mounts **multiple `TerminalOutput` instances concurrently**, each with its own pool-scoped `isVisible` prop, independent of `document.visibilityState`:
- line 650: `isVisible={poolPath === session.externalMetadata?.muxSocketPath}`
- line 669: `isVisible={poolId === session.id}`
- line 749: `isVisible={activeTabId === shellKey}` (multi-shell tabs within one session)

Each mounted `TerminalOutput` → `useTerminalStream` instance manages its **own independent WebSocket connection** and (today) its own `document.visibilitychange`/`online` listener when `NEXT_PUBLIC_RECONNECT_V2` is on (`useTerminalStream.ts:421`, registered with no `isVisible`/pool gating at all — confirmed by reading the effect, which has an empty dep array and no reference to the `isVisible` prop). This is **existing, already-shipped behavior**: a single document-level `visibilitychange` event fires identically into every mounted instance regardless of whether that instance is the one currently shown in the pool UI.

**Implication for the new resync handler**: if implemented the same way (attached per-`TerminalOutput`/per-`useTerminalStream` instance with no `isVisible` gate), returning to a backgrounded browser tab would fire `requestFullResync(true)` (or the disconnected-fallback `connect()`) for **every connected session in the pool simultaneously**, not just the one currently visible in the UI. Concretely:
- A pool session that's `isVisible={false}` (backgrounded within the app, e.g. the user is looking at a different session tab) but still `isConnected={true}` would get resynced too. This is likely **harmless-but-wasteful** for the resync case specifically — it just means an off-screen xterm buffer gets a `clearAndHome`+full-repaint write it doesn't visually need yet, using one extra `CurrentPaneRequest` round-trip per background session on every visibility-return, and adding N stall-watchdog timers running concurrently (one per mounted pool session) if all N happen to be connected.
- Because `requestFullResync` never touches focus/DOM directly (AC3 requirement) and each instance has its own isolated `TerminalStreamManager`/xterm `Terminal`, there's no cross-instance data corruption risk — the misfire is a resource-efforfectiveness concern, not a correctness one for AC3. But it **is** a correctness question for AC4/AC5: firing `connect()` + `showReconnectButton(true)` for a pool session's disconnected-but-not-currently-shown terminal on every tab refocus could produce a visible "Reconnecting" banner on a session the user isn't even looking at, right as they switch back to check on it later.
- **This exactly mirrors the existing (shipped, out-of-scope) `NEXT_PUBLIC_RECONNECT_V2` listener's behavior** — it has the identical no-pool-gating characteristic today. Given the requirements doc's explicit stance that that listener is "left as-is," the pragmatic design choice is to accept the same non-gated-by-pool semantics for the new resync listener for consistency, rather than introduce a new, asymmetric gating rule — but this should be called out explicitly as a documented decision in the plan, since it's a real behavioral surface the AC0 manual repro and any multi-session test should account for (e.g. don't assert "only the visible session resyncs"; assert "all connected sessions resync, none crash/steal focus/duplicate-fire").

## Summary of design implications to carry into planning

- No shared "visibility hook" exists to extract/reuse; the `useTerminalStream.ts:420-446` listener is the pattern to structurally mirror (shared handler across event types + single ref-backed debounce timer + guards re-checked at fire time), but it must be **added as new code in the terminal path**, not refactored out of the VCS/review-queue hooks (those solve a different, gate-only problem).
- `useDebouncedCallback` fix is risk-free w.r.t. other callers — there are none yet.
- Dedup for `visibilitychange`+`focus` firing in the same tick has a proven precedent in this exact file; reuse the "one handler, one debounce timer ref, re-check guards at fire time" shape, checking `document.visibilityState === "visible"` for both event types.
- The resync/watchdog implementation must explicitly handle: (a) `isConnected===false` but `terminalState` already `CONNECTING`/`LOADING` (don't show a reconnect button for a connect already succeeding), (b) overlap with the resize path's own resync (shared `isResyncingRef`/`waitingForPaneResponseRef`, no message-correlation ID — any `output` clears the wait, whichever resync "wins"), (c) `RESIZING`-state output queuing interacting with the stall watchdog's timer, (d) cleanup/cancellation of pending resync + watchdog timers on session switch (`sessionId` effect at `TerminalOutput.tsx:927`), and (e) the pool's `isVisible` prop is orthogonal to `document.visibilityState` — per-instance listeners will multi-fire across all mounted pool sessions on every tab refocus, matching existing `NEXT_PUBLIC_RECONNECT_V2` behavior; document this as an intentional consistency choice rather than a bug, and write tests against that reality (all connected instances resync harmlessly) rather than assuming pool-scoped gating.
- A structural ambiguity worth flagging for the plan: `output` messages carry no request-correlation ID, so `markPaneResponseReceived`/`markResyncComplete` (once wired) can only be "any output arrived while waiting," not "the specific resync response arrived." On a noisy pane (actively-printing background process), ordinary output could arrive and clear the pending-resync flag before the actual `clearAndHome+content` resync payload lands, causing the watchdog to consider the resync "complete" prematurely. This mirrors how the existing resize path already tolerates the same ambiguity (`useTerminalFlowControl.ts` resize→resync has the same no-correlation-ID limitation) — worth explicitly deciding in the plan whether to accept this pre-existing limitation for consistency or scope-add a resync-completion signal.
