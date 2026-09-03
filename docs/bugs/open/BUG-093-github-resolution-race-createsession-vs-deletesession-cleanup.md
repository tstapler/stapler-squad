# BUG-093: Data race between CreateSession's background GitHub-resolution pipeline and DeleteSession's cleanup path [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-02 (during `google-jules-integration`'s PR ship-loop, merging `origin/main` into
the feature branch — unrelated to that diff; surfaced by running the full `server/services` suite with
`-race` as a verification step after the merge)
**Impact**: Test-only observed so far, but the race is on genuinely shared production state (an
`*Instance`'s GitHub-resolution field), not just test scaffolding — worth root-causing rather than
dismissing as a test-only artifact. Once the race detector reports it, `go test`'s runtime aborts the
whole `server/services` test binary, which cascades into ~35 unrelated, otherwise-passing tests being
marked `FAIL` with `race detected during execution of test` as collateral (not 35 independent races —
confirmed via `grep -c "WARNING: DATA RACE"` = 1 for the whole run).

## Problem Description

`session.setGitHubResolutionLocked` (`session/instance_actor_setters.go:708`, called from
`(*Instance).SetGitHubResolution` via the async `runBackgroundResolutionPipeline` pipeline,
`server/services/session_creation_pipeline.go:187`) writes to an `*Instance` field with no
synchronization visible to the race detector, while `(*Instance).GetEffectiveRootDir`
(`session/instance_worktree.go:369`), called from `cleanupPartialCreation`
(`server/services/session_service.go:3448`) on a concurrent `DeleteSession` cleanup path
(`waitForDestroyLoggingSlowCleanup` → `DeleteSession.func1.1`), reads the same field without
coordinating with the writer.

## Reproduction Steps

1. `go test ./server/services/... -race` (full package, not scoped) — triggers within the first
   ~1.5 minutes of the run, in `TestCreateSession_GitHubURLResolution_NotBoundByRequestContext`
   (`server/services/session_service_create_test.go:412`).
2. Not yet confirmed reproducible via `-run TestCreateSession_GitHubURLResolution_NotBoundByRequestContext`
   in isolation — the race is between that test's own `CreateSession` background pipeline goroutine and
   a *separate* test's concurrent `DeleteSession`/cleanup goroutine (per the two goroutines' distinct
   creation stacks in the race report), so it may need `t.Parallel()` contention from neighboring tests
   in the same package to manifest, similar in shape to BUG-090/BUG-091's full-suite-load-only
   reproduction pattern.

## Root Cause (not yet confirmed)

**Hypothesis, not yet verified**: `SetGitHubResolution` (write side) and `GetEffectiveRootDir` (read
side) both touch the same `*Instance` field, but only the write side appears to go through
`sendSyncErr`/the actor's synchronization path (`session/actor.go:37`); `GetEffectiveRootDir` may be a
plain unsynchronized read not routed through the same actor-mailbox mechanism, or the two are
racing across independent `*Instance` values that happen to alias the same address after a session
resource is released/reused between `CreateSession` and a differently-scoped `DeleteSession` in a
neighboring parallel test. Needs direct inspection of `Instance`'s actor/locking model for this specific
field before concluding which.

## Files Affected

- `session/instance_actor_setters.go` (`setGitHubResolutionLocked`, `SetGitHubResolution`)
- `session/instance_worktree.go` (`GetEffectiveRootDir`)
- `session/actor.go` (`sendSyncErr`)
- `server/services/session_creation_pipeline.go` (`runBackgroundResolutionPipeline`)
- `server/services/session_service.go` (`trackCleanup`, `cleanupPartialCreation`, `waitForDestroyLoggingSlowCleanup`, `DeleteSession`)

## Fix Approach

1. Reproduce reliably in isolation first (per-package `-race -run ... -count=N`, or the full-suite
   pattern BUG-090/091 document) to rule out a test-only aliasing artifact before touching production
   locking.
2. If genuine: audit whether `GetEffectiveRootDir` needs the same actor-routed/locked access
   `SetGitHubResolution` uses, or whether the field itself needs a dedicated mutex/atomic if it's read
   from a goroutine that doesn't go through the actor mailbox by design (e.g. cleanup paths that must
   run even if the actor is torn down).
3. Follow `fix-flaky-tests-dont-defer` — this is a genuine race, not a timing-budget flake, so the fix
   belongs in the synchronization, not a longer timeout.

## Verification

After fix: `go test ./server/services/... -race -count=5` must not reproduce this race, and the ~35
tests currently observed failing as collateral (via the shared-binary abort, not independent races —
verify their own assertions still pass once the race no longer aborts the process) should return to
green.

## Related

- Discovered while verifying `google-jules-integration` (PR #674) post-merge — confirmed via
  `git log -- session/instance_actor_setters.go server/services/session_creation_pipeline.go` that no
  commit on that branch touches either file; this is inherited from `main`'s already-merged
  `async-session-creation` work (`project_plans/async-session-creation/`), not introduced by the Jules
  PR. Out of scope to fix there — filed here for visibility instead of silently ignored.
- BUG-090, BUG-091 — same `server/services` full-suite-load flake-discovery pattern, same standing rule
  (`fix-flaky-tests-dont-defer`) against re-excusing a known issue without filing it.
