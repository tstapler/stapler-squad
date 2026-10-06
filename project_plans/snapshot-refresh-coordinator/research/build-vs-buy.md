# Research: Build vs. Buy — snapshot-refresh-coordinator

**Date**: 2026-08-06
**Question**: Hand-roll the `createRefreshCoordinator`/`createSnapshotRefreshController` utility (per requirements.md), or adopt an existing library primitive?

## 1. Existing OSS library already in the dependency tree

**Checked**: `web-app/package.json` (`Read`), `grep -rn "createAsyncThunk" web-app/src` (0 hits), `grep -rn "RTK Query\|@reduxjs/toolkit/query\|tanstack" web-app/package.json web-app/src`.

### `createAsyncThunk`'s `condition` option
Not used anywhere in this codebase (zero matches). `condition()` can *skip* dispatching a new thunk while one is in flight, but it does not provide "coalesce into at-most-one pending re-run" — a call arriving during an in-flight request is simply dropped, not queued to re-run once the in-flight one resolves. It also doesn't give stale-response discard against a concurrently-arriving `WatchSessions` push event without extra hand-rolled generation tracking layered on top — at which point you've rebuilt the coordinator anyway, just wrapped in thunk machinery. Adopting it here would introduce a wholly new async-flow pattern to the codebase (no precedent) for one call path.
**Verdict: Not recommended.**

### RTK Query
**Already a dependency and already in use** — `web-app/src/lib/api/connectApi.ts` defines a `connectApi` RTK Query instance (`createApi` + a `connectBaseQuery` adapter for ConnectRPC unary calls), consumed today by `ApprovalsContext.tsx` for 5s-polling approvals data. RTK Query does have real built-in request dedup (same serialized args share one in-flight promise) and correctly ignores stale results for a given cache entry.

However, fitting it here means migrating `listSessions()` off the current imperative `useSessionService` hook pattern into an RTK-Query-owned cache slice — a materially bigger change than the "single-file utility + wiring" scoped by requirements.md's Appetite (Small, 1-3 days). Two structural frictions:
- `WatchSessions` is a push stream, not a request/response query — it isn't an RTK Query fit, so you'd end up with `listSessions` results living in RTK Query's own cache while `WatchSessions` events keep writing directly into `sessionsSlice`'s entity adapter — two sources of truth for the same session list, needing manual reconciliation (`onQueryStarted`/`upsertQueryData`) either way.
- Requirements explicitly say not to regress `sessionsSlice.ts`'s `deletedIds` tombstone filtering or the no-op-upsert skip (lines 39/51) — those live in the existing slice; RTK Query's normal pattern owns its own cache, so preserving that logic means bypassing RTK Query's idiomatic usage.

**Verdict: Viable in principle (dedup is real, and the dependency is already paid for), but not recommended for *this* item.** It's the right primitive for a larger future consolidation of session-list state under RTK Query, not for a small, scoped ordering-guard fix that must stay compatible with the existing entity-adapter slice and the streaming path. Worth flagging as a separate, larger follow-up if the team wants to unify session-list state management — out of scope here.

### TanStack Query
Not a dependency (`grep` confirmed zero references anywhere in `web-app/`). RTK Query already fills the same "server-state cache" niche and is already adopted for Approvals — pulling in a second, overlapping library for one hook would be duplication with no offsetting benefit.
**Verdict: Not recommended.**

## 2. SaaS/managed service

Not applicable — this is a purely client-side in-memory coordination primitive (sequencing two same-process async calls against a Redux store), with no external state, persistence, or cross-client concern. Confirmed and skipped.

## 3. Bespoke (hand-rolled) vs. a small focused npm package

Searched npm for "latest-wins" / "stale-while-revalidate" / single-flight request-coalescing primitives. Closest hits: `stale-while-revalidate-cache`, `stale-while-revalidate-lru-cache`, `reine` — all are **key-based memoizing caches** for function results (deduplicate concurrent calls to the *same cache key*, then serve stale-while-fetching-fresh). That's a different problem shape: this item needs "one in flight + at-most-one queued re-run + discard a resolved response if a newer request has since started," applied to a single mutable snapshot (the session list), not a keyed cache. Adapting a caching library to this shape would need at least as much glue code as the ~50-line hand-rolled version, plus a new dependency and its own API surface to learn.

`web-app/package.json`'s dependency list is deliberately scoped — UI/infra libraries (Radix, CodeMirror language packs, `@connectrpc/*`, xterm addons) and no general-purpose utility-belt packages (no lodash, ramda, `p-limit`/`p-debounce`, etc.). That's a signal the codebase prefers small hand-rolled utilities over utility-package dependencies for exactly this kind of logic.

**Verdict: Recommended.** Hand-roll a pure, framework-agnostic coordinator (as requirements.md already scopes), unit-tested per the Success Metrics in requirements.md. This is a well-understood ~50-line pattern (see below), matches existing repo conventions, and keeps the change inside the stated "Small (1-3 days)" appetite.

## 4. Fork/adapt herdr-web's `refreshCoordinator.ts`

**Confirmed via GitHub API** (`gh api search/code`, `gh api repos/kcosr/herdr-web/contents/...`): the reference file is real, at `kcosr/herdr-web:web/src/refreshCoordinator.ts` (53 lines), exporting `createSnapshotRefreshController`. Full source read and captured below for reference.

**Confirmed NOT published to npm**: `kcosr/herdr-web`'s `web/package.json` has `"private": true`; `npm view herdr-web` and `npm view refresh-coordinator` both 404 against the public registry. There is no fork/install target — the repo (`herdr.dev`'s terminal-session-manager web client) is unrelated in scope to stapler-squad and forking it for one function would be nonsensical. Repo is MIT-licensed (`gh api repos/kcosr/herdr-web --jq '.license'` → `mit`), so literal reuse would be legally fine, but the shape still needs adapting: herdr-web's version includes an `isCurrent`/`getBarrierGeneration` mutation-fencing mechanism that requirements.md's own Rabbit Holes section flags as likely unnecessary here (stapler-squad's mutations already optimistically `dispatch(upsertSession(...))` rather than waiting on a follow-up list refresh).

**Verdict: Adapt the pattern, not the code** — confirms the backlog item's own framing. Use `createSnapshotRefreshController`'s in-flight/pending-rerun/generation-discard shape as the model, but implement fresh with only as much generation-fencing as stapler-squad actually needs (confirm during planning whether the simpler no-mutation-barrier version suffices, per the Rabbit Holes note).

### Reference source (herdr-web, for the plan phase to adapt from)

```ts
export type SnapshotRefreshControllerOptions<TSnapshot> = {
  fetchSnapshot: () => Promise<TSnapshot>;
  applySnapshot: (snapshot: TSnapshot, refreshGeneration: number) => void;
  onError: () => void;
  isCurrent: () => boolean;
  getGeneration: () => number;
  getBarrierGeneration: () => number;
};

export function createSnapshotRefreshController<TSnapshot>({
  fetchSnapshot, applySnapshot, onError, isCurrent, getGeneration, getBarrierGeneration,
}: SnapshotRefreshControllerOptions<TSnapshot>) {
  let refreshInFlight = false;
  let refreshPending = false;

  const runRefresh = () => {
    const refreshGeneration = getGeneration();
    refreshInFlight = true;
    void fetchSnapshot()
      .then((next) => {
        if (!isCurrent()) return;
        if (getBarrierGeneration() > refreshGeneration) {
          refreshPending = true;
          return;
        }
        applySnapshot(next, refreshGeneration);
      })
      .catch(() => { if (isCurrent()) onError(); })
      .finally(() => {
        refreshInFlight = false;
        if (isCurrent() && refreshPending) {
          refreshPending = false;
          runRefresh();
        }
      });
  };

  return {
    request() {
      if (!isCurrent()) return;
      if (refreshInFlight) { refreshPending = true; return; }
      runRefresh();
    },
  };
}
```

Source: [`kcosr/herdr-web:web/src/refreshCoordinator.ts`](https://github.com/kcosr/herdr-web/blob/8803aba825efae88d92efac0ce16177a0fd973fe/web/src/refreshCoordinator.ts) (commit `8803aba8`).

## Summary Table

| Option | Verdict | Reason |
|---|---|---|
| `createAsyncThunk` + `condition` | Not recommended | Skips rather than coalesces; no stale-discard; unprecedented pattern in this codebase |
| RTK Query (already a dependency) | Viable, not recommended here | Dedup is real, but fitting it means a cache-ownership split with the `WatchSessions` push path and the existing `sessionsSlice` entity adapter — bigger than the scoped appetite |
| TanStack Query | Not recommended | Not a dependency; duplicates RTK Query, already adopted |
| SaaS/managed | N/A | Client-side coordination primitive, no external state |
| npm single-flight/SWR package | Not recommended | Available packages are keyed-cache memoizers, wrong problem shape; codebase avoids utility-belt deps |
| Hand-rolled, unit-tested | **Recommended** | ~50 lines, matches codebase dependency conventions, fits Small appetite |
| Fork herdr-web | Not viable | Not published to npm (`private: true`, 404 on registry); wrong-scoped repo to fork |
| Adapt herdr-web's pattern | **Recommended** | MIT-licensed reference shape confirmed via source read; adapt with only the generation-fencing this codebase actually needs |
