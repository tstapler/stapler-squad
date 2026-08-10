# Requirements: Foreground Fast Reconnect

Source: GitHub issue [TylerStaplerAtFanatics/stapler-squad#170](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/170), migrated as backlog item `49a11c44-4f03-4703-af80-b466115b1eca`.

## Motivation

`herdr-web`'s terminal reconnect policy (`web/src/terminalReconnectPolicy.ts`) distinguishes
**background** reconnects (a terminal not currently in view, reconnecting quietly) from
**foreground** reconnects (the terminal the user just switched to, where latency is visible).
For the first N foreground attempts it uses a shorter connect timeout so a focused, disconnected
terminal reconnects snappier than a backgrounded one.

## Current state (verified against this codebase, 2026-08-06)

- `web-app/src/lib/hooks/useTerminalStream.ts` uses one `BackoffState(1000, 30_000)`
  (`terminalBackoffRef`, [useTerminalStream.ts:108](../../web-app/src/lib/hooks/useTerminalStream.ts#L108))
  for every reconnect, regardless of whether the session is in view. `BackoffState` (
  `web-app/src/lib/utils/backoff.ts`) only produces the **delay before the next retry
  attempt** (full-jitter exponential backoff) — there is no separate "connect timeout" concept
  (a cap on how long one connection attempt is allowed to hang) anywhere in this hook or in
  `backoff.ts` today. This is a real gap vs. the herdr-web reference, which times out the
  *attempt itself*, not just the *gap between* attempts.
- The hook already has a tab-visibility reconnect path (Story 3.1.3,
  [useTerminalStream.ts:433-458](../../web-app/src/lib/hooks/useTerminalStream.ts#L433-L458)):
  on `visibilitychange`/`online` it resets the backoff counter and calls `connect()`
  immediately (0ms extra delay, only a 200ms debounce). That path is gated behind
  `NEXT_PUBLIC_RECONNECT_V2` and fires on *browser tab* visibility, not on *in-app session
  selection* — switching between sessions inside a single open tab does not go through it.
- All hook-level reconnect logic (`terminalBackoffRef`, the visibility listener, the
  `shouldReconnectRef`/`isHardFailedRef` state machine) is gated behind the
  `NEXT_PUBLIC_RECONNECT_V2` env flag, which defaults **off**
  (`web-app/.env.local.example:1`: `# NEXT_PUBLIC_RECONNECT_V2=true`). When the flag is off,
  reconnect is instead driven by the older path in `TerminalOutput.tsx`
  (`reconnectTimeoutRef`, lines ~736-778, ~990-992), which this item does not currently cover.
- There is no `foreground` option on `useTerminalStream` today, and no wiring from
  `SessionDetail`/`XtermTerminal`/`SessionDetailView` down to the hook indicating "this
  terminal is the one currently selected/visible in the UI."

## Proposed change (from the issue, refined against current code)

1. Add a `foreground?: boolean` option to `useTerminalStream` (`UseTerminalStreamOptions`).
2. When `foreground` transitions `false → true` (or is `true` on connect), use a shorter
   connect timeout for the first 2 reconnect attempts, and reset the backoff counter.
3. Wire `foreground={isSelected}` from the session-detail view (`SessionDetailView.tsx`)
   down through `XtermTerminal`/`TerminalOutput` to the hook.

## Acceptance Criteria

0. `useTerminalStream` accepts a `foreground?: boolean` option without changing behavior
   for existing callers that omit it (default `false`/current behavior).
1. A **connect-timeout** mechanism exists: if a reconnect attempt's WebSocket does not reach
   an established/first-message state within the active timeout, the attempt is abandoned and
   the normal backoff/retry path takes over — this mechanism does not exist in `backoff.ts`
   today and must be added, not repurposed from the existing delay-between-attempts logic.
2. When `foreground` is `true`, the first 2 reconnect attempts use a short connect timeout
   (~1200-1500ms, matching herdr-web's `TERMINAL_FOREGROUND_CONNECT_TIMEOUT_MS`); subsequent
   attempts (or all attempts when `foreground` is `false`) use the existing/longer timeout
   (~3500ms, matching `TERMINAL_CONNECT_TIMEOUT_MS`).
3. When `foreground` transitions from `false` to `true` (session just selected), the backoff
   attempt counter resets so the fast-timeout window is available immediately rather than
   being exhausted by prior background attempts.
4. `SessionDetailView`/`XtermTerminal` passes `foreground={isSelected}` (or equivalent "this
   terminal is the one currently visible to the user") into `useTerminalStream`.
5. Behavior is scoped correctly relative to the existing `NEXT_PUBLIC_RECONNECT_V2` flag and
   the pre-flag `TerminalOutput.tsx` reconnect path — the plan must state explicitly which
   path(s) this feature applies to, rather than silently only covering the V2 path while the
   flag is off by default.
6. Unit test coverage for: fast timeout used on first 2 foreground attempts, normal timeout
   used after 2 attempts or when not foreground, backoff/attempt-counter reset on
   `false → true` foreground transition, and that connect-timeout abandonment triggers the
   existing retry path rather than leaving the hook stuck in `CONNECTING`.
7. No regression to existing `useTerminalStream`/`useTerminalStream.resync.integration` test
   suites (`web-app/src/lib/hooks/__tests__/`).

## Non-goals

- Does not change the WebSocket close-code retriability rules (`NON_RETRIABLE_WS_CODES`).
- Does not change the backoff delay formula/caps for background reconnects.
- Does not add a UI affordance/indicator for "fast reconnecting" — this is a latency
  optimization, not a new visible feature.
