# Research: Pitfalls of adding a connect-timeout race to `useTerminalStream`

Scope: what commonly goes wrong when a "cap on how long one connection attempt may hang"
mechanism is bolted onto the existing reconnect state machine in
`web-app/src/lib/hooks/useTerminalStream.ts` (read in full: 490 lines), informed by the
existing test suite in
`web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` (755 lines) and the pre-flag
path in `web-app/src/components/sessions/TerminalOutput.tsx`.

Note on the requirements doc's `web/src/terminalReconnectPolicy.ts` reference: that file
does not exist anywhere in this repo (`find / -iname "terminalReconnectPolicy*"` — no
hits) or under a local `herdr-web` checkout (none found). It is external prior art cited
for the foreground/background timeout *concept*, not a file to port from directly — the
implementation has to be original to this hook's existing state shape.

## 1. Where "connect" actually resolves today (no timeout concept exists)

`connect()` (`useTerminalStream.ts:162-361`) does not return a promise that resolves on
"connected" — it kicks off `clientRef.current.streamTerminal(...)` and returns almost
immediately after starting an unawaited async IIFE (`useTerminalStream.ts:217-354`) that
consumes the stream with `for await (const msg of stream)`. "Connected" is signalled
asynchronously by the *first message* arriving (`firstMessage` flag,
`useTerminalStream.ts:219-227`: sets `isConnectingRef.current = false`,
`setIsConnected(true)`). There is currently no timer of any kind between "stream call
issued" and "first message arrives" — if the server never answers, the hook hangs in
`CONNECTING` state forever with `isConnectingRef.current = true`, which is exactly the gap
AC1 wants closed. Any connect-timeout design has to hook into this specific window: start a
timer when `connect()` begins, clear it in the `firstMessage` branch, and have it fire an
abort if `sessionId`/`shellId` never see a first message in time.

## 2. Race: timeout fires just as the connection actually succeeds

Because `firstMessage` handling and any new timeout-driven abort both mutate
`isConnectingRef`/`isConnectedRef`/`abortControllerRef`, there is a genuine last-write-wins
race if the timeout timer's callback and the `for await` loop's first iteration are both
scheduled in the same microtask/macrotask window (e.g. timeout fires in a `setTimeout`
callback at the same tick the stream's `next()` promise resolves). Two failure shapes to
guard against explicitly in the design:

- **Timeout wins the race even though the message already arrived**: the connect-timeout
  callback calls `abortControllerRef.current?.abort()` after `firstMessage` has already
  flipped `isConnectingRef.current = false` and `setIsConnected(true)`, but before the
  timeout's own cleanup ran (i.e. the timeout wasn't cleared in time). The abort would tear
  down a stream React/callers already believe is live, i.e. a duplicate/premature
  disconnect. Fix means: the timeout callback must be a no-op if `isConnectedRef.current`
  is already true (mirrors the existing `isConnectedRef.current || isConnectingRef.current`
  guard pattern already used at `useTerminalStream.ts:163`), and/or the timer must be
  cleared synchronously in the same code path that flips `firstMessage`
  (`useTerminalStream.ts:221-226`), not in a later effect.
- **Timeout fires, abort races the natural stream-close cleanup**: aborting
  `abortControllerRef.current` makes the `for await` loop throw (or return) into the
  existing `catch`/`finally` block (`useTerminalStream.ts:313-353`). That `finally` block
  already does its own state reset (`isConnectedRef.current = false`,
  `isConnectingRef.current = false`, `setTerminalState('DISCONNECTED')`) and — under
  `NEXT_PUBLIC_RECONNECT_V2` — schedules a *second* reconnect via
  `terminalBackoffRef.current.next()` / `reconnectTimerRef.current = setTimeout(...)`. If
  the new connect-timeout logic *also* independently schedules a retry (e.g. calls
  `connect()` again directly instead of just aborting and letting the existing `finally`
  block own retry scheduling), the two paths can each schedule a reconnect, producing a
  double-connect burst. **Design implication: the connect-timeout should only call
  `abortControllerRef.current.abort()` and let the existing `finally` block's retry logic
  own what happens next** (it already checks `shouldReconnectRef.current` and
  `isDisconnectingRef.current|| resync in progress`, and already computes the next backoff
  delay) — it should not itself call `connect()` or schedule an independent timer for
  retry. The only new thing the connect-timeout mechanism should own is *which* timeout
  value the *next* `next()`/attempt uses (short for foreground, normal otherwise), not a
  parallel retry path.

## 3. Timer leak across `disconnect()` / unmount if not added to the existing clear-timer set

This file already tracks two other timer refs that must be cleared on both `disconnect()`
and unmount: `reconnectTimerRef` (cleared in `disconnect()` at `useTerminalStream.ts:373-376`
and in the unmount cleanup at `useTerminalStream.ts:422-425`) and `terminalDebounceTimerRef`
(cleared in the visibility-listener effect's own cleanup, `useTerminalStream.ts:453`). A new
`connectTimeoutRef` (or similar) must be added to **both** of those same clear sites, plus a
third: it must be cleared as soon as the connect attempt resolves via `firstMessage` (success
path) AND in the `finally` block regardless of outcome, mirroring how the existing code
resets `textDecoderRef`/`scrollbackDecoderRef` unconditionally in `finally`
(`useTerminalStream.ts:328-330`). Missing any one of these four clear sites
(success/finally/disconnect/unmount) leaks a timer that can fire an abort against an
`AbortController` instance that has since been replaced by a subsequent `connect()` call
(see §4) — the classic "stale closure captured the old abortControllerRef value" bug, except
here `abortControllerRef` is a ref (shared mutable box), so the danger is actually the
inverse: the stale timer fires and aborts a *new*, unrelated in-flight connection because it
reads `abortControllerRef.current` fresh rather than a captured value. **The timeout
callback must capture the specific `AbortController` instance it was started for (closure
over a local `const myController = abortControllerRef.current` at schedule time) and compare
identity before acting**, not read `abortControllerRef.current` at fire time — otherwise a
leaked timer from attempt N can abort attempt N+1's unrelated controller.

## 4. Double-connect guard: does abort-on-timeout correctly reset state for the next `connect()`?

The guard at `useTerminalStream.ts:163` — `if (isConnectedRef.current ||
isConnectingRef.current || !sessionId) return;` — means a subsequent `connect()` call
(whether from the finally-block's own scheduled retry, or from `handleManualReconnect`, or
from a foreground-transition-triggered immediate reconnect per AC3) is a no-op unless
`isConnectingRef.current` has already been flipped back to `false`. Today
`isConnectingRef.current = false` is only set in two places: the `firstMessage` success
branch (`:222`) and the `finally` block (`:324`). Aborting via
`abortControllerRef.current.abort()` from a new connect-timeout does **not** by itself reset
`isConnectingRef` — it only causes the `for await` loop to throw/exit, which then runs the
existing `finally` block, which *does* reset it. So correctness here is contingent on the
abort actually propagating into that `finally` block promptly. Two ways this can go wrong:
- If the underlying transport/stream implementation swallows the abort signal or delays
  noticing it (e.g. the mocked `PushStream` in tests has no `AbortSignal` wiring at all —
  see §6), the `finally` block never runs, `isConnectingRef.current` stays `true` forever,
  and every subsequent `connect()` call becomes a silent no-op — this is the specific
  "double-connect guard blocks recovery" failure mode AC1/AC2's own timeout is trying to
  prevent, just relocated one level up.
- If the timeout handler manually sets `isConnectingRef.current = false` *itself* (instead
  of relying on `finally`) as a defensive belt-and-suspenders move, there is now a second
  writer of that flag, and if `finally` *also* runs (because abort did propagate), the flag
  is written twice — harmless by itself, but it signals the ownership boundary between
  "who decides connection is over" is unclear, and future edits are more likely to
  introduce a state where one writer resets refs the other writer still expects to be
  in-flight (e.g. resetting `abortControllerRef.current = null` in the timeout handler
  while `finally` still expects to read it).
- **Recommendation for the plan**: keep exactly one owner of `isConnectingRef`/`isConnectedRef`
  reset — the existing `finally` block — and have the connect-timeout mechanism only ever
  call `.abort()` (plus manage its own timer ref). Do not add a second reset path.

## 5. Interaction with the *existing* retry scheduling in the `finally` block

The `finally` block's retry scheduling (`useTerminalStream.ts:331-352`) already computes
`terminalBackoffRef.current.next()` for the *delay before the next attempt starts*. The new
connect-timeout is a different axis — *how long the next attempt itself is allowed to run*
— but the two are coupled through the same `terminalBackoffRef.current.attempt` counter,
because AC2 says "first 2 [foreground] attempts use ~1200-1500ms timeout." That means the
new mechanism needs to read `terminalBackoffRef.current.attempt` *at connect-timeout-timer
schedule time* (inside `connect()`, before the attempt increments further) to decide fast
vs. normal, and the requirements' AC3 (foreground false→true "resets backoff attempt
counter") means a foreground transition must call `terminalBackoffRef.current.reset()`
somewhere outside of `connect()`'s own unconditional `reset()` call at line 166 — today
`connect()` *always* resets backoff on every call (`terminalBackoffRef.current.reset()` at
`:166`), which actually already satisfies part of AC3 for the case where the
foreground-transition triggers a fresh `connect()` call (mirroring the existing visibility
listener at `:442`). But if AC3 is meant to apply mid-backoff-sleep (foreground flips true
while `reconnectTimerRef` is already counting down toward a scheduled retry, without a
fresh explicit `connect()` call), the plan needs to state whether a bare foreground flip
should *cancel the pending backoff sleep and connect immediately* (matching the visibility
listener's existing pattern: clear debounce, reset backoff, call `connectRef.current()`) or
just adjust the *next* attempt's timeout value without accelerating it. The requirements
text ("resets backoff counter" on the false→true transition) reads as intending the former,
but the plan must say so explicitly since it changes user-visible timing (an immediate
reconnect attempt vs. waiting out the current backoff delay).

## 6. Testing pitfalls: `Promise.race`-against-timeout with Jest fake timers

The existing test suite (`useTerminalStream.test.ts`) already exercises fake timers
extensively (`jest.useFakeTimers()` in `beforeEach` at line 390, `jest.advanceTimersByTime`
throughout) and already has a working pattern for advancing past the reconnect backoff delay
(`jest.advanceTimersByTime(35000)` — comfortably past the 30s cap). A connect-timeout race
adds real hazards on top of that pattern:

- **The stream mock (`makePushStream`, lines 110-142) has no `AbortSignal` awareness at
  all.** Its `next()` just awaits an internally-queued resolver; it never checks
  `abortControllerRef.current.signal.aborted` and would never itself reject/throw when
  aborted. If the connect-timeout implementation relies on `abortController.abort()` alone
  to unblock the `for await` loop, **the existing mock stream will hang forever** in tests
  unless the test explicitly calls `stream.end()` after advancing the timer, or the mock is
  extended to observe the abort signal. Any new tests (AC6: "connect-timeout-abandonment
  triggering retry") must either (a) extend `PushStream`/the test harness to reject its
  pending `next()` promise when the associated `AbortSignal` fires, or (b) manually call
  `stream.end()`/throw after `advanceTimersByTime` to simulate what a real aborted
  ConnectRPC stream would do. Forgetting this means a test that "advances past the
  connect-timeout" will hang the `for await` loop indefinitely and the test itself will
  time out or silently never observe the disconnected state — a classic fake-timer +
  unresolved-real-promise deadlock (advancing fake timers does not make a hung
  `await new Promise(...)` that isn't waiting on a timer resolve).
- **Mixing real microtask flushing with fake macrotask advancement is brittle.** The
  existing tests already lean on `await act(async () => { jest.advanceTimersByTime(N); })`
  to let both the timer *and* any microtasks queued by its callback flush before assertions
  — a `Promise.race([streamFirstMessage, timeoutPromise])`-shaped implementation adds
  another microtask hop (the race's `.then`) that must also flush inside the same `act()`
  call, or assertions will observe pre-race-settlement state. If the implementation instead
  uses a plain `setTimeout` + `abort()` (recommended over an explicit `Promise.race`, since
  the existing code has no promise to race against — `connect()` doesn't return a promise
  that resolves on success) this specific pitfall is largely avoided, but is worth calling
  out explicitly in the plan as *why* `Promise.race` is the wrong shape here: there is no
  "connection succeeded" promise anywhere in this file to race against; success is signalled
  by mutating refs/state inside an unawaited async IIFE, not by resolving a promise `connect()`
  holds onto.
- **Reusing `mockStreamTerminal.mockReset()`/`mockImplementation` per-test (as done
  throughout the existing suite) means each new connect-timeout test needs its own
  fresh `PushStream` and must advance the fake clock past the *specific* fast (1200-1500ms)
  or normal (3500ms) threshold being tested** — reusing the blanket `35000ms` advance used
  by existing reconnect-backoff tests would also fire the connect-timeout, muddying which
  mechanism the test is actually exercising. Tests need tight, mechanism-specific timer
  advances (e.g. `advanceTimersByTime(1400)` to prove the fast path fired but the 3500ms
  normal path has not) rather than reusing the existing coarse 35s advance.
- **`isHardFailedRef`/attempt-cap interaction**: the `finally` block already hard-fails
  after `terminalBackoffRef.current.attempt >= 5` (`useTerminalStream.ts:334-337`). A
  connect-timeout that also increments/reads the same counter needs a test proving repeated
  connect-timeout-triggered aborts don't uncap or double-count against that limit in a way
  that changes the existing hard-fail-at-5-attempts contract (AC7: no regression to existing
  suites) — i.e. a connect-timeout abandonment should count as "one attempt" toward the same
  cap that a clean-close-driven reconnect already counts against, not a separate budget.

## 7. Foreground/background flips mid-flight — which timeout intent should win?

Scenario from the research question: session A is foreground, `connect()` starts using the
fast-timeout intent; before that attempt resolves (fails or succeeds), the user switches
selection to session B, so A becomes background before its own connect-timeout has fired.

- **What the codebase's existing patterns imply**: `foreground` would naturally be threaded
  in as a hook option (`UseTerminalStreamOptions.foreground?: boolean`, per AC0/AC4), read
  fresh via a ref (mirroring how `isConnectedRef`/`shouldReconnectRef` already exist
  specifically so async callbacks read current-not-stale values) rather than captured in the
  `connect()` closure by value — `connect()` is itself a `useCallback` with a dependency
  array (`useTerminalStream.ts:360-361`) that does not currently include anything
  foreground-related, so a plain closed-over boolean would go stale the instant `connect()`
  isn't re-created (which per React's rules happens exactly when the surrounding
  component re-renders with a new prop — not synchronously mid-flight).
- **Two legitimate designs, and the plan must pick one explicitly (this is the crux of the
  research question, not a settled fact)**:
  1. **Snapshot-at-schedule-time (in-flight attempt keeps its original intent)**: the
     connect-timeout duration is decided once, when the timer is scheduled inside
     `connect()`, using whatever `foreground` value was true *then*. A mid-flight
     backgrounding of session A does not shorten or lengthen A's already-scheduled timeout.
     This matches how most reconnect-timeout implementations behave (the attempt is already
     "in flight," changing its budget mid-attempt is surprising) and is simpler to reason
     about and test (no need to re-read a ref inside an already-scheduled timer callback).
  2. **Live-read-at-fire-time (background demotes an in-flight fast attempt)**: the timer
     callback re-reads a `foregroundRef.current` at fire time and only actually aborts+retries
     with fast semantics if still foreground; otherwise treats it as a normal-timeout
     failure. This is more "correct" to the literal spirit of "background connections get a
     longer grace period," but adds complexity (the fired timeout's *consequence*, not just
     its *duration*, becomes conditional on current state) for a benefit that's marginal —
     the user already switched away from A, so a slightly-too-patient or slightly-too-eager
     abort of a tab they're not looking at has low visible impact.
  - **Recommendation for the plan**: pick design (1) (snapshot-at-schedule-time) as the
    default — it requires no new ref-reading inside the timer callback, cannot race with a
    prop change mid-timer, and matches the acceptance criteria's phrasing ("first 2
    [foreground] attempts use ~1200-1500ms timeout" describes the attempt, not a
    continuously-reevaluated live state). The plan should still state this explicitly per
    AC5's spirit of being unambiguous about scope, since it's a real design fork and not
    self-evident from the requirements doc.

## 8. Which reconnect path(s) this applies to (AC5)

Confirmed by reading the gating condition at every reconnect trigger site:
- **`NEXT_PUBLIC_RECONNECT_V2 === "true"` hook-level path** (`useTerminalStream.ts:331`,
  the `finally` block's backoff-driven retry, and the visibility/online listener at
  `:433-458`) is the only path with the concept of "attempt count" (`terminalBackoffRef`)
  and the only path a `foreground` option can meaningfully interact with — this is where
  AC1-AC3 belong.
- **The pre-flag path in `TerminalOutput.tsx`** (`reconnectTimeoutRef`, lines ~736-778,
  ~990-992) does not call `useTerminalStream`'s internal backoff/reconnect logic at all —
  when the flag is off, `useTerminalStream`'s own `finally` block skips its retry branch
  entirely (`useTerminalStream.ts:331` guards on the flag), and `TerminalOutput.tsx`
  instead just starts a flat 5000ms timer that shows a manual "reconnect" button
  (`TerminalOutput.tsx:764-770`) — there is no automatic retry, foreground or not, that a
  connect-timeout could plug into on this path. The plan must state that this feature is
  **scoped to the `NEXT_PUBLIC_RECONNECT_V2` hook-level path only**; wiring `foreground`
  through when the flag is off would have no effect since no automatic-reconnect logic runs
  in that mode.
