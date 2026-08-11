# Pre-mortem: nav-redesign

**Date**: 2026-06-24

---

## Scenario: What went wrong

It is September 2026. The nav-redesign shipped three months ago. The post-release incident log shows three categories of problems:

1. **Silent badge regression**: The Review Queue and Unfinished badges disappeared from the More sheet on mobile after the grouped rendering was deployed. The inline `href === routes.reviewQueue` special-case in BottomNav was preserved in the primary bar render path but was missed when the flat `morePages.map()` was replaced with the grouped render — the inner item render loop was copied "verbatim" as the plan instructed, but the verbatim copy was taken from a version of the code that did not include the badge branch. No test caught it because the More sheet items have zero test coverage. Users filed bugs weeks after launch.

2. **TypeScript compiled fine but tests blew up at runtime**: The `NavPage` interface now requires a `group` field. Existing `NavPage` object literals in `BottomNav.test.tsx` (constructed as mock data) had no `group` field. `tsc --noEmit` passed because the test file was not in a strict mode that caught it at the call site — or the literals were typed as `any` in the mock. The test suite silently ran against stale mock data and the FeatureFlagsContext was never mocked, so every test that touched feature-flagged items was running against a live context that threw in JSDOM. CI passed because the test runner suppressed the errors as warnings.

3. **iOS Safari More sheet overflow**: On iPhone SE (375px wide) with iOS Safari's collapsible address bar, the `maxHeight: 70vh` on `moreSheetScrollable` clipped the bottom section headers in the Settings & Tools group. Because `70vh` in iOS Safari is based on the *larger* viewport (address bar collapsed) rather than the current visible viewport, the sheet overflowed behind the browser chrome on scroll. The Handedness toggle and Account link at the bottom of the sheet were completely unreachable. This was not caught in QA because testing was done on a recent iPhone Pro with the address bar behavior disabled.

4. **Header.tsx showed newly-mobile-visible items on desktop**: Settings, Logs, Errors, Help, Escape Analytics, and Files were previously `mobileNav: false`. Removing that field made them available in the More sheet (correct). But Header.tsx renders items where `headerNav !== false`. At least two of the newly-unlocked items (`Escape Analytics`, `Files`) had neither `mobileNav: false` nor `headerNav: false` before the redesign — `mobileNav: false` was the only guard keeping them out of header rendering. After its removal, those items appeared in the desktop header bar, creating an unexpected layout regression.

---

## P1 Risks (must mitigate before implementation)

### Risk 1: Badge regression when rewriting the More sheet item render loop
**Likelihood**: High
**Impact**: High
**Mitigation**: Before touching BottomNav.tsx, audit the existing item render loop for every badge special-case (`href === routes.reviewQueue`, `href === routes.unfinished`). Write a test in `BottomNav.test.tsx` that opens the More sheet and asserts that `ReviewQueueNavBadge` and `UnfinishedNavBadge` are rendered for their respective items. Run that test against the current code (it must pass) before beginning Story 3.1.2. The plan's instruction to copy the inner render loop "verbatim" is insufficient as a safety net — the copy must be done with explicit badge-case verification.

### Risk 2: Header.tsx shows items that were previously hidden only by `mobileNav: false`
**Likelihood**: High
**Impact**: High
**Mitigation**: Before Task 1.1.1b (removing `mobileNav: false`), audit every entry being changed and confirm it also carries `headerNav: false`. The adversarial review flags this but the plan has no blocking gate. Add a required audit table to the implementation checklist: list every item whose `mobileNav: false` is being removed, and record its current `headerNav` value. Any item that does NOT have `headerNav: false` must receive it as part of Task 1.1.1b. This must be done before the PR opens — it cannot be caught post-ship.

### Risk 3: TypeScript compile passes but test NavPage literals lack `group` field, causing silent mock failures
**Likelihood**: High
**Impact**: Medium
**Mitigation**: After making `group` required on `NavPage` (Task 1.1.1a), run `cd web-app && npx tsc --noEmit` against ALL TypeScript files including test files. Then explicitly search for every `NavPage` object literal outside `nav-pages.ts` (in test fixtures, mock helpers, stubs) and add `group: "work"` (or the appropriate group) to each. The plan calls for running `tsc --noEmit` at step 5 but does not name test files as a specific check target. The missing `FeatureFlagsContext` mock in `BottomNav.test.tsx` must also be added in the same commit as the interface change — not deferred to Story 4.1.1.

### Risk 4: iOS Safari More sheet clips content due to `maxHeight: 70vh` dynamic viewport mismatch
**Likelihood**: Medium
**Impact**: High
**Mitigation**: Replace `maxHeight: "70vh"` with `maxHeight: "calc(100dvh - var(--bottom-nav-height, 72px) - env(safe-area-inset-bottom, 0px))"` in `moreSheetScrollable`. This uses the dynamic viewport unit (`dvh`) that tracks the actual visible viewport in iOS Safari with the collapsible address bar, minus the nav bar height already tracked by the ResizeObserver. Manual QA must be run on an iPhone SE or narrow-viewport device with iOS Safari before merging. The `70vh` value has no token backing and no documented reasoning — either justify it with a comment referencing specific device testing or replace it with the dynamic calculation.

---

## P2 Risks (should address but not blocking)

### Risk 5: `groupNavPages()` called with unfiltered `NAV_PAGES` produces ghost section headers
**Likelihood**: Low (within this PR)
**Impact**: Medium
**Mitigation**: Add a JSDoc comment on `groupNavPages()` stating that it must receive a pre-filtered array. The safety guarantee (empty groups do not appear) holds only if the caller pre-filters by feature flag. Future callers reading only the function signature will not know this. Document the contract at the declaration site, not just in the implementation plan.

### Risk 6: `BOTTOM_NAV_MORE` item count is never explicitly asserted after the mobileNav removal
**Likelihood**: Medium
**Impact**: Medium
**Mitigation**: Add a test assertion in `nav-pages.test.ts` (Story 4.1.3): after `mobileNav: false` removal, `BOTTOM_NAV_MORE` must contain exactly N items (enumerate them). This protects against accidental `bottomNavPrimary: true` being set on a newly-unlocked item, which would silently exclude it from the More sheet.

### Risk 7: DrawerNav feature-flag fix is coupled to the grouped rendering refactor
**Likelihood**: Low
**Impact**: Medium
**Mitigation**: The pre-existing bug (DrawerNav renders feature-flagged items regardless of flag) is fixed inside Story 2.1.2, which is also the grouped rendering change. If a visual regression is found in the grouped rendering during QA, the feature-flag fix is blocked too. Consider extracting the `useFeatureFlags()` filter into a separate commit or story within Phase 1 so it can land independently. If the coupling is accepted, note it in the PR description as a deliberate decision.

### Risk 8: `moreSheetUtilitySection` divider uses inline `style` in an intermediate task state
**Likelihood**: Medium
**Impact**: Low
**Mitigation**: Merge tasks 3.1.2b and 3.1.2c into a single atomic task. The plan's code sample for 3.1.2b contains an inline `style={{ borderTop: ... }}` and then tells the implementer to fix it in 3.1.2c. An implementer who commits after 3.1.2b will have code that violates the CSS architecture rules in the repository. Atomic task prevents this window.

### Risk 9: `sectionHeader` recipe `maxWidth: "0px"` hidden state causes horizontal reflow on collapse
**Likelihood**: Medium
**Impact**: Low
**Mitigation**: Verify that the collapsed state uses the same `width: 0; overflow: hidden` technique as the existing `navLabel` recipe in `DrawerNav.css.ts`. If `navLabel` uses `maxWidth` on transition, the approach is consistent. If it uses `width`, change `sectionHeader` to match. A brief visual QA step on the collapsed drawer transition should be included in the PR checklist.

---

## P3 Risks (monitor but acceptable)

### Risk 10: NAV_PAGES item count inconsistency ("14" vs "15") in plan text
**Likelihood**: Certain (documentation error)
**Impact**: Low
**Monitoring**: The count mismatch between Story 1.1.1 ("14 remaining entries") and Story 1.1.2 ("15 total entries") will cause implementer confusion. The test in Story 4.1.3 verifies the real count so this will surface. Fix the typo in Story 1.1.1 to read "15" before implementation starts to avoid confusion.

### Risk 11: Story 1.1.3 provides no testable acceptance criteria of its own
**Likelihood**: Certain (design gap)
**Impact**: Low
**Monitoring**: Story 1.1.3 is a consequence of Story 1.1.2, not an independent deliverable. If it is treated as a standalone story, a developer may spend time looking for code to write. Add a regression assertion on `isMoreActive` returning `true` for `/settings` paths to Story 4.1.1 and retire Story 1.1.3 as a separate story.

### Risk 12: `Notifications` entry renders in the Work group via `groupNavPages(NAV_PAGES)` without documentation
**Likelihood**: Low
**Impact**: Low
**Monitoring**: `Notifications` has `bottomNavPrimary: true` and is rendered custom in BottomNav, but it will appear in the Work group if `groupNavPages` is called on the full `NAV_PAGES`. DrawerNav renders it as a plain link (existing behavior, not a regression). Document this caveat at the `groupNavPages` export so future callers are not confused by the dual-membership of the Notifications entry.

### Risk 13: `isMoreActive` fix for Settings routes has no dedicated test assertion
**Likelihood**: Medium
**Impact**: Low
**Monitoring**: `isMoreActive` now returns `true` for `/settings` because Settings entered `BOTTOM_NAV_MORE` — not because of an explicit code change. If a future refactor re-adds `mobileNav: false` to Settings for any reason, `isMoreActive` will silently regress again. Add a direct test: `expect(isMoreActive("/settings")).toBe(true)` as part of Story 4.1.1's More sheet assertions.
