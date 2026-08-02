# ADR-001: `useTerminalStream.connect()` Uses a Single-Increment Epoch Guard, Not `useSessionService.ts`'s Double-Increment Shape

**Status**: Proposed
**Date**: 2026-08-01
**Authors**: Tyler Stapler (via SDD planning session)
**Relates to**: `project_plans/phantom-keystroke-replay/requirements.md`,
ADR-023 (`docs/adr/ADR-023-client-reconnect-browser-lifecycle.md`)

---

## Context

ADR-023 shipped a monotonic generation-counter guard
(`streamGenerationRef`) for `useSessionService.ts`'s `WatchSessions`
reconnect loop, to fix "two overlapping `visibilitychange` triggers could
open two concurrent streams." Its own Decision section named
`useTerminalStream.ts` as needing the identical fix (§5, "Phase 3") but that
phase shipped only `shouldReconnectRef` + `terminalBackoffRef` (jittered
backoff) — no generation guard — leaving the terminal stream exposed to the
same race class this backlog item (`04089969-0f19-499c-be34-2e8bcfc4f13e`)
reports.

`useSessionService.ts`'s `watchSessions()` increments `streamGenerationRef`
**twice** per invocation:

1. Once synchronously at the top of the public `watchSessions()` function
   itself (`useSessionService.ts:829`): *"Invalidate any in-flight
   startStream from prior call."*
2. Again inside the internal `startStream()` closure
   (`useSessionService.ts:833`): `const myGeneration =
   ++streamGenerationRef.current`.

pitfalls.md's research for this project explicitly flags this as a trap:
*"A naive port that increments only once... collapses these two distinct
invalidation events into one."* This ADR records why porting the *single*-
increment shape into `useTerminalStream.connect()` is nonetheless the
correct choice here, not an oversight.

## Decision

**`useTerminalStream.connect()` increments `connectionEpochRef` exactly
once, synchronously, at the very top of `connect()` — before the existing
entry guard's early-return check.** No second increment inside the
message-processing IIFE.

### Why `watchSessions()` needs two increments and `connect()` doesn't

`watchSessions()`'s two increments protect two structurally different call
boundaries:

- **Increment #1** (top of the public function) invalidates a stream started
  by a *previous, separate call* to the public `watchSessions()` API —
  i.e., an **external caller** (e.g. `ConnectionIndicator`'s manual-reconnect
  button, or a consumer component re-invoking the hook's exposed function)
  can call `watchSessions()` again while a prior call's `startStream()` is
  still running.
- **Increment #2** (top of `startStream()`) additionally distinguishes each
  of `startStream()`'s own **internal self-rescheduled retries** (the
  `finally`-equivalent blocks at `useSessionService.ts:891,936` that call
  `startStream()` again after a backoff delay) from one another and from the
  call that originally invoked them.

These are two separate re-entrancy surfaces: external multi-call and
internal self-recursion, layered on top of each other.

`useTerminalStream.connect()` has only **one** of these two surfaces.
Its entry guard —
```ts
if (isConnectedRef.current || isConnectingRef.current || !sessionId) return;
```
(line 163, unchanged by this plan) — already fully suppresses the external
re-entrancy case: while a connect attempt is in flight (`isConnectingRef.current
=== true`) or already connected (`isConnectedRef.current === true`), any
further external call to `connect()` returns immediately without ever
reaching the epoch increment. Unlike `watchSessions()`, there is no
"restart with fresh options" public re-invocation pattern for
`useTerminalStream` that legitimately needs to interrupt an in-flight
attempt from the outside — the hook's own internal reconnect scheduling
(`useTerminalStream.ts:345-350`, the `setTimeout(() => { ... connectRef.current?.() },
delay)` block) is the *only* path that calls `connect()` again after a prior
attempt has already reached its `finally` block (at which point
`isConnectingRef.current` has already been reset to `false`, lines 324/356,
so the entry guard no longer blocks it — this is exactly the reconnect case
the epoch guard needs to protect against, and it is a single self-recursive
surface, not two layered ones).

A single increment, placed at the top of `connect()` itself (before the
entry guard's check, so even an early-returning call still bumps the
counter — matching `usePathCompletions.ts`'s placement discipline of
incrementing before any `await`), is therefore sufficient: it gives every
`connect()` invocation — whether from the initial `autoConnect` effect, the
visibility/online listener, `handleManualReconnect`, or the internal
`setTimeout`-scheduled retry — a distinct epoch, and every one of those
call sites funnels through this single function. There is no second,
independent layer of re-entrancy analogous to `watchSessions()`'s
public-API-vs-internal-retry split for the epoch guard to separately
account for.

### Where the increment is placed relative to the entry guard

Placed **before** `if (isConnectedRef.current || isConnectingRef.current ||
!sessionId) return;`, not after. This means an early-returning call (e.g. a
duplicate `connect()` fired while already connecting) still consumes an
epoch value. This is intentional and harmless — epoch values are opaque
monotonic identifiers, not a resource pool; skipping a value on an
early-return has no observable effect, and placing the increment first
avoids a subtle ordering bug where a *different* logic change to the entry
guard's condition in the future could accidentally let two calls compute
the same epoch value before either increments.

## Consequences

### Positive

- Matches `usePathCompletions.ts`'s simpler, well-proven single-increment
  shape exactly where the reasoning for a single surface actually applies —
  no unnecessary complexity ported from a superficially similar but
  structurally different sibling.
- Keeps the diff small and easy to review against the existing entry guard,
  rather than requiring the reviewer to reconstruct why a second increment
  point would or wouldn't be needed.

### Negative / Trade-offs

- If `useTerminalStream.connect()` later grows a second, `watchSessions()`-style
  public re-invocation surface (e.g. a future "restart with new options"
  API distinct from the current reconnect-only shape), this decision must be
  revisited — a single increment would then under-protect exactly as
  pitfalls.md warns. No such surface exists today; this ADR should be
  updated (or a new one written) if that changes.

### Neutral

- No behavior change to `watchSessions()`/`useSessionService.ts` — this ADR
  concerns `useTerminalStream.ts` only.
- No new npm dependency; no proto/schema change.

---

## Glossary

See `project_plans/phantom-keystroke-replay/implementation/plan.md` —
Domain Glossary section.
