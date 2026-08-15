# BUG-072: `destroyWithTimeout` bounds the wait, not the work — hung cleanup leaks an untracked goroutine [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-14
**Impact**: If `Instance.Destroy()` hangs (e.g. `git worktree remove` stuck on a locked index or a stale NFS mount), `destroyWithTimeout` returns at the configured timeout, but the goroutine actually running `Destroy()` keeps executing indefinitely — untracked by `deleteCleanupWG`. `Shutdown()` can therefore return (and the process can exit) while a worktree mutation is still in flight.

## Problem Description

`destroyWithTimeout` (`server/services/session_service.go:284-295`) launches `inst.Destroy()` on a goroutine and races it against a timer via `select`. This correctly bounds how long the *caller* waits, but does nothing to cancel the underlying work: `Instance.Destroy()` (`session/instance.go:1379`) takes no `context.Context`, and nothing beneath it — `KillSession` (`session/instance_tmux.go:399`), `CleanupWorktree` (`session/instance_worktree.go:264`) → `gitManager.Cleanup` (`session/git/worktree_ops.go:346`) — accepts or enforces one either. The abandoned goroutine is not added to `deleteCleanupWG`, so `Shutdown()`'s `Wait()` doesn't see it and returns as if cleanup finished.

This is a known, documented tradeoff (see the doc comment at `session_service.go:267-277`) rather than an oversight, but it means the goroutine-leak fix in this PR (the `deleteCleanupWG` Add/Wait race) only prevents leaking a *tracked* goroutine — a genuinely hung `Destroy()` still leaks an unbounded one.

## Fix Approach

Thread `context.Context` through the cleanup chain so a timeout can actually cancel the work, not just the wait:
- `Instance.Destroy(ctx context.Context)` (`session/instance.go:1379`)
- `KillSession` (`session/instance_tmux.go:399`) — already likely wraps a `safeexec.CommandContext`-style tmux kill; verify it accepts/propagates ctx
- `CleanupWorktree` / `gitManager.Cleanup` (`session/instance_worktree.go:264`, `session/git/worktree_ops.go:346`) — needs a context-aware `git worktree remove` invocation

Then `destroyWithTimeout` derives its context from `context.WithTimeout(ctx, deleteSessionCleanupTimeout)` and passes it down, so the timeout actually cancels the subprocess instead of merely abandoning the wait.

## Related Tasks

Found during code review of PR #503 (`fix(services): eliminate deleteCleanupWG Add/Wait race, widen cleanup timeout`). Not fixed in that PR per `.claude/rules/fix-flaky-tests-dont-defer.md`'s blast-radius exception — threading a new `context.Context` parameter through `Instance.Destroy` touches shared infrastructure (`session/instance.go`, `session/instance_tmux.go`, `session/instance_worktree.go`, `session/git/worktree_ops.go`) well beyond that PR's `server/services/session_service.go` scope.
