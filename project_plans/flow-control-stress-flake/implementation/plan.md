# Implementation Plan: flow-control-stress-flake

**Feature**: Remove the real-timer serialization that makes `handles alternating text and control codes` (and its two `Control Code Heavy Output` siblings) intermittently exceed their Jest timeouts under CPU contention, by scoping `jest.useFakeTimers()` to the affected `describe` blocks and racing each write-completion promise against `jest.advanceTimersByTimeAsync(1)`.
**Date**: 2026-08-29
**Status**: Ready for implementation
**ADRs**: None — standard, already-adopted toolchain API (Jest's own built-in fake timers, already used in 20+ files in this repo), no non-standard technology choice.

---

## Domain Glossary
N/A — complexity 1, no new domain types.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Timer control in `Mixed Content Stress` describe (line 250) | `jest.useFakeTimers()` scoped via a local `beforeEach`/`afterEach` placed *inside* the describe block; per-iteration `await Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)])` | `research/stack.md` Option A; `research/build-vs-buy.md` verdict matrix (§4, "Recommended") | Raise `testTimeout` (15000ms → 30000ms+) | `research/pitfalls.md` §2: root cause is unbounded per-timer slippage under contention with no upper bound — no timeout value is provably safe, and it explicitly fails AC1 ("must not depend on real wall-clock timer scheduling") |
| Timer control (same) | (same) | (same) | `jest.useFakeTimers({ advanceTimers: true })` auto-advance mode | `research/stack.md` §1/§4: reintroduces a small but real ~20ms/tick wall-clock coupling; AC1's literal wording rules out *any* wall-clock dependency; no existing precedent for `advanceTimers: true` anywhere in this repo (`research/stack.md` §2) |
| Timer control (same) | (same) | (same) | Rewrite to a relative-throughput/completion assertion (reuse the `Large Volume Tests` fire-and-poll pattern) | `research/pitfalls.md` §3: the comparison signal is still real-clock-based (doesn't satisfy AC1); introduces new baseline-variance flakiness; weakens AC3's "must still fail on regression" guarantee |
| Timer control (same) | (same) | (same) | Delete the test | `research/build-vs-buy.md` §2: the test is the only one covering a 5000-iteration, three-way-interleaved (color/cursor/plain-text) load against `EscapeSequenceParser.processChunk` — no sibling test reproduces this shape; contradicts AC2/AC3 |
| 60fps pacing delay in `handles Claude Code style animations` (line 216) | Fake-advance it too via `jest.advanceTimersByTimeAsync(16)`, collapsing it to instant | Decision made in this plan, resolving requirements.md AC4's explicit ambiguity note | Leave that one delay on a real timer while faking everything else in the same test | The test's assertions (`completed > 0`, `parser.getBuffered() === ''`, lines 219-220) never measure elapsed wall-clock time or frame cadence — collapsing the delay changes nothing the test verifies. Mixing one real timer into an otherwise-faked test reintroduces exactly the per-timer real-wall-clock coupling AC1 exists to eliminate, and doubles the deadlock-interleaving surface for zero verification benefit. |

---

## Migration Plan
N/A — complexity 1, test-only change.

## Observability Plan
N/A — complexity 1, test-only change, no runtime/production impact.

## Risk Control
N/A — complexity 1, test-only change; standard revert via PR close + revert commit if needed.

## Unresolved Questions
None.

## Dependency Visualization

```
1.1.1a (scope fake timers: "Mixed Content Stress" beforeEach/afterEach, after line 250)
   │
   ▼
1.1.1b (race pattern: primary write-await, lines 274-279)
   │
   ▼
1.1.1c (fake-advance the periodic yield, lines 283-285)
   │
   ▼
1.1.2a (scope fake timers: "Control Code Heavy Output" beforeEach/afterEach, after line 189)
   │
   ├──────────────┐
   ▼              ▼
1.1.2b          1.1.2c
(animations:    (cursor positioning:
 race pattern    race pattern,
 lines 207-212,  lines 237-242)
 + advance 16ms
 at line 216)
   │              │
   └──────┬───────┘
          ▼
1.1.3a (repeat isolated-file test runs, AC5 baseline)
          │
   ┌──────┴───────┐
   ▼              ▼
1.1.3b          1.1.3c
(full-suite     (contention repro:
 regression,     run under concurrent
 AC6)            CPU-heavy work, AC5)
```

---

## Phase 1: Fix real-timer serialization in flow-control-stress.test.ts

### Epic 1.1: Fix real-timer serialization in flow-control-stress.test.ts
**Goal**: Replace every real-`setTimeout`-await in the file's two most timer-serialized nested `describe` blocks (`Mixed Content Stress`, `Control Code Heavy Output`) with scoped `jest.useFakeTimers()` + `jest.advanceTimersByTimeAsync(...)`, so pass/fail no longer depends on real wall-clock timer scheduling, while leaving `WatermarkTracker`, `EscapeSequenceParser`, iteration counts, and assertions untouched, and leaving the file's four other `describe` blocks (which never call any timer-advance API) completely unaffected.

#### Story 1.1.1: Fix the primary failing test with scoped fake timers
**As a** developer running the Jest suite, **I want** `handles alternating text and control codes` to stop depending on 5000 real macrotask round-trips, **so that** the test stops flaking under CPU contention (e.g. concurrent `pnpm install`/`make proto-gen`) while still exercising the same parser/tracker code path.

**Acceptance Criteria** (requirements.md AC1, AC2, AC3):
- AC1: No code path in the test awaits a real, un-advanced `setTimeout` callback.
  - *Given* the `Mixed Content Stress` describe block has `jest.useFakeTimers()` active via its local `beforeEach`, *When* the test loop executes `await Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)])` for each of the 5000 iterations, *Then* the test's total execution time is dominated by synchronous parser/string work, not by real timer-scheduling latency, and the test can complete correctly even if the fake clock is never allowed to run in real time.
- AC2: Same iteration count/shape and same code under test.
  - *Given* the fix only replaces the timer-waiting mechanism (lines 274-279, 283-285) and leaves `iterations = 5000` (line 255), the chunk-construction `if/else if/else` (lines 259-270), and `parser.processChunk(chunk)` (line 272) untouched, *When* the modified test runs, *Then* it still calls `EscapeSequenceParser.processChunk` exactly 5000 times with the same color/cursor-movement/plain-text round-robin mix, and still drives `WatermarkTracker.write()`'s pause/resume bookkeeping through the same code path.
- AC3: Assertions still catch a real regression.
  - *Given* a hypothetical regression is introduced into `WatermarkTracker.write()` (e.g., the `this.watermark = Math.max(0, this.watermark - data.length)` decrement inside the `setTimeout` callback is deleted, so `watermark` only ever grows), *When* the modified `handles alternating text and control codes` test runs, *Then* `expect(metrics.watermark).toBeLessThan(50000)` (line 290) still fails, because the assertion targets the tracker's actual computed state — the fake-timer swap changes only how the write-completion callback is awaited, not what state is asserted. (Corrected per `adversarial-review.md`: the originally-cited `LOW_WATERMARK`-check deletion doesn't perturb `watermark` at all, since the decrement runs unconditionally — it would not have made this assertion fail.)

**Files**: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

##### Task 1.1.1a: Scope fake timers to the `Mixed Content Stress` describe block (~3 min)
- Immediately after `describe('Mixed Content Stress', () => {` (line 250), add:
  ```ts
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });
  ```
- Do not add this at the outer `describe('Flow Control Stress Tests', ...)` (line 12) or any other nested describe — the other four nested describes (`Large Volume Tests`, `Rapid Small Writes`, `Watermark Behavior`, `Error Recovery`) rely on real `setInterval`/`setTimeout` firing on their own and call no timer-advance API.
- Files: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

##### Task 1.1.1b: Replace the primary write-await with the race pattern (~4 min)
- Replace lines 274-279:
  ```ts
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
  ```
  with:
  ```ts
          const writeDone = new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
          await Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)]);
  ```
- Files: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

##### Task 1.1.1c: Fake-advance the periodic yield instead of a real setTimeout(0) (~2 min)
- Replace lines 283-286 (corrected per `adversarial-review.md`; was mislabeled 283-285):
  ```ts
          // Yield occasionally
          if (i % 100 === 0) {
            await new Promise(resolve => setTimeout(resolve, 0));
          }
  ```
  with:
  ```ts
          // Yield occasionally
          if (i % 100 === 0) {
            await jest.advanceTimersByTimeAsync(0);
          }
  ```
  (Once fake timers are active from Task 1.1.1a, the original real `setTimeout(resolve, 0)` would never fire on its own and would deadlock the test; `advanceTimersByTimeAsync(0)` still flushes the microtask queue, preserving the yield's purpose without any real-timer dependency.)
- Files: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

#### Story 1.1.2: Generalize the fix to the two Control Code Heavy Output tests (AC4)
**As a** developer running the Jest suite, **I want** `handles Claude Code style animations` and `handles cursor positioning sequences` to use the same scoped-fake-timer mechanism, **so that** the fix isn't a one-off special case for line 251 and the lower-probability flake risk already named in requirements.md (100/1000-iteration variants of the same pattern) is closed in the same pass.

**Acceptance Criteria** (requirements.md AC4):
- *Given* `Control Code Heavy Output`'s two tests (`handles Claude Code style animations`, lines 190-221; `handles cursor positioning sequences`, lines 223-247) also await a real per-iteration `setTimeout(1)` via `WatermarkTracker.write()`, *When* the same scoped-fake-timers + `Promise.all` race pattern from Story 1.1.1 is applied to both (plus `jest.advanceTimersByTimeAsync(16)` for the animation test's frame-pacing delay at line 216), *Then* both tests pass deterministically with zero real-wall-clock dependency, generalizing the primary fix's mechanism rather than leaving it as a documented follow-up.

**Files**: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

##### Task 1.1.2a: Scope fake timers to the `Control Code Heavy Output` describe block (~3 min)
- Immediately after `describe('Control Code Heavy Output', () => {` (line 189), add:
  ```ts
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });
  ```
- Files: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

##### Task 1.1.2b: Fix `handles Claude Code style animations` (~4 min)
- Replace lines 207-212 (write-await block):
  ```ts
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
  ```
  with:
  ```ts
          const writeDone = new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
          await Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)]);
  ```
- Replace lines 215-216 (corrected per `adversarial-review.md`; was mislabeled as line 216 only):
  ```ts
        // Simulate 60fps animation
        await new Promise(resolve => setTimeout(resolve, 16));
  ```
  with:
  ```ts
        // Simulate 60fps animation (fake-advanced — assertions don't measure real elapsed time; see Pattern Decisions)
        await jest.advanceTimersByTimeAsync(16);
  ```
- Files: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

##### Task 1.1.2c: Fix `handles cursor positioning sequences` (~3 min)
- Replace lines 237-242:
  ```ts
          await new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
  ```
  with:
  ```ts
          const writeDone = new Promise<void>((resolve) => {
            tracker.write(safeChunk, () => {
              completed++;
              resolve();
            });
          });
          await Promise.all([writeDone, jest.advanceTimersByTimeAsync(1)]);
  ```
- Files: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts`

#### Story 1.1.3: Verify no regression across the file and the full suite
**As a** developer merging this fix, **I want** confirmation that the fixed file passes repeatedly (including under contention) and that no other test in the suite regressed, **so that** the scoped `useFakeTimers()`/`useRealTimers()` change is proven not to have leaked into the file's four untouched `describe` blocks.

**Acceptance Criteria** (requirements.md AC5, AC6):
- AC5: *Given* the fully patched file, *When* `npx jest --testPathPatterns="flow-control-stress" --no-coverage` is run repeatedly (5+ consecutive runs) and at least once concurrently with another CPU-heavy background job (approximating the original repro's `pnpm install` + `make proto-gen` contention), *Then* every run passes with no timeout.
- AC6: *Given* the same patched file, *When* `cd web-app && npx jest --no-coverage` (the full suite) is run, *Then* no previously-passing test — including this file's other 8 tests in `Large Volume Tests`, `Rapid Small Writes`, `Watermark Behavior`, and `Error Recovery`, none of which call any timer-advance API — fails, hangs, or times out, confirming the fake-timer scoping did not leak outside the two touched describe blocks.

**Files**: `web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts` (no further edits — verification only)

##### Task 1.1.3a: Repeat isolated-file test runs (~3 min)
- Run:
  ```bash
  cd web-app && for i in 1 2 3 4 5; do npx jest --testPathPatterns="flow-control-stress" --no-coverage || break; done
  ```
- Confirm all 5 runs report 11/11 tests passing (the modified 3 plus the 8 untouched ones) with no timeout and no hang.
- Files: none (verification step)

##### Task 1.1.3b: Full-suite regression check (~5 min)
- Run:
  ```bash
  cd web-app && npx jest --no-coverage
  ```
- Confirm the full suite (4445+ tests per requirements.md's repro note) passes with no new failures relative to a pre-change baseline run.
- Files: none (verification step)

##### Task 1.1.3c: Contention repro matching the original flake conditions (~5 min)
- `make proto-gen` is stamp-file-guarded (`Makefile:397-407`) — by this point in implementation `node_modules` and generated protos already exist, so a second run is a near-instant no-op and would not reproduce real CPU contention (per `adversarial-review.md`). Use a synthetic CPU-load generator instead:
  ```bash
  cd web-app && npx jest --testPathPatterns="flow-control-stress" --no-coverage &
  for i in $(seq 1 "$(nproc)"); do yes > /dev/null & done
  wait %1
  kill $(jobs -p) 2>/dev/null
  ```
  (or `stress-ng --cpu "$(nproc)" --timeout 30s &` if `stress-ng` is available, as an alternative to the `yes` loop.)
- Confirm the foregrounded Jest run still passes despite the concurrent CPU load. This is a plan-level sanity check; `sdd:4-validate`'s `validation.md` is the authoritative place for the file's formal stress-repro procedure per requirements.md AC5.
- Files: none (verification step)
