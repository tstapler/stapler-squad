# Pitfalls & Risks: Terminal Resize/Fit Feedback Loop Convergence

Agent 4 research output. Focus: AC6 regression risk (over-aggressive fix swallowing a
legitimate resize), oscillation-detector false positives, WebGL disposal correctness,
test infrastructure gotchas, and StrictMode interaction.

Source files read: `web-app/src/components/sessions/XtermTerminal.tsx` (~L240-620),
`web-app/src/lib/hooks/useTerminalFlowControl.ts` (full), `web-app/src/lib/terminal/StateApplicator.ts`
(~L537-579), `web-app/src/components/sessions/TerminalOutput.tsx` (~L780-870),
`web-app/src/components/sessions/__tests__/XtermTerminalBug.test.tsx`,
`web-app/src/components/sessions/__tests__/XtermTerminalSelection.test.tsx`,
`web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`, `web-app/jest.setup.js`,
`web-app/jest.config.js`, `web-app/next.config.*`, `web-app/package.json`.

---

## 1. "Currently-applied cols/rows" — the AC2 comparison baseline

**Finding: use `terminal.cols`/`terminal.rows` (live xterm.js state), NOT a
"last successful fit()" ref.** They are provably NOT always the same value, and using a
fit()-only ref is the failure mode most likely to *silently swallow a legitimate resize*
(the AC6 risk).

### terminal.cols/rows is mutated by a second, independent authority: StateApplicator

`web-app/src/lib/terminal/StateApplicator.ts:551-578` (`applyDimensionChange`) calls
`this.terminal.resize(targetCols, targetRows)` directly whenever a server "state" message's
`dimensions` disagree with the client's current `terminal.rows/cols`:

```ts
if (currentRows !== targetRows || currentCols !== targetCols) {
  this.terminal.resize(targetCols, targetRows);   // mutates terminal.cols/rows — NOT via FitAddon
  ...
}
```

This is invoked from `handleStateMessage` in `useTerminalFlowControl.ts` (~L208-246), which
fires on **every** server state/resync message — including the `currentPaneRequest` response
that `resize()` itself triggers 100ms after sending a `TerminalResize` RPC (~L431-451), and
the `requestFullResync` path (dimension-mismatch handler, ~L154-170). In other words: **tmux
capture-pane resync is a real, already-wired code path that mutates `terminal.cols`/`rows`
independent of `fit()`.** This directly confirms the requirements doc's speculation.

### Why this matters for AC2

- `terminal.resize()` is xterm.js's core resize primitive; both `FitAddon.fit()` and
  `StateApplicator.applyDimensionChange()` funnel through it, and **both fire
  `terminal.onResize`** (confirmed: the existing `resizeDisposable` at
  `XtermTerminal.tsx:433` already dedupes by comparing to `lastSizeRef`, which is exactly a
  "live current state" ref updated by *any* resize source — this is the right pattern to
  extend, not replace).
- If the ResizeObserver's "currently-applied" baseline is instead a **fit()-only** ref
  (`lastFitDimensionsRef`, updated only inside the ResizeObserver's own debounced callback),
  it goes **stale** the moment `StateApplicator` resizes the terminal out from under it. Two
  bad outcomes follow:
  1. **False negative (AC6 violation):** container pixel size changes for real, but
     `proposeDimensions()` happens to equal the *stale* `lastFitDimensionsRef` (coincidentally
     matching the pre-StateApplicator-resize value) → `fit()` is skipped → terminal stays
     desynced from its container indefinitely until some other event forces a fit. This is a
     genuine silent-swallow scenario, not hypothetical.
  2. **Spurious fit() no worse than today, but wrong reason:** `proposeDimensions()` differs
     from the stale ref even though the container hasn't moved at all (only
     `StateApplicator` changed `terminal.cols/rows`) → an "unnecessary" fit fires to
     reconcile — which is actually *correct* behavior (container-vs-terminal really are out
     of sync) but happens for the wrong reason if the mental model is "ref = last fit."

Comparing against **live `terminal.cols`/`terminal.rows`** directly self-heals both cases
correctly: it always reflects reality regardless of which mechanism last touched it, and a
mismatch between "what the container can fit" and "what the terminal currently is" is exactly
the condition that should trigger a reconciling `fit()`.

### Compounding risk: two authorities can leapfrog each other

Because `StateApplicator.resize()` can set `terminal.cols/rows` to a value that does **not**
match the container's actual pixel capacity (e.g. server-side tmux state briefly disagreeing
with the client's viewport during a fast resize), and because that DOM mutation can itself
perturb layout enough to fire the container's `ResizeObserver` (scrollbar appearance/font
reflow), there is a real path where: `StateApplicator.resize()` → layout perturbation →
`ResizeObserver` fires → `fit()` reconciles back to container-correct size → `onResize` fires
→ RPC → server sends state → `StateApplicator.resize()` again → repeat. **The oscillation
detector must observe the funnel point that captures both trigger sources** (see §2).

### Also note: `dimensionSyncRef` is dead code

`useTerminalFlowControl.ts:71,125-128` declares `dimensionSyncRef` and writes to it in
`requestFullResync()`, but nothing in the codebase ever reads it (`grep` confirms zero other
references). Do not assume it currently serves as the "last sent cols/rows" tracker needed for
AC3 — it's write-only today. If repurposed, rename/re-document it; if a new ref is added for
AC3's "last SENT value" dedup, don't collide with this one's now-stale semantics.

---

## 2. Where to hook the oscillation detector — multiple fit() entry points exist

The requirements describe the loop as ResizeObserver → fit() → onResize → resize(), but
**`fitAddon.fit()` is called from at least six places in `XtermTerminal.tsx`**, not just the
ResizeObserver callback:

| Line | Trigger |
|---|---|
| 381 | Initial mount (double rAF) |
| 490 | ResizeObserver debounced callback |
| 572 | `fontSize` prop change effect |
| 581 | `fontFamily` prop change effect |
| 615 | Imperative `ref.fit()` (exposed via `useImperativeHandle`) |

The imperative `fit()` is called externally from `TerminalOutput.tsx`:
- L801 — on `isVisible` becoming true (session-switch in a pooled terminal), after a 50ms `setTimeout`
- L819 — on `visualViewport` `resize` (mobile on-screen keyboard show/hide), after 300-400ms,
  guarded by an ad hoc `isFittingRef` reentrancy flag ("iOS where fit() triggers another resize event" — an existing, independently-invented guard for the same class of bug this project is fixing)

**Implication:** if the burst-detector history buffer is scoped only inside the
ResizeObserver's closure (as its natural home would suggest), it will miss oscillations
triggered by the font-size/font-family effects or by `TerminalOutput`'s visibility/viewport
paths, and will not correctly attribute a `StateApplicator`-triggered mismatch loop (§1)
either — none of those go through the ResizeObserver's own debounce timer. The correct,
requirement-consistent funnel point is **`terminal.onResize`** (`XtermTerminal.tsx:433`),
since every resize source — `fit()` from any caller, and `StateApplicator.resize()` — routes
through it. Recording oscillation history there (keyed on the `{cols, rows}` payload actually
delivered) catches the whole class, not just the container-ResizeObserver slice of it.

Also note: `TerminalOutput.tsx`'s `isFittingRef` guard is a *reentrancy* guard (prevents a
fit-in-flight from re-triggering itself), which is a different mechanism from the *value*
oscillation detector this project adds. Both should probably coexist; don't assume one
subsumes the other, and don't let the new detector fight the existing guard's timing (300/400ms
vs the ResizeObserver's 150ms debounce vs the flow-control's 200ms throttle — three
independently-tuned timers already, none synchronized with each other).

Also: a second, independent `ResizeObserver` exists one level up, on the pane-split
container (`PaneSplitRenderer.tsx:330`, `useSplitContainerSize.ts:24`). Dragging a pane
divider (not just the browser window) resizes the terminal's container and is a second, very
plausible real-world trigger for the "legitimate multi-step resize during active drag"
scenario in §3 — arguably more common in this app than raw window-edge dragging, since this
is a multi-pane terminal manager.

---

## 3. Oscillation-detector false-positive risk (window/pane drag)

**Risk is real but has a clean discriminator: whether the (cols, rows) pair recurs
*non-consecutively/without settling* vs. *terminal set-then-immediately-superseded*.**

During an active OS-level drag-resize, the browser fires many `ResizeObserverEntry` updates
at intermediate pixel sizes before settling at a final size. Two sub-cases:

- **Continuous drag, monotonically changing size:** each intermediate pixel size generally
  proposes a *different* integer `cols`/`rows` (cell width is typically 7-12px, so most
  drag steps do cross a cell boundary) — these are legitimately different resizes and should
  each fit(). A naive "same cols/rows seen ≥3 times in 2000ms" counter would NOT false-trigger
  here because the values differ each time.
- **The actual danger case:** the user pauses mid-drag (common — people drag in small
  increments, or the OS coalesces rapid mousemove into bursts with brief holds), OR the
  window is resized to a size where several consecutive pixel deltas round to the **same**
  integer `cols`/`rows` (e.g. dragging from 802px→808px, all producing 100 cols if cell width
  is 8px). Both produce the *same* `{cols, rows}` value recurring multiple times within
  2000ms — which is legitimate, transient, and should NOT trigger canvas fallback, yet matches
  a naive "same value ≥3x in 2000ms" rule.

**The correct discriminator, per the requirements' own framing ("never settles" vs.
"eventually settles"), is idle/quiescence-based, not purely count-based:**
A real oscillation loop is characterized by the SAME `{cols, rows}` recurring **with no
intervening *different* value and no gap longer than the debounce+RPC-roundtrip window** —
i.e., the terminal keeps re-proposing/re-applying the identical dimensions back-to-back
indefinitely. A legitimate drag settling at value X, by contrast, has **one final occurrence
of X after which the stream goes idle** (no more resize activity at all, because the user
released the mouse and the container is stable) — X does not keep re-triggering *itself* in a
tight cycle; it simply persists (i.e., no further `fit()`/RPC happens because the guard from
AC2/AC3 correctly no-ops once cols/rows stop changing).

Concretely: if AC2 (skip fit() when `propose == currently-applied`) and AC3 (skip RPC when
`(cols,rows) == last-sent`) are implemented correctly, a settled drag naturally produces
**zero** repeat `fit()`/RPC calls after the first application of the final size — there is
nothing left to feed the oscillation counter. The burst detector's ≥3x-in-2000ms condition
should therefore be interpreted as counting **`fit()` calls that AC2's own guard did NOT
suppress** (i.e., cases where `proposeDimensions()` genuinely disagreed with current state
repeatedly, ping-ponging between candidate values, e.g. A→B→A→B or A→A→A because something
keeps re-perturbing the DOM after each fit) — NOT counting raw `ResizeObserverEntry` events or
raw `onResize` firings, which the drag scenario will produce many of even in the fully-fixed,
non-buggy steady state. **Wiring the detector to count post-AC2/AC3-dedup fit()/resize
applications (not raw observer callbacks) is the key design choice that avoids the window-drag
false positive** — this should be stated explicitly in the ADR (AC4 requires one) since it's
easy to get backwards (counting raw events is the natural-but-wrong first instinct).

Recommend the ADR also specify: reset the rolling window / history buffer once a `fit()`
genuinely changes `terminal.cols/rows` to a *new* value not previously seen in the current
burst — so a drag sequence like 80→85→90→90→90 (three real transitions, then settling with
two harmless repeats of 90 because AC2/AC3 already no-op them, so they wouldn't even reach
the detector) doesn't accumulate cross-transition count. This is really a corollary of hooking
the detector after AC2/AC3's own dedup (previous paragraph), not a separate mechanism.

---

## 4. WebGL addon disposal pitfalls

### Existing `onContextLoss` handler (XtermTerminal.tsx:269-272) already matches the xterm.js team's own "suboptimal but simple" documented pattern:
```ts
webglAddon.onContextLoss(() => {
  console.warn(...);
  webglAddon.dispose();
});
```
The xterm.js WebGL addon docs themselves describe exactly this (`addon.onContextLoss(e => addon.dispose())`) as the *simple-but-suboptimal* approach, and separately note a more complete pattern additionally loads an explicit fallback renderer addon. **This confirms the requirements doc's suspicion that "just calling dispose()" is the minimum viable response**, not a fully-robust one — natural context loss and an oscillation-triggered fallback can reasonably share this same minimal path, *if* the "canvas renderer" framing in AC4 is corrected (see next point — this is the highest-priority finding in this section).

### AC4 says "falls back to canvas renderer" — this is very likely NOT achievable as literally stated on this codebase's xterm.js version

- `web-app/package.json` pins `"@xterm/xterm": "^6.0.0"`.
- `@xterm/addon-canvas` — the actual package that provides a canvas-based renderer — **is
  deprecated and does not work with `@xterm/xterm` v6** (confirmed via the xtermjs project's
  own direction and corroborated by `cockpit-project/cockpit` issue #22509, "`@xterm/addon-canvas`
  deprecated and will be removed in `@xterm/xterm` v6," which describes the addon being
  dropped upstream and blocking consumers from upgrading to xterm.js 6.0.0).
- `web-app/package.json` has **no** `@xterm/addon-canvas` dependency today.
- What actually happens when `webglAddon.dispose()` is called and nothing else is loaded is
  that xterm.js falls back to its **default DOM renderer** (the DOM renderer, not a canvas
  renderer, has been the built-in default since xterm.js 5.x; the canvas renderer was moved
  out into the now-deprecated addon). This matches the requirements doc's own hedge
  ("is doing nothing after dispose() sufficient... leaves xterm.js on its default DOM/canvas
  renderer") — the answer is: it leaves it on the **DOM** renderer specifically, not canvas.

**Action needed before implementation, not after:** the ADR (required by AC4) must either (a)
correct AC4's language to "falls back to the DOM renderer (xterm.js's default when no
accelerated addon is loaded)" and just call `webglAddon.dispose()` with no further addon load
— which is trivial, safe, and matches the existing `onContextLoss` handler exactly — or (b)
explicitly decide to add `@xterm/addon-canvas` as a new dependency despite its deprecation and
incompatibility with xterm v6, which would likely require pinning to an older, unmaintained
version and accepting known breakage risk. **(a) is almost certainly the intended and correct
choice** given the existing code already does exactly this for natural context loss, but this
needs to be an explicit, written ADR decision, not an assumption baked into code, because it
contradicts the literal words of AC4 as written.

### Other known xterm.js WebGL/dispose bugs worth being aware of (found via search, not yet reproduced in this codebase)
- `xtermjs/xterm.js#5181` — race condition in `terminal.dispose()`: disposing the *terminal*
  while the WebGL addon is still attached can fail because addon disposal tries to recreate/attach
  a DOM renderer mid-teardown. Relevant if the oscillation fallback path and the component's
  unmount cleanup (`XtermTerminal.tsx:520`, `terminal.dispose()`) can race — e.g. an oscillation
  burst detected in the same tick the component is unmounting. Guard: check `terminalRef.current`
  is still non-null / not already disposed before calling `webglAddon.dispose()` from the new
  fallback path, mirroring the null-checks already used elsewhere in the ResizeObserver callback
  (`XtermTerminal.tsx:454`).
- `xtermjs/xterm.js#3889` — WebGL addon can leak GPU memory / textures if `dispose()` doesn't
  fully release GL resources in some versions. Not independently reproduced here; worth a
  manual check (repeated oscillation-triggered dispose across many terminals in one session,
  e.g. multi-terminal usage per the requirements' own problem statement) for GPU memory growth
  during the manual repro test (AC7).
- Double-dispose: calling `.dispose()` twice on the same `WebglAddon` instance is a plausible
  hazard if both the natural `onContextLoss` handler AND a new oscillation-triggered fallback
  path can independently decide to dispose the same addon instance (e.g. context loss immediately
  followed by an oscillation burst before local addon-loaded state is updated). Guard with a
  local `webglDisposedRef`/similar flag checked by both call sites, since `webglAddon` is a
  closure-local variable inside the async IIFE at `XtermTerminal.tsx:264-281` and isn't currently
  exposed via a ref the ResizeObserver callback (added elsewhere in the same effect) could
  reach — this is a wiring detail the implementation will need to solve regardless (the
  oscillation-triggered fallback code, likely inside/near the ResizeObserver closure, needs a
  reference to `webglAddon` which today only exists inside the separate async WebGL-loading
  IIFE). Recommend hoisting a `webglAddonRef` (parallel to `fitAddonRef`) so both the
  `onContextLoss` handler and the new oscillation path share one disposal guard.

---

## 5. Testing pitfalls

### ResizeObserver is not globally polyfilled in this test suite
`web-app/jest.setup.js` sets up only `TextEncoder`/`TextDecoder` and `@testing-library/jest-dom`
matchers — **no global `ResizeObserver` stub exists.** Each test file that needs to control
it defines its own `MockResizeObserver` class via `Object.defineProperty(global, 'ResizeObserver', ...)`
in a `beforeEach` (see `XtermTerminalBug.test.tsx:247-259` and `XtermTerminalSelection.test.tsx:249-...`).
Consequence: `XtermTerminal.test.tsx`'s three "renders without error" tests mount the **real**
`XtermTerminal` component with **no** `ResizeObserver` global defined at all. Since
`new ResizeObserver(...)` (`XtermTerminal.tsx:453`) sits inside the same broad
`try { ... } catch (error) { console.error(...); return; }` block that wraps the whole mount
effect (`XtermTerminal.tsx:284-535`), a `ReferenceError: ResizeObserver is not defined` in
jsdom is silently swallowed — the effect returns early, the component still renders its
container `<div>` fine, and the test passes despite the entire fit/resize wiring having failed
to install. **New tests for AC1/AC2/AC3/AC6 must explicitly stub `ResizeObserver`** (following
the existing per-file pattern) or they will silently no-op exactly like the pre-existing
smoke tests do. Also worth flagging to the team separately: the broad try/catch swallowing a
`ResizeObserver`-undefined error is itself a latent masking risk outside of tests too (very old
browsers / privacy-hardened browsers without `ResizeObserver`) — out of scope to fix here, but
don't design the new oscillation/fallback logic in a way that depends on this catch block
*not* firing, since it demonstrably can.

### `proposeDimensions()` mock is a fixed constant, decoupled from `fit()`'s effect on `terminal.cols/rows`
In `XtermTerminalBug.test.tsx:79-93`, the mocked `FitAddon.proposeDimensions()` always returns
`{ cols: 200, rows: 50 }` regardless of container size, and `fit()` doesn't actually update the
mock `Terminal.cols/rows` (`MockTerminal` in that file has no `resize()` method at all — only
public mutable `cols`/`rows` fields that nothing writes). AC2's logic (`fit()` only when
`proposeDimensions()` disagrees with `terminal.cols/rows`) is **fundamentally about the
interaction between these two values**, so the existing mock is too coarse to exercise it:
a new/expanded mock will need `proposeDimensions()` to be settable per-test AND `fit()`
(or a new `resize()` method) to actually mutate the mock terminal's `cols`/`rows`, so that
"propose == current → skip" and "propose != current → fit" can both be asserted. Plan to
extend `MockFitAddon`/`MockTerminal` rather than writing a parallel one, to avoid drift between
this test file and the new resize-convergence tests.

### Two conflicting rAF-mocking styles already coexist in this test file — a trap for new tests
`XtermTerminalBug.test.tsx` uses **both**:
1. A manual `jest.spyOn(window, 'requestAnimationFrame').mockImplementation(...)` + hand-rolled
   two-level `flush()` helper (`captureRaf()`, L129-147) for the double-rAF at mount, used with
   **real** timers.
2. `jest.useFakeTimers()` + `jest.advanceTimersByTime(20)` for the ResizeObserver's
   `setTimeout(..., debounceDelay)` → double-rAF chain (L277-323), relying on Jest's *modern*
   fake timers to also virtualize `requestAnimationFrame` (which they do, as an internal
   `setTimeout`-based shim) so `advanceTimersByTime`/`runAllTimers` flushes the nested rAFs too.

These two approaches are **not interchangeable** and mixing them in one test (e.g. using
`captureRaf()`'s manual spy while fake timers are also active) will double-mock
`requestAnimationFrame` and produce confusing failures. New tests exercising the
`setTimeout(150ms) → rAF → rAF → fit()` chain in the ResizeObserver path (needed for AC1/AC2/AC6)
should follow pattern (2) — `jest.useFakeTimers()` + `advanceTimersByTime`/`runAllTimers` — since
it's the pattern already proven against this exact nested-timer shape in this file (Bug 3 tests).
Do not introduce a third style. One more specific gotcha: `jest.advanceTimersByTime(N)` only
advances by `N` — if `N` is chosen to land exactly on the 150ms debounce boundary without
enough headroom for the two chained rAF ticks Jest schedules afterward, assertions can flake
depending on Jest's internal rAF-tick granularity; prefer `jest.runAllTimers()` (as the existing
Bug 3 "happy path" test does at L288) when the goal is "let the whole chain settle," reserving
`advanceTimersByTime` for tests that specifically assert *nothing* has happened yet at a
partial-elapsed checkpoint (as the existing L297/L315 tests do, checking pre-debounce state).

### `useTerminalFlowControl.test.ts` already uses fake timers for the 200ms RPC throttle
(`L93-100`, `L174`) — the new AC3 "same value skip" logic sits right next to this existing
throttle logic and should be tested in the same file/style; no new timer-mocking pattern needed
there, just new assertions.

---

## 6. React StrictMode / double-effect-invocation risk

`web-app/next.config.*` sets `reactStrictMode: true` (line 15), so in dev builds React 18
double-invokes the mount effect (mount → synchronous cleanup → mount again) for every
component instance, including `XtermTerminal`'s main effect (`XtermTerminal.tsx:240-539`,
keyed on `[scrollback]`).

**This is lower risk than it first appears, IF the oscillation-history buffer is declared
correctly, but the placement choice matters:**

- If the history buffer is a plain variable declared **inside** the effect body (the same
  pattern already used for `lastContainerSize`/`resizeTimeout`, `XtermTerminal.tsx:451-452`),
  StrictMode's double-invoke is harmless: the first (throwaway) mount's `ResizeObserver` is
  synchronously `disconnect()`-ed during cleanup before any real async work (debounce
  timers, rAFs) has had a chance to fire, since React's dev double-invoke happens synchronously
  in the same tick, before any `setTimeout`/`requestAnimationFrame` from the first mount can
  execute. So a same-tick throwaway mount cannot pollute the second (real) mount's buffer if the
  buffer lives inside the effect closure and is re-created fresh each time the effect runs.
- **The risk is if the buffer is instead hoisted to a `useRef` at the component's top level**
  (outside the `[scrollback]` effect) for convenience/persistence-across-renders. A top-level
  ref survives StrictMode's double-invoke fine (that's expected/correct for refs generally),
  but it will **also survive across a real `[scrollback]`-triggered effect re-run**, which
  fully tears down and recreates the terminal/fitAddon/ResizeObserver
  (`XtermTerminal.tsx:511-525` cleanup, then lines 240+ re-run). A top-level ref would carry
  stale oscillation-burst history from the *old* terminal/container instance across the
  transition to a completely new one, which could cause a spurious immediate canvas-fallback
  trigger right after a `scrollback` prop change if the old instance's history happened to be
  close to the ≥3x threshold at teardown time. **Recommendation: keep the oscillation-history
  buffer scoped inside the effect body (module-local `let`/array, matching the existing
  `lastContainerSize`/`resizeTimeout` pattern), not a component-level `useRef`,** so it's
  naturally reset on every real effect re-run (both StrictMode's harmless throwaway mount and
  genuine `scrollback`-driven recreation) with no special-case reset code needed.
- Tests won't catch this by default: none of the existing test files wrap `render()` in
  `<React.StrictMode>`, so this interaction is invisible to the existing suite. If the ADR/plan
  wants explicit regression coverage for the StrictMode double-mount case, a new test wrapping
  `render(<React.StrictMode><XtermTerminal .../></React.StrictMode>)` and asserting the
  oscillation counter starts at zero / doesn't false-trigger after the throwaway mount would be
  the way to add it — not currently present anywhere in the suite.

---

## Summary of highest-priority findings for the plan/ADR phase

1. **Compare `proposeDimensions()` against live `terminal.cols`/`terminal.rows`, not a
   fit()-only ref** — `StateApplicator.applyDimensionChange()` (`StateApplicator.ts:562`)
   already mutates `terminal.cols/rows` via `terminal.resize()` independent of `fit()`, and is
   wired into the live resync/state-message path today. A fit()-only ref will go stale against
   this and can silently swallow legitimate resizes (AC6 violation).
2. **AC4's "canvas renderer" is likely unimplementable as literally worded** —
   `@xterm/addon-canvas` doesn't support `@xterm/xterm` v6 (this repo's pinned version) and
   isn't a dependency today; disposing the WebGL addon actually falls back to xterm.js's
   default **DOM** renderer. The required ADR must explicitly correct/decide this before
   implementation, not leave it implicit.
3. **Hook the oscillation detector at `terminal.onResize` (the common funnel), count only
   fit()/resize applications that survive AC2/AC3's own dedup (not raw ResizeObserver/onResize
   events)** — `fit()` is called from 5+ sites beyond the ResizeObserver callback (font-size/
   font-family effects, imperative `ref.fit()` from `TerminalOutput.tsx` on visibility and
   mobile-keyboard viewport changes), and counting raw drag-driven events (rather than
   post-dedup applications) is the specific design mistake that would false-trigger canvas
   fallback during ordinary window/pane-divider dragging.
