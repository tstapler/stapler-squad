# BUG-058: `pushAndCreatePR`/`shipViaAgentOrFallback` Split PR-Field Write and Status Transition Into Two Non-Atomic Calls [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-03, during independent review of PR #333 (fix for the lost-update race in `SetBacklogItemPRAndTransition`, `session/storage.go`). Not caused by that PR's diff — this is a pre-existing shape found while reviewing the fix, in a different code path.
**Impact**: Low. No confirmed data-integrity bug (see "Why this is lower risk than BUG-333" below) — filed as a fast-follow per this repo's convention of documenting adjacent findings rather than silently ignoring them (see `.claude/rules/fix-flaky-tests-dont-defer.md` for the general pattern this generalizes to non-test findings).

## Problem Description

`session/backlog_lifecycle.go` has two call sites with the same general shape PR #333 fixed in `SetBacklogItemPRAndTransition`: an `UpdateBacklogItem` call with `precondition: nil` (an unconditional field write) followed by a *separately* CAS-guarded status transition, rather than a single atomic write.

1. **`shipViaAgentOrFallback`**, `session/backlog_lifecycle.go:3326-3335`: writes `PrURL`/`PrNumber` unconditionally, then calls `resolveToPRPending` (line 3335) as a separate, later call.
2. **`pushAndCreatePR`**, `session/backlog_lifecycle.go:3482-3491` (field write) and `:3529` (`resolveToPRPending` call): same shape — field write, then several best-effort side calls (`EnablePRAutoMerge`, `RequestCopilotReview`), then the transition.

## Why This Is Lower Risk Than the Bug Fixed in PR #333

`SetBacklogItemPRAndTransition`'s original bug had two racing **external** callers (`report_pr_created`, an MCP tool invoked by agent sessions) that could genuinely interleave. These two call sites are reconciler-driven — `BacklogLifecycleListener` methods invoked from a single sequential reconcile loop, not exposed to concurrent external racers the way the MCP tool is. So the specific lost-update shape (two different PR numbers racing) is unlikely here.

There is also a structural difference in ordering that matters for the *other* risk PR #333's review surfaced (the `pr_pending_no_pr` / BUG-040 stuck-detector false positive): both call sites here write the PR fields **before** attempting the transition, and bail out (stay in `review`, notify) if that field write fails. So — unlike the pre-fix `SetBacklogItemPRAndTransition`, which could transition to `pr_pending` before the field write — these two sites can never produce `status=pr_pending && PrNumber==0` from their own field-write failure path. The remaining non-atomicity is a narrower window: a reader could observe `PrNumber` already set while `status` is still `review`, which is not the shape any current detector treats as an alert condition.

## Suggested Fix (not done here — out of scope for PR #333)

If ever revisited, the same primitive added in PR #333 — `EntRepository.TransitionBacklogItemStatusWithPRFields` (`session/ent_repository_backlog.go`), a single atomic `UPDATE ... WHERE` — could likely be reused directly at both call sites instead of the current two-call sequence, closing even the narrower window described above. Check whether `resolveToPRPending`'s precondition/notification semantics (referenced at both call sites) are compatible with that primitive's signature before doing so — they may need their own small extension (e.g. carrying the `resolveToPRPending`-specific "may just be a harmless race with `RecordPRCreatedOutOfBand`" no-op handling) rather than a drop-in swap.

## Related

- PR #333: https://github.com/tstapler/stapler-squad/pull/333 (the fix this finding was surfaced during review of)
- `docs/bugs/fixed/` — BUG-040 (the original silent-PR-field-write-failure bug both call sites' comments reference)
