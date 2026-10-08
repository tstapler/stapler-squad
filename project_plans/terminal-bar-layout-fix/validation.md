# Validation: Terminal Bar Layout Fix

**Date:** 2026-10-07

## AC Validation Checklist

| AC | Test method | Pass criteria |
|---|---|---|
| AC1: Redraw icon-only at ≤480 px | Browser DevTools → 375 px emulation, toolbar expanded | Only ↔️ visible; no " Redraw" text |
| AC2: Gallery + Files reachable at 375 px without horizontal scroll | Same viewport, expand toolbar, open More ▾ | Gallery and Files buttons visible in overflow row |
| AC3: No dead space >8 px between controls | Same viewport, inspect layout | All gaps are `gap: 0.25rem` (4 px) |
| AC4: Existing Jest tests pass unmodified | `cd web-app && pnpm jest --testPathPatterns="TerminalOutput" --no-coverage` | All green |
| AC5: Desktop ≥769 px unaffected | Browser at 1280 px or DevTools → desktop | Toolbar identical to current: Redraw shows "↔️ Redraw" text |
| AC6: Touch targets ≥44×44 px | DevTools → inspect computed style of Redraw button at 375 px | `minWidth`/`minHeight` still `44px` |
| AC7: Axe / Lighthouse passes | E2E `make e2e-lighthouse` or Axe DevTools extension | No new WCAG violations |

## Pre-Mortem: Likely Failure Modes

### Failure 1 — jscpd threshold breach
If the duplicated upload button JSX in `mobileOverflowRow` is ≥20 lines, `make ready-duplication-gate-web`
will fail. Fix: extract `<UploadButtons>` component and use it in both locations.
Check: `cd web-app && pnpm run lint:duplicates` before submitting PR.

### Failure 2 — Gallery/Files still invisible at 375 px
If `toolbarActions` still renders Gallery/Files before the overflow row entries, users
see them in the inline row (clipped). Mitigation: hide Gallery/Files from `toolbarActions`
on mobile with `display: none @≤768px`, keeping them only in the overflow row for mobile.
Desktop keeps inline rendering.

### Failure 3 — Redraw icon-only change breaks a test that asserts button text
The `toolbar-analytics.test.tsx` and `toolbarKeys.test.tsx` tests use `aria-label` and
`data-testid`, not inner text. But verify by running `pnpm jest --no-coverage` before
submitting; if a test does grep for " Redraw" text, update it to use `aria-label`.

### Failure 4 — `title` tooltip not shown on mobile long-press
Some Android browsers do not show `title` attribute tooltips on long-press. If this is
a concern, consider adding a `<span className={srOnly}>Redraw</span>` inside the button
for screen readers, keeping the visible label hidden via `toolbarButtonLabel`. However,
`aria-label` on the button already covers screen reader accessibility, so this is
optional.

## Go/No-Go Criteria

- **Go**: Jest tests pass, visual inspection confirms Gallery+Files visible at 375 px,
  Redraw is icon-only on phone, desktop layout unchanged, jscpd gate passes.
- **No-go**: Any AC fails, a new WCAG violation appears, or jscpd trips above threshold.

## Manual Test Script

```
1. Open Chrome DevTools → Toggle device toolbar → iPhone SE (375 × 667)
2. Navigate to a session's terminal tab
3. VERIFY: toolbar row is not overcrowded; Redraw shows ↔️ only (no text label)
4. Tap ⋯ (toolbar toggle) to expand toolbar
5. VERIFY: More ▾ button is visible in the expanded row
6. Tap More ▾
7. VERIFY: Gallery and Files buttons appear in the overflow row below the toolbar
8. Tap Gallery → iOS/Android photo picker opens (or file picker on desktop)
9. Switch to desktop (1280 px)
10. VERIFY: Redraw shows ↔️ Redraw (full label visible)
11. VERIFY: Gallery and Files appear inline in the expanded toolbar row (no More ▾ visible on desktop)
```
