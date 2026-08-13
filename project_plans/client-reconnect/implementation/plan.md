# Implementation Plan: client-reconnect

**Feature**: Robust client-side stream reconnect using browser lifecycle APIs
**Date**: 2026-06-23
**Status**: Ready for implementation
**ADRs**: docs/adr/ADR-023-client-reconnect-browser-lifecycle.md

---

## Creative Pass — Approach Comparison

| Approach | Strength | Weakness | Decision |
|---|---|---|---|
| **A. Transport-level retry wrapper** (wrap `createWatchTransport` / `createWebsocketBasedTransport`) | Zero changes to hooks; all streams inherit for free | `MessageQueue` is one-shot; terminal stream cannot be replayed after `.close()` — hard blocker | REJECTED |
| **B. Per-hook with shared utility** (add reconnect loop + browser event listeners in each hook, driven by shared `backoff.ts`) | Matches existing hook structure; no architectural surgery; `MessageQueue` recreated fresh on each terminal reconnect | Small duplication between the two hooks (~40 lines) | **SELECTED** |
| **C. Custom React context that owns reconnect state machine** (lifts reconnect logic out of hooks into a dedicated `ReconnectManager` context) | Clean separation of concerns; single state machine testable in isolation | Over-engineered for two stream types; adds a third context layer on an already-busy render tree | REJECTED |

**Selection rationale**: Approach B is the correct fit because the terminal stream's `MessageQueue` is a one-shot pipe — each reconnect must construct a fresh queue and re-run the full handshake. A transport wrapper cannot do this. The two hooks share identical retry policy so extracting it into `backoff.ts` gives 90% of Approach C's cleanliness with none of the indirection.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **Watch stream** | The `WatchSessions` server-streaming RPC that delivers session state updates | Lives in `useSessionService.ts` |
| **Terminal stream** | The `StreamTerminal` bidirectional RPC that drives the xterm.js pane | Lives in `useTerminalStream.ts` |
| **shouldReconnectRef** | `useRef<boolean>` sentinel that gates all reconnect paths; `false` when the consumer has called `stopWatching()` or `disconnect()` | Prevents reconnect after intentional teardown |
| **StreamGeneration** | Monotonically-increasing integer stored in a ref; incremented each time a new stream starts; every async continuation checks it before acting | Prevents dual-stream from overlapping reconnects |
| **BackoffState** | Instance of the `BackoffState` class from `backoff.ts`; owns `delay`, `attempt`, `reset()`, `next()` | Carries jittered exponential state across reconnect cycles |
| **Full jitter** | `Math.random() * Math.min(capMs, baseMs * 2^attempt)` — AWS-recommended formula that prevents thundering herd | Used for all backoff in this feature |
| **Thundering herd** | Multiple tabs resuming simultaneously all retry at the same interval, causing a burst of server requests | Mitigated by full jitter |
| **afterSeq** | The `lastSeqRef` value sent as `afterSeq` in the `WatchSessions` request; tells the server to replay events since that sequence number | Must be reset to 0 on seq backwards-jump |
| **seq backwards-jump** | Detected when server's event sequence number is lower than `lastSeqRef.current`; indicates server restart | Triggers snapshot path (`afterSeq: 0`) |
| **Reconnect trigger** | The event that initiates a reconnect: `visibilitychange`→visible, `online`, stream error, stream clean close | Different triggers use different delay policies |
| **Soft reconnect** | Restarting the stream without a page reload; preserves Redux state and React component tree | Replaces `window.location.reload()` in `ConnectionIndicator` |
| **Hard reload** | `window.location.reload()` — retained as an escape hatch in the tooltip | Never triggered automatically |
| **Reconnecting banner** | Thin overlay on the terminal pane shown after 2s of disconnect | Adopted from the `BrowserTab` `reconnectingBanner` pattern |
| **WS close code** | Integer in `CloseEvent.code` from the WebSocket `onclose` event | Codes 4001 (auth failure) and 4004 (session not found) must suppress reconnect |
| **Feature flag** | `NEXT_PUBLIC_RECONNECT_V2=true` env var; absent = existing behaviour; present = new behaviour | Allows staged rollout and fast rollback |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Backoff algorithm | Full jitter: `Math.random() * Math.min(cap, base * 2^n)` | AWS architecture blog | Decorrelated jitter | Slightly simpler; same herd prevention |
| Reconnect gate | `shouldReconnectRef` + `streamGeneration` counter ref | Pitfalls research #2 | Redux boolean flag | Refs avoid stale closure issues in async loops |
| Terminal reconnect placement | `useTerminalStream.ts` hook (not `TerminalOutput.tsx` component) | Architecture research | Component-level `useEffect` (current approach) | Hook is the correct layer for transport concerns; component should only own UX |
| ConnectionIndicator action | `useSessionServiceContext().watchSessions()` | Features research | `window.location.reload()` | Preserves React state; context already exposes the method |
| Staleness detection | `visibilitychange` + `online` event-driven | Requirements | `setInterval` polling (current) | Eliminates 5s poll; reacts within 200ms of browser event |
| Close code inspection | Wrap `ws.onclose` in `fromWebSocket` to propagate `CloseEvent.code` as a typed `ConnectError` metadata field | Pitfalls research #1 | Inspect at hook layer after generic Error | Transport is the right layer; preserves hook abstraction |
| `setConnectionState("connected")` timing | After first received event, not on WS open | Pitfalls research #5 | On WS open (current) | Prevents false "connected" flash before server confirms stream is live |
| Terminal banner timing | Show after 2s debounce, not immediately | UX research | Immediate | Avoids banner flash on fast reconnects |
| aria-live region | Separate visually-hidden `<div aria-live="polite">` | UX research | `aria-live` on `<button>` (current) | Screen readers suppress live updates on interactive elements |

---

## Observability Plan

- **Logs**: Every reconnect attempt logs at `console.info` level:
  `[reconnect] stream=<watch|terminal> trigger=<visibility|online|error|close> attempt=<N> delay=<Nms>`
- **Metrics**: `reconnectAttemptCount` state integer in `useSessionService` and `useTerminalStream`; exposed to `ConnectionIndicator` tooltip via context
- **Alerts**: None (internal/Tailscale only; no external alerting stack in scope)

---

## Risk Control

- **Feature flag**: `NEXT_PUBLIC_RECONNECT_V2=true` in `.env.local` / deployment env. Absent = existing behaviour preserved exactly. Present = new browser event listeners + jittered backoff + soft reconnect.
- **Rollback procedure**: Remove `NEXT_PUBLIC_RECONNECT_V2` from env config and redeploy (no code change needed). Or revert the three transport files.
- **Staged rollout**:
  - Phase 1: Land `backoff.ts` utility + unit tests only (no behaviour change)
  - Phase 2: Wire watch stream + ConnectionIndicator (feature-flagged)
  - Phase 3: Wire terminal stream (feature-flagged)
  - Phase 4: Remove feature flag after 1 week burn-in

---

## Unresolved Questions

None remaining — all pre-implementation questions have been resolved below.

### Resolved: Feature flag default

`NEXT_PUBLIC_RECONNECT_V2=true` is the committed default in `.env.local`. The flag exists for fast rollback (remove or set to `false` and rebuild), not for opt-in. Story 4.2.1 now reflects this decision.

### Resolved: Terminal scrollback policy after reconnect

**Policy: skip `requestScrollback()` on reconnect if the terminal buffer has content; only request scrollback on fresh (empty) connect.**

Rationale: UX research explicitly says "Do not clear and replay the terminal buffer on reconnect — preserve existing scroll history." Calling `requestScrollback()` unconditionally on every reconnect would prepend history the user already sees, duplicating every line. The correct policy:
- `terminal.buffer.active.length === 0` (fresh terminal, no prior output): call `requestScrollback()` to populate history.
- `terminal.buffer.active.length > 0` (reconnect after disconnect): skip `requestScrollback()`. Append `--- reconnected ---` separator only.

Implementation note: check `terminal.buffer.active.length` before calling `requestScrollback()` in the `isConnected` transition handler. This is added as Task 4 in Story 3.1.2.

---

## Dependency Visualization

```
Phase 1: backoff.ts (no deps)
    │
    ├──▶ Phase 2: useSessionService.ts (needs backoff.ts)
    │         │
    │         └──▶ Phase 2: ConnectionIndicator.tsx (needs context + new connection states)
    │
    └──▶ Phase 3: useTerminalStream.ts (needs backoff.ts)
              │
              └──▶ Phase 3: TerminalOutput.tsx reconnect banner (needs hook changes)
                        │
                        └──▶ Phase 4: Integration tests (needs all above)
```

---

## Phase 1: Shared Utilities

### Epic 1.1: Backoff + Jitter Utility

**Goal**: Create the shared retry policy module that both hooks will import.

---

#### Story 1.1.1: Create `backoff.ts` with `BackoffState` and `jitteredDelay()`

**User story**: As a hook author, I want a single place to import the retry-delay formula so all streams use consistent jitter.

**Acceptance criteria**:
- Given `attempt=0`, `baseMs=1000`, `capMs=30000`, `jitterFraction=0.2`, when `backoffState.next()` is called, then the returned delay is between 0 ms and 1000 ms (cap * jitter applied to base).
- Given `attempt=5`, when `backoffState.next()` is called, then the returned delay is at most 30000 ms (cap respected).
- Given `backoffState.reset()` is called, when `backoffState.next()` is called, then the returned delay is in the base range (attempt counter reset to 0).
- Given full-jitter formula `Math.random() * Math.min(cap, base * 2^attempt)`, when called 1000 times at `attempt=10`, then the mean delay is approximately `cap/2` (±10% tolerance in tests).

**Files**: `web-app/src/lib/utils/backoff.ts` (new file)

**Tasks**:
1. `[5m]` Create `web-app/src/lib/utils/backoff.ts` exporting `BackoffState` class with constructor `(baseMs: number, capMs: number)`, methods `next(): number` (returns jittered delay, increments attempt), `reset()` (resets attempt to 0), readonly `attempt: number`.
2. `[3m]` Export standalone `jitteredDelay(baseMs: number, capMs: number, attempt: number): number` pure function (used by the class internally; also useful for one-off callers).
3. `[5m]` Write `web-app/src/lib/utils/backoff.test.ts` — unit tests for cap, floor (≥0), reset, monotone mean with large N.

---

#### Story 1.1.2: Add `NON_RETRIABLE_WS_CODES` constant

**User story**: As a stream consumer, I want authentication and session-not-found errors to suppress reconnect so a broken session doesn't spam the server.

**Acceptance criteria**:
- Given a WebSocket closes with code `4001`, when the reconnect policy checks the code, then `isRetriableCloseCode(4001)` returns `false`.
- Given a WebSocket closes with code `4004`, when the reconnect policy checks the code, then `isRetriableCloseCode(4004)` returns `false`.
- Given a WebSocket closes with code `1006` (abnormal), when the reconnect policy checks the code, then `isRetriableCloseCode(1006)` returns `true`.

**Files**: `web-app/src/lib/utils/backoff.ts` (append), `web-app/src/lib/utils/backoff.test.ts`

**Tasks**:
1. `[3m]` Add `export const NON_RETRIABLE_WS_CODES = new Set([4001, 4004])` and `export function isRetriableCloseCode(code: number): boolean { return !NON_RETRIABLE_WS_CODES.has(code); }` to `backoff.ts`.
2. `[3m]` Add unit tests for `isRetriableCloseCode` covering codes 4001, 4004, 1000, 1006.

---

#### Story 1.1.3: Propagate WebSocket close code through both transports

**User story**: As the reconnect policy, I need to know the WS close code so I can distinguish auth failure from network drop — in both the session-watch and terminal transports.

**Acceptance criteria**:
- Given the server sends WS close code `4001`, when `fromWebSocket` handles `onclose`, then the upstream stream throws a `ConnectError` with metadata field `ws-close-code: "4001"`.
- Given `AbortSignal` fires (intentional stop via `stopWatching()`), when `fromWebSocket` handles `onclose`, then `push(null)` is called (no error thrown, no false reconnect triggered).
- Given the terminal WebSocket closes with code `4004`, when `fromWebSocket` in `websocket-transport.ts` handles the close, then a `ConnectError` with `ws-close-code: "4004"` is thrown so `getWsCloseCode` can extract it.

**Files**: `web-app/src/lib/transport/watch-ws-transport.ts`, `web-app/src/lib/transport/websocket-transport.ts`

**Tasks**:
1. `[5m]` In `watch-ws-transport.ts`, change `ws.onclose = () => push(null)` to:
   ```ts
   ws.onclose = (ev: CloseEvent) => {
     if (signal?.aborted || ev.wasClean || ev.code === 1000) {
       push(null); // clean close or intentional abort — no error
     } else {
       push(new ConnectError("WebSocket closed", Code.Unavailable, new Headers({ "ws-close-code": String(ev.code) })));
     }
   };
   ```
   The `signal?.aborted` guard is critical: when `stopWatching()` calls `ws.close()`, `ev.wasClean` is `false` and `ev.code` is `1006` — without this guard, every intentional stop would throw a `ConnectError` and trigger reconnect.
2. `[5m]` Apply the same close-code propagation to `websocket-transport.ts` (the terminal transport). Find `ws.onclose` in the `fromWebSocket` generator (line ~47) and apply the same pattern. The `it-ws/client` transport also uses native `WebSocket` underneath via the `ws.socket` handle — identify the correct close event attachment point.
3. `[3m]` Update the `parseResponses()` consumer in `createWatchTransport` to handle the new error shape without breaking the `!endStreamReceived` guard.
4. `[3m]` Add a `getWsCloseCode(err: unknown): number | null` helper at the bottom of `backoff.ts` that extracts the `ws-close-code` header from a `ConnectError`.

---

## Phase 2: Session Watch Stream

### Epic 2.1: Browser Lifecycle Signals for `useSessionService`

**Goal**: Watch stream reconnects within 200 ms of tab regaining focus or network restoration, without waiting for the backoff timer.

---

#### Story 2.1.1: Store watch options in a ref + add stream generation counter

**User story**: As the reconnect system, I need stable access to the most recent watch options so a re-triggered reconnect uses current values, not a stale closure.

**Acceptance criteria**:
- Given `watchSessions({ categoryFilter: "work" })` is called, when a reconnect fires 10 seconds later (no new `watchSessions` call), then the new stream is created with `categoryFilter: "work"` (not `undefined`).
- Given two reconnect triggers fire within 50 ms of each other, when both start `startStream`, then exactly one stream is active (the second aborts the first).

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

**Tasks**:
1. `[3m]` Add `watchOptionsRef = useRef<{ categoryFilter?: string; statusFilter?: SessionStatus } | undefined>(undefined)` near other refs (around line 720). In `watchSessions`, assign `watchOptionsRef.current = watchOptions` before calling `startStream`.
2. `[3m]` Add `streamGenerationRef = useRef(0)`. Increment it at the top of `watchSessions()` and at the top of `startStream()`. Store the local generation: `const myGeneration = ++streamGenerationRef.current`. At every `await` checkpoint inside `startStream`, add `if (streamGenerationRef.current !== myGeneration) return;`.
3. `[3m]` Replace `watchOptions?.categoryFilter` and `watchOptions?.statusFilter` in the `clientRef.current.watchSessions(...)` call (line 785) with `watchOptionsRef.current?.categoryFilter` and `watchOptionsRef.current?.statusFilter`.

---

#### Story 2.1.2: Add full jitter to the watch stream backoff loop

**User story**: As a user with multiple open tabs, I want staggered reconnect timing so all tabs don't hammer the server simultaneously when I wake my laptop.

**Acceptance criteria**:
- Given a stream error occurs on the watch stream with `reconnectDelayRef.current = 8000`, when the next reconnect fires, then the actual delay is between 0 ms and 8000 ms (jitter applied, not a fixed 8s wait).
- Given `stopWatching()` is called during a backoff sleep, when `shouldReconnectRef.current` is checked after the sleep, then `startStream` returns early (no new stream opened).

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

**Tasks**:
1. `[3m]` Import `BackoffState` from `@/lib/utils/backoff`. Replace `reconnectDelayRef = useRef(1000)` and the `Math.min(... * 2, 30_000)` lines with `backoffRef = useRef(new BackoffState(1000, 30_000))`. Replace all `reconnectDelayRef.current` usages with `backoffRef.current.next()` (for the delay value) and `backoffRef.current.reset()` (in `watchSessions` entry point).
2. `[3m]` After both `await new Promise(r => setTimeout(r, delay))` calls (lines 810 and 829), add a generation check: `if (streamGenerationRef.current !== myGeneration || !shouldReconnectRef.current) return;`.
3. `[2m]` Move `dispatch(setConnectionState("connected"))` from the top of `startStream` (line 782, before any event) to inside the `for await` loop on first iteration (set a `firstEvent = true` flag, flip to false after first dispatch). Also sync the ref directly: `isConnectedRef.current = true` alongside the dispatch (mirrors the terminal's task 0 pattern). This is critical for the `visibilitychange` handler in Story 2.1.3: without the direct ref sync, a tab-switch immediately after a reconnect could fire a spurious `watchSessions()` call because `useEffect`-synced refs lag one render cycle.

---

#### Story 2.1.3: Add `visibilitychange` and `online` event listeners

**User story**: As a user who switches back to the app tab after a background period, I want the session list to reconnect immediately without waiting up to 30 seconds.

**Acceptance criteria**:
- Given the watch stream is in a 28-second backoff sleep, when `document.visibilityState` changes to `"visible"`, then the stream reconnects within 200 ms (200 ms debounce fires).
- Given the stream is connected but last event was 20 seconds ago, when the tab becomes visible, then `watchSessions()` restarts the stream immediately.
- Given `window` fires an `online` event, when `shouldReconnectRef.current` is `true`, then the stream reconnects within 200 ms.
- Given `stopWatching()` was called before the tab became visible, when `visibilitychange` fires, then no reconnect occurs.
- Given `NEXT_PUBLIC_RECONNECT_V2` is absent or `"false"`, when the tab becomes visible, then no new event listener is registered (feature flag guards entire block).

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

**Tasks**:
1. `[5m]` Add a `useEffect` (after the staleness detector effect, ~line 864) that runs when `enabled` and `NEXT_PUBLIC_RECONNECT_V2 === "true"`. Add the following refs outside the effect:
   - `debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)`
   - `watchSessionsRef = useRef(watchSessions)` — updated unconditionally on every render: `watchSessionsRef.current = watchSessions` (one line at the top of the effect or in a preceding `useEffect` with no deps).

   Define a stable handler via `useCallback` with `[]` deps:
   ```ts
   const handleVisibilityOrOnline = useCallback((ev: Event) => {
     if (document.visibilityState !== "visible" && ev.type !== "online") return;
     if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
     debounceTimerRef.current = setTimeout(() => {
       debounceTimerRef.current = null;
       if (!shouldReconnectRef.current) return;
       if (!isConnectedRef.current || lastEventTimeRef.current < Date.now() - 15_000) {
         backoffRef.current.reset();
         watchSessionsRef.current?.(watchOptionsRef.current);
       }
     }, 200);
   }, []);
   ```
   Two architectural invariants enforced here:
   1. **Stable `useCallback([], [])`** is safe for `removeEventListener` because the exact same function reference is used for both `addEventListener` and cleanup — unlike `useDebouncedCallback` which creates a new reference per call.
   2. **`watchSessionsRef.current` indirection** prevents stale-closure capture: the `useCallback([], [])` closure cannot capture `watchSessions` directly (the empty dep array would bake in the mount-time value). `watchSessionsRef.current` always points to the latest version because the ref is updated on every render, even though the handler is stable.
   
   Register on `document` (`visibilitychange`) and `window` (`online`). Return cleanup: `clearTimeout(debounceTimerRef.current); document.removeEventListener(…); window.removeEventListener(…);`.
2. `[3m]` Replace the `setInterval`-based staleness detector (lines 852–864) with two complementary mechanisms:
   - **Event-driven** (primary): in the `visibilitychange` handler added in task 1, if `isConnected && lastEventTimeRef.current < Date.now() - 15_000`, dispatch `setConnectionState("stale")` before triggering reconnect.
   - **Backstop interval** (for always-visible tabs): add a 30-second `setInterval` (replacing the current 5-second one) that only checks staleness — do NOT reconnect inside it, just dispatch `setConnectionState("stale")` if `lastEventTimeRef.current < Date.now() - 30_000` and `shouldReconnectRef.current`. Dispatch `watchSessions(watchOptionsRef.current)` only once (set a local `backstopTriggeredRef` flag, reset on stream recovery). This backstop handles silently dead streams caused by load balancer idle-timeouts or proxy drops without a close frame — scenarios where neither `visibilitychange` nor `online` ever fires.
   Do NOT remove the interval entirely — the event-driven path handles wakes, but always-visible tabs need the backstop.
3. `[3m]` Expose `reconnectAttemptCount` (derived from `backoffRef.current.attempt`) in the return value of `useSessionService` and thread it through `SessionServiceContext` (add field `reconnectAttemptCount: number` to `SessionServiceContextValue` in `SessionServiceContext.tsx`).

---

### Epic 2.2: ConnectionIndicator Soft Reconnect

**Goal**: Replace `window.location.reload()` with a `watchSessions()` call; update visual states.

---

#### Story 2.2.1: Collapse stale + disconnected → "Reconnecting…" + soft-reconnect

**User story**: As a user who sees the "Stale" or "Offline" indicator, I want clicking it to trigger a soft reconnect (not a page reload) so I don't lose my scroll position.

**Acceptance criteria**:
- Given connection state is `"stale"`, when the `ConnectionIndicator` button is clicked, then `watchSessions()` is called (not `window.location.reload()`).
- Given connection state is `"disconnected"`, when the `ConnectionIndicator` button is clicked, then `watchSessions()` is called.
- Given connection state is `"connected"`, when rendered, then the button shows "Live" with no spinner.
- Given `reconnectAttemptCount` is 3, when hovered, then the tooltip reads "Reconnecting… attempt 3".
- Given connection state changes from `"connected"` to `"stale"`, when the `aria-live` region updates, then screen readers announce "Reconnecting…" (not the raw state change on the button).

**Files**: `web-app/src/components/layout/ConnectionIndicator.tsx`, `web-app/src/components/layout/ConnectionIndicator.css.ts`

**Tasks**:
1. `[5m]` Import `useSessionServiceContext` in `ConnectionIndicator.tsx`. Replace `handleClick`'s `window.location.reload()` with `const { watchSessions, reconnectAttemptCount } = useSessionServiceContext(); ... handleClick = () => { if (isActionable) watchSessions(); }`. Hard reload moves to tooltip link only (see task 3).
2. `[3m]` Update `STATE_LABEL` and `STATE_ARIA` maps: both `"stale"` and `"disconnected"` render as `"Reconnecting…"`. Keep `"connected"` → `"Live"`. Remove the word "Stale" and "Offline" from user-facing strings.
3. `[5m]` Add a visually-hidden `<div aria-live="polite" aria-atomic="true">` that only announces on state transitions (use `useRef` to track previous state, emit announcement text only when state changes). Move `aria-live` off the `<button>` element. Add "Reload page (resets state)" as a `<a href="#">` inside the tooltip that calls `window.location.reload()`.
4. `[3m]` Add spinner animation class in `ConnectionIndicator.css.ts` (vanilla-extract `keyframes`); render spinner instead of status dot when state is `"stale"` or `"disconnected"`. Use `vars.color.statusWarning` for the spinner colour.

---

#### Story 2.2.2: Seq backwards-jump detection

**User story**: As the reconnect system, I want to detect server restarts so the client requests a full snapshot instead of a diff that the server can't provide.

**Acceptance criteria**:
- Given `lastSeqRef.current` is 5000 and the next event has `seq=1`, when `handleSessionEvent` processes the event, then `afterSeq` is set to `0` for the next reconnect and a `listSessions` flush is dispatched immediately.
- Given `lastSeqRef.current` is 5000 and the next event has `seq=5001`, when `handleSessionEvent` processes the event, then no backwards-jump action is taken.

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

**Tasks**:
1. `[4m]` Add `needsFullResyncRef = useRef(false)` near other refs. In `handleSessionEvent` (or where `lastSeqRef.current` is updated), add the backwards-jump detection:
   ```ts
   if (event.seq > 0n && event.seq < lastSeqRef.current) {
     console.warn("[reconnect] seq backwards-jump detected — resetting afterSeq to 0");
     lastSeqRef.current = 0n;
     needsFullResyncRef.current = true; // handled in startStream after the loop
   }
   ```
   Then in `startStream`, after the `for await` loop exits (before the reconnect backoff `setTimeout`), add:
   ```ts
   if (needsFullResyncRef.current) {
     needsFullResyncRef.current = false;
     if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
       void clientRef.current?.listSessions({}).then(r => {
         // Re-check BOTH guards inside the callback: streamGenerationRef.current may have
         // advanced while listSessions was in-flight (e.g. watchSessions() called with new
         // options). Checking only shouldReconnectRef allows stale data to clobber new state.
         if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
           dispatch(setSessions(r.sessions));
         }
       });
     }
   }
   ```
   This pattern avoids `await` inside `handleSessionEvent` (which would block the `for await` event loop and silently drop events). The resync fires asynchronously via `.then()` without suspending the reconnect path, and is gated on the generation counter to prevent stale-closure dispatches after component unmount.

---

## Phase 3: Terminal Stream

### Epic 3.1: Terminal Auto-Reconnect in `useTerminalStream`

**Goal**: Terminal stream auto-reconnects after network disruption without user navigating away and back.

---

#### Story 3.1.1: Add `shouldReconnectRef` and `terminalBackoffRef` to `useTerminalStream`

**User story**: As the terminal reconnect system, I need a stable sentinel to control the reconnect loop and a jittered backoff state.

**Acceptance criteria**:
- Given `disconnect()` is called, when checked, then `shouldReconnectRef.current` is `false`.
- Given `connect()` is called, when checked, then `shouldReconnectRef.current` is `true`.
- Given the hook unmounts, when `useEffect` cleanup runs, then `shouldReconnectRef.current` is set to `false` before `disconnect()` is called.

**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

**Tasks**:
1. `[3m]` Add `shouldReconnectRef = useRef(false)` and `terminalBackoffRef = useRef(new BackoffState(1000, 30_000))` near the other refs (around line 101). Import `BackoffState` from `@/lib/utils/backoff`.
2. `[2m]` In `connect()` (line 156), place `shouldReconnectRef.current = true; terminalBackoffRef.current.reset();` **after** the early-return guard (`if (isConnectedRef.current || !sessionId) return`). Placing it before the guard means a `visibilitychange` event calling `connect()` while already connected sets `shouldReconnectRef.current = true`, overwriting a `false` set by a prior `disconnect()` call — causing spurious auto-reconnect after intentional teardown.
3. `[2m]` In `disconnect()` (line 352), add `shouldReconnectRef.current = false;` as the very first line (before the early-return guard).
4. `[2m]` In the `useEffect` cleanup (line 394), add `shouldReconnectRef.current = false;` before calling `disconnect()`.

---

#### Story 3.1.2: Add auto-reconnect loop after stream close/error

**User story**: As a user whose terminal loses network, I want the terminal to reconnect automatically so I can continue working without manually navigating.

**Acceptance criteria**:
- Given the terminal stream closes cleanly (server sends WS close), when `shouldReconnectRef.current` is `true` and `isDisconnectingRef.current` is `false`, then `connect()` is called after a jittered delay.
- Given a network error causes the stream to throw, when `shouldReconnectRef.current` is `true`, then `connect()` is called after a jittered delay.
- Given WS close code is `4001`, when the stream closes, then no reconnect is attempted.
- Given `disconnect()` is called before the backoff timer fires, when `shouldReconnectRef.current` is `false`, then no `connect()` call is made.
- Given `NEXT_PUBLIC_RECONNECT_V2` is absent, when the stream closes, then the existing `TerminalOutput.tsx` component-level reconnect logic runs (no change).

**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

**Tasks**:
0. `[2m]` At the very top of the `finally` block in `connect()` (before `setIsConnected(false)` and `setTerminalState('DISCONNECTED')`, around line 335), add:
   ```ts
   isConnectedRef.current = false; // sync ref before state setter to prevent reconnect guard race
   ```
   This is critical: React batches state updates, so `isConnectedRef` synced via `useEffect` may still be `true` when the reconnect guard `if (isConnectedRef.current) return` runs synchronously after the finally block. Setting the ref directly here ensures the guard sees the correct value.
1. `[5m]` In the `finally` block of `connect()` (after the direct ref sync in task 0 and `setTerminalState('DISCONNECTED')`, around line 337), add the reconnect block (feature-flagged):
   ```ts
   if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true"
       && shouldReconnectRef.current
       && !isDisconnectingRef.current) {
     const delay = terminalBackoffRef.current.next();
     console.info(`[reconnect] stream=terminal trigger=close attempt=${terminalBackoffRef.current.attempt} delay=${delay}ms`);
     setTimeout(() => {
       if (shouldReconnectRef.current && !isDisconnectingRef.current) {
         connect();
       }
     }, delay);
   }
   ```
   Using `setTimeout` (not `await new Promise(r => setTimeout(r, delay))`) is critical: `await` inside a `finally` block keeps the outer async function's promise pending for the full chain duration. Over N reconnect cycles this creates N nested unresolved promise frames. `setTimeout` fires the callback outside the async call stack — the `finally` block returns synchronously, the outer promise resolves, and GC can collect the frame. This mirrors how the watch stream's `startStream` handles backoff.
   
   **Important**: the `catch` block in task 2 must NOT schedule its own `setTimeout` reconnect. JavaScript always executes `finally` after `catch` — if both schedule reconnect, every retriable error produces two concurrent `connect()` calls. All scheduling lives exclusively in `finally`.
2. `[4m]` In the `catch (err)` block of `connect()` (line 333), handle non-retriable close codes ONLY — do not schedule a reconnect here. Check `getWsCloseCode(err)` from `backoff.ts`: if code is non-retriable (e.g. `4001`, `4004`), set `shouldReconnectRef.current = false` and log a warning. For retriable errors, do nothing in catch — the `finally` block in task 1 will schedule the reconnect. This ensures only one reconnect callback is ever scheduled per connection attempt.
3. `[3m]` Gate ALL component-level reconnect logic in `TerminalOutput.tsx` with `process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true"`. There are **two** reconnect paths in that file, not one:
   - Lines 779–791: `reconnectTimeoutRef` useEffect (catch/finally reconnect loop)
   - Lines 677–727: `reconnectTimeoutRef` callback / `setRetryCount` path
   Both must be wrapped: `if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") { /* existing logic */ }`. Gating only one path while leaving the other active causes concurrent hook-level and component-level reconnect on every disconnect.
4. `[3m]` In `TerminalOutput.tsx`, in the `useEffect` that handles `isConnected` transitions: after `isConnected` becomes `true`, check `terminal.buffer.active.length`. If `=== 0` (fresh empty terminal), call `requestScrollback()`. If `> 0` (reconnect with existing history), skip `requestScrollback()` and only append `"\r\n\x1b[2m--- reconnected ---\x1b[0m\r\n"`. This implements the resolved scrollback policy: preserve existing history on reconnect, no duplication.

---

#### Story 3.1.3: Add `visibilitychange` and `online` listeners to terminal stream

**User story**: As a user who resumes a terminal tab after background time, I want the terminal to reconnect immediately rather than waiting for the backoff timer.

**Acceptance criteria**:
- Given the terminal stream is in a 20-second backoff sleep, when the tab becomes visible, then `connect()` is called within 200 ms.
- Given `shouldReconnectRef.current` is `false` (e.g. user manually disconnected), when the tab becomes visible, then no reconnect fires.
- Given `NEXT_PUBLIC_RECONNECT_V2` is absent, when the tab becomes visible, then no new event listener fires (feature flag guards).

**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

**Tasks**:
1. `[5m]` Add a `useEffect` (after the existing auto-connect effect at line 390) that runs when `NEXT_PUBLIC_RECONNECT_V2 === "true"`. Add refs outside the effect:
   - `terminalDebounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)`
   - `connectRef = useRef(connect)` — updated unconditionally on every render: `connectRef.current = connect`

   Define a stable handler via `useCallback([], [])` (mirrors Story 2.1.3 exactly):
   ```ts
   const handleVisibilityOrOnline = useCallback((ev: Event) => {
     if (document.visibilityState !== "visible" && ev.type !== "online") return;
     if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
     terminalDebounceTimerRef.current = setTimeout(() => {
       terminalDebounceTimerRef.current = null;
       if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
         terminalBackoffRef.current.reset();
         console.info("[reconnect] stream=terminal trigger=visibility delay=0ms");
         connectRef.current();
       }
     }, 200);
   }, []);
   document.addEventListener("visibilitychange", handleVisibilityOrOnline);
   window.addEventListener("online", handleVisibilityOrOnline);
   return () => {
     clearTimeout(terminalDebounceTimerRef.current ?? undefined);
     document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
     window.removeEventListener("online", handleVisibilityOrOnline);
   };
   ```
   Using `useCallback([], [])` + `connectRef.current` indirection guarantees a stable function reference for `removeEventListener` across StrictMode double-mount cycles, matching Story 2.1.3's pattern. The 200ms debounce satisfies the NFR and prevents flapping on rapid OS visibility events.

---

### Epic 3.2: Terminal Reconnect UX

**Goal**: Show a non-intrusive reconnecting banner after 2s of disconnect; preserve terminal buffer; append separator after reconnect. Escalate to hard-failure state after 5 consecutive failed attempts.

---

#### Story 3.2.1: Reconnecting banner in `TerminalOutput.tsx`

**User story**: As a user whose terminal disconnects, I want a subtle banner to inform me the terminal is reconnecting so I know to wait rather than navigate away.

**Acceptance criteria**:
- Given the terminal has been disconnected for less than 2 seconds, when rendered, then no banner is shown (avoids flicker on fast reconnects).
- Given the terminal has been disconnected for 2 or more seconds, when rendered, then a "Reconnecting…" overlay banner appears at the top of the terminal pane.
- Given the terminal reconnects, when `isConnected` becomes `true`, then the banner disappears and a dim `"--- reconnected ---"` line is appended to the terminal output.
- Given the terminal disconnects but `shouldReconnect` is false (manual disconnect), when 2 seconds elapse, then no banner appears.

**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/components/sessions/TerminalOutput.css.ts`

**Tasks**:
1. `[4m]` Add `useEffect` in `TerminalOutput.tsx`: when `!isConnected`, start a 2-second timer that sets local state `showReconnectBanner = true`. Clear timer and set `showReconnectBanner = false` when `isConnected` becomes `true`. Append `"\r\n\x1b[2m--- reconnected ---\x1b[0m\r\n"` (ANSI dim) to `xterm` via `onOutput` callback on reconnect (only when banner was showing, to avoid showing it on initial connect).
2. `[3m]` Add `reconnectingBanner` style to `TerminalOutput.css.ts`: position absolute, top of terminal, full width, semi-transparent dark background, centered text "Reconnecting…" with a CSS spinner. Use `vars.color.textMuted` and `vars.space[2]`. Follow the vanilla-extract pattern from `BrowserTab`'s `reconnectingBanner` (check `web-app/src/components/sessions/BrowserTab.css.ts` for reference).
3. `[2m]` Render the banner conditionally: `{showReconnectBanner && <div className={reconnectingBanner}>Reconnecting…</div>}`. The terminal container must have `position: relative` for the absolute-positioned overlay. **Do NOT add `aria-live` to this banner** — the `ConnectionIndicator`'s live region already announces the global reconnect state; a second `aria-live` region on the terminal banner would cause double-announcements for screen reader users (ux.md Surface 7 explicitly prohibits this).

---

#### Story 3.2.2: Terminal hard-failure state after 5 consecutive attempts

**User story**: As a user whose terminal cannot reconnect after 5 tries, I want a clear failure state with a manual retry button so I know the connection has given up and can take action.

**Acceptance criteria**:
- Given `terminalBackoffRef.current.attempt` reaches 5 and `shouldReconnectRef.current` is still `true`, when the 5th reconnect attempt fails, then `shouldReconnectRef.current` is set to `false` and an `isHardFailed` state flag is set `true`.
- Given `isHardFailed` is `true`, when rendered, then the banner upgrades from "Reconnecting…" to "Connection lost — [Retry]" with a primary-styled button (not a secondary spinner).
- Given `isHardFailed` is `true`, when the [Retry] button is clicked, then `isHardFailed` is reset to `false`, `shouldReconnectRef.current` is set to `true`, `terminalBackoffRef.current.reset()` is called, and `connect()` is invoked.
- Given `isHardFailed` is `true`, when the toolbar renders, then the status text shows "Terminal unavailable".
- Given `NEXT_PUBLIC_RECONNECT_V2` is absent, when rendered, then the existing 5-attempt cap behavior in `TerminalOutput.tsx` (pre-feature code path) is unchanged.

**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`, `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/components/sessions/TerminalOutput.css.ts`

**Tasks**:
1. `[3m]` In `useTerminalStream.ts`, expose `isHardFailed: boolean` and `handleManualReconnect: () => void` from the hook return value. Add `isHardFailedRef = useRef(false)`. In the `finally` block reconnect check (Task 1 of Story 3.1.2): before scheduling `setTimeout`, check `terminalBackoffRef.current.attempt >= 5`. If so, set `shouldReconnectRef.current = false`, set `isHardFailedRef.current = true`, dispatch `setIsHardFailed(true)` (local state). `handleManualReconnect`: set `isHardFailedRef.current = false`, `shouldReconnectRef.current = true`, `terminalBackoffRef.current.reset()`, call `connect()`.
2. `[3m]` In `TerminalOutput.tsx`, read `isHardFailed` and `handleManualReconnect` from the hook. When `showReconnectBanner && isHardFailed`: render a `hardFailedBanner` instead of `reconnectingBanner` — shows "Connection lost" icon + "Connection lost — " + `<button onClick={handleManualReconnect}>Retry</button>`. When `isHardFailed`: update toolbar status text to "Terminal unavailable" (existing text node, just change the conditional).
3. `[3m]` Add `hardFailedBanner` style to `TerminalOutput.css.ts`: same absolute-position pattern as `reconnectingBanner` but `vars.color.statusDanger` background with `vars.color.textInverse` text.

---

## Phase 4: Quality + Integration

### Epic 4.1: Bug Fixes from Pitfalls Research

**Goal**: Address all HIGH and MEDIUM pitfalls before the feature is enabled in production.

---

#### Story 4.1.1: Fix `setConnectionState("connected")` fires before first event (Pitfall #5)

**(Already addressed in Story 2.1.2, Task 3 — cross-reference only.)**

---

#### Story 4.1.2: Fix `listSessions` dispatch after unmount (Pitfall #3)

**User story**: As the reconnect system, I want `listSessions` to not dispatch state after the component has unmounted so I don't get React "update on unmounted component" warnings.

**Acceptance criteria**:
- Given `stopWatching()` is called while `listSessions` is in-flight, when the fetch completes, then `dispatch(setSessions(...))` is NOT called.

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

**Tasks**:
1. `[4m]` Add generation check after both `listSessions` awaits (lines 805 and 824): `const sessions = await ...listSessions`; then `if (!shouldReconnectRef.current || streamGenerationRef.current !== myGeneration) return;`; then `dispatch(setSessions(sessions.sessions))`.

---

#### Story 4.1.3: Fix StrictMode double-mount event listener leak (Pitfall #7)

**User story**: As a developer running in React StrictMode, I want event listener cleanup to work correctly so listeners aren't registered twice.

**Acceptance criteria**:
- Given React StrictMode mounts → unmounts → remounts the component, when checked, then exactly one `visibilitychange` listener is registered (not two).

**Files**: `web-app/src/lib/hooks/useSessionService.ts`, `web-app/src/lib/hooks/useTerminalStream.ts`

**Tasks**:
1. `[2m]` Verify that both `visibilitychange` / `online` `useEffect` hooks use named function references (not inline arrow functions) so `removeEventListener` correctly deregisters them. This is already enforced by the task structure in Stories 2.1.3 and 3.1.3 — this story is a review checkpoint only.

---

### Epic 4.2: Tests + Feature Flag

**Goal**: Full test coverage; feature flag wiring; `make ci` green.

---

#### Story 4.2.1: Feature flag wiring and env config

**User story**: As an operator, I want to enable the new reconnect behaviour by setting one environment variable so I can roll it out gradually.

**Acceptance criteria**:
- Given `.env.local` does not contain `NEXT_PUBLIC_RECONNECT_V2`, when the app builds, then all reconnect code paths are unreachable (no behaviour change).
- Given `NEXT_PUBLIC_RECONNECT_V2=true` is set, when the app builds, then event listeners are registered and backoff uses jitter.

**Files**: `.env.example` (or equivalent), `web-app/src/lib/hooks/useSessionService.ts`, `web-app/src/lib/hooks/useTerminalStream.ts`

**Tasks**:
1. `[2m]` Add `NEXT_PUBLIC_RECONNECT_V2=true  # Browser-lifecycle reconnect; set to false to rollback` to `.env.local` (uncommented = **on by default**). Add the same line commented out to `.env.example` with a note: "set to true or remove comment to enable". This implements the resolved flag decision: the feature is on in all environments unless an operator explicitly disables it for rollback.
2. `[2m]` Audit all `process.env.NEXT_PUBLIC_RECONNECT_V2 === "true"` guards added in Stories 2.1.3, 3.1.2, 3.1.3 — confirm they compile in both `true` and absent states (Next.js replaces at build time; no runtime branch needed).

---

#### Story 4.2.2: Unit tests for `useSessionService` reconnect paths

**User story**: As a developer, I want unit tests for the reconnect logic so regressions are caught before merge.

**Acceptance criteria**:
- Given a mock stream that closes cleanly, when `watchSessions` is called with `NEXT_PUBLIC_RECONNECT_V2=true`, then `startStream` is called again after a jittered delay.
- Given `stopWatching()` is called during backoff sleep, when delay expires, then `startStream` is NOT called a second time.
- Given `visibilitychange` fires with `document.visibilityState = "visible"`, when `shouldReconnectRef.current` is `true` and `lastEventTimeRef.current < Date.now() - 15_000`, then `watchSessions` is reinvoked.
- Given WS close code `4001` arrives, when the stream errors, then no reconnect is attempted.

**Files**: `web-app/src/lib/hooks/useSessionService.test.ts` (new or extend existing)

**Tasks**:
1. `[5m]` Write tests for clean-close reconnect loop, `stopWatching` interruption, generation counter preventing dual-stream.
2. `[5m]` Write tests for `visibilitychange` trigger: mock `document.visibilityState`, fire synthetic event, assert `watchSessions` called.
3. `[4m]` Write test for non-retriable WS code (`4001`): mock `ConnectError` with `ws-close-code: "4001"` header, assert no reconnect.

---

#### Story 4.2.3: Unit tests for `useTerminalStream` reconnect paths

**Acceptance criteria**:
- Given stream closes with no error and `shouldReconnectRef = true`, when `NEXT_PUBLIC_RECONNECT_V2=true`, then `connect()` is called after jittered delay.
- Given `disconnect()` fires during backoff sleep, when delay expires, then `connect()` is NOT called.
- Given WS close code `4004`, when the stream errors, then `shouldReconnectRef.current` is set to `false` and no reconnect fires.

**Files**: `web-app/src/lib/hooks/useTerminalStream.test.ts` (extend existing)

**Tasks**:
1. `[5m]` Add tests for clean-close reconnect, `disconnect()` interruption, `4004` non-retriable code.
2. `[4m]` Add test for `visibilitychange` trigger: mock `document.visibilityState = "visible"`, fire event, assert `connect()` called.

---

#### Story 4.2.4: Jest test for `ConnectionIndicator` soft reconnect

**Acceptance criteria**:
- Given `connectionState = "stale"`, when `ConnectionIndicator` button is clicked, then `watchSessions` from context is called (not `window.location.reload()`).
- Given `reconnectAttemptCount = 5`, when hovered, then `title` attribute contains "attempt 5".

**Files**: `web-app/src/components/layout/ConnectionIndicator.test.tsx` (new)

**Tasks**:
1. `[4m]` Write RTL test wrapping `ConnectionIndicator` with mock `SessionServiceContext`. Assert `watchSessions` mock called on click. Assert `window.location.reload` not called. Assert tooltip contains attempt count.

---

#### Story 4.2.5: Run `make ci` and fix any failures

**Acceptance criteria**:
- Given all changes are staged, when `make ci` runs, then it exits 0.

**Files**: N/A (validation task)

**Tasks**:
1. `[5m]` Run `make ci`. Address any TypeScript type errors from the new `reconnectAttemptCount` field added to `SessionServiceContextValue`.
2. `[3m]` Run `cd web-app && npx jest --no-coverage` to confirm all new tests pass.
3. `[3m]` Run `make lint` and fix any ESLint warnings in the new files.

---

## Summary

| Phase | Epics | Stories | Tasks | Notes |
|---|---|---|---|---|
| Phase 1: Shared Utilities | 1 | 3 | 9 | No behaviour change; safe to land independently |
| Phase 2: Session Watch Stream | 2 | 5 | 15 | Feature-flagged; includes staleness detector replacement |
| Phase 3: Terminal Stream | 2 | 4 | 11 | Feature-flagged; requires Phase 1 |
| Phase 4: Quality + Integration | 2 | 5 | 14 | Requires all above phases |
| **Total** | **7** | **17** | **49** | |
