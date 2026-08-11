# ADR-023: Client Reconnect via Browser Lifecycle APIs

**Status**: Proposed
**Date**: 2026-06-23
**Authors**: Tyler Stapler
**Relates to**: Requirements `project_plans/client-reconnect/requirements.md`

---

## Context

The client has two persistent server-streaming RPCs:

1. **`WatchSessions`** (in `useSessionService.ts`) — delivers real-time session state updates. Uses an exponential backoff reconnect loop (1 s → 30 s, no jitter). The staleness detector runs on a 5-second `setInterval`. The `ConnectionIndicator` button calls `window.location.reload()` when clicked.

2. **`StreamTerminal`** (in `useTerminalStream.ts`) — drives the xterm.js terminal. The existing reconnect guard (`error && connectionAttempts > 0`) never fires on clean WebSocket close because `watch-ws-transport.ts` propagates clean WS close as `push(null)` (a generator return, not an exception). The `error` state remains `null`, so the `TerminalOutput.tsx` component-level `useEffect` reconnect guard never triggers. Users must navigate away and back to restore the terminal after a network disruption.

Three additional problems compound these gaps:

- **No browser lifecycle integration**: neither stream reacts to `visibilitychange` or `online` events. After a tab returns from background or a laptop wakes, streams wait up to 30 s before reconnecting.
- **No jitter**: thundering herd risk when multiple tabs reconnect simultaneously (e.g. laptop wake on a multi-tab user).
- **Stale closure / dual-stream race**: `watchOptions` captured by value at call time; two overlapping `visibilitychange` triggers could open two concurrent streams.

---

## Decision

**Adopt Approach B: per-hook reconnect with a shared `backoff.ts` utility module.**

### Rejected approaches

**Approach A — Transport-level retry wrapper**: wrap `createWatchTransport` / `createWebsocketBasedTransport` to intercept errors and restart the stream internally.

Rejected because: `StreamTerminal` uses a `MessageQueue` object that is one-shot — it cannot be replayed after `.close()`. Each terminal reconnect must construct a fresh `MessageQueue` and re-run the full handshake. A transport wrapper cannot perform this construction because it has no access to React refs. This approach is architecturally blocked for the terminal stream.

**Approach C — Shared `ReconnectManager` React context**: lift reconnect state into a dedicated context that owns a single state machine driving both streams.

Rejected because: the two streams have fundamentally different reconnect payloads (terminal needs fresh `MessageQueue` + `CurrentPaneRequest` handshake; watch stream needs `afterSeq` replay). A single state machine would have to encode stream-type-specific behaviour, recreating the same duplication but at a higher indirection level. Over-engineered for two streams.

### What Approach B does

1. **New `web-app/src/lib/utils/backoff.ts`**: exports `BackoffState` (owns attempt counter + `next()` / `reset()`) and `jitteredDelay(base, cap, attempt)` using the AWS full-jitter formula: `Math.random() * Math.min(cap, base * 2^attempt)`. Also exports `NON_RETRIABLE_WS_CODES` (`Set([4001, 4004])`) and `isRetriableCloseCode(code)`.

2. **`watch-ws-transport.ts`**: change `ws.onclose = () => push(null)` to propagate non-clean closes as `ConnectError` with `ws-close-code` metadata. This surfaces close codes to the hook layer without breaking the abort path (which continues to push `null`).

3. **`useSessionService.ts`**:
   - Replace `reconnectDelayRef` with `BackoffState` instance.
   - Add `watchOptionsRef` (stores most recent options) and `streamGenerationRef` (monotonic counter to prevent dual-stream).
   - Replace `setInterval` staleness detector with event-driven check inside the `visibilitychange` handler.
   - Add `visibilitychange` + `online` listeners (debounced 200 ms) that trigger `watchSessions()` immediately with `backoffState.reset()`.
   - Move `setConnectionState("connected")` dispatch to first received event (not on WebSocket open).
   - Detect seq backwards-jump (server restart) and reset `afterSeq` to 0.
   - Expose `reconnectAttemptCount` in context for `ConnectionIndicator` tooltip.

4. **`ConnectionIndicator.tsx`**: call `useSessionServiceContext().watchSessions()` instead of `window.location.reload()`. Collapse `"stale"` and `"disconnected"` to a single "Reconnecting…" label with spinner. Move `aria-live` to a separate visually-hidden `<div>`. Retain hard reload as a tooltip escape hatch.

5. **`useTerminalStream.ts`**:
   - Add `shouldReconnectRef` and `terminalBackoffRef` (`BackoffState`).
   - In the `finally` block of the `connect()` IIFE, add jittered reconnect loop gated on `shouldReconnectRef` and `isRetriableCloseCode`.
   - Add `visibilitychange` + `online` listeners (same 200 ms debounce) that call `connect()` with `backoffState.reset()`.

6. **`TerminalOutput.tsx`**: add a 2-second debounced reconnecting banner overlay. Append dim `"--- reconnected ---"` separator after successful reconnect. Guard existing component-level reconnect `useEffect` behind `!NEXT_PUBLIC_RECONNECT_V2` so it is only active when the feature flag is off.

7. **Feature flag**: `NEXT_PUBLIC_RECONNECT_V2=true`. Absent = current behaviour. Present = all of the above. Removed after 1-week burn-in.

---

## Consequences

### Positive

- Streams reconnect within 200 ms of `visibilitychange` or `online` events (previously up to 30 s).
- Full jitter eliminates thundering herd on multi-tab wakeup.
- Terminal auto-reconnects without user navigation.
- Soft reconnect in `ConnectionIndicator` preserves React state and Redux store.
- Staleness detection is now event-driven (no polling).
- Close codes 4001 and 4004 correctly suppress reconnect (prevents auth-failure retry storm).
- Feature flag enables safe staged rollout.

### Negative / Trade-offs

- Small duplication (~40 lines) between `useSessionService` and `useTerminalStream` reconnect loops. Accepted: the loops have different payloads and a shared abstraction would obscure the payload differences.
- `NEXT_PUBLIC_RECONNECT_V2` guard must be removed in a follow-up PR or it becomes permanent dead code.
- `watch-ws-transport.ts` now throws `ConnectError` (not `null`) on non-clean WS close, which changes the error shape seen by the hook. The hook already handles `ConnectError` in the `catch` block; this is additive not breaking.

### Neutral

- No new npm dependencies.
- No server-side changes.
- `make ci` must stay green throughout; each phase is independently mergeable.

---

## Glossary

See `project_plans/client-reconnect/implementation/plan.md` — Domain Glossary section — for full term definitions.

---

## Implementation Phases

| Phase | Key Files | Risk |
|---|---|---|
| 1 — Backoff utility | `backoff.ts` | None (new file, no callers yet) |
| 2 — Watch stream | `useSessionService.ts`, `ConnectionIndicator.tsx` | Medium — replaces staleness detector + reload |
| 3 — Terminal stream | `useTerminalStream.ts`, `TerminalOutput.tsx` | Medium — replaces component-level reconnect |
| 4 — Quality | Tests, feature flag, CI | Low |
