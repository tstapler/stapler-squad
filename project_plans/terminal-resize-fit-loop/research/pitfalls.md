# Research: Pitfalls — Terminal Resize/Fit Feedback Loop Fix

Agent 4 (Pitfalls), Phase 2 research for `terminal-resize-fit-loop`.

## 0. Correction to the research brief: this repo uses Jest, not Vitest

`web-app/package.json` (`"test": "jest"`, `jest: "^30.2.0"`) and `web-app/jest.config.js`
(`preset: 'ts-jest'`, `testEnvironment: 'jsdom'`) confirm the test runner is **Jest 30**, not
Vitest. `web-app/jest.setup.js` currently only polyfills `TextEncoder`/`TextDecoder` — there is
**no global `ResizeObserver` polyfill/mock today** (`grep -rn "ResizeObserver" web-app/jest.setup.js`
and a repo-wide search for `global.ResizeObserver`/`vi.stubGlobal.*ResizeObserver` both came back
empty). Any new test that mounts `XtermTerminal` and exercises its `ResizeObserver` callback will
need to add this mock from scratch — there's no existing pattern in the repo to copy. This changes
some of the guidance below (Jest's modern fake timers ≠ Vitest's), see §5.

---

## 1. Classic ResizeObserver pitfalls

**"ResizeObserver loop limit exceeded"** is the browser's own kill-switch: if a RO callback causes
a further size change to an observed element within the *same* frame, the spec allows the browser
to keep re-delivering "undelivered notifications" for up to a few cycles, then gives up and fires a
(non-fatal, but console-visible) error and defers the rest to the next frame. It is *not* what's
directly reported by users here (CPU pegged / input dead) — that symptom means the loop is escaping
this single-frame limit entirely, because each `fit()` happens inside a `setTimeout` debounce
(`XtermTerminal.tsx:289-293`), which pushes each iteration into a *new* frame/macrotask. The
browser's loop-limit-exceeded protection only guards against tight *synchronous* re-entrant loops
within one frame; a callback → `setTimeout` → resize → callback chain spanning multiple frames is
invisible to that protection and can run forever.

- **Exact bug class**: XtermTerminal's `ResizeObserver` observes `containerRef.current`, and its
  callback (via the debounced `fitAddon.fit()`) can itself change layout in ways that alter
  `containerRef.current`'s content-box: `fit()` sets `terminal.rows`/`cols`, which resizes the
  canvas/WebGL surface xterm renders into, which can be 1px off from the container due to
  the glyph-width rounding mismatch called out in the requirements (`XtermTerminal.tsx:188-197`).
  With multiple terminals mounted (per requirement acceptance criterion 1), one pane's `fit()` can
  also perturb a *sibling* pane's layout if they share a flex/grid parent whose track sizing
  depends on content size — classic "observing an element whose own resize is caused by the
  observer's callback," just laundered through a shared ancestor instead of directly.
- **Standard fixes** (MDN, web.dev, and the RO spec editors' own guidance):
  1. **Batch mutating work in `requestAnimationFrame`**, not immediately or in a raw `setTimeout`.
     The RO spec explicitly recommends deferring any DOM-mutating response to a resize to the next
     animation frame so it's not observed as part of the same delivery cycle. The current code
     does the opposite of this in the danger zone: it starts a `setTimeout(..., debounceDelay)`
     from inside the RO callback and calls `fit()` synchronously inside that timeout — no RAF
     wrapper, so `fit()`'s layout writes and RO's next read are not framed together.
  2. **Threshold / hysteresis**: gate on a *meaningful* change, not any change. The current
     container-size gate (`Math.abs(width - lastContainerSize.width) > 1`, `XtermTerminal.tsx:269-270`)
     is a reasonable pixel-level hysteresis but is provably insufficient here because 1px of
     container jitter can straddle a *cell-width* boundary — sub-cell jitter under 1px in container
     terms can still flip `proposeDimensions()`'s rounded integer `cols`/`rows` if the true cell
     width is, say, 8.97px vs the measured 9.02px (see §4 mismatch). The fix direction in
     requirement 2 — gate on **integer cols/rows delta**, not raw pixel delta — is the right
     threshold to use, since it's the actual unit `fit()` cares about, but see §2 for a subtlety
     this introduces.
  3. **Disconnect during the callback** ("ignore self-inflicted notifications"): a common pattern
     is `resizeObserver.unobserve(el)` (or a boolean re-entrancy guard) immediately before doing
     any layout-mutating work inside the callback, then re-`observe()` on the next frame after the
     mutation settles. This repo does *not* do this today — `resizeObserver.observe(containerRef.current)`
     is called once at mount and never toggled. Consider whether the confirm-twice mechanism
     (requirement 2) is meant to substitute for this, or whether both are wanted; they solve
     overlapping but not identical problems (confirm-twice bounds *how many fits* happen;
     disconnect-during-callback bounds *re-entrancy from fit's own layout writes*).
  4. **Don't read layout-invalidating properties inside the RO callback and then write to the DOM
     in the same tick** — this is the forced-synchronous-layout ("layout thrashing") pattern, which
     compounds jitter. `XtermTerminal.tsx` calls `entry.contentRect` (already computed, safe) but
     the *debug logging* on lines 276-277 also reads `terminalRef.current.cols`/`.rows` (fine,
     no forced layout) — no current forced-layout smell here, but any fix that adds
     `getBoundingClientRect()` calls inside the RO callback itself (as opposed to inside the
     init-time RAF block) would reintroduce this risk.

## 2. Pitfalls in the "two consecutive ticks" confirmation strategy — worked through with a trace

This is the most consequential correctness question in the whole fix, and it hides two distinct
failure modes depending on what a "tick" is defined to be.

### 2a. What counts as a "tick" matters enormously

`ResizeObserver` **only fires when the observed box's size actually changes** — it does not tick on
a fixed cadence. This has a sharp consequence for a naive reading of requirement 2
("...AND repeat on two consecutive ticks"):

- If "tick" = "one `ResizeObserver` callback invocation," then a **genuine, clean, one-shot resize**
  (window resized once, container settles immediately to a new stable size, browser coalesces to a
  single RO delivery for that settle) produces **exactly one** RO invocation. There is no second
  invocation coming, because the size isn't changing again — by definition, RO won't fire a second
  time for an unchanged size. A naive implementation that waits for "the same candidate value to
  arrive on RO invocation N and N+1" will **wait forever** for a confirming tick that structurally
  cannot occur, and legitimate resizes will never apply. This is worse than the "one tick of
  latency" the research question worried about — it's unbounded latency (never converges) for the
  most common, unproblematic case.
- **Conclusion**: the "tick" driving the two-consecutive-observations check must come from a source
  *decoupled from RO firing* — e.g., a RAF-driven or `setTimeout`-driven re-sampling loop that
  calls `fitAddon.proposeDimensions()` again on a fixed short delay *after* the debounce settles,
  independent of whether RO fires again. (This is also implicitly what the existing
  `setTimeout(..., 100)` "secondary fit" pattern at init, `XtermTerminal.tsx:200-206`, already does
  — it's a precedent for a decoupled re-sample, just not used for the ongoing RO path.) Get this
  wrong and requirement 2 either open-loops (fits every RO delivery, no protection) or dead-locks
  (never fits at all).

### 2b. Does "differs AND repeats twice" actually break an A/B/A/B oscillation?

Concrete trace. Let `applied` = the cols/rows currently applied to the terminal (starts at 80).
Assume decoupled per-tick sampling (per §2a) so RO firing isn't the limiting factor, and the
naive semantics are: *candidate is confirmed and fit() is scheduled only if the same proposed value
X differs from `applied` AND was also the proposed value on the immediately preceding tick.*

| tick | proposeDimensions() | differs from applied(80)? | candidate | confirmCount | action |
|---|---|---|---|---|---|
| 1 | 79 | yes | 79 | 1 | none |
| 2 | 80 | **no** (80 == applied) | — | reset to 0 | none |
| 3 | 79 | yes | 79 | 1 (candidate reset since prior tick didn't extend it) | none |
| 4 | 80 | no | — | reset to 0 | none |
| ... | (repeats forever) | | | | **`fit()` never fires** |

So for the specific case where the oscillation is *between the currently-applied value and one other
value* (the most likely real-world shape of this bug, since the WebGL glyph-mismatch jitter is
typically ±1 cell around the "true" fit point which often coincides with a size the terminal has
already settled at), the naive two-consecutive-ticks rule **does** break the infinite-fit loop —
but only as a side effect of the fact that 79 and 80 never appear on two *consecutive* ticks
together (they alternate, so the "same candidate on tick N and N+1" condition is never satisfied
for either value). The loop is broken, but **the terminal gets permanently stuck at 80** even if 79
is the "genuine" correct size — this is not a 1-tick latency bug, it's an indefinite convergence
failure, silently disguised as success because the CPU-pegging symptom goes away. Left as-is, a
user would see a terminal that's subtly the wrong size until they hit the manual "Fit" button
(which requirement 4 correctly force-bypasses this whole mechanism).

Now consider oscillation between two values **neither of which equals** `applied` — e.g. jitter
between 78 and 79 while `applied` is 80:

| tick | proposeDimensions() | differs from applied(80)? | candidate | confirmCount | action |
|---|---|---|---|---|---|
| 1 | 78 | yes | 78 | 1 | none |
| 2 | 79 | yes, but ≠ candidate(78) | 79 | reset to 1 | none |
| 3 | 78 | yes, but ≠ candidate(79) | 78 | reset to 1 | none |
| ... | (repeats forever) | | | | **`fit()` never fires** |

Same outcome — the literal "same value twice in a row" reading of "repeats on two consecutive
ticks" never fires for a strict 2-cycle alternation between *any* two distinct values, regardless of
whether either matches `applied`. **This is good for loop-prevention (no infinite RPC storm) but bad
for convergence** (never settles, needs an escape hatch). A 3-or-more-state cyclic oscillation
(A,B,C,A,B,C,...) has the identical property for the same reason: consecutive ticks are never equal.
So the mechanism as literally specified is loop-safe against *any* period-≥2 cyclic oscillation, but
provides zero forward progress in that case — worth explicitly deciding (in the plan phase) whether
that's acceptable or whether a bounded escape hatch is needed (e.g., after N failed confirmations,
force-apply the most recent value, or trigger the WebGL→canvas fallback from requirement 5 since
sustained oscillation is itself the fallback's trigger condition).

There is a second, looser possible reading of requirement 2 — "differs from applied on two
consecutive ticks" (not requiring the *same* value both times, just that both ticks disagree with
`applied`, whatever they individually are) — which behaves differently: in the 78/79 oscillation
above it *would* fire on tick 2 (both tick 1 and tick 2 differ from applied=80), applying whichever
value was seen most recently (79), and is therefore NOT immune to committing to a still-jittering
value. **The two readings are not equivalent and the plan/implementation phase must pick one
explicitly and encode it in a test**, because the "same value twice" reading is the one that
actually prevents committing to jitter, while the "differs twice" reading only reduces churn
frequency without eliminating oscillation-driven incorrect commits.

### 2c. Stuck confirmation state / stale debounce interaction

If the "tick" sampler is itself gated behind the existing `setTimeout` debounce
(`resizeTimeout` in `XtermTerminal.tsx:284-293`), there's a subtle trap: that debounce is
*restarted* (`clearTimeout` + new `setTimeout`) on every qualifying RO delivery. If ticks are
produced only when the debounce timer itself fires, and every new RO delivery cancels the pending
timer and reschedules it, then **a still-jittering container can perpetually cancel the debounce
before it ever fires even once**, meaning zero ticks are ever produced and the confirmation state
never advances (stuck at `confirmCount = 0` forever) — this is the "debounce timer fires exactly
once with a stale value" case the brief flagged, except worse: under sustained jitter it can be
starved from firing *at all*, not just fire once with data that's already stale by the time it runs.
The safe pattern is to decouple "when do we re-sample" from "did RO deliver again" — e.g. use a
fixed-cadence RAF poll for a bounded window (a few frames) after the *first* qualifying delivery,
regardless of whether more RO deliveries arrive in between, rather than a debounce that resets on
every delivery.

## 3. Pitfalls in the value-dedup approach (`useTerminalFlowControl.resize()`)

Current code (`useTerminalFlowControl.ts:364-377`) only has a time throttle
(`THROTTLE_MS = 200`, gated on `lastResizeTimeRef`), no value comparison at all. Requirement 3 adds
a value-dedup ("skip if (cols,rows) == last pair *actually sent*") that must coexist with the
existing time throttle and with the new `force: true` parameter from requirement 4.

- **Order of evaluation matters and has an observable difference, not just a performance one.**
  - If value-dedup is checked **before** the time throttle: an unchanged value is dropped
    immediately regardless of how much time has passed — correct, and it also avoids updating
    `lastResizeTimeRef` on a no-op call, which matters because a chatty caller sending the same
    size repeatedly would otherwise keep pushing the throttle window forward and could end up
    delaying a *genuinely new* size that arrives shortly after (throttle window kept "warm" by
    no-op calls). Checking dedup first avoids that starvation.
  - If the time throttle is checked **first**: a call that would've been deduped as unchanged is
    instead silently dropped by the throttle when within 200ms, and *accepted* (with no value
    check at all, under the current code) once the throttle window reopens — meaning an
    unchanged-value call that lands just after the 200ms window closes would incorrectly slip
    through today. Order-first-throttle is what's currently implemented and is exactly the gap
    requirement 3 exists to close; the fix must pick dedup-first (or run both, combined with AND)
    to actually guarantee "value-dedup independent of the 200ms throttle" as the acceptance
    criterion demands verbatim.
  - Either order is *safe* w.r.t. `force: true` only if `force` is checked before both and
    short-circuits them entirely — otherwise a forced call issued within 200ms of the last send,
    or with a value coincidentally equal to `lastSentSizeRef`, would be silently swallowed, directly
    breaking requirement 4 (reconnect-resync and manual Fit *must* always send).
- **Forgetting to update "last sent" state on the `force: true` path is a real, easy-to-miss bug
  with two independent failure directions:**
  1. **If `force: true` sends the RPC but does *not* update `lastSentSizeRef`**: a subsequent
     *non-force* call with the identical (cols, rows) will incorrectly believe the value is new
     (since `lastSentSizeRef` still holds whatever it was before the forced send) and will send a
     redundant duplicate RPC — not a correctness bug for the server (idempotent resize), but it
     defeats the whole point of adding dedup and can reintroduce churn if that non-force call
     happens to be inside a resize-loop-adjacent code path.
  2. **If `force: true` updates `lastSentSizeRef` optimistically *before* confirming the underlying
     `pushMessage()` actually succeeded** (it currently can throw — see the `try/catch` around
     `pushMessage` at `useTerminalFlowControl.ts:379-415`): and the send throws, `lastSentSizeRef`
     is now claiming the server has a size it never received. A later legitimate call reverting to
     that same size (e.g., a genuine resize back to a previous size) would be incorrectly deduped
     away as "unchanged," permanently desyncing client and server pane dimensions until the next
     *different* size arrives. The safe pattern is to update `lastSentSizeRef` only after the send
     call returns without throwing (matching how `lastResizeTimeRef` is already updated *before* the
     `pushMessage` call today at line 381, which is itself arguably an existing latent bug in the
     same family — worth flagging to the plan phase as a possible fix-while-we're-in-here).
  - **The reconnect case specifically breaks the "same value = no-op" assumption dedup relies on.**
    After a reconnect, the *server's* actual state of the pane may not match what the client
    believes it last sent — the server process may have restarted, or a different backend instance
    may now be handling the session. `lastSentSizeRef` reflects only client-side history and is not
    a proxy for server-side truth once a connection cycles. This is precisely why requirement 4
    mandates `force: true` at the reconnect-resync call site rather than relying on dedup improving
    over time — it's not just an optimization choice, it's necessary for correctness. A test that
    only asserts "the literal third argument is `true`" (as requirement 4 specifies) won't catch a
    regression where `force` is passed correctly but the dedup/throttle short-circuit order still
    swallows it — the test should also assert the RPC was actually observed to be sent, not just
    that the call site *passed* `force: true`.

## 4. Pitfalls in xterm.js WebGL → Canvas addon fallback

- **`WebglAddon.dispose()` was historically unimplemented.** GitHub issue
  [xtermjs/xterm.js#2254](https://github.com/xtermjs/xterm.js/issues/2254) ("Webgl:
  WebglAddon.dispose is not implemented") tracked exactly this — for a long stretch of xterm.js's
  history, once you loaded the WebGL addon there was **no supported way to switch back to
  DOM/canvas rendering**, because `dispose()` was a no-op stub. It was eventually closed via a fix
  (referenced as landing around PR #2548 in that thread), so modern `@xterm/addon-webgl` versions
  do support disposal — **but this means the exact `@xterm/addon-webgl` version pinned in
  `web-app/package.json` needs to be checked** before relying on a dispose-then-load-canvas-addon
  fallback; on an old enough version, `dispose()` may silently do nothing and the fallback would be
  a no-op that still leaves WebGL active.
- **GPU memory leak on dispose** — [xtermjs/xterm.js#3889](https://github.com/xtermjs/xterm.js/issues/3889)
  ("The webgl addon leaks GPU memory") reports WebGL resources (textures, buffers) not being
  released correctly even when `dispose()` *is* called, fixed via a later PR (#3890) that
  explicitly frees GL resources on dispose. If the pinned xterm.js/addon-webgl version predates that
  fix, repeatedly toggling WebGL→canvas→WebGL (e.g. if the fallback isn't strictly one-directional,
  contradicting requirement 5's explicit "one-directional fallback" design choice) would leak GPU
  memory per toggle. This is one of the concrete reasons requirement 5 specifies the fallback must
  be one-directional and not a toggle — worth preserving that constraint strictly, including making
  it structurally impossible to re-enable WebGL later in the same terminal instance's lifetime, not
  just a soft convention.
- **Recommended safe-disposal pattern from xterm.js itself**: the addon exposes an
  `onContextLoss` event, and the documented pattern is
  `webglAddon.onContextLoss(e => { webglAddon.dispose(); })` — i.e., dispose *from inside* the
  context-loss handler, not from an arbitrary external trigger. If the fallback in requirement 5 is
  instead triggered by *this codebase's own* px/col-mismatch heuristic (not a native
  `webglcontextlost` event), disposal should still happen on a microtask/next-tick boundary rather
  than synchronously inside whatever code path detected the mismatch, to avoid disposing the addon
  while it's mid-frame-render (see next point).
- **Mid-fit/resize disposal risk.** The requirements explicitly call out this exact question
  ("does disposing WebglAddon while a fit()/resize is in-flight cause a crash"). No open GitHub
  issue was found that pins this exact crash for current versions, but the closely-related
  [xtermjs/xterm.js#1416](https://github.com/xtermjs/xterm.js/issues/1416) ("Browser crash related
  to fit addon returning geometry with 'Infinity' sizes") shows the *general failure family* is
  real: when `fit()` reads renderer dimensions that are transiently `null`/not-yet-computed (e.g.
  `term.renderer.dimensions.actualCellWidth === null`, which can happen right after a renderer swap
  before the new renderer has completed its first measurement pass), `proposeDimensions()`/`fit()`
  can compute `Infinity` and attempt to resize the terminal buffer to it, consuming all available
  RAM and crashing the tab. **A WebGL→canvas swap is exactly the kind of event that can leave
  renderer dimensions transiently unmeasured** (the new canvas renderer hasn't done its first
  measurement yet), so triggering `fit()` synchronously right after/during the addon swap without a
  `Number.isFinite()` guard on the proposed dimensions (which requirement 5 already mandates) is a
  real crash risk backed by precedent, not a hypothetical. Sequence the swap as: dispose old addon →
  load new addon → wait one frame (RAF) for it to complete initial measurement → *then* call
  `fit()` with the `Number.isFinite` guard, rather than fitting synchronously in the same tick as
  the swap.
- **Visual flash/blank frame on swap.** Disposing the WebGL addon detaches its `<canvas>` (or
  hides it) and the fallback renderer needs at least one paint before content reappears; between
  those two points there is a real frame (or several) where the terminal viewport can show blank or
  stale pixels. This is a real but low-severity risk given the trigger condition (sustained px/col
  mismatch beyond tolerance) is expected to be rare and one-time per session — but if the mismatch
  heuristic is too sensitive it could fire spuriously and cause visible terminal flicker as a side
  effect purely from the corrective action, independent of the original bug. Recommend logging (or
  even a one-line toast/console warning, matching the existing debug-logging style already in
  `XtermTerminal.tsx`) whenever the fallback fires, since it's meant to be rare and any occurrence
  is diagnostically interesting.
- **Version check is a concrete action item, not just a risk to note**: confirm the exact
  `@xterm/addon-webgl` (and `@xterm/xterm`) versions pinned in `web-app/package.json` against the
  fix commits for #2254 and #3889 before assuming `dispose()` is safe and leak-free. If the pinned
  version predates those fixes, either bump the dependency as part of this work or explicitly scope
  "no dispose() while WebGL is active" out of the fallback design (e.g. reload the whole
  `Terminal` instance instead of just swapping addons, which sidesteps addon-level dispose bugs
  entirely at the cost of losing in-memory scrollback/state momentarily).

## 5. Testing pitfalls (Jest 30 + jsdom — see §0 correction)

- **No `ResizeObserver` polyfill exists in this repo today.** `jsdom` does not implement
  `ResizeObserver` natively (as of the jsdom version bundled with Jest 30's `testEnvironment: 'jsdom'`),
  and `web-app/jest.setup.js` doesn't add one. Any new test exercising `XtermTerminal`'s RO callback
  needs a mock `ResizeObserver` class installed on `global`/`window` (either hand-rolled — a class
  storing the callback and exposing a way to synthesously invoke it with fabricated
  `ResizeObserverEntry`-shaped objects — or a small package). Because this is net-new, get the
  mock's call-timing right deliberately: real `ResizeObserver` callbacks are batched and delivered
  *after* layout, asynchronously relative to the triggering DOM mutation; a naive mock that invokes
  the callback **synchronously** inside `observe()`/on every property mutation will make tests pass
  for reasons that don't hold in a real browser (e.g. masking the exact re-entrancy/ordering bugs
  this fix is trying to catch).
- **Jest's modern fake timers *do* fake `requestAnimationFrame` by default** (unlike Vitest, which
  requires opting in via `toFake: [...]`). `web-app/jest.config.js` doesn't set `fakeTimers`
  globally, so per-test `jest.useFakeTimers()` calls (already used in
  `useTerminalFlowControl.test.ts:88`) get Jest's default "modern" (`@sinonjs/fake-timers`-based)
  implementation, which mocks `requestAnimationFrame`/`cancelAnimationFrame` alongside
  `setTimeout`/`setInterval` out of the box. This is good news for testing the nested
  `requestAnimationFrame → requestAnimationFrame → setTimeout` chain in `XtermTerminal.tsx:163-208`
  and the RO-callback → `setTimeout` chain at lines 259-294 — both *can* be driven deterministically
  with `jest.advanceTimersByTime(ms)` — but it inverts one piece of guidance a Vitest-focused brief
  would give (no need to add RAF to a `toFake` list; it's already there).
- **`jest.runAllTimers()`/`jest.runOnlyPendingTimers()` are actively dangerous for tests of *this
  specific bug*.** The whole premise of the bug being fixed is a chain that reschedules itself
  (RO callback → `setTimeout` → `fit()` → triggers new RO delivery → new `setTimeout` → ...).
  `runAllTimers()` keeps executing queued timers **until the queue is empty**, so a test written
  against a not-yet-fixed (or incompletely fixed) implementation would hang the Jest worker instead
  of failing with a useful assertion — this is a real risk specifically because these tests exist to
  catch regressions of an unbounded-loop bug. Use `jest.advanceTimersByTime(<fixed ms budget>)` in
  bounded increments and assert a call-count ceiling (e.g. "`fit()` called ≤ N times after
  simulating M ticks of jitter over 2 seconds of virtual time") rather than "run until settled" —
  the *point* of several of these tests is to prove the code does NOT run until naturally settled
  on its own within an unbounded number of iterations.
- **Nested `setTimeout`/RAF chains interacting with `mockReturnValueOnce` sequencing is fragile.**
  `XtermTerminal.tsx` calls `fitAddon.proposeDimensions()` in more than one place per logical
  "resize event" — once for debug logging (`XtermTerminal.tsx:173`, `202`) and implicitly again
  inside `fit()` itself. A test that mocks `proposeDimensions()` with a queued sequence via
  `mockReturnValueOnce(...).mockReturnValueOnce(...)` to simulate jitter across ticks can silently
  desync from the intended tick if the implementation calls it a different number of times per tick
  than the test author assumed (e.g. once for logging + once inside `fit()` = 2 calls consumed per
  "logical tick," not 1) — the test will still run and produce *a* result, just not the one the
  scenario intends, which is a classic silent-wrong-assertion trap rather than an outright failure.
  Prefer mocking at a coarser boundary (stub `fit()` itself and drive its behavior off a
  `proposeDimensions` mock that returns a value keyed off elapsed virtual time / call count checked
  inside the mock, not off `mockReturnValueOnce` chaining) to make the mock robust to incidental
  changes in how many times internal code calls `proposeDimensions()`.
- **`@xterm/addon-webgl` / WebGL are unusable in jsdom.** `jsdom`'s `HTMLCanvasElement.getContext('webgl')`
  returns `null` (no real WebGL implementation), so `new WebglAddon()` and any WebGL-context-loss
  simulation cannot be exercised through real xterm.js/browser APIs in this test environment — the
  existing code already tolerates this gracefully via its `try/catch` around `webglAddon` construction
  at init (`XtermTerminal.tsx:150-155`), but that means any new tests for requirement 5 (WebGL→canvas
  fallback triggering on sustained px/col mismatch) **must test the fallback's decision logic and
  guard conditions in isolation** (mock the addon's public surface — `onContextLoss`, `dispose()`,
  and a fabricated dimensions object) rather than attempting true WebGL integration coverage — and
  should be explicit in the test names/comments that this is deliberately testing the fallback
  *trigger and disposal sequencing*, not real WebGL behavior, so a future reader doesn't mistake
  green tests here for real-browser WebGL verification. Manual/E2E verification (acceptance
  criterion 7, "proxy: concurrent instances") remains the only way to observe the real WebGL path.
- **`act()`/async warnings from timer-driven state updates.** Both call sites under test
  (`resizeObserver` callback → `setTimeout` → `fitAddon.fit()` → xterm's `onResize` → React state
  update in the parent) and the RAF-chained init path update component state from inside fake-timer
  callbacks. Advancing fake timers outside of React Testing Library's `act()`/`await act(async () =>
  {...})` wrapper is a common source of "An update to Component inside a test was not wrapped in
  act(...)" warnings and, worse, of tests that pass locally but flake in CI depending on
  microtask/macrotask ordering — wrap every `jest.advanceTimersByTime(...)` call that's expected to
  trigger a state update in `act()`.

---

## Summary of concrete, codebase-grounded findings (not just general theory)

- `web-app/jest.setup.js` has no `ResizeObserver` mock/polyfill at all today — this is greenfield
  test infrastructure, not an existing pattern to extend.
- The project is **Jest 30**, not Vitest as the research brief assumed; Jest's modern fake timers
  fake `requestAnimationFrame` by default (Vitest does not), which simplifies driving
  `XtermTerminal.tsx`'s nested RAF/setTimeout chains in tests but makes `runAllTimers()` a hang
  risk specifically for these loop-regression tests.
- `useTerminalFlowControl.ts:381` currently updates `lastResizeTimeRef.current = now` **before**
  calling the (throwable) `pushMessage(...)` — the same "update tracking state before confirming
  the send succeeded" hazard flagged for the new `lastSentSizeRef` dedup state already exists in
  the codebase for the time-throttle state today, and is worth fixing in the same change.
- `xtermjs/xterm.js` GitHub history confirms three specific, version-dependent hazards relevant to
  requirement 5: `WebglAddon.dispose()` was a no-op for a long stretch (#2254), disposal leaked GPU
  resources even once implemented (#3889) until a later fix, and `fit()` computing `Infinity` from
  transiently-unmeasured renderer dimensions is a real, previously-crash-causing failure mode
  (#1416) — directly motivating requirement 5's `Number.isFinite` guard and the plan phase should
  add "check pinned `@xterm/addon-webgl` version against these fix commits" as an explicit task.
