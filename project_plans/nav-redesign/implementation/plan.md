# Implementation Plan: nav-redesign

**Feature**: Add group taxonomy to nav items; render grouped sections in DrawerNav and BottomNav More sheet; restore mobile access for all currently-hidden routes; consolidate Settings/Config Files/Features into a single nav entry.
**Date**: 2026-06-24
**Status**: Ready for implementation
**ADRs**: decisions/ADR-001-nav-grouping-approach.md

---

## Risky Assumptions

| # | Assumption | Risk if Wrong | Mitigation |
|---|-----------|---------------|-----------|
| A1 | Grouped sections alone are sufficient for feature discoverability — no in-product onboarding (tooltips, first-run overlay) is needed | Power users adapted to the flat list may have muscle-memory disruption; new users may still miss the More sheet entirely | Monitor for any user feedback post-ship; if discoverability complaints arise, add a first-run "tap More to see all features" tooltip as a follow-up |
| A2 | Settings & Tools as the first group in the More sheet is correct — most users open the sheet to reach Settings | If Automation or Insights are higher-frequency for typical users, the order is wrong | Validate with any available analytics on which "More" items are tapped most |
| A3 | The settings page already handles sub-navigation for Config Files and Features as tabs/sub-pages — removing them from nav doesn't orphan any user flow | If `/settings/features` has no in-page link from the Settings page, users can no longer reach Features | Verify `/settings` page has a Features tab or link before shipping (Task 1.0.0 pre-flight) |

---

## Dependency Visualization

```
Phase 0 — Pre-flight
  Task 1.0.0 (Header.tsx audit: verify headerNav: false on all 8 currently-hidden items)
       │
Phase 1 — Data Layer
  Story 1.1.1 (nav-pages.ts: NavGroup type + group field + groupNavPages())
       │
       ├──► Story 1.1.2 (Settings consolidation: remove Config Files + Features entries)
       │
       └──► Story 1.1.3 (Active-state bug fix: Config Files query-param href)

Phase 2 — DrawerNav
  depends on Phase 1
  Story 2.1.1 (DrawerNav.css.ts: sectionHeader + sectionDivider styles)
       │
       └──► Story 2.1.2 (DrawerNav.tsx: grouped rendering + feature-flag filtering)

Phase 3 — BottomNav More Sheet
  depends on Phase 1
  Story 3.1.1 (BottomNav.css.ts: moreSheetSection + moreSheetSectionHeader styles)
       │
       └──► Story 3.1.2 (BottomNav.tsx: grouped More sheet + BOTTOM_NAV_MORE from all non-primary groups)

Phase 4 — Tests
  depends on Phases 1–3
  Story 4.1.1 (BottomNav.test.tsx: fix PRIMARY_ITEMS + add FeatureFlagsContext mock)
  Story 4.1.2 (DrawerNav.test.tsx: new test file for group header rendering)
  Story 4.1.3 (nav-pages.test.ts: unit tests for groupNavPages())
```

---

## Phase 0: Pre-flight Audit

### Task 1.0.0: Header.tsx audit — verify headerNav: false on all 8 currently-hidden items (~3 min)
**Problem**: Eight items currently have `mobileNav: false` which will be removed in Task 1.1.1b. The `HEADER_NAV_PAGES` export filters items where `headerNav !== false`. If any of these 8 items is missing `headerNav: false`, removing `mobileNav: false` will cause them to unexpectedly appear in the desktop header nav bar.

**Items to check** (those currently marked `mobileNav: false`): Settings, Insights, Logs, Errors, Help, Escape Analytics, Files, and the two being removed (Config Files, Features).

**Action**:
- Verify each of the 8 items has `headerNav: false` in `nav-pages.ts`
- If any is missing `headerNav: false`, add it before running Task 1.1.1b
- Specifically check: `Escape Analytics` and `Files` — pitfalls research flagged these as likely missing the flag

**Files**: `web-app/src/lib/nav-pages.ts` (read-only audit; add `headerNav: false` if any are missing)

---

## Phase 1: Data Layer

### Epic 1.1: NavPage interface and data changes
**Goal**: Give every nav item a `group` field; export a `groupNavPages()` utility; remove dead Settings sub-entries from the flat list; fix the Config Files active-state bug at source.

#### Story 1.1.1: Add NavGroup type and groupNavPages utility
**As a** frontend developer, **I want** every NavPage to carry a `group: NavGroup` field and a `groupNavPages()` helper, **so that** DrawerNav and BottomNav can render section headers without reimplementing grouping logic.

**Acceptance Criteria**:
- `NavGroup = "work" | "automation" | "insights" | "settings"` is exported from `nav-pages.ts`
- `NavPage.group` is a required field (not optional) so TypeScript enforces assignment on all 14 remaining entries after consolidation
- `groupNavPages(pages: NavPage[]): Map<NavGroup, NavPage[]>` is exported; it preserves the order groups are first encountered; empty groups (all items flag-filtered) are excluded by the caller, not the helper
- `NAV_GROUP_LABELS: Record<NavGroup, string>` is exported with display strings: `{ work: "Work", automation: "Automation", insights: "Insights", settings: "Settings & Tools" }`
- All existing derived exports (`MOBILE_NAV_PAGES`, `BOTTOM_NAV_PRIMARY`, `BOTTOM_NAV_MORE`) continue to work — no import-site breakage
- After this story, `BOTTOM_NAV_MORE` includes Settings, Logs, Errors, Help, Escape Analytics, Files (previously `mobileNav: false`) because the `mobileNav: false` exclusion is removed from those entries

**Files**:
- `web-app/src/lib/nav-pages.ts`

##### Task 1.1.1a: Add NavGroup type and NAV_GROUP_LABELS (~3 min)
- After the `NavPage` interface declaration, add:
  ```typescript
  export type NavGroup = "work" | "automation" | "insights" | "settings";
  export const NAV_GROUP_LABELS: Record<NavGroup, string> = {
    work: "Work",
    automation: "Automation",
    insights: "Insights",
    settings: "Settings & Tools",
  };
  ```
- Add `group: NavGroup` as a required field on the `NavPage` interface (insert after `featureFlag?: string`)
- Files: `web-app/src/lib/nav-pages.ts`

##### Task 1.1.1b: Assign groups to all NAV_PAGES entries and remove mobileNav: false (~4 min)
- Group assignments (apply to every entry in `NAV_PAGES`):
  - Sessions, Backlog, Unfinished, Review Queue, Notifications → `group: "work"`
  - Workflows, Rules → `group: "automation"`
  - Insights, History, Escape Analytics → `group: "insights"`
  - Settings, Logs, Errors, Help, Files → `group: "settings"`
- Remove `mobileNav: false` from: Settings, Insights, Logs, Errors, Help, Escape Analytics, Files — these are now accessible on mobile via the More sheet's "settings" and "insights" groups
- Keep `headerNav: false` where it exists (that field is unchanged)
- Keep `bottomNavPrimary: true` where it exists (that field is unchanged)
- Files: `web-app/src/lib/nav-pages.ts`

##### Task 1.1.1c: Add groupNavPages() utility function (~2 min)
- Append after the derived array exports:
  ```typescript
  export function groupNavPages(pages: NavPage[]): Map<NavGroup, NavPage[]> {
    const map = new Map<NavGroup, NavPage[]>();
    for (const page of pages) {
      const existing = map.get(page.group);
      if (existing) {
        existing.push(page);
      } else {
        map.set(page.group, [page]);
      }
    }
    return map;
  }
  ```
- Files: `web-app/src/lib/nav-pages.ts`

---

#### Story 1.1.2: Settings consolidation — remove Config Files and Features nav entries
**As a** user, **I want** a single "Settings" nav entry, **so that** I don't have to know to check three separate places for configuration.

**Acceptance Criteria**:
- The `Config Files` entry (`href: routes.settings + "?tab=config-files"`) is removed from `NAV_PAGES`
- The `Features` entry (`href: routes.settingsFeatures`) is removed from `NAV_PAGES`
- The single `Settings` entry (`href: routes.settings`) remains with `group: "settings"`
- `NAV_PAGES` has 15 total entries after removal (was 17 = original 16 + Notifications custom; now 15 real entries, all group-assigned)
- Navigating to `/settings` and `/settings/features` still works — routes are untouched; only the nav entries are removed

**Files**:
- `web-app/src/lib/nav-pages.ts`

##### Task 1.1.2a: Remove Config Files and Features from NAV_PAGES (~2 min)
- Delete the two lines:
  - `{ href: routes.settings + "?tab=config-files", label: "Config Files", icon: SlidersHorizontal, headerNav: false }`
  - `{ href: routes.settingsFeatures, label: "Features", icon: Settings, headerNav: false }`
- Remove the now-unused import: `SlidersHorizontal` from lucide-react (verify it is not used elsewhere in the file)
- Files: `web-app/src/lib/nav-pages.ts`

---

#### Story 1.1.3: Fix Config Files active-state bug (query-param href)
**As a** user on the Config Files settings tab, **I want** the Settings nav entry to appear active, **so that** I have visual confirmation of my location.

**Acceptance Criteria**:
- After Story 1.1.2, the Config Files entry no longer exists in NAV_PAGES, so the `pathname.startsWith("/settings?tab=config-files")` (always-false) bug is eliminated by removal
- The remaining `Settings` entry (`href: routes.settings`) uses the standard `pathname.startsWith("/settings")` check which correctly matches `/settings`, `/settings/features`, and `/settings?tab=config-files`
- `isMoreActive` in BottomNav correctly returns `true` when pathname is `/settings` or any `/settings/*` path
- No change to `DrawerNav.tsx` active-state logic is needed — `pathname.startsWith(page.href)` already works correctly for `/settings`

**Files**:
- `web-app/src/lib/nav-pages.ts` (done by Story 1.1.2)
- No additional code changes needed; verify by running tests after Phase 1

**Note**: The `/settings` prefix collision described in pitfalls §5b (Settings and Features both matching `startsWith("/settings")`) is resolved by consolidation — there is now only one Settings entry, so both `/settings` and `/settings/features` correctly highlight it.

---

## Phase 2: DrawerNav Desktop Grouped Rendering

### Epic 2.1: DrawerNav grouped sections with headers
**Goal**: Replace the flat `NAV_PAGES.map()` in DrawerNav with a two-level render that shows section headers between groups; add feature-flag filtering that DrawerNav currently lacks.

#### Story 2.1.1: Add section header styles to DrawerNav.css.ts
**As a** developer, **I want** CSS tokens for group header labels and section spacing, **so that** DrawerNav can render group names without inline styles.

**Acceptance Criteria**:
- New `sectionHeader` style is exported: small-caps, muted text color (`vars.color.textMuted`), small font size (`vars.fontSize.xs`), padding that aligns with nav items, with `overflow: hidden` + `transition` to match `navLabel`'s show/hide animation
- New `sectionSpacer` style is exported: a small vertical gap between groups (not a divider line)
- No hardcoded hex values; all values reference `vars.*` tokens
- No hardcoded `zIndex` integers

**Files**:
- `web-app/src/components/layout/DrawerNav.css.ts`

##### Task 2.1.1a: Add sectionHeader and sectionSpacer styles (~3 min)
- Append to `DrawerNav.css.ts`:
  ```typescript
  export const sectionHeader = recipe({
    base: {
      padding: `${vars.space[3]} ${vars.space[3]} ${vars.space[1]}`,
      fontSize: vars.fontSize.xs,
      fontWeight: vars.fontWeight.bold,
      color: vars.color.textMuted,
      textTransform: "uppercase",
      letterSpacing: "0.05em",
      overflow: "hidden",
      transition: "opacity 150ms ease, max-width 200ms ease",
      whiteSpace: "nowrap",
      "@media": {
        "(prefers-reduced-motion: reduce)": {
          transition: "none",
        },
      },
    },
    variants: {
      visible: {
        true: { opacity: 1, maxWidth: "200px" },
        false: { opacity: 0, maxWidth: "0px" },
      },
    },
    defaultVariants: { visible: true },
  });

  export const sectionSpacer = style({
    height: vars.space[2],
  });
  ```
- Files: `web-app/src/components/layout/DrawerNav.css.ts`

---

#### Story 2.1.2: Update DrawerNav.tsx to render grouped sections
**As a** desktop user, **I want** the sidebar to show group section headers between navigation items, **so that** I can scan by category rather than reading every item.

**Acceptance Criteria**:
- DrawerNav renders items in groups: Work → Automation → Insights → Settings & Tools
- Each group has a section header (`<li role="presentation">`) containing the group label
- Section headers respect the `isDrawerOpen` state: visible when expanded, hidden when collapsed (same `visible: isDrawerOpen` variant used by `navLabel`)
- Feature-flagged items that are disabled are excluded from render (DrawerNav currently shows all items regardless of flag — this bug is fixed here)
- The `useFeatureFlags()` hook is imported and used to filter `NAV_PAGES` before grouping
- Empty groups (all items flag-filtered) do not render a section header
- The existing badge special-cases (`routes.reviewQueue`, `routes.unfinished`, `routes.notifications`) are preserved exactly — no badge is dropped
- `drawerDivider` between sections replaces `sectionSpacer` if the group is not the last group (optional — either is acceptable; consistency matters more than specific choice)
- Collapsed icon-only mode still works: icons render, labels hidden, section headers hidden

**Files**:
- `web-app/src/components/layout/DrawerNav.tsx`

##### Task 2.1.2a: Import new utilities and contexts (~2 min)
- Add to DrawerNav.tsx imports:
  - `groupNavPages, NAV_GROUP_LABELS, type NavGroup` from `@/lib/nav-pages`
  - `useFeatureFlags` from `@/lib/contexts/FeatureFlagsContext`
  - `sectionHeader, sectionSpacer` from `./DrawerNav.css`
- Remove direct `NAV_PAGES` usage from the render; keep the import for the filter step
- Files: `web-app/src/components/layout/DrawerNav.tsx`

##### Task 2.1.2b: Replace flat map with grouped render (~5 min)
- Replace the `{NAV_PAGES.map((page) => { ... })}` block with:
  ```typescript
  const { flags } = useFeatureFlags();
  const visiblePages = NAV_PAGES.filter((p) => !p.featureFlag || flags[p.featureFlag]);
  const groups = groupNavPages(visiblePages);

  // Render: for each group, emit a section header li + item lis
  {Array.from(groups.entries()).map(([group, pages], groupIndex) => (
    <React.Fragment key={group}>
      {groupIndex > 0 && <li role="presentation" aria-hidden="true"><div className={sectionSpacer} /></li>}
      <li role="presentation" aria-hidden="true">
        <span className={sectionHeader({ visible: isDrawerOpen })}>
          {NAV_GROUP_LABELS[group]}
        </span>
      </li>
      {pages.map((page) => {
        // ... existing item render logic unchanged ...
      })}
    </React.Fragment>
  ))}
  ```
- The inner item render loop is identical to the current `NAV_PAGES.map()` body — copy it verbatim; only the outer structure changes
- Import `React` if not already imported (needed for `React.Fragment`)
- Files: `web-app/src/components/layout/DrawerNav.tsx`

---

## Phase 3: BottomNav More Sheet Grouped Rendering

### Epic 3.1: BottomNav More sheet sections with headers
**Goal**: Replace the flat `morePages.map()` in the More sheet with grouped sections; ensure all previously-hidden routes are now included in `BOTTOM_NAV_MORE`.

#### Story 3.1.1: Add More sheet section styles to BottomNav.css.ts
**As a** developer, **I want** CSS for More sheet section headers and scroll containment, **so that** grouped sections render correctly on small screens.

**Acceptance Criteria**:
- New `moreSheetSection` style: a grouping container, no visual border, just semantic grouping
- New `moreSheetSectionHeader` style: small-caps muted label, left-aligned, horizontal padding matching `moreSheetItem`, vertical padding smaller than item padding, `borderBottom` separator or just top spacing
- New `moreSheetScrollable` style: applied to the sheet's scrollable content area; sets `overflowY: auto`, `maxHeight: 70vh` to prevent overflow on small screens
- No hardcoded hex values or magic `zIndex` numbers
- The `moreSheet` style itself does NOT need `overflow: hidden` or `max-height` — those go on the inner scrollable wrapper

**Files**:
- `web-app/src/components/layout/BottomNav.css.ts`

##### Task 3.1.1a: Add moreSheetSection, moreSheetSectionHeader, moreSheetScrollable styles (~3 min)
- Append to `BottomNav.css.ts`:
  ```typescript
  export const moreSheetScrollable = style({
    overflowY: "auto",
    // Use 100dvh (dynamic viewport height) so the sheet doesn't overflow behind
    // iOS Safari's collapsible address bar. Fall back to 70vh for browsers without dvh support.
    maxHeight: "calc(100dvh - var(--bottom-nav-height, 72px) - env(safe-area-inset-bottom, 0px))",
    "@supports": {
      "not (max-height: 1dvh)": {
        maxHeight: "70vh",
      },
    },
    WebkitOverflowScrolling: "touch",
  });

  export const moreSheetSection = style({
    // Semantic grouping — no visual styling needed; section header provides separation
  });

  export const moreSheetSectionHeader = style({
    display: "block",
    padding: `${vars.space[3]} ${vars.space[6]} ${vars.space[1]}`,
    fontSize: vars.fontSize.xs,
    fontWeight: "700",
    color: vars.color.textMuted,
    textTransform: "uppercase",
    letterSpacing: "0.05em",
    userSelect: "none",
  });
  ```
- Files: `web-app/src/components/layout/BottomNav.css.ts`

---

#### Story 3.1.2: Update BottomNav.tsx More sheet to render grouped sections
**As a** mobile user, **I want** the "More" sheet to show all routes organized by section, **so that** I can find Settings, Insights, Logs, and other previously-hidden routes on mobile.**

**Acceptance Criteria**:
- The More sheet renders items grouped by `NavGroup`: Automation, Insights, Settings & Tools (Work items go in the primary bar, not the More sheet)
- Each group in the More sheet has a `<span className={styles.moreSheetSectionHeader}>` label
- Empty groups (all items feature-flag filtered) render no header
- The scrollable content area (groups + items) is wrapped in `moreSheetScrollable` to prevent overflow on small screens
- The Handedness toggle and Account link remain at the bottom of the sheet, separated from nav groups by a `drawerDivider`-equivalent (`borderTop` on the utility section or a divider element) — these are NOT part of any `NavGroup`
- `isMoreActive` computation still works: `morePages.some(item => pathname?.startsWith(item.href))` — since `morePages` now includes the previously-hidden items, Settings and others correctly light up the More button
- The CSS module mock in `BottomNav.test.tsx` must be updated to include the new class names (`moreSheetScrollable`, `moreSheetSection`, `moreSheetSectionHeader`)
- Primary bar items (Sessions, Backlog, Unfinished, Review Queue, Notifications) are excluded from the More sheet — `BOTTOM_NAV_MORE` filters them out via `!p.bottomNavPrimary`, which is unchanged
- The More sheet `<div role="navigation">` gains `aria-modal="true"` to signal to screen readers that focus is contained (full focus trap is deferred; this is the minimum viable accessibility fix)

**Files**:
- `web-app/src/components/layout/BottomNav.tsx`

##### Task 3.1.2a: Import new utilities and styles (~2 min)
- Add to BottomNav.tsx imports:
  - `groupNavPages, NAV_GROUP_LABELS` from `@/lib/nav-pages`
  - The new style names from `./BottomNav.css`: `moreSheetScrollable`, `moreSheetSection`, `moreSheetSectionHeader`
- Files: `web-app/src/components/layout/BottomNav.tsx`

##### Task 3.1.2b: Replace flat morePages.map() with grouped render (~5 min)
- In the More sheet JSX, replace:
  ```tsx
  {morePages.map((item) => { ... })}
  ```
  with:
  ```tsx
  <div className={styles.moreSheetScrollable}>
    {/* Sort so Settings & Tools appears first — it is the highest-frequency group in the More sheet */}
    {Array.from(groupNavPages(morePages).entries()).sort(([a], [b]) =>
      a === "settings" ? -1 : b === "settings" ? 1 : 0
    ).map(([group, pages]) => (
      <section key={group} className={styles.moreSheetSection} aria-label={NAV_GROUP_LABELS[group]}>
        <span className={styles.moreSheetSectionHeader} aria-hidden="true">
          {NAV_GROUP_LABELS[group]}
        </span>
        {pages.map((item) => {
          // ... existing item render logic unchanged ...
        })}
      </section>
    ))}
    <div style={{ borderTop: `1px solid var(--border-color)` }}>
      {/* Handedness toggle */}
      ...existing handedness button...
      {/* Account link */}
      ...existing account link...
    </div>
  </div>
  ```
- The divider between nav groups and utility items should use a `style` class, not an inline style. Add a `moreSheetUtilitySection` style to `BottomNav.css.ts` with `borderTop: \`1px solid ${vars.color.borderColor}\`` and update this task accordingly.
- Files: `web-app/src/components/layout/BottomNav.tsx`, `web-app/src/components/layout/BottomNav.css.ts`

##### Task 3.1.2c: Add moreSheetUtilitySection style (~2 min)
- Add to `BottomNav.css.ts`:
  ```typescript
  export const moreSheetUtilitySection = style({
    borderTop: `1px solid ${vars.color.borderColor}`,
    marginTop: vars.space[1],
  });
  ```
- Update the More sheet JSX to use `styles.moreSheetUtilitySection` instead of the inline `style` noted in task 3.1.2b
- Files: `web-app/src/components/layout/BottomNav.css.ts`, `web-app/src/components/layout/BottomNav.tsx`

---

## Phase 4: Tests

### Epic 4.1: Test coverage for grouping logic and nav components
**Goal**: Fix the hardcoded PRIMARY_ITEMS breakage risk in BottomNav.test.tsx; add a FeatureFlagsContext mock; write a new DrawerNav test file; unit-test groupNavPages().

#### Story 4.1.1: Fix BottomNav.test.tsx — PRIMARY_ITEMS and FeatureFlagsContext mock
**As a** developer, **I want** BottomNav tests to derive primary item expectations from `BOTTOM_NAV_PRIMARY` rather than a hardcoded list, **so that** adding or reordering primary items does not silently break tests.

**Acceptance Criteria**:
- `BottomNav.test.tsx` no longer hardcodes `PRIMARY_ITEMS` as a literal array
- The test derives expected labels from `BOTTOM_NAV_PRIMARY` (filtered to exclude feature-flagged items, since `FeatureFlagsContext` mock returns empty flags)
- A `FeatureFlagsContext` mock is added: `jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({ useFeatureFlags: () => ({ flags: {} }) }))`
- The CSS module mock in `BottomNav.test.tsx` is extended to include the new class names: `moreSheetScrollable`, `moreSheetSection`, `moreSheetSectionHeader`, `moreSheetUtilitySection`
- All existing BottomNav tests continue to pass

**Files**:
- `web-app/src/components/layout/__tests__/BottomNav.test.tsx`

##### Task 4.1.1a: Add FeatureFlagsContext mock and update CSS mock (~3 min)
- Add after the existing `useHandedness` mock:
  ```typescript
  jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
    useFeatureFlags: () => ({ flags: {} }),
  }));
  ```
- Extend the `BottomNav.css` mock object to include:
  ```typescript
  moreSheetScrollable: "moreSheetScrollable",
  moreSheetSection: "moreSheetSection",
  moreSheetSectionHeader: "moreSheetSectionHeader",
  moreSheetUtilitySection: "moreSheetUtilitySection",
  ```
- Files: `web-app/src/components/layout/__tests__/BottomNav.test.tsx`

##### Task 4.1.1b: Replace hardcoded PRIMARY_ITEMS with derived expectations (~4 min)
- Import `BOTTOM_NAV_PRIMARY` at the top of the test file (after jest.mock calls):
  ```typescript
  import { BOTTOM_NAV_PRIMARY } from "@/lib/nav-pages";
  ```
- Replace the hardcoded `PRIMARY_ITEMS` constant with:
  ```typescript
  // Derive expected items: exclude feature-flagged items (flags: {} means all flags off)
  const expectedPrimaryItems = BOTTOM_NAV_PRIMARY.filter((p) => !p.featureFlag);
  ```
- Update the "renders all primary nav items" test to iterate `expectedPrimaryItems` and use `p.shortLabel ?? p.label` to match what BottomNav renders
- Remove the `remaining.size === 0` assertion style and replace with a direct `expect(screen.getByText(...))` per item
- Files: `web-app/src/components/layout/__tests__/BottomNav.test.tsx`

---

#### Story 4.1.2: Write DrawerNav.test.tsx
**As a** developer, **I want** DrawerNav unit tests, **so that** grouped rendering and feature-flag filtering have a safety net against regressions.

**Acceptance Criteria**:
- `web-app/src/components/layout/__tests__/DrawerNav.test.tsx` is created
- Tests cover:
  1. Renders section headers ("Work", "Automation", "Insights", "Settings & Tools") when drawer is open
  2. Section headers have zero visible width when drawer is closed (`visible: false` variant — CSS mock returns class strings)
  3. Feature-flagged items are excluded when flag is off
  4. Feature-flagged items appear when flag is on
  5. Active item receives `aria-current="page"` for a known route
- Mocks required: `next/navigation`, `next/link`, `NavigationContext`, `ReviewQueueNavBadge`, `UnfinishedNavBadge`, `NotificationsNavBadge`, `FeatureFlagsContext`, `DrawerNav.css`
- Test naming convention: `DrawerNav_should_<effect>_When_<condition>`

**Files**:
- `web-app/src/components/layout/__tests__/DrawerNav.test.tsx` (new file)

##### Task 4.1.2a: Create DrawerNav.test.tsx with mocks and baseline tests (~5 min)
- Create `web-app/src/components/layout/__tests__/DrawerNav.test.tsx`
- Mock structure mirrors `BottomNav.test.tsx`: mock `next/navigation`, `next/link`, `NavigationContext`, badge components, CSS module, `FeatureFlagsContext`
- `NavigationContext` mock: `useNavigation: () => ({ isDrawerOpen: true, toggleDrawer: jest.fn() })`
- CSS module mock: return class name strings for all exported names (`drawer`, `navList`, `navItem`, `navIcon`, `navLabel`, `sectionHeader`, `sectionSpacer`, `navBadgeWrapper`, `toggleButton`, `drawerDivider`, `badge`)
- Write 5 tests as specified in Acceptance Criteria
- Files: `web-app/src/components/layout/__tests__/DrawerNav.test.tsx`

---

#### Story 4.1.3: Unit tests for groupNavPages()
**As a** developer, **I want** unit tests for `groupNavPages()`, **so that** the grouping logic is verified in isolation before the components use it.

**Acceptance Criteria**:
- `web-app/src/lib/__tests__/nav-pages.test.ts` is created (or appended to if it already exists)
- Tests cover:
  1. Items are placed in the correct group bucket
  2. Group order matches insertion order (Map preserves insertion order)
  3. An empty array returns an empty Map
  4. All 15 NAV_PAGES entries (after consolidation) are accounted for across the four groups
  5. `NAV_GROUP_LABELS` has entries for all four NavGroup values
- Test naming convention: `groupNavPages_should_<effect>_When_<condition>`

**Files**:
- `web-app/src/lib/__tests__/nav-pages.test.ts` (new file)

##### Task 4.1.3a: Create nav-pages.test.ts (~4 min)
- Create `web-app/src/lib/__tests__/nav-pages.test.ts`
- Import `groupNavPages`, `NAV_PAGES`, `NAV_GROUP_LABELS`, `NavGroup` from `@/lib/nav-pages`
- Write 5 tests as specified in Acceptance Criteria
- Verify the total item count: `groupNavPages(NAV_PAGES)` entries across all groups sum to `NAV_PAGES.length`
- Files: `web-app/src/lib/__tests__/nav-pages.test.ts`

---

## Implementation Order Summary

Execute in this order to minimize broken-state time:

0. **Task 1.0.0** — Header.tsx pre-flight audit (add headerNav: false where missing)
1. **Task 1.1.1a** — add NavGroup type and NAV_GROUP_LABELS
2. **Task 1.1.1b** — assign groups + remove mobileNav: false
3. **Task 1.1.2a** — remove Config Files and Features entries
4. **Task 1.1.1c** — add groupNavPages() utility
5. Run `cd web-app && npx tsc --noEmit` — verify no TypeScript errors from data changes
6. **Task 2.1.1a** — add DrawerNav section header styles
7. **Task 2.1.2a** — import new utilities in DrawerNav.tsx
8. **Task 2.1.2b** — replace flat map with grouped render in DrawerNav
9. **Task 3.1.1a** — add BottomNav More sheet section styles (including moreSheetUtilitySection)
10. **Task 3.1.2a** — import new utilities in BottomNav.tsx
11. **Task 3.1.2b + 3.1.2c** — replace flat morePages.map() with grouped render
12. **Task 4.1.1a** — add FeatureFlagsContext mock + CSS mock update to BottomNav.test.tsx
13. **Task 4.1.1b** — replace hardcoded PRIMARY_ITEMS
14. **Task 4.1.2a** — create DrawerNav.test.tsx
15. **Task 4.1.3a** — create nav-pages.test.ts
16. Run `make build && make test` — full validation

---

## Bugs Fixed by This Plan

| Bug | Location | Fix |
|-----|----------|-----|
| Config Files active-state never highlights (query-param href) | `nav-pages.ts` line 52 | Entry removed; Settings entry (`/settings`) correctly matches all settings routes |
| 8 routes unreachable on mobile (`mobileNav: false`) | `nav-pages.ts` lines 46–58 | `mobileNav: false` removed; items now in `BOTTOM_NAV_MORE` via group membership |
| DrawerNav renders feature-flagged items regardless of flag | `DrawerNav.tsx` line 34 | `useFeatureFlags()` filter added before grouping (Story 2.1.2) |
| Empty group header renders when all items are flag-filtered | `BottomNav.tsx` (future risk) | `groupNavPages()` is called on pre-filtered array; empty groups produce no header |
| BottomNav.test.tsx PRIMARY_ITEMS hardcode breaks on any primary item change | `BottomNav.test.tsx` line 77 | Derived from `BOTTOM_NAV_PRIMARY` (Story 4.1.1) |
| BottomNav.test.tsx missing FeatureFlagsContext mock | `BottomNav.test.tsx` | Mock added (Story 4.1.1) |

---

## Out of Scope (confirmed)

- Header.tsx grouped rendering (hamburger menu) — same pattern applies but not required by requirements
- Settings page internal tab navigation — settings page sub-nav is out of scope
- Keyboard focus trap in More sheet — WCAG 2.1 SC 2.1.2 technically requires one for modal-like overlays; the More sheet is styled as a bottom sheet but is not a true ARIA `dialog`. Implementing a full focus trap is deferred to a follow-up accessibility PR. **Mitigation**: add `aria-modal="true"` to the More sheet element in Story 3.1.2 so screen readers treat it correctly, and document the known gap in the PR description
- Accordion/collapsible DrawerNav sections — always-expanded per requirements
- URL changes — no routes modified
