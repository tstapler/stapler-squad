# Research: Edge Cases & Unstated Needs — snapshot-refresh-coordinator

## Confirmed call sites (matches requirements.md exactly)

| # | Site | Location | Filters passed |
|---|---|---|---|
| 1 | `listSessions()` | `web-app/src/lib/hooks/useSessionService.ts:212-238` | `{category, status, includeArchived}` — caller-supplied, varies per call |
| 2 | watch-stream initial snapshot | `useSessionService.ts:838-845` | `{category: watchOptionsRef.current?.categoryFilter, status: watchOptionsRef.current?.statusFilter}` |
| 3 | backwards-jump full resync (success path) | `useSessionService.ts:874-884` | same `watchOptionsRef.current` filters as #2 |
| 3b | backwards-jump full resync (error path) | `useSessionService.ts:918-929` | same as #2, duplicate of #3 in the catch block |
| 4 | 30s staleness backstop | `useSessionService.ts:958-976`, fires `watchSessionsRef.current?.(watchOptionsRef.current)` at line 971, which re-enters `watchSessions()` → site #2's `startStream()` closure | delegates to #2's filters |

Only one long-lived hook instance exists app-wide: `GlobalSessionServiceProvider` (`web-app/src/lib/contexts/SessionServiceContext.tsx:63`) calls `useSessionService` once. This confirms the requirements doc's open question — **per-hook-instance coordinator (via `useRef`) is sufficient; no cross-instance singleton needed.**

## sessionsSlice.ts guards already in place (do not duplicate)

- `deletedIds` tombstone filter — `setSessions` (line 39) strips any session ID marked deleted before `setAll`, and `removeSession` (line 78-81) adds to the tombstone map. This prevents a stale snapshot from resurrecting a just-deleted session, but does **not** prevent a stale snapshot from otherwise overwriting fresher per-field data.
- No-op-upsert skip — only in `upsertSession` (lines 51-59), compares `updatedAt` timestamps to avoid redundant re-renders. **`setSessions` has no equivalent timestamp check** — it's an unconditional full replace (`sessionsAdapter.setAll`), which is exactly the mechanism the coordinator must guard from the outside since the slice itself has no ordering awareness.

## Finding 1: Unmount / re-render behavior — not a real risk for this hook, but confirm scope explicitly

There is no `isMountedRef`/cleanup guard anywhere in `useSessionService.ts` (checked all 9 `useEffect` blocks) — a late-resolving `listSessions()` promise dispatches to Redux regardless of whether the calling render is still current. This is not a React "setState after unmount" warning risk because `dispatch` targets the global Redux store, not local component state, and the store outlives any render. Because `GlobalSessionServiceProvider` mounts the hook exactly once for the app's lifetime (see table above), the coordinator does not need to add an unmount-cancellation story — the existing `streamGenerationRef` pattern already exists for the one thing that *does* get torn down and restarted (the stream itself via `watchSessions()`/`stopWatching()`). Recommend the plan state this explicitly rather than silently assuming it, since it's a real gap in a hook used elsewhere (e.g. `useValidateRules.ts`, `useGenerateRule.ts` both have explicit "abort in-flight request on unmount" comments) — this hook is the outlier because of its singleton-provider usage, not because unmount-safety is unimportant in general.

## Finding 2 (flag per task instructions): distinct-filter requests collapsing into one in-flight call is a real gap in current scope

The 4 call sites pass **different, potentially conflicting filter sets**:
- Site #1 (`listSessions()`) is caller-invoked with arbitrary `{category, status, includeArchived}` — e.g. a filtered board view calling `listSessions({status: SessionStatus.ACTIVE})`.
- Sites #2/#3/#3b/#4 all use `watchOptionsRef.current` (`categoryFilter`/`statusFilter`), which is a **different shape** (no `includeArchived`) and reflects the watch stream's active filter, not necessarily what any given `listSessions()` caller wants.

If the coordinator's `request()` naively collapses *any* concurrent call into "one in flight, discard/merge the rest," a burst where site #1 fires with `{status: ACTIVE}` while site #4's backstop fires with `watchOptionsRef.current` (e.g. unfiltered) would produce one of two bad outcomes depending on implementation:
1. The second caller's filters are silently dropped (its request never actually executes with its own filters — it just rides the first request's response), returning session data scoped to the *wrong* filter to that caller's dispatch, or
2. If the coordinator is keyed only by "is a request in flight" with no key/param awareness, a `status: ACTIVE`-filtered response could get dispatched via `setSessions` (full replace) and *permanently* narrow the visible session set until the next unfiltered call — this directly contradicts the full-replace semantics of `setSessions` (line 38-41), which has no concept of "this replace was scoped to a filter."

**This is a genuine, unstated gap.** herdr-web's reference `refreshCoordinator.ts` (per requirements.md's Rabbit Holes section) presumably coordinates a single homogeneous refresh operation with no parameters — stapler-squad's four call sites are not homogeneous. Two viable resolutions, either of which the plan should pick explicitly:
- **(a) Key the coordinator's in-flight/pending slot by a serialized filter signature** (e.g. `JSON.stringify({category, status, includeArchived})`), so two calls with different filters run independently (each gets its own "one in flight + one pending" queue) and only same-filter bursts coalesce. This preserves correctness at the cost of the coordinator no longer collapsing *all* traffic into one RPC during a burst that happens to mix filters.
- **(b) Restrict the coordinator to only the 3 stream-internal call sites (#2/#3/#3b/#4), which all share `watchOptionsRef.current` and are therefore filter-homogeneous, and leave site #1 (`listSessions()`, caller-supplied arbitrary filters) uncoordinated** — since a UI caller invoking `listSessions({status: X})` deliberately wants a scoped result, not to be silently merged with the stream's unfiltered/differently-filtered refresh.

Recommend the plan phase pick (a) or (b) explicitly and add a unit test asserting that two `request()` calls with different filter keys do not collapse into one — this scenario is not covered by the requirements.md success metrics as written (metric 2 only tests "older response never overwrites newer state," not "different-filter responses never conflate").

## Finding 3: RPC-level timeout/cancellation — already handled upstream, coordinator does not need its own

`clientRef.current.listSessions(...)` goes through `createWatchTransport` (`useSessionService.ts:201-206`) with `createRpcTimingInterceptor` — grepped for a request-level timeout/deadline config; ConnectRPC transports support `timeoutMs` at the call-options level but this codebase does not appear to set one explicitly for `listSessions` (no `{timeoutMs: ...}` passed at any of the 4 call sites). This means a genuinely slow/hung `ListSessions` RPC has no bounded worst case today, independent of the coordinator. This is a pre-existing gap, not something the coordinator introduces or is required to fix (out of scope per requirements.md's "Must not change the existing RPC contracts"), but the plan should note it rather than assume the coordinator provides cancellation — a "pending re-run" queued behind a hung in-flight request will wait indefinitely, which could feel like a regression if a user's later action (e.g. explicitly re-opening the session list) appears to hang because it got coalesced behind a stuck earlier call with no way to know that happened. Consider whether `request()` should expose a way for a caller to observe "my request is still queued behind an earlier one" (even just for a loading-state accuracy concern) — not a blocker, but worth flagging as a UX edge case for the plan phase, especially since `dispatch(setLoading(true))` in `listSessions()` (line 216) already exists and could become inaccurate if a queued-but-not-yet-executed request site doesn't also set loading.

## Finding 4: The same coalescing gap already exists twice elsewhere — signal to generalize

Grepped `web-app/src/lib` for in-flight-request patterns. Two other hooks implement a **strictly weaker** version of the same problem — "skip if already in flight," with no pending-rerun queue, i.e. a poll/refresh tick that lands while a request is in flight is silently **dropped**, not coalesced into a follow-up call:

- `web-app/src/lib/hooks/useStuckBacklogItems.ts:69-87` — `inFlightRef` boolean guard around `fetchItems()`; a `setInterval` poll tick (60s cadence) that fires while a previous fetch is still in flight just returns early and that tick's refresh never happens (next poll is 60s later).
- `web-app/src/lib/hooks/useSessionSummary.ts:94` (`pollInFlightRef`) — same shape, guarding a poll-triggered `getSessionSummary` call.

Neither of these has the "one in flight + at most one pending re-run" guarantee the new coordinator is meant to provide — they have "one in flight, extra requests during that window are lost, not deferred." This is exactly the gap class the herdr-web-modeled coordinator solves. **Signal for generalizing**: since this shape recurs (3 sites now: the 4 `useSessionService` call sites, `useStuckBacklogItems`, `useSessionSummary`), the coordinator utility is a good candidate for a shared location (e.g. `web-app/src/lib/utils/refreshCoordinator.ts`) reusable beyond `useSessionService`, even though requirements.md scopes this item to `useSessionService.ts` only. Recommend noting this as a natural follow-up rather than expanding this item's scope — migrating `useStuckBacklogItems`/`useSessionSummary` to the new utility is additional call-site risk (different RPC shapes, different loading-state semantics) that would blow past this item's "Small (1-3 days)" appetite.

## Recommendations for the plan phase

1. Explicitly decide and document resolution to Finding 2 (filter-keyed coordination vs. scoping the coordinator to only the filter-homogeneous stream-internal call sites) — this is the most consequential open gap, not covered by requirements.md's stated success metrics.
2. State explicitly (as a one-line note, not new work) that unmount-safety is out of scope because `useSessionService` has exactly one long-lived instance app-wide (Finding 1) — avoids a reviewer flagging it as a missed edge case.
3. Note the pre-existing lack of RPC-level timeout as a known gap the coordinator does not fix (Finding 3) — call out the `dispatch(setLoading(true))` interaction if a "pending re-run" queue is added, since a queued call setting loading=true before it's actually started could be misleading.
4. File a follow-up (not in this item) to consider migrating `useStuckBacklogItems.ts` and `useSessionSummary.ts` to the new coordinator utility once it exists (Finding 4) — do not fold into this item's scope.
