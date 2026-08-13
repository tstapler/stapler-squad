# Research: snapshot-refresh-coordinator — architecture

**Date**: 2026-08-06
**Scope**: pure `createRefreshCoordinator<T>()` (or class-based equivalent) utility + wiring into the 4 `listSessions`-triggering call sites in `web-app/src/lib/hooks/useSessionService.ts`. Does not touch `WatchSessions` stream reconnect/backoff (already covered by `streamGenerationRef`, see `project_plans/client-reconnect/research/architecture.md`).

## 0. Relationship to prior research (`client-reconnect/research/architecture.md`)

That doc researched the **stream** reconnect lifecycle (`watchSessions`/`startStream`, §2–3, §5's Event-Command-Policy table) and is the reason `streamGenerationRef`, `backoffRef` (a `BackoffState` instance, `web-app/src/lib/utils/backoff.ts`), and the whole jittered-backoff/close-code machinery exist in the file today. Its recommendation (§7 file-change map) to add `web-app/src/lib/utils/backoff.ts` as a **pure, framework-agnostic class instantiated via `useRef`** was implemented and is the closest in-repo precedent for this task — see `useSessionService.ts:176`: `const backoffRef = useRef(new BackoffState(1000, 30_000));`. `createRefreshCoordinator<T>()` should follow the exact same shape: a plain class/factory with no React or ConnectRPC imports, instantiated the same way.

**Gap that doc left**: it covers the *stream's* lifecycle (one WebSocket connection, guarded by `streamGenerationRef`) but not the separate, unguarded problem of multiple **unary `ListSessions` RPCs** racing each other and clobbering Redux via `dispatch(setSessions(...))`. `streamGenerationRef` only fences re-entrant `startStream()` calls against each other — it does not (and structurally cannot, since it's specific to one closure) fence the public `listSessions()` call site (line 212) against the stream's own internal `listSessions()` calls, which is exactly the gap this coordinator closes.

**Note on line-number drift**: the prior doc's line numbers (`watchSessions` at 763, backstop `useEffect` at 851, etc.) predate the backoff/jitter work landing. Current line numbers (verified 2026-08-06, this file): `watchSessions` at `useSessionService.ts:814`, `startStream` inner closure at `:831`, backstop `useEffect` at `:959`. Cite current line numbers below.

## 1. The four call sites (verified)

| # | Call site | Line | Params | Dispatches |
|---|---|---|---|---|
| 1 | Public `listSessions()` | `useSessionService.ts:220` (dispatch at `:226`) | `{category, status, includeArchived}` from caller args | `setSessions`, `setError`, conditionally `setSystemMemoryPct` (local `useState`, `:229`) |
| 2 | `startStream()` initial snapshot | `:840` (dispatch at `:845`) | `watchOptionsRef.current` (`categoryFilter`/`statusFilter`) | `setSessions` only |
| 3a | Backwards-jump resync (stream-end branch) | `:876` (dispatch at `:881`) | same as #2 | `setSessions` only |
| 3b | Backwards-jump resync (catch branch) | `:921` (dispatch at `:926`) | same as #2 | `setSessions` only |
| — | Backstop trigger | `:971` | n/a — calls `watchSessionsRef.current(...)`, which re-enters `watchSessions()` → `startStream()` → call site #2 | indirect |

Confirmed external callers of the public `listSessions()` (`web-app/src/app/page.tsx:367`, `web-app/src/components/pane/PaneSplitRenderer.tsx:191`) — the latter passes `{ includeArchived }`, a **different filter** than the stream's `watchOptionsRef`-derived calls. This matters for §4 below. (`useRepositorySuggestions.ts:42` calls `client.listSessions({})` on its own client instance, bypassing the hook entirely — not one of the 4 in-scope call sites.)

## 2. Where the coordinator should live

**Recommendation: `web-app/src/lib/utils/refreshCoordinator.ts`**, sibling to `backoff.ts` in the same directory, same style (plain exported class, no framework imports, colocated `refreshCoordinator.test.ts`).

**Instantiation: per-hook-instance via `useRef`, not a module singleton.** Follow the exact precedent at `useSessionService.ts:176` (`useRef(new BackoffState(...))`):

```ts
const refreshCoordinatorRef = useRef(new RefreshCoordinator<Session[]>());
```

Reasons, citing the existing pattern directly:
- `streamGenerationRef`, `backoffRef`, `isConnectedRef`, etc. (`useSessionService.ts:176–197`) are **all** per-hook-instance refs today — there is no module-level shared state anywhere in this file, even though `GlobalSessionServiceProvider` (`SessionServiceContext.tsx`) means there is only one long-lived hook instance in production. Introducing a module singleton here would be the only piece of shared mutable state in an otherwise ref-scoped file, breaking the file's existing convention for no benefit (requirements.md's own Open Questions section reaches the same conclusion: "per-instance is likely sufficient").
- A module singleton actively **hurts testability**: `useSessionService.test.ts`-style tests (see `web-app/src/lib/hooks/__tests__/`) render the hook multiple times across test cases; a singleton would leak in-flight/pending state across unrelated test cases unless manually reset, whereas a `useRef`-scoped instance is naturally garbage-collected per render tree.
- The requirements doc's own out-of-scope note confirms the mutation/list-service call sites elsewhere (`useRepositorySuggestions.ts`) are separate `client` instances entirely — there's no cross-hook-instance coordination need to justify a singleton's downside.

## 3. Data flow: fetch-only or fetch+dispatch as one atomic unit?

**Fetch + dispatch must be one atomic unit inside the coordinator**, exactly as requirements.md's Rabbit Holes section flags. Concretely: the coordinator's `request()` API must accept the dispatch (or an `onResult` callback that performs it) as a parameter it invokes *itself*, gated by its own internal generation check — not something the caller does unconditionally after `await`ing `request()`.

Why this is required, not optional: with only 1 in-flight RPC guaranteed by the coordinator, the actual hazard isn't two responses racing on the wire (the coordinator already prevents 2 concurrent `ListSessions` calls) — it's a caller that awaits `request()` and then dispatches on its own. Under the coalescing model (§ below), a caller whose request got folded into someone else's pending re-run must **not** dispatch the data it locally captured (it may not even receive a return value corresponding to its own filters — see §4) — it must let the coordinator's internal `onResult` be the only path to `dispatch(setSessions(...))`. If the call site pattern were `const sessions = await coordinator.request(fetcher); dispatch(setSessions(sessions))`, a coalesced caller could easily still be holding a stale `sessions` value it fetched at a different time, or worse, callers could accidentally interleave two dispatches from two different `.then()` chains with no ordering guarantee — reintroducing the exact race this utility exists to remove.

Concrete call-site shape (site #1, `listSessions()` at `useSessionService.ts:212-240`):

```ts
const listSessions = useCallback(async (listOptions?) => {
  if (!clientRef.current) return;
  dispatch(setLoading(true));
  dispatch(setError(null));
  try {
    await refreshCoordinatorRef.current.request(
      () => clientRef.current!.listSessions({ category: listOptions?.category, status: listOptions?.status, includeArchived: listOptions?.includeArchived }),
      (response) => {
        dispatch(setSessions(response.sessions));
        dispatch(setError(null));
        if (response.systemMemoryPct > 0) setSystemMemoryPct(response.systemMemoryPct);
      }
    );
  } catch (err) {
    dispatch(setError(err instanceof Error ? err.message : "Failed to list sessions"));
  } finally {
    dispatch(setLoading(false));
  }
}, [dispatch]);
```

`setLoading`/`setError` bracketing stays **outside** the coordinator (it's a per-invocation UX affordance, not a correctness concern — see §4's note on why gating it would misbehave under coalescing). Only the fetch-and-entity-dispatch pair goes through `request()`.

## 4. Consistency requirement: discard stale response, or dispatch anyway?

**Discard outright — no dispatch — for a superseded response.** `sessionsSlice.ts`'s existing staleness handling (`deletedIds` tombstone filter at `:39`, no-op-upsert skip at `:51`) solves a **different, per-entity** problem (a delete or a no-change update racing a slower snapshot) and does not help here: `setSessions` (`:38-41`) does an unconditional `sessionsAdapter.setAll`, i.e. a **full replace** — there is no per-record merge to fall back on for a whole-snapshot dispatch. If a stale response were dispatched anyway, `setAll` would blow away every session added/updated by a *newer* response or a concurrent `WatchSessions` `upsertSession` event that arrived in between, with no tombstone or timestamp check protecting it. This is exactly the bug described in requirements.md's Problem Statement — dispatching a stale `setSessions` unconditionally is the bug, not a mitigation for it.

**Integration contract**:
- `RefreshCoordinator<T>.request(fetcher, onResult)` increments an internal generation counter each time a fetch actually *starts* (i.e., when a request is picked up from idle or from the coalesced-pending slot — not on every `request()` call, since most calls during a burst never start their own fetch at all).
- After `fetcher()` resolves, compare the fetch's captured generation against the coordinator's current generation. Equal → call `onResult(result)` (which dispatches). Not equal → return without calling `onResult` at all. No stale dispatch, ever.
- Given the "≤1 in-flight fetch, ≤1 coalesced pending re-run" design (§5), a generation mismatch at resolution time is actually now structurally impossible in the common case — the *next* fetch can only start after the current one's `finally` clears in-flight state, strictly after this resolution's generation check already ran. The check is retained anyway as a documented invariant (cheap, makes the "never dispatch stale" contract explicit and independently testable — see the regression test requirements.md §Success Metrics calls for) rather than dead code, and it's what protects a future refactor that relaxes strict one-in-flight-ness from silently reintroducing the race.

**Filter-mismatch caveat (observation, not a fix required here)**: because `setSessions` is a full-replace shared by call sites with genuinely different filters (`PaneSplitRenderer.tsx:191`'s `{ includeArchived }` vs. the stream's `watchOptionsRef`-derived filters), the coordinator's "latest fetcher wins, earlier ones are silently dropped" coalescing means a caller requesting an archived-inclusive list could have its request dropped in favor of an unrelated unfiltered watch-triggered resync, and vice versa. This is **not a new problem introduced by the coordinator** — today, with no coordination at all, the exact same full-replace-regardless-of-filter hazard already exists (whichever of the two concurrent responses happens to resolve last wins, filter-blind). The coordinator does not make it worse and arguably narrows the window (only 1 network round-trip in flight at a time instead of N), but it doesn't fix the filter-correctness issue either. Fixing that is `sessionsSlice.ts` merge/dedup-logic territory, explicitly out of scope per requirements.md.

## 5. Is herdr-web's full "barrier generation" (mutation-fencing) needed?

**No — definitively out of scope, confirmed by tracing the actual dispatch call sites.** herdr-web's barrier-generation mechanism exists to discard a snapshot fetched *before* a mutation was applied, so a slow in-flight `list` fetch that started pre-mutation doesn't overwrite the mutation's optimistic write when it finally resolves. Grepping this codebase's mutation paths confirms the precondition for needing that mechanism doesn't hold here:

- `createSession`, `updateSession`, and session-delete flows dispatch `upsertSession`/`removeSession` directly on RPC success (e.g. `handleSessionEvent`'s `sessionCreated`/`sessionUpdated` cases at `useSessionService.ts:760-772` route through `upsertSession`, which is a per-record merge, not a full replace) — they do **not** wait for or trigger a follow-up `listSessions()` snapshot refresh to reflect the mutation. There is no code path where a mutation's effect is only visible via the next snapshot fetch.
- Because mutations use `upsertSession` (merge) and snapshot refreshes use `setSessions` (replace), the two write paths are already structurally separated: an in-flight, pre-mutation snapshot response that resolves *after* a mutation's `upsertSession` would, at worst, briefly reintroduce a stale value for that one session's fields until the next snapshot or stream event corrects it — the same class of staleness the no-op-upsert skip (`sessionsSlice.ts:51`) and tombstone filter (`:39`) already partially guard against for the delete case. This is a narrower, pre-existing risk (full-replace-vs-single-mutation ordering) that a barrier-generation mechanism keyed to "the last mutation's generation" would only partially close anyway, since `setSessions` has no per-field merge to apply a barrier *to* — it would need to become a merge operation first, which is explicitly out-of-scope `sessionsSlice.ts` work.
- Building barrier-generation machinery to solve a race that requirements.md's own Rabbit Holes section flags as **not concretely observed** would be scope creep against the stated "Small (1-3 days), single-file utility" appetite.

**Recommendation**: ship the simpler "one in flight + latest-pending-rerun + self-generation stale-discard" coordinator (§3-4) with no mutation-awareness at all. If a concrete mutation-vs-snapshot race is ever found in practice, it should be scoped as its own follow-up against `sessionsSlice.ts`'s merge logic, not folded into this utility.

## 6. Coordinator shape (illustrative sketch, not final code)

```ts
// web-app/src/lib/utils/refreshCoordinator.ts
export class RefreshCoordinator<T> {
  private inFlight = false;
  private generation = 0;
  private pending: { fetcher: () => Promise<T>; waiters: Array<() => void> } | null = null;

  /** Coalesces concurrent calls into "≤1 in flight + ≤1 pending rerun"; discards a
   *  response's onResult if a newer fetch has since started. Resolves once the
   *  fetch this call ultimately contributed to (direct or coalesced) settles. */
  request(fetcher: () => Promise<T>, onResult: (result: T) => void): Promise<void> {
    if (this.inFlight) {
      if (!this.pending) this.pending = { fetcher, waiters: [] };
      else this.pending.fetcher = fetcher; // latest wins; earlier queued fetcher dropped
      return new Promise((resolve) => this.pending!.waiters.push(resolve));
    }
    return this.run(fetcher, onResult);
  }

  private async run(fetcher: () => Promise<T>, onResult: (result: T) => void): Promise<void> {
    this.inFlight = true;
    const myGeneration = ++this.generation;
    try {
      const result = await fetcher();
      if (myGeneration === this.generation) onResult(result);
    } finally {
      this.inFlight = false;
      const next = this.pending;
      this.pending = null;
      if (next) {
        const rerun = this.run(next.fetcher, onResult);
        void rerun.then(() => next.waiters.forEach((w) => w()));
      }
    }
  }
}
```

`T` = `Session[]` (or the full `ListSessionsResponse` if `systemMemoryPct` handling stays inside `onResult`, as shown in §3) when wired into `useSessionService.ts`. Unit tests (per requirements.md §Success Metrics) should cover: single request resolves normally; a burst of N calls while one is in flight collapses to exactly 2 fetcher invocations total (the original + one coalesced rerun); a superseded fetch's `onResult` is never called; all callers coalesced into one rerun have their returned promise resolve once that rerun settles.

## Summary of recommendations

1. New file: `web-app/src/lib/utils/refreshCoordinator.ts` (+ `refreshCoordinator.test.ts`), styled after `backoff.ts`.
2. Instantiate via `useRef(new RefreshCoordinator<...>())` inside `useSessionService.ts`, matching `backoffRef` at line 176 — no module singleton.
3. Route only the RPC call + `dispatch(setSessions(...))` (+ `setSystemMemoryPct` where applicable) through `request()`'s `onResult`; keep `setLoading`/`setError` bracketing outside it.
4. Stale responses are discarded outright (no dispatch) — `sessionsSlice.ts`'s per-entity staleness handling doesn't cover whole-snapshot `setAll` and isn't a substitute.
5. No barrier-generation/mutation-fencing — confirmed unnecessary because mutations write via `upsertSession` (merge), never via a follow-up snapshot fetch.
