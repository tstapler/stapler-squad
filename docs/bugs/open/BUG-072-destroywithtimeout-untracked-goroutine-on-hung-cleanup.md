# BUG-072: `destroyWithTimeout` bounds the wait, not the work — hung cleanup leaks an untracked goroutine [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-14
**Impact**: If `Instance.Destroy()` hangs (e.g. `git worktree remove` stuck on a locked index or a stale NFS mount), `destroyWithTimeout` returns at the configured timeout, but the goroutine actually running `Destroy()` keeps executing indefinitely — untracked by `deleteCleanupWG`. `Shutdown()` can therefore return (and the process can exit) while a worktree mutation is still in flight.

## Problem Description

`destroyWithTimeout` (`server/services/session_service.go:284-295`) launches `inst.Destroy()` on a goroutine and races it against a timer via `select`. This correctly bounds how long the *caller* waits, but does nothing to cancel the underlying work: `Instance.Destroy()` (`session/instance.go:1379`) takes no `context.Context`, and nothing beneath it — `KillSession` (`session/instance_tmux.go:399`), `CleanupWorktree` (`session/instance_worktree.go:264`) → `gitManager.Cleanup` (`session/git/worktree_ops.go:346`) — accepts or enforces one either. The abandoned goroutine is not added to `deleteCleanupWG`, so `Shutdown()`'s `Wait()` doesn't see it and returns as if cleanup finished.

This is a known, documented tradeoff (see the doc comment at `session_service.go:267-277`) rather than an oversight, but it means the goroutine-leak fix in this PR (the `deleteCleanupWG` Add/Wait race) only prevents leaking a *tracked* goroutine — a genuinely hung `Destroy()` still leaks an unbounded one. `deleteSessionCleanupTimeout` is set to 5s to match `KillTmuxSessionByTitle`'s cap on the underlying kill-session subprocess; because that timeout only bounds the wait (not the work), git-diff and worktree cleanup that legitimately takes longer than 5s on large repos will also hit this timeout and log a "still running in background" error even though nothing is actually hung — a further argument for the context-threading fix below rather than only widening the constant.

`Instance.Destroy()` (`session/instance.go:1379-1421`) runs `KillSession()` → (if lifecycle listeners are registered) `UpdateDiffStats()` → `CleanupWorktree()` sequentially, and the whole sequence executes inside the single outer 5s `destroyWithTimeout(liveInst.Destroy, deleteSessionCleanupTimeout)` window from `server/services/session_service.go`. `killSessionTimeout` (`session/tmux/tmux.go`), also 5s, caps the `kill-session` subprocess inside `KillSession()`'s underlying `TmuxSession.Close()` — but that 5s is *nested inside*, not parallel to, the outer 5s budget, since `KillSession()` is just the first of the three sequential steps `Destroy()` runs. Concretely: a `kill-session` subprocess that is merely slow (not hung) and takes close to its own 5s allowance can by itself consume the entire outer `deleteSessionCleanupTimeout` window, leaving little or no time left for `UpdateDiffStats()` or `CleanupWorktree()` to run before `destroyWithTimeout` gives up on the wait — even though nothing in the chain actually hung. This compounds the "only bounds the wait" problem above: the nesting means the effective budget for diff-stats capture and worktree cleanup is not really 5s, but "whatever remains of 5s after kill-session," which can be arbitrarily close to zero.

## Fix Approach

Thread `context.Context` through the cleanup chain so a timeout can actually cancel the work, not just the wait:
- `Instance.Destroy(ctx context.Context)` (`session/instance.go:1379`)
- `KillSession` (`session/instance_tmux.go:399`) — already likely wraps a `safeexec.CommandContext`-style tmux kill; verify it accepts/propagates ctx
- `CleanupWorktree` / `gitManager.Cleanup` (`session/instance_worktree.go:264`, `session/git/worktree_ops.go:346`) — needs a context-aware `git worktree remove` invocation

Then `destroyWithTimeout` derives its context from `context.WithTimeout(ctx, deleteSessionCleanupTimeout)` and passes it down, so the timeout actually cancels the subprocess instead of merely abandoning the wait.

## Related Tasks

Found during code review of PR #503 (`fix(services): eliminate deleteCleanupWG Add/Wait race, widen cleanup timeout`). Not fixed in that PR per `.claude/rules/fix-flaky-tests-dont-defer.md`'s blast-radius exception — threading a new `context.Context` parameter through `Instance.Destroy` touches shared infrastructure (`session/instance.go`, `session/instance_tmux.go`, `session/instance_worktree.go`, `session/git/worktree_ops.go`) well beyond that PR's `server/services/session_service.go` scope.
