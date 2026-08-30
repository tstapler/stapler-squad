# Implementation Plan: snapshot-refresh-coordinator

**Feature**: A pure `RefreshCoordinator<T>` utility (`web-app/src/lib/utils/refreshCoordinator.ts`), wired via `useRef` into the 4 `listSessions`-triggering call sites in `web-app/src/lib/hooks/useSessionService.ts`, so at most one `ListSessions` RPC is in flight at a time and a response that's been superseded by a newer request is discarded rather than dispatched.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001-wire-all-four-call-sites-with-last-caller-wins-coalescing.md

---

## System type

Small client-side concurrency-control utility (one new pure TS class, ~60-80 lines) plus
wiring into 4 existing call sites in one hook file. Not a new subsystem, not a cache layer,
not an `createAsyncThunk`/RTK Query migration (`research/build-vs-buy.md` explicitly rules
those out for this appetite). The closest structural precedent already in the codebase is
`BackoffState` (`web-app/src/lib/utils/backoff.ts`) — same shape: framework-free class,
instantiated once per hook instance via `useRef`, fully unit-testable in isolation.

## Step 0.5 — Alternatives considered

| # | Approach | Strength | Weakness |
|---|---|---|---|
| A | **Single coordinator wired into all 4 call sites; `request(fetcher, onResult)` takes the fetcher/handler per call, last-caller-wins for the coalesced pending rerun** (chosen) | Matches `research/requirements.md`'s explicit scope ("wire the coordinator into ... the four listed call sites"); `research/architecture.md` §4 confirms this does not make the pre-existing filter-blind `setSessions` full-replace hazard *worse* than today (whichever concurrent response resolves last already wins, filter-blind, with zero coordination) | A caller whose request gets coalesced away never gets its own filtered data applied — it silently rides whatever the latest queued caller's filter produced. Must be documented as an accepted, pre-existing tradeoff, not hidden. |
| B | Scope the coordinator to only the 3 filter-homogeneous stream-internal call sites (initial snapshot, backwards-jump resync ×2, backstop reconnect — all share `watchOptionsRef.current`); leave the public `listSessions()` (site #1) uncoordinated, exactly as today | Filter-homogeneity guaranteed by construction — zero risk of one caller's filtered response overwriting another's differently-filtered intent, because the excluded call site's arbitrary filters never enter the coordinator at all | Directly contradicts `requirements.md`'s Scope section, which names all 4 call sites as in-scope; leaves the exact race requirements.md's Problem Statement opens with (`listSessions()` racing the stream's own internal calls) fully unguarded for the one caller-invoked path |
| C | Filter-signature-keyed coordinator map (`Map<string, RefreshCoordinator<T>>`, partitioned by `JSON.stringify(filters)`) wired into all 4 sites | Only option that closes the race for all 4 sites without ever substituting one filter's result for another's | Real complexity for a scenario `research/ux.md` confirms is not a live UI burst pattern today (no dropdown fires a `listSessions` call per interaction); needs its own map-entry lifecycle/pruning, its own bug surface, and blows past the "Small (1-3 days), single-file utility" appetite in `requirements.md` |

**Chosen: A.** Recorded in the Pattern Decisions table below (row "Call-site scope") with B
and C as rejected alternatives. See `ADR-001` for the full write-up — this is the plan's
central, most consequential decision per the assigned task brief.

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `RefreshCoordinator<T>` | New class, `web-app/src/lib/utils/refreshCoordinator.ts`. Coalesces concurrent `request()` calls into "≤1 in-flight fetch + ≤1 coalesced pending rerun," discards a fetch's result if superseded before it resolves, and settles every caller's own returned `Promise<void>` (resolve or reject) exactly once — including callers folded into the pending slot. | Framework-free, no React/ConnectRPC imports, styled after `BackoffState` (`backoff.ts`). |
| `CoordinatorState<T>` | New discriminated-union type, internal to `RefreshCoordinator<T>`: `{ kind: "idle" } \| { kind: "inFlight"; generation: number } \| { kind: "inFlightWithPending"; generation: number; pending: PendingRequest<T> }`. | Replaces herdr-web's two independent `refreshInFlight`/`refreshPending` booleans (see Pattern Decisions) — makes "pending set while not in flight" unrepresentable. |
| `PendingRequest<T>` | New interface: `{ fetcher: () => Promise<T>; onResult: (result: T) => void; waiters: Array<{ resolve: () => void; reject: (err: unknown) => void }> }`. | Only exists inside the `inFlightWithPending` state variant. `fetcher`/`onResult` are overwritten (not appended) by each new coalesced `request()` call — last-caller-wins. |
| `generation` | Monotonically-increasing counter, incremented once per **fetch that actually starts** (i.e. when `run()` is entered — from idle, or from draining `pending` — never on a `request()` call that only updates `pending`). | Mirrors `streamGenerationRef`'s "one increment per real attempt" idiom (`useSessionService.ts:829,833`), not "one increment per every call that merely asked." See Pattern Decisions for why this, not per-call incrementing, was chosen. |
| `fetcher` | Type alias `() => Promise<T>`. The RPC call itself, supplied fresh by the caller on every `request()` invocation — never bound at `RefreshCoordinator` construction time. | Closes over the caller's *current* filter args (`listOptions`, or `watchOptionsRef.current`) read at call time — no stale-closure risk (`research/pitfalls.md` §2). |
| `onResult` | Type alias `(result: T) => void`. The fetch-and-dispatch side effect (e.g. `dispatch(setSessions(response.sessions))`), invoked by the coordinator itself only if the fetch's `generation` is still current when it resolves. | Never called by the caller directly after `await request()` — see "Fetch+dispatch atomicity" Pattern Decision. |
| `request(fetcher, onResult)` | `RefreshCoordinator<T>`'s sole public method. Returns `Promise<void>` that settles once the fetch this call ultimately contributed to (direct or coalesced) settles — resolves on success, rejects with the fetch's error on failure, for **every** caller, not just the one whose fetcher ran. | The per-caller settle guarantee (including for a caller whose request got coalesced away) is what prevents `listSessions()`'s `setLoading(true)`/`setLoading(false)` bracket from sticking open (`research/pitfalls.md` §4). |
| `coalesced caller` | A `request()` call that arrives while `state.kind !== "idle"`; its `fetcher`/`onResult` either become the new `pending` payload (overwriting any prior pending payload) or are dropped in favor of a still-later coalesced caller, but its returned promise is always pushed onto `pending.waiters` and settled once the eventual rerun completes. | — |
| `stale-response discard` | The behavior where a fetch's result is silently *not* passed to `onResult` because `generation` advanced (a new fetch started) between the fetch beginning and resolving. Under the ≤1-in-flight design this is structurally unreachable in steady state (§ Pattern Decisions) but is kept as a documented, independently-tested invariant. | `research/architecture.md` §4. |
| `refreshCoordinatorRef` | New `useRef(new RefreshCoordinator<ListSessionsResponse>())` in `useSessionService.ts`, instantiated once per hook instance (matches `backoffRef` at line 176) — **not** a module-level singleton. | See Pattern Decisions "Instantiation scope." |
| `ListSessionsResponse` | Existing generated type (`web-app/src/gen/session/v1/session_pb.ts:96-109`), fields `sessions: Session[]` and `systemMemoryPct: number`. | Chosen as `RefreshCoordinator`'s `T` — see Pattern Decisions "Generic parameter." Not a new type. |

---

## Pattern Decisions

Per `.claude/rules/interface-pollution-checklist.md`: this adds exactly one new exported
type (`RefreshCoordinator<T>`), no new interface layer, no Manager/Service/Handler wrapper —
`useSessionService.ts` calls the class's one public method directly. PoEAA is not applicable
here — no persistence/service layer, confirmed against `research/architecture.md` and
`research/build-vs-buy.md` (both explicitly rule out RTK Query / `createAsyncThunk` as the
wrong shape for this problem, not merely unnecessary).

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Call-site scope | Wire all 4 call sites (Approach A) | This plan, §0.5 | (B) scope to 3 stream-internal sites only; (C) filter-signature-keyed map | `requirements.md`'s Scope section explicitly lists all 4 sites; `research/architecture.md` §4 confirms A doesn't worsen the pre-existing filter-blind hazard. Full reasoning in `ADR-001`. |
| Concurrency control shape | Not a classic GoF pattern — closest is a **single-flight / promise-coalescing concurrency primitive** (no canonical GoF name; forcing Strategy/Observer/etc. onto it would be a mismatch) | `design-patterns` skill (GoF) | Observer (for the "waiters" list) | The waiters list is a fixed-size, one-shot completion signal (settles once, then discarded), not a subscribe/publish channel with multiple future events — Observer's repeated-notification semantics don't fit a single settle-once promise. |
| Internal state representation | Discriminated union `CoordinatorState<T>` (`idle \| inFlight \| inFlightWithPending`) | `type-driven-design` skill | Two independent booleans (`refreshInFlight`/`refreshPending`, herdr-web's own shape, and this task brief's own callout) | herdr-web's `refreshInFlight=false, refreshPending=true` is a real illegal state (pending only ever means anything while a fetch is running) that the booleans don't prevent at the type level; the union makes it unrepresentable — `pending` literally doesn't exist as a field unless `kind === "inFlightWithPending"`. |
| Generation increment point | Increment once per **fetch start** (`run()` entry) | `research/architecture.md` §4 | Increment once per every `request()` call (`research/pitfalls.md` §"Summary" bullet 2) | The two research docs disagree; resolved here. Under the ≤1-in-flight/≤1-pending design, a `request()` call that only updates `pending` never itself starts a fetch, so incrementing generation for it protects nothing — the next real fetch's generation check can only ever race against fetches that have already fully drained. Per-fetch-start incrementing keeps `generation` numbers 1:1 with actual RPC attempts, matching `streamGenerationRef`'s own "one increment per real attempt" spirit (`useSessionService.ts:829,833`) rather than "one increment per consideration of starting one." |
| Coalesced-caller failure handling | `run()`'s rerun promise chain uses `.then(onFulfilled, onRejected)` (both arms), rejecting every `pending.waiters` entry when the coalesced fetch fails | This plan (fixes a bug in `research/architecture.md` §6's own "illustrative, not final" sketch) | `research/architecture.md` §6's sketch, which chains `rerun.then(() => waiters.forEach(resolve))` with no rejection arm | The sketch's single-arm `.then()` means a coalesced caller's `request()` promise never settles at all if the rerun's `fetcher()` throws — reproducing exactly the stuck-`loading` bug `research/pitfalls.md` §4 warns about, just on the error path instead of the success path. Task 1.3 adds the regression test this fix requires. |
| `RefreshCoordinator` instantiation scope | Per-hook-instance via `useRef`, matching `backoffRef` | `research/architecture.md` §2 | Module-level singleton | Every other piece of mutable state in `useSessionService.ts` (`streamGenerationRef`, `backoffRef`, `isConnectedRef`, etc.) is per-hook-instance; a singleton would be the only exception, would leak in-flight/pending state across `renderHook`-based test cases, and has no cross-instance coordination need (`GlobalSessionServiceProvider` mounts the hook exactly once app-wide). |
| Generic parameter `T` | `RefreshCoordinator<ListSessionsResponse>` (one shared instance/type across all 4 sites) | This plan | `RefreshCoordinator<Session[]>` with `systemMemoryPct` handled outside the coordinator | Site #1 (`listSessions()`) needs `response.systemMemoryPct` inside its own `onResult` closure (`setSystemMemoryPct(...)`, existing behavior at `useSessionService.ts:228-230`) — passing the full `ListSessionsResponse` through keeps that logic inside `onResult` without a second parallel field on the coordinator. |
| `setLoading`/`setError` ownership | Stay outside the coordinator, bracketing site #1's own `request()` call in its existing `try/catch/finally` (unchanged shape) | `research/architecture.md` §3 | Move `setLoading`/`setError` inside the coordinator as a "single writer" keyed off `inFlight` transitions (`research/pitfalls.md` §4's alternative suggestion) | Once `request()` correctly settles (resolve or reject) for every caller including coalesced ones (see "Coalesced-caller failure handling" row above), site #1's own `finally { dispatch(setLoading(false)) }` always runs — the stuck-loading risk is closed by fixing the settle-signal bug, not by relocating ownership. Moving it inside the coordinator would also require the coordinator to know about Redux, breaking its framework-free design goal. |
| Mutation-fencing (barrier generation) | None | `research/architecture.md` §5 | herdr-web's `isCurrent`/`getBarrierGeneration` mechanism | Confirmed unnecessary: this codebase's mutations (`createSession`/`updateSession`/delete) dispatch `upsertSession`/`removeSession` directly on RPC success, never waiting on a follow-up snapshot fetch — the precondition for needing barrier-fencing doesn't hold. Building it would be scope creep against the requirements' "Small" appetite and its own Rabbit Holes note. |

---

## Migration Plan

Omitted — no schema, database, or proto changes (`requirements.md` Constraints: "frontend
only, no backend or proto changes").

## Observability Plan

- **No new metrics/alerts.** `research/ux.md` confirms this change has zero user-facing
  surface and is not tied to any existing SLO; the existing `console.error("Failed to list
  sessions:", error)` at `useSessionService.ts:234` already covers the one failure path with
  user impact (site #1's own error dispatch).
- **No new structured logs required for the success/coalescing path** — coalescing is an
  internal correctness improvement, not an event operators need visibility into per
  `research/ux.md`'s finding that this reduces (never increases) network call volume during
  bursts. Not adding a `console.debug` on every coalesce avoids introducing log-volume noise
  for a non-actionable event.
- Existing error dispatch paths (`useSessionService.ts:233`, `:913`) are unchanged by this
  work — a fetch's failure still surfaces via the same `dispatch(setError(...))` call it does
  today, just gated by the coordinator's generation check like every other `onResult`.

## Risk Control

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Coalesced-caller promise never settles on a rejected rerun (bug reproduced from the reference sketch) | Was High pre-mitigation | High (reintroduces the exact stuck-`loading` bug this item exists to close, on the error path) | Pattern Decisions "Coalesced-caller failure handling" row: both-arm `.then(resolve, reject)`; Task 1.3 adds a dedicated regression test (`request_should_rejectAllCoalescedWaiters_When_theCoalescedFetchRejects`) |
| A caller's own filter/response gets silently discarded because a differently-filtered caller's request coalesced ahead of it | Medium (accepted tradeoff, not new — see ADR-001) | Low-Medium (matches pre-existing behavior; not a regression) | Explicitly documented in ADR-001 and the Domain Glossary (`coalesced caller`); Story 2.5 adds a targeted unit test proving this is a *documented*, not silent, tradeoff |
| Feature registry / test discipline skipped because "it's just an internal refactor" | Low-Medium | Low (process debt) | Story 3.2 explicitly budgets the `useSessionService.test.ts` integration test the requirements' Success Metric 3 calls for; no feature-registry entry needed (no new RPC, no new UI — confirmed against `.claude/rules/feature-registry.md`'s scope: backend RPC / frontend UI additions, neither of which this is) |
| A future edit to `useSessionService.ts` accidentally awaits `request()` and then dispatches on its own (reintroducing the exact race this closes) | Low | High if it happens | Task 2.1-2.4 each wire `onResult` as the *only* dispatch path for their site; the `request()` JSDoc (Task 1.1) states explicitly "never dispatch the return value of `await request()` — it resolves to `void`" as a type-level guard (return type is literally `Promise<void>`, so a caller physically cannot extract a result to dispatch from the await) |
| Regression of the existing `streamGenerationRef` guard on the stream reconnect loop | Low | High (would reopen a already-closed race) | Tasks 2.2-2.4 wrap only the `listSessions()` RPC call + its immediate `dispatch(setSessions(...))` inside `request()`/`onResult`; every existing `streamGenerationRef.current !== myGeneration` check (`useSessionService.ts:844,869,891,914,925,936`) stays exactly where it is, unmodified — verified by a diff review in Story 2.6 |

## Unresolved Questions

None blocking. Two explicit non-blocking follow-ups noted per `research/features.md` Finding
4 (not folded into this item's scope, per its own recommendation):
- Migrate `web-app/src/lib/hooks/useStuckBacklogItems.ts:69-87` and
  `web-app/src/lib/hooks/useSessionSummary.ts:94` from their current "skip if in flight, drop
  the tick" pattern onto `RefreshCoordinator<T>` once it exists — separate backlog item, not
  this one.
- If a concrete mutation-vs-stale-snapshot race is ever observed in practice (contradicting
  the "mutations always write via `upsertSession`, never a follow-up snapshot fetch" premise
  this plan relies on), barrier-generation fencing would need its own scoped follow-up against
  `sessionsSlice.ts` — not a gap in this plan, just out of its bounds per `requirements.md`'s
  Rabbit Holes section.

## Dependency Visualization

```
┌──────────────────────────────────────────────────────────┐
│ Epic 1: RefreshCoordinator<T> utility (pure, no React)     │
│  Story 1.1 — core coalescing (idle/inFlight/pending states)│
│  Story 1.2 — stale-response discard (generation check)     │
│  Story 1.3 — per-caller settle incl. coalesced-error fix    │
└───────────────────────────┬──────────────────────────────┘
                             │ (Epic 1 must exist before any wiring)
                             ▼
┌──────────────────────────────────────────────────────────┐
│ Epic 2: Wire into useSessionService.ts                     │
│  Story 2.1 — instantiate refreshCoordinatorRef              │
│  Story 2.2 — site #1: public listSessions() (needs 2.1)     │
│  Story 2.3 — site #2: watch-stream initial snapshot         │
│               (needs 2.1)                                   │
│  Story 2.4 — site #3/#3b: backwards-jump resync,             │
│               success + error paths (needs 2.1)             │
│  Story 2.5 — unit test: differently-filtered coalesced       │
│               caller loses its own filter (documents          │
│               the accepted ADR-001 tradeoff) (needs 2.2-2.4) │
│  Story 2.6 — diff review: streamGenerationRef checks          │
│               untouched (needs 2.3, 2.4)                     │
└───────────────────────────┬──────────────────────────────┘
                             │ (site #4 needs no direct wiring —
                             │  it re-enters site #2's already-
                             │  wired code path; Story 2.7 adds
                             │  a test proving this)
                             ▼
┌──────────────────────────────────────────────────────────┐
│ Epic 3: Regression proof (requirements.md Success Metrics) │
│  Story 3.1 — backstop-triggered reconnect is naturally       │
│               coordinated via site #2 (needs 2.3)            │
│  Story 3.2 — integration test: two overlapping                │
│               listSessions() calls resolved out of order      │
│               (needs 2.2)                                     │
└──────────────────────────────────────────────────────────┘
```

---

## Phase 1: The coordinator utility

### Epic 1.1: `RefreshCoordinator<T>`
**Goal**: A pure, framework-free class implementing "≤1 in-flight fetch + ≤1 coalesced
pending rerun + stale-response discard + per-caller settle," fully unit-tested in isolation
before any wiring happens.

#### Story 1.1.1: Core coalescing — single request and burst-of-N collapse
**As a** `useSessionService.ts` call site, **I want** my `request()` call to either run
immediately or be folded into a pending rerun, **so that** at most one `ListSessions` fetch is
ever in flight regardless of how many call sites fire close together.

**Acceptance Criteria**:
- A single `request()` call with no concurrent activity runs its fetcher immediately.
  - *Given* a fresh `new RefreshCoordinator<ListSessionsResponse>()` (state `{ kind: "idle" }`), *When* `coordinator.request(fetcherA, onResultA)` is called and `fetcherA` resolves with `{ sessions: [sessionX], systemMemoryPct: 42 }`, *Then* `fetcherA` was called exactly once, `onResultA` was called exactly once with that exact object, and the coordinator's state returns to `{ kind: "idle" }`.
- A burst of N `request()` calls while one is in flight collapses to exactly 2 fetcher invocations total (the original + one coalesced rerun), with only the **latest** queued fetcher/onResult pair actually running.
  - *Given* `coordinator.request(fetcherA, onResultA)` is in flight (state `{ kind: "inFlight", generation: 1 }`, `fetcherA` not yet resolved), *When* `coordinator.request(fetcherB, onResultB)` then `coordinator.request(fetcherC, onResultC)` are both called before `fetcherA` resolves, *Then* the state is `{ kind: "inFlightWithPending", generation: 1, pending: { fetcher: fetcherC, onResult: onResultC, waiters: [...] } }` (not `fetcherB` — last-caller-wins), `fetcherB` is never invoked, and once `fetcherA` resolves, `fetcherC` is invoked exactly once and `onResultC` fires with `fetcherC`'s result.

**Files**: `web-app/src/lib/utils/refreshCoordinator.ts`, `web-app/src/lib/utils/refreshCoordinator.test.ts`

##### Task 1.1.1a: Scaffold `RefreshCoordinator<T>` with `CoordinatorState<T>` (~4 min)
- Create `web-app/src/lib/utils/refreshCoordinator.ts`. Define `type CoordinatorState<T> = { kind: "idle" } | { kind: "inFlight"; generation: number } | { kind: "inFlightWithPending"; generation: number; pending: PendingRequest<T> }` and `interface PendingRequest<T> { fetcher: () => Promise<T>; onResult: (result: T) => void; waiters: Array<{ resolve: () => void; reject: (err: unknown) => void }> }`.
- Class `RefreshCoordinator<T>` with private `state: CoordinatorState<T> = { kind: "idle" }` and private `generation = 0`.
- Add a top-of-file JSDoc: what it coalesces, and explicitly "never dispatch the return value of `request()` — it resolves to `Promise<void>`; all side effects happen inside `onResult`, invoked by the coordinator itself" (Risk Control row 4).
- Files: `refreshCoordinator.ts`.

##### Task 1.1.1b: Implement `request()` — idle path and coalescing path (~5 min)
- Implement `request(fetcher: () => Promise<T>, onResult: (result: T) => void): Promise<void>`:
  - If `this.state.kind === "idle"`: return `this.run(fetcher, onResult)`.
  - Else (`"inFlight"` or `"inFlightWithPending"`): build/overwrite `pending = { fetcher, onResult, waiters: [...existing waiters if any] }`, set `this.state = { kind: "inFlightWithPending", generation: this.state.generation, pending }`, and return `new Promise<void>((resolve, reject) => pending.waiters.push({ resolve, reject }))`.
- Files: `refreshCoordinator.ts`.

##### Task 1.1.1c: Implement `run()` — fetch, settle, drain pending synchronously (~5 min)
- Implement private `async run(fetcher, onResult): Promise<void>`: increments `this.generation`, sets `this.state = { kind: "inFlight", generation: this.generation }`, `await fetcher()`, on success (if `this.generation` still matches the captured value) call `onResult(result)`; the drain-pending step (clear state, check for a queued `pending`, synchronously start its rerun if present) happens in a `finally` block with no `await` between clearing state and checking `pending` (Pitfall 1b — no yielded microtask between the two).
- Files: `refreshCoordinator.ts`.

##### Task 1.1.1d: Unit tests — single request + burst-of-N collapse (~5 min)
- `describe("RefreshCoordinator")`: `request_should_invokeFetcherOnce_When_calledWithNoConcurrentActivity`; `request_should_collapseBurstToLatestCaller_When_NCallsArriveWhileOneInFlight` (3-caller burst per the Story's AC, asserting `fetcherB` never invoked, `fetcherC` invoked exactly once). Use the manually-resolved-promise pattern from `useGenerateRule.test.ts:103-130` (no fake timers).
- Files: `refreshCoordinator.test.ts`.

#### Story 1.1.2: Stale-response discard
**As a** caller whose fetch has been superseded, **I want** my `onResult` never invoked with
outdated data, **so that** `dispatch(setSessions(...))` never fires with data older than what a
newer request already applied.

**Acceptance Criteria**:
- A response resolving after a newer fetch has already started and completed is discarded — its `onResult` is never called.
  - *Given* two independently-controlled promises `pA`/`pB` (`resolveA`/`resolveB` held open), `coordinator.request(() => pA, onResultA)` called first (starts immediately, `generation === 1`), then `coordinator.request(() => pB, onResultB)` called while `pA` is still pending (coalesces as `pending`, will become `generation === 2` when it runs), *When* `resolveB({ sessions: [sessionY], systemMemoryPct: 10 })` fires, microtasks flush, *then* `resolveA({ sessions: [sessionX], systemMemoryPct: 5 })` fires (A resolves second, after B's rerun has already started and finished) *Then* `onResultA` is never called and `onResultB` is called exactly once with B's data.

**Files**: `web-app/src/lib/utils/refreshCoordinator.test.ts`

##### Task 1.1.2a: Unit test — out-of-order resolution discards the stale one (~5 min)
- Port `research/pitfalls.md` §3's exact pattern: `pA`/`pB` manually-resolved promises, `coordinator.request(() => pA, onResultA)` then `coordinator.request(() => pB, onResultB)` while A is in flight, resolve B then A, assert `onResultA` not called / `onResultB` called once. Test name: `request_should_discardStaleOnResult_When_aSupersededFetchResolvesAfterANewerOne`.
- Files: `refreshCoordinator.test.ts`.

##### Task 1.1.2b: Unit test — generation invariant holds even with 3+ chained coalesces (~4 min)
- Chain 3 sequential in-flight windows (A running, B queued then C coalesces over B, C running, D queued) and assert only A's and C's `onResult`s ever fire, never B's or a stale one. Test name: `request_should_neverInvokeOnResultForAnIntermediatelyCoalescedCaller_When_MultipleCallsQueueInSuccession`.
- Files: `refreshCoordinator.test.ts`.

#### Story 1.1.3: Per-caller settle, including coalesced-caller failure (the reference-sketch bug fix)
**As a** call site whose request got coalesced away, **I want** my `request()` promise to
settle (resolve or reject) once the actual fetch it rode along with completes, **so that** my
own `finally { dispatch(setLoading(false)) }` always runs — including when that fetch fails.

**Acceptance Criteria**:
- Every caller folded into a pending rerun gets its own promise resolved once that rerun succeeds.
  - *Given* `coordinator.request(fetcherA, onResultA)` in flight, `coordinator.request(fetcherB, onResultB)` coalesces (its returned promise `pB` is pending), *When* `fetcherA` resolves and the coalesced rerun (`fetcherB`) then resolves, *Then* `pB` resolves (does not reject, does not hang).
- Every caller folded into a pending rerun gets its own promise **rejected** if that rerun fails — not left hanging.
  - *Given* the same setup, *When* the coalesced rerun's `fetcherB` rejects with `new Error("network down")`, *Then* `pB` rejects with that same error (not left pending forever) — this is the fix for the bug present in `research/architecture.md` §6's illustrative (single-arm `.then()`) sketch.

**Files**: `web-app/src/lib/utils/refreshCoordinator.test.ts`

##### Task 1.1.3a: Implement both-arm settle in the pending-drain step (~4 min)
- In `run()`'s `finally` block (Task 1.1.1c), when starting the coalesced rerun, chain it as `this.run(pending.fetcher, pending.onResult).then(() => pending.waiters.forEach(w => w.resolve()), (err) => pending.waiters.forEach(w => w.reject(err)))` — both arguments to `.then()`, not a single success-only arm. Also ensure the *direct* (non-coalesced) caller path (`return this.run(...)` from `request()`) correctly propagates a rejected `fetcher()` as a rejected `request()` promise (re-throw the caught error at the end of `run()` after the `finally` block's synchronous drain logic has run).
- Files: `refreshCoordinator.ts`.

##### Task 1.1.3b: Unit tests — coalesced-caller resolve and reject paths (~5 min)
- `request_should_resolveEveryCoalescedWaiter_When_theCoalescedFetchSucceeds`; `request_should_rejectAllCoalescedWaiters_When_theCoalescedFetchRejects` (asserts `await expect(pB).rejects.toThrow("network down")`, and that this does not throw an unhandled-rejection warning). Also assert the *direct* caller's own promise rejects on its own fetcher's failure (`request_should_rejectCallersOwnPromise_When_itsOwnFetcherRejectsAndNoCoalescingOccurred`).
- Files: `refreshCoordinator.test.ts`.

---

## Phase 2: Wiring into `useSessionService.ts`

### Epic 2.1: Instantiate and wire the coordinator into the 4 call sites
**Goal**: Every existing `listSessions`-triggering call site routes its fetch + dispatch
through `refreshCoordinatorRef.current.request(...)`, with `setLoading`/`setError` bracketing
and the existing `streamGenerationRef` checks left otherwise untouched.

#### Story 2.1.1: Instantiate `refreshCoordinatorRef`
**As** `useSessionService`, **I want** one `RefreshCoordinator<ListSessionsResponse>` per hook
instance, **so that** all 4 call sites share the same coalescing state.

**Acceptance Criteria**:
- The ref is created once per hook mount, alongside the other per-instance refs.
  - *Given* `GlobalSessionServiceProvider` mounts `useSessionService` once, *When* the hook body runs, *Then* `refreshCoordinatorRef.current instanceof RefreshCoordinator` is true and it is never reassigned on subsequent renders (created via `useRef(new RefreshCoordinator<ListSessionsResponse>())`, matching `backoffRef`'s exact pattern at line 176).

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.1a: Add the ref and import (~2 min)
- Add `import { RefreshCoordinator } from "@/lib/utils/refreshCoordinator";` and `import type { ListSessionsResponse } from "@/gen/session/v1/session_pb";` to the import block (`useSessionService.ts:1-38`). Add `const refreshCoordinatorRef = useRef(new RefreshCoordinator<ListSessionsResponse>());` immediately after `backoffRef` (line 176), with a one-line comment: "Coalesces concurrent ListSessions fetches across all 4 call sites in this hook; see refreshCoordinator.ts."
- Files: `useSessionService.ts`.

#### Story 2.1.2: Wire site #1 — public `listSessions()`
**As a** caller of `useSessionServiceContext().listSessions(...)`, **I want** my call
coordinated with the other 3 internal sites, **so that** a slow response from an earlier call
can never clobber my newer one (or vice versa).

**Acceptance Criteria**:
- A single `listSessions({status: SessionStatus.ACTIVE})` call still dispatches its result and clears loading, unchanged from today's behavior.
  - *Given* `clientRef.current.listSessions` resolves with `{ sessions: [sessionActive1], systemMemoryPct: 55 }`, *When* `listSessions({ status: SessionStatus.ACTIVE })` is called and awaited, *Then* `dispatch(setSessions([sessionActive1]))` fires exactly once, `setSystemMemoryPct(55)` fires, `dispatch(setLoading(true))` then `dispatch(setLoading(false))` bracket the call as before, and `dispatch(setError(null))` fires.

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.2a: Route site #1's fetch+dispatch through `request()` (~5 min)
- Inside the existing `try { ... } catch (err) { ... } finally { ... }` block (`useSessionService.ts:219-237`, unchanged), replace lines 220-230's `const response = await clientRef.current.listSessions({...}); dispatch(setSessions(response.sessions)); dispatch(setError(null)); if (response.systemMemoryPct > 0) { setSystemMemoryPct(response.systemMemoryPct); }` with:
  ```ts
  await refreshCoordinatorRef.current.request(
    () => clientRef.current!.listSessions({
      category: listOptions?.category,
      status: listOptions?.status,
      includeArchived: listOptions?.includeArchived,
    }),
    (response) => {
      dispatch(setSessions(response.sessions));
      dispatch(setError(null));
      if (response.systemMemoryPct > 0) setSystemMemoryPct(response.systemMemoryPct);
    }
  );
  ```
  Leave `dispatch(setLoading(true))`/`dispatch(setError(null))` (line 216-217) and the surrounding `try/catch/finally` (lines 214-237) exactly as-is — only the body of the `try` changes.
- Files: `useSessionService.ts`.

#### Story 2.1.3: Wire site #2 — watch-stream initial snapshot
**As** the watch stream's `startStream()` closure, **I want** its initial-snapshot fetch
coordinated with the other sites, **so that** a concurrent `listSessions()` caller and the
stream's own snapshot fetch never race each other unguarded.

**Acceptance Criteria**:
- The initial snapshot still gates on `streamGenerationRef` exactly as before, now via `onResult`.
  - *Given* `streamGenerationRef.current === myGeneration` at the moment the coordinator's `onResult` fires with `{ sessions: [sessionZ], systemMemoryPct: 0 }`, *When* `onResult` runs, *Then* it checks `shouldReconnectRef.current` and `streamGenerationRef.current === myGeneration` (both must hold) before `dispatch(setSessions([sessionZ]))` — identical guard to today's inline check at `useSessionService.ts:844`, just relocated inside `onResult`.

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.3a: Route site #2's fetch+dispatch through `request()` (~5 min)
- Replace `useSessionService.ts:840-845` (`const initialResponse = await clientRef.current.listSessions({...}); if (!shouldReconnectRef.current || streamGenerationRef.current !== myGeneration) return; dispatch(setSessions(initialResponse.sessions));`) with:
  ```ts
  await refreshCoordinatorRef.current.request(
    () => clientRef.current!.listSessions({
      category: watchOptionsRef.current?.categoryFilter,
      status: watchOptionsRef.current?.statusFilter,
    }),
    (response) => {
      if (!shouldReconnectRef.current || streamGenerationRef.current !== myGeneration) return;
      dispatch(setSessions(response.sessions));
    }
  );
  ```
  The `streamGenerationRef`/`shouldReconnectRef` check moves *inside* `onResult` (still runs, just at dispatch time instead of immediately after `await`) — this is required, not cosmetic: `request()` may return well after `fetcher()` resolves if this call got coalesced behind another, so re-checking at the point `onResult` actually fires (which the coordinator itself guarantees happens synchronously right after the winning fetch resolves) is what keeps the guard meaningful.
- Files: `useSessionService.ts`.

#### Story 2.1.4: Wire sites #3/#3b — backwards-jump full resync (success path + error path)
**As** the stream's backwards-jump resync logic, **I want** both its success-path and
error-path full resyncs coordinated with the rest, **so that** a resync triggered by a stream
close racing a resync triggered by a stream error (or either racing site #1/#2) never
double-fires or clobbers a newer result.

**Acceptance Criteria**:
- Both resync call sites use the same coordinator and the same `streamGenerationRef` guard, now inside `onResult`.
  - *Given* `needsFullResyncRef.current === true` and the stream ends normally (success path, `useSessionService.ts:874-884`), *When* the resync's `onResult` fires with `{ sessions: [sessionW], systemMemoryPct: 0 }` and `streamGenerationRef.current === myGeneration`, *Then* `dispatch(setSessions([sessionW]))` fires — identical outcome to today's `.then(r => { if (...) dispatch(setSessions(r.sessions)); })`.

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.4a: Route site #3 (success-path resync) through `request()` (~4 min)
- Replace `useSessionService.ts:876-883`'s `void clientRef.current?.listSessions({...}).then(r => { if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) { dispatch(setSessions(r.sessions)); } });` with:
  ```ts
  void refreshCoordinatorRef.current.request(
    () => clientRef.current!.listSessions({
      category: watchOptionsRef.current?.categoryFilter,
      status: watchOptionsRef.current?.statusFilter,
    }),
    (response) => {
      if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
        dispatch(setSessions(response.sessions));
      }
    }
  );
  ```
  (Kept `void`-fired/not-awaited, matching today's fire-and-forget shape — this call site doesn't block the surrounding `if` block on its resolution today either.)
- Files: `useSessionService.ts`.

##### Task 2.1.4b: Route site #3b (error-path resync) through `request()` — same shape (~4 min)
- Apply the identical transformation from Task 2.1.4a to `useSessionService.ts:921-928` (the catch-block duplicate). Both sites now call the *same* `refreshCoordinatorRef.current.request(...)`, so if a stream-close and a stream-error somehow both trigger a resync in quick succession, they coalesce like any other pair of call sites instead of firing two independent, uncoordinated RPCs.
- Files: `useSessionService.ts`.

##### Task 2.1.4c: Unit test — both resync call sites share coordination (~4 min)
- In `useSessionService.test.ts`, add a test asserting `clientRef.current.listSessions` mock call count stays at 1 (not 2) when both the success-path and error-path resync logic are triggered back-to-back within the same coordinator window (drive via the existing stream-mocking harness in that file; verify exact mock shape first). Test name: `watchSessions_should_coalesceBackToBackResyncTriggers_When_bothSuccessAndErrorPathsFireCloseTogether`.
- Files: `useSessionService.test.ts`.

#### Story 2.1.5: Document the accepted filter-heterogeneity tradeoff with a dedicated test
**As** a future reader of this code, **I want** a test that makes ADR-001's accepted tradeoff
observable, **so that** "a differently-filtered caller can lose its own filter to a coalesced
later caller" is a documented, tested behavior — not a silent surprise discovered in
production.

**Acceptance Criteria**:
- A `listSessions({status: ACTIVE})` call coalesced behind an unfiltered watch-stream resync ends up reflecting the unfiltered result, not its own filtered one — and this is asserted, not just implied.
  - *Given* `coordinator` has a fetch in flight (any site), `listSessions({status: ACTIVE})` (site #1) then `watchSessions()`'s resync (unfiltered, site #3) both call `request()` while that fetch is in flight, with the resync's call arriving last, *When* the in-flight fetch resolves and the coalesced rerun (the resync's unfiltered fetcher) resolves with `{ sessions: [allSessions], systemMemoryPct: 0 }`, *Then* `dispatch(setSessions([allSessions]))` fires (from the resync's `onResult`) — the `status: ACTIVE`-filtered caller's own `onResult` (which would have dispatched only active sessions) is never invoked, matching ADR-001's documented accepted tradeoff.

**Files**: `web-app/src/lib/hooks/useSessionService.test.ts`

##### Task 2.1.5a: Add the coalesced-filter-loss regression test (~5 min)
- Test name: `listSessions_should_loseItsOwnFilteredOnResult_When_coalescedBehindADifferentlyFilteredLaterCaller` — links back to `ADR-001` in a code comment so a future reader who trips over this behavior finds the rationale immediately, not just the assertion.
- Files: `useSessionService.test.ts`.

#### Story 2.1.6: Confirm `streamGenerationRef` guard is untouched
**As** the existing stream-reconnect safety net, **I want** every one of my checkpoints left
exactly where they are, **so that** this refactor doesn't regress a guard `requirements.md`
explicitly says must not regress.

**Acceptance Criteria**:
- All 6 existing `streamGenerationRef.current !== myGeneration` checkpoints still exist, unmodified in condition, at their (possibly relocated-inside-`onResult`) call sites.
  - *Given* the diff produced by Stories 2.1.2-2.1.4, *When* reviewed line by line, *Then* every one of the 6 checks originally at `useSessionService.ts:844,869,891,914,925,936` is still present with the identical boolean expression (only 844/876-ish's textual line position moved *inside* an `onResult` callback body, per Task 2.1.3a/2.1.4a/2.1.4b — the check itself is byte-identical).

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.6a: Manual diff review checklist (~3 min)
- Grep `streamGenerationRef.current` in the final file; confirm exactly 6 (or the count found during Task 2.1.2-2.1.4's edits, if reconnect logic elsewhere also references it) comparison checkpoints remain, none deleted, none with an altered condition. Record the confirmed count in the task's completion note.
- Files: `useSessionService.ts` (review only, no edit expected).

---

## Phase 3: Regression proof (requirements.md Success Metrics)

### Epic 3.1: Prove the original race is closed
**Goal**: Directly verify requirements.md's 3 Success Metrics with concrete tests — not just
trust that Phase 1/2's unit tests imply it.

#### Story 3.1.1: Backstop-triggered reconnect is naturally coordinated
**As** the 30s staleness backstop, **I want** my `watchSessionsRef.current?.(...)` call to
benefit from the same coordination as site #2, **so that** Success Metric 1 ("no two
`ListSessions` RPCs in flight ... including the staleness-backstop-triggered reconnect")
holds without needing its own separate wiring.

**Acceptance Criteria**:
- The backstop's reconnect re-enters `watchSessions()` → `startStream()` → site #2's already-coordinated fetch; no independent `listSessions` call exists at the backstop's own call site.
  - *Given* `watchSessionsRef.current?.(watchOptionsRef.current)` fires at `useSessionService.ts:971` while site #2's `startStream()` initial-snapshot fetch (from a separate, still-in-flight `watchSessions()` invocation) is in flight, *When* the backstop's call re-enters `startStream()` and reaches its own `listSessions` call, *Then* that call goes through the *same* `refreshCoordinatorRef.current.request(...)` (Task 2.1.3a), so it coalesces exactly like any other concurrent site #2 invocation — mock `listSessions` call count stays at 1 across both invocations while the first is in flight.

**Files**: `web-app/src/lib/hooks/useSessionService.test.ts`

##### Task 3.1.1a: Unit test — backstop reconnect coalesces with an in-flight snapshot fetch (~5 min)
- Drive two overlapping `watchSessions()` calls (simulating the backstop firing while an initial `startStream()` snapshot fetch is still in flight) and assert `mockListSessions` was called once during the overlap window. Test name: `backstopReconnect_should_coalesceWithAnInFlightInitialSnapshotFetch_When_bothInvokeStartStreamConcurrently`.
- Files: `useSessionService.test.ts`.

#### Story 3.1.2: Integration test — out-of-order `listSessions()` resolution no longer clobbers state
**As** the requirements' Success Metric 3, **I want** a `useSessionService`/`sessionsSlice`
integration test simulating two overlapping `listSessions()` calls resolved out of order, **so
that** the fix is proven at the level the bug was originally described (Redux store state), not
only at the coordinator-unit level.

**Acceptance Criteria**:
- Two overlapping `listSessions()` calls, where the first-issued call's response resolves *after* the second-issued call's response, leave the Redux store reflecting only the second (newer) call's data.
  - *Given* a test Redux store wired to `sessionsSlice`, a `renderHook(() => useSessionService())` instance, and a mocked `clientRef.current.listSessions` returning `pA` on the first call and `pB` on the second (both manually-resolved), *When* `act(() => { result.current.listSessions({ status: SessionStatus.ACTIVE }); })` fires (call A), then `act(() => { result.current.listSessions({}); })` fires while A is still pending (call B, coalesces per Story 2.1.2's wiring), then `resolveB({ sessions: [sessionAll], systemMemoryPct: 0 })` fires, microtasks flush, then `resolveA({ sessions: [sessionActiveOnly], systemMemoryPct: 0 })` fires, *Then* `store.getState().sessions` (via `selectAllSessions`) contains `[sessionAll]`, never `[sessionActiveOnly]`, and `store.getState().sessions.loading === false` (not stuck `true`).

**Files**: `web-app/src/lib/hooks/useSessionService.test.ts`

##### Task 3.1.2a: Build the out-of-order-resolution test harness (~5 min)
- Extend `useSessionService.test.ts`'s existing mock-client setup (verify its exact current shape first) to support `mockListSessions.mockReturnValueOnce(pA).mockReturnValueOnce(pB)` with independently-held `resolveA`/`resolveB` closures, following the exact pattern from `research/pitfalls.md` §3 / `useGenerateRule.test.ts:103-130`.
- Files: `useSessionService.test.ts`.

##### Task 3.1.2b: Write the out-of-order-resolution assertion test (~5 min)
- Test name: `listSessions_should_reflectOnlyTheNewerCallsData_When_anOlderCallsResponseResolvesAfterANewerOnesResponse`. Assert final store state (via `selectAllSessions`) and `loading === false` per the Story's AC.
- Files: `useSessionService.test.ts`.

---

## Verification

- `cd web-app && npx jest --no-coverage --testPathPatterns="refreshCoordinator.test"` — all Phase 1 tests green.
- `cd web-app && npx jest --no-coverage --testPathPatterns="useSessionService.test"` — all Phase 2/3 tests green, including the 3 new integration-level tests.
- `cd web-app && npx tsc --noEmit` (or the project's standard type-check command) — confirms `RefreshCoordinator<ListSessionsResponse>`'s generic wiring type-checks across all 4 call sites.
- No changes to `proto/`, `session/`, `server/` — confirmed via `git status` before commit (frontend-only per `requirements.md` Constraints).

## Implementation Notes (2026-08-24)

Two deviations from the task breakdown above, made during implementation:

1. **`guarded` option and `LIST_SESSIONS_TIMEOUT_MS`, not in the original Epic 1/2 tasks.**
   These were required to resolve adversarial-review's two BLOCKERs (see ADR-001's Amendment
   section) — the original Phase 1/2 tasks above predate that review and don't include them.
2. **Story 2.1.4c's dedicated "success-path and error-path resync fire close together"
   integration test was not written as a separate test.** Both resync call sites now route
   through the identical `refreshCoordinatorRef.current.request(..., { guarded: true })` call
   (Tasks 2.1.4a/2.1.4b), so "two guarded requests overlap and coalesce" is exactly the scenario
   already covered by `refreshCoordinator.test.ts`'s
   `request_should_overwriteAGuardedPendingFetcher_When_ALaterGuardedCallerCoalesces` plus the
   real-hook-level `backstopReconnect_should_coalesceWithAnInFlightInitialSnapshotFetch_When_bothInvokeStartStreamConcurrently`
   (Story 3.1.1a, which exercises two concurrent `watchSessions()`/`startStream()` invocations
   end-to-end). Driving the mocked WebSocket stream to completion on both the success and error
   paths simultaneously to construct Story 2.1.4c's literal scenario would have added test
   complexity for coverage already proven by the above.

Also: `validation.md`'s literal test-body description for
`request_should_discardStaleOnResult_When_aSupersededFetchResolvesAfterANewerOne` (2-caller
pA/pB, "B resolves before A") describes an interleaving that's unreachable under the finalized
≤1-in-flight-serialized design (Pattern Decisions' "Generation increment point" row) — a
superseded fetcher's `fetcher()` is never even invoked, so it can't "resolve after" anything.
Implemented as a 3-caller scenario (A running, B queued, C supersedes B before B's fetcher ever
runs) instead, which proves the same requirement (a superseded response's `onResult` is never
invoked) via the guarantee the finalized design actually provides.

## Follow-up (not fixed here): premature `loading:false` under coalescing

Found by `code:review`'s architecture pass during shipping (2026-08-24), not by the original
plan/adversarial-review: site #1's `finally { dispatch(setLoading(false)) }`
(`useSessionService.ts`'s `listSessions()`) clears as soon as *that caller's own* `request()`
settles — not when a coalesced-behind rerun whose data will actually land in Redux next
finishes. Under the coordinator's deferred-execution model this window is a full RPC
round-trip (wider than pre-diff, where each concurrent `listSessions()` call fired its own
already-running RPC). UI-only: Redux `sessions` data is never wrong, only `loading` can read
`false` for one or more ticks mid-refresh (spinner flicker). Out of scope for this item's
acceptance criteria (AC3 is about the stuck-`true`-forever failure mode, which is fixed).

The "obvious" fix — gate the clear on a `RefreshCoordinator.isBusy` getter — is wrong: the
coordinator's `state`/`pending` are shared across all 4 call sites, including the 3
stream-lifecycle sites that never touch `loading` and fire routinely on WebSocket reconnects;
gating on global busy-ness would keep `loading` stuck `true` on unrelated background activity.
A correct fix needs a site-#1-scoped "is my own request (including anything it got coalesced
into) actually done" signal, not the coordinator's shared busy state. Left as a scoped
follow-up, documented inline at `useSessionService.ts`'s `listSessions()` `finally` block.
