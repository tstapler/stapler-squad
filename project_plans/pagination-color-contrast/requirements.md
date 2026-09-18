# Requirements: pagination-color-contrast

## Complexity

1 (quick task) — single-token hex-value edit in one file (`theme.css.ts`), following an
existing in-file precedent (the `retro` theme's identical fix). No new architecture,
no new UX pattern, no new dependency.

## Source

Backlog item `45d2cd48-ceed-4f99-af21-a401abd9cbe5`: "a11y: color-contrast violation on
pagination 'Next' button fails accessibility.spec.ts IT-5.1"

## Problem

`tests/e2e/accessibility.spec.ts` IT-5.1 ("Main page has no critical or serious
accessibility violations" and "Secondary routes are accessible") fail deterministically
on a single axe-core `color-contrast` violation (impact: `serious`): white text
(`#ffffff`) on an indigo background (`#6366f1`) measures **4.46:1**, just under the
WCAG 2 AA button/text threshold of **4.5:1**.

Reproduction:
```
cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line
# 4 failed, 16 passed — both [chromium] and [chromium-dom] projects fail the two IT-5.1 tests
```

## Root cause (confirmed by code inspection, not yet fixed)

`#6366f1` / `#ffffff` is the `primary` / `primaryText` pair of the **`clean` theme**
(`web-app/src/styles/theme.css.ts:636,640`), and `clean` is the app's default theme
(`web-app/src/lib/contexts/ThemeContext.tsx:30`, confirmed by `initialTheme` default in
the same file). `vars.color.primary` is consumed as a button background with
`vars.color.primaryText` foreground in 412 `.css.ts` files across the app (verified via
`grep -rln "vars.color.primary\b" web-app/src --include="*.css.ts" | wc -l`), including
`web-app/src/components/ui/ModalTour.css.ts:146-151`'s `primaryButton` recipe, which is
the style class rendering the axe-flagged pagination-adjacent "Next" button.

Manual contrast calculation (WCAG relative-luminance formula) on `#ffffff` vs `#6366f1`
reproduces the reported ratio: **4.465:1**, i.e. under the 4.5:1 AA threshold for normal
text/UI components — confirming this is a token-level, not component-level, bug.

**This is not a novel problem class.** The exact same defect — `primaryText` (`#ffffff`)
on `primary` failing 4.5:1 — was already found and fixed for the `retro` theme in this
same file:
```ts
// theme.css.ts:412
primary: "#cc245f", /* was #ff2d78 — #fff on #ff2d78 = 3.56:1 fails WCAG AA; #cc245f = 5.27:1 ✅ */
```
The `clean` theme's `primary` token was never given the equivalent audit/fix.

## Not a duplicate

Distinct from #424 (`accessibility.spec.ts:249`, buffered-update banner focus indicator,
`[chromium-dom]`-only click timeout — a keyboard-order/focus bug, not contrast).

## Acceptance criteria (draft — refined further in validation.md)

1. `web-app/src/styles/theme.css.ts`'s `clean` theme `primary` token is changed to a
   value that measures ≥4.5:1 contrast against `primaryText` (`#ffffff`), following the
   existing in-file convention of an inline comment recording old ratio / new ratio
   (see `retro` theme's `primary` line for the pattern).
2. The change is a token-only edit — no component/CSS-architecture rule changes, since
   `.claude/rules/css-architecture.md` already mandates `vars.color.*` token usage (this
   bug is in the token's *value*, not in a rule violation).
3. `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line` passes
   (0 failed) for both `[chromium]` and `[chromium-dom]` projects — specifically the two
   IT-5.1 tests.
4. No other theme's contrast ratios regress (spot check: the fix only touches the
   `clean` theme block; other themes' `primary`/`primaryText` pairs are unchanged).
5. Visual regression check: the new `primary` shade is a plausible/on-brand darkening of
   indigo (not a jarring hue shift) — sanity-checked by eye via `make run` or a manual
   dev-server instance per this repo's CLAUDE.md manual-testing convention (never via
   `make install-service` against the live instance).

## Non-goals

- Auditing/fixing contrast for other themes beyond `clean` (out of scope; file separately
  if found).
- Redesigning the pagination component or `ModalTour` primary-button styling beyond the
  color token.
- Addressing #424 (unrelated focus-indicator bug).

## Constraints

- Must follow `.claude/rules/css-architecture.md`: token lives in
  `web-app/src/styles/theme.css.ts`, referenced elsewhere only via `vars.color.primary`.
- `web-app/src/styles/theme.css.ts` has `/* eslint-disable no-restricted-syntax */` at
  the top specifically because it is the one file allowed to hold literal hex values.
- 412 call sites consume this token — the fix is inherently app-wide, not scoped to one
  button; this is intentional (fixing the token fixes every consumer at once) but means
  the visual sanity check (AC 5) should glance at a few other `primary`-colored surfaces
  (e.g. primary CTA buttons in `Button.css.ts`), not just the pagination/tour button.
