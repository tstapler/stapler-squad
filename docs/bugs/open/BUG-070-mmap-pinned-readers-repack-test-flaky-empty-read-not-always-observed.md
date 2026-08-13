# BUG-070: `TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack` fails intermittently when the already-unmapped guard path is never exercised [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-12
**Impact**: `session/unfinished/gogitstore` test suite fails nondeterministically (`go test ./session/... -run TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack -count=3` reproduced 3/3 failures in one run) whenever the real concurrent repack in the stress run doesn't happen to race a reader against an already-unmapped generation before all readers finish.

## Problem Description

The test builds an 80-commit fixture, spins up concurrent readers pinning a `lockedIndex` handle while a background goroutine drives a real `git gc`/repack, and requires that during the run it observed **both**:
- `sawFullRead` — a reader completing a full, correct read against a still-live (not-yet-retired) generation, and
- `sawEmptyRead` — a reader hitting the already-unmapped guard path (a clean empty read after the generation was unmapped).

Both are `atomic.Bool` flags set opportunistically inside the reader goroutines (`mmap_stage2_test.go:418-707`) depending on exactly when, relative to the real repack's completion, each reader happens to run. The final assertions:

```go
if !sawFullRead.Load() {
    t.Error("no reader ever observed a full, correct read — the pre-retire/still-pinned path was never exercised")
}
if !sawEmptyRead.Load() {
    t.Error("no reader ever observed a clean empty read after unmap — the already-unmapped guard path was never exercised")
}
```

require that the actual test run's race-timing produced *both* interleavings at least once. Observed failures were consistently `sawEmptyRead` never firing — i.e. the real `git gc` repack + retire + unmap sequence didn't complete early enough, relative to reader scheduling, for any reader to observe the post-unmap state before `readerWG.Wait()` returned.

This is a "coverage completeness" self-check (confirming the test itself is meaningful, not a correctness assertion about production code) but it's wired as a hard `t.Error`, so it fails the test run whenever the specific interleaving it wants to confirm didn't happen to occur — which depends on real subprocess (`git gc`) timing relative to goroutine scheduling, both of which are inherently non-deterministic on a loaded local machine.

## Reproduction Steps

```
go test ./session/unfinished/gogitstore/... -run TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack -count=3 -v
```

Reproduced 3/3 (all three iterations failed) in one run, always on the same assertion:

```
mmap_stage2_test.go:708: no reader ever observed a clean empty read after unmap — the already-unm...
```

## Root Cause

Not fully isolated — same class of "real-subprocess-timing-dependent test" already documented at length elsewhere in this package (see BUG-069, and this file's own extensive doc comments around `git gc`/`maintenance.auto` races). The test's correctness-of-implementation assertions (`retiring`, `pins == 0`, `unmapped`) all passed in every observed failure — only the two "did we exercise both code paths" self-check flags failed, and only `sawEmptyRead`. This suggests the real repack (via `git gc`, a subprocess call) is not reliably completing early enough relative to the reader goroutines' fixed work loop for the already-unmapped guard path to be hit before `readerWG.Wait()` unblocks.

## Files Likely Affected

- `session/unfinished/gogitstore/mmap_stage2_test.go` — `TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack`, `sawFullRead`/`sawEmptyRead` self-check assertions (`:700-708`).

## Fix Approach

Not attempted in this session (out of scope — prototype package, same as BUG-069). Candidates, cheapest first:
- (a) Retry the whole stress loop (readers + repack) up to N times if either self-check flag is still unset, instead of failing on the first pass — turns "did this exact run happen to interleave both ways" into "does this mechanism reliably produce both interleavings across a bounded number of attempts."
- (b) Increase reader iteration count/duration so more real-repack completions can land within the window, making it far more likely at least one reader observes the post-unmap state.
- (c) Inject a synchronization point (e.g. have the repack goroutine signal readers once the retire+unmap has actually completed) so at least one more reader iteration is guaranteed to run against the post-unmap state deterministically, rather than relying on wall-clock racing.

## Verification

`go test ./session/unfinished/gogitstore/... -run TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack -count=5 -v` passes on all 5 iterations, repeatably (run 2+ times to confirm no regression to the old flake rate).

## Related Tasks

Discovered while continuing the `RepairCorruptedGitRepo`/worktree-creation-failure investigation (`/errors/` page triage) and cross-checking `session/unfinished/gogitstore` after BUG-069 was filed for a different test in the same package. Confirmed **unrelated** to `RepairCorruptedGitRepo` (`session/repo_path.go`) — no file overlap. Also confirmed unrelated to the concurrently-checked-out `backlog/stapler-squad-bound-mmap-crash-subprocess-timeout` worktree branch — that branch's tip (`fa5cf8ea8`) is already an ancestor of `main` and its diff against `main` is empty, i.e. its work already landed and does not touch this test. Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than silently re-excused as "known flake."
