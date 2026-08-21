# Feature Research: client-reconnect

**Date**: 2026-06-23
**Branch**: stapler-squad-reconnect
**Codebase root**: `web-app/src/`

---

## 1. WatchSessions Reconnect — Detailed Anatomy

**File**: `web-app/src/lib/hooks/useSessionService.ts` (lines 763–838)

### How it works today

`watchSessions()` wraps a recursive `startStream()` async function. The design is:

1. Abort any existing stream via `AbortController`.
2. Reset `reconnectDelayRef.current = 1000` and `shouldReconnectRef.current = true`.
3. Open a ConnectRPC server-streaming call: `clientRef.current.watchSessions({ afterSeq: lastSeqRef.current, ... })`.
4. Consume events in a `for await` loop; each event calls `handleSessionEvent(event)`.
5. `handleSessionEvent` advances `lastSeqRef.current` when `event.seq > lastSeqRef.current` (line 705). This cursor is the server-side replay key — the backend can replay up to 1 hour of missed events.
6. On **normal stream close** (server-side): calls `listSessions` to flush missed state, fires `onReconnectRef.current?.()`, waits `reconnectDelayRef.current` ms, doubles the delay (cap: 30,000 ms), recurses.
7. On **error** (network drop etc.): same path as normal close.
8. On `AbortError`: returns immediately (intentional stop via `stopWatching()`).

### Sequence-replay capability

The `afterSeq` parameter in the reconnect call means the server will replay events from the last seen sequence number. This is a **strong guarantee against missed events**. The client does NOT need to wipe Redux state on reconnect — `upsertSession` and `removeSession` are idempotent by design.

### Race conditions observed

- **Double-listSessions on reconnect**: Both the "normal close" and "error" paths call `listSessions` before scheduling the reconnect. This is correct but means every reconnect fires a full `ListSessions` RPC regardless of whether state was actually missed. Combined with the `afterSeq` replay, the `listSessions` call is redundant except for the very first reconnect after a long gap (>1 hour replay window).
- **`reconnectDelayRef` and `watchOptions` closure capture**: `watchOptions` is captured in the outer `watchSessions` closure. If the caller changes filter parameters while reconnecting, the reconnect will use the stale filters. The `useCallback` dep array (`[handleSessionEvent, dispatch]`) does not include `watchOptions`, which is correct (filters are explicit arguments), but means callers cannot update filters without calling `watchSessions()` again explicitly.
- **No jitter**: `reconnectDelayRef.current * 2` with no randomness. All open tabs after a 30s disconnect will reconnect simultaneously.
- **No browser lifecycle awareness**: No `visibilitychange`, `online`, or `focus` handlers. A tab backgrounded for 25s will still wait up to 30s more before reconnecting.

### State cleanup on reconnect

- Redux store is **not cleared** — `listSessions` overwrites the `sessions` array via `setSessions`.
- `lastSeqRef.current` is **not reset** — preserves the replay cursor.
- `connectionState` transitions: `"connected"` on stream start → `"disconnected"` on stream end/error → `"connected"` again after reconnect.

---

## 2. Terminal Stream Reconnect — Current State

**File**: `web-app/src/lib/hooks/useTerminalStream.ts` (full file, 419 lines)

The `useTerminalStream` hook itself has **no reconnect loop**. The `finally` block in the message processing loop (lines 335–338) sets `isConnected = false` and `terminalState = 'DISCONNECTED'` — and stops there. There is no `shouldReconnectRef`, no backoff timer, no retry.

### Where reconnect lives: TerminalOutput.tsx

Reconnect logic is in the **consuming component** `web-app/src/components/sessions/TerminalOutput.tsx`, not in the hook:

- **Auto-reconnect effect** (lines 778–791): Fires when `isConnected === false && error !== null && connectionAttempts > 0 && connectionAttempts < 5`. Uses exponential backoff: `min(1000 * 2^(attempts-1), 10000)` ms. Capped at 5 attempts.
- **connectionAttempts** is incremented when the connection transitions from connected → disconnected (tracked via `previousConnectionStateRef`).
- After 5 failures: `showReconnectButton = true`, loading overlay cleared. User sees a "Reconnect" button; clicking it calls `handleManualReconnect` which resets `connectionAttempts = 0` and calls `connect()`.
- **No browser lifecycle signals**: No `visibilitychange`, `online`, or `focus` listeners in TerminalOutput either.

### The `connect()` function

`useTerminalStream.connect()` (lines 156–345) creates a new `AbortController`, a fresh `MessageQueue`, sends the handshake `CurrentPaneRequest` (with terminal dimensions), and opens the `streamTerminal` bidirectional WebSocket stream. On reconnect, there is **no sequence number or cursor** — it always starts fresh. The server sends the current pane snapshot on every connection via the `CurrentPaneRequest` handshake (scrollback lines, includeEscapes=true), so the terminal buffer is effectively **reset and replayed** from server state on each connect.

### Terminal state machine

`TerminalState` has 6 states: `DISCONNECTED | CONNECTING | LOADING | STABLE | RESIZING | FETCHING_SCROLLBACK`. On disconnect (any error or close), it always falls to `DISCONNECTED`. This state is exposed to consumers but TerminalOutput does not display a spinner during reconnect — it shows "Disconnected" text in the status bar and a reconnect counter.

---

## 3. Session Detail Page — Disconnect Interaction

When a user is on a session detail page and the terminal disconnects:

1. `useTerminalStream` fires its `finally` block → `isConnected = false`, `terminalState = DISCONNECTED`.
2. `TerminalOutput` detects `wasConnected && !isConnected` → schedules a 5s timeout after which `showReconnectButton = true`.
3. Concurrently, the auto-reconnect effect (lines 778–791) fires if there is an `error`. If the WS closed cleanly (no error object), the auto-reconnect effect **does not fire** — only `showReconnectButton` will eventually appear.
4. The session-watch stream (WatchSessions) is **independent** — it runs at layout level in `GlobalSessionServiceProvider` and does not share state with the terminal stream. A terminal disconnect does not affect session list state; a WatchSessions reconnect does not re-trigger terminal reconnect.
5. There is no cross-stream coordination: if WatchSessions reconnects and replays a `sessionUpdated` event, `TerminalOutput` will not reconnect the terminal stream as a result.

### Key gap

If the WebSocket closes cleanly (normal close, no error), `error` remains `null` in `TerminalOutput`, so the auto-reconnect effect guard (`error && connectionAttempts > 0`) **never triggers**. The terminal stays disconnected until the user clicks the reconnect button (which appears after 5s). This is the dominant failure mode after a network hiccup — the WS close is clean.

---

## 4. Staleness Detector and ConnectionIndicator

### Staleness detector

**File**: `useSessionService.ts` lines 851–864

A `setInterval` fires every 5,000 ms. If `lastEventTimeRef.current` is set, `shouldReconnectRef.current === true`, and the gap since the last event is > 15,000 ms, it dispatches `setConnectionState("stale")`.

This is a **polling-based heuristic**, not event-driven. It fires 3 times before marking stale (5s, 10s, 15s). The stale state is purely cosmetic — it does not trigger a reconnect attempt. It just changes the Redux `connectionState` value, which `ConnectionIndicator` reads.

### ConnectionIndicator

**File**: `web-app/src/components/layout/ConnectionIndicator.tsx` (55 lines)

Reads `connectionState` from Redux via `selectConnectionState`. Three states:
- `"connected"` → green "Live" label, button disabled.
- `"stale"` → "Stale" label, button enabled.
- `"disconnected"` → "Offline" label, button enabled.

On click when not `"connected"`: calls `window.location.reload()`. This is the current UX for manual recovery. It destroys all React state, re-fetches everything, and produces a full page flash. The `watchSessions` function is **not accessible** from `ConnectionIndicator` — it lives in `SessionServiceContext` but the indicator reads only the Redux store, not the context.

### The watchSessions re-trigger path

`watchSessions` is exposed via `GlobalSessionServiceProvider` → `useSessionServiceContext()`. ConnectionIndicator does not use `useSessionServiceContext`. To fix the reload behavior, ConnectionIndicator must either: (a) import `useSessionServiceContext` and call `watchSessions()`, or (b) dispatch a Redux action that a middleware/effect handles by calling `watchSessions()`.

---

## 5. retry.ts — Current Usage

**File**: `web-app/src/lib/utils/retry.ts`

`retryOperation<T>()` provides exponential backoff for promise-based operations (maxRetries, initialDelay, maxDelay, backoffMultiplier, onRetry callback). `isRetryableError()` checks message strings for network keywords.

**Current usage**: `retryOperation` is **defined but not used anywhere in the codebase** except its own file. No hook imports it. The WatchSessions and TerminalOutput reconnect loops both implement their own ad-hoc backoff inline. `retry.ts` is an orphan utility that could serve as the shared policy module required by the reconnect-v2 spec.

---

## 6. Transport Layer

**File**: `web-app/src/lib/transport/watch-ws-transport.ts` (200 lines)

A custom ConnectRPC transport for streaming calls. It creates a raw `WebSocket`, waits for `ws.onopen`, sends the Connect envelope, then drives an async generator over incoming binary frames. The WS close (`ws.onclose → push(null)`) propagates as a normal generator return (not an error). The WS error (`ws.onerror → push(ConnectError)`) propagates as a thrown error.

This explains the terminal bug above: a clean WS close propagates as `return`, not `throw`, so the `catch (err)` in `useTerminalStream` is not triggered and `error` stays `null`.

The transport has **no reconnect logic of its own** — it is a thin wrapper. Reconnect must happen in the hook or component layer.

**File**: `web-app/src/lib/transport/websocket-transport.ts` — the bidirectional transport used by `useTerminalStream` (for `streamTerminal`). It follows the same pattern.

---

## 7. Unstated User Needs

Beyond the explicit requirements, several implicit needs emerge:

### Offline-capable display

When the terminal WebSocket closes, the xterm buffer retains whatever was rendered. Users expect to scroll through history even while disconnected. The current implementation preserves the buffer (no clear on disconnect) — this is correct and should be preserved in any reconnect implementation. A reconnect that clears the buffer and waits for server scrollback creates a blank-screen flash. The open question from the requirements ("clear xterm buffer or resume from cursor position?") is implicitly answered: **do not clear the buffer**; show a reconnecting overlay instead.

### Multiple tabs coordination

The `afterSeq` replay means multiple open tabs will each independently replay from their own last-seen sequence. If Tab A is in the foreground and Tab B is backgrounded for 20 minutes, Tab B's reconnect will request replay from its `lastSeqRef` (possibly an old value) — the server will replay up to 1 hour of events. No special coordination is needed, but jitter is critical to avoid thundering-herd on the `/watchSessions` endpoint.

### Progressive degradation on terminal reconnect failure

After 5 failed auto-reconnect attempts, TerminalOutput shows a manual reconnect button but no further backoff. If the server is down for 5+ minutes, the user is stuck clicking manually. A longer-lived backoff with a visible countdown (open question #3 from requirements) would address this.

### No server-side terminal sequence number

The terminal stream (`streamTerminal`) does not expose a sequence number analogous to `afterSeq` for WatchSessions. The `scrollbackResponse` proto has `oldestSequence` and `newestSequence`, but these are for the scrollback buffer, not the live stream. There is no mechanism to "resume" the terminal stream from a specific point — every reconnect gets a fresh pane snapshot from the server. This answers open question #2 from requirements: there is no sequence number for terminal stream events.

---

## Summary Table

| Surface | Has Reconnect? | Backoff? | Jitter? | Browser Signals? | Seq Replay? |
|---|---|---|---|---|---|
| WatchSessions (`useSessionService`) | Yes, infinite loop | Yes, 1s→30s | None | None | Yes (`afterSeq`) |
| Staleness detector | N/A (heuristic) | N/A | N/A | None | N/A |
| ConnectionIndicator | Via `window.location.reload()` | N/A | N/A | None | N/A |
| Terminal (`useTerminalStream`) | None (hook only) | N/A | N/A | None | None (full pane replay) |
| Terminal (`TerminalOutput` component) | Yes, 5 attempts | Yes, 1s→10s | None | None | None |
| `retry.ts` utility | Defined, unused | Yes | None | N/A | N/A |

---

## Key Implementation Notes for reconnect-v2

1. **`retry.ts` is the right centralization point** — add jitter there (suggest `delay * (1 + Math.random() * 0.3)` for truncated exponential), extend `maxRetries` to `Infinity` for stream reconnect, and expose a `StreamReconnectPolicy` preset.
2. **WatchSessions `afterSeq` is already safe** — the hook does not need to wipe Redux state on reconnect; `listSessions` + `afterSeq` replay together are sufficient.
3. **`visibilitychange` handler** should call `watchSessions()` directly (resetting backoff) rather than waiting for the next timer tick. The `reconnectDelayRef.current = 1000` reset on explicit `watchSessions()` call is already present — leverage it.
4. **Terminal reconnect on clean WS close** requires changing the transport close path: `ws.onclose` currently pushes `null` (generator return), which does not set `error`. Either propagate close as a `ConnectError` in the transport, or catch the `finally` block in `useTerminalStream` and set `error` explicitly when `isDisconnectingRef.current === false` (i.e., the close was not intentional).
5. **ConnectionIndicator fix** requires importing `useSessionServiceContext` in `ConnectionIndicator` to get `watchSessions`, replacing `window.location.reload()` with `watchSessions()`.
6. **Feature flag isolation**: `NEXT_PUBLIC_RECONNECT_V2=true` should gate: the `visibilitychange`/`online` listeners, jitter, the ConnectionIndicator fix, and terminal auto-reconnect. The flag can be read with `process.env.NEXT_PUBLIC_RECONNECT_V2 === "true"` at the hook level.
