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
once, synchronously, at the top of `connect()` — immediately *after* the
existing entry guard's early-return check has passed, not before it.** No
second increment inside the message-processing IIFE.

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

A single increment, placed at the top of `connect()` itself immediately
after the entry guard's check has passed (so an early-returning,
guard-blocked call never bumps the counter — see "Where the increment is
placed relative to the entry guard" below — while still matching
`usePathCompletions.ts`'s placement discipline of incrementing before any
`await`), is therefore sufficient: it gives every `connect()` invocation
that actually proceeds — whether from the initial `autoConnect` effect, the
visibility/online listener, `handleManualReconnect`, or the internal
`setTimeout`-scheduled retry — a distinct epoch, and every one of those
call sites funnels through this single function. There is no second,
independent layer of re-entrancy analogous to `watchSessions()`'s
public-API-vs-internal-retry split for the epoch guard to separately
account for.

### Where the increment is placed relative to the entry guard

Placed **after** `if (isConnectedRef.current || isConnectingRef.current ||
!sessionId) return;`, not before. An early-returning (guard-blocked) call —
e.g. a duplicate `connect()` fired while a prior attempt is still
CONNECTING or already CONNECTED — must never reach the increment at all.

This was corrected during plan repair (2026-08-02) after `pre-mortem.md`
Failure #1 (P1) identified that incrementing *before* the guard lets a
guard-blocked call still bump `connectionEpochRef.current`. Since the guard
is exactly what currently prevents a second real `connect()` from running
while one is already in flight, a guard-blocked call that still consumes an
epoch would orphan the real in-flight attempt: that attempt's own captured
`epoch` would permanently mismatch `connectionEpochRef.current` at every
later checkpoint (`firstMessage`, `catch`, `finally`), even though no
second real attempt ever started to complete the handoff — silently
stranding the connection (e.g. stuck showing "CONNECTING", or silently
reverting to disconnected) with no console error. This is exactly the
rapid-reconnect flapping scenario (visibility + online listeners firing
close together) this backlog item is about, so letting a guard-blocked
call consume an epoch would reintroduce a version of the very bug this
epoch guard exists to fix.

Placing the increment after the guard means epoch values are still opaque
monotonic identifiers, not a resource pool — the counter simply advances
one *fewer* value than it would have — but now only for calls that
actually proceed past the guard, which is what every checkpoint's
`epoch === connectionEpochRef.current` comparison depends on staying
meaningful. See `implementation/plan.md` Task 3.1.1.1 (implementation) and
Task 3.2.1.0 (the Jest regression test proving a guard-blocked call does
not orphan the real in-flight attempt).

### Addendum — `disconnect()` participates as a reader, not an incrementer

This ADR's analysis above concerns `connect()`-vs-`connect()` re-entrancy
only. During plan repair (2026-08-02), both `implementation/architecture-review.md`
and `implementation/adversarial-review.md` independently identified a second,
distinct interleaving this ADR did not originally address:
`disconnect()`-vs-`connect()` — `disconnect()` (`useTerminalStream.ts:371-413`)
has its own `await` point (the `setTimeout`-backed promise at line 392-407)
and, prior to this repair, its post-await continuation mutated the same
shared refs (`isConnectedRef` via `setIsConnected`, the two decoder refs)
unconditionally, regardless of whether a *different*, independently-triggered
`connect()` call had started and completed while `disconnect()` was still
awaiting. `shouldReconnectRef.current = false` (set synchronously at
`disconnect()`'s entry) does **not** close this gap — it only suppresses a
*future* auto-reconnect from being scheduled; it has no effect on a
`connect()` call that was already triggered by something else (e.g. the
visibility/online listener) before or during `disconnect()`'s await.

This gap was also raised, and left unresolved, in this exact codebase's own
prior (unmerged) adversarial review pass on an earlier version of this hook.
Three independent findings converging on the same gap was treated as strong
signal that it needs an actual code-level guard rather than a documented
"this is fine" justification.

**Resolution**: `disconnect()` now captures `connectionEpochRef.current`
at its own entry (`epochAtDisconnectStart`, read-only — `disconnect()` never
increments the counter, since it is not itself a new connection attempt) and
gates its post-await connection-state mutations (`setIsConnected(false)`,
the two decoder resets) behind `epochAtDisconnectStart ===
connectionEpochRef.current`. Its `isDisconnectingRef.current = false`
bookkeeping reset is not gated — it always runs, since a stuck-`true`
in-progress flag would permanently block all future `disconnect()` calls
regardless of which epoch is current. See `implementation/plan.md` Task
3.1.1.5 for the implementation task and Task 3.2.1.4 for the corresponding
Jest regression test.

This does not change the Decision above (`connect()` still increments
exactly once, for the reasons already argued) — it only extends the epoch
counter's set of *readers* to include `disconnect()`, alongside `connect()`'s
own loop/catch/finally checkpoints.

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
