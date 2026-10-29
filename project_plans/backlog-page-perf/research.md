# Research: Backlog Page Performance

## Codebase Findings

### Frontend — `web-app/src/app/backlog/page.tsx` (1,198 lines)

**Data flow on mount:**
1. `useWatchBacklogItems()` fires an immediate `listBacklogItems` REST snapshot unconditionally (in a `useEffect` on `statusFilterKey`, line ~205).
2. A `WatchBacklogItems` gRPC-Web streaming RPC is opened on a `StreamingWSBridge` transport. Snapshot events arrive before the "live" phase begins.
3. Redux `backlogItemsSlice` stores all items keyed by ID. `selectAllBacklogItems` is a `createSelector`-memoized selector.
4. A `WeakMap`-based per-proto mapping cache preserves item object identity across stream events, preventing unnecessary re-renders in deeply memoized consumers.

**Performance issues found:**

| Issue | Location | Severity |
|---|---|---|
| `sortedItems` not in `useMemo` — re-sorts on every render | `page.tsx` ~line 485 | High |
| `BacklogTableRow` not `React.memo` — re-renders all rows on any store update | `page.tsx` `BacklogTableRow` | High |
| Redundant `listBacklogItems` RPC on warm navigation | `useWatchBacklogItems.ts` ~line 205 | Medium |
| View switch (list↔board) = full route navigation + stream teardown/reconnect | `backlog/page.tsx`, `backlog/board/page.tsx` | Medium |
| Sequential N+1 GitHub issue import (20 issues = 20 serial RPCs) | `page.tsx` ~line 600 | Low (UX, not perf) |

**View switching detail:**
- List → Board is a hard `<AppLink href="/backlog/board">` navigation.
- Board page mounts its own independent `useWatchBacklogItems` instance.
- Even though the Redux store persists across navigation, the new mount fires a fresh `listBacklogItems` snapshot unconditionally.
- The new WebSocket stream goes through "connecting" → snapshot delivery → "live" on every switch.

---

### Backend — `server/services/backlog_service_query.go`

**`ListBacklogItems` query plan (5 queries):**
1. `SELECT backlog_items.* + source edge WHERE status [NOT] IN (...) ORDER BY priority ASC, updated_at DESC LIMIT 1000`
2. Pending guidance flags — batched single query against `guidance_requests`
3. Item sessions + review verdicts — batched single query: `WHERE backlog_item_id IN (<all IDs>)`
4. Unresolved blockers — chunked at 500: `WHERE blocked_id IN (<all IDs>)`
5. **N+1**: `annotateDuplicatePending` calls `storage.ListItemSessions(ctx, p.Id)` per review-status item inside the loop at line ~166 — even though query 3 already loaded all sessions

**Index analysis (`session/ent/schema/backlog_item.go`):**

Present indexes:
- `(status, priority)`, `(status, updated_at)`, `(status, queued_at)`, `(status, chain_fired)`, `(status)`, `(external_id)`, `(external_url)`

**Missing**: `(priority, updated_at)` composite index. The default sort is `ORDER BY priority ASC, updated_at DESC`. The default filter is `WHERE status NOT IN ('done','archived','superseded')` — a negation. SQLite cannot range-scan the existing `(status, priority)` index for a NOT IN predicate; it falls back to a full table scan + filesort. A `(priority, updated_at)` index would cover the sort without depending on the status predicate.

**Pagination**: Not implemented. `ListBacklogItemsRequest` has no `limit`/`offset`/cursor fields. Safety cap is hardcoded at 1000 server-side. No client-controlled pagination.

**Server-side caching**: None. Every RPC does a full live SQLite read.

---

## Related Existing Work

- `backlogItemsSlice` already has `upsertItem` staleness guard on `updatedAt`
- `useWatchBacklogItems` already has `WeakMap` proto→domain mapping cache
- `liveVersion` counter on items avoids flash-animation re-renders for unrelated rows (but rows still re-render due to missing `React.memo`)

## Comparable Patterns in This Codebase

- `BacklogItemCard.tsx` (used by Kanban board) already tracks `liveVersion` for in-place flash — this pattern can be reused in `BacklogTableRow`
- `usePersistedViewState` already handles stable references for filter state
