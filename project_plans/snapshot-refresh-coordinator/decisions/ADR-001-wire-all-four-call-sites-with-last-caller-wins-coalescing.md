# ADR-001: Wire `RefreshCoordinator` into all 4 call sites with last-caller-wins coalescing

**Status**: Accepted
**Date**: 2026-08-06
**Related**: `project_plans/snapshot-refresh-coordinator/requirements.md`, `research/architecture.md` §1-4, `research/pitfalls.md` §1a, `research/features.md` Finding 2

## Context

`useSessionService.ts` has 4 call sites that trigger a `ListSessions` RPC:

1. The public `listSessions(listOptions?)` (`useSessionService.ts:212-240`) — filters come
   from the caller's arbitrary `{category, status, includeArchived}` args.
2. The watch-stream's initial snapshot (`:838-845`) — filters come from
   `watchOptionsRef.current` (`{categoryFilter, statusFilter}`).
3. The backwards-jump full resync, success path (`:874-884`) — same filters as #2.
3b. The backwards-jump full resync, error path (`:918-929`) — same filters as #2, duplicate
   of #3 in the catch block.

Sites #2, #3, and #3b are filter-homogeneous (all read `watchOptionsRef.current`). Site #1 is
not — it's driven by whatever a caller (e.g. `page.tsx`'s `"R"` shortcut, or
`PaneSplitRenderer.tsx`'s `{ includeArchived }` call) passes in, which can differ from the
stream's active filter.

A coordinator that blindly coalesces "any concurrent `request()` call into one shared
fetch/response" risks a caller receiving (or, symmetrically, being denied) data scoped to a
filter it never asked for, because `setSessions` (`sessionsSlice.ts:38-41`) is an unconditional
full replace with no per-caller filter tagging.

## Decision

Wire the coordinator into **all 4 call sites** (Approach A), with `request(fetcher, onResult)`
taking the fetcher/handler fresh on every call (never bound at construction) and
**last-caller-wins** semantics for the coalesced pending rerun: if multiple `request()` calls
arrive while a fetch is in flight, only the *most recently* queued caller's `fetcher`/`onResult`
actually runs when the pending rerun executes. Every caller — including ones whose own
fetcher never ran — still gets its own `request()` promise settled (resolved or rejected) once
that single rerun completes, so `setLoading`/`setError` bracketing at site #1 never sticks.

## Alternatives considered

**B — Scope the coordinator to only the 3 filter-homogeneous stream-internal sites (#2, #3,
#3b), leave site #1 uncoordinated.** Filter-homogeneity would be guaranteed by construction:
zero risk of one caller's filtered response overwriting another's differently-filtered intent,
because the excluded site's arbitrary filters never enter the coordinator. Rejected because
`requirements.md`'s Scope section explicitly names all 4 call sites as in scope ("Wire the
coordinator into `useSessionService.ts`'s internal `listSessions`-triggering call sites (the
four listed in Problem Statement)"), and because it leaves unguarded the exact race the
requirements' own Problem Statement opens with — a caller-invoked `listSessions()` racing the
stream's own internal `listSessions()` calls.

**C — Filter-signature-keyed coordinator map** (`Map<string, RefreshCoordinator<T>>`,
partitioned by `JSON.stringify(filters)`, so two calls with different filters run
independently and only same-filter bursts coalesce). This is the only option that fully closes
the race for all 4 sites without ever substituting one filter's result for another's. Rejected
because `research/ux.md` confirms no live UI burst pattern today actually mixes filters (no
dropdown fires a `listSessions` call per interaction; the only filtered calls are the 3
internal, already-homogeneous stream sites) — so it solves a currently-theoretical problem at
real, ongoing cost: a growing/pruned map, its own cache-key hygiene, and loss of the simpler
"≤1 RPC in flight, period" invariant the rest of this plan's reasoning (e.g. the generation
check being structurally redundant in steady state, per `research/architecture.md` §4) relies
on. It also blows past `requirements.md`'s "Small (1-3 days), single-file utility" appetite.

## Consequences

- **Accepted tradeoff, not a regression**: a caller whose request gets coalesced away does not
  get its own filtered data applied — it silently rides whatever the latest queued caller's
  filter produced (or fails if that fetch fails). `research/architecture.md` §4 confirms this
  does not make anything *worse* than today: with zero coordination, whichever of two
  concurrent unary responses happens to resolve last already wins, filter-blind, at the
  `setSessions` full-replace level. The coordinator's only change to this pre-existing
  behavior is narrowing the window (≤1 network round-trip in flight at a time instead of N)
  and making the "who wins" rule deterministic (last *queued*, not last *resolved*) rather
  than a race on network timing.
- This tradeoff is made observable, not silent: `project_plans/snapshot-refresh-coordinator/implementation/plan.md` Story 2.1.5 adds a dedicated regression test
  (`listSessions_should_loseItsOwnFilteredOnResult_When_coalescedBehindADifferentlyFilteredLaterCaller`)
  proving and documenting the exact behavior, with a comment linking back to this ADR.
- If a real, observed bug ever traces back to this tradeoff (e.g. a UI report of "my filtered
  view briefly showed the wrong sessions"), the fix is a scoped follow-up adopting Alternative
  C (filter-signature keying) — not a redesign of the coordinator's core coalescing shape,
  since C's difference from the chosen design is additive (a map of coordinators, not a
  different coalescing algorithm).
- Fixing the underlying `setSessions` full-replace-regardless-of-filter hazard at the
  `sessionsSlice.ts` level (e.g. giving `setSessions` a filter-scoped merge instead of a blind
  `setAll`) remains explicitly out of scope for this item per `requirements.md`'s Out of Scope
  section.

## Amendment (2026-08-24): resolving adversarial-review's two BLOCKERs

`implementation/adversarial-review.md` returned **BLOCKED** on two gaps this ADR's original
"Consequences" section did not cover. Both are resolved with code-level fixes, not documented
risk, per `implementation/pre-mortem.md` P1 item #3's explicit requirement that a BLOCKER may
not be closed the same way the (already-accepted) filter-heterogeneity Concern was:

- **Blocker 1 (hung/slow fetch stalls the whole coordinator)**: every fetcher closure at all 4
  call sites now passes ConnectRPC's native `{ timeoutMs: LIST_SESSIONS_TIMEOUT_MS }` (15s;
  `useSessionService.ts`), the same mechanism already used for `createSession`'s
  `CREATE_SESSION_TIMEOUT_MS`. A fetch that hangs past this bound rejects, which the
  coordinator's existing settle/drain logic already unblocks queued callers from — proven at
  `refreshCoordinator.test.ts`'s `request_should_unblockQueuedCallers_When_aHungFetcherEventuallyTimesOut`.
  No coordinator-level `AbortSignal` plumbing was needed.
- **Blocker 2 (a queued guarded stream-flush fetcher can be silently discarded)**: `request()`
  gained a `{ guarded?: boolean }` option (`refreshCoordinator.ts`). Sites #2/#3/#3b (the
  stream-reconnect flush, gated on `streamGenerationRef`) pass `guarded: true`; site #1 (the
  public `listSessions()`) does not. A non-guarded caller arriving while a guarded fetcher sits
  in the coordinator's `pending` slot no longer overwrites it — it rides the guarded fetch's
  own outcome instead (still an instance of this ADR's existing accepted "loses its own
  onResult" tradeoff, just with guarded fetchers given priority over unguarded ones). Proven at
  the coordinator level
  (`request_should_neverOverwriteAGuardedPendingFetcher_When_ALaterNonGuardedCallerCoalesces`)
  and at the real call-site/Redux-store level
  (`useSessionService.test.ts`'s
  `listSessions_should_neverOverwriteAQueuedGuardedStreamSnapshot_When_ALaterUnguardedListSessionsCoalesces`).

`implementation/pre-mortem.md` P1 item #1 (a thrown `onResult` wedging the coordinator at
`inFlight` forever) is also closed with a code-level fix: `run()`'s drain step is in a
`finally` block, so it always executes regardless of whether `fetcher()` rejects or `onResult`
throws — proven at `request_should_returnToIdleState_When_onResultThrowsSynchronously`.
