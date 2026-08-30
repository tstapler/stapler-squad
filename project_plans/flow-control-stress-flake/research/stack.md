# Research: Stack (Agent 1) — flow-control-stress-flake

## Toolchain versions (VERIFIED — `web-app/package.json`)

- `jest`: `^30.2.0`
- `ts-jest`: `^29.4.11`
- `@types/jest`: `^30.0.0`
- Node: `v26.0.0` (`node --version` in this worktree)
- `web-app/jest.config.js` (the `"web-app"` project used by `flow-control-stress.test.ts`): no `timers` key set — Jest 30 has only one fake-timer implementation ("modern", `@sinonjs/fake-timers`-based; the legacy implementation was removed in Jest 27), so `useFakeTimers()` here is unconditionally the modern one with the full async API surface below.
- `node_modules` is not installed in this worktree (`find` for `@jest/fake-timers` under it returned nothing), so the API surface below is **INFERRED** from Jest's public docs/changelog for the pinned `^30.2.0` range, not verified against an installed `.d.ts`. This surface (`advanceTimersByTimeAsync`, `runAllTimersAsync`, `useFakeTimers({ advanceTimers })`) has been stable since Jest 27 and is unchanged in 29/30, so the inference risk is low, but flag it as unverified-in-this-worktree if that matters later.

## 1. Fake timer API surface and the await-a-real-timer-callback gotcha

Jest 30's modern fake timers (from `@sinonjs/fake-timers`) expose, among others:

- `jest.useFakeTimers()` — replaces global `setTimeout`/`setInterval`/etc. Timers no longer fire on their own; nothing advances until told to.
- `jest.advanceTimersByTime(msToRun)` — **synchronous**. Advances the fake clock and synchronously runs any timers whose deadline falls within that window, but any `.then`/microtask chains queued by those timer callbacks are only *scheduled*, not drained — code doing `await someTimerDrivenPromise` right after a plain `advanceTimersByTime` call inside the same tick can still race the callback's `resolve()`.
- `jest.advanceTimersByTimeAsync(msToRun)` — **async** variant: after advancing, it also flushes the microtask queue (`await`s a `Promise.resolve()` internally) so any promise resolved by the callback settles before the returned promise resolves. This is the one that matters here.
- `jest.runAllTimersAsync()` / `jest.runOnlyPendingTimersAsync()` — drain-the-queue variants; async-safe but **unbounded** — they run every currently-scheduled (and, for `runAllTimersAsync`, every subsequently-scheduled) timer until none remain. Dangerous for any code that reschedules itself (see §2 below — this codebase has an explicit written rule against using these for exactly that reason).
- `jest.useFakeTimers({ advanceTimers: true })` (or `{ advanceTimers: <ms> }`) — "auto-advance" mode: Jest installs a **real** background interval (default granularity ~20ms of real wall-clock time per tick, configurable via the numeric form) that keeps nudging the fake clock forward on its own, so code can `await` a promise chained off a faked `setTimeout` without any manual `advanceTimersByTime*` call. This reintroduces a *small, fixed-granularity* wall-clock dependency (the background interval itself), but decouples the total wall-clock cost from the *iteration count* — it's a constant ~20ms/tick cost instead of the current pattern's ~1ms × 5000-round-trip serialization. Available in the pinned Jest 30 range (introduced Jest 27.something, unchanged since).

### The deadlock gotcha (confirmed applicable here)

`await new Promise((resolve) => { tracker.write(chunk, () => { completed++; resolve(); }) })` where `tracker.write` internally does `setTimeout(() => { ...; callback(); }, 1)`: once fake timers are active, that inner `setTimeout` never fires on its own. `await`ing the promise **before** anything advances the fake clock deadlocks the test (nothing else in the JS single-threaded runtime can advance the clock while the `await` is suspended). The two ways to avoid this that fit an ordinary `for` loop with a per-iteration `await`:

1. **Race the advance and the await together**, since both are promises:
   ```ts
   const writeDone = new Promise<void>((resolve) => {
     tracker.write(safeChunk, () => { completed++; resolve(); });
   });
   await Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)]);
   ```
   This works because `Promise.all` starts both promises immediately; `advanceTimersByTimeAsync` fires the pending `setTimeout` synchronously-then-flushes-microtasks in the same turn, so `writeDone`'s `resolve()` runs and `Promise.all` settles — no dependency on wall-clock scheduling at all, since the "clock" is now the fake one Jest advances programmatically.
2. **Auto-advance mode** (`useFakeTimers({ advanceTimers: true })`) — leave the loop body exactly as it is (`await new Promise(...)` with no explicit advance call); the background real interval ticks the fake clock forward until the pending timer's deadline is crossed, then the promise resolves on its own. Simpler diff, but still has *some* wall-clock coupling (the ~20ms polling granularity), just a small constant one instead of a per-iteration one.

Given AC1 ("must not depend on real wall-clock timer scheduling to determine pass/fail" at all), pattern 1 (manual `advanceTimersByTimeAsync` via `Promise.all`) is the stricter fit — it has **zero** wall-clock dependency, whereas auto-advance still has a small one. Pattern 2 is simpler code but reintroduces a (much smaller, bounded) real-timer coupling that a sufficiently starved CI runner could theoretically still miss, though the risk is far lower than the current ~5000× multiplier.

## 2. Existing fake-timer usage in this codebase — does an await-a-timer-callback precedent exist?

- `grep -rln "useFakeTimers"` finds ~29 files under `web-app/src/`, but **none** currently call `advanceTimersByTimeAsync` or `runAllTimersAsync`, and all existing `advanceTimersByTime` call sites (`BacklogItemDetail.test.tsx`, `XtermTerminal.test.tsx`, `ReviewQueueContent.auto-advance.test.tsx`, etc.) use the **synchronous** variant, wrapped in React Testing Library's `act()`, to drive React state updates/effects — not to resolve a bare `await new Promise(resolve => setTimeout(resolve, N))` in a plain (non-React) async function body. There is no existing "await a promise resolved by a real/faked setTimeout inside a hot loop" precedent to match for consistency — the fix here would be the first instance of that specific pattern in the repo.
- `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx:12-16` (its own file-header comment) documents an explicit, hard-won rule directly relevant to caution here:
  > "Per pitfalls §5: the fake-timer 'drain the whole queue' APIs must never be used here -- the bug this suite guards against is a self-rescheduling timer loop, and those APIs would hang the Jest worker on a regression instead of failing with a useful assertion. Only `jest.advanceTimersByTime()` with bounded, fixed millisecond budgets is used throughout."

  This doesn't directly forbid `advanceTimersByTimeAsync` for `flow-control-stress.test.ts` (that file's loop is a fixed 5000-iteration bound, not a self-rescheduling timer, so `runAllTimersAsync`/`runOnlyPendingTimersAsync` wouldn't hang the same way), but it is precedent in this codebase for preferring **bounded, explicit** timer-advance calls over "drain everything" APIs whenever there's any risk of an unbounded or self-perpetuating timer chain — reinforcing that the `Promise.all([writeDone, advanceTimersByTimeAsync(1)])` per-iteration pattern (explicit, bounded, one timer at a time) is more consistent with this repo's established caution than reaching for `runAllTimersAsync()` as a blanket fix.

## 3. Per-test timeout config precedent

- `web-app/jest.config.js` sets no global `testTimeout` at the project level; Jest's default (5000ms) is overridden per-test via the third `test(name, fn, timeoutMs)` argument throughout this file: `handles Claude Code style animations` → 10000ms, `handles cursor positioning sequences` → 10000ms, `handles alternating text and control codes` (the flaky one) → 15000ms, and one more sibling → 10000ms (`flow-control-stress.test.ts:221,247,291,365`). This is the existing convention for "stress test needs more time" in this file — a raised-timeout-only fix (no fake timers) would be consistent with that convention but doesn't address AC1 (still depends on real wall-clock scheduling, just with more slack — same failure mode, lower probability, not eliminated, matching the requirements doc's own framing of sibling tests at 100/1000 iterations as "hasn't yet been observed to flip flaky, not evidence it's immune").
- No repo-wide `testTimeout` in `jest.config.js`, and no CI-side Jest timeout-multiplier environment variable was found (`grep -rn "testTimeout|CI.*timeout|JEST.*TIMEOUT|timeoutMultiplier"` across `.github/workflows/*.yml` and `web-app/jest.config.js` returned nothing). Whatever fixed budget a test declares is the actual budget in CI, with no environment-based slack applied. This raises the stakes for AC1 — a fix that keeps any real-wall-clock coupling remains a potential CI flake source with no config-level safety net available elsewhere in the pipeline.

## 4. Node/Jest version-specific caveats

- Node v26 + Jest 30: no known incompatibilities with fake timers; `@sinonjs/fake-timers` (Jest's fake-timer backend) tracks Node's timer APIs including `setImmediate`/`queueMicrotask`, which this test doesn't use, so no gaps expected.
- `useFakeTimers({ advanceTimers: true })` auto-advancing mode is available in the pinned Jest 30 range (stable since Jest 27, unchanged) — see §1. It is a legitimate simpler alternative but, per AC1's literal wording ("must not depend on real wall-clock timer scheduling to determine pass/fail"), the manual `advanceTimersByTimeAsync`-via-`Promise.all` approach is the closer fit since it has no wall-clock coupling at all, vs. auto-advance's small fixed-interval one.
- No teardown gotcha specific to this repo beyond the general one already documented at `XtermTerminal.test.tsx:225` (`jest.useRealTimers()` in `afterEach`) — any new fake-timer usage in `flow-control-stress.test.ts` must add a matching `jest.useRealTimers()` afterEach/afterAll (currently absent from the file, since it uses no fake timers today) to avoid leaking fake timers into subsequent tests in the same file or contaminating other tests in the JS Dom worker if fake timers install globally-scoped hooks.

## Summary of options for `sdd:3-plan` to choose between

| Option | Wall-clock coupling | Diff size | Consistency with repo precedent |
|---|---|---|---|
| A. `jest.useFakeTimers()` + `Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)])` per iteration | None | Small, localized to the loop body | New pattern for this repo, but consistent with the `XtermTerminal.test.tsx` precedent of preferring bounded/explicit advances over drain-everything APIs |
| B. `jest.useFakeTimers({ advanceTimers: true })`, loop body unchanged | Small, fixed (~20ms/tick, not iteration-count-scaled) | Smallest diff | No existing precedent for `advanceTimers: true` in this repo |
| C. Raise `testTimeout` further (e.g. 15000 → 30000+) | Same as today, just more slack | Trivial | Matches existing per-test-timeout convention in this file, but doesn't satisfy AC1 |

Option A is the strictest read of AC1. Option B is simpler but retains bounded wall-clock coupling. Option C doesn't meet AC1 at all — included only for completeness since it's the file's existing convention for "test needs more time."
