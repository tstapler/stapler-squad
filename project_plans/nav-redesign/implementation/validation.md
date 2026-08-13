# Validation Plan: nav-redesign

**Date**: 2026-06-24

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: All 16+ nav items reachable on mobile (no mobileNav: false exclusions) | `nav-pages.test.ts` | `groupNavPages_should_includeAllNavPages_When_noMobileNavFalseExists` | Unit | Happy path — verify no item has mobileNav: false |
| REQ-1: All 16+ nav items reachable on mobile | `nav-pages.test.ts` | `BOTTOM_NAV_MORE_should_containSettingsLogsErrorsHelpFiles_When_mobileNavFalseRemoved` | Unit | Edge — previously-hidden items are now present in BOTTOM_NAV_MORE |
| REQ-1: All 16+ nav items reachable on mobile | `BottomNav.test.tsx` | `BottomNav_should_renderSettingsInMoreSheet_When_moreSheetIsOpened` | Integration | Happy path — open More sheet, Settings link is present |
| REQ-2: Items rendered in correct groups (Work / Automation / Insights / Settings) | `nav-pages.test.ts` | `groupNavPages_should_placeItemsInCorrectGroupBuckets_When_calledWithNAV_PAGES` | Unit | Happy path — spot check one item per group |
| REQ-2: Items rendered in correct groups | `nav-pages.test.ts` | `groupNavPages_should_preserveInsertionOrder_When_calledWithMixedGroups` | Unit | Edge — group order matches first-encounter order in input array |
| REQ-2: Items rendered in correct groups | `DrawerNav.test.tsx` | `DrawerNav_should_renderAllFourGroupHeaders_When_drawerIsOpen` | Integration | Happy path — "Work", "Automation", "Insights", "Settings & Tools" all present |
| REQ-2: Items rendered in correct groups | `BottomNav.test.tsx` | `BottomNav_should_renderGroupHeadersInMoreSheet_When_moreSheetIsOpened` | Integration | Happy path — section headers appear in More sheet |
| REQ-3: Empty groups don't render section headers | `nav-pages.test.ts` | `groupNavPages_should_returnEmptyMap_When_inputIsEmptyArray` | Unit | Edge — empty array → empty Map |
| REQ-3: Empty groups don't render section headers | `DrawerNav.test.tsx` | `DrawerNav_should_notRenderGroupHeader_When_allGroupItemsAreFeatureFlaggedOff` | Integration | Edge — flag off for all items in a group, header not rendered |
| REQ-3: Empty groups don't render section headers | `BottomNav.test.tsx` | `BottomNav_should_notRenderEmptyGroupHeader_When_allGroupItemsFlaggedOff` | Integration | Edge — More sheet skips header for groups with zero visible items |
| REQ-4: Feature-flagged items hidden when flag is off | `DrawerNav.test.tsx` | `DrawerNav_should_hideFeatureFlaggedItem_When_flagIsOff` | Unit | Happy path — flag off → item absent |
| REQ-4: Feature-flagged items hidden when flag is off | `DrawerNav.test.tsx` | `DrawerNav_should_showFeatureFlaggedItem_When_flagIsOn` | Unit | Edge — flag on → item present |
| REQ-4: Feature-flagged items hidden when flag is off | `BottomNav.test.tsx` | `BottomNav_should_hideBacklogInPrimaryBar_When_backlogFlagIsOff` | Integration | Happy path — derived from BOTTOM_NAV_PRIMARY, not hardcoded |
| REQ-5: Active-state detection works for all items including Settings sub-pages | `nav-pages.test.ts` | `SettingsEntry_should_usePathPrefixHref_When_configFilesEntryRemoved` | Unit | Happy path — Settings href is "/settings", no query-param |
| REQ-5: Active-state detection works for all items | `DrawerNav.test.tsx` | `DrawerNav_should_markSettingsActive_When_pathnameIsSettingsFeatures` | Integration | Happy path — /settings/features activates Settings item |
| REQ-5: Active-state detection works for all items | `DrawerNav.test.tsx` | `DrawerNav_should_markSessionsActive_When_pathnameIsRoot` | Integration | Happy path — / activates Sessions item (aria-current="page") |
| REQ-5: Active-state detection works for all items | `BottomNav.test.tsx` | `BottomNav_should_setMoreButtonActive_When_pathnameIsSettings` | Integration | Edge — isMoreActive returns true for /settings |
| REQ-6: Badges (ReviewQueue, Unfinished, Notifications) render on correct items | `DrawerNav.test.tsx` | `DrawerNav_should_renderReviewQueueBadge_When_reviewQueueItemRendered` | Unit | Happy path — badge present next to Review Queue |
| REQ-6: Badges render on correct items | `DrawerNav.test.tsx` | `DrawerNav_should_renderUnfinishedBadge_When_unfinishedItemRendered` | Unit | Happy path — badge present next to Unfinished |
| REQ-6: Badges render on correct items | `BottomNav.test.tsx` | `BottomNav_should_notRenderNotificationsInMoreSheet_When_notificationsIsPrimary` | Integration | Edge — Notifications stays in primary bar, not duplicated in More sheet |
| REQ-7: More sheet shows grouped sections with headers | `BottomNav.test.tsx` | `BottomNav_should_renderAutomationSectionHeader_When_moreSheetIsOpened` | Integration | Happy path — "Automation" header present |
| REQ-7: More sheet shows grouped sections with headers | `BottomNav.test.tsx` | `BottomNav_should_renderInsightsSectionHeader_When_moreSheetIsOpened` | Integration | Happy path — "Insights" header present |
| REQ-7: More sheet shows grouped sections with headers | `BottomNav.test.tsx` | `BottomNav_should_renderWorkflowsAndRulesInAutomationGroup_When_moreSheetIsOpened` | Integration | Happy path — items appear under the correct section header |
| REQ-8: DrawerNav shows grouped sections with headers | `DrawerNav.test.tsx` | `DrawerNav_should_hideSectionHeaders_When_drawerIsCollapsed` | Unit | Edge — collapsed mode hides headers (visible: false variant) |
| REQ-8: DrawerNav shows grouped sections with headers | `DrawerNav.test.tsx` | `DrawerNav_should_renderSectionSpacerBetweenGroups_When_drawerIsOpen` | Unit | Happy path — sectionSpacer rendered between groups |
| REQ-UTIL: groupNavPages utility correctness | `nav-pages.test.ts` | `groupNavPages_should_accountForAllNAV_PAGES_When_groupsAreSummed` | Unit | Happy path — sum of all group buckets equals NAV_PAGES.length |
| REQ-UTIL: NAV_GROUP_LABELS coverage | `nav-pages.test.ts` | `NAV_GROUP_LABELS_should_haveEntryForAllFourNavGroupValues_When_imported` | Unit | Happy path — all four keys present with non-empty strings |
| REQ-UTIL: Settings consolidation removes Config Files and Features entries | `nav-pages.test.ts` | `NAV_PAGES_should_notContainConfigFilesEntry_When_imported` | Unit | Happy path — no entry with href containing "?tab=config-files" |
| REQ-UTIL: Settings consolidation removes Config Files and Features entries | `nav-pages.test.ts` | `NAV_PAGES_should_notContainFeaturesEntry_When_imported` | Unit | Happy path — no entry with href "/settings/features" |

---

## Test Files and Responsibilities

| File | New or Modified | Responsibility |
|---|---|---|
| `web-app/src/lib/__tests__/nav-pages.test.ts` | **New** | All `groupNavPages()` unit tests + NAV_PAGES data-layer invariants |
| `web-app/src/components/layout/__tests__/BottomNav.test.tsx` | **Modified** | Fix PRIMARY_ITEMS hardcode; add FeatureFlagsContext mock; add More sheet group tests |
| `web-app/src/components/layout/__tests__/DrawerNav.test.tsx` | **New** | All DrawerNav grouped rendering, feature-flag filtering, active-state, badge tests |

---

## Test Stack

- **Unit**: Jest + React Testing Library (RTL) — `@testing-library/react`
- **Integration**: Jest + RTL with mocked Next.js navigation (`jest.mock("next/navigation", ...)`)
- **No Playwright e2e tests for this feature** — UI is structural refactor with no new routes; existing e2e suite covers route reachability

---

## Required Mocks per File

### `nav-pages.test.ts`
No component mounts; no mocks required. Pure function and data tests only.

### `DrawerNav.test.tsx` (new file — mirrors BottomNav.test.tsx structure)
```typescript
jest.mock("next/navigation", () => ({ usePathname: jest.fn() }));
jest.mock("next/link", () => { /* MockLink from BottomNav.test.tsx */ });
jest.mock("@/lib/contexts/NavigationContext", () => ({
  useNavigation: () => ({ isDrawerOpen: true, toggleDrawer: jest.fn() }),
}));
jest.mock("@/components/sessions/ReviewQueueNavBadge", () => ({
  ReviewQueueNavBadge: () => <span data-testid="review-queue-badge" />,
}));
jest.mock("@/components/sessions/UnfinishedNavBadge", () => ({
  UnfinishedNavBadge: () => <span data-testid="unfinished-badge" />,
}));
jest.mock("@/components/sessions/NotificationsNavBadge", () => ({
  NotificationsNavBadge: () => <span data-testid="notifications-badge" />,
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: {} }),  // override per-test for flag-on scenarios
}));
jest.mock("../DrawerNav.css", () => ({
  drawer: "drawer", navList: "navList", navItem: "navItem",
  navIcon: "navIcon", navLabel: "navLabel", navBadgeWrapper: "navBadgeWrapper",
  toggleButton: "toggleButton", drawerDivider: "drawerDivider", badge: "badge",
  sectionHeader: "sectionHeader", sectionSpacer: "sectionSpacer",
}));
```

### `BottomNav.test.tsx` additions
```typescript
// Add after existing useHandedness mock:
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: {} }),
}));

// Extend CSS mock with new class names:
moreSheetScrollable: "moreSheetScrollable",
moreSheetSection: "moreSheetSection",
moreSheetSectionHeader: "moreSheetSectionHeader",
moreSheetUtilitySection: "moreSheetUtilitySection",
```

---

## Test Implementation Notes

### Replacing hardcoded `PRIMARY_ITEMS` in BottomNav.test.tsx
```typescript
import { BOTTOM_NAV_PRIMARY } from "@/lib/nav-pages";

// Derive expected items at test runtime (flags: {} means all feature flags off)
const expectedPrimaryItems = BOTTOM_NAV_PRIMARY.filter((p) => !p.featureFlag);
```

### Opening the More sheet in BottomNav integration tests
```typescript
fireEvent.click(screen.getByRole("button", { name: "More navigation options" }));
// Then query for items/headers within the sheet
```

### Testing collapsed DrawerNav (section headers hidden)
```typescript
// Override NavigationContext mock for specific test:
jest.mocked(useNavigation).mockReturnValue({ isDrawerOpen: false, toggleDrawer: jest.fn() });
// sectionHeader mock returns "sectionHeader" class string; test checks element is rendered
// but has the CSS module's collapsed variant applied (the class string)
```

### Feature-flag overrides per-test in DrawerNav tests
```typescript
import { useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
jest.mocked(useFeatureFlags).mockReturnValue({ flags: { backlog: true } });
```

---

## Coverage Targets

- **Unit test coverage**: ≥80% line coverage on `nav-pages.ts` (groupNavPages, NAV_GROUP_LABELS, NAV_PAGES data)
- **All grouping utility functions**: happy path + all four edge cases (empty input, single item, all same group, insertion-order preservation)
- **All nav components**: group rendering (headers present/absent), mobile visibility, feature-flag filtering
- **Badge invariants**: ReviewQueueNavBadge, UnfinishedNavBadge, NotificationsNavBadge each asserted in DrawerNav tests
- **Active-state bug fixed by removal**: assert no item with query-param href exists in NAV_PAGES after consolidation

---

## Test Count Summary

| File | Unit Tests | Integration Tests | Total |
|---|---|---|---|
| `nav-pages.test.ts` | 9 | 0 | 9 |
| `DrawerNav.test.tsx` | 4 | 7 | 11 |
| `BottomNav.test.tsx` (additions) | 0 | 8 | 8 |
| **Total** | **13** | **15** | **28** |

---

## Requirement Coverage

| Requirement | Tests Covering It | Covered? |
|---|---|---|
| REQ-1: All items reachable on mobile | 3 | Yes |
| REQ-2: Items in correct groups | 5 | Yes |
| REQ-3: Empty groups skip headers | 3 | Yes |
| REQ-4: Feature-flag filtering | 4 | Yes |
| REQ-5: Active-state correctness | 4 | Yes |
| REQ-6: Badges on correct items | 3 | Yes |
| REQ-7: More sheet grouped sections | 4 | Yes |
| REQ-8: DrawerNav grouped sections | 2 | Yes |

**All 8 requirements covered. 28 total test cases (13 unit, 15 integration). 8/8 requirements covered (100%).**
