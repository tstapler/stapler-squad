# Requirements: client-reconnect

**Date**: 2026-06-23
**Type**: feature addition
**Complexity**: 3 — cross-cutting change touching two transports, multiple hooks, state machine

## Problem Statement

The client-side reconnect process for both the session-watch stream (`useSessionService.ts`) and the terminal stream (`useTerminalStream.ts`) does not take advantage of browser lifecycle APIs. When a tab comes back from the background, the network is restored, or a WebSocket drops, the existing code either waits for a slow backoff timer, reloads the full page, or does nothing at all (terminal). This produces a poor UX — stale session lists, lost terminal output — and risks hammering the backend when many tabs reconnect simultaneously after a network event. The fix should be centralised in the transport layer so all current and future streams inherit robust reconnect behaviour for free.

## Baseline

| Surface | Current behaviour | Gap |
|---|---|---|
| `useSessionService` — WatchSessions | Exponential backoff (1s → 30s), no browser signals | Waits up to 30s after tab regains focus or network restores |
| Staleness detector | `setInterval` polling every 5s; marks stale after 15s of silence | Unnecessary polling; not event-driven |
| `ConnectionIndicator` | Calls `window.location.reload()` on click when disconnected | Destroys React state, re-fetches everything, causes full flash |
| `useTerminalStream` | No reconnect logic; drops to `DISCONNECTED` on error/close | User must navigate away and back to restore terminal |
| Backoff jitter | None | Thundering herd: all open tabs reconnect at the same instant after a 30s dropout |
| `watch-ws-transport.ts` | No retry logic; streams propagate errors to callers | Every consumer must implement its own reconnect |
| `websocket-transport.ts` | No retry logic | Same |

## Users / Consumers

- Developers using the stapler-squad web UI, often across multiple browser tabs and from varied networks (Wi-Fi, Tailscale, mobile)
- The Go backend (`server/services/`) — a direct stakeholder in "don't hammer me"

## Success Metrics

- **Visibility reconnect**: after a tab returns from background (`visibilitychange`), all dropped streams reconnect within 1 s (not after the next backoff timer fires)
- **Network reconnect**: after `window` fires `online`, streams reconnect immediately (not after waiting out the current backoff interval)
- **Terminal resilience**: `useTerminalStream` auto-reconnects on WebSocket drop without user navigation; re-applies scrollback to avoid blank terminal
- **Soft reconnect**: clicking the `ConnectionIndicator` in disconnected/stale state triggers a soft reconnect (`watchSessions()` restart), not a page reload
- **Jitter**: all backoff delays include ±20% random jitter so N open tabs do not all hit the server at the same instant
- **No over-calling**: the `online` / `visibilitychange` handlers are debounced; if the network flaps three times in 2 s, only one reconnect attempt fires

## Appetite

Large (3–6 weeks)
*(Centralising reconnect in the transport and threading browser lifecycle signals through to both stream types is the higher-effort path; it is chosen because it future-proofs all streams.)*

## Constraints

- Must not break the existing `useSessionService` `afterSeq` catch-up replay — reconnects must still pass `lastSeqRef.current`
- Transport changes must remain compatible with the server-side `StreamingWSBridge` protocol
- No new npm dependencies unless strictly necessary (browser APIs cover the main need)
- `make ci` must stay green

## Non-functional Requirements

- **Performance SLO**: reconnect attempt must fire within 200 ms of the triggering browser event (visibility, online)
- **Scalability**: solution must not degrade with N tabs open (jitter is the primary control)
- **Security classification**: internal (localhost / Tailscale only)
- **Data residency**: no special requirements

## Scope

### In Scope

- Centralise reconnect policy (backoff with jitter, max-delay cap) into a shared utility or transport-level hook
- Add `document.addEventListener("visibilitychange", ...)` reconnect trigger to WatchSessions
- Add `window.addEventListener("online", ...)` reconnect trigger to WatchSessions
- Debounce the online / visibilitychange handlers (200 ms) to avoid flapping
- Replace `setInterval`-based staleness detector with an event-driven approach (reset timer on each event, fire once on expiry)
- Patch `ConnectionIndicator` to call `watchSessions()` restart instead of `window.location.reload()`
- Add auto-reconnect to `useTerminalStream`: when the stream ends or errors (and `autoConnect` is true), enter backoff and reconnect; re-request scrollback after reconnection
- Add jitter to all existing backoff loops (WatchSessions, future terminal)
- Thread the same browser-event signals to the terminal stream reconnect

### Out of Scope

- Server-side changes
- Protocol changes to `StreamingWSBridge`
- Reconnect for unary calls (they already retry at the call site)
- Mobile / native reconnect behaviour (this is a web-only change)
- Moving the entire transport to a library (e.g. `@connectrpc/connect-web` retry plugin)

## Rabbit Holes

- **Shared transport-level retry**: centralising in the transport `next()` function is clean but the ConnectRPC transport interface doesn't expose a reconnect lifecycle hook natively — this may require wrapping streams in a generator that loops, which interacts subtly with `AbortSignal` propagation and `EndStreamResponse` detection
- **Terminal scrollback on reconnect**: re-requesting scrollback after reconnect risks showing duplicate content if the terminal already has output; need a clear "clear and replay" vs "delta-only" strategy
- **`visibilitychange` false positives**: a tab that is backgrounded but still streaming (e.g. the backend is sending events) should not reset the stream; the handler must check whether the stream is actually dead before initiating a reconnect
- **AbortController lifecycle**: the current `watchSessions` creates a new `AbortController` on every reconnect; this pattern must survive being moved closer to the transport without leaking signal objects

## Alternatives Considered

- **Do nothing / user-triggered reconnect**: status quo; rejected because the ConnectionIndicator's hard reload is jarring and the terminal has no recovery path at all
- **Polling fallback (SSE instead of WebSocket)**: rejected — adds server-side protocol work and is less efficient
- **Third-party retry library** (e.g. `async-retry`, `p-retry`): overkill; the existing `retry.ts` + browser APIs cover the need without an extra dependency
- **React Query / TanStack Query for caching + reconnect**: would require a major refactor of the Redux slice; out of appetite

## Feasibility Risks

- **`EndStreamResponse` race on reconnect**: the existing transport throws `"missing EndStreamResponse"` if the server closes the WebSocket abnormally. Auto-reconnect must not mask this error as a transient failure when it is actually a server-side protocol bug
- **Multiple `visibilitychange` listeners accumulating**: if `watchSessions` is called more than once without cleanup, listeners could stack — must ensure cleanup in `useEffect` return
- **`useTerminalStream` reconnect and `isDisconnectingRef`**: the intentional-disconnect guard (`isDisconnectingRef`) must survive the new auto-reconnect path without blocking legitimate reconnects

## Observability Requirements

- Log each reconnect attempt at `debug` level: which stream, which trigger (backoff timer / visibilitychange / online event), delay used
- Expose reconnect attempt count in `ConnectionIndicator` tooltip ("Reconnecting… attempt 3")
- Existing `localStorage.getItem("debug-terminal")` pattern is acceptable for terminal stream debug logging

## Risk Control

- Feature flag: add `NEXT_PUBLIC_RECONNECT_V2=true` env var; when absent, keep existing behaviour (allows safe rollout and rollback without a deploy)
- Rollback: remove the flag or revert the transport files; no DB migration, no protocol change
- Staged: land transport utility first, wire WatchSessions second, wire terminal stream third — each step is independently shippable

## Open Questions

1. Should the terminal auto-reconnect clear the xterm buffer and replay scrollback, or attempt a "resume from cursor position" approach?
2. Is there a server-side sequence number for terminal stream events analogous to `afterSeq` for session events (would allow lossless resume)?
3. Should the `ConnectionIndicator` show a countdown ("Reconnecting in 8 s") or just a spinner?
4. Should jitter be uniform random or truncated exponential (for tighter clustering)?
