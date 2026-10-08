# Verification: Terminal Bar Layout Fix

Measured with Playwright against a separate instance built from commit `866a70d7b`
(`PORT=62873`, private tmux socket), session `bar-test` (bash). Values are
`getBoundingClientRect()` results, not CSS inference.

## 375×800 (phone)

| AC | Result | Evidence |
|---|---|---|
| 1 Redraw icon-only | PASS | Redraw button 44×44; `aria-label`/`title` unchanged |
| 2 Gallery/Files without horizontal scroll | PASS | Toolbar ⋯ → More ▾: Gallery x 112–187, Files x 47–108 (viewport 0–375). Overflow row `scrollWidth == clientWidth == 351`; `documentElement.scrollWidth == 375`. Before the wrap fix both sat at negative x and needed scrolling |
| 3 No dead space > ~8 px | PASS | Primary row gaps 4 px (toggle 211–255, keyboard 259–303, Redraw 307–351); overflow row `gap: 0.25rem` |
| 6 Touch targets ≥ 44×44 | PASS | All primary and overflow buttons measured h = 44, w ≥ 44 (Copy 64, Paste 67, Bottom 65, Clear 65, Scrolling 83, Mouse 73, Gallery 75, Files 61) |
| 7 Axe | PASS (axe only) | axe-core 4.10.3, tags wcag2a/2aa/21aa, toolbar expanded + More open: 7 violations, none inside the toolbar or overflow row (pane `tablist` aria-required-children ×2; color-contrast on tab / bottom-nav labels ×5). **Lighthouse CI not run locally.** |

## 1280×800 (desktop)

| AC | Result | Evidence |
|---|---|---|
| 5 Unchanged ≥ 769 px | PASS | Redraw text `↔️ Redraw`, label span `display: inline`; Gallery (x 919, w 92) and Files (x 1019, w 77) inline in `toolbar-actions`; More ▾ and `toolbar-overflow-row` not rendered/visible |

Caveat: no side-by-side against a `main` build. The "Mouse" button measured at x 1289
(past the 1280 viewport edge) in this pane layout; it is not touched by this change
(the wrapper around the upload buttons is `display: contents` on desktop) but was not
confirmed identical to `main`.

## Tests

`pnpm exec jest --testPathPatterns="components/sessions" --no-coverage`: 129 suites,
1554 tests passed. `make web-build` succeeds. Jest does not load vanilla-extract
CSS, so it missed an invalid `& > button` selector that broke the production build;
the browser run is what caught it.
