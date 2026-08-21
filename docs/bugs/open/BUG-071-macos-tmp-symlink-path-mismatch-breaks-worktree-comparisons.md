# BUG-071: macOS `/tmp` → `/private/tmp` symlink causes path-string mismatches in worktree comparisons across 3 `server/services` tests [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-14
**Impact**: 3 tests in `server/services` fail deterministically on macOS (confirmed reproducing on `main`, unrelated to any in-flight diff): `TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession`, `TestSessionRetentionSweeper_SkipsDirtyWorktree`, `TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound`.

## Problem Description

`t.TempDir()` on macOS returns a path under `/var/folders/...`, but macOS's `/tmp` (and by extension `/var`) is itself a symlink to `/private/var`. Some code path in the worktree/dirty-check/shared-worktree comparison logic resolves the real (symlink-free) path — e.g. via `git` invocations that report `/private/var/folders/...`, or `filepath.EvalSymlinks` — while the test's own recorded expectation still holds the raw, unresolved `/var/folders/...` string. The two get compared as plain strings and never match, so the code under test takes the "not equal / not found" branch even though the paths refer to the same directory.

## Reproduction Steps

1. `go test ./server/services/... -run 'TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession|TestSessionRetentionSweeper_SkipsDirtyWorktree|TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound' -v -timeout 60s` on macOS (Darwin) — all 3 fail deterministically.
2. Confirmed reproducing identically on `main` (checked out `main`'s copies of `server/services/backlog_service_test.go` and `server/services/session_retention_sweeper_test.go` into an otherwise-unrelated branch and re-ran) — not caused by any in-flight diff.
3. `TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession` (`backlog_service_test.go:3376`) fails with an explicit `/var/...` vs `/private/var/...` string diff — the clearest direct evidence of the root cause.
4. `TestSessionRetentionSweeper_SkipsDirtyWorktree` (`session_retention_sweeper_test.go:143`) and `TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound` (`session_retention_sweeper_test.go:233`) fail with plain `Should be true` assertions — consistent with the same underlying path mismatch causing the sweeper's dirty-check / shared-worktree-path lookup to silently treat two references to the same directory as different, and skip the "retain" branch. Not yet confirmed which specific comparison line does the mismatched lookup in the sweeper code — needs a debug print of both path strings at the comparison site to nail down exactly (see Fix Approach).

## Root Cause

Likely (not yet 100% confirmed for the two sweeper tests, but consistent with the confirmed case above): a raw `t.TempDir()`-derived path string is compared against a path that has been symlink-resolved somewhere in the git-worktree/dirty-check code path, and macOS's `/var` → `/private/var` symlink makes an otherwise-identical directory reference fail a plain `==`/`assert.Equal` string comparison.

## Files Likely Affected

- `server/services/backlog_service_test.go:3376` — direct evidence, `SpawnSessionFromItem`'s creator-call path vs. the triage-time `pool.firstCall().workDir`.
- `server/services/session_retention_sweeper_test.go:143,233` — the dirty-worktree and shared-worktree-with-sibling checks in the retention sweeper's implementation (file not yet identified — likely `server/services/session_retention_sweeper.go`).

## Fix Approach

Not yet investigated. Likely one of:
- Normalize both sides of the comparison through `filepath.EvalSymlinks` (or equivalent) before comparing, at whichever call site(s) do the mismatched string comparison.
- If the mismatch originates from `t.TempDir()` itself, wrap it in test helpers with `filepath.EvalSymlinks(t.TempDir())` in the affected tests only (narrower fix, doesn't touch production code, but risks the same bug recurring in a future test that doesn't remember to do this).

## Verification

All 3 tests pass reliably on macOS after the fix; unaffected on Linux CI (which doesn't have this symlink).

## Related Tasks

Discovered while verifying `go test ./server/services/... -timeout 20m` for the `TestDeleteSession_PublishesDeletedEvent`/`server/services` goroutine-leak-timeout backlog item — confirmed unrelated to that fix (same failures reproduce identically on `main`). Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than silently re-excused as "known pre-existing, unrelated"; not fixed in that session because root-causing the exact comparison site in `session_retention_sweeper.go` and validating a normalization fix across 3 call sites in 2 unrelated files would have meaningfully expanded that change's blast radius beyond its own scope (`server/services/session_service.go` + tests).
