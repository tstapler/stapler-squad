# Validation Plan: pagination-next-color-contrast

**Date**: 2026-08-21

## Happy Path Scenario

Given the clean theme's `primary` token is `#6366f1` (4.46:1 against `#ffffff`, below WCAG AA 4.5:1), when the developer changes `theme.css.ts:636` to `#5457ef` with the standard ratio comment, then `accessibility.spec.ts` IT-5.1 passes with 0 failed across both `[chromium]` and `[chromium-dom]` projects against a freshly built binary, with no byte change to the retro/wh40k primary lines and no other consumer `.css.ts` file touched.

## Requirement → Test Mapping

Domain Glossary note: plan.md's Domain Glossary is N/A for this change (no new types/methods) — test names below reference file:line and hex values directly instead of domain terms.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: primary hex change + ratio comment | `web-app/src/styles/theme.css.ts` | `themeCssTs_should_ContainExactRatioCommentLine_When_Line636Edited` (`grep -n 'primary: "#5457ef"' web-app/src/styles/theme.css.ts`) | Unit (verification command) | Happy path — line 636 reads exactly `primary: "#5457ef", /* was #6366f1 — #fff on #6366f1 = 4.46:1 fails WCAG AA; #5457ef = 5.27:1 ✅ */` |
| REQ-1: primary hex change + ratio comment | `web-app/src/styles/theme.css.ts` | `themeCssTs_should_NotRetainOldHexAtLine636_When_EditApplied` (`grep -c '"#6366f1"' web-app/src/styles/theme.css.ts` — expect count to drop by exactly 1 vs. pre-edit baseline, and line 636 specifically to no longer match `#6366f1`) | Unit (verification command, error/regression path) | Error path — catches an incomplete edit (old hex left in place, or comment added without changing the value) |
| REQ-1: primary hex change + ratio comment | `tests/e2e/accessibility.spec.ts` | `computedBackgroundColor_should_Resolve_To_5457ef_When_NextButtonRendered` (Playwright `page.evaluate` reading `getComputedStyle` on the ModalTour Next button / Button CTA, or covered implicitly by axe's contrast recompute in IT-5.1) | Integration | Confirms the token change actually reaches the rendered DOM through the vanilla-extract → CSS pipeline, not just the source file |
| REQ-2: token-only edit, no consumer file touched | repo root | `gitDiffStat_should_ListExactlyOneFile_When_Story1_1_1_Applied` (`git diff --stat` → only `web-app/src/styles/theme.css.ts`) | Unit (verification command) | Happy path — proves zero consumer `.css.ts` files were edited regardless of the 412-reference/142-file blast radius |
| REQ-2: token-only edit, no consumer file touched | repo root | `vars_color_primary_ReferenceCount_should_BeUnchanged_When_EditApplied` (`grep -c 'vars\.color\.primary\b' -r web-app/src --include='*.css.ts' \| grep -v theme.css.ts`, before vs. after) | Unit (verification command, error/regression path) | Error path — would catch an edit that accidentally touched a reference site (e.g. an IDE refactor rename) even though it left the file list intact |
| REQ-2 (Story 1.1.2 spot-check): foreground-usage blast radius documented | `web-app/src/components/sessions/SessionList.css.ts:107`, `ReviewQueuePanel.css.ts:287,429,493` | `foregroundUsageFinding_should_MatchStory1_1_2Table_When_SpotChecked` (manual relative-luminance check against `background`/`cardBackground`/`panelBgSecondary`/`surfaceMuted`/`accentBg`) | Integration (read-only, no test-file automation) | Confirms `SessionList.css.ts:107` reconfirms "no new crossing" and `ReviewQueuePanel.css.ts` reconfirms the genuine `accentBg`-composite 3.50:1→2.97:1 WCAG 1.4.11 regression, both as predicted in plan.md — not a pass/fail gate, a documentation check feeding the PR body |
| REQ-3: `accessibility.spec.ts` passes on fresh build | `tests/e2e/accessibility.spec.ts` | `IT-5.1_MainPageHasNoCriticalOrSeriousViolations_should_Pass_When_RunAgainstFreshBinary` (`make build` then `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line`) | Integration (E2E, real binary + browser + axe-core) | Happy path — 0 failed across `[chromium]` and `[chromium-dom]` |
| REQ-3: build freshness (staleness guard) | repo root | `stapleSquadBinary_should_HaveMtimeNewerThanThemeCssTsEdit_When_MakeBuildRuns` (`ls -la ./stapler-squad` vs. `git log -1 --format=%cI -- web-app/src/styles/theme.css.ts` or the edit timestamp) | Unit (verification command, error path) | Error path — guards against `ensureBinary()`'s 1-hour cache reuse producing a false pass (pitfalls.md §5) |
| REQ-4: no regression to retro/wh40k primary pairs | `web-app/src/styles/theme.css.ts` | `themeCssTs_should_KeepLines412_524_528ByteIdentical_When_Story1_1_1Applied` (`git diff web-app/src/styles/theme.css.ts` — confirm hunk touches only line 636) | Unit (verification command) | Happy path / error path combined — a diff touching 412, 524, or 528 is the failure signal for this single check |
| REQ-5: visual sanity + snapshot regen | see UX Acceptance Tests below | — | E2E / visual | Folded into UX Acceptance Tests (user-facing) |
| REQ-6: `make lint` passes | repo root | `makeLint_should_ExitZero_When_OnlyCssTsAndPngFilesChanged` (`make lint`) | Unit (verification command) | Happy path — Go-only linters have no `.go` diff to flag; passing is procedural evidence, not CSS-safety verification (per plan.md Story 1.5.1) |

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| REQ-5a: new shade regenerates both existing `visual-clean` snapshots, no new files added | `tests/e2e/visual-regression.spec.ts` | `visualClean_should_RewriteExactlyTwoExistingSnapshots_When_UpdateSnapshotsRun` | Playwright (`--project=visual-clean --update-snapshots`) | 1. `make build` fresh binary 2. `npx playwright test visual-regression.spec.ts --project=visual-clean --update-snapshots` 3. `git status` confirms only `session-list-empty.png` and `omnibar-open.png` modified, no new files |
| REQ-5b: tour modal (fixed Next button) is actually captured in-frame, or manual fallback performed | `tests/e2e/visual-regression.spec.ts` scenario defs + `BacklogTourModal.tsx` trigger condition | `tourModal_should_BeInFrameOrManualFallbackRecorded_When_SnapshotsRegenerated` | Playwright + manual browser (CLAUDE.md "Manual/interactive testing" workflow, port 8999) | 1. Read scenario definitions for `session-list-empty`/`omnibar-open` 2. If neither triggers `BacklogTourModal`, run `go build -o /tmp/ssq-manual-test .` then `PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`, open browser, trigger tour Next button + Button CTA 3. Record which path was used in PR body 4. `kill %1` |
| REQ-5c: `#5457ef` reads as a plausible darker indigo, not a hue shift | regenerated PNGs / manual swatch | `colorShift_should_ReadAsDarkerIndigoNotHueShift_When_ComparedSideBySide` | Manual visual check (screenshot diff or swatch) | 1. Compare `#6366f1` vs `#5457ef` swatches or before/after screenshots 2. Confirm hue stays in the blue-violet family (~243°), not shifted toward teal/magenta |
| REQ-5d (recommended, non-blocking): spot-check `SessionList.css.ts` active-filter-toggle and `ReviewQueuePanel.css.ts` tag/banner states | none (no new automated snapshot) | `secondaryConsumers_should_LookVisuallyIntact_When_ManuallyInspected` | Manual browser check (same session as REQ-5b) | 1. While in the manual browser session, trigger the active-filter-toggle state on the session list 2. Trigger tag/banner states on `/review-queue` 3. Confirm nothing looks obviously broken 4. Note result in PR body (recommendation only, not a merge gate) |
| REQ-3 (visual half): snapshot regen runs against the same fresh binary as the accessibility spec | n/a | `snapshotRegen_should_UseSameFreshBinaryAsAccessibilitySpec_When_BothRunInSameSession` | Process check | Confirm `make build` from Story 1.2.1 is still fresh (rerun if several minutes elapsed) before Story 1.3.1's `--update-snapshots` run |

## Test Stack

- **Unit**: No new unit-test framework needed — "unit" rows above are shell/grep/git verification commands run from the repo root, since this is a single hex-literal + comment edit with no new function, branch, or type.
- **Integration**: Playwright (`tests/e2e/accessibility.spec.ts`) against a `make build`-fresh `stapler-squad` binary — the closest thing to an integration test here, since it exercises the real vanilla-extract build pipeline, the served app, and axe-core's contrast engine together.
- **E2E / UX**: Playwright visual regression (`tests/e2e/visual-regression.spec.ts --project=visual-clean --update-snapshots`) plus a manual-browser fallback checklist (CLAUDE.md's "Manual/interactive testing" workflow on port 8999) for confirming the tour modal and secondary consumers.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Verification commands (REQ-1, 2, 4, 6) | `git diff --stat`, `git diff web-app/src/styles/theme.css.ts`, targeted `grep -c`, `make lint` | 100% of the 4 verification-only requirements pass before proceeding to Phase 2 |
| E2E / accessibility (REQ-3) | `cd tests/e2e && npx playwright test accessibility.spec.ts --reporter=line` | `0 failed`, IT-5.1 passing under both `[chromium]` and `[chromium-dom]` |
| Visual regression (REQ-5) | `cd tests/e2e && npx playwright test visual-regression.spec.ts --project=visual-clean --update-snapshots` | Both existing snapshots rewritten, no new files, tour-modal visibility explicitly recorded (test or manual fallback) |

- All 6 requirements: at least one test/verification row above (6/6 = 100% mapped).
- No code-level line-coverage target applies — there is no new Go or TypeScript logic (`plan.md`'s Domain Glossary and Migration/Observability/Risk sections are all N/A for the same reason).
- Story 1.1.2's foreground-usage finding (the `ReviewQueuePanel.css.ts` `accentBg`-composite WCAG 1.4.11 regression) is explicitly out of this fix's authorized scope — it is documented and spot-checked, not gated on, per plan.md's scoping call.
