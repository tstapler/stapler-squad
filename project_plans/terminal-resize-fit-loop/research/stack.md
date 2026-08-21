# Research: Technology Stack — Terminal Resize/Fit Feedback Loop Fix

## 1. Installed xterm.js versions (exact, from `web-app/package-lock.json`)

| Package | Declared (package.json) | Resolved (lockfile) |
|---|---|---|
| `@xterm/xterm` | `^5.5.0` | **5.5.0** |
| `@xterm/addon-fit` | `^0.10.0` | **0.10.0** |
| `@xterm/addon-webgl` | `^0.18.0` | **0.18.0** |
| `@xterm/addon-search` | `^0.15.0` | 0.15.0 |
| `@xterm/addon-web-links` | `^0.11.0` | 0.11.0 |

`node_modules` is not installed in this worktree, so behavior was verified against upstream published source (unpkg) rather than the local copy — versions match exactly, so this is safe to rely on.

### `FitAddon.proposeDimensions()` — exact source (`@xterm/addon-fit@0.10.0`, deobfuscated)

```js
proposeDimensions() {
  if (!this._terminal) return;
  if (!this._terminal.element || !this._terminal.element.parentElement) return;
  const core = this._terminal._core;
  const dims = core._renderService.dimensions;
  if (dims.css.cell.width === 0 || dims.css.cell.height === 0) return;

  const scrollbarWidth = this._terminal.options.scrollback === 0 ? 0 : core.viewport.scrollBarWidth;
  const parentStyle = window.getComputedStyle(this._terminal.element.parentElement);
  const parentHeight = parseInt(parentStyle.getPropertyValue('height'));
  const parentWidth = Math.max(0, parseInt(parentStyle.getPropertyValue('width')));
  const elementStyle = window.getComputedStyle(this._terminal.element);
  const availableHeight = parentHeight - (padTop + padBottom);
  const availableWidth = parentWidth - (padLeft + padRight) - scrollbarWidth;

  return {
    cols: Math.max(2, Math.floor(availableWidth / dims.css.cell.width)),
    rows: Math.max(1, Math.floor(availableHeight / dims.css.cell.height)),
  };
}
```

**Key findings, directly relevant to AC2/AC5:**

- **Return type is `ITerminalDimensions | undefined`** — `{ cols: number, rows: number }` or `undefined`. It is **`undefined`**, not `Infinity`, when: the terminal isn't mounted, has no parent element, or `dims.css.cell.width/height === 0` (renderer not measured yet). Current `XtermTerminal.tsx` code does not appear to null-check `proposeDimensions()` results defensively at all call sites — this guard (`dims?.cols && dims?.rows`, or `Number.isFinite`) still matters as a defensive measure per AC5, even though 0.10.0 itself won't return `Infinity`/`NaN` in the normal path (unlike the older versions in GitHub issues #4338/#1416, which predate the zero-cell-width guard).
- **cols/rows are always integers** — `Math.floor()` is applied, with `Math.max(2, …)` / `Math.max(1, …)` floors. So AC2's requirement ("integer cols/rows that differ from currently-applied size") is naturally satisfied by `proposeDimensions()`'s own contract; the bug is that `XtermTerminal.tsx`'s `ResizeObserver` callback (line ~259) currently only gates on raw container pixel delta (`Math.abs(width - lastContainerSize.width) > 1`), never calls `proposeDimensions()` to check whether the *integer* result actually changed before scheduling `fit()`. That's the root cause of AC1/AC2's churn: sub-pixel container jitter that doesn't cross a cell-width boundary still triggers `fit()` on every observer tick.
- Because `cols = floor(width / cellWidth)`, a container width sitting near a cell-width boundary (e.g., `cellWidth × 119.98`) will flap between 119 and 120 as sub-pixel measurement noise (amplified by WebGL cell-width mismatch, see below) pushes it across the floor boundary — this is the "boundary-flapping" scenario AC2's two-consecutive-ticks confirmation is designed to dampen.

### GitHub issue context (xterm.js upstream)

- [#4338 — Fit addon fails to propose correct dimensions](https://github.com/xtermjs/xterm.js/issues/4338) and [#1416 — Browser crash from `Infinity` sizes](https://github.com/xtermjs/xterm.js/issues/1416): historical reports of `proposeDimensions()` returning `NaN`/`Infinity` when `actualCellWidth` was null — predates the current 0.10.0 zero-check guard shown above, but confirms `Number.isFinite`-style defensive guards are an established, recommended pattern in this ecosystem (AC5's requirement).
- [#4113 — revamp resize logic in demo](https://github.com/xtermjs/xterm.js/issues/4113): confirms that at non-1.0 `devicePixelRatio` / browser zoom, the DOM element resize and `FitAddon.fit()`'s own computed size diverge, producing "additional resize adjustments" — i.e. exactly the amplification mechanism described in requirements.md for WebGL glyph-width measurement mismatch.
- No official xterm.js "ResizeObserver feedback loop" issue was found describing this exact bug pattern; the loop described in requirements.md is specific to this app's own `ResizeObserver` → `fit()` → `onResize` → RPC → re-render wiring, not an upstream xterm.js bug per se.

### WebGL addon (`@xterm/addon-webgl@0.18.0`) — cell-width mismatch / fallback

- Confirmed pattern from xterm.js docs/README: the WebGL addon exposes an `onContextLoss` event; the documented/recommended way to handle WebGL failure is `addon.onContextLoss(e => { addon.dispose(); })`, then falling back to another renderer.
- **Important gap**: xterm.js v5+ no longer bundles a canvas renderer in core — it was extracted into a separate `@xterm/addon-canvas` package, which **is not currently a dependency** in `web-app/package.json` (grep confirms no `addon-canvas` entry in `package.json` or `package-lock.json`). Requirements.md's constraint says "No new dependencies expected," but AC5 says "fallback to canvas renderer" — **these are in tension**. Without `@xterm/addon-webgl` and without `@xterm/addon-canvas`, xterm.js falls back to its built-in DOM renderer (present in core, no addon needed) rather than a canvas renderer. Flag this for the planning phase: either (a) add `@xterm/addon-canvas` as a new (small, no-transitive-dep) dependency, or (b) redefine AC5's "canvas renderer" fallback as "disable/dispose the WebGL addon, falling back to xterm's built-in DOM renderer" to honor the no-new-deps constraint. This is a decision only the planner/user can make — worth surfacing explicitly in plan.md.
- Existing app code already logs the exact "Actual pixels per column" vs "Expected pixels per column" comparison (`XtermTerminal.tsx` lines 188–197, confirmed) but takes no corrective action — this is the hook point for AC5's tolerance-check + fallback logic.

## 2. Testing stack

**Jest, not Vitest** — despite requirements.md's constraints section stating "Testing follows existing Vitest/RTL conventions," the actual test runner in `web-app/package.json` is:

```json
"scripts": { "test": "jest", "test:watch": "jest --watch" },
"devDependencies": {
  "@testing-library/jest-dom": "^6.9.1",
  "@testing-library/react": "^16.3.0",
  "@types/jest": "^30.0.0",
  "jest": "^30.2.0",
  "jest-environment-jsdom": "^30.2.0",
  "ts-jest": "^29.4.4"
}
```

`web-app/jest.config.js`:
```js
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'jsdom',
  roots: ['<rootDir>/src'],
  testMatch: ['**/__tests__/**/*.ts?(x)', '**/?(*.)+(spec|test).ts?(x)'],
  moduleNameMapper: { '^@/(.*)$': '<rootDir>/src/$1' },
  setupFilesAfterEnv: ['<rootDir>/jest.setup.js'],
};
```

`web-app/jest.setup.js` only polyfills `TextEncoder`/`TextDecoder` for jsdom — **no `ResizeObserver` polyfill exists**. jsdom does not implement `ResizeObserver` natively, so any new test exercising `XtermTerminal.tsx`'s `ResizeObserver` callback (AC2's dead-band/2-tick logic) will need either a manual `class MockResizeObserver` assigned to `global.ResizeObserver` in the test file (there's no existing shared mock to reuse — this file doesn't exist anywhere in `web-app/src`), or a small addition to `jest.setup.js`. There is currently **no test file for `XtermTerminal.tsx` at all** (`find` confirms no `__tests__` entry or `.test.tsx` sibling) — AC2/AC5 tests will be new files, not additions to an existing suite.

### Existing pattern to follow: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

This is the canonical example for AC3/AC4 (resize dedup + force:true tests):
- Uses `@testing-library/react`'s `renderHook` + `act`.
- `jest.useFakeTimers()` in `beforeEach`, `jest.useRealTimers()` in `afterEach`, `jest.advanceTimersByTime(ms)` to move time forward deterministically (already exercises the 200ms throttle: see `describe('resize', …)` block, lines 126–175).
- Mocks `@/gen/session/v1/events_pb` (protobuf-generated types) and `@/lib/terminal/StateApplicator`/`EchoOverlay` via `jest.mock(...)` at module level with hand-rolled class stand-ins — reuse this mocking pattern rather than inventing a new one.
- `createTestOptions()` helper builds a fresh `pushMessageRef`/`isConnectedRef`/mock terminal per test — reuse this pattern for the new `force: true` regression test (AC4) and the value-dedup test (AC3).
- Existing `it('should throttle to 200ms', …)` test (lines 140–156) is the direct precedent to extend/adapt for AC3's dedup-independent-of-throttle behavior.

**Confirmed current `resize()` signature has only 2 params** (`resize(cols, rows)` — no third arg) in `web-app/src/lib/hooks/useTerminalFlowControl.ts`; call sites in `web-app/src/components/sessions/TerminalOutput.tsx` at lines 327, 351, and 510 all currently call `resize(cols, rows)` with no third argument — confirms AC4's `force: true` param doesn't exist yet and must be added to the function signature plus threaded through the two named call sites (351 = resync, 510 = manual Fit button; line 327 is the normal value-changed path and should NOT get `force: true`).

## 3. Existing debounce/throttle utilities

`web-app/src/lib/hooks/useDebounce.ts` already exists and exports two hooks:
- `useDebounce<T>(value, delay)` — debounces a *value* via `useState` + `useEffect`/`setTimeout`.
- `useDebouncedCallback<T>(callback, delay)` — debounces a *callback*, tracking `timeoutId` in `useState`.

Both are React-hook-shaped (call `useState`/`useEffect` internally), so they're only usable inside function components/other hooks — **not** directly usable inside a `ResizeObserver` callback closure (which lives inside a `useEffect` in `XtermTerminal.tsx`, not itself a hook context) or inside `useTerminalFlowControl`'s imperative `resize()` callback (which needs synchronous ref-based dedup, not a stateful debounce). `XtermTerminal.tsx` already hand-rolls its own `resizeTimeout`/`resizeCount` adaptive-debounce logic imperatively (lines 256–294) rather than using this hook, for exactly that reason — imperative `useRef`-based timers, not the `useDebounce` hooks, are the established pattern for non-component-render debouncing in this codebase.

No `lodash.debounce`/`lodash.throttle` is a direct dependency — `lodash`/`lodash.merge`/`lodash.memoize` appear only as transitive deps in `package-lock.json` (not in `package.json` directly), so `lodash` should not be imported as first-party debounce/throttle tooling.

**Recommendation for planning**: implement AC2's 2-tick confirmation and AC3's value-dedup as small `useRef`-held state machines (last-applied-value ref + pending-candidate ref + tick counter), following the imperative pattern already used in `XtermTerminal.tsx`'s existing `ResizeObserver` callback and `useTerminalFlowControl.ts`'s existing `lastResizeTimeRef`/throttle pattern — not the `useDebounce`/`useDebouncedCallback` hooks (wrong context) and not a new dependency.

## Open items for planning phase

1. **AC5 "canvas renderer" fallback has no backing dependency.** `@xterm/addon-canvas` is not installed; requirements.md constraints say no new deps expected. Needs an explicit decision: add the addon, or redefine the fallback target as xterm's built-in DOM renderer.
2. **requirements.md's testing-stack reference is stale**: it says "Vitest/RTL conventions" but the repo uses Jest + RTL. Plan.md and any test-writing tasks should say Jest explicitly to avoid confusion during implementation.
3. **No `ResizeObserver` mock exists anywhere in the test suite.** AC2/AC5/AC6 test tasks should include adding a minimal mock (either local to the new test file or added to `jest.setup.js` if reused across files).
