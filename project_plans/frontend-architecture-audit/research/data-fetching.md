# Data Fetching Patterns — Research Findings

## Hooks That Call ConnectRPC / API Layer

22 hooks in `lib/hooks/` make API calls. Categorized by pattern:

### Pattern A: Redux-backed ConnectRPC streaming (useSessionService)
`useSessionService.ts` (646 lines) is the primary data-fetching hub. It:
- Creates a ConnectRPC client directly with `createClient`
- Dispatches to `sessionsSlice` Redux store for all state
- Exposes `loading` and `error` via `useAppSelector(selectSessionsLoading)` / `selectSessionsError`
- Error handling: every `catch` block runs `dispatch(setError(error.message))` and re-throws nothing — errors are fire-and-forget stored in Redux
- Wraps a WebSocket streaming connection for `watchSessions`

This hook is instantiated in: `SessionServiceContext` (global), `OmnibarContext` (second instance, `createSession` only), and `useSessionActions` (third instance, re-wraps CRUD methods for CockpitShell).

### Pattern B: Redux-backed ConnectRPC with watch transport (useReviewQueue)
`useReviewQueue.ts` (~230 lines) follows the same Redux dispatch pattern as `useSessionService` but for review queue state. It:
- Creates its own ConnectRPC client
- Dispatches to `reviewQueueSlice`
- Error handling: `catch (err) { dispatch(setError(err instanceof Error ? err.message : "Failed...")) }` — same string-in-Redux pattern
- Supports both WebSocket push and 30-second fallback polling

### Pattern C: RTK Query polling (useApprovals, ApprovalsContext)
`useApprovals.ts` (103 lines) uses RTK Query `useGetApprovalsQuery` with `pollingInterval: 5000`. Error handling converts RTK Query's error shape to `Error | null` via a `useMemo`. Returns `{ approvals, loading, error, approve, deny, refresh }`.

`ApprovalsContext.tsx` (67 lines) is a near-identical wrapper around the same `useGetApprovalsQuery` call with the same error conversion logic. The only difference: `ApprovalsContext` does not accept a `sessionId` filter — it holds all approvals globally.

**Duplication:** Both files contain this identical error conversion block:
```ts
typeof queryError === "object" && "error" in queryError
  ? String((queryError as { error: unknown }).error)
  : "Unknown error"
```
`useApprovals.ts` wraps it in `useMemo`; `ApprovalsContext.tsx` does it inline. There is no shared helper.

### Pattern D: Direct ConnectRPC client per hook
`useSessionVcs.ts`, `useVcsStatus.ts`, `useBranchSuggestions.ts`, `useWorktreeSuggestions.ts`, `usePathCompletions.ts`, `useRepositorySuggestions.ts`, `useFileService.ts` each create their own `createClient(SessionService, transport)` instance. There is no shared transport or client factory — every hook calls `createConnectTransport` directly. 

`useTerminalStream.ts`, `useTerminalSnapshot.ts`, `useLiveTail.ts` use a custom `createWatchTransport` (WebSocket-based) for streaming, also each creating their own transport.

### Pattern E: History-specific hooks with pagination
`useNotificationHistory.ts`, `useSearchHistory.ts`, `usePathHistory.ts` manage local pagination state with `useState` + a `loadMore` callback pattern. No shared pagination abstraction.

## Error Handling Inconsistencies

Three distinct error handling patterns exist with no standard:

1. **Redux error string** (`useSessionService`, `useReviewQueue`): `dispatch(setError(error.message))`. Error is a serialized string in Redux; consumers reconstruct `Error | null` from the string selector.

2. **RTK Query error object → Error conversion** (`useApprovals`, `ApprovalsContext`): `isLoading`/`error` from RTK Query are manually converted to the `{ loading: boolean; error: Error | null }` shape used everywhere else.

3. **Direct throw / undefined** (`usePathCompletions`, `useRepositorySuggestions`, etc.): These hooks use try/catch internally and return empty arrays on failure with no exposed error state at all.

The return interface `{ loading: boolean; error: Error | null }` appears in ~8 hook return types but each constructs it differently.

## Duplicate Logic Instances

`useApprovals` and `ApprovalsContext` are functionally duplicated — both poll the same RTK Query endpoint with the same interval, both expose `approve`/`deny`/`refresh`, both convert the error shape identically. When `ApprovalPanel` (uses `useApprovals`) and `ApprovalNavBadge` (uses `ApprovalsContext`) are both mounted, two independent 5-second polling loops run against the same endpoint.

`useSessionService` is instantiated three times in the running app: the `GlobalSessionServiceProvider` (global), `OmnibarContext` (for `createSession` only), and `useSessionActions` (for CRUD callbacks passed to CockpitShell). All three share state via Redux but independently initialize the hook's internal refs and callbacks.

`useReviewQueue` (used via `ReviewQueueContext`) and direct imports of `useReviewQueueContext` in components call the same underlying data — there is one instantiation point here, but `ReviewQueuePanel` also calls `useApprovalsContext` for approve/deny, creating a dependency on two separate data sources for one component.

## Loading State Patterns

Components render loading states inconsistently:
- Some check `loading && <Spinner />` before rendering content
- Some render content with stale data while `loading` is true (optimistic)
- `useSessionService` never resets `loading` to `true` on subsequent list calls unless the Redux state transitions — there is no per-operation loading granularity; it is a global `sessions.loading` flag
