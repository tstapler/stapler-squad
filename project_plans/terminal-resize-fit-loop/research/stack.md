# Stack Research: Terminal Resize/Fit Feedback Loop Convergence

Agent 1 (Stack) — Phase 2 research for `terminal-resize-fit-loop`.

## 1. Current package versions (already at latest)

From `web-app/package.json` / `web-app/package-lock.json` (no `node_modules` installed in this
worktree — versions confirmed via lockfile, resolved versions match semver ranges exactly):

| Package | package.json range | Locked/resolved | npm `latest` (checked live) |
|---|---|---|---|
| `@xterm/xterm` | `^6.0.0` | `6.0.0` | `6.0.0` |
| `@xterm/addon-fit` | `^0.11.0` | `0.11.0` | `0.11.0` |
| `@xterm/addon-webgl` | `^0.19.0` | `0.19.0` | `0.19.0` |

**Implication:** there is no version bump available that changes `FitAddon.proposeDimensions()`
or `WebglAddon` disposal behavior — the repo is already on the newest release of all three
packages. Any fix must be implemented in application code (`XtermTerminal.tsx`,
`useTerminalFlowControl.ts`), not obtained via a dependency upgrade.

## 2. `FitAddon.proposeDimensions()` — exact contract (v0.11.0 source)

Fetched directly from `unpkg.com/@xterm/addon-fit@0.11.0/src/FitAddon.ts` (the shipped version):

```ts
const MINIMUM_COLS = 2;
const MINIMUM_ROWS = 1;

public proposeDimensions(): ITerminalDimensions | undefined {
  if (!this._terminal) return undefined;
  if (!this._terminal.element || !this._terminal.element.parentElement) return undefined;

  const core = (this._terminal as any)._core;
  const dims: IRenderDimensions = core._renderService.dimensions;

  if (dims.css.cell.width === 0 || dims.css.cell.height === 0) return undefined;

  const scrollbarWidth = (this._terminal.options.scrollback === 0
    ? 0
    : (this._terminal.options.overviewRuler?.width || ViewportConstants.DEFAULT_SCROLL_BAR_WIDTH));

  const parentElementStyle = window.getComputedStyle(this._terminal.element.parentElement);
  const parentElementHeight = parseInt(parentElementStyle.getPropertyValue('height'));
  const parentElementWidth = Math.max(0, parseInt(parentElementStyle.getPropertyValue('width')));
  const elementStyle = window.getComputedStyle(this._terminal.element);
  const elementPadding = { top: ..., bottom: ..., right: ..., left: ... }; // parsed from computed style
  const availableHeight = parentElementHeight - (elementPadding.top + elementPadding.bottom);
  const availableWidth = parentElementWidth - (elementPadding.right + elementPadding.left) - scrollbarWidth;

  return {
    cols: Math.max(MINIMUM_COLS, Math.floor(availableWidth / dims.css.cell.width)),
    rows: Math.max(MINIMUM_ROWS, Math.floor(availableHeight / dims.css.cell.height)),
  };
}

public fit(): void {
  const dims = this.proposeDimensions();
  if (!dims || !this._terminal || isNaN(dims.cols) || isNaN(dims.rows)) return;
  const core = (this._terminal as any)._core;
  if (this._terminal.rows !== dims.rows || this._terminal.cols !== dims.cols) {
    core._renderService.clear();
    this._terminal.resize(dims.cols, dims.rows);
  }
}
```

Key facts for the implementation plan:

- **Returns `undefined` when:** the terminal has no `_terminal`, no `.element`, no
  `.element.parentElement` (not yet mounted/attached to DOM), or when `dims.css.cell.width === 0
  || dims.css.cell.height === 0` (font metrics not measured yet — the exact race the existing
  `[XtermTerminal] Cell dimensions not available yet!` console warning at line 378 is guarding
  against).
- **cols/rows are always integers** — `Math.floor(availableWidth / cellWidth)`. This is the
  authoritative signal AC2 wants: gate `fit()` scheduling on whether `proposeDimensions().cols`/
  `.rows` differ from the terminal's *currently applied* `terminal.cols`/`terminal.rows` (or a
  locally-tracked "last scheduled" value), not on raw container pixel deltas.
- **`fit()` itself already no-ops when cols/rows are unchanged** (`if (this._terminal.rows !==
  dims.rows || this._terminal.cols !== dims.cols)`) — so calling `fitAddon.fit()` redundantly is
  cheap in isolation, but the surrounding `XtermTerminal.tsx` code still does console logging,
  `getBoundingClientRect()` reads, and (via `terminal.onResize`) can still re-trigger the resize
  RPC path if `fit()` *does* change dims marginally due to the WebGL sub-pixel wobble described in
  the requirements — so gating **before** calling `fit()` (per AC2) is still valuable to avoid
  wasted `_renderService.dimensions` reads and forced-clear/resize cycles, and is a much cheaper
  check than calling into xterm internals every ResizeObserver tick.
- `proposeDimensions()` reads `window.getComputedStyle` on both the terminal element and its
  parent — this is what a test needs to mock (JSDOM's `getComputedStyle` returns real but
  zero-valued layout by default, so component tests already stub `FitAddon` entirely — see §5).
- No `undefined`-cols/rows case exists once cell metrics are available; `NaN` is only possible via
  the private `_core._renderService.dimensions` object being malformed, which `fit()` also already
  guards against (`isNaN(dims.cols) || isNaN(dims.rows)`).

## 3. `WebglAddon` disposal / fallback (v0.19.0 source)

Fetched from `unpkg.com/@xterm/addon-webgl@0.19.0/src/WebglAddon.ts`:

```ts
export class WebglAddon extends Disposable implements ITerminalAddon, IWebglApi {
  private _onContextLoss = this._register(new Emitter<void>());
  public readonly onContextLoss = this._onContextLoss.event;

  public activate(terminal: Terminal): void {
    ...
    const renderService: IRenderService = unsafeCore._renderService;
    this._renderer = this._register(new WebglRenderer(...));
    this._register(Event.forward(this._renderer.onContextLoss, this._onContextLoss));
    renderService.setRenderer(this._renderer);   // <-- installs WebGL renderer

    // Registered as a disposable teardown callback — runs when dispose() is called:
    this._register(toDisposable(() => {
      if ((this._terminal as any)._core._store._isDisposed) return;
      const renderService: IRenderService = (this._terminal as any)._core._renderService;
      renderService.setRenderer((this._terminal as any)._core._createRenderer()); // <-- default (canvas) renderer
      renderService.handleResize(terminal.cols, terminal.rows);
    }));
  }
}
```

**Key finding — this directly resolves AC4's design question:**

- xterm.js does **not** need a "live renderer swap" API distinct from addon lifecycle. Calling
  `webglAddon.dispose()` is *itself* the fallback mechanism: the addon's own disposable teardown
  calls `renderService.setRenderer(core._createRenderer())`, which re-installs whatever
  renderer `_createRenderer()` returns by default — the canvas renderer — and then calls
  `renderService.handleResize(cols, rows)` to force a re-render at current dimensions.
- `onContextLoss` fires (from `WebglRenderer`) when the browser's WebGL context is lost (e.g. GPU
  driver reset, too many contexts, tab backgrounding on some platforms) — **it does not fire for
  ordinary resize/fit churn**. `XtermTerminal.tsx` (lines 269–272) already wires
  `webglAddon.onContextLoss(() => webglAddon.dispose())`, which is the *browser-driven* context-loss
  fallback, already correct and doesn't need to change.
- What AC4 actually needs is a **new, separate trigger**: an oscillation/refit-burst detector
  (same cols/rows recurring ≥3× within a rolling 2000ms window) that calls `webglAddon.dispose()`
  *proactively*, i.e. treating pathological resize oscillation as a signal to abandon WebGL even
  though the GPU context itself is fine. This means the fix needs to keep a reference to the
  currently-loaded `WebglAddon` instance (it's currently a function-local `const webglAddon`
  inside the async IIFE at lines 264–281 — not stored in a ref) so the oscillation detector,
  which lives in the `ResizeObserver` closure further down the effect, can reach it and call
  `.dispose()`. This will require introducing e.g. a `webglAddonRef` alongside the existing
  `fitAddonRef`/`terminalRef` pattern.
- **Backstop case (AC4):** if the oscillation detector fires but no `WebglAddon` was ever
  successfully loaded (mobile/no-WebGL2 path at line 278–280, or the `catch` branch at line
  275–277), `webglAddonRef.current` will be `null`/`undefined` — the detector must
  `console.error(...)` in that branch per the acceptance criterion, since there is nothing to
  dispose and canvas is presumably already active (so the oscillation has a different root cause
  worth flagging distinctly rather than silently swallowing).
- `dispose()` is idempotent/safe to call multiple times in principle (guarded by the `Disposable`
  base class's internal disposed-flag, standard for this codebase's `vs/base/common/lifecycle`
  pattern used throughout xterm.js internals) — but the existing `onContextLoss` handler could
  race with the new oscillation-burst handler both calling `dispose()` on the same addon; the
  plan should have the ref set to `null` after disposal so either path can check
  `if (webglAddonRef.current) { webglAddonRef.current.dispose(); webglAddonRef.current = null; }`
  to avoid a double-dispose depending on `Disposable`'s exact re-entrancy guarantees (not verified
  from source — treat double-dispose as a defensive-coding concern rather than a confirmed bug).

## 4. ResizeObserver feedback-loop best practices

The browser's own `ResizeObserver loop limit exceeded` (Chrome) / `ResizeObserver loop completed
with undelivered notifications` (Firefox/others) error is a distinct, lower-level mechanism from
the bug described in requirements.md — it fires when a *single* observer callback triggers a
layout change that would require redelivering notifications within the same frame, and the
browser silently defers/drops the notification (it's not user-actionable and commonly suppressed
in error monitoring). It is **not** what's causing the CPU-pegging bug here; the requirements'
loop is an *application-level* debounce/dedup failure (the observer keeps firing legitimately on
each real DOM mutation caused by `fit()`/scrollbar changes), not the browser's built-in loop
guard tripping.

Established patterns for avoiding the app-level version of this problem (corroborated via search,
consistent with common practice — see Sources):

1. **Debounce/rAF before acting** — already present in `XtermTerminal.tsx` (150ms `setTimeout` +
   double `requestAnimationFrame`), which is a reasonable pattern; the gap is what happens *after*
   the debounce fires, not the debounce mechanism itself.
2. **Gate on the semantically-meaningful output value, not the raw trigger** — this is exactly
   AC2: don't re-fit because pixels changed by >1px, re-fit because `proposeDimensions()` produces
   different integer cols/rows than what's currently applied. This eliminates sub-cell/sub-pixel
   wobble as a re-trigger source at the root, independent of how good the debounce is.
2b. Corollary used elsewhere in this codebase already (line 491–497): re-sync the tracked
   "last observed size" to the *post-fit* DOM size before the next observer tick, specifically to
   suppress the observer's own self-triggered notification (fit() resizing the terminal element →
   observer fires again). This same idea generalizes to cols/rows: after a fit, the "last applied
   cols/rows" the gate compares against should be updated so the next observer tick (even if it
   fires) is filtered before touching `proposeDimensions()`/`fit()` again.
3. **Keep the observer callback itself cheap** — read-only (measure), never write layout
   synchronously inside the callback; the existing code already only schedules work.
4. **Disconnect on unmount** — already done in this file's cleanup return.
5. **Burst/oscillation detection as a circuit breaker** — not a standard browser-API pattern, this
   is bespoke to this bug (AC4): track a rolling window of "resolved cols/rows" values over the
   last 2000ms; if the *same* (cols, rows) pair — or an alternating pair — recurs ≥3 times, that's
   evidence the fit/measure loop itself is unstable (e.g. WebGL glyph metrics vs. CSS cell metrics
   disagreeing by a fraction of a pixel, causing `proposeDimensions()` to alternate between two
   adjacent integer values), and the correct circuit-breaker response per AC4 is to fall back to
   canvas rendering (§3) rather than trying to further tighten the pixel/integer gate.

## 5. Testing: simulating `ResizeObserver` + `FitAddon` in JSDOM/Jest

JSDOM has no native `ResizeObserver` implementation, so every test file that needs one defines its
own mock/stub — there is **no shared global stub** in `web-app/jest.setup.js` (confirmed: that
file only polyfills `TextEncoder`/`TextDecoder` and loads `@testing-library/jest-dom`; 9 lines
total, no ResizeObserver reference).

### Existing pattern (`XtermTerminalBug.test.tsx`) — the template to extend

- **`jest.mock('@xterm/xterm', ...)`** builds a `MockTerminal` class and stashes a shared
  `harness` object (`fitCalledCount`, `onResizeCb`, `triggerFit()`, `reset()`) as
  `(MockTerminal as any).__harness`, retrievable in tests via
  `(require('@xterm/xterm').Terminal as any).__harness` (there's a local `getHarness()` helper —
  not shown in the excerpt read but referenced at line 278/306).
- **`jest.mock('@xterm/addon-fit', ...)`** provides a `MockFitAddon` whose `fit()` increments
  `harness.fitCalledCount` and calls `harness.triggerFit()` (which invokes the captured
  `onResize` callback with `{ cols: 200, rows: 50 }` by default) — `proposeDimensions()` is
  stubbed to always return `{ cols: 200, rows: 50 }` (a **constant**, not currently wired to
  respond differently per test case — this will need to become configurable/dynamic to write
  the AC2/AC5 sub-cell-resize test, since the new gating logic depends on
  `proposeDimensions()` returning *different* values across calls to distinguish "real change"
  from "sub-cell noise").
- **`jest.mock('@xterm/addon-webgl', ...)`** stubs `WebglAddon` with no-op `onContextLoss()` /
  `dispose()` — will need extending to make `dispose()` observable (e.g. `jest.fn()`) so a new
  oscillation-detector test can assert it was called, and to make the mock instance retrievable
  (harness pattern) similar to the Terminal/FitAddon harness.
- **ResizeObserver mock** is defined per-`describe` block via
  `Object.defineProperty(global, 'ResizeObserver', { writable: true, configurable: true, value: class MockResizeObserver { constructor(cb) { observerCallback = cb; } observe(){} unobserve(){} disconnect(){} } })`,
  capturing the callback in a closure variable (`observerCallback`), then a local
  `fireResizeObserver(width, height)` helper constructs a `ResizeObserverEntry`-shaped object
  (`contentRect`, `borderBoxSize: []`, `contentBoxSize: []`, `devicePixelContentBoxSize: []`,
  `target`) and invokes `observerCallback([entry], {} as ResizeObserver)` inside `act(...)`.
- **Timer control**: tests use `jest.useFakeTimers()` / `jest.advanceTimersByTime(N)` /
  `jest.runAllTimers()` to drive the 150ms debounce + double-rAF chain deterministically, paired
  with a `captureRaf()` helper (referenced, not yet read in full) that intercepts
  `requestAnimationFrame` calls so `flush()` can synchronously run queued rAF callbacks — needed
  because fake timers alone don't advance `requestAnimationFrame`.
- **`XtermTerminal.test.tsx`** (the sibling test file) currently has **no** `ResizeObserver`
  references at all — it doesn't exercise the resize path; all resize/fit-loop coverage lives in
  `XtermTerminalBug.test.tsx`. New component tests for AC5 (sub-cell resize burst, real
  cols/rows-change regression test) most naturally extend `XtermTerminalBug.test.tsx`'s existing
  harness/mock scaffolding rather than duplicating it in `XtermTerminal.test.tsx`, or the mocks
  should be factored out if both files need them.
- A second, simpler `ResizeObserver` mock pattern exists elsewhere in the repo for reference:
  `web-app/src/components/layout/__tests__/BottomNav.test.tsx` (`global.ResizeObserver =
  jest.fn().mockImplementation(...)`) — less capable (no entry-driving helper) than the
  `XtermTerminalBug.test.tsx` pattern; prefer the latter as the template.

### What the new tests (AC5) will need beyond what exists today

1. A **configurable** `proposeDimensions()` mock (return value settable per test, or driven by a
   sequence) to simulate: (a) sub-cell wobble — same integer cols/rows returned every call despite
   the observer firing on tiny pixel deltas — and (b) a real change — different integer cols/rows
   on a later call.
2. A way to assert `fit()` / the resize RPC path (`useTerminalFlowControl.resize`) was called
   **at most once** across a burst of ResizeObserver entries with unchanged proposed dims — the
   `harness.fitCalledCount` counter already supports this assertion style.
3. For the oscillation-detector/WebGL-fallback test (AC4): drive ≥3 identical/alternating
   `proposeDimensions()` results within a simulated 2000ms rolling window (fake timers), then
   assert the mocked `WebglAddon.dispose` (`jest.fn()`) was called, plus a `console.error` spy
   for the "no addon to dispose" backstop branch.
4. `useTerminalFlowControl.test.ts` (separate file, own mocks for `@bufbuild/protobuf`,
   `@/gen/session/v1/events_pb`, `@/lib/terminal/StateApplicator`, `@/lib/terminal/EchoOverlay`)
   is the right place for the pure value-dedup unit test on `resize(cols, rows)` (AC3) — it mocks
   the RPC push (`pushMessage`) directly, so asserting "no RPC + no follow-up
   `currentPaneRequest`" for a repeated `(cols, rows)` call is a matter of calling `resize()` twice
   with the same args (bypassing/advancing past the 200ms time throttle) and asserting
   `pushMessage` call count.

## Sources

- [xterm.js `@xterm/addon-fit` v0.11.0 source (unpkg)](https://unpkg.com/@xterm/addon-fit@0.11.0/src/FitAddon.ts)
- [xterm.js `@xterm/addon-webgl` v0.19.0 source (unpkg)](https://unpkg.com/@xterm/addon-webgl@0.19.0/src/WebglAddon.ts)
- npm registry `dist-tags.latest` for `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-webgl` (checked live, all match the versions already pinned in this repo)
- [Resolving "Resize Observer Loop Limit Exceeded" Errors](https://www.dhiwise.com/post/resolving-resize-observer-loop-limit-exceeded-errors)
- [How to fix `ResizeObserver loop limit exceeded` — TrackJS](https://trackjs.com/javascript-errors/resizeobserver-loop-limit-exceeded/)
- In-repo: `web-app/src/components/sessions/XtermTerminal.tsx`, `web-app/src/lib/hooks/useTerminalFlowControl.ts`, `web-app/src/components/sessions/__tests__/XtermTerminalBug.test.tsx`, `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx`, `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`, `web-app/jest.setup.js`, `web-app/src/components/layout/__tests__/BottomNav.test.tsx`
