# ADR-012: RTK Query with Protobuf Serialization Boundary for Unary ConnectRPC Calls

## Status
Accepted

## Context

The current data-fetching architecture centers on `useSessionService.ts`, a ~500-line hook that mixes:
- ConnectRPC unary calls (request/response: list sessions, get approvals, get approval rules)
- ConnectRPC server-streaming calls (terminal output, session watch)
- Redux dispatch for updating state
- `setInterval`-based polling for approval data
- Local error state management

This design has three operational problems:

### Problem 1: Protobuf class instances in Redux state

ConnectRPC responses return protobuf-generated class instances (e.g., `ListApprovalsResponse`). Redux Toolkit's default `serializableCheck` middleware correctly rejects these because class instances are not plain serializable objects. The current workaround is that `serializableCheck` is either disabled or suppressed for the relevant slices. This means:
- Time-travel debugging is broken for these slices (Redux DevTools cannot serialize)
- Shallow equality comparisons in `useSelector` behave unexpectedly (class instances always produce new references)
- Protobuf data in Redux state cannot be persisted or rehydrated safely

### Problem 2: No cache invalidation semantics

Approval data is polled every N seconds unconditionally. When a user approves or rejects a request, the UI optimistically updates but then gets clobbered by the next poll. There is no declarative way to say "this mutation invalidates the approvals list".

### Problem 3: Server-streaming and unary calls are mixed in one hook

Streaming hooks (`StreamTerminalOutput`, `WatchSessions`) have completely different lifecycle management than unary hooks (request once, cache result, re-request on mutation). Mixing them in `useSessionService` makes both harder to reason about.

### Options Evaluated

| Option | Pros | Cons |
|--------|------|------|
| **RTK Query with custom ConnectRPC baseQuery** | Integrated with Redux Toolkit (already a dependency); cache invalidation via tags; automatic polling; serialization boundary enforced at the baseQuery level | Requires custom `baseQuery` implementation; protobuf serialization boundary adds one conversion step |
| **connect-query (TanStack Query + ConnectRPC)** | Official ConnectRPC library; minimal boilerplate | Streaming support is being reworked (Issue #524, March 2025); less stable for this use case; adds TanStack Query as a dependency alongside Redux Toolkit |
| **SWR + manual ConnectRPC calls** | Lightweight | Not integrated with existing Redux store; adds another state management layer |
| **Keep current setInterval polling** | No migration cost | Doesn't solve the serialization problem; no cache invalidation; hard to test |

### Why not connect-query

The `@connectrpc/connect-query-es` library is reworking its streaming support as of March 2025 (Issue #524). Until the v2 streaming redesign stabilizes, it is risky to adopt for a project where streaming is a first-class feature. RTK Query is already in the dependency tree via `@reduxjs/toolkit` — it adds no new packages.

### Streaming endpoints are excluded

ConnectRPC server-streaming calls (`StreamTerminalOutput`, `WatchSessions`) cannot be managed by RTK Query — RTK Query has no first-class streaming story. These hooks remain as manual Redux dispatch hooks. This is not a limitation; streaming hooks have fundamentally different semantics (they run indefinitely until cancelled, not request/response).

## Decision

Use **RTK Query** (`createApi` from `@reduxjs/toolkit/query`) for all ConnectRPC **unary** calls. Streaming calls remain as manual dispatch hooks.

### Serialization boundary

The serialization boundary is enforced at the `baseQuery` level. Every response is converted to a plain JSON object before entering the RTK Query cache:

```ts
// lib/api/serialization.ts
import type { Message } from '@bufbuild/protobuf';

export function toPlainObject<T extends Message>(msg: T): Record<string, unknown> {
  return msg.toJson() as Record<string, unknown>;
}
```

```ts
// lib/api/connectApi.ts
import { createApi } from '@reduxjs/toolkit/query/react';
import { toPlainObject } from './serialization';
import { ConnectError } from '@connectrpc/connect';

const connectBaseQuery = async ({ method, request }: ConnectQueryArg) => {
  try {
    const response = await method(request);
    return { data: toPlainObject(response) };
  } catch (error) {
    if (error instanceof ConnectError) {
      return { error: { status: error.code, error: error.message } };
    }
    return { error: { status: 'UNKNOWN', error: String(error) } };
  }
};

export const connectApi = createApi({
  reducerPath: 'connectApi',
  baseQuery: connectBaseQuery,
  tagTypes: ['Approvals', 'ApprovalRules', 'ApprovalAnalytics', 'Sessions'],
  endpoints: () => ({}),
});
```

### Cache tag strategy

| Endpoint | providesTags | invalidatesTags |
|----------|-------------|-----------------|
| `getApprovals` | `['Approvals']` | — |
| `getApprovalRules` | `['ApprovalRules']` | — |
| `getApprovalAnalytics` | `['ApprovalAnalytics']` | — |
| `approveRequest` | — | `['Approvals', 'ApprovalAnalytics']` |
| `rejectRequest` | — | `['Approvals', 'ApprovalAnalytics']` |

### Polling

RTK Query's built-in `pollingInterval` replaces all `setInterval`-based polling. Default interval: 5000ms for approvals.

### Type reconstruction at the consumption layer

Because protobuf class instances are serialized to plain objects at the boundary, components that need to call methods on protobuf objects must reconstruct them:

```ts
// In a component or selector
import { ListApprovalsResponse, fromJson } from '@buf/...';
const response = fromJson(ListApprovalsResponse, cachedPlainObject);
```

In practice, most display components only read string/number fields — reconstruction is only needed when calling protobuf utility methods.

### What stays as manual dispatch hooks

- `useTerminalStream` — server-streaming; runs indefinitely
- `useWatchSessions` — server-streaming; runs indefinitely
- Any future streaming endpoint

These hooks remain in `lib/hooks/useSessionService.ts` (or a split-out `useStreamingHooks.ts`).

## Consequences

### Positive
- Redux `serializableCheck` can be re-enabled for all RTK Query state (no protobuf class instances in store)
- Approve/reject mutations automatically invalidate the approvals cache — no stale data after action
- Polling managed by RTK Query's `pollingInterval` — no manual `setInterval`/`clearInterval` lifecycle management
- `useSelector` equality checks work correctly on plain objects
- RTK Query adds no new dependencies (already in `@reduxjs/toolkit`)

### Negative / Constraints
- Type reconstruction required when consuming protobuf utility methods from cache (uncommon in practice)
- Custom `baseQuery` requires understanding ConnectRPC error codes for mapping to RTK Query error shape
- `toPlainObject` serialization step adds one allocation per response (negligible performance impact for typical session management workloads)

## References
- RTK Query docs: https://redux-toolkit.js.org/rtk-query/overview
- `@bufbuild/protobuf` `toJson()`: https://buf.build/docs/reference/javascript/bufbuild-protobuf
- connect-query streaming issue: https://github.com/connectrpc/connect-query-es/issues/524
- Research synthesis: `project_plans/front-end-refactor/research/synthesis.md`
- Implementation: Phase 3, Story 3.1 in `docs/tasks/front-end-refactor.md`
