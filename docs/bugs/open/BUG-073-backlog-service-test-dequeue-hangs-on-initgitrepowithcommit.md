# BUG-073: `TestDequeueNextQueuedItems_should_AutoSpawnReadyItem_When_SlotFreeAndConfigDefault` hangs in `initGitRepoWithCommit`, blowing the full-package test timeout [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-14
**Impact**: A full, unscoped `go test ./server/services/...` run (no `-run` filter) hit the default 180s-class timeout and dumped goroutine stacks centered on this test, blocked inside `initGitRepoWithCommit` (`backlog_service_test.go`) — a real `git` subprocess call used as test fixture setup. Contributes to the same symptom class as [[BUG-067]] (package-level CI timeout), but the specific stuck call site here is a `git` subprocess invocation in fixture setup, not general suite accumulation.

## Problem Description

While verifying an unrelated `server/services/session_service*.go` change (goleak/timeout test fixes, PR #503), a full-package `go test ./server/services/...` run timed out after 180s. The goroutine dump pointed at `TestDequeueNextQueuedItems_should_AutoSpawnReadyItem_When_SlotFreeAndConfigDefault` blocked inside `initGitRepoWithCommit`. Scoped runs (`-run 'TestShutdown|TestDestroyWithTimeout'`) do not hit this and pass cleanly and quickly, so this is isolated to this specific test/fixture, not the package-wide slowdown BUG-067 already describes.

## Reproduction Steps

1. `go test ./server/services/... -timeout 180s -count=1` in this sandboxed dev environment — hits the timeout with a goroutine dump naming `TestDequeueNextQueuedItems_should_AutoSpawnReadyItem_When_SlotFreeAndConfigDefault` / `initGitRepoWithCommit`.
2. Not yet confirmed whether this reproduces on `main` in isolation (i.e. with only this one test run via `-run`), nor whether it's specific to this sandbox (e.g. a slow/contended `git` binary, filesystem, or lack of a global git config causing `git commit` to hang on an interactive prompt) versus a genuine hang in CI too.

## Root Cause

Not yet investigated. `initGitRepoWithCommit` shells out to `git` (likely `git init`/`git commit`) as fixture setup; a `git commit` invoked without `user.name`/`user.email` configured, or without `-m` properly quoted, can hang waiting on an editor/prompt in some environments — a plausible but unconfirmed hypothesis.

## Files Likely Affected

- `server/services/backlog_service_test.go` — `initGitRepoWithCommit` helper and `TestDequeueNextQueuedItems_should_AutoSpawnReadyItem_When_SlotFreeAndConfigDefault`.

## Fix Approach

Not yet investigated. Likely candidates once root-caused:
- Ensure the `git commit` invocation always passes `-m` and runs with `GIT_AUTHOR_NAME`/`GIT_AUTHOR_EMAIL` (or `--author`) set so it never falls back to an interactive editor.
- Add an explicit timeout/context to the subprocess call per `.claude/rules/prefer-go-git-over-subshells.md` — or migrate the fixture to `go-git` entirely, consistent with that rule.

## Verification

`go test ./server/services/... -timeout 180s -count=1` completes without hanging or dumping goroutine stacks; the specific test passes reliably across repeated runs.

## Related Tasks

Found while verifying PR #503 (`fix(services): eliminate deleteCleanupWG Add/Wait race, widen cleanup timeout`) — confirmed unrelated to that PR's scope (`backlog_service_test.go` is untouched by it). Not fixed in that session per `.claude/rules/fix-flaky-tests-dont-defer.md`'s blast-radius exception: root-causing a `git` subprocess hang in unrelated fixture code is out of scope for a PR limited to `server/services/session_service.go` and its tests. See also [[BUG-067]] for the broader package-timeout symptom this may be contributing to.

**2026-08-21 update**: Fixed by a separate backlog item (branch `backlog/stapler-squad-migrate-git-fixtures-to-go-git`), which replaced `initGitRepoWithCommit`'s `git` CLI subprocess calls with `go-git` (`server/services/git_fixture_test.go`), eliminating the subprocess-hang failure mode entirely rather than diagnosing the specific hang cause (config prompt vs. quoting vs. contention) — go-git has no external-binary-availability or interactive-prompt failure mode the way subprocess `git` does. Confirms the root-cause hypothesis this doc had left "not yet investigated": a real `git` subprocess call in fixture setup, not the test logic itself.
