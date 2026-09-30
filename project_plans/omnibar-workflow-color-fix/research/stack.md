# Stack Research: omnibar-workflow-color-fix

## Versions (`web-app/package.json`)
- `@vanilla-extract/css` `^1.20.1`, `@vanilla-extract/recipes` `^0.5.7`, `@vanilla-extract/next-plugin` `^2.5.1`
- `react` `^19.0.0`, `typescript` `^5.9.3`, `jest` `^30.2.0` / `jest-environment-jsdom` `^30.2.0`

## How vanilla-extract tokens work here
`web-app/src/styles/theme.css.ts` defines a `vars` contract (via `createThemeContract`/`createTheme`, one block per theme) with a `color.accentHover` (low-alpha rgba overlay, e.g. `rgba(45, 156, 219, 0.2)` in dark theme) and a separate `color.accentText` (solid hex, contrast-checked — see the inline comments at theme.css.ts:113 and :220 documenting the measured contrast ratio against `accentBg`). All 7 theme variants define both tokens (lines ~112-679). Consuming `*.css.ts` files import `{ vars }` and reference tokens as plain object properties inside `style({...})` calls — e.g. `web-app/src/components/ui/AtCommandDropdown.css.ts:42` currently does `color: vars.color.accentHover` for the `slug` style, which is the bug: that token is meant for backgrounds, not foreground text. This is purely a compile-time object-property swap (`accentHover` → `accentText`); vanilla-extract resolves these to CSS custom properties at build time, so there's no runtime/type distinction between "background token" and "text token" — both are just `string` in the `vars` contract.

## Tooling that would (or wouldn't) catch this class of bug
- **stylelint** (`^17.6.0` + `stylelint-config-standard`/`stylelint-config-css-modules`) is configured via `web-app/.stylelintrc.js`, but `package.json`'s `lint:css` script runs `stylelint 'src/**/*.css' --allow-empty-input` — glob is `*.css`, not `*.css.ts`. vanilla-extract source files aren't linted by stylelint at all (only the CSS vanilla-extract emits at build time would be, and that's not wired into `lint:css`). No rule here would have flagged the wrong token.
- **No custom eslint rule** exists for semantic-token misuse (checked `eslint-plugin-analytics` and `eslint-plugin-rpc-lifecycle`, the two local plugins in `devDependencies` — neither targets CSS/theme tokens).
- **No jest-axe / accessibility test tooling**: confirmed absent from `devDependencies`, and a comment in `web-app/src/components/sessions/CreatePullRequestModal.test.tsx:17` explicitly notes `jest-axe` is not a devDependency in this repo yet and no other test file uses it. Contrast regressions like this one are not caught by any automated a11y check.
- **No existing test file** for the component: `AtCommandDropdown.tsx`/`.css.ts` have no sibling `.test.tsx`. No snapshot test exists to update or extend.

## Implication for this fix
Nothing in the toolchain enforces "background tokens vs. text tokens" — the swap is a manual, human-verified correctness fix (matching the precedent at `BacklogItemDetail.css.ts`, `JulesStatusBadge.css.ts`, `PlanVerdictBox.css.ts`, `InlineNotice.css.ts`, all of which already use `accentText` for foreground text). Verification is: `cd web-app && npx tsc --noEmit` (or the existing build) to confirm the `.css.ts` still compiles, plus `make lint` and `npx jest --no-coverage` per the acceptance criteria — there is no lint rule, snapshot, or a11y test that will independently confirm the contrast improved, so this stays a visually-verified/manual-review fix, not a test-gated one.
