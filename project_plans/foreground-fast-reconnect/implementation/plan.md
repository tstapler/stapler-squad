# Implementation Plan: foreground-fast-reconnect

**Feature**: Shorter connect-timeout for the first 2 reconnect attempts of the terminal the user has currently selected, so a stalled foreground handshake is abandoned and retried sooner than a backgrounded one.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: None — small, well-scoped hook + backoff-utility change; no new subsystem, no unusual technology, no irreversible/high-blast-radius decision. (Per the task brief's own framing: this is a React hook + backoff-utility change, not architecture requiring a decision record.)

**Scope framing** (do not over-architect): this plan touches exactly 3 source files (`backoff.ts`, `useTerminalStream.ts`, `TerminalOutput.tsx`) plus their existing test files. No new modules, no new npm dependency (confirmed in `research/build-vs-buy.md`), no new React context, no UI changes (non-goal). `SessionDetailView.tsx` and `XtermTerminal.tsx` require **no changes** — `research/architecture.md` §2 corrects the requirements doc's assumption that they're touchpoints.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `foreground` | New optional boolean on `UseTerminalStreamOptions`; `true` when this hook instance's terminal is the one currently selected/displayed to the user. | Sourced from `TerminalOutput`'s existing `isVisible` prop — not a new UI concept. |
| `foregroundRef` | `useRef<boolean>` mirror of the `foreground` prop, updated in a dedicated transition effect so `connect()` always reads the current value, not a stale closure. | Mirrors the existing `isConnectedRef`/`shouldReconnectRef` ref-mirroring convention already in this file. |
| foreground false→true transition | The moment `foregroundRef.current` flips from `false` to `true` between renders (session just selected, was not selected before). | Detected in a new standalone `useEffect([foreground])`, distinct from the mount effect and the visibilitychange effect. |
| connect-timeout | A cap on how long a single connect attempt may wait for its first stream message before being abandoned. Distinct from backoff *delay* (gap between attempts). | Did not exist anywhere in this codebase before this feature (`research/stack.md`). |
| `foregroundConnectAttemptRef` | New, independent `useRef<number>` counting connect attempts since the terminal most recently became foreground. Incremented once per `connect()` call; reset to `0` only on the foreground false→true transition. | **Must not** reuse `terminalBackoffRef.current.attempt` — that value is always `0` when read, because `connect()` unconditionally calls `terminalBackoffRef.current.reset()` at its own entry (line 166), so `.attempt` never survives to be read by a later attempt (`research/architecture.md` §4). |
| `terminalBackoffRef` / `BackoffState` | Existing shared backoff-delay tracker (`web-app/src/lib/utils/backoff.ts`), one instance per hook, `BackoffState(1000, 30_000)`. Governs delay *between* attempts. Untouched by this feature except for one new `.reset()` call site. | `web-app/src/lib/hooks/useTerminalStream.ts:108`. |
| `connectTimeoutMs(foreground, attemptsSinceForeground)` | New pure function in `backoff.ts`, alongside `jitteredDelay`. Returns `FOREGROUND_CONNECT_TIMEOUT_MS` if `foreground && attemptsSinceForeground < FOREGROUND_FAST_ATTEMPTS`, else `CONNECT_TIMEOUT_MS`. | Pure, no React/timer dependency — matches `jitteredDelay`'s existing shape and testability. |
| `FOREGROUND_CONNECT_TIMEOUT_MS` | New constant, `1200` (ms). Fast connect-timeout for the first `FOREGROUND_FAST_ATTEMPTS` foreground attempts. | Chosen at the lower bound of AC2's "~1200-1500ms" range — the exact value herdr-web's `TERMINAL_FOREGROUND_CONNECT_TIMEOUT_MS` uses isn't inspectable (different repo, unvendored — `research/build-vs-buy.md` §4), so this re-derives directly from the AC's own numbers. |
| `CONNECT_TIMEOUT_MS` | New constant, `3500` (ms). Normal connect-timeout: used for background attempts, and for foreground attempts once `FOREGROUND_FAST_ATTEMPTS` is exhausted. | Matches AC2's "~3500ms, matching `TERMINAL_CONNECT_TIMEOUT_MS`". Named without a "BACKGROUND\_" prefix because it also applies to foreground attempt 3+, not only backgrounded terminals. |
| `FOREGROUND_FAST_ATTEMPTS` | New constant, `2`. Number of connect attempts (since the foreground transition) eligible for the fast timeout. | Matches AC2's "first 2 reconnect attempts". |
| `connectTimeoutRef` | New `useRef<ReturnType<typeof setTimeout> \| null>` — the pending connect-timeout timer for the in-flight attempt. | Must be cleared at all 4 existing timer-cleanup sites this file already has a pattern for (success, `finally`, `disconnect()`, unmount) — see Epic 2.3/2.4. |
| `attemptController` | A **local `const`** capturing `abortControllerRef.current` at the moment the connect-timeout timer is scheduled (inside `connect()`, right after `abortControllerRef.current = new AbortController()`). | The timer callback aborts this captured reference, not a live re-read of `abortControllerRef.current` — prevents a stale timer from a superseded attempt from cross-aborting a newer one (`research/architecture.md` §1, `research/pitfalls.md` §3). |
| `firstMessage` | Existing local boolean inside `connect()`'s message-processing IIFE (`useTerminalStream.ts:219`) that flips `false` when the first stream message arrives. | The connect-timeout's success/clear signal — same flag, no new one needed. |
| `NEXT_PUBLIC_RECONNECT_V2` | Existing env flag gating all hook-level auto-reconnect logic in `useTerminalStream.ts`. Defaults **off**. | This feature is scoped exclusively to the code paths already gated by this flag — see AC5 below. |
| `isVisible` | Existing prop on `TerminalOutputProps` (`TerminalOutput.tsx:65`), already computed correctly by `SessionDetailView.tsx` at all 3 call sites as "is this pooled terminal the one currently selected/displayed." | Reused as the source value for `foreground`, not duplicated as a second prop. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Timeout-duration selection | Pure strategy function `connectTimeoutMs(foreground, attemptsSinceForeground)` co-located in `backoff.ts` next to `jitteredDelay` | Strategy pattern (GoF); type-driven "pure policy function" per `type-driven-design` skill | (a) Inline the fast/normal calculation directly inside `connect()` with magic numbers | Scatters policy constants into an already-490-line hook file; harder to unit-test without mounting the hook; breaks the convention `backoff.ts` already established for this exact class of pure, dependency-free policy helper (`isRetriableCloseCode`, `getWsCloseCode`, `jitteredDelay`) |
| Timeout-duration selection (cont'd) | — | — | (c) New sibling module `ForegroundTimeoutPolicy` class mirroring herdr-web's file shape | `research/build-vs-buy.md` §4 found this fragments one small, already-cohesive unit of attempt-based policy logic (delay formula + timeout formula are two properties of the same "attempt N" state) across two files for no isolation benefit, and would duplicate `BackoffState.reset()`'s existing 3 call sites with a second, parallel reset call |
| Attempt counting for the fast-timeout window | New, independent `foregroundConnectAttemptRef`, **not** `terminalBackoffRef.current.attempt` | Dedicated Value/counter object, not repurposed unrelated state (avoids the "no-op getter reused for two meanings" smell) | Reuse `terminalBackoffRef.current.attempt`, read at `connect()`-entry | `connect()` calls `terminalBackoffRef.current.reset()` unconditionally at its own top (`useTerminalStream.ts:166`) on every invocation including automatic retries — `.attempt` is provably always `0` at the point anything downstream could read it (`research/architecture.md` §4, traced step-by-step); gating on it would make "first 2 attempts" never advance past attempt 0 |
| Connect-timeout race mechanism | Plain `setTimeout` that calls `.abort()` on the same, already-existing per-attempt `abortControllerRef` (captured locally as `attemptController`) | Mirrors this file's own existing disconnect-timeout precedent (`useTerminalStream.ts:392-400`, a bare `setTimeout` + `abortControllerRef.current.abort()`) | `Promise.race([streamPromise, timeoutPromise])` | `connect()` has no "connection succeeded" promise to race against — success is signaled by mutating refs/state inside an unawaited async IIFE (`for await` loop), not by resolving a promise `connect()` holds. The one existing `Promise.race` timeout in this codebase (`ConfigPageContent.tsx:205-213`) already demonstrates the failure mode: its `setTimeout` is never captured/cleared after the race settles, leaking a timer (`research/build-vs-buy.md` §3) |
| Connect-timeout race mechanism (cont'd) | — | — | A second, independent `AbortController` combined via `AbortSignal.any([...])` | Adds a signal-composition layer for no behavioral benefit — both timeout-driven abort and disconnect-driven abort want the identical effect (kill this attempt's stream); `streamTerminal(...)` accepts only one `signal` field (`research/architecture.md` §1) |
| Foreground/background flip mid-attempt | **Snapshot-at-schedule-time**: `connectTimeoutMs` is computed once, when the timer is scheduled inside `connect()`, using whatever `foreground` was true then. A mid-flight change to `foreground` does not alter an already-scheduled timer's duration or its consequence. | Simplest correct option; matches AC2's literal phrasing ("first 2 attempts... use..." describes the attempt, not a continuously reevaluated live state) | Live-read `foregroundRef.current` inside the timer callback at fire-time, and conditionally skip/soften the abort if no longer foreground | Adds complexity (the fired timer's *consequence*, not just its scheduled *duration*, becomes conditional on current state) for marginal, likely-imperceptible UX benefit — the user has already switched away (`research/pitfalls.md` §7, design fork explicitly flagged there and resolved here) |
| `foregroundConnectAttemptRef` reset scope | Reset **only** on the `foreground` false→true transition (per AC3's literal text); **not** additionally reset on a successful connect (first message received) while still foreground | Minimal-scope interpretation of AC3 | Also reset the counter to 0 on every successful `firstMessage` while foreground, so a terminal that used 1 fast attempt during a flaky startup gets a fresh fast-attempt budget on a later disconnect | `research/architecture.md` §4 flagged this as a genuinely open question the AC text doesn't resolve and herdr-web's source isn't inspectable to settle by precedent; resetting on success would silently make "fast" attempts effectively unlimited across a long foreground session, which is a larger behavior change than what AC3 asks for. Revisit if telemetry later shows this matters. |
| Feature's reconnect-path scope | Implement exclusively inside the `NEXT_PUBLIC_RECONNECT_V2`-gated hook-level path in `useTerminalStream.ts` | Matches this file's own existing convention: every other piece of hook-level auto-reconnect logic (`terminalBackoffRef`, the `finally`-block retry scheduler, the visibilitychange listener) is already gated the same way | Also implement an equivalent fast/normal timeout inside `TerminalOutput.tsx`'s pre-flag legacy path (~lines 736-778, 990-992) | That path has **no automatic retry loop at all** to attach a timeout to — a dropped connection there only starts a flat 5000ms timer that reveals a manual "Reconnect" button (`TerminalOutput.tsx:764-770`); building an equivalent mechanism there would mean inventing a second, parallel auto-reconnect system, well outside this feature's scope (AC5 explicitly requires stating this rather than silently only covering V2) |
| Wiring `foreground` from the UI | Extend `TerminalOutput.tsx`'s existing `useTerminalStream({...})` call with `foreground: isVisible` | Reuse existing signal rather than invent a parallel one — DRY / single source of truth | Thread a brand-new `foreground` prop from `SessionDetailView.tsx` down through `TerminalOutput.tsx`, in parallel with `isVisible` | `SessionDetailView.tsx` already computes and passes `isVisible={poolId === session.id}` (and the mux/shell-PTY equivalents) at all 3 `TerminalOutput` call sites — semantically identical to "is this session currently selected" (`research/architecture.md` §2). A second prop would need to be kept in sync with the first for no benefit. |

---

## Observability Plan
- **Logs**: connect-timeout abandonment logs a `console.warn` line matching this file's existing `[reconnect] ...` convention (e.g. `console.warn(`[reconnect] stream=terminal trigger=connect-timeout foreground=${foregroundRef.current} attempt=${foregroundConnectAttemptRef.current} timeoutMs=${connectTimeoutMs}`)`), consistent with the existing `[reconnect] stream=terminal trigger=close attempt=... delay=...` (`useTerminalStream.ts:340`) and `[reconnect] stream=terminal trigger=visibility delay=0ms` (`:443`) lines — same prefix convention, new `trigger=connect-timeout` value, diagnosable the same way in production logs.
- **Metrics**: none added. This app has no existing client-side metrics emission point for reconnect events (confirmed: only `console.*` logging exists in this subsystem) — out of scope to add one for this change.
- **Alerts**: none — client-side hook behavior, no alerting surface exists or is warranted for a UX latency tweak.

## Risk Control
- **Feature flag**: none new. The entire mechanism only activates inside the `if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true" ...)` branches this file already gates its other auto-reconnect logic behind (`useTerminalStream.ts:331`, `:434`) — `NEXT_PUBLIC_RECONNECT_V2` defaults off (`web-app/.env.local.example:1`), so this feature ships dark by default, same as the rest of the V2 reconnect subsystem it extends.
- **Rollback procedure**: revert the 3 source-file commits (`backoff.ts`, `useTerminalStream.ts`, `TerminalOutput.tsx`); no data/schema/config migration involved. Because the feature is inert with the flag off, an emergency rollback without a code revert is also available: setting/confirming `NEXT_PUBLIC_RECONNECT_V2` unset in the deployed build disables it along with the rest of V2.
- **Staged rollout**: piggybacks on however `NEXT_PUBLIC_RECONNECT_V2` itself is staged/enabled — no independent rollout plan needed for this change specifically.
- **Activation ownership (pre-mortem Failure #4, P2)**: this repo currently has one active user/maintainer (`tstapler`, also the original issue reporter). Shipping this PR with `NEXT_PUBLIC_RECONNECT_V2` still off is acceptable only if `tstapler` personally flips it on in their own dev/deployed instance within the same work session as merging, to dogfood it — otherwise file a one-line follow-up backlog item ("enable NEXT_PUBLIC_RECONNECT_V2 and confirm foreground-fast-reconnect feels snappier") so the flag-flip doesn't silently fall off the radar the way pre-mortem Failure #4 describes.
- **`FOREGROUND_CONNECT_TIMEOUT_MS=1200` is an unvalidated starting guess, not a measured value** (pre-mortem Failure #2, P1): herdr-web's real constant isn't inspectable (different, unvendored repo — `research/build-vs-buy.md` §4), so `1200` was re-derived from AC2's own "~1200-1500ms" range, not from measured connect-to-first-message latency. On higher-RTT links (VPN, corporate proxy, remote dev) a healthy connection could regularly exceed 1200ms, causing foreground attempts 1-2 to be aborted and retried needlessly — more churn for the terminals a user is actively watching than the pre-feature 3500ms baseline, and invisible in production since the Observability Plan adds no metrics (only an unmonitored `console.warn`). **Before enabling `NEXT_PUBLIC_RECONNECT_V2` broadly**: validate `1200`ms against real p95/p99 connect-to-first-message latency (e.g. temporarily counting `trigger=connect-timeout foreground=true` log lines, or a manual test over a VPN/high-RTT link), and note in the PR description that `1200`ms is a starting value subject to tuning. File a follow-up backlog item to add a lightweight success/failure counter for connect-timeout-triggered aborts if dogfooding surfaces flappiness.

## Unresolved Questions
None. (The one open design fork identified in research — whether `foregroundConnectAttemptRef` should also reset on a successful connect — is resolved as a documented Pattern Decision above, not left blocking.)

## Dependency Visualization

```
Phase 1: Backoff Policy (backoff.ts)
  Epic 1.1 (constants + connectTimeoutMs) ──> Epic 1.2 (unit tests)
        │
        ▼
Phase 2: Hook Wiring (useTerminalStream.ts) — depends on Phase 1's exported connectTimeoutMs
  Epic 2.1 (foreground option) ──> Epic 2.2 (foreground refs + transition effect)
        │                                  │
        ▼                                  ▼
  Epic 2.3 (connect-timeout race in connect(), uses connectTimeoutMs + foregroundConnectAttemptRef)
        │
        ▼
  Epic 2.4 (timer cleanup at disconnect/unmount sites)
        │
        ▼
Phase 3: UI Wiring (TerminalOutput.tsx) — depends on Phase 2's `foreground` option existing
  Epic 3.1 (pass foreground: isVisible)
        │
        ▼
Phase 4: Test Coverage — depends on Phases 1-3 being implemented
  Epic 4.1 (test harness AbortSignal support) ──> Epic 4.2 (new foreground/timeout tests)
        │
        ▼
  Epic 4.3 (regression verification of existing suites)
```

---

## Phase 1: Backoff Timeout Policy

### Epic 1.1: Connect-timeout constants and pure selection function
**Goal**: Add the missing "cap on one connection attempt's duration" concept to `backoff.ts`, as a pure, dependency-free function alongside the existing `jitteredDelay`, satisfying AC1's "must be added, not repurposed."

#### Story 1.1.1: Pure connect-timeout policy function
**As a** developer wiring foreground-aware reconnect behavior, **I want** a single pure function that maps `(foreground, attemptsSinceForeground)` to a timeout duration, **so that** the policy is centralized, unit-testable in isolation from React, and consistent with how `jitteredDelay` already centralizes the delay formula.
**Acceptance Criteria**:
- AC1: A connect-timeout concept (attempt-duration cap) exists in `backoff.ts` and is genuinely new, not a repurposing of `jitteredDelay`/`BackoffState`.
  - *Given* `backoff.ts` before this change has no timeout-duration concept, *When* `connectTimeoutMs` is added, *Then* it is a standalone exported function with no shared implementation with `jitteredDelay` (different signature, different purpose: duration cap vs. delay-before-next-attempt).
- AC2: Fast timeout for first 2 foreground attempts, normal timeout otherwise.
  - *Given* `foreground=true, attemptsSinceForeground=0`, *When* `connectTimeoutMs(true, 0)` is called, *Then* it returns `1200` (`FOREGROUND_CONNECT_TIMEOUT_MS`).
  - *Given* `foreground=true, attemptsSinceForeground=2`, *When* `connectTimeoutMs(true, 2)` is called, *Then* it returns `3500` (`CONNECT_TIMEOUT_MS`), since `2 >= FOREGROUND_FAST_ATTEMPTS`.
  - *Given* `foreground=false`, *When* `connectTimeoutMs(false, 0)` is called, *Then* it returns `3500` regardless of `attemptsSinceForeground`.
**Files**: `web-app/src/lib/utils/backoff.ts`

##### Task 1.1.1a: Add connect-timeout constants (~2 min)
- In `web-app/src/lib/utils/backoff.ts`, after the existing `NON_RETRIABLE_WS_CODES`/close-code helpers section, add:
  ```ts
  // ---------------------------------------------------------------------------
  // Connect-timeout policy (foreground vs. background)
  // ---------------------------------------------------------------------------

  /** Fast connect-timeout for the first FOREGROUND_FAST_ATTEMPTS attempts since a terminal became foreground. */
  export const FOREGROUND_CONNECT_TIMEOUT_MS = 1200;

  /** Normal connect-timeout: background attempts, and foreground attempts beyond FOREGROUND_FAST_ATTEMPTS. */
  export const CONNECT_TIMEOUT_MS = 3500;

  /** Number of connect attempts (since the most recent foreground transition) eligible for the fast timeout. */
  export const FOREGROUND_FAST_ATTEMPTS = 2;
  ```
- Files: `web-app/src/lib/utils/backoff.ts`

##### Task 1.1.1b: Add `connectTimeoutMs` pure function (~3 min)
- Immediately below the constants from 1.1.1a, add:
  ```ts
  /**
   * Returns the connect-timeout (ms) for a reconnect attempt: the maximum time
   * to wait for the first stream message before abandoning the attempt.
   * Foreground terminals get a shorter timeout for their first
   * FOREGROUND_FAST_ATTEMPTS attempts since becoming foreground; all other
   * attempts (background, or foreground beyond the fast window) use the
   * normal timeout.
   */
  export function connectTimeoutMs(foreground: boolean, attemptsSinceForeground: number): number {
    if (foreground && attemptsSinceForeground < FOREGROUND_FAST_ATTEMPTS) {
      return FOREGROUND_CONNECT_TIMEOUT_MS;
    }
    return CONNECT_TIMEOUT_MS;
  }
  ```
- Files: `web-app/src/lib/utils/backoff.ts`

### Epic 1.2: Unit tests for `connectTimeoutMs`
**Goal**: Lock in the fast/normal selection behavior at the pure-function level, independent of the hook, per AC6's "fast timeout used on first 2 foreground attempts, normal timeout used after 2 attempts or when not foreground."

#### Story 1.2.1: `connectTimeoutMs` test coverage
**As a** reviewer, **I want** `connectTimeoutMs` covered by direct unit tests, **so that** the timeout-selection policy is verifiable without mounting `useTerminalStream` or Jest fake timers.
**Acceptance Criteria**:
- AC2/AC6 (pure-function slice): all 3 branches (fast, exhausted-fast-window, non-foreground) are asserted directly.
**Files**: `web-app/src/lib/utils/backoff.test.ts`

##### Task 1.2.1a: Add `describe("connectTimeoutMs")` block (~5 min)
- In `web-app/src/lib/utils/backoff.test.ts`, import `connectTimeoutMs`, `FOREGROUND_CONNECT_TIMEOUT_MS`, `CONNECT_TIMEOUT_MS`, `FOREGROUND_FAST_ATTEMPTS` from `@/lib/utils/backoff` and add:
  ```ts
  describe("connectTimeoutMs", () => {
    it("connectTimeoutMs_should_returnForegroundTimeout_When_foregroundTrueAndAttemptZero", () => {
      expect(connectTimeoutMs(true, 0)).toBe(FOREGROUND_CONNECT_TIMEOUT_MS);
    });
    it("connectTimeoutMs_should_returnForegroundTimeout_When_foregroundTrueAndAttemptOne", () => {
      expect(connectTimeoutMs(true, 1)).toBe(FOREGROUND_CONNECT_TIMEOUT_MS);
    });
    it("connectTimeoutMs_should_returnNormalTimeout_When_foregroundTrueAndAttemptEqualsFastAttempts", () => {
      expect(connectTimeoutMs(true, FOREGROUND_FAST_ATTEMPTS)).toBe(CONNECT_TIMEOUT_MS);
    });
    it("connectTimeoutMs_should_returnNormalTimeout_When_foregroundFalseRegardlessOfAttempt", () => {
      expect(connectTimeoutMs(false, 0)).toBe(CONNECT_TIMEOUT_MS);
      expect(connectTimeoutMs(false, 5)).toBe(CONNECT_TIMEOUT_MS);
    });
    it("connectTimeoutMs_should_beStandaloneFunction_When_comparedToJitteredDelay", () => {
      // AC1: locks in that connectTimeoutMs is genuinely new, not a repurposing
      // of jitteredDelay/BackoffState — different signature, different purpose.
      expect(connectTimeoutMs).not.toBe(jitteredDelay);
      expect(connectTimeoutMs.length).toBe(2); // (foreground, attemptsSinceForeground)
    });
  });
  ```
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="backoff.test"` to verify.
- Files: `web-app/src/lib/utils/backoff.test.ts`

---

## Phase 2: Hook-Level `foreground` and Connect-Timeout Wiring

### Epic 2.1: `foreground` option plumbing
**Goal**: Add the `foreground?: boolean` option to `useTerminalStream` without changing behavior for existing callers that omit it (AC0).

#### Story 2.1.1: `foreground` option on `UseTerminalStreamOptions`
**As a** caller of `useTerminalStream`, **I want** an optional `foreground` flag, **so that** I can indicate this terminal is the one currently selected/visible without being forced to pass it.
**Acceptance Criteria**:
- AC0: default `false`, no compile or runtime behavior change for existing callers that omit it.
  - *Given* an existing caller (e.g. any test using `RECONNECT_OPTIONS` without `foreground`) invokes `useTerminalStream({...})` without `foreground`, *When* `connect()` runs and the mocked stream delivers its first message synchronously (as every existing test already does, before any `jest.advanceTimersByTime` call), *Then* `isConnected` becomes `true` exactly as before this change — `foregroundRef.current` defaults to `false`, so `connectTimeoutMs(false, 0)` evaluates to `3500`ms, a duration no existing test's fake-timer advance is exposed to.
  - **Clarifying note on AC0 vs. AC1**: "no behavior change" is scoped to *observable* behavior under working connections (existing callers still connect/reconnect identically). AC1 separately *requires* a connect-timeout to newly exist for every attempt, foreground or not — this is an intentional new safety net (previously: an unresponsive server hung the hook in `CONNECTING` forever), not a regression, and it is invisible to any currently-passing test because none of them stall past 3500ms of simulated time before delivering a message.
**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.1.1a: Add `foreground?: boolean` to the options interface (~2 min)
- In `web-app/src/lib/hooks/useTerminalStream.ts`, inside `interface UseTerminalStreamOptions` (around line 36-52), add after `isExternal?: boolean;`:
  ```ts
  /** True when this terminal is the one currently selected/visible to the user (drives fast connect-timeout). */
  foreground?: boolean;
  ```
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.1.1b: Destructure `foreground` with default `false` (~2 min)
- In the `useTerminalStream({ ... })` function signature (around line 76-89), add `foreground = false,` to the destructured parameters (alongside `initialCols`, `initialRows`).
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

### Epic 2.2: `foreground` transition state (refs + effect)
**Goal**: Track `foreground` in a ref (avoiding stale closures in `connect()`) and detect the false→true transition to reset both the existing backoff counter and the new fast-attempt counter (AC3).

#### Story 2.2.1: `foregroundRef` + `foregroundConnectAttemptRef` + transition effect
**As a** the hook itself, **I want** the false→true transition of `foreground` to reset both `terminalBackoffRef` and a new `foregroundConnectAttemptRef`, **so that** a session just switched to gets the full fast-timeout window immediately rather than inheriting exhausted state from before it was selected.
**Acceptance Criteria**:
- AC3: false→true transition resets the attempt counter.
  - *Given* `foregroundRef.current` is `false` and `foregroundConnectAttemptRef.current` is `2` (fast window previously exhausted), *When* the `foreground` prop transitions `false → true` and the new standalone `useEffect([foreground])` runs, *Then* `terminalBackoffRef.current.reset()` fires and `foregroundConnectAttemptRef.current` is set to `0`, so the next `connect()` call computes `connectTimeoutMs(true, 0)` = `1200`ms again.
- AC3 (pre-mortem Failure #1, P1): stale pending backoff wait does not survive the transition.
  - *Given* a `reconnectTimerRef` timer is pending (scheduled while `foreground` was `false`, e.g. a 20s backoff delay), *When* `foreground` transitions `false → true`, *Then* that timer is cleared and `connect()` is invoked immediately (subject to the `shouldReconnectRef`/not-already-connecting/not-already-connected guards) rather than being left to fire on its original, stale schedule.
**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.2.1a: Add `foregroundRef` and `foregroundConnectAttemptRef` (~2 min)
- Near the other refs (around line 108-111, alongside `terminalBackoffRef`), add:
  ```ts
  const foregroundRef = useRef(foreground);
  const foregroundConnectAttemptRef = useRef(0);
  ```
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.2.1b: Add the foreground-transition effect (~8 min)
- **Revised per pre-mortem Failure #1 (P1)**: resetting the counters alone is not enough. If this
  terminal is currently sitting out a long, already-scheduled `reconnectTimerRef` backoff delay
  (computed while it was backgrounded — up to `BackoffState`'s 30s cap), that pending wait
  survives the transition untouched, so the user selects a stalled terminal and it still sits
  idle for up to 30s before the fast 1200ms connect-timeout ever gets a chance to run — the exact
  scenario this feature exists to fix. The transition effect must also clear that pending timer
  and trigger an immediate reconnect, not just reset the counters for whatever the *next* attempt
  would have been.
- As a new, standalone `useEffect` (do **not** merge into the mount effect at line 416-430 or the visibilitychange effect at 432-458 — this file's convention is one narrowly-scoped effect per concern), placed after the existing ref-sync effect (~line 126-128):
  ```ts
  // Detect the foreground false→true transition (AC3): reset both the
  // existing backoff-delay counter and the new fast-connect-timeout
  // attempt counter so a just-selected terminal gets the full fast
  // window immediately, not whatever was left over from before it was
  // foreground (or from background attempts that happened while it wasn't selected).
  //
  // Also clear any pending reconnectTimerRef and reconnect immediately (pre-mortem
  // Failure #1, P1): without this, a terminal that was mid-backoff-delay while
  // backgrounded would still sit out that stale, potentially-30s wait after being
  // selected, before the fast connect-timeout ever got a chance to apply.
  useEffect(() => {
    const wasForeground = foregroundRef.current;
    foregroundRef.current = foreground;
    if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return; // same gate as the rest of this file's auto-reconnect logic (consistency, triad Engineering-lens round 2)
    if (!wasForeground && foreground) {
      terminalBackoffRef.current.reset();
      foregroundConnectAttemptRef.current = 0;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
        if (shouldReconnectRef.current && !isConnectingRef.current && !isConnectedRef.current) {
          connectRef.current?.();
        }
      }
    }
  }, [foreground]);
  ```
- This mirrors the existing visibilitychange effect's own precedent (`useTerminalStream.ts:433-458`, which already clears backoff state and calls `connect()` immediately on `visibilitychange`/`online`) — the foreground transition is the in-app equivalent of that same "the user is looking at this now, don't make them wait out a stale delay" signal, just scoped to session selection instead of browser-tab visibility. Guard on `shouldReconnectRef`/`isConnectingRef`/`isConnectedRef` the same way that effect does, to avoid double-connecting an attempt already in flight or a terminal that's already connected (e.g. `reconnectTimerRef` was null because there was nothing pending).
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

### Epic 2.3: Connect-timeout race integrated into `connect()`
**Goal**: Actually schedule and act on the connect-timeout inside `connect()`, reusing the existing `abortControllerRef`/`firstMessage` machinery so timeout-triggered abandonment falls into the existing retry pipeline with zero new retry-scheduling code (AC1, AC2, AC5).

#### Story 2.3.1: Schedule and act on the connect-timeout
**As a** the hook, **I want** each connect attempt to be bounded by `connectTimeoutMs(foreground, attemptsSinceForeground)`, **so that** a hanging handshake is abandoned and retried instead of leaving the hook stuck in `CONNECTING` forever.
**Acceptance Criteria**:
- AC1: hanging attempt gets abandoned via abort, existing retry path takes over.
  - *Given* `connect()` has started an attempt (`abortControllerRef.current` freshly set, `attemptController` locally captured), *When* `firstMessage` is still `true` when the scheduled timer fires, *Then* `attemptController.abort()` is called, the `for await` loop throws, and the existing `catch`/`finally` block (lines 313-353) schedules the next attempt via `terminalBackoffRef.current.next()` — no new retry-scheduling code is added.
- AC2: fast timeout for first 2 foreground attempts.
  - *Given* `foregroundRef.current === true` and `foregroundConnectAttemptRef.current === 0`, *When* `connect()` computes its timeout via `connectTimeoutMs(foregroundRef.current, foregroundConnectAttemptRef.current)`, *Then* the scheduled timer fires after `1200`ms if no message has arrived by then.
- AC5: scoped to the `NEXT_PUBLIC_RECONNECT_V2` path.
  - *Given* `process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true"`, *When* `connect()` runs, *Then* the connect-timeout `setTimeout` is not scheduled at all (guarded the same way the existing retry-scheduling and visibilitychange logic already are), since there is no automatic-reconnect loop in that mode for a timeout-triggered abort to feed into.
- Race guard (pre-mortem Failure #3, P2): a first message that lands in the same tick the timer fires must win, not the abort.
  - *Given* the timer callback fires, *When* it runs, *Then* it first re-checks the same `firstMessage` flag the success path checks — if `firstMessage` is already `false` (a message already landed, even if this callback was scheduled microtasks before `clearTimeout` ran), the callback is a no-op and does **not** call `attemptController.abort()`.
**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.3.1a: Add `connectTimeoutRef` and lift `firstMessage` to a ref (~4 min)
- Near the other timer refs (around line 110-111, alongside `reconnectTimerRef`), add:
  ```ts
  const connectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  ```
- **Correction (triad Engineering-lens round 2, blocker)**: the existing `let firstMessage = true;` (line 219) is declared *inside* the nested async IIFE that processes stream messages — a different function scope than `connect()`'s own top level, where the connect-timeout `setTimeout` callback (Task 2.3.1b) is scheduled. A callback in that outer scope cannot read a `let` declared inside the inner IIFE; as originally drafted this plan's race-guard callback would not compile. Fix: add a ref instead, alongside `connectTimeoutRef`:
  ```ts
  const firstMessageRef = useRef(true);
  ```
  Then in the nested IIFE, replace `let firstMessage = true;` with resetting the ref at the same point (`firstMessageRef.current = true;`), and replace every subsequent read/write of the local `firstMessage` variable (the `if (firstMessage) { ... firstMessage = false; }` block at lines 221-227) with `firstMessageRef.current` — same logic, ref-backed so it's readable from `connect()`'s outer scope by the timeout callback in Task 2.3.1b.
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.3.1b: Compute timeout, increment attempt counter, schedule the timer (~5 min)
- In `connect()`, immediately after `abortControllerRef.current = new AbortController();` (line 184), add:
  ```ts
  const attemptController = abortControllerRef.current;
  const timeoutMs = connectTimeoutMs(foregroundRef.current, foregroundConnectAttemptRef.current);
  foregroundConnectAttemptRef.current += 1;
  const attemptNumber = foregroundConnectAttemptRef.current; // logged value, captured before the increment below (fixes an off-by-one in the log line)
  if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true") {
    connectTimeoutRef.current = setTimeout(() => {
      connectTimeoutRef.current = null;
      // Race guard (pre-mortem Failure #3, P2) — best-effort, not a perfect fix:
      // re-check firstMessageRef immediately before aborting, so a message that
      // already landed and was processed (firstMessageRef.current already false)
      // is not retroactively aborted. This closes the case where the success
      // path's callback ran first in the event-loop's task ordering. It does NOT
      // — and cannot, in a single-threaded event loop with two independently
      // scheduled callbacks — guarantee which of the two wins when both become
      // eligible to run at effectively the same instant; whichever the JS engine
      // dequeues first decides the outcome, same as any timeout-based
      // cancellation (e.g. `AbortSignal.timeout()`, `fetch` timeout patterns).
      // Accepted as adequate: an occasional abort-of-an-attempt-that-was-about-
      // to-succeed just falls into the existing, already-tested retry path.
      if (!firstMessageRef.current) return;
      console.warn(`[reconnect] stream=terminal trigger=connect-timeout foreground=${foregroundRef.current} attempt=${attemptNumber} timeoutMs=${timeoutMs}`);
      attemptController.abort();
    }, timeoutMs);
  }
  ```
- Import `connectTimeoutMs` from `@/lib/utils/backoff` in this file's existing `backoff` import (line 10): `import { BackoffState, connectTimeoutMs, getWsCloseCode, isRetriableCloseCode } from "@/lib/utils/backoff";`.
- **Note**: `foregroundConnectAttemptRef.current += 1` runs unconditionally (even when the flag is off) — this is intentionally cheap and inert when V2 is off, matching `research/architecture.md` §5's recommendation, since no reconnect is ever scheduled to consume it in that mode.
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.3.1e: Test — a message processed before the timer fires is not retroactively aborted (~4 min)
- **Added per triad Engineering-lens review (pre-mortem Failure #3, P2)**. Add a regression test in `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`: push the first message and let it be processed (`firstMessageRef.current` becomes `false`) *before* advancing fake timers to the connect-timeout duration, then assert `capturedSignal?.aborted` stays `false` and `isConnected` becomes `true`.
- **Scope note (triad UX-lens round 2)**: this test proves the guard's actual mechanism — a processed message prevents a *later-firing* timer from aborting — not "whichever of the two events happens closer to real-world-simultaneously always wins." That stronger claim isn't testable or guaranteeable in a single-threaded event loop (see the code comment in Task 2.3.1b); don't write a test that asserts it, and don't claim pre-mortem Failure #3 is "fully closed" — it's mitigated to the same standard as any other timeout-based abort pattern in this codebase or elsewhere.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 2.3.1c: Clear the timer on first-message success (~3 min)
- Inside the `if (firstMessage) { ... }` block (lines 221-227), add the clear **before** `firstMessage = false`:
  ```ts
  if (firstMessage) {
    if (connectTimeoutRef.current) {
      clearTimeout(connectTimeoutRef.current);
      connectTimeoutRef.current = null;
    }
    isConnectingRef.current = false;
    setIsConnected(true);
    setScrollbackLoaded(true);
    setTerminalState('LOADING');
    firstMessage = false;
  }
  ```
- This is the primary clear site — it stops the timer before an otherwise-healthy connection could ever be spuriously aborted later in its life (clearing only in `finally` would leave it live for the stream's entire remaining lifetime).
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.3.1d: Clear the timer defensively in the outer `finally` block (~2 min)
- In the outer `finally` block (around line 322-330, alongside the existing `textDecoderRef`/`scrollbackDecoderRef` resets), add:
  ```ts
  if (connectTimeoutRef.current) {
    clearTimeout(connectTimeoutRef.current);
    connectTimeoutRef.current = null;
  }
  ```
- Covers the case where the stream errors/ends before `firstMessage` ever fires (e.g. immediate close, non-retriable error), so no dangling timer outlives the attempt.
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

### Epic 2.4: Timer cleanup at `disconnect()` / unmount sites
**Goal**: Add `connectTimeoutRef` to the same clear-timer sites this file already maintains for `reconnectTimerRef`, so a pending connect-timeout never fires against a torn-down attempt after a manual disconnect or component unmount (`research/pitfalls.md` §3).

#### Story 2.4.1: Clear `connectTimeoutRef` on `disconnect()` and unmount
**As a** the hook, **I want** `connectTimeoutRef` cleared wherever `reconnectTimerRef` already is, **so that** a leaked timer can never abort a superseded or already-torn-down attempt.
**Acceptance Criteria**:
- Correctness requirement underlying AC1/AC7 (no test-observable leak, no regression): a pending connect-timeout must not fire after `disconnect()` or unmount.
  - *Given* a connect-timeout timer is pending (`connectTimeoutRef.current` set) and the caller invokes `disconnect()`, *When* `disconnect()` runs, *Then* `connectTimeoutRef.current` is cleared via `clearTimeout` and set to `null`, identically to how `reconnectTimerRef` is already cleared at `useTerminalStream.ts:373-376`.
  - *Given* the same pending timer, *When* the component unmounts (the mount effect's cleanup at `useTerminalStream.ts:420-428` runs), *Then* `connectTimeoutRef.current` is cleared the same way `reconnectTimerRef.current` already is at lines 422-425.
**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.4.1a: Clear `connectTimeoutRef` in `disconnect()` (~2 min)
- In `disconnect()` (around line 372-376), alongside the existing `reconnectTimerRef` clear, add:
  ```ts
  if (connectTimeoutRef.current) {
    clearTimeout(connectTimeoutRef.current);
    connectTimeoutRef.current = null;
  }
  ```
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.4.1b: Clear `connectTimeoutRef` in the unmount cleanup effect (~2 min)
- In the mount/cleanup effect's cleanup function (around line 420-428), alongside the existing `reconnectTimerRef` clear, add:
  ```ts
  if (connectTimeoutRef.current) {
    clearTimeout(connectTimeoutRef.current);
    connectTimeoutRef.current = null;
  }
  ```
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 2.4.1c: Test — `disconnect()` before first message clears the pending connect-timeout (~4 min)
- **Added per triad Engineering-lens round 2 review** — `validation.md`'s AC7 row commits to `useTerminalStream_should_notLeakConnectTimeout_When_disconnectCalledBeforeFirstMessage` with no corresponding task. Add to `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`: start a connect attempt (no message pushed), call `disconnect()` before advancing any timers, then advance fake timers past `FOREGROUND_CONNECT_TIMEOUT_MS`/`CONNECT_TIMEOUT_MS` and assert `attemptController.abort()`/the `console.warn` connect-timeout log never fires — proving Task 2.4.1a's clear actually prevents the leaked timer from acting on a torn-down attempt.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

---

## Phase 3: UI Wiring

### Epic 3.1: `TerminalOutput.tsx` → `useTerminalStream`
**Goal**: Thread the existing `isVisible` prop one hop further as `foreground`, satisfying AC4 with a one-line change and no new prop threaded through `SessionDetailView.tsx` or `XtermTerminal.tsx`.

#### Story 3.1.1: Pass `foreground: isVisible` into `useTerminalStream`
**As a** `SessionDetailView` rendering a pooled terminal, **I want** the terminal's `foreground` state to reflect whether it's the one currently selected, **so that** the fast connect-timeout applies to the terminal the user is actually looking at.
**Acceptance Criteria**:
- AC4: `foreground={isSelected}`-equivalent wiring reaches the hook.
  - *Given* `SessionDetailView.tsx`'s main-terminal call site already computes `isVisible={poolId === session.id}` (`SessionDetailView.tsx:702`, unchanged by this feature), *When* that `TerminalOutput` instance renders and calls `useTerminalStream({..., foreground: isVisible})`, *Then* the hook's `foreground` option is `true` exactly when the pooled session is the one currently selected/displayed — identically for the mux-pool (`isVisible={poolPath === session.externalMetadata?.muxSocketPath}`) and shell-PTY-pool (`isVisible={activeTabId === shellKey}`) call sites, with no changes needed at those call sites.
**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 3.1.1a: Add `foreground: isVisible` to the `useTerminalStream({...})` call (~2 min)
- In `web-app/src/components/sessions/TerminalOutput.tsx`, in the existing `useTerminalStream({...})` call (line 456-470), add `foreground: isVisible,` alongside the existing `isExternal: isExternal,` line.
- No other change to this file — `isVisible`'s existing fit+focus effect (lines 949-956) is untouched, kept as a separate concern from `foreground`'s reconnect-timing semantics per `research/architecture.md` §2's note (same input value, deliberately separate names for two different purposes).
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

### Epic 3.2: Wiring regression tests
**Goal**: Close a gap flagged by triad Engineering-lens review — `validation.md` commits to `TerminalOutput_should_passForegroundTrue/False_When_isVisiblePropTrue/False`.
**Correction (triad Engineering-lens round 2, blocker)**: `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` is **not** a new file — it already exists on `main` (363 lines, `describe("TerminalOutput reconnect banner", ...)`, Stories 3.2.1/3.2.2 from a prior feature) with its own established `useTerminalStream` mock (`jest.mock("@/lib/hooks/useTerminalStream", ...)`, `makeStreamMock()` helper, line 39/139). This task **extends** that existing describe block/mock helper — it must not create/overwrite the file, which would destroy the existing 10-test suite.

#### Story 3.2.1: `foreground` wiring is unit-tested at the component boundary
**As a** reviewer, **I want** a test proving `TerminalOutput` passes `foreground: isVisible` into `useTerminalStream`, **so that** AC4 has direct test coverage rather than only being implied by the one-line diff.
**Acceptance Criteria**:
- AC4 (test coverage): matches validation.md's `Requirement → Test Mapping` row for AC4.
**Files**: `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` (existing file, extend only)

##### Task 3.2.1a: Extend `TerminalOutput.reconnect.test.tsx` with 2 new tests (~8 min)
- In the existing file, reuse the existing `makeStreamMock()`/`(useTerminalStream as jest.Mock).mockReturnValue(...)` mocking convention already used throughout (see line 139, 170, 198). Add, either inside the existing `describe("TerminalOutput reconnect banner", ...)` block or a new sibling `describe("TerminalOutput foreground wiring", ...)`:
  - `TerminalOutput_should_passForegroundTrue_When_isVisiblePropTrue` — render with `isVisible={true}`, assert `(useTerminalStream as jest.Mock)` was called with an options object containing `foreground: true`.
  - `TerminalOutput_should_passForegroundFalse_When_isVisiblePropFalse` — render with `isVisible={false}`, assert `foreground: false`.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="TerminalOutput.reconnect"` to verify both the 2 new tests and the existing 10 pass unmodified.
- Files: `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx`

---

## Phase 4: Test Coverage — Hook Level

### Epic 4.1: Test harness `AbortSignal` support
**Goal**: The existing `makePushStream` mock has no `AbortSignal` awareness (`research/pitfalls.md` §6) — extend it minimally so tests can simulate a real aborted stream, rather than hanging the `for await` loop forever when fake timers advance past a connect-timeout.

#### Story 4.1.1: `makePushStream` observes an `AbortSignal`
**As a** test author, **I want** `makePushStream` to reject its pending `next()` when the stream's `AbortSignal` fires, **so that** tests can advance fake timers past `FOREGROUND_CONNECT_TIMEOUT_MS`/`CONNECT_TIMEOUT_MS` and observe the resulting abort/retry without the mock hanging indefinitely.
**Acceptance Criteria**:
- Test-infra prerequisite for AC6 (not itself an AC, but required to write AC6's tests correctly).
  - *Given* `mockStreamTerminal.mockImplementation((_, opts) => makePushStream(opts?.signal).iterable)` is used in a test, *When* the captured `AbortSignal` fires (`attemptController.abort()` is called by the connect-timeout), *Then* the `PushStream`'s pending `next()` promise rejects with an `Error` (mirroring a real aborted ConnectRPC stream), unblocking the `for await` loop into the existing `catch`/`finally` path.
**Files**: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.1.1a: Extend `makePushStream` to accept and observe a signal (~5 min)
- In `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`, change `makePushStream<T>()` to `makePushStream<T>(signal?: AbortSignal)`, and inside the iterator's `next()`, add an abort listener that rejects the currently-pending resolver:
  ```ts
  function makePushStream<T>(signal?: AbortSignal): PushStream<T> {
    const queue: T[] = [];
    const resolvers: Array<{ resolve: () => void; reject: (err: Error) => void }> = [];
    let done = false;
    let aborted = false;

    signal?.addEventListener('abort', () => {
      aborted = true;
      const pending = resolvers.splice(0);
      pending.forEach((r) => r.reject(new Error('aborted')));
    });

    const push = (msg: T) => {
      queue.push(msg);
      resolvers.shift()?.resolve();
    };
    const end = () => {
      done = true;
      resolvers.shift()?.resolve();
    };

    const iterable: AsyncIterable<T> = {
      [Symbol.asyncIterator]() {
        return {
          async next(): Promise<IteratorResult<T>> {
            if (aborted) throw new Error('aborted');
            while (queue.length === 0 && !done) {
              await new Promise<void>((resolve, reject) => resolvers.push({ resolve, reject }));
            }
            if (queue.length > 0) {
              return { value: queue.shift()!, done: false };
            }
            return { value: undefined as any, done: true };
          },
        };
      },
    };
    return { iterable, push, end };
  }
  ```
- Update `PushStream<T>` interface's `next()`-related type only if needed for TS; existing callers that don't pass `signal` keep working unchanged (optional param).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

### Epic 4.2: New foreground/connect-timeout tests
**Goal**: Directly cover AC2, AC3, and AC6's four listed assertions at the hook level, using the fake-timer conventions already established in the `auto-reconnect` describe block.

#### Story 4.2.1: Foreground connect-timeout behavior
**As a** reviewer, **I want** hook-level tests proving fast vs. normal timeout selection, attempt-counter reset, and timeout-driven retry, **so that** AC2/AC3/AC6 are verifiably met and protected against regression.
**Acceptance Criteria**:
- AC6 (all 4 sub-assertions): fast timeout on first 2 foreground attempts; normal timeout after 2 attempts or when not foreground; attempt-counter reset on false→true transition; connect-timeout abandonment triggers the existing retry path rather than leaving the hook stuck in `CONNECTING`.
  - *Given* `renderHook(() => useTerminalStream({...RECONNECT_OPTIONS, foreground: true}))` with a `PushStream` that never receives a pushed message, *When* the test runs `await act(async () => { jest.advanceTimersByTime(1200); })`, *Then* `mockStreamTerminal`'s captured `AbortSignal` fires, the stream's `next()` rejects, `result.current.terminalState` becomes `'DISCONNECTED'` (not stuck at `'CONNECTING'`), and a reconnect is scheduled via the existing `terminalBackoffRef.current.next()` path (observable via a second `mockStreamTerminal` call after advancing past the scheduled backoff delay too).
  - *Given* the same setup but `foreground: false`, *When* the test advances `jest.advanceTimersByTime(1200)` only, *Then* no abort/timeout has fired yet (stream still pending, `terminalState` still `'CONNECTING'`); *When* the test advances a further `2300`ms (total `3500`ms), *Then* the abort fires.
  - *Given* `foreground: true` and 2 prior failed attempts already consumed (`foregroundConnectAttemptRef.current === 2` after two connect-timeout-driven retries), *When* a 3rd attempt starts, *Then* it uses `3500`ms (not `1200`ms) — advancing only `1200`ms does not abort it.
  - *Given* `foreground` starts `false` with `foregroundConnectAttemptRef.current` already at `2` (from a prior background sequence), *When* the hook's `foreground` prop is changed to `true` via `rerender({...RECONNECT_OPTIONS, foreground: true})` and a subsequent `connect()` attempt starts, *Then* that attempt uses `1200`ms again (counter was reset by the transition effect).
**Files**: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1a: Test — fast timeout aborts at 1200ms on first foreground attempt (~5 min)
- Add to the `useTerminalStream — auto-reconnect (NEXT_PUBLIC_RECONNECT_V2)` describe block (or a new sibling describe, `useTerminalStream — foreground connect-timeout`, placed after it):
  ```ts
  it('connect_should_abortAndRetry_When_foregroundTrueAndFirstAttemptExceeds1200ms', async () => {
    let capturedSignal: AbortSignal | undefined;
    let callCount = 0;
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      callCount++;
      capturedSignal = opts?.signal;
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: true })
    );

    await act(async () => { result.current.connect(); });
    expect(capturedSignal?.aborted).toBe(false);

    await act(async () => { jest.advanceTimersByTime(1200); });
    await waitFor(() => expect(result.current.terminalState).toBe('DISCONNECTED'));
    expect(capturedSignal?.aborted).toBe(true);
  });
  ```
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1b: Test — normal timeout used when not foreground (~4 min)
- Add:
  ```ts
  it('connect_should_notAbort_When_notForegroundAndOnly1200msElapsed', async () => {
    let capturedSignal: AbortSignal | undefined;
    mockStreamTerminal.mockImplementation((_msg: unknown, opts?: { signal?: AbortSignal }) => {
      capturedSignal = opts?.signal;
      return makePushStream(opts?.signal).iterable;
    });

    const { result } = renderHook(() =>
      useTerminalStream({ ...RECONNECT_OPTIONS, foreground: false })
    );

    await act(async () => { result.current.connect(); });
    await act(async () => { jest.advanceTimersByTime(1200); });
    expect(capturedSignal?.aborted).toBe(false);

    await act(async () => { jest.advanceTimersByTime(2300); }); // total 3500ms
    await waitFor(() => expect(capturedSignal?.aborted).toBe(true));
  });
  ```
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1c: Test — normal timeout used on 3rd+ foreground attempt (~5 min)
- Add a test that: connects with `foreground: true`, lets 2 connect-timeout-driven aborts happen (advance `1200`ms twice, letting the backoff-delay `setTimeout` fire each time to trigger the next `connect()`), then on the 3rd attempt asserts advancing only `1200`ms does **not** abort but advancing to `3500`ms does. Follow the existing pattern of tracking `streams: ReturnType<typeof makePushStream>[]` per attempt (as in `connect_should_reconnectAfterJitteredDelay_When_streamClosesCleanlyAndShouldReconnectTrue`, lines 472-499).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1d: Test — `foregroundConnectAttemptRef` resets on false→true transition (~5 min)
- Add a test using `renderHook`'s `rerender`: start with `foreground: false`, drive `foregroundConnectAttemptRef` up via 2 failed background attempts (background timeout is 3500ms so this test can instead just assert post-transition behavior directly, per the Given-When-Then above — no need to actually exhaust background attempts first; simplest form: set `foreground: false` initially with no connect attempts made, `rerender` to `foreground: true`, then connect and assert the first attempt still uses `1200`ms, matching "resets to 0" trivially at zero prior attempts). Prefer the stronger form matching the GWT above (drive the counter to 2 first via `foreground: true` then flip to `false` then back to `true`) if achievable within the ~5 min budget; otherwise the simpler form is an acceptable, still-meaningful regression guard — note which form was implemented in the PR description.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1e: Test — connect-timeout abandonment does not leave hook stuck in `CONNECTING` (~3 min)
- This is largely covered by Task 4.2.1a's assertion (`terminalState` reaches `'DISCONNECTED'`, not stuck at `'CONNECTING'`); add one explicit assertion that a subsequent attempt is scheduled and eventually reached `'CONNECTING'` again after the backoff delay elapses (`jest.advanceTimersByTime` past `terminalBackoffRef`'s computed delay), proving the double-connect guard (`isConnectingRef`) was correctly reset by the `finally` block rather than left stuck `true` forever.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1f: Test — pending stale backoff wait does not survive foreground transition (~5 min)
- **Added per pre-mortem Failure #1 (P1)**. Add a test: with `foreground: false`, drive a failed attempt so `reconnectTimerRef` schedules a long backoff delay (e.g. force `terminalBackoffRef` to a multi-second delay by simulating a couple of prior failures, or directly assert against a known delay from `BackoffState`'s formula); then `rerender({...RECONNECT_OPTIONS, foreground: true})`. Assert: (a) the original `reconnectTimerRef` timer is cleared (advancing fake timers to its original fire time does *not* trigger a second/duplicate `connect()` call beyond the immediate one), and (b) `connect()`/`streamTerminal` is invoked immediately (within the same `act()`, before any `jest.advanceTimersByTime`), proving the stale wait was pre-empted rather than left to elapse on its original schedule.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1g: Test — AC0, no behavior change for callers that omit `foreground` (~5 min)
- **Added per triad Engineering-lens round 2/3 review** — `validation.md` commits two AC0 tests with no corresponding plan tasks. Add both:
  - `useTerminalStream_should_connectAsBeforeChange_When_foregroundOmitted`: render with the existing `RECONNECT_OPTIONS` (no `foreground` key), push the first message synchronously as every pre-existing test already does, assert `isConnected` becomes `true` — same shape as this file's pre-existing connect-success tests, just asserting the omitted-`foreground` default path specifically.
  - `useTerminalStream_should_handleStreamErrorIdentically_When_foregroundOmittedAndConnectionFails`: same omitted-`foreground` setup, but the mocked stream throws immediately; assert the existing `catch`/`finally` error-handling path runs unchanged (state reaches `DISCONNECTED`/retry-scheduled as it does today), proving the new connect-timeout machinery doesn't alter error-path behavior when inert-by-default.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1i: Test — AC3, counters untouched on non-false→true transitions (~4 min)
- **Added per triad Engineering-lens round 3 review** — `validation.md` commits `useTerminalStream_should_notResetCounters_When_foregroundDoesNotTransitionFalseToTrue` with no corresponding plan task. Add: rerender with `foreground` staying `true→true` and separately `true→false`, and in neither case assert `terminalBackoffRef.current.reset()` was called or `foregroundConnectAttemptRef.current` was zeroed — only the literal `false→true` edge triggers the reset (Task 2.2.1b).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

##### Task 4.2.1h: Test — AC5, connect-timeout only scheduled when `NEXT_PUBLIC_RECONNECT_V2` is on (~4 min)
- **Added per triad Engineering-lens round 2 review** — `validation.md` commits `connect_should_scheduleConnectTimeout_When_reconnectV2FlagEnabled` and `connect_should_notScheduleConnectTimeout_When_reconnectV2FlagDisabled` with no corresponding plan tasks. Add both: with the flag on, assert a connect-timeout timer is pending after `connect()` (e.g. via `jest.getTimerCount()` delta, or by advancing to just past the timeout and observing the abort); with the flag off (or unset), assert no such timer is ever scheduled (advancing timers well past `CONNECT_TIMEOUT_MS` never triggers an abort).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`

- Run `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalStream.test"` after 4.2.1a-h to verify all new tests pass.

### Epic 4.3: Regression verification
**Goal**: Confirm AC7 — no regression to existing `useTerminalStream`/`useTerminalStream.resync.integration` suites.

#### Story 4.3.1: Full existing suite still passes
**As a** reviewer, **I want** confirmation the existing test suites are unaffected, **so that** AC7 is verifiably satisfied before merge.
**Acceptance Criteria**:
- AC7: no regression.
  - *Given* the existing `useTerminalStream — auto-reconnect (NEXT_PUBLIC_RECONNECT_V2)`, `useTerminalStream — ResizeQuiescence state machine`, `useTerminalStream — scrollback decoder isolation`, and `useTerminalStream.resync.integration` suites (all in `web-app/src/lib/hooks/__tests__/`), *When* `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalStream"` runs after all Phase 1-3 changes, *Then* every previously-passing test still passes, since `connectTimeoutRef` is cleared inside the `firstMessage` block before any of those tests' fake-timer advances reach even the shortest new timeout (1200ms) — each existing test pushes its first message synchronously before calling `jest.advanceTimersByTime`.
**Files**: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`, `web-app/src/lib/hooks/__tests__/useTerminalStream.resync.integration.test.ts` (read-only verification, no edits expected)

##### Task 4.3.1a: Run full existing suite and confirm zero regressions (~3 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalStream"` (covers both the unit and resync-integration suites).
- If any previously-passing test now fails, root-cause against this plan's Epic 2.3/2.4 clear-timer sites (most likely cause: a missing clear site letting a connect-timeout fire against a test's mocked stream) before considering the plan complete.
- Files: none changed by this task — verification only.
