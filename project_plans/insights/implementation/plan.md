# Implementation Plan: insights

**Feature**: Elevate the Insights page from a functional baseline to a polished, high-performance analytics tool with time-range filtering, per-session drill-down, cost projections, skeleton loading, virtual scrolling, live-update stability, and session search/filter.
**Date**: 2026-05-31
**Status**: Ready for implementation
**ADRs**: None (all technology choices use existing stack; see Key Decisions below)

---

## Executive Summary

The Insights page already has a solid data pipeline (ConnectRPC streaming, `GetInsightsSummary`, `WatchInsights`). This plan closes **seven gaps**:

1. `from`/`to` timestamp fields are in `InsightsFilters` and the proto but never sent to the backend — wire them through.
2. No time-range UI — add a `TimeRangeFilter` bar using URL search params.
3. `SessionsTable` has no search or filtering — add client-side text + model filter using Fuse.js.
4. No per-session drill-down — add a `SessionDetailDrawer` slide-over using data already in memory.
5. No cost projections — compute `projectedMonthly` client-side from daily buckets.
6. No skeleton loaders — replace the spinner with proper skeleton placeholders.
7. Re-render thrash and scroll jumps on live updates — virtualize with `TableVirtuoso`, memo charts, surgical session patches.

**No new proto RPCs, no new npm packages, no backend schema changes.**

---

## Key Architectural Decisions

### Decision 1: Time range filter — URL search params via `useFilterState`

**Choice**: URL search params (`from`, `to`, `preset` keys) via the existing `useFilterState` hook.

**Rationale**:
- Time range is inherently shareable/bookmarkable state; local component state loses it on refresh.
- `useFilterState` is already battle-tested in the codebase and handles `router.replace` + `useSearchParams` correctly.
- `useHistoryFilters` shows the localStorage alternative — appropriate for transient UI state, but time range should be URL-driven.
- `InsightsDashboard` must be wrapped in `<Suspense>` in `page.tsx` because `useSearchParams` requires it in App Router.

**Filter keys**: `preset` ("today" | "7d" | "30d" | "90d" | "all"), `from` (ISO date string), `to` (ISO date string).

### Decision 2: Per-session detail — slide-over drawer, not a new route

**Choice**: `SessionDetailDrawer` component using `createPortal` + Radix Dialog (same as `Modal`/`ModalContent`), not `/insights/session/[sessionId]`.

**Rationale**:
- All `SessionTokenSummary` data is already in memory from `summary.sessions` — no extra RPC.
- A new App Router dynamic route would require a loading state, separate fetch, and URL navigation that disrupts the filter state.
- The `ApprovalDrawer` CSS pattern provides a proven slide-over template.
- **CRITICAL constraint**: Per css-architecture rules, overlays with `position:fixed` must use `createPortal` to escape transform stacking contexts. New drawer uses `createPortal(..., document.body)`.
- **Data limitation**: `SessionTokenSummary` has NO `TurnTimeline` field. The R2 requirement mentions "turn timeline" but the proto doesn't support it. Detail view uses `message_count` as a "turns" count, `skill_activations` as the activations list, and `top_tools` for tools breakdown. This is explicitly acknowledged; full per-turn data would require a new RPC (out of scope).

### Decision 3: Virtual scrolling — `TableVirtuoso` from react-virtuoso

**Choice**: `TableVirtuoso` from `react-virtuoso` (already installed, `^4.18.7`).

**Rationale**:
- `react-window` is NOT installed; do not add it.
- `react-virtuoso` is already proven in `VirtualLogList.tsx`.
- `TableVirtuoso` preserves semantic `<table>/<tbody>/<tr>` HTML structure, critical for accessibility.
- `react-virtuoso`'s `data` prop diffing avoids full re-renders on live updates, directly addressing R6.
- **Critical pitfall (P2)**: `TableVirtuoso` requires a height-constrained container. The `virtualContainer` style must set `height: min(600px, 70vh); overflow-y: auto` or Virtuoso renders all rows.

### Decision 4: Live update stability — surgical session patches + sort stability

**Choice**: Merge `InsightsEvent.session` patches into a `Map<string, SessionTokenSummary>` keyed by `conversationId`, sort by `last_message_at` descending before passing to Virtuoso.

**Rationale**:
- Current code calls `setSummary(res)` on every event, replacing the entire array and causing Virtuoso to diff all rows.
- `InsightsEvent` already carries an optional `session` field — use it for `event_type === "update"` (surgical patch); only full-refetch on `event_type === "parse_complete"`.
- Stable descending sort ensures new sessions append at the bottom, not index 0, preserving Virtuoso's scroll anchoring.

---

## Dependency Visualization

```
Epic 1: Time Range Filter
  Task 1.1 (wire from/to to hook) ──────────────┐
  Task 1.2 (TimeRangeFilter component)           │
  Task 1.3 (InsightsDashboard wiring + Suspense)─┤─ blocks ──► all other epics (provides filtered data)
  Task 1.4 (SessionsTable client-side search)    │
  Task 1.5 (filter state through live updates)───┘

Epic 2: Per-Session Detail
  Task 2.1 (SessionDetailDrawer shell)
  Task 2.2 (drawer content from SessionTokenSummary) ─ depends on 2.1
  Task 2.3 (row click wiring in SessionsTable) ─────── depends on 2.1, 2.2
  Task 2.4 (keyboard accessibility) ─────────────────── depends on 2.1

Epic 3: Cost Projections
  Task 3.1 (useProjectedCost hook)
  Task 3.2 (ProjectedCostCard component) ─ depends on 3.1
  Task 3.3 (budget threshold localStorage) ─ depends on 3.2

Epic 4: Performance
  Task 4.1 (skeleton loaders) ─ independent
  Task 4.2 (chart useMemo) ─── independent
  Task 4.3 (TableVirtuoso) ─── independent (but coordinates with Task 2.3)
  Task 4.4 (WatchInsights reconnect backoff) ─ independent
  Task 4.5 (surgical live update) ─ depends on 4.4
```

---

## Phase 1: Core Data Plumbing

### Epic 1.1: Time Range Filter (R1, R7)

**Goal**: Wire time range from URL params through the hook to the backend RPC; add filter UI; add session table search/filter; preserve filter across live updates.

#### Story 1.1.1: Wire `from`/`to` to backend RPC
**As a** developer, **I want** the `from`/`to` filters to be sent to `GetInsightsSummaryRequest`, **so that** the backend returns only data in the selected time window.
**Acceptance Criteria**:
- `GetInsightsSummaryRequest` includes `from` and `to` Timestamps when set in filters
- `WatchInsightsRequest` also includes `from`/`to` so stream events are scoped
- `fetchSummary` `useCallback` dep array includes `filters.from` and `filters.to`
**Files**: `web-app/src/lib/hooks/useInsightsService.ts`

##### Task 1.1.1a: Add `from`/`to` to GetInsightsSummaryRequest (~3 min)
- In `fetchSummary` callback, add to request object:
  ```ts
  ...(filters.from && { from: Timestamp.fromDate(filters.from) }),
  ...(filters.to && { to: Timestamp.fromDate(filters.to) }),
  ```
- Add `Timestamp` import from `@bufbuild/protobuf`
- Update `useCallback` dep array: `[filters.includeOrphans, filters.modelFilter, filters.sessionIdFilter, filters.from, filters.to]`
- Files: `web-app/src/lib/hooks/useInsightsService.ts`

##### Task 1.1.1b: Add `from`/`to` to WatchInsightsRequest (~2 min)
- In `startWatch`, pass from/to to the watch request:
  ```ts
  const req = create(WatchInsightsRequestSchema, {
    ...(filters.from && { from: Timestamp.fromDate(filters.from) }),
    ...(filters.to && { to: Timestamp.fromDate(filters.to) }),
  });
  ```
- Thread `filters.from`/`filters.to` into `startWatch` dep array
- Files: `web-app/src/lib/hooks/useInsightsService.ts`

---

#### Story 1.1.2: TimeRangeFilter component with URL state
**As a** user, **I want** to select a time preset (Today / 7d / 30d / 90d / All) or custom date range, **so that** all charts and tables update to show data for that window.
**Acceptance Criteria**:
- Filter bar with 5 preset buttons visible at top of InsightsDashboard
- Custom range: two date inputs (from/to) shown when "Custom" selected
- Active preset highlighted with primary button styling
- URL params `preset`, `from`, `to` updated on selection; shareable/bookmarkable
- `InsightsDashboard` wrapped in `<Suspense fallback={<InsightsDashboardSkeleton />}>`
**Files**: `web-app/src/app/insights/TimeRangeFilter.tsx`, `web-app/src/app/insights/TimeRangeFilter.css.ts`, `web-app/src/app/insights/page.tsx`, `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 1.1.2a: Create `TimeRangeFilter.css.ts` (~3 min)
- Add `filterBar`, `presetGroup`, `presetButton`, `presetButtonActive`, `customRange`, `dateInput` styles
- Import tokens from `@/styles/theme-contract.css` via `vars`
- Use `recipe()` for `presetButton` with `active` variant
- Files: `web-app/src/app/insights/TimeRangeFilter.css.ts`

##### Task 1.1.2b: Create `TimeRangeFilter.tsx` component (~5 min)
- Props: `{ value: TimeRangeValue; onChange: (v: TimeRangeValue) => void }`
- `TimeRangeValue` type: `{ preset: "today" | "7d" | "30d" | "90d" | "all" | "custom"; from?: Date; to?: Date }`
- Preset buttons render from a `PRESETS` constant array
- Custom range shows two `<input type="date">` elements when `preset === "custom"`
- `onChange` fires immediately on preset click or date input blur
- Files: `web-app/src/app/insights/TimeRangeFilter.tsx`, `web-app/src/app/insights/TimeRangeFilter.css.ts`

##### Task 1.1.2c: Wire URL params in `InsightsDashboard` (~4 min)
- Use `useSearchParams()` and `useRouter()` at top of `InsightsDashboard`
- Parse `preset`, `from`, `to` from URL params (strings)
- **CRITICAL — stable Date references**: derive `Date` objects via `useMemo` keyed on the string values from URL params, NOT inline `new Date()` on every render. Example: `const fromDate = useMemo(() => fromParam ? new Date(fromParam) : undefined, [fromParam])`. Passing `new Date()` inline causes a new reference on every render, triggering infinite `fetchSummary`/`startWatch` re-creation cycles.
- Also memoize `client` and `transport` in `useInsightsSummary` using `useMemo`/`useRef` to prevent re-creation on re-renders (add a note in Task 1.1.1a to address this while in that file)
- Pass stable `{ from: fromDate, to: toDate, includeOrphans: true }` as `filters` to `useInsightsSummary`
- Render `<TimeRangeFilter value={...} onChange={(v) => router.replace(...)} />`
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`, `web-app/src/lib/hooks/useInsightsService.ts`

##### Task 1.1.2d: Wrap InsightsDashboard in Suspense in `page.tsx` (~2 min)
- Import `InsightsDashboardSkeleton` (created in Epic 4.1)
- Wrap `<InsightsDashboard />` in `<Suspense fallback={<InsightsDashboardSkeleton />}>`
- Files: `web-app/src/app/insights/page.tsx`

---

#### Story 1.1.3: Session table search and model filter (R7)
**As a** user, **I want** to search sessions by project path and filter by model, **so that** I can quickly find sessions of interest.
**Acceptance Criteria**:
- Text input above table filters rows by project path (case-insensitive substring, powered by Fuse.js)
- Model dropdown (using `<select>` or `MultiSelect` component) filters to sessions using the selected model
- Both filters work client-side on in-memory data
- "Clear filters" button resets both
- Filter state is local component state (not URL params — transient)
- Filters survive live update cycles (state not reset when new sessions arrive)
**Files**: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 1.1.3a: Add search/filter state and logic to `SessionsTable` (~5 min)
- Add `searchText: string`, `modelFilter: string` local state with `useState`
- Import `Fuse` from `fuse.js`; create `useMemo(() => new Fuse(sessions, { keys: ['projectPath'], threshold: 0.4 }), [sessions])`
- Filtering pipeline: `sessions → fuse.search(searchText) → modelFilter → showOrphans`
- Derive `uniqueModels` from `sessions` for dropdown options
- Add clear button that resets both filters
- Files: `web-app/src/app/insights/SessionsTable.tsx`

##### Task 1.1.3b: Add filter bar styles to `SessionsTable.css.ts` (~3 min)
- Add `filterBar`, `searchInput`, `modelSelect`, `clearButton` styles using `vars` tokens
- Files: `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 1.1.3c: Render filter bar in `SessionsTable` JSX (~3 min)
- Render `<div className={filterBar}>` above the table with search input, model select, clear button
- Display `Sessions ({displayed.length} of {sessions.length})` in header when filters active
- Files: `web-app/src/app/insights/SessionsTable.tsx`

---

#### Story 1.1.4: Preserve filter state through live updates
**As a** user, **I want** my active time range and search filters to persist when live updates arrive, **so that** I don't lose my place.
**Acceptance Criteria**:
- Time range URL params unchanged by live update cycles
- Local `searchText` and `modelFilter` state in `SessionsTable` not reset when `sessions` prop changes
- Virtuoso scroll position maintained across live updates (handled by Task 4.5)
**Files**: `web-app/src/lib/hooks/useInsightsService.ts`, `web-app/src/app/insights/SessionsTable.tsx`

##### Task 1.1.4a: Verify filter stability — no state resets on re-render (~2 min)
- Confirm `SessionsTable` local filter state is in the component (not derived from props) — already correct by architecture
- Confirm `InsightsDashboard` does not reset URL params on live update (URL is set by user interaction only, not by hook state changes) — verify no `router.replace` calls in update paths
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`, `web-app/src/lib/hooks/useInsightsService.ts`

---

## Phase 2: Per-Session Detail

### Epic 2.1: Session Detail Drawer (R2)

**Goal**: Clicking a session row opens a slide-over panel showing all available session metadata, tools breakdown, and skill activations from `SessionTokenSummary`.

**Important constraint**: `SessionTokenSummary` has NO `TurnTimeline` field. The detail view shows `message_count` as a "turns" count proxy, `skill_activations` as strings, and `top_tools` with call counts. Per-turn data is not available without a new RPC (out of scope).

#### Story 2.1.1: `SessionDetailDrawer` component
**As a** user, **I want** to click a session row and see a slide-over panel with full session details, **so that** I can inspect usage without leaving the page.
**Acceptance Criteria**:
- Slide-over panel slides in from the right, uses `createPortal`
- Shows: session ID, conversation ID, model, project path, cost, message count, date range, cache hit rate
- Tools breakdown: table of `top_tools` (tool name, call count, MCP server)
- Skill activations: list of `skill_activations` strings
- Close button (X) and Escape key close the drawer
- Focus trap within drawer while open (Radix Dialog handles this)
- After close, scroll position in SessionsTable is preserved
**Files**: `web-app/src/app/insights/SessionDetailDrawer.tsx`, `web-app/src/app/insights/SessionDetailDrawer.css.ts`

##### Task 2.1.1a: Create `SessionDetailDrawer.css.ts` (~4 min)
- Styles: `overlay`, `drawer`, `drawerHeader`, `drawerTitle`, `closeButton`, `section`, `metaGrid`, `metaLabel`, `metaValue`, `toolsTable`, `skillList`, `skillBadge`
- `drawer`: `position: fixed; top: 0; right: 0; height: 100vh; width: min(480px, 90vw); overflow-y: auto`
- `overlay`: `position: fixed; inset: 0; background: rgba(0,0,0,0.4)`
- Add `slideIn` keyframe: `@keyframes slideIn { from { transform: translateX(100%) } to { transform: translateX(0) } }`
- **CRITICAL — `zIndex` is a plain constant, NOT in `vars`**: import as `import { zIndex } from '@/styles/theme-contract.css'` and reference `zIndex.slideOver` for the drawer, `zIndex.slideOver - 1` for the overlay. Do NOT write `vars.zIndex.*` (does not exist).
- First add `slideOver: 700` to the `zIndex` constant map in `theme-contract.css.ts` (between `dropdown: 500` and `modal: 1000`). The drawer at 700 sits above dropdowns but below modals/dialogs — correct for a page-level slide-over.
- Files: `web-app/src/app/insights/SessionDetailDrawer.css.ts`, `web-app/src/styles/theme-contract.css.ts`

##### Task 2.1.1b: Create `SessionDetailDrawer.tsx` component (~5 min)
- Props: `{ session: SessionTokenSummary | null; onClose: () => void }`
- Render nothing when `session === null`
- Use `createPortal(<>overlay + drawer</>, document.body)`
- Drawer header: session ID chip + close button
- Metadata section: grid of label/value pairs (model, path, cost, turns proxy, date range, cache hit rate)
- Tools section: `top_tools` table (tool name, count, MCP server)
- Skill activations section: `skill_activations` list as badges (empty state if none)
- Escape key handler via `useEffect` → `document.addEventListener('keydown', ...)`
- Files: `web-app/src/app/insights/SessionDetailDrawer.tsx`

##### Task 2.1.2: Wire row click in `SessionsTable` (~3 min)
- Add `onSessionClick?: (session: SessionTokenSummary) => void` to `SessionsTable` props
- Render `<tr ... onClick={() => onSessionClick?.(s)} style={{ cursor: 'pointer' }} role="button" tabIndex={0}>`
- Add `onKeyDown` for Enter/Space key activation (accessibility)
- Files: `web-app/src/app/insights/SessionsTable.tsx`

##### Task 2.1.3: Wire drawer in `InsightsDashboard` (~3 min)
- Add `selectedSession: SessionTokenSummary | null` state to `InsightsDashboard`
- Pass `onSessionClick={(s) => setSelectedSession(s)}` to `<SessionsTable>`
- Render `<SessionDetailDrawer session={selectedSession} onClose={() => setSelectedSession(null)} />`
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`

---

## Phase 3: Cost Projections

### Epic 3.1: Projected Monthly Cost (R3)

**Goal**: Show a "Projected this month" card computed client-side from daily buckets, with optional localStorage-backed budget warning.

#### Story 3.1.1: `useProjectedCost` hook
**As a** user, **I want** to see how much I'm projected to spend this month, **so that** I can plan my Claude API budget.
**Acceptance Criteria**:
- Returns `null` when fewer than 7 days of data in the current calendar month (too noisy)
- Formula: `(total cost in current month so far) / (days with data in current month) × (days in current month)`
- Returns `{ projectedMonthly: number; daysData: number; daysInMonth: number }`
- Pure computation from `daily[]` array, no external state
**Files**: `web-app/src/lib/hooks/useProjectedCost.ts`

##### Task 3.1.1a: Create `useProjectedCost.ts` (~5 min)
- Input: `daily: DailyTokenBucket[]`
- Filter `daily` to current calendar month (compare `bucket.date` month/year to `new Date()`)
- Count distinct days with data, sum cost
- Guard: return `null` if days < 7
- Compute projection: `avgDailyCost * daysInMonth()`
- Export type `ProjectedCostResult`
- Files: `web-app/src/lib/hooks/useProjectedCost.ts`

#### Story 3.1.2: Projected cost card + budget threshold (R3)
**As a** user, **I want** a "Projected this month" card with a configurable budget alert, **so that** I know if I'm on track to exceed my budget.
**Acceptance Criteria**:
- "Projected this month" card shown in `SummaryCards` area when `useProjectedCost` returns non-null
- Budget threshold: text input (stored in localStorage) with SSR hydration guard
- Warning state: when `projectedMonthly > threshold`, card uses amber/warning styling + banner at top of page
- Threshold input rendered in the card itself (not a settings page)
- localStorage key: `insights_budget_threshold_usd`
**Files**: `web-app/src/app/insights/ProjectedCostCard.tsx`, `web-app/src/app/insights/ProjectedCostCard.css.ts`, `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 3.1.2a: Create `ProjectedCostCard.css.ts` (~3 min)
- Styles: `card`, `cardWarning` (recipe variant with amber border + bg), `label`, `value`, `budgetInput`, `warningText`
- Import `vars` from `@/styles/theme-contract.css`
- Use `recipe()` with `{ warning: boolean }` variant for `card`
- Files: `web-app/src/app/insights/ProjectedCostCard.css.ts`

##### Task 3.1.2b: Create `useBudgetThreshold` hook (~3 min)
- New file: `web-app/src/lib/hooks/useBudgetThreshold.ts`
- Returns `{ threshold: number | null; setThreshold: (v: number | null) => void; isHydrated: boolean }`
- Uses `useHistoryFilters` SSR guard pattern: `isHydrated` flag + `useEffect` load from `localStorage`
- localStorage key: `insights_budget_threshold_usd`
- Both `InsightsDashboard` and `ProjectedCostCard` import this hook — avoids duplicating threshold state
- Files: `web-app/src/lib/hooks/useBudgetThreshold.ts`

##### Task 3.1.2c: Create `ProjectedCostCard.tsx` (~4 min)
- Props: `{ projection: ProjectedCostResult; threshold: number | null; isHydrated: boolean; onThresholdChange: (v: number | null) => void }`
- Warning logic: `isWarning = isHydrated && threshold !== null && projection.projectedMonthly > threshold`
- Card uses `card({ warning: isWarning })` recipe class
- Budget input: `<input type="number" ...>` calls `onThresholdChange` on change
- Show subtext: `Based on ${daysData} days of data`
- Files: `web-app/src/app/insights/ProjectedCostCard.tsx`

##### Task 3.1.2d: Wire into `InsightsDashboard` (~2 min)
- Import `useProjectedCost`, `ProjectedCostCard`, `useBudgetThreshold`
- Call `const projection = useProjectedCost(summary?.daily ?? [])`
- Call `const { threshold, setThreshold, isHydrated } = useBudgetThreshold()`
- Derive `isOverBudget = isHydrated && threshold !== null && projection !== null && projection.projectedMonthly > threshold`
- Render warning banner at top of page when `isOverBudget`
- Render `{projection && <ProjectedCostCard projection={projection} threshold={threshold} isHydrated={isHydrated} onThresholdChange={setThreshold} />}` after `<SummaryCards>`
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`

---

## Phase 4: Performance

### Epic 4.1: Skeleton Loaders (R4)

**Goal**: Replace spinner with proper skeleton placeholders; show immediately on mount before first RPC response.

#### Story 4.1.1: InsightsDashboardSkeleton and chart loading states
**As a** user, **I want** to see card-shaped placeholders immediately when navigating to the Insights page, **so that** the page feels instantly responsive.
**Acceptance Criteria**:
- `InsightsDashboardSkeleton` renders 4 summary card skeletons + 2 chart placeholder blocks + table row skeletons
- Used as `<Suspense>` fallback in `page.tsx`
- Chart `dynamic()` calls updated with `loading: () => <Skeleton width="100%" height={200} />`
- `isLoading: true` banner remains but skeleton shows instead of the old spinner for initial load
**Files**: `web-app/src/app/insights/InsightsDashboardSkeleton.tsx`, `web-app/src/app/insights/InsightsDashboard.tsx`

##### Task 4.1.1a: Create `InsightsDashboardSkeleton.tsx` (~4 min)
- Import `Skeleton` from `@/components/ui/Skeleton`
- Import `grid2` and `section` from `./InsightsDashboard.css` to reuse the existing grid layout (do NOT use inline `style={{ display: 'grid' }}` — css-architecture rules prohibit inline layout styles)
- Compose: 4x card skeletons in a `<div className={grid2}>`, 2x chart placeholder rectangles (`<Skeleton variant="rectangular" width="100%" height={200} />`) in a `<div className={grid2}>`, 5x table row skeletons
- Export `InsightsDashboardSkeleton`
- Files: `web-app/src/app/insights/InsightsDashboardSkeleton.tsx`

##### Task 4.1.1b: Add loading prop to dynamic() chart imports (~3 min)
- In `InsightsDashboard.tsx`, update all 3 `dynamic()` calls to add `loading: () => <Skeleton variant="rectangular" width="100%" height={200} />`
- Replace `loading && !summary` spinner with `<InsightsDashboardSkeleton />` for the initial empty state
- Files: `web-app/src/app/insights/InsightsDashboard.tsx`

---

### Epic 4.2: Chart Memoization (R4, R6)

**Goal**: Prevent chart re-computation on every live update event.

#### Story 4.2.1: Wrap chart data transforms in useMemo
**As a** developer, **I want** chart data transforms memoized by their input array reference, **so that** live update events don't cause unnecessary chart re-renders.
**Acceptance Criteria**:
- All `toDataPoints`/`toChartData` calls in chart components wrapped in `useMemo`
- Charts only re-render when their specific data prop changes, not on every dashboard re-render
**Files**: `web-app/src/app/insights/DailySpendChart.tsx`, `web-app/src/app/insights/ModelBreakdownChart.tsx`, `web-app/src/app/insights/ModelOverTimeChart.tsx`

##### Task 4.2.1a: Add useMemo to DailySpendChart (~2 min)
- Wrap inline data transform in `useMemo(() => toDataPoints(daily), [daily])`
- Import `useMemo` from `react`
- Files: `web-app/src/app/insights/DailySpendChart.tsx`

##### Task 4.2.1b: Add useMemo to ModelBreakdownChart (~2 min)
- Wrap `toChartData(models)` in `useMemo(() => toChartData(models), [models])`
- Files: `web-app/src/app/insights/ModelBreakdownChart.tsx`

##### Task 4.2.1c: Add useMemo to ModelOverTimeChart (~2 min)
- Wrap the series data computation in `useMemo(() => buildSeries(daily, mode), [daily, mode])`
- Files: `web-app/src/app/insights/ModelOverTimeChart.tsx`

---

### Epic 4.3: Virtual Scrolling for SessionsTable (R5, R6)

**Goal**: Replace plain `<table>` with `TableVirtuoso` for performant rendering of 500+ sessions.

#### Story 4.3.1: Replace SessionsTable with TableVirtuoso
**As a** user, **I want** the sessions table to scroll smoothly with hundreds of sessions, **so that** the page remains responsive regardless of session count.
**Acceptance Criteria**:
- `TableVirtuoso` used when `sessions.length > 50`; plain table used below threshold
- Container has `height: min(600px, 70vh); overflow-y: auto` to enable virtualization
- Scroll position preserved when live updates arrive (Virtuoso handles this via `data` prop diffing)
- Table header (`<thead>`) rendered via `fixedHeaderContent` prop (stays visible on scroll)
- Sort order: `last_message_at` descending (new sessions append at bottom, preserving scroll anchor)
**Files**: `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 4.3.1a: Add `virtualContainer` style to `SessionsTable.css.ts` (~2 min)
- Add `virtualContainer` style: `{ height: 'min(600px, 70vh)', overflowY: 'auto' }`
- Files: `web-app/src/app/insights/SessionsTable.css.ts`

##### Task 4.3.1b: Refactor `SessionsTable` to use `TableVirtuoso` (~5 min)
- Import `TableVirtuoso` from `react-virtuoso`
- Extract row render logic into `itemContent` callback: `(index, session) => <tr>...</tr>`
- Set `fixedHeaderContent={() => <tr><th>...</th>...</tr>}`
- Wrap in `<div className={virtualContainer}>`
- Keep plain `<table>` path for `displayed.length <= 50` (no virtualization overhead)
- Sort `displayed` by `s.lastMessageAt` descending before passing to `data` prop
- Files: `web-app/src/app/insights/SessionsTable.tsx`

---

### Epic 4.4: WatchInsights Reconnect + Surgical Updates (R6)

**Goal**: Add exponential backoff reconnect to the watch stream and apply surgical per-session patches on `event_type === "update"` events.

#### Story 4.4.1: Exponential backoff reconnect
**As a** developer, **I want** the WatchInsights stream to automatically reconnect after a dropped connection, **so that** the live update indicator doesn't go dark silently.
**Acceptance Criteria**:
- Stream reconnects automatically after disconnect with exponential backoff (1s → 2s → 4s → ... → 30s max)
- Reconnect stops when component unmounts (abort signal checked)
- `isLiveUpdating` remains accurate (false while disconnected, true while connected)
**Files**: `web-app/src/lib/hooks/useInsightsService.ts`

##### Task 4.4.1a: Add backoff reconnect loop to `startWatch` (~5 min)
- Wrap stream loop in `while (!abort.signal.aborted)` with try/catch
- On successful connection, reset `retryDelay = 1000`
- On error (not abort), `await new Promise(r => setTimeout(r, retryDelay))` then `retryDelay = Math.min(retryDelay * 2, 30_000)`
- Check `abort.signal.aborted` before each retry
- Files: `web-app/src/lib/hooks/useInsightsService.ts`

#### Story 4.4.2: Surgical session state patches
**As a** developer, **I want** `event_type === "update"` events to patch only the changed session, **so that** live updates don't cause full SessionsTable re-renders.
**Acceptance Criteria**:
- `InsightsEvent.session` field used to surgically update a single session entry in `summary.sessions`
- Full `fetchSummary` only called on `event_type === "parse_complete"` or when `InsightsEvent.session` is absent
- No new React state shape required — use functional `setSummary` updater for immutable patch
**Files**: `web-app/src/lib/hooks/useInsightsService.ts`

##### Task 4.4.2a: Implement surgical update logic (~5 min)
- In stream event handler:
  ```ts
  if (event.eventType === "update" && event.session) {
    setSummary(prev => {
      if (!prev) return prev;
      const key = event.session!.conversationId || event.session!.sessionId;
      const idx = prev.sessions.findIndex(s => (s.conversationId || s.sessionId) === key);
      if (idx === -1) return { ...prev, sessions: [...prev.sessions, event.session!] };
      const sessions = [...prev.sessions];
      sessions[idx] = event.session!;
      return { ...prev, sessions };
    });
    setIsLiveUpdating(false);
  } else if (event.eventType === "parse_complete") {
    await fetchSummary();
    setIsLiveUpdating(false);
  }
  ```
- Files: `web-app/src/lib/hooks/useInsightsService.ts`

---

## File Change List

| Task | Files Created | Files Modified |
|---|---|---|
| 1.1.1a/b | — | `web-app/src/lib/hooks/useInsightsService.ts` |
| 1.1.2a | `web-app/src/app/insights/TimeRangeFilter.css.ts` | — |
| 1.1.2b | `web-app/src/app/insights/TimeRangeFilter.tsx` | — |
| 1.1.2c | — | `web-app/src/app/insights/InsightsDashboard.tsx` |
| 1.1.2d | — | `web-app/src/app/insights/page.tsx` |
| 1.1.3a/b/c | — | `web-app/src/app/insights/SessionsTable.tsx`, `web-app/src/app/insights/SessionsTable.css.ts` |
| 1.1.4a | — | `web-app/src/app/insights/InsightsDashboard.tsx`, `web-app/src/lib/hooks/useInsightsService.ts` |
| 2.1.1a | `web-app/src/app/insights/SessionDetailDrawer.css.ts` | `web-app/src/styles/theme-contract.css.ts` |
| 2.1.1b | `web-app/src/app/insights/SessionDetailDrawer.tsx` | — |
| 2.1.2 | — | `web-app/src/app/insights/SessionsTable.tsx` |
| 2.1.3 | — | `web-app/src/app/insights/InsightsDashboard.tsx` |
| 3.1.1a | `web-app/src/lib/hooks/useProjectedCost.ts` | — |
| 3.1.2a | `web-app/src/app/insights/ProjectedCostCard.css.ts` | — |
| 3.1.2b | `web-app/src/lib/hooks/useBudgetThreshold.ts` | — |
| 3.1.2c | `web-app/src/app/insights/ProjectedCostCard.tsx` | — |
| 3.1.2d | — | `web-app/src/app/insights/InsightsDashboard.tsx` |
| 4.1.1a | `web-app/src/app/insights/InsightsDashboardSkeleton.tsx` | — |
| 4.1.1b | — | `web-app/src/app/insights/InsightsDashboard.tsx` |
| 4.2.1a/b/c | — | `web-app/src/app/insights/DailySpendChart.tsx`, `ModelBreakdownChart.tsx`, `ModelOverTimeChart.tsx` |
| 4.3.1a | — | `web-app/src/app/insights/SessionsTable.css.ts` |
| 4.3.1b | — | `web-app/src/app/insights/SessionsTable.tsx` |
| 4.4.1a | — | `web-app/src/lib/hooks/useInsightsService.ts` |
| 4.4.2a | — | `web-app/src/lib/hooks/useInsightsService.ts` |

**New files**: 10 (added `useBudgetThreshold.ts`)
**Modified files**: 9 (some modified by multiple tasks)

---

## Risk Register

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | `TableVirtuoso` needs height-constrained container — if `min(600px, 70vh)` is insufficient on small screens, rows won't virtualize | Medium | Set explicit `height` in `virtualContainer`; add a `data-testid` to verify container height in tests |
| R2 | `createPortal` may cause SSR hydration mismatch if drawer renders on server — `session` prop is `null` initially so portal renders nothing, but `document.body` ref not available in SSR | Low | Guard with `typeof document !== 'undefined'` before calling `createPortal`; or use `useEffect` to set `mounted = true` before rendering portal |
| R3 | Surgical session patch uses `conversationId || sessionId` as key — orphan sessions may have empty `sessionId`; key collision if two orphan sessions share a `conversationId` | Low | Use `conversationId` as primary key (it's the JSONL UUID); `sessionId` fallback only for non-orphans |
| R4 | `useProjectedCost` projection formula assumes uniform daily spend — burst days (e.g., heavy weekend use) inflate projection. Only 7-day guard reduces noise but doesn't eliminate it | Low | Display "based on N days" subtext to set expectations; no algorithmic fix needed |
| R5 | `useSearchParams()` requires `<Suspense>` boundary — if developer removes or moves the boundary, Next.js will throw a runtime error in production | Medium | Add a comment in `page.tsx` explaining the Suspense requirement; add to the feature-testing-registry |

---

## Acceptance Criteria Cross-Reference

| Requirement | Epic/Story | Covered |
|---|---|---|
| R1 — Time Range Filter UI (preset + custom) | 1.1.2 | ✅ |
| R1 — Wire from/to to RPC | 1.1.1 | ✅ |
| R1 — Filter persists through live updates | 1.1.4 | ✅ |
| R2 — Per-session detail slide-over | 2.1 | ✅ |
| R2 — Detail shows metadata, tools, skills | 2.1.1b | ✅ |
| R2 — No TurnTimeline (uses proxies) | Decision 2 / 2.1.1b | ✅ acknowledged |
| R3 — Projected monthly card (≥7 days) | 3.1.1, 3.1.2 | ✅ |
| R3 — Budget threshold with localStorage | 3.1.2b | ✅ |
| R4 — Skeleton loaders on mount | 4.1 | ✅ |
| R4 — Chart lazy-load with skeleton | 4.1.1b | ✅ |
| R5 — Virtual scrolling for 500+ sessions | 4.3 | ✅ |
| R6 — Scroll stability on live update | 4.3.1b + 4.4.2a | ✅ |
| R6 — Smooth chart updates | 4.2 | ✅ |
| R7 — Text search + model filter | 1.1.3 | ✅ |
