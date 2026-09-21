# Validation Plan: omnibar-workflow-color-fix

**Date**: 2026-09-11

## Happy Path Scenario
Given the omnibar's `@`-mention dropdown open with a workflow suggestion row visible, when the `slug` style's `color` token is swapped from `vars.color.accentHover` to `vars.color.accentText`, then the `@slug` text (e.g. `@knowledge-maintenance`) renders as solid, WCAG-AA-contrast accent text instead of a washed-out translucent overlay, with nothing else on the row changed.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC0: `slug` style uses `vars.color.accentText` instead of `vars.color.accentHover` | `web-app/src/components/ui/AtCommandDropdown.css.ts` | Manual code inspection of `slug` style block (line 42) | Manual/static | Open the file, confirm the `color` property in the `slug` style reads `vars.color.accentText`, not `vars.color.accentHover`. |
| AC1: `@slug` text renders as solid, non-translucent, WCAG-AA-contrast accent color, reusing `accentText` the same way `JulesStatusBadge` does | N/A (visual) | Manual visual check — workflow row text solidity/color | Manual/visual | With the running app, type `@` + a workflow slug in the omnibar; observe the slug text is a solid accent color matching `accentText`'s rendering elsewhere (e.g. `JulesStatusBadge`), not a faded/tinted overlay. Contrast ratios already computed analytically in `plan.md` (light 9.38:1, dark 5.71:1) — this check confirms the rendered result matches that computation, it does not recompute it. |
| AC2: No other visual property of the row changes (icon, name, description, selected/hover background all unchanged) | `web-app/src/components/ui/AtCommandDropdown.css.ts` | `git diff` single-line-change check | Manual/static | Run `git diff -- web-app/src/components/ui/AtCommandDropdown.css.ts` and confirm exactly one line changed (the `color` value inside `slug`) — `fontFamily`, `fontSize`, `flexShrink`, `minWidth`, and every other style block are untouched. |
| AC3: File compiles; `make lint` / `npx jest --no-coverage` pass with no new failures | n/a (build/lint/test tooling, not a project test file) | `tsc --noEmit`, `make lint`, `npx jest --no-coverage` regression run | Automated (regression, not new) | Run `cd web-app && npx tsc --noEmit`, `make lint` (repo root), and `cd web-app && npx jest --no-coverage`; confirm all three complete with no new errors/failures versus pre-fix baseline. |
| AC4: Fix visually confirmed in a running instance of the app | N/A (manual) | Manual end-to-end visual confirmation in running app | Manual/visual | Per Task 1.1.1c: run `cd web-app && pnpm dev` (or a manual backend instance per CLAUDE.md's manual-testing section), open the omnibar, type `@<workflow-slug>`, and confirm the slug text renders as solid accent color rather than the pre-fix faded overlay. Optionally stash/restore the change for a before/after comparison. |

## UX Acceptance Tests
| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Workflow slug text is legible/solid accent color with no other row regression (plan Task 1.1.1c) | N/A | Manual visual check of `AtCommandDropdown` workflow row | Browser (`pnpm dev` or manual app instance) | 1. Start `cd web-app && pnpm dev` (or build/run a manual instance per this repo's CLAUDE.md manual-testing section if backend behavior is needed). 2. Open the omnibar (Meta+K). 3. Type `@` followed by a configured workflow slug (e.g. `@knowledge-maintenance`) to trigger `AtCommandDropdown`. 4. Confirm the `@slug` text renders as solid `accentText` and not the faded/translucent `accentHover` overlay. 5. Confirm the icon, name, description, and selected/hover background of the row are unchanged from before the fix. 6. Stop the dev server / manual instance when done. |

## Test Stack
This fix has no automated test for the touched component (`research/stack.md` confirms no existing `AtCommandDropdown` test file). Verification is `cd web-app && npx tsc --noEmit` (compile) + `make lint` + `cd web-app && npx jest --no-coverage` (regression-only — confirms the one-line token swap introduces no new failures elsewhere) + one manual visual check (Task 1.1.1c). Adding new automated test infrastructure for this component (jest-axe contrast assertions, a new `AtCommandDropdown.test.tsx`, or extending a Playwright spec) was explicitly considered and deferred to follow-up work per `research/build-vs-buy.md`'s verdict — not omitted by oversight. The plan's Tech Debt Disposition section also flags a related follow-up: no lint rule currently guards against reusing a background/overlay token (`accentHover`, `accentBg`) as a foreground `color`, so this misuse class could recur undetected; that guard is out of scope for this complexity-1 fix.

## Coverage Targets and How to Measure
N/A — complexity-1 fix to an already-untested component; no new coverage target set. The existing `cd web-app && npx jest --coverage` baseline must not regress as a result of this change.
