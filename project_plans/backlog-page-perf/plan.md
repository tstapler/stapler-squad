# Plan: Backlog Page Performance

## Phased Approach

Fix the highest-impact, lowest-risk items first (render path), then backend queries, then view switching architecture.

---

## Phase 1 — Frontend Render Path (Highest Impact, Zero DB Risk)

### Task 1.1 — Memoize `sortedItems`

**File**: `web-app/src/app/backlog/page.tsx`  
**Change**: Wrap `sortedItems` in `useMemo` with `[visibleItems, sortCol, sortAsc]` deps.

```ts
const sortedItems = useMemo(
  () => [...visibleItems].sort((a, b) => { /* existing comparator */ }),
  [visibleItems, sortCol, sortAsc]
);
```

The sort runs O(n log n) on every render today. With `useMemo` it runs only when the sort configuration or filtered item set changes.

### Task 1.2 — Memoize `BacklogTableRow`

**File**: `web-app/src/app/backlog/page.tsx`  
**Change**: Wrap `BacklogTableRow` with `React.memo`.

```ts
const BacklogTableRow = React.memo(function BacklogTableRow({ ... }) { ... });
```

Today every stream heartbeat or unrelated item update causes all N visible rows to re-render. With `React.memo`, only rows whose `item`, `isActive`, or `isSelected` props actually changed will re-render.

**Props to stabilize** (must use stable references or primitive values):
- `onRowClick` — wrap in `useCallback` (keyed on `[]`)
- `onToggleSelect` — wrap in `useCallback` (keyed on `[]`)
- `isActive`, `isExiting`, `isSelected` — already primitives
- `isSelectMode` — already primitive

### Task 1.3 — Group items with `useMemo`

**File**: `web-app/src/app/backlog/page.tsx`  
**Change**: Ensure `groupedItems` derivation (from `sortedItems`) is also in `useMemo`.

---

## Phase 2 — Backend N+1 Fix

### Task 2.1 — Eliminate `annotateDuplicatePending` N+1

**Files**: `server/services/backlog_service_query.go`  
**Root cause**: `annotateDuplicatePending` (line ~570) calls `storage.ListItemSessions(ctx, p.Id)` per review-status item. Query 3 in `ListBacklogItemSummaries` already loaded `item_sessions` for all items via `WithBacklogItem().All(ctx)`.

**Fix**: Pass the already-loaded sessions (from the Phase 2/3 batch query) down to `annotateDuplicatePending` as a parameter, keyed by item ID. Eliminate the per-item DB call.

Sketch:
```go
// In ListBacklogItemSummaries or backlog_service_query.go:
sessionsByItemID := make(map[string][]*ent.ItemSession)
for _, s := range allSessions {
    id := s.BacklogItemID.String()
    sessionsByItemID[id] = append(sessionsByItemID[id], s)
}

// Pass down to annotateDuplicatePending:
func (s *BacklogService) annotateDuplicatePending(
    ctx context.Context,
    item *backlogpb.BacklogItem,
    sessions []*ent.ItemSession,
) { ... }
```

### Task 2.2 — Add `(priority, updated_at)` DB index

**File**: `session/ent/schema/backlog_item.go`  
**Change**: Add composite index to cover the default sort:

```go
index.Fields("priority", "updated_at"),
```

Then run ent codegen (`make ent-gen`) and create a new Flyway/Atlas migration. This allows SQLite to serve the default `ORDER BY priority ASC, updated_at DESC` from an index instead of a filesort, especially when the WHERE clause is a NOT IN negation that prevents index use on the existing status-prefixed indexes.

---

## Phase 3 — Skip Redundant RPC on Warm Navigation

### Task 3.1 — Guard `useWatchBacklogItems` mount refresh

**File**: `web-app/src/lib/hooks/useWatchBacklogItems.ts`  
**Change**: In the mount `useEffect`, check if the Redux store already has items before firing `listBacklogItems`. If the store has items, skip the snapshot RPC and rely on the stream's snapshot events to fill any gaps.

```ts
useEffect(() => {
  const storeHasItems = Object.keys(store.getState().backlogItems.items).length > 0;
  if (!storeHasItems) {
    refresh(); // cold mount only
  }
  // always establish the stream
}, [statusFilterKey]);
```

This eliminates the redundant REST RPC on every list↔board navigation where the Redux store is already populated.

---

## Phase 4 — Shared Stream Subscription (View Switching)

### Task 4.1 — Hoist `useWatchBacklogItems` into the backlog layout

**File**: `web-app/src/app/backlog/layout.tsx`  
**Change**: Move the `useWatchBacklogItems` call from `page.tsx` and `board/page.tsx` into the shared `backlog/layout.tsx`. Expose stream state via a context or pass it down as props. The stream stays alive when the user switches between list and board views because the layout component is never unmounted during those navigations.

This is the highest-impact view-switching fix: one stream, shared between both views. The stream going from "connecting" → snapshot → "live" on every tab switch is completely eliminated.

**Layout context pattern** (to avoid prop drilling):
```ts
// backlog/layout.tsx
export const BacklogStreamContext = createContext<BacklogStreamState | null>(null);

export default function BacklogLayout({ children }) {
  const stream = useWatchBacklogItems();
  return (
    <BacklogStreamContext.Provider value={stream}>
      {children}
    </BacklogStreamContext.Provider>
  );
}
```

---

## Risk Register

| Risk | Likelihood | Mitigation |
|---|---|---|
| `React.memo` breaks row highlight if prop identity is unstable | Medium | Audit all props; wrap callbacks in `useCallback` before wrapping row in memo |
| Ent codegen for new index needs manual migration | Low | Follow existing `make ent-gen` + migration workflow documented in CLAUDE.md |
| Layout-hoisted stream misses per-view filter state | Low | Pass filter config into `useWatchBacklogItems` from layout; board and list use same filter defaults |
| Warm-store guard skips refresh that was needed | Low | Stream snapshot events will still deliver current state; guard is only for the redundant one-shot REST call |

## Implementation Order

1. Phase 1 (render path) — largest perceived-performance impact, no backend changes, safe to ship independently
2. Phase 2 (backend N+1 + index) — backend changes, needs ent migration, ship behind feature branch
3. Phase 3 (warm store guard) — small hook change, dependent on Phase 1 being stable
4. Phase 4 (shared stream) — largest refactor, most risk, ship last after other phases validate
