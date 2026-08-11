# Architecture Research — Insights Page Enhancement

## Current State

`/web-app/src/app/insights/` contains:
- `page.tsx` — Server Component wrapper; renders `<InsightsDashboard />`
- `InsightsDashboard.tsx` — Client Component ("use client"); single hook `useInsightsSummary({ includeOrphans: true })`; no URL params; no filter state
- Charts are lazy-loaded via `dynamic(..., { ssr: false })`
- No routing sub-pages exist under `/insights/`

## R1: Time Range Filter State Management

### Decision: URL search params via existing `useFilterState`

An existing `useFilterState` hook at `lib/hooks/useFilterState.ts` already handles URL search params for filter state. It uses `useSearchParams` + `router.replace` with `scroll: false`. This is the right choice because:
1. Time range is sharable/bookmarkable state
2. The existing hook is battle-tested and avoids re-inventing URL state management
3. `useHistoryFilters` shows the localStorage alternative pattern, but time range should be in URL (not localStorage) so it's linkable

**Recommended filter keys**: `from` (ISO date string), `to` (ISO date string), `preset` ("7d" | "30d" | "90d" | "custom")

**Integration point**: `InsightsDashboard.tsx` must be wrapped in `<Suspense>` because `useSearchParams()` requires it in Next.js App Router. Wrap in `page.tsx`:

```tsx
<Suspense fallback={<InsightsDashboardSkeleton />}>
  <InsightsDashboard />
</Suspense>
```

**Wiring to hook**: Parse URL params to `Date` objects, pass as `filters.from`/`filters.to` into `useInsightsSummary`. The hook already accepts these; they just need to be wired through to `GetInsightsSummaryRequest`.

## R2: Per-Session Detail View

### Decision: Slide-over panel (not new route)

**Why slide-over over `/insights/session/[sessionId]`**:
1. The data is already in memory (all sessions from `summary.sessions`)
2. No additional RPC is needed — `SessionTokenSummary` has all detail-view data
3. The existing `ApprovalDrawer` pattern provides a complete, proven slide-over implementation with `position:fixed`, `slideIn` keyframe, z-index management, and no backdrop overlay
4. App Router dynamic routes would require a loading state + separate data fetch

**Implementation plan**:
- Add `selectedSessionId: string | null` local state to `InsightsDashboard`
- `SessionsTable` receives `onRowClick: (session: SessionTokenSummary) => void`
- New `SessionDetailDrawer` component mirrors `ApprovalDrawer`'s CSS pattern (`position:fixed`, right-anchored, z-index between `dropdown:500` and `modal:1000`)
- Drawer content: session metadata header, model/path/dates, `message_count` as "Turns" count, `skill_activations` list, `top_tools` list with call counts
- Must use `createPortal(..., document.body)` per CSS architecture rules (avoid transform stacking context issues)

**z-index**: Add `slideOver: 600` to `zIndex` constants in `theme-contract.css.ts`.

## R3: Cost Projections

Client-side computation from `daily[]` array:
```
avgDailyCost = totalCostUsd / max(daysWithData, 1)
projectedMonthly = avgDailyCost * 30
```

A new `ProjectionCard` component alongside `SummaryCards`. Budget threshold stored in `localStorage` using the same pattern as `useHistoryFilters` (SSR guard: `if (typeof window === 'undefined') return defaultValue`).

## R4: Skeleton Loaders

Strategy: replace the `loadingBanner` spinner with proper skeletons using existing `Skeleton` component.

- `InsightsDashboardSkeleton` — used as `<Suspense>` fallback in `page.tsx`
- Compose from `Skeleton` (`components/ui/Skeleton.tsx`) — already has shimmer via vanilla-extract
- Layout: 4 summary card skeletons in `grid2`, then chart placeholder blocks (fixed heights matching chart containers), then table row skeletons
- Charts remain behind `dynamic(..., { ssr: false })` — show skeleton until loaded; use `loading` prop or a `null` state in the chart wrapper

## R5: Large Session Counts — Virtual Scrolling

### Decision: react-virtuoso Virtuoso component for SessionsTable

`react-virtuoso` is already installed (`^4.18.7`) and proven in `VirtualLogList.tsx`. For `SessionsTable`:

- Replace `<table>` with `Virtuoso` component; render rows as divs styled to look like table rows (or use `TableVirtuoso` from react-virtuoso for semantic table rendering)
- `react-virtuoso` provides `TableVirtuoso` component that renders `<table>/<tbody>/<tr>` structure natively — use this to preserve table semantics
- Threshold: apply virtualization when `sessions.length > 50` (below that, plain `<table>` is fine)

**Alternative: server-side pagination via ListSessionTokens**
- Use `ListSessionTokens` (page_size=50, cursor-based) for true server-side pagination
- Better for very large datasets (thousands of sessions)
- Drawback: requires a separate data fetch, breaks "all sessions already in memory" assumption
- Recommendation: virtualize client-side first (simpler, instant); add ListSessionTokens pagination as a future option

## R6: Smooth Live Updates

**Problem**: `WatchInsights` stream triggers `fetchSummary()` on each event, which calls `setSummary(res)` and replaces the entire `sessions` array, causing the virtual list to re-render and potentially jump scroll position.

**Solution**:
1. Use `react-virtuoso`'s `data` prop — Virtuoso only re-renders changed rows, not the full list
2. Add session data to a Map keyed by `conversationId` for O(1) lookup; `useCallback` merge function that patches individual sessions rather than replacing the array
3. `WatchInsights` `InsightsEvent` already includes `optional SessionTokenSummary session` — use this to do surgical updates without a full re-fetch when `event_type === "update"` (only re-fetch on `event_type === "parse_complete"`)

**Scroll stability pattern** (from `VirtualLogList`): `followOutput` prop is `false` for insights (unlike log tail); Virtuoso's `restoreStateFrom` or maintaining `initialTopMostItemIndex` preserves position across data changes.

## R7: Session Table Search/Filter

Client-side filtering using data already in memory:
- Text search: `fuse.js` (already in `package.json` `^7.3.0`) for fuzzy path/model search
- Model filter: dropdown using unique models from `sessions` array (same pattern as `useHistoryFilters.uniqueModels`)
- Filter state: local component state (not URL params — transient, not sharable)

**Filter bar placement**: inside `SessionsTable` component header area (alongside existing orphan toggle), keeping it co-located with the data.

## Routing Notes

No new Next.js routes are needed (slide-over approach avoids `/insights/session/[id]`). If a dedicated page is later desired, it can be added as `app/insights/session/[sessionId]/page.tsx` using `searchParams` to pass the ID without a separate data fetch (pass sessionId, look up from already-fetched data in parent context or via a URL-driven re-fetch of `GetInsightsSummary` with `session_id_filter`).

## Summary

- **Time range filter**: URL search params via existing `useFilterState` hook; `InsightsDashboard` wrapped in `<Suspense>` in `page.tsx`
- **Per-session detail**: Slide-over drawer (not new route) using `ApprovalDrawer` CSS pattern + `createPortal`; data already in memory from `summary.sessions`
- **Virtual scrolling**: `TableVirtuoso` from react-virtuoso (already installed); surgical session updates from `InsightsEvent.session` field to prevent scroll jumps
