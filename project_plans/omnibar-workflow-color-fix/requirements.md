# Requirements: omnibar-workflow-color-fix

## Source

Backlog item `2873534f-7fa4-4219-89b4-fb9fa3cdd438`, title: "Need to fix the coloring for workflows in the omnibar."

Description: a mobile screenshot of the omnibar's `@`-mention dropdown showing a workflow suggestion (`@knowledge-maintenance`, lightning-bolt icon) rendered with different text coloring than the repo suggestions above it (`@ssq`, `@pw`), which render in bold white.

## Complexity

**1 (quick task).** Confirmed root cause via direct code read — single-token CSS fix in one file, no new dependencies, no architectural change.

## Root cause (verified)

`web-app/src/components/ui/AtCommandDropdown.css.ts`, the `slug` style (rendering `@{wf.slug}` in `AtCommandDropdown.tsx:67`) sets:

```ts
export const slug = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.accentHover,   // <-- wrong token
  ...
});
```

`vars.color.accentHover` (`web-app/src/styles/theme.css.ts`) is a **low-alpha rgba background overlay** meant for hover-state backgrounds (e.g. `rgba(45, 156, 219, 0.2)` in the dark theme), not a foreground text color — it has never been contrast-checked against `cardBackground` the way `textPrimary`/`textMuted`/`accentText` explicitly have (see the WCAG-ratio comments beside those tokens in `theme.css.ts`). Used as `color` on text, it renders as a washed-out, low-contrast, oddly-tinted blue — matching the screenshot's `@knowledge-maintenance` handle, in contrast to the repo rows (`OmnibarRepoResult.css.ts`'s `repoName`), which correctly use `vars.color.textPrimary`.

The codebase already has the correct token for this exact case: `vars.color.accentText` — a solid, WCAG-AA-verified color designed to sit on `accentBg`/be used as accent-colored text (used this way in `BacklogItemDetail.css.ts`, `JulesStatusBadge.css.ts`, `PlanVerdictBox.css.ts`, `InlineNotice.css.ts`).

Every one of the 6 theme variants (light, dark, matrix, cyberpunk77, wh40k, clean — `theme.css.ts`) defines both tokens, so the bug reproduces in every theme, not just the dark theme shown in the screenshot.

## Acceptance Criteria

0. `AtCommandDropdown.css.ts`'s `slug` style uses `vars.color.accentText` instead of `vars.color.accentHover` for the `@slug` text color.
1. The workflow suggestion row's `@slug` text renders as a solid, WCAG-AA-contrast accent color consistent with how other accent-colored text renders elsewhere in the app (e.g. `JulesStatusBadge`), not a washed-out translucent tint.
2. No other visual property of the workflow dropdown row (icon, name, description, selected/hover background) changes — this is a single-property color fix, not a redesign.
3. `web-app/src/components/ui/AtCommandDropdown.css.ts` compiles and `make lint`/`cd web-app && npx jest --no-coverage` pass with no new failures.

## Non-goals

- Redesigning the `@`-mention dropdown's layout or the repo-result rows (`OmnibarRepoResult.tsx`) — those already use correct tokens and are out of scope.
- Changing the `accentHover`/`accentText` token definitions themselves in `theme.css.ts` — the tokens are correctly defined; the bug is a misuse at one call site.
- Any change to `WorkflowDetector.ts` detection logic — this is purely a rendering/styling fix.

## Open Questions

None — root cause and fix are unambiguous from direct code inspection.
