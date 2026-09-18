# Requirements: pagination Next button color-contrast fix

Backlog item: `45d2cd48-ceed-4f99-af21-a401abd9cbe5`

## Problem

`tests/e2e/accessibility.spec.ts` IT-5.1 fails deterministically (axe-core
`color-contrast`, impact: serious) on the pagination "Next" button. It uses the
clean theme's `primary`/`primaryText` token pair (`#6366f1` bg / `#ffffff` fg,
4.46:1) which is below the WCAG 2 AA 4.5:1 threshold for normal text.

## Scope

Single-token fix in `web-app/src/styles/theme.css.ts`, clean theme only. No
consuming `.css.ts`, component, or CSS-architecture-rule changes.

## Requirements

1. Change clean theme `primary` (theme.css.ts:636) from `#6366f1` to `#5457ef`
   (≥4.5:1 against `#ffffff`), with an inline ratio comment matching the
   existing convention seen at theme.css.ts:412, 389, 475, 477, 501.
2. Token-only edit — do not touch any file that consumes `vars.color.actionPrimary`
   / the primary token, per `.claude/rules/css-architecture.md`.
3. `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line`
   passes 0 failed (both `[chromium]` and `[chromium-dom]`) against a binary
   built via `make build` (not a stale cached one — e2e's `ensureBinary()`
   reuses binaries under 1 hour old).
4. No regression to retro (theme.css.ts:412) or wh40k (theme.css.ts:524,528)
   primary/primaryText contrast pairs — those lines stay byte-identical.
5. Visual sanity: new shade reads as a plausible darker indigo (not a hue
   shift) on Button.css.ts CTA and ModalTour.css.ts Next button; regenerate
   and commit `tests/e2e/tests/snapshots/visual-clean/` snapshots.
6. `make lint` passes with the change applied.

## Out of scope

- Other themes' contrast issues (not reported as failing).
- `.claude/rules/css-architecture.md`'s "no hardcoded hex" rule does not apply —
  `theme.css.ts` is the token *source*, already exempted from that lint rule.
- The unrelated `#424` focus-indicator failure (different assertion, different bug).

## Verification

- `npx playwright test accessibility.spec.ts --reporter=line` (tests/e2e/)
- `make lint`
- Manual contrast check: WCAG contrast ratio of `#ffffff` on `#5457ef`.
