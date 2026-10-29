# Requirements: Backlog Page Performance

## Problem Statement

The backlog page loads and switches views slowly. Users experience sluggish initial loads, slow view transitions between list/board, and laggy re-renders when live updates arrive via the WatchBacklogItems stream.

## Goals

Make the backlog page feel snappy:
- Instant data on first render when the Redux store is already warm (i.e., navigating back)
- Fast initial load when the store is cold
- Smooth list ↔ board view switching without full stream teardown/reconnect
- No UI stutter when live stream updates arrive

## Non-Goals

- Full server-side pagination with cursor navigation in the UI (complexity not justified by typical backlog sizes)
- Replacing the Redux store with a different state management library
- Moving away from ConnectRPC / WatchBacklogItems streaming

## Acceptance Criteria

1. The backlog list page renders its rows without blocking on a network round-trip when the Redux store already holds backlog items (i.e., cold mount shows skeleton; warm mount renders items immediately before the refresh RPC completes).
2. `sortedItems` in `page.tsx` is wrapped in `useMemo` keyed on `visibleItems`, `sortCol`, and `sortAsc` so the sort only re-runs when those values change.
3. `BacklogTableRow` is wrapped in `React.memo` so individual rows do not re-render when an unrelated item's live update arrives.
4. The `annotateDuplicatePending` N+1 in `backlog_service_query.go` is eliminated: the already-loaded session data from Phase 2/3 is used instead of re-querying `ListItemSessions` per review-status item.
5. A composite `(priority, updated_at)` index is added to `backlog_items` so the default `ORDER BY priority ASC, updated_at DESC` sort is served by an index rather than a filesort.
6. The `useWatchBacklogItems` mount refresh is skipped when the Redux store already has items, eliminating the redundant `listBacklogItems` RPC on warm navigations.
7. View switching between list and board does not re-establish the WebSocket stream when the existing stream is still live; a shared subscription (via layout or context) is preferred.

## Out-of-Scope (this iteration)

- Server-side pagination (deferred to follow-up)
- Server-side result caching / TTL layer
- Batching the sequential GitHub issue import N+1 (separate UX task)
