# BUG-092: `TriggerHandoffSummary`'s dispatched goroutine can reach a terminal status before the synchronous read-back, under load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-28, PR #647's (`docs/diataxis-migration-and-rules-to-skills`) CI `Test` job, run [33146922127](https://github.com/tstapler/stapler-squad/actions/runs/33146922127/job/98770702345)
**Impact**: Intermittent CI failure on `go test ./server/services -race` with full-suite coverage load. Not reproducible locally (10/10 passes in isolation, including under `-race -count=10`).

## Problem Description

```
--- FAIL: TestTriggerHandoffSummary_SynthesizesPendingResponse_When_NoRowExistsYet (0.41s)
    handoff_summary_service_test.go:259:
        Error:      []sessionv1.HandoffSummaryStatus{1, 2} does not contain 4
        Test:       TestTriggerHandoffSummary_SynthesizesPendingResponse_When_NoRowExistsYet
```

The test's own doc comment already names the exact contract being checked: `TriggerHandoffSummary` dispatches `GenerateAndPersist` in a background goroutine without waiting for the interim `GENERATING` upsert to land before its own synchronous read-back, so the handler's response, the instant it returns, must report `PENDING` or `GENERATING` (values 1 or 2) — never a terminal status (`READY`/`ERROR`, value 4 here is `ERROR`) that early. Under this CI run's load, the dispatched goroutine reached `ERROR` (a fast, deterministic "conversation file not found" failure, since the test isolates `$HOME` to an empty temp dir specifically to make that goroutine fail fast) before the handler's own read-back ran — violating the "never terminal this early" contract the test exists to enforce.

## Confirmed unrelated to PR #647

PR #647's diff is a pure documentation/comment migration (moved `.claude/docs/*.md` files, converted `.claude/rules/*.md` to skills, repointed doc-comment references) — zero files under `session/handoff_summary*.go` or `server/services/handoff_summary_service*.go` were touched. Local verification on the same tree:
- `go test ./server/services/... -run TestTriggerHandoffSummary_SynthesizesPendingResponse_When_NoRowExistsYet -v -count=5` — 5/5 pass.
- `go test ./server/services/... -run TestTriggerHandoffSummary -v -count=10 -race` — 10/10 pass (all 3 subtests).

## Likely Cause

Not yet root-caused at the implementation level. The test's own doc comment states the intended invariant ("does not wait for the interim GENERATING upsert to land before its own read"), which implies the synchronous path is supposed to structurally win this race (e.g., the read-back happens before the dispatched goroutine is even scheduled, or there's an ordering guarantee elsewhere). If a real CI run can observe the dispatched goroutine already at a terminal status, either:
1. The "synchronous read wins" assumption is timing-dependent rather than structurally guaranteed (e.g., no synchronization point actually prevents the dispatched goroutine's fast failure path from completing and persisting before the caller's read), and only holds under low scheduling contention (a bare `go test -run` locally), not CI's heavier load; or
2. There's a genuine ordering bug in `TriggerHandoffSummary` (likely `server/services/handoff_summary_service.go` or `session/handoff_summary_generator.go` — not yet located precisely) where the dispatch-then-read ordering isn't actually enforced by anything.

## Files Likely Affected

- `server/services/handoff_summary_service_test.go:240-265` — the test itself.
- `server/services/handoff_summary_service.go` (`TriggerHandoffSummary`) and/or `session/handoff_summary_generator.go` (`GenerateAndPersist`) — wherever the dispatch-then-read ordering is (or isn't) actually enforced.

## Fix Approach

1. Read `TriggerHandoffSummary`'s implementation to find whether there's supposed to be a synchronization point (e.g., the interim `GENERATING` row write happening synchronously before the goroutine dispatch, vs. the goroutine itself writing the first row) that CI's timing is defeating.
2. If the "read wins" assumption was never actually guaranteed by code (just usually true under low contention), either add an explicit ordering guarantee (e.g., synchronously write the initial `GENERATING` row before dispatching the goroutine that can later overwrite it with a terminal status) or relax the test's contract to match what's actually guaranteed.
3. If a real synchronization bug is found, fix it and add a `-race`-safe regression test that can deterministically force the goroutine to finish before the read-back (e.g., inject a synchronization hook) to prove the fix without relying on real timing.

## Verification

After the fix: `go test ./server/services/... -run TestTriggerHandoffSummary -race -count=100` with zero failures, run both in isolation and interleaved with the rest of the `server/services` package under full `-race` load to better approximate CI's contention.

## Related

Logged per the `fix-flaky-tests-dont-defer` skill's standing rule rather than silently re-excused. Discovered incidentally while shipping an unrelated documentation migration PR — not fixed in that PR's scope since it requires reading and understanding the handoff-summary generation/dispatch ordering, which that PR never touches.
