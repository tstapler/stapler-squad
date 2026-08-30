# Requirements: `flow-control-stress.test.ts` — 'handles alternating text and control codes' times out under system load

## Source

Backlog item `7cc29340-330b-413d-8e26-5418c550ccde`. This document was authored directly
from the backlog item's title, description, and repro notes; no interactive ideation
interview was run (non-interactive triage session). Filed as a side-discovery while working
an unrelated backlog item (`0ddc4edb-ae2e-4d85-b9cf-067af72be323`, `useFocusTrap` focus
return) — not fixed there, to avoid scope creep on that change.

## Complexity

**1 (quick task)** — single test file, one test case, no production code path involved, no
new dependency, no user-facing surface. Scoped research to Agents 1 (stack), 4 (pitfalls),
6 (build-vs-buy) per the `sdd:2-research` quick-task calibration rule.

## Problem Statement

`web-app/src/lib/terminal/__tests__/flow-control-stress.test.ts:251`, test `Flow Control
Stress Tests › Mixed Content Stress › handles alternating text and control codes`,
intermittently exceeds its 15000ms Jest timeout:

```
Flow Control Stress Tests › Mixed Content Stress › handles alternating text and control codes
Exceeded timeout of 15000 ms for a test.
```

### Repro

- Failed once running the full `npx jest --no-coverage` suite (4445+ tests) in `web-app/`
  immediately after `pnpm install` + `make proto-gen` on the same machine — i.e. under CPU
  contention from those two commands plus the rest of the concurrently-running suite.
- Re-running `npx jest --testPathPatterns="flow-control-stress"` alone (no contention)
  passed twice in a row.

### Root cause (diagnosed this triage pass, read directly from the test source)

The failing test body (lines 251–286) is fundamentally different from its sibling tests in
the same file, and the difference is the cause:

```ts
for (let i = 0; i < iterations; i++) {   // iterations = 5000
  // ...build chunk...
  const safeChunk = parser.processChunk(chunk);
  if (safeChunk.length > 0) {
    await new Promise<void>((resolve) => {
      tracker.write(safeChunk, () => {
        completed++;
        resolve();
      });
    });
  }
  if (i % 100 === 0) {
    await new Promise(resolve => setTimeout(resolve, 0));
  }
}
```

`WatermarkTracker.write()` (defined at the top of the same file, lines ~20–38) completes
its callback via a **real** `setTimeout(fn, 1)` — not a mock/fake timer. Because the loop
`await`s that callback on every iteration where `safeChunk.length > 0` (roughly all 5000,
since every generated chunk is non-empty), the test serializes ~5000 real macrotask
round-trips, each nominally 1ms, before it can finish — a ~5000ms floor before any actual
work or contention is counted, on top of the `i % 100` extra yields.

Node does not clamp `setTimeout` delays the way browsers do, but it also gives no
scheduling guarantee: under CPU contention (a concurrent `pnpm install`, `make proto-gen`,
and 4000+ other Jest tests competing for the event loop and CPU), each of those 5000 timer
call
backs can slip from ~1ms to several ms of real wall-clock delay before Node's timer
wheel and event loop get back to them. With 5000 sequential dependencies, small per-timer
slippage compounds linearly and can push total wall-clock time well past the fixed 15000ms
budget — with no logic error in the code under test.

This matches the item's own diagnosis ("a timing-dependent stress test with no slack") and
is consistent with the repro: isolated (uncontended) runs stay near the 5000ms floor and
pass comfortably; run alongside CPU-heavy concurrent work, the same 5000 timer round-trips
take longer in wall-clock terms and trip the fixed budget.

**Contrast with sibling tests in the same file** that stay stable under load:
- `handles 500KB plain text without crash` and `handles 1MB with ANSI color codes` fire all
  writes in a tight loop *without* awaiting each one, then poll for completion via a 10ms
  `setInterval` — they don't serialize on the real timer per-iteration.
- `handles Claude Code style animations` and `handles cursor positioning sequences` do await
  per-iteration like the failing test, but run far fewer iterations (100 and 1000 vs. 5000)
  and already carry a 10000ms budget for the smaller counts — same pattern, smaller
  multiplier, hasn't yet been observed to flip flaky (not evidence it's immune, just lower
  probability).

This is a timing-budget/test-design issue, not a defect in `WatermarkTracker`,
`EscapeSequenceParser`, or any production flow-control code.

## Acceptance Criteria

1. `handles alternating text and control codes` no longer serializes 5000 real
   `setTimeout`-based macrotask round-trips as its critical path — it must not depend on
   real wall-clock timer scheduling to determine pass/fail.
2. The test still exercises the same production code it does today at equivalent
   iteration count/shape (`EscapeSequenceParser.processChunk` across the same 5000-chunk
   text/color/cursor-movement mix; `WatermarkTracker`'s pause/resume/watermark bookkeeping).
3. The test's own assertions (`completed` count, final `watermark < 50000`) still hold and
   still fail if the underlying flow-control logic regresses (i.e. the fix must not make the
   test vacuously pass).
4. The fix generalizes to the file's other real-timer-serialized tests where practical
   (`handles Claude Code style animations`, `handles cursor positioning sequences`) rather
   than being a one-off special case for line 251 alone — or, if not generalized in this
   pass, the plan explicitly states why and files the follow-up.
5. `npx jest --testPathPatterns="flow-control-stress" --no-coverage` passes reliably,
   including when run concurrently with other CPU-heavy work (validated per
   `implementation/validation.md`'s stress-repro procedure).
6. No other test in the suite regresses (`cd web-app && npx jest --no-coverage`).

## Non-Goals

- Do not change `WatermarkTracker` or `EscapeSequenceParser` production-equivalent logic
  (`WatermarkTracker` is test-only scaffolding defined in this spec file, not shipped code;
  `EscapeSequenceParser` itself is out of scope for this fix — only how the test drives it).
- Do not touch the unrelated `useFocusTrap` focus-return item this was discovered alongside.
- Do not attempt a repo-wide audit of every real-timer-dependent test — scope is this file
  (see AC4 for the one explicit exception already identified).

## Open Questions

- None — root cause and fix direction are unambiguous from reading the test source; deferred
  to `sdd:3-plan` is only the choice of *mechanism* (fake timers vs. raised timeout vs.
  throughput-based assertion), which the research phase will resolve.
