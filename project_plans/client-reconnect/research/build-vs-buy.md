# Build vs Buy: client-reconnect

**Date**: 2026-06-23
**Feature**: client-reconnect (centralised WebSocket reconnect with backoff+jitter+browser lifecycle APIs)
**Codebase context**: Next.js 15 / React 19 / `@connectrpc/connect-web@2.1.1` / pnpm

---

## Codebase snapshot

Two custom WebSocket transports exist side-by-side:

| File | Used by | WS implementation |
|---|---|---|
| `src/lib/transport/watch-ws-transport.ts` | `useSessionService.ts` | Native `WebSocket` (browser API directly) |
| `src/lib/transport/websocket-transport.ts` | legacy codepath | `it-ws` (node-style async iterator) |

Reconnect logic today lives **in the hook** (`useSessionService.ts`, lines 763-838), not in the transport:
- Backoff state: `reconnectDelayRef` doubles on failure, caps at 30 s, no jitter
- `afterSeq` catch-up replay is already wired (`lastSeqRef.current` → `watchSessions` request)
- A duplicate pattern exists in `useInsightsService.ts` (lines 84-140) — the same manual loop with the same absence of jitter and browser lifecycle events
- A shared `retryOperation()` utility lives at `src/lib/utils/retry.ts` but covers only promise-based (unary) retries, not long-lived streams

There is **no** `visibilitychange` or `online`/`offline` listener anywhere in the streaming path.

---

## Option 1: `reconnecting-websocket` npm package

**Package**: `reconnecting-websocket@4.4.0` (last publish 2020-02-07, single maintainer)

The package wraps a native `WebSocket` and exposes the same `WebSocket` interface, auto-reconnecting on close/error. It is widely used (millions of weekly downloads) and well-understood.

**Compatibility with this codebase**: `watch-ws-transport.ts` uses `new WebSocket(wsUrl)` directly. The package could be a drop-in swap: `new ReconnectingWebSocket(wsUrl)`. However:
- The transport wraps the WS in a custom async-iterator (`fromWebSocket`), which attaches `ws.onmessage`, `ws.onerror`, `ws.onclose`. These callbacks would still work with `ReconnectingWebSocket`.
- **Critical problem**: `afterSeq` replay requires that every new connection send a *fresh* request envelope containing the current `lastSeqRef` value. `ReconnectingWebSocket` reconnects at the raw WS level and re-sends nothing; the connect handshake envelope (with `afterSeq`) would not be re-sent on each internal reconnect. The server would replay from seq 0 or error.
- The package has not been updated in 6 years (no ESM exports, pre-ES2020 API surface) and its GitHub issues list WebSocket API incompatibilities with some bundlers.

**Pros**
- Zero implementation cost for basic reconnect
- Battle-tested raw reconnect loop

**Cons**
- Does not handle the ConnectRPC envelope handshake on reconnect — `afterSeq` breaks
- Abandoned since 2020; no ESM; TypeScript types are a `@types` stub
- Would not add jitter, debounce, or browser lifecycle awareness on its own
- Any fix for the handshake problem requires essentially re-implementing the feature anyway at the transport level

**Verdict: Not recommended.** The `afterSeq` constraint is a hard blocker.

---

## Option 2: `@connectrpc/connect-web` built-in retry for streaming

**Installed version**: `2.1.1` (latest as of 2026-06-23 is `2.1.2`)

Inspecting the installed package at `dist/esm/`:
- The `Interceptor` type (defined in `interceptor.d.ts`) wraps `(next: AnyFn) => AnyFn` where `AnyFn` takes `UnaryRequest | StreamRequest` and returns `Promise<UnaryResponse | StreamResponse>`.
- Interceptors **can** wrap streaming responses by wrapping `StreamResponse.message` (an `AsyncIterable`). A retry interceptor is technically implementable.
- However, `@connectrpc/connect-web` ships **no built-in retry interceptor** of any kind. The library is deliberately minimal on the client side.
- The ConnectRPC roadmap (as of 2026) does not include client-side streaming retry; the recommendation from the maintainers is to handle reconnect at the application layer.
- A streaming interceptor retry is architecturally tricky: the interceptor receives `StreamResponse` only once, wrapping the async-iterable. Catching an error inside that iterable and re-initiating a new stream from inside an interceptor requires re-calling `next(req)` — which works, but loses the `afterSeq` value unless the interceptor itself tracks the last seen sequence and mutates the request, creating coupling between the interceptor and domain-specific request fields.

**Pros**
- No new dependencies
- Interceptors are already used in the transport setup (`createAuthInterceptor`, `createRpcTimingInterceptor`)

**Cons**
- No built-in retry; would require fully custom interceptor code
- Streaming retry interceptor design that handles `afterSeq` mutation is non-trivial and couples transport layer to domain fields
- Easier and cleaner to handle reconnect at the hook level (already where it lives) or in a thin transport-level wrapper with access to domain state

**Verdict: Not recommended** as a source of built-in retry. Viable as an extension point if a custom interceptor approach is preferred, but the transport-level hook approach is simpler.

---

## Option 3: `async-retry` / `p-retry`

**`async-retry@1.3.3`**: Last publish 2021-08-17; no updates in 5 years. Minimalist API (`retry(fn, opts)`). No ESM. Essentially stale.

**`p-retry@8.0.0`**: Actively maintained (last publish 2026-03-26). Pure ESM. Wraps `retry` npm package. Adds `isNetworkError` detection via its `is-network-error` dependency.

**Applicability**: Both are designed for **promise-based retries** (call a function, catch rejection, wait, call again). The streaming reconnect pattern is **not a promise retry** — it's a long-lived `for await` loop that needs to restart. You could wrap `startStream()` in `p-retry`, but:
- `p-retry` would need to treat each stream attempt as a single promise (resolved when the stream ends normally, rejected on error)
- The backoff/delay logic `p-retry` provides overlaps but does not replace the need to track `lastSeqRef` between retries
- `p-retry`'s max-retries model (`retries: N`) doesn't naturally express "retry forever until `shouldReconnect = false`"
- The existing `retryOperation()` utility at `src/lib/utils/retry.ts` already covers the promise-retry pattern for unary calls; these libraries would be additive but redundant

**Pros** (p-retry only)
- Actively maintained, ESM, small (~2 KB)
- `isNetworkError` helper is useful for filtering retryable errors

**Cons**
- Wrong abstraction: designed for promise-retry, not long-lived stream restart
- Does not help with jitter, visibilitychange, online/offline
- Would add a dependency without covering the full scope of the requirement

**Verdict: Not recommended** for the streaming reconnect loop. `p-retry` may be Viable as a drop-in replacement for `retryOperation()` (unary calls) if that code needs improvement, but that is out of scope here.

---

## Option 4: `exponential-backoff` npm package

**Package**: `exponential-backoff@3.1.3` (last publish 2025-10-10, maintained by Coveo team).

Provides `backOff(request, options)` — a generic promise wrapper with configurable jitter strategies (`"full"`, `"decorrelated"`, `"none"`), max delay, max attempts, and retry-condition predicates.

```ts
// Example usage
import { backOff } from "exponential-backoff";
await backOff(() => startStream(), {
  jitter: "full",
  maxDelay: 30_000,
  retry: (e) => !(e instanceof AbortError),
});
```

**Fit with this codebase**: Same wrapping concern as `p-retry` (promise-based), but the jitter algorithms are well-implemented. The codebase's existing backoff in `useSessionService.ts` (line 811: `Math.min(delay * 2, 30_000)`) has **no jitter**, which is the specific bug this requirement calls out. The correctness risks of implementing full-jitter from scratch are low but real (off-by-one on first attempt, max-delay interaction with jitter, handling `Math.random()` seeding).

The jitter math itself is about 5 lines:
```ts
// Full jitter (AWS recommended): random value in [0, cap]
const jitter = Math.random() * Math.min(cap, base * 2 ** attempt);
```

This is simple enough that importing a ~5 KB library purely for this is unnecessary.

**Pros**
- Actively maintained, TypeScript-native
- Correct jitter implementations out of the box (avoids subtle bugs)
- `jitter: "full"` matches AWS best-practice "full jitter" recommendation

**Cons**
- ~5 KB addition for logic that is 5–10 lines of arithmetic
- Same promise-wrapping mismatch as p-retry for long-lived streams
- Adds a dependency for code that is straightforward to verify correct in review

**Verdict: Viable** but unnecessary. The jitter formula is simple enough to implement correctly in a shared utility (extend `src/lib/utils/retry.ts` with a `backoffWithJitter()` helper) without a new dependency.

---

## Option 5: `@tanstack/react-query`

**Package**: `@tanstack/react-query@5.101.1` (latest 2026-06-23, very actively maintained).

React Query provides server-state management with built-in caching, background refetching, stale-while-revalidate, and `refetchOnWindowFocus` / `refetchOnReconnect` hooks that map directly to `visibilitychange` and `online` events.

**Migration cost assessment**:
- The codebase uses `@reduxjs/toolkit` with a `sessionsSlice` Redux store. `useSessionService.ts` dispatches to `setSessions`, `upsertSession`, `removeSession`, `setConnectionState`, `setError`. All session data and connection state lives in Redux.
- React Query is a fundamentally different state management paradigm: it owns the cache, invalidation, and refetch lifecycle. Migrating would require either (a) replacing Redux session state with React Query's cache, touching dozens of `useAppSelector` call sites across the app, or (b) an awkward hybrid where React Query manages fetching but Redux manages derived state — the worst of both worlds.
- `@tanstack/react-virtual` is already installed (a different Tanstack library), but `@tanstack/react-query` itself is not present.
- Bundle impact: React Query adds ~12 KB gzipped. Current total bundle limit is 5 MB (ungzipped), so not a blocker, but non-trivial.
- React Query's `useQuery` / `useInfiniteQuery` are designed for **request-response data**, not long-lived streaming. There is no first-class streaming support; `WatchSessions` would still need manual management.

**Pros**
- `refetchOnWindowFocus` and `refetchOnReconnect` are exactly the browser lifecycle hooks required
- Excellent devtools and community

**Cons**
- Migration from Redux is a multi-week refactor touching the entire session data layer
- No streaming support — the core WatchSessions stream would still be manual
- Out of scope per requirements ("Moving entire transport to a library" is explicitly out of scope)
- Adds bundle weight with no direct streaming benefit

**Verdict: Not recommended** for this feature. The `visibilitychange` and `online` triggers are native browser events that can be wired in ~20 lines without React Query.

---

## Option 6: Native Page Visibility API + Network Information API

**Page Visibility API** (`document.visibilityState`, `visibilitychange` event):
- Baseline support: Chrome 33+, Firefox 18+, Safari 6.1+, Edge 12+
- **No polyfill needed** for any modern browser target
- Standard pattern: `document.addEventListener("visibilitychange", handler)`

**Network Information API** (`navigator.onLine`, `online`/`offline` events):
- `navigator.onLine` and `window.addEventListener("online", handler)` — Baseline: Chrome 4+, Firefox 3.5+, Safari 5+
- `navigator.connection` (Network Information API) for connection type/speed — **not Baseline**: Chrome 61+, no Firefox, no Safari. Unreliable.
- For this feature, only `online`/`offline` events are needed (not `navigator.connection`), which have universal modern browser support.

**Debouncing requirement**: The 200 ms debounce on tab-show/online is trivially implemented with `setTimeout`/`clearTimeout` — no library needed.

**Pros**
- Zero new dependencies
- Universal modern browser support for the specific APIs needed (`visibilitychange`, `online`/`offline`)
- ~15 lines of code to add to a shared hook or utility

**Cons**
- `navigator.connection` is not universally available (but is not needed for this feature)
- Requires care to clean up event listeners on component unmount (standard `useEffect` cleanup pattern)

**Verdict: Recommended.** No polyfill or library needed. Wire `visibilitychange` and `online` directly.

---

## Option 7: Custom build (backoff + jitter + visibilitychange + online from scratch)

**Complexity analysis of what needs to be built**:

1. **Backoff with jitter** — the existing `reconnectDelayRef` doubling loop needs one change: replace `Math.min(delay * 2, 30_000)` with full-jitter: `Math.random() * Math.min(30_000, 1_000 * 2 ** attempt)`. This is ~3 lines.

2. **Max-delay cap** — already present (30 s cap). No change needed.

3. **`visibilitychange` trigger** — ~10 lines in a `useEffect`:
   ```ts
   const handler = () => {
     if (document.visibilityState === "visible" && shouldReconnectRef.current) {
       clearTimeout(debounceRef.current);
       debounceRef.current = setTimeout(() => startStream(), 200);
     }
   };
   document.addEventListener("visibilitychange", handler);
   return () => document.removeEventListener("visibilitychange", handler);
   ```

4. **`online` event trigger** — same pattern (~8 lines).

5. **Centralisation** — the same backoff/jitter/browser-event pattern is duplicated in `useInsightsService.ts`. The requirement calls for a shared utility. A `useStreamReconnect` hook or a `createReconnectingStream` factory in `src/lib/transport/` would centralise this. Estimated: ~80–120 lines including JSDoc.

**Correctness risks**:
- Jitter math: low risk, single formula, easily unit-tested
- Event listener leak: standard useEffect cleanup, well-understood
- Interaction with `afterSeq`: no risk — the seq tracking already lives in `useSessionService`, and `startStream()` already reads `lastSeqRef.current` at call time. Adding `visibilitychange`/`online` as additional triggers to call the same `startStream()` function is additive-only.
- Race condition (tab becomes visible while stream is connecting): mitigated by the 200 ms debounce and `shouldReconnectRef` guard.
- Double-reconnect (e.g., `online` fires while `visibilitychange` is already reconnecting): mitigated by the existing `abortControllerRef.abort()` at the top of `startStream()`.

**Pros**
- No new dependencies (requirement says "no new npm dependencies unless strictly necessary")
- Full control over timing, behaviour, and interaction with `afterSeq` and `AbortController`
- Centralising in `src/lib/transport/` or a shared `useStreamReconnect` hook directly satisfies the requirement
- All existing tests continue to work; the change is additive

**Cons**
- Jitter formula needs a unit test to prevent regression
- The two duplicate backoff patterns (`useSessionService` + `useInsightsService`) need to be refactored to share the utility — moderate but bounded effort

**Verdict: Recommended.** The scope is small (~100–150 lines total across utility + two hook updates), correctness risks are low, and the requirement explicitly prefers no new npm dependencies.

---

## Summary

| Option | Verdict | Key reason |
|---|---|---|
| `reconnecting-websocket` | Not recommended | Breaks `afterSeq` replay; abandoned 2020 |
| `@connectrpc/connect-web` built-in retry | Not recommended | No built-in streaming retry exists in v2.1.1 |
| `async-retry` / `p-retry` | Not recommended | Wrong abstraction (promise-retry vs. stream restart) |
| `exponential-backoff` | Viable | Correct jitter implementations; dependency is unnecessary given simplicity |
| `@tanstack/react-query` | Not recommended | Requires full Redux migration; out of scope |
| Native browser APIs | Recommended | No polyfill needed; `visibilitychange` + `online` universally supported |
| Custom build | Recommended | ~100–150 lines, no deps, full control over `afterSeq` interaction |

---

## Key findings (3-bullet summary)

- **Build custom, use native APIs.** The reconnect logic is ~100–150 lines when centralised: extend `src/lib/utils/retry.ts` with a `backoffWithJitter()` helper (3 lines of math), add `visibilitychange` and `online` event listeners to the streaming hooks (~15 lines each), and extract a shared `useStreamReconnect` hook so `useSessionService` and `useInsightsService` stop duplicating the pattern. No new npm dependencies are justified.

- **`reconnecting-websocket` is a hard no due to `afterSeq`.** The existing catch-up replay mechanism sends the last observed sequence number on every new connection (`afterSeq: lastSeqRef.current`). Any library that reconnects at the raw WebSocket level without re-sending the ConnectRPC handshake envelope breaks this guarantee. All WebSocket-wrapping libraries (including `reconnecting-websocket`) share this flaw.

- **`@connectrpc/connect-web@2.1.1` has no built-in streaming retry.** Interceptors can wrap `AsyncIterable<O>` to catch errors and call `next(req)` again, but this approach couples the transport layer to domain-specific fields (`afterSeq`) and is harder to test than the current hook-level loop. The hook-level approach with a shared utility is simpler, more testable, and already established in the codebase.
