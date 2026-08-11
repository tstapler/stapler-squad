# Requirements: Foreground Fast Reconnect

Source: GitHub issue [TylerStaplerAtFanatics/stapler-squad#170](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/170), migrated as backlog item `49a11c44-4f03-4703-af80-b466115b1eca`.

> **Note (2026-08-10)**: Requirements, research, plan, pre-mortem, and validation for
> this project were already produced in a prior session (2026-08-06 through
> 2026-08-09) — see `implementation/plan.md`, `implementation/pre-mortem.md`,
> `implementation/validation.md`, `research/*.md`. This session re-verified those
> artifacts' cited line numbers against current `HEAD` (they match exactly — zero
> drift) and re-ran the Phase 4 readiness gate inline rather than redispatching
> subagents to redo already-complete, already-adversarially-reviewed work. The
> backlog item's AC list has since been renumbered/expanded to 10 items (was 8,
> AC0-AC7); AC9/AC10 below are new top-level entries but their substance was
> already addressed in `plan.md`'s Risk Control section (pre-mortem Failures #2, #4).

## Motivation

`herdr-web`'s terminal reconnect policy (`web/src/terminalReconnectPolicy.ts`) distinguishes
**background** reconnects (a terminal not currently in view, reconnecting quietly) from
**foreground** reconnects (the terminal the user just switched to, where latency is visible).
For the first N foreground attempts it uses a shorter connect timeout so a focused, disconnected
terminal reconnects snappier than a backgrounded one.

## Current state (verified against this codebase, 2026-08-06; re-verified 2026-08-10)

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

## Acceptance Criteria (backlog item's current 10-item numbering)

1. `useTerminalStream` accepts a `foreground?: boolean` option without changing behavior
   for existing callers that omit it (default `false`).
2. A connect-timeout mechanism (cap on one connection attempt's duration, distinct from
   backoff delay-between-attempts) is added to `backoff.ts` as a standalone
   `connectTimeoutMs()` function.
3. When `foreground` is `true`, the first 2 reconnect attempts use ~1200ms connect-timeout;
   all other attempts (fast window exhausted, or `foreground` is `false`) use ~3500ms.
4. When `foreground` transitions `false→true`, both the backoff attempt counter and the
   fast-attempt counter reset, AND any already-pending stale backoff-delay timer
   (`reconnectTimerRef`) is cleared with an immediate reconnect attempt — not just the
   counters (pre-mortem Failure #1, P1: resetting only counters still leaves a stale
   up-to-30s delay in flight).
5. `TerminalOutput.tsx` passes `foreground: isVisible` into `useTerminalStream`, reusing
   the existing `isVisible` prop already computed correctly at all 3 `SessionDetailView.tsx`
   call sites — no changes needed there or in `XtermTerminal.tsx`.
6. Feature is scoped exclusively to the `NEXT_PUBLIC_RECONNECT_V2`-gated hook path; the
   pre-flag legacy `TerminalOutput.tsx` reconnect path is explicitly out of scope
   (documented, not silently skipped) since it has no automatic retry loop to attach a
   timeout to.
7. Test coverage for: fast vs. normal timeout selection; counter reset on foreground
   transition (incl. stale-timer clearing); connect-timeout abandonment triggering retry
   without getting stuck in `CONNECTING`; a message landing before the timer fires not
   being retroactively aborted (pre-mortem Failure #3, P2); no timer leak on
   `disconnect()`/unmount.
8. No regression to existing `useTerminalStream`, `useTerminalStream.resync.integration`,
   and `TerminalOutput.reconnect` test suites.
9. `FOREGROUND_CONNECT_TIMEOUT_MS=1200` is documented as an unvalidated starting guess
   (herdr-web's real value isn't inspectable — different, unvendored repo) with a required
   pre-broad-rollout validation step against real connect-to-first-message latency on
   VPN/high-RTT links (pre-mortem Failure #2, P1).
10. An activation owner is named for flipping `NEXT_PUBLIC_RECONNECT_V2` on (or filing a
    tracked follow-up) so the feature doesn't ship code-complete but permanently dark
    (pre-mortem Failure #4, P2).

## Target user and success metric (added per triad Product-lens review)

- **Target user**: any stapler-squad user switching between sessions in the web UI whose
  previously-backgrounded terminal has gone stale/disconnected — the reporter (`@TylerStaplerAtFanatics`,
  the primary/sole active user of this app today) is both the source of the complaint and the
  first person who will notice whether it's fixed. This is a personal-tool latency complaint,
  not a segment-targeted feature; "target user" here means "whoever has `NEXT_PUBLIC_RECONNECT_V2`
  enabled and switches sessions often enough to notice reconnect lag," which in practice is a
  small, known population.
- **Success metric (qualitative, since no client metrics pipeline exists — see plan.md's
  Observability Plan)**: after `NEXT_PUBLIC_RECONNECT_V2` is enabled for dogfooding, a
  session-switch onto a disconnected terminal visibly reconnects in ~1-2s instead of up to ~3.5s+,
  confirmed by manual observation (and, if pre-mortem Failure #2's flappiness risk shows up
  instead, that failure mode is the counter-signal that the metric was wrong to skip). No
  automated metric is added in this change — see Risk Control in plan.md for the explicit
  follow-up if manual dogfooding is inconclusive.

## Non-goals

- Does not change the WebSocket close-code retriability rules (`NON_RETRIABLE_WS_CODES`).
- Does not change the backoff delay formula/caps for background reconnects.
- Does not add a UI affordance/indicator for "fast reconnecting" — this is a latency
  optimization, not a new visible feature.
