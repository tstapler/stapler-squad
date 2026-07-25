# Stack Research: client-reconnect

**Date**: 2026-06-23
**Feature**: Client-side reconnect improvement (visibilitychange, online/offline, jitter, terminal auto-reconnect)

---

## 1. Browser APIs

### Page Visibility API (`document.visibilityState` / `visibilitychange`)

- **Baseline**: Universally supported. Part of the living standard since Chrome 33, Firefox 18, Safari 7. No polyfill required.
- **Usage pattern**: `document.addEventListener("visibilitychange", handler)` — fires when the tab moves between foreground and background. Check `document.hidden` (boolean) or `document.visibilityState` (`"visible" | "hidden" | "prerender"`).
- **Codebase precedent**: `useSessionVcs.ts` (line 125) and `useVcsStatus.ts` (line 102) already guard poll intervals with `if (!document.hidden)`. `useReviewQueueNotifications.ts` (line 279) checks `document.hidden` before showing browser notifications. The pattern is established; it just needs to be wired to the stream reconnect path.

### `window` online/offline Events

- **Baseline**: `window.addEventListener("online", handler)` — universally supported (same baseline as Page Visibility). `navigator.onLine` boolean is synchronously readable.
- **Network Information API** (`navigator.connection`, `effectiveType`, `downlink`) is a **separate**, non-standard API. It is **not in scope** per the requirements and is not present in the codebase. Do not use it; `online`/`offline` events are sufficient.
- **Polyfill**: None needed for `online`/`offline` events.

### Debounce requirement (200 ms)

- `useDebounce` and `useDebouncedCallback` already exist at `web-app/src/lib/hooks/useDebounce.ts`. Both are value-based debounce hooks using `useState` + `useEffect`/`setTimeout`. They are suitable for debouncing reconnect triggers from `visibilitychange` and `online` events.
- For the reconnect case a raw `setTimeout`-based debounce (stored in a `useRef`) is simpler than the hook form, since the handler is imperative (calls `startStream()`), not reactive state. Either approach works.

### BroadcastChannel

- Already used in `web-app/src/lib/utils/broadcastChannel.ts` for notification sync. Could be used in future to coordinate reconnect across tabs, but is **not in scope** for this feature.

---

## 2. ConnectRPC / connect-web

### Installed versions

| Package | Specifier | Installed |
|---|---|---|
| `@connectrpc/connect` | `^2.1.1` | `2.1.1` |
| `@connectrpc/connect-web` | `^2.1.1` | `2.1.1` |
| `@bufbuild/protobuf` | `^2.11.0` | `2.11.0` |

### Built-in retry/reconnect support

ConnectRPC 2.x **does not include built-in retry or reconnect logic** for streaming calls. The library provides `interceptors` (for unary and streaming) but interceptors operate at the individual RPC level — they cannot restart a streaming call after it ends. The upstream spec (Connect-RPC) defines a retry policy for unary RPCs only.

**Conclusion**: all reconnect logic for streaming calls must live in application code, exactly as the current `watchSessions` implementation does (the `startStream` async loop in `useSessionService.ts`). This is the correct architectural location.

### Streaming transports in this codebase

Two custom transports wrap the ConnectRPC Transport interface:

| Transport | File | Used by | Notes |
|---|---|---|---|
| `createWatchTransport` | `lib/transport/watch-ws-transport.ts` | `useSessionService` | WebSocket for streaming, HTTP fetch for unary. Propagates errors to callers; no retry. |
| `createWebsocketBasedTransport` | `lib/transport/websocket-transport.ts` | `useTerminalStream` | Full bidirectional WebSocket via `it-ws`. Propagates errors to callers; no retry. |

Both transports surface stream errors to their callers via thrown errors/rejected promises. Reconnect must be implemented at the hook layer (caller), not inside the transport.

The `lib/api/transport.ts` singleton (`getConnectTransport`) is for **unary** calls only and is separate from both streaming transports.

---

## 3. React version and hook patterns

### Installed version

React **19.1.1** (`react@19.1.1` from pnpm lockfile). `@types/react` 19.1.16.

### Relevant React 19 patterns for event listener lifecycle

- `useEffect` cleanup with `addEventListener` / `removeEventListener` is the standard pattern. React 19 has not changed this.
- React 19 introduced the `use()` hook (for promises/context) and compiler optimizations, but nothing that changes event listener patterns.
- **`useRef` for stable callbacks**: The codebase already uses this pattern extensively — `onReconnectRef`, `onNotificationRef`, `onApprovalResponseRef` in `useSessionService.ts` (lines 124–141). This is the correct approach for capturing the latest callback value in a long-lived effect without triggering re-renders.
- **`useCallback` for stable function identity**: Used for `watchSessions`, `stopWatching`, `connect`, `disconnect`. A new `watchSessions` call caused by `visibilitychange` should call the existing stable reference, not trigger re-renders.
- **Pattern for adding browser event listeners in hooks**: Canonical form already established in `useBrowserLogStream.ts` (lines 167–208) — `useEffect` that adds event listeners and returns a cleanup function that removes them.

### React 19 `useEffectEvent` (experimental)

`useEffectEvent` (proposed as `useEvent` in the RFC, now `useEffectEvent` in canary) is available in React 19 canary but **not in stable**. Do not use it; `useRef` + manual sync in `useEffect` is the correct stable substitute.

---

## 4. Existing codebase utilities to reuse

### `lib/utils/retry.ts`

Exports `retryOperation<T>` (exponential backoff for promise operations) and `isRetryableError`. Currently used for **unary** call retries. Not suitable as-is for streaming reconnect because:
1. It wraps a finite promise, not an infinite stream loop
2. No jitter (see section 5)
3. The streaming reconnect uses a persistent `reconnectDelayRef` across iterations, which `retryOperation`'s closure cannot model

**Recommendation**: extend `retry.ts` to export a `computeBackoffDelay(attempt, options)` pure function that includes jitter. Both `retryOperation` and the streaming reconnect loop can then call this shared primitive.

### `lib/hooks/useDebounce.ts`

Exports `useDebounce<T>` (value debounce) and `useDebouncedCallback<T>` (callback debounce). Can be used to debounce the `online`/`visibilitychange` reconnect handler. The `useDebouncedCallback` form is closest to what's needed, but the `setTimeout`-in-ref approach may be more lightweight for a single imperative trigger.

### `lib/store/sessionsSlice.ts` — `ConnectionState`

```typescript
export type ConnectionState = "connected" | "stale" | "disconnected";
```

No schema changes needed. The `visibilitychange` and `online` handlers will call the existing `watchSessions()` entry point, which already calls `dispatch(setConnectionState(...))`.

### `lib/hooks/useSessionService.ts` — existing reconnect loop structure

The `startStream` async function (lines 777–838) already handles:
- `shouldReconnectRef` guard (only reconnects when desired)
- `reconnectDelayRef` exponential backoff (1s → 30s, doubling)
- `listSessions()` reconciliation on reconnect
- `onReconnect` callback invocation

What is missing:
1. Jitter on `reconnectDelayRef`
2. `visibilitychange` → immediate reconnect (reset delay, call `startStream`)
3. `online` event → immediate reconnect (same)
4. Debounce on both triggers (200 ms per requirements)

### `lib/hooks/useTerminalStream.ts` — no reconnect at all

The terminal hook has `connect`/`disconnect` callbacks and an `autoConnect` option but **no reconnect loop**. The `finally` block in the message-processing async IIFE (lines 334–338) sets `isConnected = false` and `terminalState = DISCONNECTED` but does not attempt to reconnect. A reconnect loop following the same `shouldReconnect + backoff + browserEvent` pattern as `useSessionService` needs to be added.

---

## 5. Backoff + jitter patterns in TypeScript

### Problem: thundering herd

Without jitter, all tabs reconnect at the same instant after a shared outage (network drop, server restart). With N open tabs each having a 30s delay, they all fire simultaneously, overwhelming the server.

### Recommended algorithm: "Full Jitter" (AWS blog, 2015)

The most widely recommended pattern for distributed reconnect:

```typescript
function computeBackoffDelay(
  attempt: number,
  baseMs: number = 1000,
  capMs: number = 30_000
): number {
  // Full jitter: random in [0, min(cap, base * 2^attempt)]
  const ceiling = Math.min(capMs, baseMs * Math.pow(2, attempt));
  return Math.random() * ceiling;
}
```

Alternatives:
- **Equal Jitter**: `ceiling/2 + Math.random() * (ceiling/2)` — avoids very short waits
- **Decorrelated Jitter**: `Math.random() * (3 * prevDelay - baseMs) + baseMs` — produces different distribution
- **"Capped exponential + random fraction"** (common simpler form): `min(cap, base * 2^attempt) * (0.5 + Math.random() * 0.5)`

**Recommendation for this codebase**: Full Jitter is simplest to reason about and sufficient. The existing code uses `reconnectDelayRef.current * 2` (capped at 30s) without any random component. Change it to:

```typescript
const jitter = Math.random();          // [0, 1)
const delay = Math.min(
  reconnectDelayRef.current * 2 * jitter,
  30_000
);
```

Or extract to a shared `computeBackoffDelay(attempt, base, cap)` in `retry.ts`.

### Debounce for browser events

The `online` and `visibilitychange` events can fire in bursts (e.g., rapid network flapping). The 200 ms debounce specified in requirements means: only reconnect if the trigger has been stable for 200 ms. Standard `setTimeout` pattern:

```typescript
let debounceTimer: ReturnType<typeof setTimeout> | null = null;
function scheduleReconnect() {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    debounceTimer = null;
    // reset delay and re-enter startStream
    reconnectDelayRef.current = 1000;
    startStream();
  }, 200);
}
```

Store `debounceTimer` in a `useRef` to survive across renders without triggering re-renders.

---

## 6. Key gaps to close

| Gap | Location | Required change |
|---|---|---|
| No jitter | `useSessionService.ts` line 811, 831 | Add `Math.random()` factor to `reconnectDelayRef` update |
| No browser event triggers | `useSessionService.ts` | Add `useEffect` wiring `visibilitychange` + `online` → debounced `startStream()` reset |
| Staleness detector is polling | `useSessionService.ts` lines 852–864 | Replace `setInterval` with event-driven: `visibilitychange` visible→ check `lastEventTimeRef` once; no interval |
| ConnectionIndicator calls `window.location.reload()` | `ConnectionIndicator.tsx` line 25 | Replace with call to `watchSessions()` restart (needs the function threaded in via props or context) |
| Terminal has no reconnect | `useTerminalStream.ts` lines 332–338 | Add `shouldReconnectRef` + backoff loop mirroring `useSessionService` |
| No shared jitter utility | `lib/utils/retry.ts` | Add `computeBackoffDelay(attempt, base, cap)` export |

---

## Summary

- **Browser APIs**: `visibilitychange` and `online`/`offline` are baseline — no polyfill needed. The Network Information API (`navigator.connection`) is not needed and not in scope. The codebase already uses `document.hidden` in two hooks, establishing the pattern.
- **ConnectRPC 2.1.1** has no built-in streaming retry. All reconnect logic must live at the hook layer (already the case for `useSessionService`; `useTerminalStream` needs it added).
- **React 19.1.1**: No new APIs needed. `useEffect` + `useRef`-stabilized callbacks is the right pattern. `useDebounce.ts` and `retry.ts` are the key existing utilities to extend.
