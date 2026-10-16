# Validation Plan: pagination-color-contrast

**Date**: 2026-08-16

## Happy Path Scenario
Given the `clean` theme's `primary` token is `#6366f1` (fails 4.5:1 against white), when
it's changed to `#5457ef` per plan.md Task 1.1.1a, then `accessibility.spec.ts` IT-5.1
passes with 0 failures.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: `clean` theme `primary` token changed to a value ≥4.5:1 against `primaryText` (`#ffffff`), with inline old/new-ratio comment matching `retro`'s convention | N/A — throwaway `node -e` one-liner (Task 1.1.1b) | `cleanTheme_primaryContrast_should_PassWCAG_AA_When_TokenUpdated` | Verification script (no unit-testable logic — config-value change) | Compute WCAG relative-luminance contrast of `#ffffff` vs the new `#5457ef` using the same hexToRgb→linearize→luminance→contrast formula `check-theme-contrast.ts` implements; assert ratio ≥4.5 (expect ~5.27) |
| AC2: change is token-only — no `.css-architecture.md` rule change, no component edits | N/A — `git diff` scope check (Task 1.1.1a) | `themeTokenEdit_should_BeSingleLineDiff_When_FixApplied` | Integration (static diff verification) | `git diff web-app/src/styles/theme.css.ts` shows exactly one changed line (636, the `clean` block's `primary` field); no other file or line in the repo differs |
| AC3: `accessibility.spec.ts` passes (0 failed) for `[chromium]` and `[chromium-dom]`, specifically the two IT-5.1 tests | `tests/e2e/accessibility.spec.ts` | `accessibilitySpec_IT5_1_should_PassWCAG_AA_When_CleanThemeTokenFixed` | E2E | After Task 1.1.2a's fresh `make build` embeds the fix into `server/web/dist`, run `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line`; expect `0 failed` (was `4 failed, 16 passed`), both "Main page has no critical or serious accessibility violations" and "Secondary routes are accessible" passing on both projects |
| AC4: no other theme's `primary`/`primaryText` pair regresses | N/A — `grep` scope check (Task 1.1.2c) | `otherThemeTokens_should_RemainUnchanged_When_CleanThemeFixed` | Integration (static verification) | `grep -n "primary:\|primaryText:" web-app/src/styles/theme.css.ts`; confirm `retro` (line 412, `#cc245f`/`#ffffff`) and `wh40k` (lines 524/528, `#c0a020`/`#0c0a08`) blocks are byte-identical to pre-change values — only the `clean` block's line 636 differs |
| AC5: new shade is a plausible/on-brand darkening of indigo, not a jarring hue shift | `tests/e2e/visual-regression.spec.ts` | `visualRegressionSnapshots_cleanTheme_should_RegenerateWithoutHueShift_When_TokenUpdated` | E2E (snapshot) | After Task 1.1.2a's fresh build, run `npx playwright test visual-regression.spec.ts --update-snapshots --project=visual-clean` from `tests/e2e/`; review regenerated PNGs under `tests/e2e/tests/snapshots/visual-clean/` for subtle-not-jarring diffs across `primary`-colored surfaces (Header/Omnibar per research/pitfalls.md finding 4), then stage and commit alongside the token edit |

## UX Acceptance Tests
(Complete this section only for user-facing features; omit for pure infrastructure.)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Primary CTA and onboarding-tour "Next" button read as an on-brand darkening of indigo (not shifted toward blue/purple/magenta) when viewed under the `clean` theme | N/A — manual check (Task 1.1.3b) | `primaryButtonShade_should_ReadAsOnBrandIndigo_When_ViewedManually` | manual | 1. `go build -o /tmp/ssq-manual-test . && PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &` (never `make install-service`, per CLAUDE.md). 2. With `clean` theme active (app default), view `Button.css.ts`'s `intent: "primary"` CTA variant and `ModalTour.css.ts`'s `primaryButton` "Next" button side by side. 3. Confirm both render `#5457ef` (vs prior `#6366f1`) as a slightly-darker indigo with no hue shift. 4. `kill %1` when done. |

## Test Stack
- **Unit**: N/A — config-value change, no unit-testable logic. This is a single string
  literal edit in `web-app/src/styles/theme.css.ts` (a file explicitly exempted from
  `lint-css-tokens` via its top-of-file `/* eslint-disable no-restricted-syntax */`, per
  `.claude/rules/css-architecture.md`).
- **Integration**: Static verification via `git diff` (scope check, Task 1.1.1a) and
  `grep` (other-theme regression check, Task 1.1.2c) — no test runner involved, both are
  plan-prescribed shell commands run against the edited file.
- **E2E / UX**: `accessibility.spec.ts` IT-5.1 (Task 1.1.2b) is the authoritative pass/fail
  gate for the reported bug. `visual-regression.spec.ts` snapshot regeneration (Task
  1.1.3a) is local dev-run hygiene, not a CI gate (`visual-regression.spec.ts` is excluded
  from `e2e-video.yml`'s `FEATURE_SPECS` allowlist and not run by `ux-analysis.yml`). The
  manual visual sanity check (Task 1.1.3b) covers on-brand-ness, which no automated check
  asserts.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Unit | N/A | N/A — no new Go/TS logic introduced; standard coverage thresholds don't apply to a single string-literal token edit |
| Integration | `grep -n "primary:\|primaryText:" web-app/src/styles/theme.css.ts` | 0 diff in the `retro` and `wh40k` blocks; exactly 1 changed line (636) in the `clean` block |
| E2E / UX | `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line` | **The actual pass/fail bar for this fix**: IT-5.1 goes from `4 failed, 16 passed` (pre-fix, per requirements.md's reproduction) to `0 failed` on both `[chromium]` and `[chromium-dom]` projects |
