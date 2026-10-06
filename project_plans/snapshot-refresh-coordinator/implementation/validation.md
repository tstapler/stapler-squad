# Validation Plan: snapshot-refresh-coordinator

**Date**: 2026-08-07

## Happy Path Scenario
Given the Baseline (no in-flight tracking exists today for `ListSessions` calls at any of the 4 call sites in `useSessionService.ts`), when a caller invokes `useSessionServiceContext().listSessions(...)` with no other call site concurrently fetching, then `refreshCoordinatorRef.current.request(fetcher, onResult)` runs the fetcher immediately, invokes `onResult` exactly once with the resolved `ListSessionsResponse`, settles the caller's own promise, and returns the coordinator to `{ kind: "idle" }` — identical externally-observable behavior to today's uncoordinated single-call path, proving the coordinator adds zero overhead in the common case while closing the race in the concurrent case.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Success Metric 1 — no two `ListSessions` RPCs in flight concurrently (single-caller baseline) | `refreshCoordinator.test.ts` | `request_should_invokeFetcherOnce_When_calledWithNoConcurrentActivity` | Unit (happy) | Fresh coordinator, one `request()` call, fetcher resolves — fetcher called once, `onResult` called once, state returns to `idle`. |
| Success Metric 1 — no two `ListSessions` RPCs in flight concurrently (burst collapse, the core coalescing mechanism) | `refreshCoordinator.test.ts` | `request_should_collapseBurstToLatestCaller_When_NCallsArriveWhileOneInFlight` | Unit (happy — 3-caller burst) | While A is in flight, B then C call `request()`; only C's fetcher/onResult wins the coalesced slot, B is never invoked — proves ≤1-in-flight holds under burst load. |
| Success Metric 1 — sites #3/#3b (backwards-jump resync, success + error path) share one coordinator | `useSessionService.test.ts` | `watchSessions_should_coalesceBackToBackResyncTriggers_When_bothSuccessAndErrorPathsFireCloseTogether` | Integration | Both resync call sites fire close together; asserts `mockListSessions` call count stays at 1, not 2. |
| Success Metric 1 — site #4 (staleness backstop) coordinates via site #2's already-wired path | `useSessionService.test.ts` | `backstopReconnect_should_coalesceWithAnInFlightInitialSnapshotFetch_When_bothInvokeStartStreamConcurrently` | Integration | Backstop-triggered `watchSessions()` re-enters `startStream()` while an earlier snapshot fetch is still in flight; asserts `mockListSessions` call count stays at 1. |
| Success Metric 2 — a superseded response's `onResult` is never invoked (stale-response discard) | `refreshCoordinator.test.ts` | `request_should_discardStaleOnResult_When_aSupersededFetchResolvesAfterANewerOne` | Unit (error/edge path — out-of-order resolution) | A starts, B coalesces; B resolves first (its rerun completes), then A resolves late — `onResultA` never called, `onResultB` called once. |
| Success Metric 2 — generation invariant holds across 3+ chained coalesces | `refreshCoordinator.test.ts` | `request_should_neverInvokeOnResultForAnIntermediatelyCoalescedCaller_When_MultipleCallsQueueInSuccession` | Unit (edge path) | Chained A→(B coalesced over by C)→C→(D queued); only A's and C's `onResult` ever fire, never B's. |
| Success Metric 2 & 3 — an older response never clobbers newer Redux state (the original bug, proven at store level) | `useSessionService.test.ts` | `listSessions_should_reflectOnlyTheNewerCallsData_When_anOlderCallsResponseResolvesAfterANewerOnesResponse` | Integration (Redux store + `renderHook`) | Two overlapping `listSessions()` calls (A filtered, B unfiltered) resolve out of order — B (issued second) resolves last but is the *newer* fetch; store ends up reflecting only B's data via `selectAllSessions`, and `loading === false` (not stuck). |
| Scope — per-caller settle, including the coalesced-caller failure fix (Risk Control row 1: reference-sketch bug) | `refreshCoordinator.test.ts` | `request_should_resolveEveryCoalescedWaiter_When_theCoalescedFetchSucceeds` | Unit (happy) | A coalesced caller B's own returned promise resolves once the rerun it rode along with succeeds. |
| Scope — per-caller settle, error path (prevents stuck `dispatch(setLoading(false))`) | `refreshCoordinator.test.ts` | `request_should_rejectAllCoalescedWaiters_When_theCoalescedFetchRejects` | Unit (error) | The coalesced rerun's fetcher rejects; B's promise rejects with the same error instead of hanging forever. |
| Scope — direct (non-coalesced) caller's own fetch failure still propagates | `refreshCoordinator.test.ts` | `request_should_rejectCallersOwnPromise_When_itsOwnFetcherRejectsAndNoCoalescingOccurred` | Unit (error) | A single `request()` call with no coalescing whose fetcher rejects — the caller's own promise rejects, not silently swallowed. |
| ADR-001 accepted tradeoff — a differently-filtered coalesced caller loses its own filtered result (must be a documented, tested behavior, not a silent surprise) | `useSessionService.test.ts` | `listSessions_should_loseItsOwnFilteredOnResult_When_coalescedBehindADifferentlyFilteredLaterCaller` | Integration | `listSessions({status: ACTIVE})` coalesces behind an unfiltered resync call; final dispatch reflects the unfiltered result, and the filtered caller's own `onResult` is proven never invoked. |
| Constraint — `streamGenerationRef` guard (6 checkpoints) must not regress | `useSessionService.ts` (review only) | Task 2.1.6a manual diff-review checklist | Non-test verification | Grep `streamGenerationRef.current` post-wiring; confirm all 6 checkpoints present, byte-identical condition, relocated only inside `onResult` bodies where required — **not covered by an automated assertion**, tracked as a manual gate per the plan. |
| Constraint — `sessionsSlice.ts` tombstone (`deletedIds`) / no-op-upsert skip unchanged | *(existing suite)* `sessionsSlice.test.ts` | *(pre-existing tests, unmodified)* | Regression (existing) | No new test needed — this item is explicitly out of scope for new coverage; existing `sessionsSlice.test.ts` tests continue to pass unmodified, confirming no regression. |

## UX Acceptance Tests
N/A — pure internal refactor, no new UI surface (confirmed: no `design/ux.md` exists for this project; `requirements.md`'s Users/Consumers section states "no user-facing behavior change expected").

## Test Stack
- **Unit**: Jest (existing web-app test stack), manually-resolved-promise pattern (no fake timers) per `useGenerateRule.test.ts:103-130`, for `refreshCoordinator.ts` in isolation — no React/ConnectRPC imports.
- **Integration**: Jest + React Testing Library `renderHook`, mocked ConnectRPC client (`clientRef.current.listSessions`), real `sessionsSlice` reducer wired to a test Redux store — proves the fix at the level the original bug (`requirements.md` Problem Statement) was described: Redux store state, not just coordinator-internal state.
- **E2E / UX**: N/A — no UI surface change; not applicable per Constraints ("frontend only... no user-facing behavior change").

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="refreshCoordinator|useSessionService"` | ≥80% line on new/touched files (`refreshCoordinator.ts`, `useSessionService.ts`'s 4 wired call sites) |
| Type check | `cd web-app && npx tsc --noEmit` | 0 errors — confirms `RefreshCoordinator<ListSessionsResponse>`'s generic wiring type-checks across all 4 call sites (plan.md Verification section) |

- All public methods of `RefreshCoordinator<T>` (`request()` is the sole public method): happy path + error path covered (see table above).
- The one external integration point (`clientRef.current.listSessions`, a ConnectRPC call): unit-mocked in `refreshCoordinator.test.ts` (fetcher closures) and integration-tested against the real Redux store in `useSessionService.test.ts`.
- Migration test: **N/A** — no schema, database, or proto changes (`requirements.md` Constraints: "frontend only... no backend or proto changes"; plan.md's own Migration Plan section is empty for the same reason).
