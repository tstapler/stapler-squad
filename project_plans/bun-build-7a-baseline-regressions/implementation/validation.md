# Validation Plan: bun-build-7a-baseline-regressions

**Date**: 2026-10-22
**Status**: Ready for implementation

---

## Step 0 — Happy Path

**Given** the e2e suite targets the Next build with all four regression fixes applied (R1–R4), **when** `cd tests/e2e && bun run test` runs, **then** the three previously-failing Next baseline tests (route announcer live-region count, theme FOUC font-class count, not-found fixture match) pass, the `switching the Up Next tab leaves scroll and the history length unchanged` test passes, and all `Bun only` describe blocks are skipped on the Next run.

This sentence anchors all test design below. Error paths and edge cases are variations on it — not equal-priority items.

---

## Test Suite

### R1 — Next Route Announcer Absent (`route-focus-and-announce.spec.ts`)

**Root cause**: The `test.describe('route focus and announcements (Next baseline)', ...)` block lacks a `test.skip(isBunBuild(), ...)` guard, so Next-specific assertions (`role=alert`, `__next-route-announcer__`) run against the Bun build and fail.

#### Unit Tests

**UT-R1-1** — `isBunBuild_should_returnFalse_when_webBuildHelperEnvIsAbsent`
- File: `tests/e2e/helpers/web-build.ts` (tested via Jest or inline Playwright `test`)
- Scenario: `process.env.WEB_BUILD` is unset (or set to any non-`"bun"` value).
- Expected: `isBunBuild()` returns `false`.
- Verifies: the guard does not fire on the Next build (the Next baseline block runs).

**UT-R1-2** — `isBunBuild_should_returnTrue_when_webBuildEnvIsSetToBun`
- File: `tests/e2e/helpers/web-build.ts`
- Scenario: `process.env.WEB_BUILD === "bun"` (or equivalent Bun-build indicator).
- Expected: `isBunBuild()` returns `true`.
- Verifies: the guard fires on the Bun build (the Next baseline block is skipped).

#### Behavioral Acceptance Tests (UX AC from R1)

**BAT-R1-1** — `routeFocusNextBaseline_should_skipAllThreeTests_when_bunBuildRuns`
- Target: `tests/e2e/route-focus-and-announce.spec.ts`
- Precondition: test run targets the Bun build.
- Action: the runner enters the `route focus and announcements (Next baseline)` describe block.
- Expected: all three tests inside (`clicking the History nav link...`, `skip link: Tab reaches it...`, `the History nav item exposes aria-current...`) report status `skipped`, not `passed` or `failed`.
- Also expected: the `route focus and announcements (Bun only)` describe block runs normally (not skipped).

**BAT-R1-2** — `routeFocusNextBaseline_should_findExactlyOneLiveRegionWithRoleAlert_when_nextBuildRunsAfterNavLinkClick`
- Target: `tests/e2e/route-focus-and-announce.spec.ts`
- Precondition: test run targets the Next build; user is on `/review-queue`.
- Action: `shell.navLink(/history/i).click({ force: true })`.
- Expected: `shell.liveRegions.filter({ hasText: HISTORY_TITLE })` has count 1, and the matching element has `role="alert"` (`__next-route-announcer__`).
- This directly verifies the AC in R1: "exactly one live region containing the History page title with `role='alert'`."

---

### R2 — Theme FOUC: Font-Variable-Class and Custom-Theme Assertions (`theme-fouc.spec.ts`)

**Root cause**: The `test.describe('theme FOUC prevention (Next baseline)', ...)` block lacks a `test.skip(isBunBuild(), ...)` guard. The two failing tests assert `next/font`-specific class structure that does not exist in the Bun build.

#### Unit Tests

**UT-R2-1** — `themeFoucNextBaseline_should_skipDescribeBlock_when_bunBuildGuardIsAdded`
- Type: structural/annotation check (verifiable via `grep` or AST).
- Scenario: `tests/e2e/theme-fouc.spec.ts` has `test.skip(isBunBuild(), ...)` as the first statement inside the `theme FOUC prevention (Next baseline)` describe callback.
- Expected: the guard is present and calls `isBunBuild()` (not `!isBunBuild()`), matching the mirror convention of the Bun-only block.

**UT-R2-2** — `themeFoucFontClasses_should_haveIdenticalCountAcrossAllThemes_when_nextBuildRuns`
- File: `tests/e2e/theme-fouc.spec.ts` — the `'font variable classes survive every theme'` test itself.
- Scenario: Next build; `loadWithStorage` called for each of `THEMES` = `['matrix', 'cyberpunk77', 'wh40k', 'clean', 'light', 'dark']`.
- Expected (error path check): `sizes.size === 1` (class count identical across all themes); `shared.length >= 4` (font variable classes present under every theme). If `sizes.size > 1`, the test was previously running on the Bun build where `next/font` classes are absent — the error path is that the count differs.

#### Behavioral Acceptance Tests (UX AC from R2)

**BAT-R2-1** — `themeFoucNextBaseline_should_skipAllFourTests_when_bunBuildRuns`
- Precondition: Bun build.
- Action: runner enters `theme FOUC prevention (Next baseline)`.
- Expected: all four tests (`'each stored theme is applied...'`, `'font variable classes survive every theme'`, `'a custom: theme applies its class...'`, `'an unknown stored theme degrades...'`) are skipped; the `theme FOUC prevention (Bun only, UX G10)` block runs.

**BAT-R2-2** — `themeFoucFontClasses_should_passClassCountAndSharedClassChecks_when_nextBuildRuns`
- Precondition: Next build.
- Action: `'font variable classes survive every theme'` test runs.
- Expected: `sizes.size === 1` (difference of ≤1 is the AC; the test uses exactly 1); `shared.length >= 4`.

**BAT-R2-3** — `themeFoucCustomTheme_should_writesCssPropertiesBeforeFirstPaint_when_nextBuildRuns`
- Precondition: Next build; `stapler-theme: 'custom:e2e'` and a custom theme JSON in localStorage.
- Action: `'a custom: theme applies its class and CSS variables before first paint'` test runs.
- Expected: `facts.classes` contains `'e2e-custom-theme'`; `facts.style` contains `'--e2e-probe'`; `document.documentElement.style.getPropertyValue('--e2e-probe') === '42px'`.

---

### R3 — Not-Found Baseline Changed (`not-found.spec.ts`)

**Root cause**: No `app/not-found.tsx` exists, so the Next App Router has no 404 entry point. Navigating to an unknown path redirected to `/` (Sessions page). The fixture `next-baseline.json`.`notFound` records the old redirect behavior; both Next baseline tests fail now that the app shows the NotFound view instead.

#### Unit Tests

**UT-R3-1** — `notFoundTsx_should_compileWithoutError_when_useClientIsAddedToNotFoundComponent`
- Type: build verification (`make build`).
- Scenario: `web-app/src/app/NotFound.tsx` has `"use client";` as its first line; `web-app/src/app/not-found.tsx` is a Server Component wrapper re-exporting `NotFound as default` and exporting `metadata`.
- Expected: `make build` exits 0; no TypeScript diagnostic `"cannot export 'metadata' from a client component"`.
- Error path: if `"use client"` is missing from `NotFound.tsx` and `not-found.tsx` imports from it while exporting `metadata`, the Next build emits a compile-time error. This test catches that regression.

**UT-R3-2** — `notFoundMetadata_should_exportTitleFromRouteMeta_when_notFoundTsxCreated`
- Type: static import check.
- Scenario: `web-app/src/app/not-found.tsx` imports `NOT_FOUND_META` from `"./route-meta"` and sets `metadata.title = NOT_FOUND_META.title`.
- Expected: `NOT_FOUND_META.title` resolves to `"Page not found - Stapler Squad"` (from `route-meta.ts`).
- Verifies: the fixture update in Story 3.1.2 uses the same value as the source of truth.

#### Integration Test (data source: `next-baseline.json` fixture + live Next build)

**IT-R3-1** — `measureNotFound_should_returnUpdatedFieldsMatchingFixture_when_notFoundTsxWired`
- File: `tests/e2e/not-found.spec.ts`, via `measureNotFound(page)` in `BaselineProbes.ts`.
- Precondition: Next build running; `not-found.tsx` in place; `next-baseline.json`.`notFound` updated.
- Action: `measureNotFound(page)` called for `MISSING_PATH = '/nonexistent'`.
- Expected return value: `{ status: 200, finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`.
- Also verifies: `BaselineProbes.ts` `waitForURL` timeout reduced to ≤500 ms (no 5-second wait when URL stays at `/nonexistent`).

#### Behavioral Acceptance Tests (UX AC from R3)

**BAT-R3-1** — `notFoundNextBaseline_should_matchUpdatedFixture_when_unknownPathNavigated`
- Precondition: Next build; `not-found.tsx` created; `next-baseline.json` updated.
- Action: `'an unknown path answers 200 and ends on the recorded page'` runs.
- Expected: `observed` deep-equals `baseline.notFound` = `{ status: 200, finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`.

**BAT-R3-2** — `notFoundNextBaseline_should_allowNavAfterLandingOnUnknownPath_when_nextBuildRuns`
- Precondition: Next build; after `measureNotFound(page)`.
- Action: `shell.navLink(/history/i).click({ force: true })`.
- Expected: `page.title()` === `'History - Stapler Squad'`. Confirms the second Next baseline test `'the nav still works from the page an unknown path lands on'` passes.

---

### R4 — Tab-Switch Resets Scroll on Both Builds (`scroll-policy.spec.ts`)

**Root cause**: `SCROLLER` in `AppShellPage.ts` picks the element with the largest `scrollHeight - clientHeight`. After a tab switch in Up Next, a different panel becomes the largest scroller, so `scrollTop()` reads a new element that starts at 0. Fixing requires a stable `data-testid="up-next-scroller"` on the outer `UnfinishedTab` container so `SCROLLER` can short-circuit to it.

#### Unit Tests

**UT-R4-1** — `scroller_should_returnTestidElement_when_upNextScrollerPresentAndScrollable`
- Type: Playwright `page.evaluate` of the `SCROLLER` inline expression.
- Scenario: the `/unfinished` page is loaded; `document.querySelector('[data-testid="up-next-scroller"]')` returns a non-null element with `overflowY === 'auto'` and `scrollHeight - clientHeight > 0`.
- Expected: `SCROLLER` returns `{ el: <the testid element>, range: <positive number> }`.
- Verifies: the short-circuit path works when the Up Next page is rendered.

**UT-R4-2** — `scroller_should_fallbackToLargestOverflowScan_when_testidElementAbsent`
- Scenario: a page other than `/unfinished` is loaded (e.g. `/history`); `document.querySelector('[data-testid="up-next-scroller"]')` returns `null`.
- Expected: `SCROLLER` falls through to the `for (const el of document.querySelectorAll('*'))` scan and returns `{ el: <largest scroller>, range: <...> }` (same behavior as the old SCROLLER).
- Error path: if the testid element is `null` but `SCROLLER` returns `null` instead of falling back, `scrollTop()` would throw — this test catches that regression.

#### Integration Test (DOM state across tab switch)

**IT-R4-1** — `upNextTabSwitch_should_preserveScrollerIdentity_when_tabChanges`
- File: `tests/e2e/scroll-policy.spec.ts`, test `'switching the Up Next tab leaves scroll and the history length unchanged'`.
- Precondition: `/unfinished` loaded; viewport `1280x360`; `prs` tab active; `shell.scrollTo(SCROLL_OFFSET)` confirmed via `expect.poll`.
- Action: `upNext.selectTab('queue')` (triggers `history.replaceState`).
- Expected:
  - `shell.scrollTop()` within `SCROLL_TOLERANCE_PX` (2 px) of `SCROLL_OFFSET`.
  - `history.length` after switch equals `historyLength` before switch.
- Verifies: `data-testid="up-next-scroller"` plus the updated `SCROLLER` pin the container identity across the tab change.

#### Behavioral Acceptance Tests (UX AC from R4)

**BAT-R4-1** — `tabSwitch_should_leaveScrollTopWithin2px_when_upNextTabChanges`
- Precondition: user on `/unfinished` with `prs` tab; scrolled to `SCROLL_OFFSET = 40 px`; `scrollRange()` > 40.
- Action: click the `queue` tab.
- Expected: `shell.scrollTop()` is within 2 px of 40. (Fails before fix because SCROLLER switches to a new element at `scrollTop = 0`.)

**BAT-R4-2** — `tabSwitch_should_leaveHistoryLengthUnchanged_when_upNextTabChanges`
- Precondition: same as BAT-R4-1; `history.length` recorded before tab click.
- Action: click the `queue` tab.
- Expected: `history.length` equals the recorded value. (Confirms `replaceState` not `pushState`.)

---

### R5 — Bun POP Scroll Restore (Out of Scope)

R5 is not validated here. `test.fail()` on `'Back restores the previous scroll position (within 2px)...'` in `tests/e2e/scroll-policy.spec.ts` remains in place. No test cases are added for R5.

---

## Coverage Summary

| Requirement | Unit (happy) | Unit (error) | Integration | Behavioral AC | Total |
|---|---|---|---|---|---|
| R1 — route announcer guard | UT-R1-1 | UT-R1-2 | — | BAT-R1-1, BAT-R1-2 | 4 |
| R2 — theme FOUC guard | UT-R2-1 | UT-R2-2 | — | BAT-R2-1, BAT-R2-2, BAT-R2-3 | 5 |
| R3 — not-found unification | UT-R3-1 | UT-R3-2 | IT-R3-1 | BAT-R3-1, BAT-R3-2 | 5 |
| R4 — tab-switch scroll | UT-R4-1 | UT-R4-2 | IT-R4-1 | BAT-R4-1, BAT-R4-2 | 5 |
| R5 — Bun POP restore | (out of scope) | | | | 0 |
| **Total** | **4** | **4** | **2** | **9** | **19** |

**Requirements coverage**: 4 of 4 actionable requirements (R1–R4) have at least one unit, integration, and/or behavioral test. R5 is explicitly excluded by the requirements document. Coverage fraction: **4/4 = 100%** of in-scope requirements.

---

## Verification Checklist (per phase)

After **Phase 1** (R1/R2 guards added):
- [ ] UT-R2-1: guard annotation present in `theme-fouc.spec.ts` (grep check)
- [ ] BAT-R1-1: Bun run of `route-focus-and-announce.spec.ts` — Next baseline block fully skipped
- [ ] BAT-R2-1: Bun run of `theme-fouc.spec.ts` — Next baseline block fully skipped

After **Phase 2** (verify Next still green):
- [ ] BAT-R1-2: Next run of `route-focus-and-announce.spec.ts` — live-region count = 1, `role=alert`
- [ ] BAT-R2-2 + BAT-R2-3: Next run of `theme-fouc.spec.ts` — font class count and custom theme tests pass

After **Phase 3** (R3 not-found wired):
- [ ] UT-R3-1: `make build` exits 0
- [ ] UT-R3-2: `NOT_FOUND_META.title === "Page not found - Stapler Squad"` in `route-meta.ts`
- [ ] IT-R3-1: `measureNotFound` returns `{ status: 200, finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`
- [ ] BAT-R3-1 + BAT-R3-2: both `not found (Next baseline)` tests pass

After **Phase 4** (R4 testid + SCROLLER):
- [ ] UT-R4-1: SCROLLER returns testid element on `/unfinished`
- [ ] UT-R4-2: SCROLLER falls back correctly on `/history`
- [ ] IT-R4-1 / BAT-R4-1 + BAT-R4-2: scroll and history.length preserved after tab switch
- [ ] `bun-expected-diff.json` no longer contains the R4 test entry

After **Phase 5** (Final CI gate):
- [ ] `make ci` exits 0
- [ ] Full e2e suite on Next: R1–R4 pass; R5 still shows `test.fail()` (expected-to-fail, not unexpected fail)
- [ ] Full e2e suite on Bun: Next baseline blocks skipped; Bun-only blocks run
