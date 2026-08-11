# Research: Feature Landscape — Terminal Resize/Fit Feedback Loop Fix

Agent 2 (Features), Phase 2 research. Repo: stapler-squad web-app.

## 1. Edge cases beyond the listed ACs

### 1.1 Font-size change mid-jitter — real interaction, needs guarding
`XtermTerminal.tsx` has a separate `useEffect` (lines ~343-349) that reacts to `fontSize`
prop changes independent of the mount effect:
```tsx
useEffect(() => {
  if (terminalRef.current && terminalRef.current.options.fontSize !== fontSize) {
    terminalRef.current.options.fontSize = fontSize;
    // Defer fit to avoid synchronous resize events
    setTimeout(() => fitAddonRef.current?.fit(), 0);
  }
}, [fontSize]);
```
This calls `fitAddon.fit()` **directly**, bypassing the `ResizeObserver` callback entirely —
so any dead-band/2-tick-confirm logic added inside the `ResizeObserver` handler does **not**
protect this path. `fit()` here changes `terminal.cols/rows`, which fires `terminal.onResize`,
which calls `onResizeRef.current(cols, rows)` → parent's `handleTerminalResize` → `resize()`.
If the *value-dedup* in `resize()` (AC3) is keyed on last-sent (cols, rows), this path is
already covered by that dedup — but the **container** itself doesn't change size when font
size changes, so after `fit()` recalculates cols/rows, the `ResizeObserver` should *not* fire
again (no container size change) — so this is not a loop risk today. It IS a risk if a future
change makes the container's size also depend on content (e.g. an auto-sizing wrapper), which
would create a real cross-trigger between the font-size effect and the ResizeObserver. Recommend:
note this path explicitly in the plan/tests as "second caller of `fit()` outside the
ResizeObserver", verify it still respects the value-dedup in `resize()`, and that its own
`fitAddonRef.current?.fit()` call doesn't need dead-banding (it's not itself an observer
callback that can re-trigger from its own output).

### 1.2 Terminal unmounted mid-debounce-timer — already handled correctly
The mount effect's cleanup (lines 299-312) already does:
```tsx
return () => {
  if (resizeTimeout) clearTimeout(resizeTimeout);
  resizeObserver.disconnect();
  ...
  terminal.dispose();
  terminalRef.current = null;
  fitAddonRef.current = null;
  ...
};
```
So a pending `setTimeout` from the adaptive debounce (10ms/250ms) is cleared on unmount, and
the observer is disconnected before dispose. **No new edge case here** — but any new dead-band
state (e.g. a "tick counter" or "last confirmed dims" ref for the 2-tick-confirm rule) must be
reset/cleared in this same cleanup path, otherwise a fast unmount+remount (e.g. React
StrictMode double-invoke in dev, or a session-switch that remounts `XtermTerminal` via the
`[scrollback]` dependency) could carry stale tick state into a new terminal instance if the
refs are hoisted outside the effect. Recommend keeping new dead-band state as local closures
inside the effect (as `lastContainerSize`/`resizeCount` already are) rather than component-level
refs, so a fresh effect run gets a fresh closure automatically.

Also note: the effect's dependency array is `[scrollback]` — so `XtermTerminal` is **fully
recreated** (new `Terminal`, new `ResizeObserver`) whenever `scrollback` prop changes, which
is an existing possible source of a fresh ResizeObserver firing an initial resize while an old
one's debounce timer is still pending — but cleanup runs first (effect cleanup always runs
before the next effect body on a dep change), so this is safe today.

### 1.3 Multiple terminals resizing near-simultaneously — NOT a real mechanism here
Traced the render tree: `XtermTerminal` is only ever rendered by `TerminalOutput.tsx`, which is
only ever rendered by `SessionDetail.tsx` (`web-app/src/components/sessions/SessionDetail.tsx`),
and `SessionDetail`'s tab content uses conditional rendering (`{activeTab === "terminal" && (...)}
`) — only one `TerminalOutput`/`XtermTerminal` is mounted **per SessionDetail instance**, and
grep across `web-app/src/app/**` (`page.tsx`, `review-queue/page.tsx`) shows only ever a single
`<SessionDetail>` mounted at a time (a single detail pane or a single modal). There is no
grid/split view, no tiled-pane layout, and no code path where two `XtermTerminal` instances
share a flex/grid parent whose size depends on sibling content. This confirms the requirements
doc's assumption: **the "3 concurrently-mounted terminals" in AC1 can only mean 3 separate
browser tabs/windows, each with its own independent DOM tree and its own `SessionDetail`** —
not sibling panes in one layout. No cross-pane lockstep-amplification mechanism exists in this
codebase to design against. Do not build defenses for a shared-parent scenario; it would be
speculative. (If a tiled/split layout is added later, this section should be revisited.)

### 1.4 Fractional cols/rows from `proposeDimensions()` — never happens
`@xterm/addon-fit` is pinned at `^0.10.0` (`web-app/package.json`). The package's
`FitAddon.proposeDimensions()` implementation (well-known, stable across 0.x) computes:
```js
cols: Math.max(MINIMUM_COLS, Math.floor(availableWidth / dims.css.cell.width)),
rows: Math.max(MINIMUM_ROWS, Math.floor(availableHeight / dims.css.cell.height))
```
i.e. it always `Math.floor()`s to integers before returning. **`proposeDimensions()` never
returns fractional cols/rows** — AC2's "integer cols/rows" framing is accurate/redundant with
upstream behavior, not an extra guard needed against the library. (Could not verify directly
against installed `node_modules` in this worktree — package is not installed on disk here —
but this is standard, documented `xterm-addon-fit` behavior for all 0.x releases including the
pinned `^0.10.0`.) No special fractional-handling code is needed; `!==` integer comparison in
the dead-band check is sufficient.

### 1.5 First fit after mount — should NOT get the 2-tick-confirm gate
The mount effect already fits explicitly and deterministically outside the `ResizeObserver`:
a double-`requestAnimationFrame` initial `fit()` (lines 163-208), followed by a **second**
explicit `fit()` 100ms later ("Force one more fit after a short delay to ensure accurate
sizing"). Both of these are direct `fitAddon.fit()` calls, not `ResizeObserver`-driven, so the
2-tick-confirm rule (which the plan should scope to the `ResizeObserver` callback only) doesn't
apply to them today and shouldn't be extended to them — doing so would break the existing
"ultra-fast initial sizing" intent and likely regress AC1's own reference to fast initial
layout. The `ResizeObserver`'s existing adaptive debounce (10ms for the first 3 observed
resizes, 250ms after) is a *separate, complementary* mechanism to the 2-tick-confirm value
check being added — the two should compose (debounce delay before scheduling, dead-band+2-tick
gate before actually calling `fit()`), not replace one another. Recommend: keep the
resizeCount<=3 fast-path for scheduling cadence, but still require the integer cols/rows
dead-band + 2-consecutive-tick check before firing `fit()`, even during the first 3 resizes —
because the double-RAF + secondary-100ms-fit already handles "fast initial sizing" outside the
observer; the observer's job during startup is just to catch genuine late layout shifts (e.g.
web font loading, flexbox reflow after sibling panels render), which is exactly the class of
sub-pixel jitter the fix targets.

## 2. Other ResizeObserver / fit-loop-prone patterns in the codebase

Full-repo grep for `ResizeObserver` under `web-app/src` returns exactly **two** hits:

1. `web-app/src/components/sessions/XtermTerminal.tsx` — the production code covered above.
2. `web-app/src/app/test/terminal-stress/page.tsx` (lines 136-139) — a Playwright-facing stress
   test harness with its own independent `Terminal`/`FitAddon`/`WebglAddon` setup:
   ```tsx
   const resizeObserver = new ResizeObserver(() => {
     fit.fit();
   });
   resizeObserver.observe(terminalRef.current);
   ```
   This is the **anti-pattern in its purest form**: no size-delta check, no debounce, no
   value-dedup — every single `ResizeObserverEntry` callback unconditionally calls `fit()`.
   It's out of scope for this fix (the requirements only name `XtermTerminal.tsx`,
   `useTerminalFlowControl.ts`, `TerminalOutput.tsx`), and being a test/stress harness under
   `app/test/`, it's plausibly *intentionally* unthrottled to stress-test worst-case render
   load. Still worth flagging to the planning phase as a candidate for a follow-up cleanup
   once the production dead-band pattern is proven, so the two don't visibly diverge as
   "the right way" vs "the way nobody bothered to fix." Not recommended as in-scope work now.

No other `ResizeObserver`, `useResizeObserver`, or resizable-panel library (react-resizable,
react-resizable-panels, `Panel`/`onResize` grid systems) usages exist elsewhere in `web-app/src`
— so there's no other established "dead-band" convention borrowed from a sibling feature.

The nearest genuine convention for *this specific problem shape* (an optional bypass flag on a
throttled dispatch function) already exists in the **same file** (`useTerminalFlowControl.ts`):
`requestFullResync(urgent: boolean = false)` (lines 94-111) — an optional boolean parameter
that, when `true`, skips the 2-second time-based throttle entirely:
```ts
const requestFullResync = useCallback((urgent: boolean = false) => {
  ...
  const RESYNC_THROTTLE_MS = 2000;
  if (!urgent && timeSinceLastResync < RESYNC_THROTTLE_MS && lastResyncTimeRef.current !== 0) {
    console.log(`... Resync throttled ...`);
    return;
  }
  ...
```
There is already a passing regression test for this exact bypass shape:
`web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` → `describe('requestFullResync')
→ it('should allow urgent resync to bypass throttle')`. This is the strongest available
precedent for how AC4's `force: true` parameter on `resize()` should be named, typed, and
tested — mirror `requestFullResync`'s `urgent` pattern (optional 3rd param defaulting to
`false`, an early-return guard that becomes a no-op when the flag is true) rather than
inventing a new convention. Existing test file structure (`describe('resize')` block, lines
126-177) is the natural place to add `force`-bypass test cases, consistent with the
`describe('requestFullResync')` `urgent`-bypass test right below it.

## 3. Unstated needs and testable thresholds

### 3.1 Should `force: true` bypass the RPC-level rate limit (200ms throttle) entirely, or only value-dedup?
The `requestFullResync(urgent)` precedent (see §2) answers this directly by example: `urgent`
bypasses the **time-based throttle** entirely (not just some inner dedup — there's no separate
dedup in `requestFullResync`, only the throttle). Applying the same convention to `resize()`:
`force: true` should bypass **both** the 200ms time throttle (existing) and the new value-dedup
check (AC3) — i.e. `force: true` should unconditionally send the RPC. This matches why the two
named call sites need it: reconnect-resync (server may have restarted/lost state, so even an
unchanged cols/rows pair must be re-sent to force a fresh `currentPaneRequest`) and the manual
"Fit" button (user explicitly asked for a refresh — respecting a stale dedup cache here would
be user-visibly broken, e.g. "I clicked Fit and nothing happened"). Recommend: `force` bypasses
throttle + dedup together, single flag, no plan to split them into two flags — no evidence in
the codebase or requirements that a "bypass dedup but still throttle" state is needed, and
splitting adds complexity/API surface without a stated use case.

### 3.2 What does "settles to zero within a bounded number of debounce cycles" mean as a testable threshold?
Concrete proposal, informed by the existing adaptive debounce constants (`resizeCount <= 3 ? 10
: 250` ms) and the 200ms RPC throttle:
- **Unit-test level** (fake timers, `useTerminalFlowControl` / `XtermTerminal` in isolation):
  simulate N consecutive `ResizeObserver` callback firings with the *same* integer
  cols/rows (the jitter case — sub-pixel container changes that floor to the same value).
  Assert `fitAddon.fit()` is called **at most once** (the 2-tick-confirm should converge and
  then produce zero further `fit()` calls once cols/rows stop changing) — i.e. after the first
  confirmed application, additional identical-dimension `ResizeObserverEntry` callbacks must
  not schedule new timers or call `fit()` again. A "bounded number of cycles" test should
  assert convergence within **at most 2 ResizeObserver callback ticks** (matching the 2-tick-
  confirm rule itself) — not an open-ended "eventually stops" assertion, which is untestable
  without a timeout/flake risk.
- **RPC level**: simulate the same jitter feeding into `resize()` repeatedly with an unchanged
  (cols, rows) pair; assert `pushMessage` (the RPC send) is called **exactly once** total,
  regardless of how many times `resize()` is invoked with the same values — this is the direct
  testable form of AC3 ("skips sending the RPC when incoming (cols, rows) equals last pair
  actually sent").
- **Boundary-flapping case** (dims oscillate between two values across ticks, e.g. 80x24 →
  79x24 → 80x24 → ...): the 2-tick-confirm rule as literally stated in AC2 ("repeat on two
  consecutive ticks") means a value must appear twice *in a row* before it's applied — an
  oscillation that never repeats twice consecutively should converge to **zero** additional
  `fit()` calls after the initial applied size, i.e. the test should assert no unbounded growth
  in `fit()` call count across an arbitrarily long injected oscillation sequence (e.g. 50 ticks
  → still only 1 `fit()` call, not 50 and not even 25).
- **Manual/E2E proxy** (AC7): since there's no literal tiled layout, "concurrent instances" for
  manual verification should mean 3 separate browser tabs each on a different session's
  `SessionDetail`, left open through a tab background/resume cycle or one `window.resize`, with
  CPU profiling (`chrome://tracing` or the app's own `--profile --trace` per
  `docs/PROFILING.md`) showing `fit()`/RPC-send call counts settling to 0 within roughly 1
  second of the triggering event, not continuing indefinitely. This is a manual/perf proxy, not
  a unit test, and should be documented as such rather than automated.

### 3.3 Other unstated needs surfaced during research
- **WebGL fallback must be added net-new to production code** — `XtermTerminal.tsx` currently
  has no `contextLost` handler and no persisted ref to the `WebglAddon` instance (it's a local
  `const webglAddon` inside a `try` block, never stored for later `dispose()`); AC5 requires
  adding both. A working reference pattern already exists in the test harness
  (`web-app/src/app/test/terminal-stress/page.tsx` lines ~108-113):
  ```tsx
  webgl.onContextLoss(() => {
    console.warn('WebGL context lost, falling back to canvas');
    webgl.dispose();
    setRendererType('canvas');
  });
  ```
  but AC5 is about a *sustained px/col mismatch* (not a hard context-loss event), so the
  production fix needs new logic — track consecutive/sustained mismatch beyond tolerance (not
  just the one-shot mismatch log that already exists at lines 188-197) and then `dispose()` the
  `WebglAddon` and let the terminal fall back to its default (canvas) renderer. This ties back
  to `Number.isFinite` guards per AC5 — `actualPixelsPerCol` is a division
  (`rect.width / terminal.cols`) that can produce `Infinity`/`NaN` if `terminal.cols` is 0 or
  `rect.width` is 0 during a transient layout state (e.g. tab backgrounded via
  `visibility: hidden`, which can zero out `getBoundingClientRect()` in some browsers) — the
  existing code at line 189 already guards `terminal.cols > 0` but does not guard
  `Number.isFinite(actualPixelsPerCol)` before the comparison at line 194; that gap should be
  closed since AC5 explicitly calls for `Number.isFinite` guards.
- **`force: true` regression test should assert the literal third argument** (AC4 is explicit
  about this) — meaning the test should inspect the actual call site source/behavior (e.g. via
  a spy on `resize` asserting `resize).toHaveBeenCalledWith(cols, rows, true)`), not just that
  the RPC eventually got sent. This is a stronger assertion than end-to-end behavior testing
  and should be called out explicitly in the validate/plan phase so it isn't watered down to a
  looser "was the RPC sent" check during implementation.
