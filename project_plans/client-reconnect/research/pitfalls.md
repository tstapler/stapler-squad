# Pitfalls & Risks: client-reconnect

**Date**: 2026-06-23
**Branch**: stapler-squad-reconnect
**Scope**: useSessionService.ts, useTerminalStream.ts, watch-ws-transport.ts, pkg/events/bus.go

---

## 1. React StrictMode double-mount

**Risk level**: Medium

React 18 StrictMode mounts effects, tears them down, then mounts them again synchronously in development. For the reconnect feature this means:

- Any `addEventListener("visibilitychange", ...)` registered inside a `useEffect` will be registered twice if the cleanup does not remove it. The second registration accumulates silently — after the StrictMode remount, two handlers fire on every `visibilitychange` event.
- Each `useEffect` run creates a new `AbortController`. If the cleanup path aborts the controller but the reconnect backoff `setTimeout` was already queued (from the first mount), the callback fires against an aborted signal. The `AbortError` guard catches it, but the `shouldReconnectRef` is already `false` from the cleanup, so the second mount's `startStream()` races against the first mount's pending timer.
- `useSessionService` currently registers no `visibilitychange` listener — the new feature will add one. The safest pattern is to return a cleanup function from `useEffect` that removes the exact handler reference: `window.removeEventListener("visibilitychange", handler)`. Inline arrow functions do not satisfy this — the handler must be a named variable.

**Current state in codebase**: `GlobalSessionServiceProvider` calls `useSessionService` exactly once (SessionServiceContext.tsx:77). StrictMode does not appear to be enabled (`React.StrictMode` is not present in any app file), but Next.js 14 enables it by default in dev mode for pages in the App Router. The risk is real in dev.

---

## 2. WebSocket close codes: transient vs. permanent

**Risk level**: High

The current `fromWebSocket` generator in `watch-ws-transport.ts` (line 46–47) collapses all `onerror` and `onclose` events into a single `push(null)` (close) or `push(new ConnectError(..., Code.Unavailable))` (error). Neither branch inspects the WebSocket close code.

WebSocket close codes that are **permanent** and must not trigger reconnect:

| Code | Meaning | Action |
|------|---------|--------|
| 4001 | Authentication failure (application-defined; common convention) | Do not reconnect — token is invalid |
| 4003 | Forbidden / not authorized | Do not reconnect |
| 4004 | Session not found | Do not reconnect |
| 1008 | Policy violation | Do not reconnect |
| 1011 | Unexpected server error after protocol negotiation (if server sends explicit close with this code) | Reconnect with exponential backoff |
| 1001 | Going away (server restart) | Reconnect immediately |
| 1006 | Abnormal closure (no close frame — network cut) | Reconnect with jitter |
| 1000 | Normal closure | Reconnect only if `shouldReconnectRef` is true |
| 1001–1015 | Protocol-level codes | Reconnect unless 1008 |

The `EndStreamResponse` race described in the requirements is directly caused by abnormal closure (code 1006): the server drops the TCP connection without sending a Connect end-stream envelope, so `parseResponses()` (line 182–186 in watch-ws-transport.ts) throws `ConnectError("stream ended without end-stream message", Code.Internal)`. The current catch block in `startStream()` treats `Code.Internal` as a transient error and reschedules. **This is correct behavior but masks a real bug if it repeats without recovery.** The fix: track consecutive `Code.Internal` occurrences; after N consecutive failures with no successful events in between, surface a non-retriable error.

The transport needs a `CloseEvent` handler on `ws.onclose` that extracts `event.code` and `event.wasClean` before pushing null. Pass the code upstream so the reconnect policy can branch on it.

---

## 3. visibilitychange timing: network not yet restored

**Risk level**: Medium

`document.visibilitychange` fires when the Page Visibility API transitions to `"visible"`. On mobile and laptop wake-from-sleep, this happens before the OS network stack is reestablished. A reconnect attempt issued immediately on `visibilitychange` will fail at the WebSocket handshake (`ws.onerror`) within milliseconds.

The 200ms debounce specified in requirements reduces but does not eliminate this. On a Mac waking from sleep, the Wi-Fi reconnect typically takes 1–3 seconds. A failed attempt will enter the backoff loop, which starts at 1000ms — acceptable. The real concern is that the first failed attempt resets the backoff accumulator or sets an error state visible to the user before a recovery attempt has had a chance to succeed.

**Interaction with `online` event**: The `online` event fires when `navigator.onLine` transitions to `true`, which happens after OS-level network restoration but before Tailscale or VPN layers are operational. The backend at `localhost:8543` is typically local, so this is less of a concern than for remote servers — but if Tailscale is in the path (e.g., mobile app connecting to `onyx.staplerhome.internal:8444`), `online` is a false positive. The reconnect policy should treat both `visibilitychange` and `online` events as triggers for a probe attempt, but not reset the error state until a probe succeeds.

**Recommendation**: After triggering reconnect from a lifecycle event, use the first successful event from the stream (not the successful WebSocket open) as the signal that the connection is healthy. Do not dispatch `setConnectionState("connected")` at WebSocket open time — only do so on the first received `SessionEvent`.

**Current code issue**: `startStream()` dispatches `setConnectionState("connected")` at line 782, before the first message arrives, which means the UI briefly shows "Live" even when the stream is not yet delivering data.

---

## 4. Memory leaks: component unmount during backoff

**Risk level**: High

The `startStream` function (lines 777–836 in useSessionService.ts) is an async recursive function that calls itself after a `setTimeout`. If the component unmounts while the timeout is pending:

1. The `useEffect` cleanup at line 873 calls `stopWatching()`, which sets `shouldReconnectRef.current = false` and aborts the controller.
2. The pending `setTimeout` callback fires after unmount and calls `startStream()`.
3. `startStream()` checks `shouldReconnectRef.current` at line 778 and returns early — **this guard works correctly**.

However, there is a subtler issue: the `listSessions` call inside the reconnect path (lines 803–807 and 823–827) captures `clientRef.current` from the time the async function was entered. If the component unmounts during this await and a new component mounts (reinitializing `clientRef.current` to a new client), the old async invocation holds a stale reference to the old client and will complete its `listSessions` then dispatch `setSessions` to Redux. Because Redux is a singleton store, this dispatch lands in the new component's store — a phantom update from a dead connection.

**Fix**: check `shouldReconnectRef.current` after each `await` (after `listSessions`, after `setTimeout`), not just at the start of `startStream`.

For `useTerminalStream`, the pattern is different: the message processing IIFE (lines 209–339) holds `setIsConnected`, `setError`, and `setTerminalState` in its closure. After unmount, React ignores state updates on unmounted components (React 18 no longer warns, but they are dropped). The `finally` block at line 335 (`setIsConnected(false)`) will fire harmlessly. No leak here, but no reconnect either — terminal has no reconnect at all.

---

## 5. Stale closures in async reconnect loops

**Risk level**: High

`startStream` is defined inside `watchSessions`, which is a `useCallback` with deps `[handleSessionEvent, dispatch]`. The function captures:

- `watchOptions` (from the outer `watchSessions` call) — **stale after the first call**: if `watchSessions` is called with a `categoryFilter` and the filter changes later, the reconnect loop continues with the original filter because `watchOptions` is closed over from the first invocation.
- `clientRef.current` — safe (ref, not stale).
- `abortControllerRef.current` — safe (ref, not stale), but note that each `startStream()` call overwrites `abortControllerRef.current` with a new controller at line 780. If two `startStream` calls overlap (e.g., from a visibilitychange event firing while a reconnect backoff is in progress), both write to the same ref. The second call's abort controller overwrites the first's, leaving the first WebSocket with no abort path. **This is the dual-listener accumulation risk from the requirements.**

- `reconnectDelayRef.current` — safe (ref).
- `shouldReconnectRef.current` — safe (ref).
- `handleSessionEvent` — returned from `useCallback([dispatch])`, stable unless dispatch changes. Redux dispatch is stable. Low risk.

**The most dangerous stale closure**: `watchOptions`. Since `watchOptions` is a plain object parameter to `watchSessions` (not a ref), it is captured by value at the time `watchSessions` is called. If the category filter changes, the caller must call `watchSessions` again with new options — which it does (calling `watchSessions` aborts the old stream at line 770). But if a `visibilitychange`-triggered reconnect calls `startStream()` directly (not `watchSessions`), it will use the stale `watchOptions`. The reconnect logic must always go through `watchSessions` or store the current filter in a ref.

---

## 6. Thundering herd beyond jitter

**Risk level**: Low for this app, worth documenting

The requirements call for 200ms debounce on lifecycle events and jitter on reconnect. The Go backend runs as a single local process (`localhost:8543`). Goroutine overhead per `WatchSessions` connection is modest: one subscriber channel (`bufferSize` events), one goroutine blocked on `select`. For 20 tabs reconnecting simultaneously, the concern is:

- 20 concurrent `EventsSince()` calls: these take `bufMu` lock briefly (binary search over `buf` slice). With 10,000 max events at 1 hour TTL, this is O(log N) per call, ~20 concurrent readers — fine because `bufMu` is a `sync.Mutex` not `RWMutex`, so they serialize. Under burst this adds latency but does not deadlock.
- 20 concurrent `listSessions` calls (the reconcile step before reconnect): each hits the in-memory poller cache (`reviewQueuePoller.GetInstances()`), not SQLite. Likely a `sync.RWMutex` read path — concurrent reads are fine.
- 20 new WebSocket upgrade requests in under 200ms: Go's HTTP server uses a goroutine-per-connection model. 20 goroutines is trivially within limits.

**Verdict**: not a real concern for a local single-user server. Add a note in the feature flag documentation but do not engineer around it.

---

## 7. afterSeq gap: buffer overflow or server restart

**Risk level**: Medium

The event buffer (`pkg/events/bus.go`) has two eviction policies:
1. **TTL-based**: events older than `eventBufTTL` (1 hour) are pruned on every `Publish`.
2. **Size-based**: if `len(buf) > eventBufMaxLen` (10,000), the oldest events are dropped.

When the client sends `afterSeq` that refers to an event that has been pruned (either by TTL or by the 10,000-event cap), `EventsSince` returns all buffered events from the oldest available. The client receives a partial replay — it will be missing events between `lastSeqRef` and the oldest buffered event. This gap is **silent**: neither the server nor the client detects it.

On **server restart**, the `EventBus` is a new instance with `nextSeq` starting at 1. The client's `lastSeqRef` holds a large number from the previous run. `EventsSince(lastSeq)` will return `nil` (no buffered events with seq > lastSeq, since the new bus starts from 1). The client falls into the live path with no snapshot. The `listSessions` reconcile step (lines 803–807) will catch up state, but any events that fired between the reconnect attempt and the reconcile will be missed.

**Detection**: the client could detect a gap by watching for seq numbers to jump backwards (new seq < last seen seq). When this happens, it should treat the connection as a fresh connection (pass `afterSeq: 0`), which triggers the server's full snapshot path (lines 1780–1812 in session_service.go).

**Current behavior**: `lastSeqRef.current` is only updated upward (line 705: `if (event.seq > lastSeqRef.current)`), so a backwards jump from server restart would result in `afterSeq` being sent with the stale large value, getting no replay, and receiving only live events — missing the initial snapshot.

---

## 8. online event false positives

**Risk level**: Medium (higher for Tailscale/remote use)

`window.addEventListener("online", handler)` fires when `navigator.onLine` transitions from `false` to `true`. `navigator.onLine` is set by the OS network interface status — it becomes `true` as soon as any network interface is up, not when the application-layer backend is reachable.

Scenarios where this is a false positive for this app:

1. **Wi-Fi reconnect after sleep**: OS reports online before DHCP assignment completes. A WebSocket to `localhost:8543` will fail (connection refused or timeout) if the server is also waking. On Linux/Mac desktop, the server is a systemd user service that survives sleep, so `localhost:8543` is reachable within milliseconds. Low risk for local use.

2. **Tailscale not yet connected** (mobile/remote users per `~/.claude/projects/.../memory/MEMORY.md`): `navigator.onLine` is true (the device has Wi-Fi) but Tailscale's WireGuard tunnel is not yet established. `onyx.staplerhome.internal:8444` is unreachable for several seconds. The reconnect attempt fails, enters backoff, and recovers — but the backoff may be slow (up to 30s at max) if multiple failures pile up before Tailscale is ready.

3. **Captive portal**: `navigator.onLine` is true, network is up, but HTTP requests are intercepted. WebSocket upgrade to `localhost:8543` would fail with a non-WebSocket HTTP response, causing `ws.onerror`.

**Recommendation**: treat `online` events identically to `visibilitychange` — they trigger a reconnect attempt with the existing backoff jitter, not a backoff reset. Only reset the backoff delay to its minimum when a stream successfully delivers its first event (not when the WebSocket opens). This prevents the backoff from resetting on each false-positive `online` event and then burning through retries.

---

## Summary of highest-priority risks

| # | Risk | Severity | Fix complexity |
|---|------|----------|----------------|
| 2 | WebSocket close code not inspected; permanent errors (auth failure) treated as transient | High | Medium — add `CloseEvent` handler to transport, pass code to reconnect policy |
| 5 | Stale `watchOptions` closure in reconnect loop; dual-stream if visibilitychange fires during backoff | High | Medium — store options in a ref; gate `startStream` on AbortController uniqueness |
| 4 | `listSessions` dispatch fires after unmount; `shouldReconnectRef` not checked after each `await` | High | Low — add three guard checks after awaits |
| 7 | Server restart causes `afterSeq` to reference a newer seq than any buffered event; no initial snapshot sent | Medium | Medium — detect seq backwards jump; pass `afterSeq: 0` on detected gap |
| 3 | `setConnectionState("connected")` dispatched before first event; lifecycle events trigger reconnect before network is ready | Medium | Low — move dispatch to first received event |
| 1 | StrictMode double-mount accumulates visibilitychange listeners | Medium | Low — use named handler variable in cleanup |
| 8 | `online` event fires before Tailscale tunnel is ready; if backoff resets on `online`, rapid retry storm | Medium | Low — never reset backoff on lifecycle events, only on successful stream delivery |
| 6 | 20-tab thundering herd on wake | Low | None required |
