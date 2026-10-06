# Research: Build vs. Buy — `flow-control-stress.test.ts` timeout flake

Agent 6 of `sdd:2-research` for backlog item `7cc29340-330b-413d-8e26-5418c550ccde`.

## 1. Existing OSS/stdlib option: is a new dependency justified?

Checked `web-app/package.json` directly (`grep -E '"(jest|sinon|fake-timers|jest-extended)'`):

```
"jest": "^30.2.0"
"jest-environment-jsdom": "^30.2.0"
"ts-jest": "^29.4.11"
```

No `@sinonjs/fake-timers`, no `jest-extended`, no other timer-mocking package as a direct
dependency. Jest 30's own fake timer implementation is already built on `@sinonjs/fake-timers`
internally (bundled transitively, not exposed as something to import directly) — it is the
same engine a standalone `@sinonjs/fake-timers` install would provide, just already wired into
`jest.useFakeTimers()` with zero extra configuration.

Jest's built-in API already covers everything this fix needs:
- `jest.useFakeTimers()` — replace real timers with controllable fake ones
- `jest.advanceTimersByTimeAsync(ms)` — advance fake time and flush the microtask queue so
  `await`ed promises chained off `setTimeout` callbacks resolve deterministically
- `jest.runAllTimersAsync()` — drain all pending timers/microtasks

This codebase already has 20+ test files using exactly this API (e.g.
`src/components/sessions/__tests__/XtermTerminal.test.tsx`,
`src/components/backlog/BacklogItemCard.test.tsx`,
`src/app/review-queue/__tests__/ReviewQueueContent.auto-advance.test.tsx`) — confirmed via
`grep -rln "useFakeTimers\|advanceTimersByTimeAsync\|runAllTimersAsync" src`. There is
established local precedent and no adoption cost.

**Verdict: no new dependency is justified.** Jest's built-in fake timer API is sufficient,
already the de facto standard in this repo, and solves the exact problem (deterministic,
instant advancement of `setTimeout`-driven async code) without adding install size, a new
transitive dependency to audit, or an inconsistent pattern relative to the other 20+ files
already using it.

## 2. "Buy" option: delete/skip the test instead of fixing it

Read the full file (`web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`, 430
lines, 11 tests across 6 `describe` blocks) to check coverage overlap for
`handles alternating text and control codes` (lines 251–291) against its 7 siblings:

| Test | Shape it covers that this test doesn't |
|---|---|
| `handles 500KB plain text without crash` | Plain text only, no `EscapeSequenceParser`, no `await`-per-write (fire-and-poll pattern) |
| `handles 1MB with ANSI color codes` | ANSI color only (single escape-sequence family), fire-and-poll pattern |
| `handles 10000 small writes with batching` | No parser at all, plain fixed-format lines |
| `handles Claude Code style animations` | Single repeated escape pattern (`\x1b[2K\r...`), 100 iterations, real 16ms inter-iteration delay (simulates frame pacing, not incidental) |
| `handles cursor positioning sequences` | Single escape family (`\x1b[{row};{col}H`), 1000 iterations |
| `watermark correctly tracks pending vs completed bytes` | Watermark bookkeeping in isolation, no parser |
| `multiple small writes below HIGH_WATERMARK never pause` | Sub-threshold behavior only |
| `pause/resume cycles work correctly` | Explicit multi-cycle pause/resume with real inter-cycle delay (delay is meaningful — models drain time) |
| `handles partial sequences at chunk boundaries gracefully` | Parser buffering only, tiny (5 chunks) |
| `recovers from parser reset during high load` | `parser.reset()` mid-stream behavior, 100 iterations |

The failing test is the **only** one that interleaves three distinct escape-sequence
families (color / cursor-up / plain text) round-robin across 5000 iterations in a single
run, and it's one of only two 1000+ iteration tests exercising the parser (`cursor
positioning sequences` is the other, at 1000 vs. this test's 5000 — a 5x difference in
scale). No sibling test reproduces this specific mixed-content, high-iteration-count shape.
Deleting it would drop coverage of `EscapeSequenceParser.processChunk` under a
realistically heterogeneous, sustained load — a scenario distinct enough (per the
requirements doc's own AC2) that the fix is required to preserve it, not retire it.

**Verdict: do not delete.** The test has an identifiable, non-redundant purpose. This isn't
a duplicate stress test; skipping it purely to dodge a fixable timing bug in test
scaffolding removes real signal for a Jest-scaffolding change, not a two-line justification.

## 3. Hand-rolled polling/advance loop vs. Jest's documented async timer APIs

The requirements doc's own root-cause section identifies the failure mode precisely: the
test `await`s a **real** `setTimeout(fn, 1)` per iteration, 5000 times, so wall-clock
scheduling slippage under CPU contention accumulates linearly past the fixed 15000ms Jest
timeout.

Two ways to remove the real-timer dependency:

- **Hand-rolled**: e.g. a custom `Promise`/microtask-flushing loop, a manual
  `process.nextTick` spin-wait, or reducing `HIGH_WATERMARK`/iteration count to shrink the
  real-time floor. These either reimplement (worse and less-tested) what Jest's fake timer
  engine already does, or weaken AC2 (same iteration count/shape) to work around the
  problem rather than removing the real-timer dependency per AC1.
- **Jest's built-in API**: wrap the test body in `jest.useFakeTimers({ doNotFake: [...] })`
  (or full fake timers if nothing else in the test needs real time) and replace the
  per-iteration `await new Promise(...)` timer wait with
  `await jest.advanceTimersByTimeAsync(1)` (matching `WatermarkTracker`'s hardcoded 1ms
  delay) or a single `await jest.runAllTimersAsync()` per iteration. This is the documented,
  first-party mechanism Jest ships specifically for "deterministic async code driven by
  `setTimeout`" — it collapses the ~5000ms+ real-time floor to near-zero and removes any
  wall-clock dependency entirely, satisfying AC1 exactly.

**Recommendation: use Jest's built-in `advanceTimersByTimeAsync`/`runAllTimersAsync`, not a
hand-rolled advancement loop.** It's already the pattern used 20+ times elsewhere in this
codebase (see §1), it's the documented, actively-maintained upstream implementation (Jest
30 / `@sinonjs/fake-timers` under the hood) rather than an LLM-authored substitute, and it
requires no new test-scaffolding code beyond the `useFakeTimers()`/`advanceTimersByTimeAsync`
calls themselves.

## 4. Verdict matrix

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **Fake timers via built-in Jest API** (`useFakeTimers` + `advanceTimersByTimeAsync`/`runAllTimersAsync`) | Removes real-timer dependency entirely (AC1); preserves exact iteration count/shape and assertions (AC2/AC3); zero new dependencies; matches 20+ existing precedents in this repo; generalizes cleanly to the two other real-timer-serialized tests named in AC4 (`Claude Code style animations`, `cursor positioning sequences`) | Requires care with any *intentionally* real delays in the same test (e.g. `handles Claude Code style animations`'s 16ms frame-pacing sleep is meaningful pacing, not incidental — needs to stay real or be advanced deliberately, not swallowed by a blanket `useFakeTimers()`) | **Recommended** |
| **Raise the timeout** (e.g. 15000ms → 30000/60000ms) | Trivial one-line change | Doesn't fix the root cause — a real-timer serialization chain with *no* slack; under sufficiently severe contention (CI runners, parallel suites) the new ceiling can still be exceeded; violates AC1 explicitly ("must not depend on real wall-clock timer scheduling to determine pass/fail") | **Not recommended** |
| **Rewrite to a throughput/completion assertion** (e.g. fire all writes without awaiting each one, poll via `setInterval` like the two "Large Volume" tests already do) | Also removes the real-timer-per-iteration serialization; reuses an already-proven pattern in the same file | Still leaves a real-timer poll loop (smaller risk, not zero); changes the test's control flow more than the minimal fake-timer swap; slightly diverges from AC4's framing of generalizing the *same* fix pattern across the three affected tests, since the other two don't use the poll pattern today | **Viable** (secondary option if fake timers hit an unforeseen blocker with `WatermarkTracker`'s async shape) |
| **Delete the test** | Zero maintenance cost going forward | Loses real, non-duplicated coverage of `EscapeSequenceParser.processChunk` under mixed-content, 5000-iteration load (see §2) — no sibling test covers this shape; contradicts AC2/AC3 which require preserving equivalent coverage and assertion strength | **Not recommended** |
