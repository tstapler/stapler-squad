# Architecture Research: client-reconnect

**Date**: 2026-06-23
**Branch**: stapler-squad-reconnect

---

## 1. Integration Points for Each Approach

### Current Reconnect Logic Inventory

| Surface | File | Line(s) | What it does |
|---|---|---|---|
| `watchSessions` backoff loop | `useSessionService.ts` | 763–839 | Recursive `startStream()` with `reconnectDelayRef` doubling 1s→30s, no jitter |
| `stopWatching` | `useSessionService.ts` | 842–849 | Sets `shouldReconnectRef.current = false`, aborts controller |
| Staleness detector | `useSessionService.ts` | 851–864 | `setInterval` every 5s, marks stale if `lastEventTimeRef > 15s` |
| Auto-watch effect | `useSessionService.ts` | 866–876 | Calls `watchSessions()` on mount, `stopWatching()` on unmount |
| Transport (watch) | `watch-ws-transport.ts` | 84–200 | `createWatchTransport` — no retry; raw WebSocket + `runStreamingCall` |
| Transport (terminal) | `websocket-transport.ts` | 62–334 | `createWebsocketBasedTransport` — bidirectional WS via `it-ws/client`; no retry |
| Terminal connect | `useTerminalStream.ts` | 156–345 | Creates `AbortController`, calls `streamTerminal`, drops to `DISCONNECTED` on error/close — **no reconnect** |
| Terminal disconnect | `useTerminalStream.ts` | 352–387 | Closes `MessageQueue`, aborts controller after 1s timeout |
| `ConnectionIndicator` | `ConnectionIndicator.tsx` | 23–26 | `window.location.reload()` on click — destroys all React state |
| `GlobalSessionServiceProvider` | `SessionServiceContext.tsx` | 61–105 | Single `useSessionService` instance at layout level; `autoWatch: true` |

### Approach A: Shared Utility Hook (`useReconnect`)

**What changes:**
- Extract backoff + jitter + browser-event subscription into `web-app/src/lib/hooks/useReconnect.ts`
- `watchSessions` calls `useReconnect` instead of inline `setTimeout`
- `useTerminalStream` calls `useReconnect` with its `connect` callback
- `ConnectionIndicator` calls `watchSessions()` (already wired via Redux; just needs context access)

**Touchpoints:**
1. `useSessionService.ts` — remove inline backoff loop; call shared hook
2. `useTerminalStream.ts` — add auto-reconnect using shared hook (new `shouldReconnectRef`, similar to `shouldReconnectRef` in session service)
3. New file: `web-app/src/lib/hooks/useReconnect.ts`
4. `ConnectionIndicator.tsx` — change `window.location.reload()` to call `watchSessions()` via context (requires adding `watchSessions` to Redux or exposing via a new context)
5. `web-app/src/lib/utils/retry.ts` — extend or replace with jitter-aware version

**Pros:** Minimal transport-layer risk; each hook retains direct control over reconnect semantics; no interaction with `runStreamingCall` interceptor chain.
**Cons:** Browser lifecycle events must be registered in two places (or a third shared context); `visibilitychange` listeners could accumulate if `watchSessions` is re-called without cleanup.

### Approach B: Transport-Level Retry Wrapper

**What changes:**
- Inside `createWatchTransport.stream()` and `createWebsocketBasedTransport.stream()`, wrap the `next()` call in a loop that catches errors and retries

**Key constraint from `run-call.d.ts`:**
```ts
export declare function runStreamingCall<I, O>(opt: {
    req: Omit<StreamRequest<I, O>, "signal" | "message"> & { message: AsyncIterable<...> };
    next: StreamingFn<I, O>;   // (req) => Promise<StreamResponse<I, O>>
    timeoutMs?: number;
    signal?: AbortSignal;
    interceptors?: Interceptor[];
}): Promise<StreamResponse<I, O>>;
```

`runStreamingCall` returns a `StreamResponse` whose `.message` is an `AsyncGenerator`. The retry must happen at the `message` iteration level — not by re-calling `runStreamingCall` (which would restart the interceptor chain and re-acquire the signal).

**The correct insertion point:**
Inside `next()`, wrap `parseResponses()` (the generator) in a new outer generator that catches errors and loops. However:
- The outer generator must close the existing WebSocket and open a new one.
- The `AbortSignal` passed to `next()` controls the entire stream lifetime; aborting it means intentional cancel, not retry.
- A distinct inner `AbortController` is needed per attempt, parented to the outer `signal` via `signal.addEventListener("abort", ...)`.

**Approach B sequence (watch transport):**
```
runStreamingCall
  └─ next(req)
       └─ [retry loop]
            ├─ new WebSocket(wsUrl)
            ├─ parseResponses() generator  ← yields messages
            │    └─ if error AND !signal.aborted → schedule retry, close WS, reopen
            └─ returns StreamResponse { message: retryingGenerator }
```

**Pros:** All future streams get retry for free.
**Cons:** Subtle — the `EndStreamResponse` guard ("`stream ended without end-stream message`" at `watch-ws-transport.ts:183`) fires on abnormal close; the retry wrapper must distinguish this from a protocol error vs. a transient drop. `afterSeq` replay state is invisible at the transport level (it lives in `useSessionService.ts:160`); transport-level retry would replay from the beginning, not from the last sequence.

### Approach C: Per-Hook (Minimal Change)

**What changes:**
- Add `visibilitychange` / `online` event listeners directly inside `useSessionService.ts`'s `watchSessions` `useEffect`
- Add auto-reconnect loop to `useTerminalStream.ts`'s `connect()` IIFE `catch` block
- Add jitter to both existing backoff delays

**Touchpoints:**
1. `useSessionService.ts` — add browser event listeners inside the existing `useEffect` at line 866; debounce them; wire to `watchSessions()` call
2. `useTerminalStream.ts` — add `shouldReconnectRef` + backoff loop in the `finally` block of the message processing IIFE (lines 333–338)
3. Both files — add `±20%` jitter to `setTimeout` calls
4. `ConnectionIndicator.tsx` — needs `watchSessions` access (see below)

**Pros:** Lowest risk; each hook's reconnect is independently tunable; no transport-layer interaction.
**Cons:** Code duplication; browser events registered in two places.

---

## 2. AbortController Lifecycle

### watchSessions (useSessionService.ts)

```
watchSessions() called
  → abort existing abortControllerRef.current (if any)
  → set shouldReconnectRef.current = true
  → reset reconnectDelayRef.current = 1000

startStream() [async, recursive]
  → abortControllerRef.current = new AbortController()   ← created here
  → pass .signal to clientRef.current.watchSessions(...)
  → for await (const event of stream) { ... }
  → on normal end OR catch:
       if shouldReconnectRef.current:
         setTimeout → startStream()         ← new AbortController next iteration
       else:
         (controller stays null from stopWatching)

stopWatching()
  → shouldReconnectRef.current = false
  → abortControllerRef.current.abort()     ← signals intentional stop
  → abortControllerRef.current = null
```

**Controller lifetime**: one per stream attempt; created at the top of `startStream()`, cleared in `stopWatching()`. The abort signal propagates into `fromWebSocket()` which closes the WS and terminates the generator.

**For the new approach**: If browser events trigger `watchSessions()` restart (which calls `abortControllerRef.current.abort()` first), the signal from the previous attempt is fired and the old IIFE exits cleanly via the `AbortError` guard at line 815. A new controller is then created. This lifecycle is safe for per-hook approach; for transport-level retry, a parent/child signal pattern is needed.

### useTerminalStream

```
connect() called
  → isConnectedRef check (guard against double-connect)
  → abortControllerRef.current = new AbortController()    ← created here
  → pass .signal to clientRef.current.streamTerminal(...)
  → message processing IIFE runs
  → IIFE finally:
       setIsConnected(false)
       setTerminalState('DISCONNECTED')
       [NO reconnect attempt]

disconnect() called
  → isDisconnectingRef.current = true
  → messageQueueRef.current.close()
  → setTimeout 1s → abortControllerRef.current.abort()   ← only path to abort
  → abortControllerRef.current = null
  → isDisconnectingRef.current = false
```

**Critical gap for reconnect**: `abortControllerRef.current` is only nulled in `disconnect()`, not in `connect()`'s error/finally path. If a reconnect is added in the IIFE's `finally`, the old (already-fired) AbortController is still in `abortControllerRef.current`. The reconnect must create a new one before re-calling `connect()`.

**`isDisconnectingRef` conflict**: the terminal reconnect path must NOT set `isDisconnectingRef.current = true` before re-connecting. The reconnect should bypass `disconnect()` entirely — it should reset state directly (`isConnectedRef.current = false`, `abortControllerRef.current = null`, `messageQueueRef.current = null`) then call `connect()`.

**Required new refs for terminal auto-reconnect**:
- `shouldReconnectRef: useRef(false)` — set `true` on `connect()`, `false` on user-initiated `disconnect()`
- `terminalReconnectDelayRef: useRef(1000)` — terminal-specific backoff

---

## 3. ConnectRPC Streaming Generator Chain

### Full call path (watch transport):

```
useSessionService.watchSessions()
  → clientRef.current.watchSessions({ afterSeq }, { signal })
      [ConnectRPC client generated by createClient()]
        → transport.stream(method, signal, timeoutMs, header, input, contextValues)
             ↓  [watch-ws-transport.ts:90]
             → runStreamingCall({
                 req: { stream: true, method, url, message: input, ... },
                 next: async (req) => {           ← THE HOOK POINT
                   const ws = new WebSocket(wsUrl)
                   await ws.onopen
                   ws.send(encodeEnvelope(0, requestMsg))
                   return {
                     message: parseResponses()    ← AsyncGenerator<MessageShape<O>>
                   }
                 },
                 interceptors: [authInterceptor, rpcTimingInterceptor],
                 signal,
               })
              ↓
              [interceptor chain runs around next()]
              ↓
             returns StreamResponse { message: AsyncGenerator }
        ↓
      AsyncGenerator yielded to caller
    ↓
  for await (const event of stream) { ... }   ← useSessionService.ts:794
```

### Retry wrapper insertion point

Inside `next()`, replace the direct `parseResponses()` return with a retrying generator:

```ts
// Inside next(req) in createWatchTransport:
async function* retryingResponses(): AsyncGenerator<MessageShape<O>> {
  while (!signal?.aborted) {
    const ws = new WebSocket(wsUrl);
    // ... setup ...
    try {
      for await (const msg of parseResponses(ws, signal)) {
        yield msg;
      }
      return; // clean server close → stop retrying
    } catch (err) {
      if (signal?.aborted) return; // intentional cancel
      if (isProtocolError(err)) throw err; // don't mask protocol bugs
      await jitterBackoff(delay);
    }
  }
}
return { ...req, message: retryingResponses() };
```

**Key subtlety**: `runStreamingCall` returns a `Promise<StreamResponse>` — the `StreamResponse.message` is the generator. The interceptors see the `StreamResponse` object but consume `.message` lazily. A retry loop INSIDE `next()` works here because each yield to the interceptors is transparent; a new WS connection replaces the old one transparently. The interceptors (auth, timing) are NOT re-run per retry — this is acceptable for auth (cookies are session-scoped) but means the timing interceptor will see the full multi-attempt duration.

**`afterSeq` limitation**: the transport has no knowledge of `lastSeqRef`. Transport-level retry would re-request from seq=0 unless `afterSeq` is threaded through. This is the main reason to prefer per-hook approach (approach A/C) over transport-level (approach B).

### Terminal transport (websocket-transport.ts)

Same pattern: `runStreamingCall` wraps a `next()` that creates a WS via `it-ws/client`. The `input` `AsyncIterable` includes the initial handshake AND subsequent client messages from `MessageQueue`. On reconnect, a NEW `MessageQueue` must be created (the old one was `.close()`d). Retry inside `next()` would need to re-consume the `input` iterable from the beginning, but `MessageQueue` is a one-shot source — it cannot be replayed after close. This makes transport-level retry structurally harder for the terminal stream.

---

## 4. Data Flow for visibilitychange / online Triggers

### Option 4A: Shared context (new `NetworkReconnectContext`)

```
NetworkReconnectContext
  ├─ useEffect: document.addEventListener("visibilitychange", onVisible)
  ├─ useEffect: window.addEventListener("online", onOnline)
  ├─ debounceRef: 200ms timer
  └─ emits: reconnectEvent$ (EventEmitter or callback list)
        ↓                           ↓
  useSessionService           useTerminalStream
  (subscribed via useEffect)  (subscribed via useEffect)
```

**Placement**: wrap inside `GlobalSessionServiceProvider` or at the `layout.tsx` level above it.

**Pros**: single listener registration; N hooks subscribe. Avoids the "multiple `visibilitychange` listeners accumulating" risk from the requirements doc.
**Cons**: adds a new context provider; terminal instances may be unmounted when the event fires.

### Option 4B: Per-hook useEffect (simpler)

```
useSessionService:
  useEffect(() => {
    const onVisible = debounce(() => {
      if (document.visibilityState === "visible" && shouldReconnectRef.current) {
        abortControllerRef.current?.abort(); // kills current (maybe dead) stream
        reconnectDelayRef.current = 0;       // immediate retry
        startStream();
      }
    }, 200);
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("online", onVisible);
    return () => {
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("online", onVisible);
    };
  }, [/* stable: shouldReconnectRef, startStream via ref */]);

useTerminalStream:
  useEffect(() => {
    const onVisible = debounce(() => {
      if (document.visibilityState === "visible" && shouldReconnectRef.current && !isConnectedRef.current) {
        connect(); // already guards against double-connect via isConnectedRef
      }
    }, 200);
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("online", onVisible);
    return () => { /* cleanup */ };
  }, [connect]);
```

**Placement**: each hook registers its own listeners; cleanup in `useEffect` return ensures no stacking when the hook is remounted.

**visibilitychange false positive guard** (from requirements):
- `watchSessions`: check `!isConnectedRef.current` OR check if the stream is silently alive by checking `lastEventTimeRef` recency (less than staleness threshold).
- `useTerminalStream`: `isConnectedRef.current` is already synced to state; if `true`, the stream is live — skip reconnect.

### Recommended: Option 4B for initial implementation

Lower risk, no new context required. The requirements say listeners must be debounced and idempotent — both conditions are met by per-hook `useEffect` cleanup. A shared context can be added later if a third stream type is added.

---

## 5. Event-Command-Policy Table

| Browser Event | Condition | Command | Stream State | Policy |
|---|---|---|---|---|
| `visibilitychange` (hidden→visible) | `shouldReconnectRef = true` AND (`!isConnected` OR `lastEvent > 15s ago`) | Abort current stream → `startStream()` immediately | `disconnected` → `connected` | 200ms debounce; reset delay to 0 before calling |
| `visibilitychange` (hidden→visible) | `shouldReconnectRef = true` AND `isConnected` AND `lastEvent < 15s` | No-op | `connected` | Stream is alive; don't disrupt |
| `visibilitychange` (visible→hidden) | — | No-op | unchanged | Do not kill streams on hide; they stay alive to buffer events |
| `online` (network restored) | `shouldReconnectRef = true` | Abort current stream → `startStream()` immediately | `disconnected` → `connecting` | 200ms debounce; reset delay to 0 before calling |
| `offline` | — | No-op | unchanged | Let the stream fail naturally; next backoff attempt will handle |
| Stream error (unexpected) | `shouldReconnectRef = true` | `listSessions()` reconcile → `setTimeout(reconnectDelay)` → `startStream()` | `disconnected` → `connected` | Backoff doubles each failure (1s→2s→4s…→30s); jitter ±20% |
| Stream end (server close, normal) | `shouldReconnectRef = true` | Same as stream error | `disconnected` → `connected` | Reconnect immediately (delay starts at current `reconnectDelayRef`) |
| `stopWatching()` called | — | `shouldReconnectRef = false` → abort | `disconnected` | No reconnect; intentional stop |
| `ConnectionIndicator` clicked (stale/disconnected) | `watchSessions` ref available | `watchSessions()` restart | `disconnected` → `connected` | Soft reconnect; replaces `window.location.reload()` |
| Backoff timer fires | `shouldReconnectRef = true` | `startStream()` | `disconnected` → `connecting` | With jitter: `delay * (1 + (Math.random() * 0.4 - 0.2))` |
| Terminal stream error/close | `shouldReconnectRef = true` AND `!isDisconnectingRef` | Reset refs → `connect()` after backoff | `DISCONNECTED` → `CONNECTING` | Same jitter policy; re-request scrollback after first message received |
| Terminal `connect()` called | `!isConnectedRef.current` | `abortControllerRef.current = new AbortController()` → `streamTerminal()` | `DISCONNECTED` → `CONNECTING` | Guard prevents double-connect |
| Terminal user disconnect | — | `shouldReconnectRef = false` → close `MessageQueue` → abort | `DISCONNECTED` | Intentional; no reconnect |

---

## 6. Key Implementation Notes

### Jitter formula
```ts
function jitteredDelay(base: number): number {
  const jitter = base * 0.4 * (Math.random() - 0.5); // ±20%
  return Math.max(0, base + jitter);
}
```

### Feature flag wiring
`NEXT_PUBLIC_RECONNECT_V2` is an env var read at build time in Next.js (`output: "export"`). Pattern:
```ts
const RECONNECT_V2 = process.env.NEXT_PUBLIC_RECONNECT_V2 === "true";
```
Guard new code paths behind this flag; existing code paths kept in `else` branches.

### ConnectionIndicator soft reconnect
`ConnectionIndicator` is a presentational component with no context access. To soft-reconnect it needs either:
- **Option 1**: dispatch a Redux action `triggerReconnect` that `useSessionService` listens to via `useEffect`
- **Option 2**: expose `watchSessions` through `SessionServiceContext` (already present in `SessionServiceContext.tsx:46`) and call `useSessionServiceContext().watchSessions()` from `ConnectionIndicator`

Option 2 is cleaner — `SessionServiceContext` already includes `watchSessions`. Update `ConnectionIndicator` to call `useSessionServiceContext().watchSessions()` with stored `watchOptions` (need to capture them, e.g. via a ref in `GlobalSessionServiceProvider`).

### `afterSeq` preservation on reconnect
`lastSeqRef.current` (line 160 of `useSessionService.ts`) must be passed in the reconnect call. The existing `startStream()` already does this (`afterSeq: lastSeqRef.current` at line 789). Browser-event-triggered reconnects must also call `startStream()` (not `watchSessions()` which resets the delay but doesn't reset `lastSeqRef`). The per-hook approach preserves this naturally.

### Terminal scrollback on reconnect (open question from requirements)
Two strategies:
- **Clear + replay**: call `onScrollbackReceived` with empty string first, then request fresh scrollback after `firstMessage` received. Safe but causes a blank flash.
- **Delta-only**: server currently has no sequence number for terminal stream events (unlike session events). Without server-side sequencing, delta-only is not reliable. Recommend clear + replay for v1.

### `retryOperation` utility
The existing `web-app/src/lib/utils/retry.ts` is promise-based and unsuitable for infinite streaming reconnect. It should not be used for this feature. A new streaming-specific backoff utility should be created.

---

## 7. File Change Map (Recommended: Approach A/C hybrid — per-hook with shared utility)

| File | Change |
|---|---|
| `web-app/src/lib/utils/backoff.ts` (new) | `jitteredDelay(base, jitterFraction)` + `BackoffState` class |
| `web-app/src/lib/hooks/useSessionService.ts` | Add `visibilitychange`/`online` listeners; add jitter to `reconnectDelayRef`; reset delay on browser event |
| `web-app/src/lib/hooks/useTerminalStream.ts` | Add `shouldReconnectRef`; add reconnect loop in IIFE `finally`; add browser event listeners |
| `web-app/src/components/layout/ConnectionIndicator.tsx` | Replace `window.location.reload()` with `useSessionServiceContext().watchSessions()` |
| `web-app/src/lib/store/sessionsSlice.ts` | No change needed |
| `web-app/src/lib/transport/watch-ws-transport.ts` | No change for v1 (per-hook preferred) |
| `web-app/src/lib/transport/websocket-transport.ts` | No change for v1 |
| `web-app/next.config.ts` | Add `NEXT_PUBLIC_RECONNECT_V2` to `env` export if needed (or just read `process.env` directly) |
