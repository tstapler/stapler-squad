# Pitfalls Research — Insights Page Enhancement

## P1: recharts AreaChart Performance with Many Data Points

### Risk Profile: Medium

`ModelOverTimeChart.tsx` uses `AreaChart` with `stackId="1"` — stacked areas. The chart maps over `models` (up to ~6 model families) × `daily` (up to 90 buckets) = ~540 data points total.

**Findings**:
- recharts v3 renders SVG paths; each `<Area>` generates one `<path>` per series. With 6 models × 90 points this is manageable (D3 curve generation is ~O(n) per series)
- The existing `defs` block creates `<linearGradient>` elements per model — 6 gradients is well within browser limits
- The `ResponsiveContainer` adds a `ResizeObserver` per chart instance; 3 chart instances on page is fine
- **Real risk**: Each `WatchInsights` update triggers a full `setSummary(res)` causing all 3 charts to re-render with new arrays. With memoization (`useMemo` on `toDataPoints`), this is mitigated. Without it, re-renders occur on every live update event.

**Mitigation**: Wrap `toDataPoints` calls in `useMemo` keyed on the `daily` array reference. The current code computes them inline on every render — add `useMemo`.

**Hardcoded hex colors**: `ModelBreakdownChart.tsx` uses hardcoded `PALETTE` hex values (`#6366f1`, etc.) and `ModelOverTimeChart.tsx` uses `MODEL_COLORS`. These bypass the vanilla-extract theme contract. For dark/light theme correctness, these should eventually be replaced with `vars.color.*` tokens, but this is a pre-existing issue, not new.

## P2: react-virtuoso + vanilla-extract CSS Compatibility

### Risk Profile: Low

`react-virtuoso` and vanilla-extract are compatible:
- vanilla-extract generates static class names at build time; react-virtuoso's row containers accept `className` props
- `VirtualLogList.tsx` already combines both (`styles.scrollContainer` from vanilla-extract + `Virtuoso` component) with no issues
- `TableVirtuoso` requires a fixed or known container height. The current `SessionsTable` uses `overflowX: "auto"` without a fixed height. **This is the key issue**: `Virtuoso`/`TableVirtuoso` needs a height-constrained container.

**Pitfall**: If `SessionsTable`'s container has `height: auto`, Virtuoso will render all rows (no virtualization). Must set an explicit `height` or `maxHeight` on the scroll container (e.g., `maxHeight: "600px"`, `overflowY: "auto"`).

**Fix**: In `SessionsTable.css.ts`, add a `virtualContainer` style with `height: min(600px, 70vh); overflowY: auto` and wrap `TableVirtuoso` in it.

## P3: Next.js App Router + dynamic() for recharts

### Risk Profile: Low (already solved)

The codebase already uses the correct pattern:

```tsx
const DailySpendChart = dynamic(
  () => import("./DailySpendChart").then((m) => m.DailySpendChart),
  { ssr: false }
);
```

The `ssr: false` flag prevents the recharts SVG (which uses browser APIs like `ResizeObserver`, `window`) from running during SSR. This avoids hydration mismatches.

**Potential pitfall for new charts**: If a developer adds a new chart component and forgets `ssr: false`, Next.js will attempt SSR of recharts, producing a `window is not defined` error or hydration mismatch. **All new chart components must follow this pattern**.

**`loading` prop**: `dynamic()` accepts a `loading` component prop. Currently no loading state is specified — the chart area is blank until JS loads. To show a skeleton:

```tsx
const DailySpendChart = dynamic(
  () => import("./DailySpendChart").then((m) => m.DailySpendChart),
  {
    ssr: false,
    loading: () => <Skeleton width="100%" height={200} />,
  }
);
```

This is the correct pattern for R4 (faster initial page load).

## P4: WatchInsights Stream Reconnection on Filter Change

### Risk Profile: Medium

**Current behavior**: `startWatch` is inside a `useCallback` that only depends on `fetchSummary`. When filters change, `fetchSummary` is recreated (new `useCallback`), `startWatch` is recreated, `useEffect` fires, and the old stream is aborted and a new one starts.

**Gap 1**: `WatchInsightsRequest` is created with `{}` (no from/to). This means the stream receives ALL events regardless of the time filter. On filter change, the re-fetch gets filtered data but the stream watches unfiltered events — an "update" event for a session outside the time window will trigger an unnecessary `fetchSummary()` with the correct filter. This is logically correct but wasteful.

**Gap 2**: If the stream drops (network hiccup, server restart), the `catch` block silently swallows the error and the stream is never restarted. The `isLiveUpdating` indicator stays false. The user sees stale data with no indication.

**Mitigation for Gap 2**: Add exponential backoff reconnect logic:

```typescript
let retryDelay = 1000;
(async () => {
  while (!abort.signal.aborted) {
    try {
      const stream = client.watchInsights(req, { signal: abort.signal });
      retryDelay = 1000; // reset on success
      for await (const event of stream) { ... }
    } catch (err) {
      if (abort.signal.aborted) break;
      await new Promise(r => setTimeout(r, retryDelay));
      retryDelay = Math.min(retryDelay * 2, 30_000);
    }
  }
})();
```

**Gap 3**: During the window between `abortWatchRef.current.abort()` and the new stream starting, an update could be missed. This is acceptable — the `fetchSummary()` call at the start of filter changes will catch it.

## P5: localStorage Budget Threshold — SSR Issues

### Risk Profile: Low (well-understood pattern)

Next.js App Router renders components on the server first. `localStorage` is a browser-only API. Accessing it during SSR throws `ReferenceError: localStorage is not defined`.

**Existing pattern in the codebase** (`useHistoryFilters.ts`):

```typescript
const loadFromStorage = <T,>(key: string, defaultValue: T): T => {
  if (typeof window === 'undefined') return defaultValue;
  try {
    const item = window.localStorage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  } catch { return defaultValue; }
};
```

And the hydration guard pattern:
```typescript
const [isHydrated, setIsHydrated] = useState(false);
useEffect(() => {
  // Load from localStorage here
  setIsHydrated(true);
}, []);
```

**For budget threshold**: Render the budget card with the default value (`null` / "not set") on first render, then hydrate with the actual localStorage value after mount. Show nothing (or a neutral state) until `isHydrated`. This avoids the hydration mismatch that occurs when SSR output differs from client output.

**Do NOT**: Access `localStorage` directly in component body or during initial render. This will cause a React hydration error in Next.js App Router.

## P6: SessionsTable scroll jump on live update

### Risk Profile: Medium (related to P4)

When `setSummary(res)` replaces the entire sessions array, the `SessionsTable` receiving a new `sessions` prop will re-render. With a plain `<table>`, React reconciles DOM nodes efficiently if keys are stable (`key={s.conversationId || s.sessionId}`). With virtualization, scroll position is preserved by `react-virtuoso` automatically when the `data` prop changes (it diffs by index).

**Main risk**: If session order changes between updates (e.g., new session appears at index 0), virtuoso may jump. **Fix**: Sort sessions by `last_message_at` descending client-side and maintain a stable sort before passing to Virtuoso. New sessions appended at the end avoid index shifts.

## P7: `position:fixed` Drawer Without createPortal

### Risk Profile: Medium (CSS architecture rule violation)

Per `css-architecture.md`: "Always use `createPortal(..., document.body)` for overlays that must escape the component tree."

`ApprovalDrawer` uses `position:fixed` but does NOT use `createPortal` — it renders inline. The insights page layout uses no CSS `transform` or `filter` ancestors (it's a simple flex column), so `position:fixed` works there. However, if the layout ever gains a `transform`, the drawer will break.

**Recommendation**: The new `SessionDetailDrawer` should use `createPortal` (Radix Dialog wraps it automatically). Alternatively, use the existing `Modal`/`ModalContent` component (which uses Radix Dialog with `Portal`) with a custom sheet-style variant — this is safer and respects the architecture rules.

## Summary

- **recharts re-render on live updates** is the primary performance risk — wrap `toDataPoints` in `useMemo` in all chart components to prevent recomputation on unrelated state changes
- **WatchInsights stream drops silently** — add reconnect loop with exponential backoff; current code swallows all errors after initial connection
- **localStorage SSR**: follow `useHistoryFilters` hydration guard pattern exactly — `isHydrated` flag + `useEffect` load; never access `localStorage` in render body
