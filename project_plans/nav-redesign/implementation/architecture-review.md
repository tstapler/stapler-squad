# Architecture Review: nav-redesign

**Date**: 2026-06-24
**Verdict**: CONCERNS

---

## Issues

### CONCERN: `group` required field creates a two-tier problem with special items

The plan makes `group: NavGroup` required on `NavPage` (Story 1.1.1, Task 1.1.1a) and enforces it via TypeScript. This is sound for all 15 entries that remain in `NAV_PAGES` after consolidation. However, three special items in `BottomNav.tsx` — Notifications (custom badge-driven), New Session FAB, and Account — are not `NavPage` entries and never were. They are hard-coded directly in JSX. These items are untouched by the `group` field constraint and are correctly out of scope.

The real risk is the Notifications entry *inside* `NAV_PAGES`. It remains in `NAV_PAGES` (marked `bottomNavPrimary: true`) but is explicitly excluded from `BOTTOM_NAV_PRIMARY` via `p.href !== routes.notifications`. After Phase 1, it will also carry `group: "work"`, meaning `groupNavPages(NAV_PAGES)` will include it in the Work group — and DrawerNav will render it as a plain link (no badge) alongside the other Work items. This is the existing DrawerNav behavior so it is not a regression, but it is a footgun: `groupNavPages()` called on the full `NAV_PAGES` produces a Work group containing an item that BottomNav deliberately special-cases. The plan does not document this caveat, and future callers of `groupNavPages(NAV_PAGES)` may not know to exclude it.

**Recommendation**: Document at the `groupNavPages` export that `NAV_PAGES` includes the Notifications entry which is intentionally special-cased in BottomNav. Alternatively, add a `customRender?: boolean` sentinel on `NavPage` to signal callers that this item must be handled out-of-band. The required `group` field is otherwise correct.

---

### CONCERN: `groupNavPages()` intentionally does not filter empty groups — but callers are not shown the contract

Story 1.1.1 specifies: "empty groups (all items flag-filtered) are excluded by the caller, not the helper." This is a reasonable division of responsibility, but neither the DrawerNav render (Task 2.1.2b) nor the BottomNav render (Task 3.1.2b) shows an explicit empty-group guard. The DrawerNav snippet iterates `Array.from(groups.entries()).map(...)` without a `.filter(([, pages]) => pages.length > 0)` guard. If a flag knocks out every item in a group the helper still returns that group in the Map (it inserts on first encounter before filtering), so the section header will render with no items beneath it.

Wait — re-reading the helper body: `groupNavPages` is called on a pre-filtered `visiblePages` array. If all items in a group are flag-filtered, none of them appear in `visiblePages`, so the group never gets inserted into the Map at all. The guarantee therefore holds transitively, but only if callers always pre-filter. This precondition is met by both DrawerNav (filters before calling) and BottomNav (filters before calling). The contract is correct, but it is invisible; a future caller that passes unfiltered `NAV_PAGES` directly will get ghost headers.

**Recommendation**: Add a JSDoc comment on `groupNavPages` stating: "Pass a pre-filtered array; the function does not filter by featureFlag. Groups with no items will not appear in the result only if the caller has already excluded those items." One sentence prevents a silent bug.

---

### CONCERN: Derived export `BOTTOM_NAV_MORE` semantics change silently breaks existing `isMoreActive` behavior

After Task 1.1.1b removes `mobileNav: false` from Settings, Logs, Errors, Help, Escape Analytics, and Files, those items enter `BOTTOM_NAV_MORE` (because `BOTTOM_NAV_MORE = NAV_PAGES.filter(p => p.mobileNav !== false && !p.bottomNavPrimary)`). The plan notes this in Story 1.1.1 acceptance criteria: "After this story, `BOTTOM_NAV_MORE` includes Settings, Logs, Errors, Help, Escape Analytics, Files."

This is correct and desired. However, `isMoreActive` in `BottomNav.tsx` is computed as:
```typescript
const isMoreActive = morePages.some((item) => pathname?.startsWith(item.href));
```
`morePages` is already `filterByFlag(BOTTOM_NAV_MORE)`. Because Settings, Errors, Logs, etc., now appear in `morePages`, navigating to `/settings` will now light up the "More" button — which is the intended fix. But `/settings?tab=config-files` uses `pathname?.startsWith("/settings")` which matches the single Settings entry correctly. No bug here, but it should be explicitly called out in Story 1.1.3's acceptance criteria that `isMoreActive` now returns `true` for these routes precisely *because* they are in `BOTTOM_NAV_MORE`. The current plan's Story 1.1.3 mentions this as a goal but does not explain the mechanism; if a developer reads only Story 1.1.3 they may look for explicit `isMoreActive` code changes that don't exist.

**Recommendation**: Add one sentence to Story 1.1.3: "No code change to `isMoreActive` is needed — the behavior change is an automatic consequence of the items entering `BOTTOM_NAV_MORE` in Story 1.1.1."

---

### CONCERN: `sectionHeader` uses `recipe()` in DrawerNav but `style()` in BottomNav — inconsistency is fine but should be intentional

DrawerNav's `sectionHeader` (Task 2.1.1a) is a `recipe()` with a `visible` variant. BottomNav's `moreSheetSectionHeader` (Task 3.1.1a) is a plain `style()` with no variant. The difference is appropriate: DrawerNav headers need to hide when the drawer is collapsed (`visible: false` → `opacity: 0, maxWidth: 0`), while BottomNav's More sheet is always either fully open or fully closed (no partial-collapse state). This divergence is architecturally sound.

However, the plan's DrawerNav snippet uses `maxWidth: "0px"` for the hidden state, which causes a brief horizontal reflow on collapse. The `navLabel` in the existing `DrawerNav.css.ts` presumably handles this with `overflow: hidden` + `maxWidth`. The plan includes `overflow: hidden` and a `transition` on `maxWidth`, which should work. The only risk is a brief layout shift if `maxWidth` transitions at a different rate than `navLabel`. This is a minor visual polish issue, not an architectural problem.

**Recommendation**: The approach is sound. If the collapsed state produces a visible reflow, use `width: 0` + `overflow: hidden` instead of `maxWidth: 0` to match whatever pattern the existing `navLabel` recipe uses.

---

### MINOR: Inline `style` in BottomNav More sheet divider (Task 3.1.2b) is flagged but the fix is in the same task

Task 3.1.2b includes an inline `style={{ borderTop: '1px solid var(--border-color)' }}` in the JSX, then immediately flags it for replacement in the same task description: "The divider between nav groups and utility items should use a `style` class, not an inline style." Task 3.1.2c adds `moreSheetUtilitySection`. This is fine — the self-correction is explicit — but the implementation plan describes an intermediate invalid state within a single task. The CSS architecture rules prohibit inline styles that override the cascade.

**Recommendation**: Merge tasks 3.1.2b and 3.1.2c into a single atomic task, or reorder so that `moreSheetUtilitySection` is added to the CSS file in 3.1.1a (alongside the other styles) and referenced from the start. The current plan is executable but creates a window where the code violates the CSS architecture rule mid-task.

---

### MINOR: DrawerNav test file (Story 4.1.2) does not test the collapsed state path for section headers

The DrawerNav test plan (Task 4.1.2a) includes test case 2: "Section headers have zero visible width when drawer is closed (`visible: false` variant — CSS mock returns class strings)." A CSS module mock returns string class names (e.g., `sectionHeader: "sectionHeader"`), so the test will verify that the element has class `"sectionHeader"` — but it cannot verify that `sectionHeader({ visible: false })` produces a *different* class than `sectionHeader({ visible: true })`. With vanilla-extract `recipe()` in tests, the mock flattens all variants to one string.

**Recommendation**: Either (a) assert that the rendered section header element receives a different className when `isDrawerOpen` is false versus true by inspecting the rendered output and checking for the presence of the `visible` prop value in the class string, or (b) accept that this behavior is covered by visual regression / Storybook rather than a Jest unit test. The test as described will not actually catch a regression where `visible: isDrawerOpen` is accidentally hardcoded to `true`.

---

### MINOR: `NAV_PAGES` item count inconsistency in plan text

Story 1.1.1 acceptance criteria states: "TypeScript enforces assignment on all 14 remaining entries after consolidation." Story 1.1.2 acceptance criteria states: "NAV_PAGES has 15 total entries after removal." The current file has 17 entries (counting the comment-marked Notifications entry). Removing Config Files and Features (2 entries) leaves 15. The "14" in Story 1.1.1 is a typo; the consistent number is 15. This does not affect implementation but will cause confusion when a developer counts items.

**Recommendation**: Change "14 remaining entries" in Story 1.1.1 to "15 remaining entries" to match Story 1.1.2.

---

### MINOR: `groupNavPages()` return type uses `Map<NavGroup, NavPage[]>` but the research doc proposes `Map<NavGroup | "ungrouped", NavPage[]>`

The research doc (architecture.md and stack.md) proposed an "ungrouped" fallback key for items with no group. The plan drops this fallback because `group` is made required. This is the right call — the fallback was only needed for the optional-group design. However, the research doc still shows the `"ungrouped"` key in its code samples. No action needed in code, but if the research doc is consulted again it may cause confusion.

**Recommendation**: No code change needed. Optionally strike through or note the outdated "ungrouped" samples in the research doc if it is expected to be consulted during implementation.

---

## Summary

The plan is well-structured and the core architectural choices are sound: required `group` on `NavPage`, `groupNavPages()` on pre-filtered arrays, vanilla-extract `recipe()` for stateful DrawerNav headers and plain `style()` for the More sheet, and no new dependencies. The primary concerns are documentation gaps rather than design flaws: the Notifications entry's dual membership in `NAV_PAGES` and BottomNav's custom-render path is an undocumented footgun for future callers of `groupNavPages()`; the empty-group safety guarantee depends silently on caller pre-filtering; and the `isMoreActive` fix in Story 1.1.3 reads as incomplete without the explanation that the behavior change is mechanical, not code-driven. None of these block implementation, but each should be addressed in the plan or as inline code comments before the PR is opened.