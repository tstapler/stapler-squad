# Research: Build vs. Buy — pagination Next button color-contrast fix

## Question

Is there an existing tool/script/CI check in this repo that already computes or
validates WCAG contrast ratios for theme tokens, that should be used to
double-check `#5457ef` on `#ffffff` (and confirm the untouched retro/wh40k
pairs) rather than trusting the hand-computed ratio in the backlog item?

## Answer: no build-vs-buy decision applies — one exists, but it's stale and unwired

This is a single hex-value edit with the target value already given by the
acceptance criteria. There's no algorithm to choose, no library to select, no
service to integrate — "build vs. buy" doesn't really apply. The only
substantive question is whether an existing verification tool should be
trusted or run instead of a manual check, and the answer is no:

- **`web-app/scripts/check-theme-contrast.ts`** (wired to `npm run check-contrast`
  in `web-app/package.json:34`) does exist and implements the same WCAG
  relative-luminance / contrast-ratio formula this task needs. However:
  - It is **not invoked anywhere in CI or `make`** — `grep` across
    `web-app/package.json`, `Makefile`, and `.github/workflows/*.yml` finds
    only the npm script definition itself, no caller.
  - Its color table is a **hardcoded, stale snapshot**, not derived from
    `theme.css.ts`. Its `clean` theme entry has `primary: "#7c3aed"` and
    `background: "#0f0f11"` (a dark theme), which doesn't match the actual
    `cleanTheme` in `web-app/src/styles/theme.css.ts` (`primary: "#6366f1"`
    at line 636, `background: "#ffffff"` — a light theme). The theme set it
    covers (`matrix`, `cyberpunk77`, `wh40k`, `clean`) also doesn't match the
    current theme names in `theme.css.ts` (e.g. `cyberpunk77Theme`'s primary
    is defined with its own inline ratio comment at line 412, independent of
    this script).
  - Running it in its current state would validate numbers that don't
    reflect the source of truth, so it would not catch a regression in the
    actual token file and would need to be resynced with `theme.css.ts`
    first — out of scope for a single-token fix per the requirements
    ("Token-only edit... No consuming .css.ts, component, or CSS-architecture-
    rule changes").
- No other contrast-checking tool exists: no `wcag-contrast`,
  `color-contrast-checker`, `chroma-js`, or similar package appears in
  `web-app/package.json` or `tests/e2e/package.json` (checked via grep for
  `contrast|wcag|a11y|axe|colorjs|chroma`); the only a11y-related deps are
  `@axe-core/playwright` (already used by the e2e test this fix targets) and
  Storybook's `@storybook/addon-a11y`.

## Recommendation

Don't add a dependency or repair `check-theme-contrast.ts` for this change —
that's disproportionate to a one-line hex edit. Instead:

1. Trust the acceptance criteria's target value but verify it with a quick
   manual/CLI computation using the same WCAG relative-luminance formula
   `check-theme-contrast.ts` already implements (sRGB → linearize → relative
   luminance → `(L1+0.05)/(L2+0.05)`), rather than trusting the backlog
   item's stated ratio blindly.
2. Rely on `npx playwright test accessibility.spec.ts` (axe-core
   `color-contrast` check) as the authoritative pass/fail gate per
   requirement #3 — that's the actual production contrast checker already in
   the toolchain (`@axe-core/playwright`) and it will validate the real
   rendered DOM/CSS, unlike the stale standalone script.
3. For retro/wh40k, no computation is needed at all — requirement #4 only
   requires those lines stay byte-identical, which a diff confirms rather
   than a contrast calculation.

## Manual verification performed

Hand-computed WCAG contrast ratio for `#ffffff` on `#5457ef` using the
standard sRGB relative-luminance formula (same math as
`check-theme-contrast.ts`'s `luminance()`/`contrast()` functions):

- `#5457ef` → linear RGB ≈ (0.0886, 0.0953, 0.8636) → relative luminance ≈ 0.1494
- `#ffffff` → relative luminance = 1.0
- Contrast ratio = `(1.0 + 0.05) / (0.1494 + 0.05)` ≈ **5.27:1**

This clears the WCAG 2 AA 4.5:1 threshold for normal text with margin (the
backlog item's own claim of "≥4.5:1" is confirmed, and the actual margin is
larger than the minimum, giving some slack against future rounding/rendering
differences).

## Source locations referenced

- `web-app/scripts/check-theme-contrast.ts` — stale standalone contrast
  checker (`npm run check-contrast`), not wired into CI/`make`, hardcoded
  colors out of sync with `theme.css.ts`.
- `web-app/package.json:34` — `check-contrast` script definition.
- `web-app/src/styles/theme.css.ts:636` — target line (`primary: "#6366f1"`
  in `cleanTheme`, to become `#5457ef`).
- `web-app/src/styles/theme.css.ts:412` — existing inline-ratio-comment
  convention example (`cyberpunk77Theme` primary).
- `web-app/src/styles/theme.css.ts:389,475,477,501` — further examples of
  the same inline-comment convention.
- `web-app/src/styles/theme.css.ts:524,528` — wh40k `primary`/`primaryText`
  pair (out of scope, must stay byte-identical).
- `tests/e2e/package.json` — confirms `@axe-core/playwright` is the a11y
  testing dependency already in use; no separate contrast-checking package.
