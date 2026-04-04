# ADR-008: Redux Toolkit for Frontend State Management

## Status
Accepted

## Context

The web UI frontend (`web-app/`) grew organically around React Context API and custom hooks. By the time this ADR was written the codebase had:

- **5 Context providers** (`NotificationContext`, `AuthContext`, `ReviewQueueContext`, `ApprovalsContext`, `OmnibarContext`)
- **23+ custom hooks**, each independently managing its own state, polling intervals, and ConnectRPC client
- **9–10 separate ConnectRPC transport instantiations** — one per hook, all pointing at the same backend
- **No coordinated cache invalidation** between related data domains (e.g. approvals triggering a session refresh)
- **Heavy prop drilling** — `SessionDetail` accepted 12+ props; `SessionList` accepted 8+
- **Manual localStorage persistence** duplicated across 7+ hooks via scattered `useEffect` pairs
- **Stale closure workarounds** — multiple `// eslint-disable-next-line react-hooks/exhaustive-deps` suppressions and `useRef` indirection patterns to break dependency cycles in streaming hooks

The pain was concentrated in the **consumption layer** (accessing and sharing state) and the **polling coordination layer** (multiple hooks independently fetching the same conceptual data). The fetching logic itself — ConnectRPC calls, WebSocket streams, retry handling — was already well-isolated in individual hooks.

### Why not RTK Query?

RTK Query (the data-fetching extension of Redux Toolkit) was evaluated and rejected for this migration. The primary blockers were:

1. **ConnectRPC streaming incompatibility**: The frontend has three long-lived server-push streams (`watchSessions`, `watchReviewQueue`, and the terminal WebSocket). RTK Query's `onCacheEntryAdded` can wrap server-streaming connections, but the terminal stream (`useTerminalStream`) is bidirectional — it sends `Input` and `Resize` messages back to the server — which RTK Query has no model for.

2. **Protobuf serialization boundary**: `@bufbuild/protobuf` generates class instances with methods, BigInt fields, and non-enumerable internal fields. RTK Query stores responses in Redux state which must be serializable plain objects. A full normalization boundary (serializing every protobuf response to a plain object at the cache layer) is a larger, riskier refactor than the current migration.

3. **Risk/reward during spike**: The goal of this change is to reduce consumption complexity and improve DevTools observability — not to rebuild the fetching layer. Base RTK achieves the goal with a fraction of the migration risk.

RTK Query remains a viable future step for the pure polling hooks (`useApprovals`, `useApprovalRules`, `useApprovalAnalytics`) once the protobuf serialization boundary decision is made.

## Decision

Adopt **base Redux Toolkit** (store, slices, `createEntityAdapter`) for shared frontend state. The fetching hooks (`useApprovals`, `useReviewQueue`, `useSessionService`) retain all their existing fetching, polling, and ConnectRPC client logic. The only change is **where state lives**: hooks dispatch to the Redux store instead of calling local `setState`.

### What changes

| Before | After |
|--------|-------|
| Each Context provider wraps a hook and re-renders its subtree | State lives in Redux; components read via `useAppSelector` |
| Sessions stored in `useState` inside `useSessionService` | Sessions in `sessionsSlice` via `createEntityAdapter` (normalised, deduped) |
| Approvals stored in `useState` inside `useApprovals` | Approvals in `approvalsSlice` |
| Review queue stored in `useState` inside `useReviewQueue` | Review queue in `reviewQueueSlice` |
| Prop drilling through `SessionList` → `SessionCard` → `SessionDetail` | Components select directly from store |
| Manual `useEffect` + `localStorage` per filter | `redux-persist` (future: not in initial migration) |

### What does not change

- All ConnectRPC client creation, polling intervals, streaming connections
- `AuthContext` — local auth state, no cross-component sharing needed
- `OmnibarContext` — ephemeral UI modal state, not shared data
- `useTerminalStream`, `useTerminalFlowControl`, `useTerminalMetrics` — terminal streaming is architecturally isolated and must remain so

### Key implementation decisions

**`serializableCheck: false`**
Redux Toolkit's default middleware warns when non-serializable values are stored in state. Protobuf `Message` instances fail this check. Since the prior `useState` approach stored the same objects with the same non-serializable characteristics, disabling the check is a lateral move with no change in actual behaviour. The comment in `store.ts` documents this explicitly.

**Errors stored as `string | null` in Redux**
Redux state must be serializable. `Error` objects are not (their stack traces and prototype chain cause issues). Hook return values convert the stored string back to `Error | null` via `useMemo` for full backward compatibility with existing consumers.

**WebSocket events trigger `refresh()` in Phase 1**
The `watchReviewQueue` stream delivers incremental `itemAdded`, `itemRemoved`, and `itemUpdated` events. In the original code, these were handled via `setState` functional updaters that read-and-mutate the current queue. With Redux, the equivalent would be a normalised `ReviewItem` entity adapter — which is Phase 2 work. In Phase 1, WebSocket push events call `refresh()` to trigger a full re-fetch. The 30-second fallback poll is the safety net between push events.

**`handleSessionEvent` dispatches `updateSessionStatus` rather than reading store state in a closure**
The `statusChanged` stream event previously used a `sessions.find()` closure which forced `handleSessionEvent` (and transitively `watchSessions`) to get a new identity on every session change, causing WebSocket reconnects. The `updateSessionStatus` reducer action runs inside the reducer where Immer state is always current, breaking the dependency cycle entirely.

**`removeItem` uses explicit return, not Immer mutation**
Protobuf class instances are not plain objects; Immer may not correctly proxy them. The `removeItem` reducer uses the explicit-return pattern (`return { ...state, reviewQueue: new ReviewQueue(...) }`) to bypass Immer's proxy entirely, while still benefiting from RTK's action/reducer structure and test infrastructure.

## Consequences

### Positive
- **Redux DevTools**: Full time-travel debugging, action replay, and state inspection for sessions, approvals, and review queue
- **Prop drilling eliminated**: Components anywhere in the tree can call `useAppSelector(selectAllSessions)` without callback chains
- **Stable function identities**: `handleSessionEvent` no longer captures `sessions` in its closure, preventing WebSocket reconnect storms on state changes
- **Optimistic updates use slice actions**: `removeApproval(id)` and `removeItem(sessionId)` are named, tested, single-responsibility actions rather than inline `setState` callbacks
- **41 reducer tests**: Slice logic is unit-testable without React, ConnectRPC mocks, or component setup
- **Foundation for RTK Query**: The store is already configured; adding RTK Query for pure REST hooks is additive, not structural

### Negative
- **`serializableCheck: false`**: Redux DevTools cannot serialize state snapshots for time-travel replay of protobuf-containing state slices. Debugging state history shows class instances rather than plain JSON.
- **WebSocket push events are degraded to full re-fetches** in Phase 1 of this migration. The review queue has slightly higher latency on `itemAdded` events than before (full re-fetch vs. in-place append).
- **Two state systems in parallel**: `AuthContext` and `OmnibarContext` remain as Context. Developers need to know which system owns which state.

## Migration Phases

This ADR covers **Phase 1** only.

| Phase | Scope | Status |
|-------|-------|--------|
| 1 | Redux store + slices for sessions, approvals, review queue; hooks dispatch to store | ✅ Complete (`spike/rtk-state-management`) |
| 2 | Normalised `ReviewItem` entity adapter; WS `itemAdded`/`itemRemoved` handled in-place | Future |
| 3 | RTK Query for pure polling hooks (`useApprovals`, `useApprovalRules`, `useApprovalAnalytics`) | Future — requires protobuf serialization boundary decision |
| 4 | Migrate `AuthContext` / `OmnibarContext` to Redux if cross-component sharing becomes needed | Future — low priority, currently has no pain |

## References
- [Redux Toolkit docs](https://redux-toolkit.js.org/)
- [createEntityAdapter](https://redux-toolkit.js.org/api/createEntityAdapter)
- [RTK Query streaming with `onCacheEntryAdded`](https://redux-toolkit.js.org/rtk-query/usage/streaming-updates)
- `docs/tasks/frontend-quick-wins.md` — existing frontend improvement tracking
