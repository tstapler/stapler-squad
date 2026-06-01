# Stack Research — Insights Page Enhancement

## Key Package Versions

From `web-app/package.json`:

- **recharts**: `^3.8.1` — latest v3 series; includes AreaChart, LineChart, BarChart, ResponsiveContainer already in use
- **react-virtuoso**: `^4.18.7` — already installed and actively used for log viewers (`VirtualLogList`)
- **@tanstack/react-virtual**: `^3.13.25` — also installed and used in `SessionList.tsx` (`useVirtualizer`)
- **react-window**: NOT in package.json — do not add; use existing `react-virtuoso` or `@tanstack/react-virtual`
- **@vanilla-extract/css**: `^1.20.1` (devDep); `@vanilla-extract/recipes`: `^0.5.7`; `@vanilla-extract/next-plugin`: `^2.5.1`
- **next**: `15.3.2`
- **react**: `^19.0.0`
- **@radix-ui/react-dialog**: `^1.1.15` — available for slide-over/modal patterns
- **fuse.js**: `^7.3.0` — fuzzy search, can be used for client-side session table filtering

## Skeleton Loader Pattern

A `Skeleton` component already exists at `web-app/src/components/ui/Skeleton.tsx` with a corresponding `Skeleton.css.ts`. It supports `rectangular`, `circular`, and `text` variants with a shimmer animation using `vars.color.borderSubtle`/`borderMuted`. The `SessionCardSkeleton` is the canonical example of how to compose multi-field skeleton loaders from this primitive. No new skeleton infrastructure is needed — reuse `Skeleton` directly.

The CSS uses `@vanilla-extract/css` `keyframes` + vanilla-extract theme tokens, so it works correctly in Next.js SSR contexts (class names are build-time, no hydration flash).

## Dynamic Import Pattern (Already Established)

`InsightsDashboard.tsx` already uses the correct Next.js App Router pattern:

```tsx
const DailySpendChart = dynamic(() => import("./DailySpendChart").then((m) => m.DailySpendChart), { ssr: false });
```

All three recharts charts are lazy-loaded with `ssr: false`. This is the established pattern. New charts (e.g., ProjectionCard) must follow this pattern. The comment in the file notes recharts + D3 = ~1.2MB.

## Virtualization: Prefer react-virtuoso

Two virtualization options exist in-tree:
1. **react-virtuoso** (`Virtuoso`) — used in `VirtualLogList` (both `shared/` and `logs/`); handles variable row heights, follow-output (live-tail), and overscan natively; good for the sessions table
2. **@tanstack/react-virtual** (`useVirtualizer`) — used in `SessionList.tsx` for the main sessions list; more manual but gives exact control

For `SessionsTable`, `react-virtuoso` is the simpler choice: the `Virtuoso` component handles scroll stability better with live updates (its `data` prop diffing avoids full re-renders). `FixedSizeList` from react-window is NOT available — skip it.

## Existing Reusable Components

| Component | Path | Reusable for |
|---|---|---|
| `Skeleton` | `components/ui/Skeleton.tsx` | Insight card/chart/table skeletons |
| `SessionCardSkeleton` | `components/sessions/SessionCardSkeleton.tsx` | Pattern reference |
| `Modal` / `ModalContent` | `components/ui/Modal.tsx` | Could wrap a detail slide-over (uses Radix Dialog + createPortal) |
| `ApprovalDrawer` + CSS | `components/sessions/ApprovalDrawer.css.ts` | Slide-over pattern — `position:fixed`, right-anchored, `slideIn` keyframe |
| `useFilterState` | `lib/hooks/useFilterState.ts` | URL-param-based filter state |
| `useHistoryFilters` | `lib/hooks/useHistoryFilters.ts` | localStorage-backed filter pattern with hydration guard |
| `MultiSelect` | `components/shared/MultiSelect.tsx` | Model filter dropdown |

## CSS Architecture Notes

- All new `.css.ts` files must import from `@/styles/theme-contract.css` via `vars`
- `zIndex` slots: `dropdown: 500`, `modal: 1000`, `dialog: 1070` — slide-over for detail view should use a new `slideOver` slot between `dropdown` and `modal` (e.g., 600) or reuse `modal: 1000`
- `ApprovalDrawer.css.ts` imports from `@/styles/theme.css` (not `theme-contract.css`) — both are valid; prefer `theme-contract.css` for new code
- No CSS modules for new components; `.css.ts` only

## Summary

- **react-window is NOT installed**; use `react-virtuoso` (`Virtuoso`) for the SessionsTable virtual scroll
- **Skeleton infra already exists** at `components/ui/Skeleton.tsx` — compose insight-specific skeletons from it
- **dynamic() with `ssr: false`** is already the established chart loading pattern; all new charts must follow it
