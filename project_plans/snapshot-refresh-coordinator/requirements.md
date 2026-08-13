# Requirements: snapshot-refresh-coordinator

**Date**: 2026-08-06
**Type**: refactor (frontend, session list refresh path)
**Complexity**: 2 — single-file utility + wiring into one existing hook

## Problem Statement
Session list refreshes in `useSessionService.ts` (`web-app/src/lib/hooks/useSessionService.ts`) are triggered from multiple independent call sites with no coordination: the caller-invoked `listSessions()` (line 212), the watch-stream's initial snapshot and backwards-jump full resync (lines 840, 876, 921), and the 30s staleness backstop that re-invokes `watchSessions()` (line 971, which itself calls `listSessions` again at line 840). Each of these can fire a `ListSessions` RPC while another is still in flight. `setSessions` (`web-app/src/lib/store/sessionsSlice.ts:38`) does a full replace of the Redux entity adapter's contents on every dispatch, with no sequencing guard — if an older, slower `ListSessions` response resolves after a newer, faster one, the older response's `dispatch(setSessions(...))` overwrites the store with stale data, silently reverting any session state that changed in between (including sessions the newer response added, updated, or that a `WatchSessions` stream event upserted concurrently).

## Baseline
Today there is no in-flight tracking for `ListSessions` calls at all — `listSessions()` (useSessionService.ts:212) has no guard against a second call starting before the first resolves, and none of the four call sites check whether another is already in flight before firing. The existing `streamGenerationRef` counter (declared line 185, checked at 844/869/891/914/925/936) does guard the `WatchSessions` *stream* lifecycle against a stale reconnect loop resuming after a newer one started, but it does not cover plain `listSessions()` RPC calls, which are a separate code path entirely. There have been no reported user-visible symptoms from this gap (ConnectRPC responses are typically fast and arrive close to in order in practice), but the race is real and unguarded, per the herdr-web-inspired backlog item motivating this work.

## Users / Consumers
Internal: the `GlobalSessionServiceProvider` (`web-app/src/lib/contexts/SessionServiceContext.tsx`) and any component calling `useSessionServiceContext().listSessions()`. No external consumers — this is an internal data-consistency hardening change with no user-facing behavior change expected (other than possibly *fewer* redundant network calls during event bursts).

## Success Metrics
- No two `ListSessions` RPCs are in flight concurrently from `useSessionService.ts`'s internal call sites (listSessions, watch-stream initial snapshot, backwards-jump resync, staleness-backstop-triggered reconnect).
- A `ListSessions` response that started before a subsequent request never overwrites state with data older than what the subsequent (or a concurrent `WatchSessions` event) already applied.
- Verified via unit tests on the extracted coordinator utility (pure, no React/RPC dependencies) plus an integration-level test on `useSessionService`/`sessionsSlice` simulating out-of-order resolution of two overlapping `listSessions()` calls.

## Appetite
Small (1–3 days) — this is a single pure-utility extraction plus wiring into existing call sites, not a new subsystem.

## Constraints
- Must not change the existing `ListSessions`/`WatchSessions` RPC contracts (`proto/session/v1/session.proto`) — this is a client-side coordination fix only.
- Must not regress the existing `streamGenerationRef` guard on the `WatchSessions` stream reconnect loop — the new coordinator wraps `listSessions`-triggering call sites, it does not replace the stream generation counter.
- Must not regress `sessionsSlice.ts`'s existing `deletedIds` tombstone filtering (line 39) or the no-op-upsert skip (line 51) — those remain the mechanism for per-session staleness; the coordinator solves whole-snapshot ordering, not per-field merge.
- Frontend only (`web-app/src/lib/**`); no backend or proto changes.

## Non-functional Requirements
- **Performance SLO**: reduces redundant `ListSessions` calls during event bursts (a stated motivation) — should not add measurable latency to the common single-request case.
- **Scalability**: not applicable — single coordinator instance per `useSessionService` hook instance, same as today's per-hook refs.
- **Security classification**: internal, no auth/data-exposure surface change.

## Scope
### In Scope
- Extract a small, pure, framework-agnostic `createRefreshCoordinator<T>()` (or equivalently scoped) utility modeled on herdr-web's `refreshCoordinator.ts` pattern: `request()` collapses a burst of concurrent refresh requests into "one in flight + at most one pending re-run after it completes," and discards/ignores a response if a newer request has since started (the "barrier generation" concept from the reference implementation).
- Wire the coordinator into `useSessionService.ts`'s internal `listSessions`-triggering call sites (the four listed in Problem Statement) so only one `ListSessions` fetch is in flight at a time, with newer requests properly coalesced rather than dropped.
- Unit tests for the coordinator utility in isolation (request coalescing, pending re-run behavior, stale-response discard).
- A regression test demonstrating the out-of-order-response scenario is now handled correctly (older response no longer clobbers newer state).

### Out of Scope
- Changing `WatchSessions` stream reconnect/backoff logic (`BackoffState`, `streamGenerationRef`) — already has its own generation-counter guard, not part of this item.
- Changing `sessionsSlice.ts`'s per-session merge/dedup logic (`deletedIds`, no-op-upsert skip) — those already exist and are out of scope.
- Any change to other RPC call sites in `useSessionService.ts` unrelated to session-list snapshot refresh (createSession, updateSession, etc.).
- Backend/proto changes.

## Rabbit Holes
- herdr-web's reference implementation includes an `isCurrent` / `getBarrierGeneration` mechanism to discard a snapshot fetched before a *mutation* was applied. stapler-squad's mutation RPCs (create/update/delete session) already optimistically `dispatch(upsertSession(...))` on success rather than waiting for a follow-up list refresh, so a full barrier-generation mutation-fencing mechanism is likely unnecessary scope here — confirm in research/plan whether the simpler "one in flight + latest-wins" coordinator (without mutation barriers) is sufficient, and only add barrier-generation complexity if a concrete race against a mutation is found.
- `watchSessions()`'s initial snapshot (line 840) and the plain `listSessions()` call (line 212) currently both dispatch `setSessions` with the full response; make sure the coordinator wraps the *fetch+dispatch* as one unit, not just the fetch, or the ordering guarantee doesn't actually hold at the Redux-store level.

## Open Questions
- Should the coordinator be a shared singleton across all `useSessionService` hook instances, or per-hook-instance (via `useRef`, matching the existing ref-based pattern in this file)? Current codebase reads as one long-lived hook instance per app (`GlobalSessionServiceProvider`), so per-instance is likely sufficient — confirm in research.
