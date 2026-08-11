# Research: refresh-coordinator pitfalls

Scope: what commonly goes wrong building a "one in flight + at most one pending
rerun + stale-response discard" coalescing utility, and what this specific
design (wired into `web-app/src/lib/hooks/useSessionService.ts`) must guard
against. No herdr-web source was found in this repo or reachable filesystem —
findings below are grounded in general coalescing-pattern failure modes plus
direct inspection of this codebase's existing patterns.

## 1. Classic coalescing pitfalls

### 1a. Argument mismatch between the in-flight call and the pending rerun

The four call sites pass **different filter arguments** to `ListSessions`:

- `listSessions()` (`useSessionService.ts:212-240`) — filters come from the
  caller's `listOptions` (category/status/includeArchived), no default filter.
- Watch-stream initial snapshot (`useSessionService.ts:838-845`) — uses
  `watchOptionsRef.current` (categoryFilter/statusFilter).
- Backwards-jump full resync, success path (`useSessionService.ts:874-884`)
  and error path (`useSessionService.ts:918-929`) — same `watchOptionsRef`.

A naive "in flight + pending flag" coordinator (`inFlight: boolean; pending:
boolean`) with no memory of *which* fetcher/args triggered the pending flag
will, on the pending rerun, either re-run the **original** in-flight call's
args (losing whatever the second caller actually wanted) or silently rerun
with **whatever args happen to be captured in a closure created once** — both
wrong for this codebase, where a plain `listSessions({status: X})` call and a
watch-stream's unfiltered/differently-filtered snapshot can legitimately
overlap. Because `setSessions` is a full-store replace with no filter
tagging, applying the wrong caller's filtered result silently corrupts the
displayed session list for the caller who didn't get their own fetch.

**Guard for this design**: `request()` must take the fetcher (and its result
handler) as a parameter *per call*, not configure the coordinator with one
bound fetcher up front. Store the **latest** fetcher/handler pair when
coalescing (last-call-wins for the pending rerun), never the first. This
matches herdr-web's stated "at most one pending re-run" semantics and is a
correctness requirement here, not just a style choice — the plan should call
out explicitly that "latest caller's args win" is the accepted tradeoff (a
call site coalesced away gets whatever the last-queued call's filter
produces, not its own), consistent with the Rabbit Holes note that this is a
whole-snapshot ordering fix, not per-argument merge.

### 1b. Race between clearing `inFlight` and checking `pending`

Classic hand-rolled bug shape:
```js
try {
  await doFetch();
} finally {
  inFlight = false;
  if (pending) { pending = false; run(); } // window here
}
function request() {
  if (inFlight) { pending = true; return; }
  run();
}
```
JS is single-threaded, so this is only unsafe if an `await` (or any
microtask-yielding call) is interposed **between** clearing `inFlight` and
checking/clearing `pending` — e.g. awaiting `dispatch(...)` or a
`Promise.resolve()` before the pending check runs, letting a concurrent
synchronous `request()` call interleave and see `inFlight === false` before
`pending` has been consulted, causing either a double-fetch or a dropped
pending rerun.

**Guard**: keep the check-clear-rerun sequence fully synchronous (no `await`
between them), same discipline this codebase already uses for
`streamGenerationRef` — every dependent check happens immediately after the
`await` that could have raced it (`useSessionService.ts:844, 869, 891, 914,
925, 936`), not deferred behind another async boundary.

### 1c. Thundering herd on completion via error retrigger

If the pending-rerun mechanism fires automatically whenever a request
completes — success *or* failure — and a call site's own error handling
re-invokes `request()` on failure (or a periodic backstop keeps re-queuing
during a persistent outage), you get a tight retry loop.

**Guard**: the coordinator itself must never auto-retry on error. It should
only run a pending rerun when a *new* `request()` call arrived during the
in-flight window — never synthesize one from a failure. None of the 4 call
sites currently auto-retry on `listSessions` failure (they `dispatch(setError(...))`
and stop; the 30s backstop and visibility/online handlers call `watchSessions()`,
a separate code path, not a `listSessions` retry loop), so this is a
guard to preserve, not a gap to close.

## 2. React-specific: stale closures

This file already has an established idiom for avoiding stale closures in
long-lived callback/stream code: refs re-assigned on every render rather than
captured once —
```js
watchSessionsRef.current = watchSessions;  // useSessionService.ts:979
dispatchRef.current = dispatch;            // useSessionService.ts:980
```
and `watchOptionsRef` (`useSessionService.ts:183`, written at `819`) for the
same reason — reconnects must use current filter options, not whatever was
in scope when `startStream` was first created.

**Applied to the coordinator**: if `createRefreshCoordinator()` is
instantiated once via `useRef(createRefreshCoordinator())` (empty deps, as
the coordinator itself has no React dependencies — it's a pure utility per
the requirements), it must hold **no captured business closures** internally.
Its only state should be flow-control primitives (`inFlight: boolean`,
`pending: { fetcher, onResult } | null`, `generation: number`) — never a
bound `dispatch` or bound filter args. Every call site passes its own
fresh `() => Promise<T>` fetcher (which itself closes over the current
`dispatch` — stable per Redux's `useAppDispatch` — and the current
`listOptions`/`watchOptionsRef.current`, read at call time) into
`request(fetcher, onResult)` each time. Given that, the coordinator instance
being created once and reused across renders is safe with no
ref-current-on-every-render treatment needed, unlike `watchSessionsRef`/
`dispatchRef` — because the coordinator never stores a closure across
renders, only across the lifetime of one in-flight+pending cycle, and that
lifetime is always populated by the freshest call.

## 3. Testing out-of-order resolution deterministically

The codebase already has the exact pattern needed, no new test infra
required. `web-app/src/lib/hooks/useGenerateRule.test.ts:103-130` uses a
manually-resolved promise:
```js
let resolveRpc!: (value) => void;
const rpcPromise = new Promise((resolve) => { resolveRpc = resolve; });
mockFn.mockReturnValue(rpcPromise);
...
act(() => { resolveRpc(value); });
```
For two-overlapping-calls ordering (response A resolves *after* response B,
despite A starting first), extend this to two independently-controlled
promises and resolve them out of program order:
```js
let resolveA!: (v) => void, resolveB!: (v) => void;
const pA = new Promise((r) => { resolveA = r; });
const pB = new Promise((r) => { resolveB = r; });
mockListSessions.mockReturnValueOnce(pA).mockReturnValueOnce(pB);

const coordinator = createRefreshCoordinator<Result>();
const p1 = coordinator.request(() => pA, applyA);
const p2 = coordinator.request(() => pB, applyB); // arrives while A in flight

resolveB({ sessions: [...] }); // B resolves first
await flushMicrotasks();
resolveA({ sessions: [...] }); // A resolves second — must be discarded
await Promise.all([p1, p2]);

expect(applyA).not.toHaveBeenCalled(); // stale, discarded
expect(applyB).toHaveBeenCalledTimes(1);
```
No fake timers needed — this is pure microtask ordering, controlled directly
by when each `resolve*` is invoked, matching the existing pattern in this
repo (`jest` 30, `testing-library/react`'s `act`/`waitFor`). Fake timers
would only be relevant if the coordinator itself introduced a debounce delay,
which the herdr-web-style design as scoped here does not.

## 4. Silent-failure: does discarding a stale response strand `loading`?

Checked all 4 call sites for `setLoading`/`setError` usage — **only one of
the four touches `loading` at all**:

- `listSessions()` (`useSessionService.ts:212-240`): `dispatch(setLoading(true))`
  at line 216, unconditional `dispatch(setLoading(false))` in the `finally`
  at line 236.
- Watch-stream initial snapshot (`838-845`), backwards-jump resync success
  path (`874-884`), and error-path resync (`918-929`): **none dispatch
  `setLoading`** today at all.

So the stuck-loading risk is narrower than "any discarded response" — it's
specific to `listSessions()`'s own call: if `listSessions()`'s `request()`
call gets coalesced away (i.e. it becomes the *first* caller during another
call's in-flight window and is folded into that other fetch rather than
getting its own network round-trip), the coordinator must still guarantee
`listSessions()`'s own promise/callback resolves so its `finally` block runs
and clears `loading`. If the coordinator's `request()` silently drops a
coalesced call with no completion signal back to that specific caller (e.g.
it only resolves the *last* queued caller's promise, not every caller who
was waiting), `dispatch(setLoading(true))` at line 216 has no matching
`dispatch(setLoading(false))` and `loading` sticks at `true` in
`sessionsSlice` — with no visible error, since no exception was thrown either.

**Concrete guard for the plan**: `request()` must resolve (or invoke a
per-caller completion callback) for **every** caller that called it, not just
whichever caller's fetcher ultimately executed — coalescing must be
transparent at the completion-signal level even though only one network call
happens. Equivalently (and simpler, matching the Rabbit Holes guidance that
the coordinator should wrap "fetch+dispatch as one unit"): move
`setLoading`/`setError` inside the coordinator-wrapped unit driven by the
coordinator's own `inFlight` transition, rather than leaving each call site
to manage its own local `setLoading` pair around an outer `request()` call —
this sidesteps the per-caller-signal requirement entirely, since there is
then only one `loading` writer, transitioning off the coordinator's own
in-flight state rather than any individual caller's promise settling.

## Summary of design implications for `createRefreshCoordinator<T>()`

1. `request(fetcher: () => Promise<T>, onResult: (result: T) => void)` — no
   bound fetcher/args at construction time; last-caller-wins for the pending
   rerun's fetcher choice.
2. Increment a `generation` counter on every `request()` call (not only when
   a fetch actually starts); compare it synchronously immediately after the
   `await` when a response lands, discarding (skip `onResult`) if stale —
   mirroring `streamGenerationRef`'s check-immediately-after-every-await
   idiom already established in this file.
3. Keep the in-flight-clear → pending-check sequence synchronous, no `await`
   between them.
4. Never auto-retry on error; only a genuinely new `request()` call during
   the flight should trigger a pending rerun.
5. No internal captured business closures — safe to build once via
   `useRef(createRefreshCoordinator())`.
6. Every caller of `request()` must get a completion signal even when
   coalesced away, or `setLoading`/`setError` responsibility should move
   inside the coordinator-wrapped unit so there's a single writer keyed off
   the coordinator's own state transitions.
