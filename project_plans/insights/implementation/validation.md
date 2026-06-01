# Validation Plan: Insights Page Enhancement

**Feature**: Insights Page Enhancement (R1–R7)
**Date**: 2026-05-31
**Status**: Ready for implementation gate

---

## 1. Test Coverage Matrix

| Req | Acceptance Criterion | Unit Tests | Component Tests | E2E Tests |
|-----|----------------------|------------|-----------------|-----------|
| R1 | Filter bar visible with 5 preset options | — | CT-01 | E2E-01 |
| R1 | Custom date range picker shown when "Custom" selected | — | CT-02 | E2E-02 |
| R1 | Changing filter re-fetches and re-renders | UT-01 | CT-03 | E2E-03 |
| R1 | Active filter persists through live-update cycles | UT-02 | CT-04 | E2E-04 |
| R1 | Active filter clearly indicated in UI | — | CT-01 | — |
| R1 | Backend RPC includes from/to timestamps | UT-01 | — | E2E-03 |
| R2 | Session row clickable, opens slide-over | — | CT-05 | E2E-05 |
| R2 | Detail shows metadata: ID, model, path, cost, message count, date range | — | CT-06 | E2E-05 |
| R2 | Detail shows tools breakdown (top_tools) | — | CT-06 | — |
| R2 | Detail shows skill activations list | — | CT-06 | — |
| R2 | Close button and Escape key close drawer | — | CT-07 | E2E-06 |
| R2 | Close returns to insights with filters intact | — | CT-08 | E2E-06 |
| R2 | Loading and empty states handled | — | CT-09 | — |
| R3 | "Projected this month" card shown when ≥7 days data | UT-03 | CT-10 | E2E-07 |
| R3 | Card hidden when <7 days of data | UT-03 | CT-11 | — |
| R3 | Projection formula correct: (monthSpend/days)×daysInMonth | UT-04 | — | — |
| R3 | Budget threshold stored in localStorage | UT-05 | CT-12 | — |
| R3 | Warning styling when projected > threshold | — | CT-13 | E2E-08 |
| R3 | Warning banner at top of page when over budget | — | CT-14 | E2E-08 |
| R4 | Skeleton shown immediately on mount before RPC response | — | CT-15 | E2E-09 |
| R4 | Charts show loading overlay, not blocking render | — | CT-16 | — |
| R4 | First meaningful paint (summary cards) < 200ms | — | — | PERF-01 |
| R4 | Charts lazy-load independently via dynamic() | — | CT-16 | — |
| R5 | TableVirtuoso used when sessions > 50 rows | — | CT-17 | — |
| R5 | Table scrolls smoothly at 500 sessions | — | — | PERF-02 |
| R5 | Sort/filter re-render < 100ms after change | — | — | PERF-03 |
| R6 | Only changed data regions re-render on live update | UT-06 | CT-18 | — |
| R6 | Scroll position not reset on live update | — | CT-19 | E2E-10 |
| R6 | Live update indicator remains visible | — | CT-20 | E2E-10 |
| R7 | Text search filters by project path (case-insensitive) | UT-07 | CT-21 | E2E-11 |
| R7 | Model filter dropdown filters by model family | UT-08 | CT-22 | E2E-11 |
| R7 | Filters work client-side (no extra RPC) | UT-07, UT-08 | — | — |
| R7 | Filter state preserved when live update arrives | UT-09 | CT-23 | — |
| R7 | Clear filter button resets all filters | — | CT-24 | E2E-12 |

---

## 2. Unit Tests

Tests in `web-app/src/lib/hooks/__tests__/` and `web-app/src/lib/utils/__tests__/`.

### UT-01: `useInsightsService` — from/to timestamps passed to RPC

**File**: `web-app/src/lib/hooks/__tests__/useInsightsService.test.ts`
**Test name**: `useInsightsService_should_includeFromToTimestamps_When_filtersHaveDateRange`
**Verifies**:
- `GetInsightsSummaryRequest` proto contains `from` and `to` Timestamp fields when filters provide them
- `WatchInsightsRequest` proto contains `from` and `to` when filters provide them
- Neither field is present when filters have no date range (no spurious `undefined` field)
**Method**: Mock the ConnectRPC transport; capture the outgoing request object; assert `request.from` and `request.to` match `Timestamp.fromDate(filterDate)`.

### UT-02: `useInsightsService` — filter dates stable, no infinite re-fetch

**File**: `web-app/src/lib/hooks/__tests__/useInsightsService.test.ts`
**Test name**: `useInsightsService_should_notRestartStream_When_filterObjectReferenceChangesButValuesDoNot`
**Verifies**:
- Passing a new `Date` object with the same ISO timestamp does not trigger a new `fetchSummary` call
- `startWatch` is not recreated when `filters.from` has the same time value but a new object reference
**Method**: Render hook twice with `filters.from = new Date(sameISOString)`; assert `fetchSummary` mock called exactly once total.

### UT-03: `useProjectedCost` — ≥7 day guard

**File**: `web-app/src/lib/hooks/__tests__/useProjectedCost.test.ts`
**Test name**: `useProjectedCost_should_returnNull_When_fewerThan7DaysInCurrentMonth`
**Verifies**: Returns `null` when daily buckets for the current month span < 7 distinct days.
**Also covers**: Returns non-null result when ≥7 days present.

### UT-04: `useProjectedCost` — projection formula

**File**: `web-app/src/lib/hooks/__tests__/useProjectedCost.test.ts`
**Test name**: `useProjectedCost_should_computeCorrectProjection_When_given10DaysOfSpend`
**Verifies**:
- Formula: `(sum of daily costs in current month) / (count of days with data) × (days in calendar month)`
- Cross-month data (prior months) is excluded from the calculation
- `daysData` and `daysInMonth` fields in return value are correct
**Method**: Construct synthetic `DailyTokenBucket[]` with known values; freeze clock with `jest.useFakeTimers`; assert return value matches hand-computed projection.

### UT-05: `useBudgetThreshold` — localStorage persistence + SSR hydration guard

**File**: `web-app/src/lib/hooks/__tests__/useBudgetThreshold.test.ts`
**Test name**: `useBudgetThreshold_should_loadFromLocalStorage_When_hydrated`
**Also**: `useBudgetThreshold_should_returnIsHydratedFalse_When_mountedWithoutEffect`
**Verifies**:
- `isHydrated` is `false` before the effect runs, `true` after
- `threshold` loaded from `localStorage` key `insights_budget_threshold_usd` on hydration
- `setThreshold(null)` removes the localStorage entry
- `setThreshold(100)` writes `"100"` to localStorage
**Method**: Mock `localStorage` with `jest.spyOn`; use `act()` to flush effects.

### UT-06: Surgical session patch — `setSummary` updater in `useInsightsService`

**File**: `web-app/src/lib/hooks/__tests__/useInsightsService.test.ts`
**Test name**: `useInsightsService_should_patchSingleSession_When_updateEventArrives`
**Verifies**:
- An `InsightsEvent` with `eventType === "update"` and a `session` field patches only that session in `summary.sessions`
- Other sessions in the array are not replaced (reference equality)
- A session not yet in the array is appended
- `fetchSummary` is NOT called for `"update"` events with a session field
**Method**: Feed synthetic events through the event handler function extracted from the hook; assert state shape.

### UT-07: Session search — Fuse.js path filtering

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `sessionSearch_should_filterByProjectPath_When_searchTextProvided`
**Verifies**:
- Substring match on `projectPath`, case-insensitive
- Non-matching sessions are excluded
- Clearing search text restores full list
**Method**: Render `SessionsTable` with fixture data; type into search input; assert row count.

### UT-08: Model filter — dropdown filter logic

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `modelFilter_should_showOnlyMatchingModel_When_modelSelected`
**Verifies**:
- Selecting a model from the dropdown hides sessions that don't match
- Selecting the empty/all option restores full list
- `uniqueModels` derived list contains distinct model values from sessions prop

### UT-09: Filter state stability — local state not reset on prop change

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `sessionTableFilters_should_preserveSearchText_When_sessionsPropUpdates`
**Verifies**:
- After user types a search query, re-rendering with a new `sessions` array (simulating a live update) does not clear `searchText`
- `modelFilter` also unchanged on prop update

---

## 3. Component Tests (RTL)

Tests in `web-app/src/app/insights/__tests__/`.

### CT-01: `TimeRangeFilter` — preset buttons render and active state

**File**: `web-app/src/app/insights/__tests__/TimeRangeFilter.test.tsx`
**Test name**: `TimeRangeFilter_should_renderAllPresets_When_mounted`
**Verifies**: 5 preset buttons visible (Today, Last 7 days, Last 30 days, Last 90 days, All time); active preset has `presetButtonActive` class applied.

### CT-02: `TimeRangeFilter` — custom range inputs shown conditionally

**File**: `web-app/src/app/insights/__tests__/TimeRangeFilter.test.tsx`
**Test name**: `TimeRangeFilter_should_showDateInputs_When_customPresetSelected`
**Verifies**: Two `<input type="date">` elements hidden initially; appear after clicking "Custom"; disappear when switching to a preset.

### CT-03: `InsightsDashboard` — filter change triggers re-fetch

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_refetchSummary_When_presetChanges`
**Verifies**: Clicking a different time preset causes `useInsightsSummary` to call `fetchSummary` with updated `from`/`to` values; mock hook captures call args.

### CT-04: `InsightsDashboard` — live update does not change URL params

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_notChangeUrlParams_When_liveUpdateArrives`
**Verifies**: After a synthetic live update event, `router.replace` is not called with new params; `useSearchParams` read values unchanged.

### CT-05: `SessionsTable` — row click opens drawer

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_callOnSessionClick_When_rowClicked`
**Verifies**: Clicking a `<tr>` row invokes `onSessionClick` with the correct `SessionTokenSummary` object. Also: Enter key press on row fires same callback (keyboard accessibility).

### CT-06: `SessionDetailDrawer` — renders all metadata sections

**File**: `web-app/src/app/insights/__tests__/SessionDetailDrawer.test.tsx`
**Test name**: `SessionDetailDrawer_should_renderMetadataToolsAndSkills_When_sessionProvided`
**Verifies**:
- Session ID, model, project path, cost, message count, date range displayed
- Cache hit rate displayed
- `top_tools` rows rendered (tool name, count, MCP server columns)
- `skill_activations` items rendered as badges
- Empty state for skill activations shown when array is empty

### CT-07: `SessionDetailDrawer` — close on button click and Escape

**File**: `web-app/src/app/insights/__tests__/SessionDetailDrawer.test.tsx`
**Test name**: `SessionDetailDrawer_should_callOnClose_When_closeButtonClicked`
**Also**: `SessionDetailDrawer_should_callOnClose_When_EscapeKeyPressed`
**Verifies**: Close button triggers `onClose` prop; `keydown` event with `Escape` triggers `onClose`.

### CT-08: `InsightsDashboard` — filters intact after drawer close

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_preserveTimeRangeFilter_When_drawerClosed`
**Verifies**: URL params `preset`, `from`, `to` unchanged before and after drawer open/close cycle.

### CT-09: `SessionDetailDrawer` — null session renders nothing

**File**: `web-app/src/app/insights/__tests__/SessionDetailDrawer.test.tsx`
**Test name**: `SessionDetailDrawer_should_renderNothing_When_sessionIsNull`
**Verifies**: When `session={null}`, no portal content is mounted (drawer and overlay absent from DOM).

### CT-10: `ProjectedCostCard` — renders when projection non-null

**File**: `web-app/src/app/insights/__tests__/ProjectedCostCard.test.tsx`
**Test name**: `ProjectedCostCard_should_renderProjectedAmount_When_projectionProvided`
**Verifies**: Dollar amount rendered; "Based on N days" subtext present; budget input visible.

### CT-11: `InsightsDashboard` — projected card absent when data < 7 days

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_notRenderProjectedCard_When_insufficientData`
**Verifies**: When `useProjectedCost` returns `null`, `ProjectedCostCard` is not in the DOM.

### CT-12: `ProjectedCostCard` — threshold persisted to localStorage

**File**: `web-app/src/app/insights/__tests__/ProjectedCostCard.test.tsx`
**Test name**: `ProjectedCostCard_should_saveThresholdToLocalStorage_When_inputChanges`
**Verifies**: Typing a value into the budget input writes to `localStorage` key `insights_budget_threshold_usd`.

### CT-13: `ProjectedCostCard` — warning styling when over budget

**File**: `web-app/src/app/insights/__tests__/ProjectedCostCard.test.tsx`
**Test name**: `ProjectedCostCard_should_applyWarningVariant_When_projectedExceedsThreshold`
**Verifies**: When `projectedMonthly > threshold` and `isHydrated`, card element has the `cardWarning` class (amber styling); when under threshold, warning class absent.

### CT-14: `InsightsDashboard` — warning banner at top when over budget

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_showWarningBanner_When_projectedExceedsThreshold`
**Verifies**: When `isOverBudget` derived state is true, a banner element is present at the top of the dashboard with appropriate ARIA role (`role="alert"`).

### CT-15: `InsightsDashboardSkeleton` — renders on initial mount before data

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_showSkeleton_When_summaryIsLoading`
**Verifies**: When the hook returns `isLoading: true` and `summary: null`, the skeleton component is rendered and the real summary cards are not.

### CT-16: Dynamic chart loading — Skeleton shown as chart fallback

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_renderChartSkeleton_When_chartNotYetHydrated`
**Verifies**: Before lazy-loaded chart chunks resolve, `<Skeleton>` placeholders occupy the chart areas (not empty space, not a crash).

### CT-17: `SessionsTable` — `TableVirtuoso` used above 50-row threshold

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_useTableVirtuoso_When_rowCountExceeds50`
**Verifies**: With 51+ sessions, the component renders a `TableVirtuoso` element (identifiable by `data-testid="virtuoso-table"` or component name). With ≤50 sessions, plain `<table>` rendered.

### CT-18: Surgical patch — chart components do not re-render on unrelated update

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_notRerenderCharts_When_unrelatedSessionUpdates`
**Verifies**: Memoized chart components wrapped in `React.memo` do not re-render when a single session patch arrives (only the sessions array reference changes, not `daily` or `models` data).
**Method**: Spy on chart render function; fire a synthetic "update" event; assert render count unchanged.

### CT-19: Live update — scroll position not reset in SessionsTable

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_maintainScrollPosition_When_liveUpdateArrivesWithVirtuoso`
**Verifies**: `TableVirtuoso` `data` prop is updated immutably; `scrollTo` / `scrollToIndex` not called during a patch update (would indicate a jump).

### CT-20: Live update indicator — pulsing dot visible during update

**File**: `web-app/src/app/insights/__tests__/InsightsDashboard.test.tsx`
**Test name**: `InsightsDashboard_should_showLiveIndicator_When_isLiveUpdatingTrue`
**Verifies**: When hook returns `isLiveUpdating: true`, the live indicator element is present in the DOM; when `isLiveUpdating: false` (after update settles), indicator is still present but not in pulsing state.

### CT-21: Text search — case-insensitive substring match

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_filterCaseInsensitively_When_searchTextTyped`
**Verifies**: Typing "projects" matches `/home/user/Projects/foo`; typing "CLAUDE" matches paths with "claude".

### CT-22: Model filter — dropdown shows unique models

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_showUniqueModels_When_sessionsHaveMixedModels`
**Verifies**: Dropdown options exactly match the deduplicated set of model values from the sessions prop; selecting one hides non-matching rows.

### CT-23: Filter survival — search state preserved on sessions prop update

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_preserveSearchAndModelFilter_When_sessionsPropUpdated`
**Verifies**: After typing a search query and selecting a model, re-rendering with a new sessions array (simulating live update) leaves both filter inputs unchanged and the display count remains filtered.

### CT-24: Clear filter button — resets all filters

**File**: `web-app/src/app/insights/__tests__/SessionsTable.test.tsx`
**Test name**: `SessionsTable_should_clearAllFilters_When_clearButtonClicked`
**Verifies**: After applying search + model filter, clicking "Clear filters" resets `searchText` to `""`, `modelFilter` to `""`, and shows the full session list.

---

## 4. E2E Tests (Playwright)

Spec files in `tests/e2e/`. All tests run against the test server at `http://localhost:8544`.
Each spec file begins with `// @feature insights:filter, insights:detail, insights:projection, insights:performance`.

### E2E-01: Time range filter — preset selection

**File**: `tests/e2e/insights-time-range.spec.ts`
**Description**: Navigate to `/insights`; assert filter bar is visible; click "Last 7 days"; assert URL param `preset=7d` is set.
**Acceptance criteria verified**: R1 — filter bar visible, presets selectable, URL updated.

### E2E-02: Time range filter — custom date range

**File**: `tests/e2e/insights-time-range.spec.ts`
**Description**: Click "Custom" preset; assert two date inputs appear; fill from/to dates; assert URL params `from` and `to` are set with correct ISO strings.
**Acceptance criteria verified**: R1 — custom range date picker functional, URL params updated.

### E2E-03: Time range filter — data re-renders after preset change

**File**: `tests/e2e/insights-time-range.spec.ts`
**Description**: Load page; note summary card values for "All time"; switch to "Last 7 days"; assert summary card values change (different total or loading state appears then resolves).
**Acceptance criteria verified**: R1 — changing filter re-fetches and re-renders all charts and cards.

### E2E-04: Time range filter — persists through live update cycle

**File**: `tests/e2e/insights-time-range.spec.ts`
**Description**: Set filter to "Last 30 days"; wait for a live update event (WatchInsights stream tick visible via pulsing dot change); assert URL still has `preset=30d` and filter button still shows active state.
**Acceptance criteria verified**: R1 — filter persists through live-update cycles.

### E2E-05: Session detail — click row opens drawer

**File**: `tests/e2e/insights-session-detail.spec.ts`
**Description**: Navigate to `/insights`; wait for sessions table to populate; click first row; assert slide-over drawer appears with session ID, model, project path, and cost visible.
**Acceptance criteria verified**: R2 — row clickable, drawer opens, metadata displayed.

### E2E-06: Session detail — close restores filter state

**File**: `tests/e2e/insights-session-detail.spec.ts`
**Description**: Set "Last 7 days" filter; open a session drawer; press Escape; assert drawer closes, URL still has `preset=7d`, filter button still active.
**Acceptance criteria verified**: R2 — close returns user to insights with filters intact; Escape key works.

### E2E-07: Cost projection card — visible with sufficient data

**File**: `tests/e2e/insights-projections.spec.ts`
**Description**: Navigate to insights on a workspace with ≥7 days of current-month data; assert "Projected this month" card is present; assert displayed dollar value is a positive number.
**Acceptance criteria verified**: R3 — projection card shown when ≥7 days of data.

### E2E-08: Budget alert — warning state when threshold exceeded

**File**: `tests/e2e/insights-projections.spec.ts`
**Description**: Set budget threshold to $0.01 (guaranteed to be exceeded); assert projection card has amber/warning styling; assert warning banner present at top of page.
**Acceptance criteria verified**: R3 — warning styling and banner appear when projected > threshold.

### E2E-09: Skeleton loader — visible before data arrives

**File**: `tests/e2e/insights-performance.spec.ts`
**Description**: Navigate to `/insights` with network throttled (Playwright `slow3G` profile); assert skeleton card elements are visible in the DOM before first RPC response arrives (check within 200ms of navigation via `page.waitForSelector('[data-testid="skeleton-card"]', { timeout: 200 })`).
**Acceptance criteria verified**: R4 — skeleton shown immediately on mount; first meaningful paint < 200ms.

### E2E-10: Live update — scroll position preserved

**File**: `tests/e2e/insights-live-update.spec.ts`
**Description**: Load page with 50+ sessions; scroll to bottom of sessions table; wait for a live update event; assert scroll position unchanged (measure `scrollTop` via `page.evaluate()` before and after update).
**Acceptance criteria verified**: R6 — incoming update does not reset scroll position.

### E2E-11: Session table search and filter

**File**: `tests/e2e/insights-search-filter.spec.ts`
**Description**: Type a partial project path into the search input; assert visible rows reduce to matching sessions only. Select a model from the dropdown; assert further reduction to sessions with that model.
**Acceptance criteria verified**: R7 — text search and model filter work; rows filtered client-side.

### E2E-12: Clear filter — resets all session filters

**File**: `tests/e2e/insights-search-filter.spec.ts`
**Description**: Apply search + model filter; assert reduced row count; click "Clear filters"; assert row count returns to full unfiltered count.
**Acceptance criteria verified**: R7 — clear filter button resets all filters.

---

## 5. Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| EC-01 | Custom date range where `from > to` | Filter bar shows validation error; RPC not called with inverted range; displayed "from" date input highlighted with error state |
| EC-02 | `useProjectedCost` with exactly 7 days of data | Returns non-null projection (boundary is inclusive: ≥7) |
| EC-03 | `useProjectedCost` called on last day of month (day 31) | `daysInMonth` correctly computed; projection equals current spend |
| EC-04 | `SessionDetailDrawer` opened during active live update | Drawer content shows data from the moment it was opened; update does not replace selected session while drawer is open |
| EC-05 | `SessionDetailDrawer` with `skill_activations = []` | Skill activations section shows empty state message, not a broken list |
| EC-06 | `SessionDetailDrawer` with `top_tools = []` | Tools breakdown section shows empty state message |
| EC-07 | `SessionsTable` search with no matching results | Table body shows "No sessions match your filters" empty state; row count shows `0 of N` |
| EC-08 | Budget threshold set to `0` | Treated as "no threshold set" (or explicitly shows 0 warning immediately); avoids divide-by-zero |
| EC-09 | `TableVirtuoso` container has zero height (collapsed parent) | Log a warning; fall back to plain table rendering; no invisible rows |
| EC-10 | Live update arrives while `SessionDetailDrawer` is open | Sessions list in background updates; selected session in drawer not affected until user closes and reopens |
| EC-11 | SSR render of `ProjectedCostCard` before hydration | `isHydrated = false`; no threshold shown; no warning styling; prevents localStorage access during SSR |
| EC-12 | `InsightsDashboard` mounts with `preset` URL param but no `from`/`to` | Preset is resolved to a date range on the client; correct `from`/`to` derived dates passed to RPC |
| EC-13 | `WatchInsights` stream drops and reconnects during active use | Reconnect with exponential backoff; `isLiveUpdating` goes false during gap, true after reconnect; filter state unchanged |
| EC-14 | Surgical patch for a session whose `conversationId` is empty string | Falls back to `sessionId` as lookup key; does not treat all empty-`conversationId` sessions as the same entry |
| EC-15 | `InsightsDashboardSkeleton` rendered via `<Suspense>` before `useSearchParams` resolves | Skeleton uses only static layout classes (no `useSearchParams` calls); no crash or blank screen |

---

## 6. Performance Benchmarks

### PERF-01: First meaningful paint < 200ms

**How to verify**:
- Use Playwright with `page.tracing.start()` + `page.tracing.stop()` to capture a Chrome trace.
- Measure time from `page.goto('/insights')` navigation commit to the first frame where `[data-testid="skeleton-card"]` is present in the DOM.
- Assertion: `timeToSkeleton < 200ms`.
- Automated: add as a Playwright performance test in `tests/e2e/insights-performance.spec.ts` with `test.slow()` annotation.
- Lighthouse CI threshold: `first-contentful-paint < 2000ms` (the skeleton is content).

### PERF-02: SessionsTable 500-row scroll at 60fps

**How to verify**:
- Inject 500 synthetic `SessionTokenSummary` fixtures via mock transport in a Playwright test.
- Use `page.evaluate()` to call `requestAnimationFrame`-based FPS counter during programmatic scroll of the table container.
- Assertion: frame rate stays ≥ 55fps for a 2-second scroll (allowing brief dips at scroll start/end).
- Manual verification: use Chrome DevTools Performance panel on `http://localhost:8543/insights` with 500-session data; "Frames" lane should show no red bars.
- Acceptance: no single frame > 33ms (< 30fps) during sustained scroll.

### PERF-03: Sort/filter re-render < 100ms

**How to verify**:
- In a Jest test, use `performance.now()` bracketing around a `userEvent.type` search input event followed by `await screen.findByText(...)`.
- With 200 synthetic sessions, the full filter pipeline (Fuse.js search → model filter → Virtuoso update) must complete in < 100ms.
- Acceptance: `filterRenderTime < 100ms` in the Jest timing assertion.

### PERF-04: Time range filter → chart re-render < 500ms

**How to verify**:
- Playwright test: measure time from preset button click to all three chart components reporting their `onAnimationEnd` event (or `data-loaded="true"` attribute set by chart wrapper).
- Acceptance: `chartUpdateTime < 500ms` under normal network conditions.

### PERF-05: Per-session detail slide-over open < 150ms

**How to verify**:
- Playwright test: measure time from `row.click()` to `drawer.isVisible()` returning true (all data already in memory, no RPC needed).
- Acceptance: `drawerOpenTime < 150ms`.

---

## 7. Implementation Readiness Gate

### Gate 1: Requirements Coverage

Checking plan.md against requirements.md acceptance criteria:

| Requirement | Plan Coverage | Gap? |
|-------------|---------------|------|
| R1 — Filter bar with 5 preset options | Story 1.1.2b (TimeRangeFilter component with `PRESETS` array) | None |
| R1 — Custom date range picker | Story 1.1.2b (custom inputs when `preset === "custom"`) | None |
| R1 — Changing filter re-fetches everything | Story 1.1.1a/b + 1.1.2c (`fetchSummary` dep on `from`/`to`) | None |
| R1 — Filter persists through live-update cycles | Story 1.1.4 (URL params not reset by hook) | None |
| R1 — Active filter clearly indicated | Story 1.1.2b (`presetButtonActive` recipe variant) | None |
| R1 — Backend RPC wired | Story 1.1.1a/b (`from`/`to` in `GetInsightsSummaryRequest` and `WatchInsightsRequest`) | None |
| R2 — Slide-over detail view | Story 2.1.1 (SessionDetailDrawer) | None |
| R2 — Detail shows metadata, tools, skills | Story 2.1.1b (explicit fields listed) | **Partial**: `TurnTimeline` proxy acknowledged; label text should say "Message count" not "Turns" — noted in adversarial review minor |
| R2 — Close restores filter state | Story 2.1.3 (drawer state in InsightsDashboard, not URL) | None |
| R2 — Loading and empty states | Story 2.1.1b (null guard renders nothing) | **Partial**: Empty state for `top_tools` and `skill_activations` not explicitly specified in plan tasks; add to EC-05/EC-06 |
| R3 — Projected card ≥7 days | Story 3.1.1a (7-day guard) | None |
| R3 — Budget threshold localStorage | Story 3.1.2b (useBudgetThreshold) | None |
| R3 — Warning styling + banner | Story 3.1.2c/d | None |
| R4 — Skeleton on mount | Story 4.1.1a/b | None |
| R4 — Charts lazy-load independently | Story 4.1.1b (dynamic() loading prop) | None |
| R4 — First paint < 200ms | Story 4.1.1 (Suspense boundary) | None — no explicit task to measure/test perf target; covered by PERF-01 |
| R5 — Virtual scrolling for 500+ | Story 4.3.1 (TableVirtuoso) | None |
| R5 — Smooth scroll at 500 rows | Story 4.3.1b (Virtuoso `data` prop diffing) | None — no explicit perf test task; covered by PERF-02 |
| R5 — Sort/filter < 100ms | Story 1.1.3a (Fuse.js client-side) | None — no explicit perf test task; covered by PERF-03 |
| R6 — Only changed regions re-render | Story 4.2.1 (useMemo on charts) + Story 4.4.2 (surgical patch) | None |
| R6 — Scroll not reset on update | Story 4.3.1b (Virtuoso `data` prop, no scroll reset) | None |
| R6 — Live update indicator visible | Plan references `isLiveUpdating` state but does not have an explicit task to ensure the indicator remains visible during the update event (it currently sets `isLiveUpdating(false)` immediately on patch) | **Minor gap**: Task 4.4.2a sets `setIsLiveUpdating(false)` after patch; indicator should pulse `true` → `false` on each event cycle, not just while stream is active. Current wording is ambiguous. |
| R7 — Text search by project path | Story 1.1.3a (Fuse.js) | None |
| R7 — Model filter dropdown | Story 1.1.3a (`uniqueModels` derived state) | None |
| R7 — Filters client-side | Story 1.1.3a (no RPC triggered) | None |
| R7 — Filter state on live update | Story 1.1.4 (local state in SessionsTable not reset) | None |
| R7 — Clear filter button | Story 1.1.3a (clear button resets both) | None |

**Coverage: 7/7 requirements covered.** Three minor gaps identified (not blockers):
- R2: Empty state spec for tools/skills tables missing from plan tasks (covered by EC-05, EC-06, CT-06 in this validation).
- R4/R5: No perf test tasks in plan — covered by PERF-01 through PERF-05 in this validation.
- R6: Live update indicator wording in Task 4.4.2a is ambiguous; clarify in implementation.

---

### Gate 2: Test Coverage

Every acceptance criterion maps to at least one test case:

- R1: CT-01, CT-02, CT-03, CT-04, E2E-01, E2E-02, E2E-03, E2E-04, UT-01 — PASS
- R2: CT-05, CT-06, CT-07, CT-08, CT-09, E2E-05, E2E-06 — PASS
- R3: UT-03, UT-04, UT-05, CT-10, CT-11, CT-12, CT-13, CT-14, E2E-07, E2E-08 — PASS
- R4: CT-15, CT-16, E2E-09, PERF-01 — PASS
- R5: CT-17, PERF-02, PERF-03 — PASS
- R6: UT-06, CT-18, CT-19, CT-20, E2E-10 — PASS
- R7: UT-07, UT-08, UT-09, CT-21, CT-22, CT-23, CT-24, E2E-11, E2E-12 — PASS

**Coverage: Every acceptance criterion has ≥1 test.**

---

### Gate 3: Adversarial Review

**Verdict from adversarial-review.md: CONCERNS (not BLOCKED)**

The four concerns are resolved in the plan (some were fixed mid-plan, some are addressed in this validation):

| Concern | Resolution Status |
|---------|------------------|
| `vars.zIndex` vs `zIndex` constant import | Resolved in plan (Task 2.1.1a explicitly corrected to `import { zIndex }`) |
| Budget warning state lifted to `useBudgetThreshold` | Resolved in plan (Task 3.1.2b creates `useBudgetThreshold`; both dashboard and card use it) |
| Inline `style={{ display: 'grid' }}` in skeleton | Resolved in plan (Task 4.1.1a updated to import `grid2` from `InsightsDashboard.css.ts`) |
| Unstable Date references → infinite re-fetch | Resolved in plan (Decision 1 / Task 1.1.2c explicitly warns about `new Date()` inline and mandates `useMemo`) |

**Conclusion: All four concerns are addressed in the plan. Gate 3: PASS.**

---

### Gate 4: Architecture Soundness

Checking for contradictions between research findings and the plan:

| Check | Finding |
|-------|---------|
| `react-window` vs `react-virtuoso` | Plan correctly chooses `react-virtuoso` / `TableVirtuoso`; `react-window` explicitly NOT listed. Decision 3 cites `react-virtuoso ^4.18.7` already installed. No contradiction. |
| CSS architecture compliance | Plan uses `recipe()`, `vars` tokens, no inline layout styles (after adversarial correction). `createPortal` used for drawer. Compliant with ADR-009. |
| `createPortal` SSR guard | Plan explicitly documents the SSR guard (`typeof document !== 'undefined'` or `useEffect` mounted flag). Risk R2 addressed. |
| `useSearchParams` / Suspense | Plan Task 1.1.2d wraps `InsightsDashboard` in `<Suspense>`. Correct for Next.js App Router. |
| No new proto RPCs | Plan confirms no new RPC needed; all data exists in `SessionTokenSummary`. Compliant with technical constraint. |
| `zIndex` constant pattern | Corrected; plan references `import { zIndex }` (constant), not `vars.zIndex`. Consistent with codebase pattern. |
| Fuse.js availability | Plan says "Import Fuse from `fuse.js`". Confirmed: `"fuse.js": "^7.3.0"` is in `web-app/package.json`. No risk. |
| `Timestamp` import source | Plan cites `@bufbuild/protobuf`. This is the correct package for `Timestamp.fromDate()` in the bufbuild/Connect-ES stack. Consistent with existing proto usage. |
| `useBudgetThreshold` SSR hydration pattern | Plan references `useHistoryFilters` SSR guard as the model. This pattern is documented and battle-tested in the codebase. |

**Gate 4: PASS.** All architecture references confirmed against the codebase.

---

## 8. Readiness Gate Summary

| Gate | Result | Notes |
|------|--------|-------|
| 1 — Requirements coverage | PASS | 7/7 requirements covered; 3 minor gaps patched by this validation plan |
| 2 — Test coverage | PASS | Every acceptance criterion has ≥1 test case |
| 3 — Adversarial review | PASS | Verdict is CONCERNS (not BLOCKED); all 4 concerns resolved in plan |
| 4 — Architecture soundness | PASS | All references confirmed; `fuse.js ^7.3.0` present in `web-app/package.json` |

**Overall readiness verdict: PASS**

Implementation may begin.
