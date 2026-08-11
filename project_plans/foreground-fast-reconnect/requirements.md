# Requirements: Foreground Fast Reconnect

Source: backlog item `49a11c44-4f03-4703-af80-b466115b1eca` (migrated from
[TylerStaplerAtFanatics/stapler-squad#170](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/170)).
Interactive ideate interview skipped per pipeline instructions — requirements
below are derived directly from the item's 10 acceptance criteria.

## Problem

`useTerminalStream.ts`'s reconnect loop (gated behind `NEXT_PUBLIC_RECONNECT_V2`)
uses one `BackoffState(1000, 30_000)` for every reconnect, regardless of
whether the terminal is currently the one the user is looking at. There is no
per-attempt connect timeout at all today — a hung connection attempt is only
bounded by whatever the underlying transport/stream does. herdr-web's
`terminalReconnectPolicy.ts` demonstrates a foreground/background split:
focused terminals get a short connect timeout for their first few attempts so
tab switches feel snappy; backgrounded terminals use a longer, more patient
timeout.

## Functional Requirements

1. **New hook option** — `useTerminalStream` accepts `foreground?: boolean`
   (default `false`). Omitting it must not change behavior for any existing
   caller.
2. **New backoff primitive** — `backoff.ts` gets a standalone
   `connectTimeoutMs()` function: a cap on the duration of a *single*
   connection attempt, orthogonal to `BackoffState`'s delay-*between*-attempts.
3. **Fast-window policy** — while `foreground` is `true`, the first 2
   reconnect attempts use ~1200ms connect-timeout. Every other case (fast
   window exhausted, or `foreground` is `false`) uses ~3500ms.
4. **Foreground transition resets state, not just counters** — on
   `false → true`: reset the backoff attempt counter, reset the fast-attempt
   counter, clear any pending `reconnectTimerRef` stale-delay timer, and fire
   an immediate reconnect attempt. (A pre-mortem on the original plan found
   that resetting only counters still leaves an already-scheduled long delay
   in flight — the user selects a terminal and it silently waits out up to a
   stale 30s backoff timer before the reset counters ever get used.)
5. **Wiring** — `TerminalOutput.tsx` passes `foreground: isVisible` into
   `useTerminalStream`. `isVisible` is already correctly computed at all 3
   `SessionDetailView.tsx` call sites (`poolPath === ...`, `poolId === ...`,
   `activeTab === "browser"` / `activeTabId === shellKey`) — no changes needed
   there or in `XtermTerminal.tsx`.
6. **Scope boundary** — this only applies to the `NEXT_PUBLIC_RECONNECT_V2`
   reconnect-loop path. The pre-flag legacy reconnect path in
   `TerminalOutput.tsx` has no automatic retry loop to attach a connect
   timeout to and is explicitly out of scope — documented, not silently
   dropped.
7. **Test coverage** for: fast vs. normal timeout selection; counter reset on
   foreground transition including the stale-timer-clearing fix; connect-
   timeout abandonment triggering a retry without the state machine getting
   stuck in `CONNECTING`; a message that lands before the timer fires is not
   retroactively aborted; no timer leak on `disconnect()`/unmount.
8. **No regression** to `useTerminalStream.test.ts`,
   `useTerminalStream.resync.integration.test.ts`, or
   `TerminalOutput.reconnect.test.tsx`.
9. **Document the 1200ms constant as an unvalidated guess.** herdr-web's real
   production value isn't inspectable from source alone — this repo is
   picking 1200ms by analogy. Must be documented in code (comment) as
   needing validation against real connect-to-first-message latency on
   VPN/high-RTT links before a broad rollout.
10. **Name an activation owner.** `NEXT_PUBLIC_RECONNECT_V2` gates the whole
    reconnect-loop feature (not just this change) and is off by default in
    this codebase today — ship must name who flips it on, or file a tracked
    follow-up item, so this doesn't ship code-complete but permanently dark.

## Non-Functional / Constraints

- Vanilla-extract / CSS rules N/A — this is hook + wiring only, no styling.
- Follow existing patterns in `backoff.ts` (pure functions + a small stateful
  class) rather than introducing a new abstraction.
- `useTerminalStream` is already a large hook; keep the diff additive — no
  unrelated refactors.
- Per `.claude/rules/feature-testing-registry.md` / session-creation-registry:
  this is not a new omnibar action, detector, or session-creation mode, so
  those registries are not implicated. Confirm during research.

## Out of Scope

- The pre-`NEXT_PUBLIC_RECONNECT_V2` legacy reconnect path (AC6).
- Flipping `NEXT_PUBLIC_RECONNECT_V2` on by default (AC10 only requires
  naming an owner/follow-up, not flipping the flag).
- Any change to `XtermTerminal.tsx` or the `SessionDetailView.tsx` visibility
  computation itself (AC5 says these are already correct).

## Acceptance Criteria (verbatim from backlog item)

1. `useTerminalStream` accepts a `foreground?: boolean` option without
   changing behavior for existing callers that omit it (default `false`).
2. A connect-timeout mechanism (cap on one connection attempt's duration,
   distinct from backoff delay-between-attempts) is added to `backoff.ts` as
   a standalone `connectTimeoutMs()` function.
3. When `foreground` is `true`, the first 2 reconnect attempts use ~1200ms
   connect-timeout; all other attempts (including once the fast window is
   exhausted, or when `foreground` is `false`) use ~3500ms.
4. When `foreground` transitions `false→true`, both the backoff attempt
   counter and the fast-attempt counter reset, AND any already-pending stale
   backoff-delay timer (`reconnectTimerRef`) is cleared with an immediate
   reconnect attempt.
5. `TerminalOutput.tsx` passes `foreground: isVisible` into
   `useTerminalStream`, reusing the existing `isVisible` prop — no changes
   needed there or in `XtermTerminal.tsx`.
6. Feature is scoped exclusively to the `NEXT_PUBLIC_RECONNECT_V2`-gated hook
   path; the pre-flag legacy path is explicitly out of scope (documented).
7. Unit/integration test coverage for: fast vs normal timeout selection,
   counter reset on foreground transition (incl. stale-timer clearing),
   connect-timeout abandonment triggering retry without getting stuck in
   `CONNECTING`, a message landing before the timer fires not being
   retroactively aborted, no timer leak on `disconnect()`/unmount.
8. No regression to existing `useTerminalStream`,
   `useTerminalStream.resync.integration`, and `TerminalOutput.reconnect`
   test suites.
9. `FOREGROUND_CONNECT_TIMEOUT_MS=1200` documented as an unvalidated
   starting guess with a required pre-broad-rollout validation step.
10. An activation owner is named for flipping `NEXT_PUBLIC_RECONNECT_V2` on
    (or a tracked follow-up filed).
