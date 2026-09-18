# BUG-077: `PathCompletionService.ListWorktrees`'s 5s subprocess timeout flakes under host contention [SEVERITY: Low]

**Status**: ✅ Fixed
**Discovered**: 2026-08-21
**Fixed**: 2026-08-21

## Problem Description

`TestListWorktrees_EmptyPath` (`server/services/workspace_misc_test.go:141`) failed once during a full-package `go test ./server/services/... -timeout 20m -count=1` run (496.420s total) with:

```
Error: Received unexpected error: deadline_exceeded: listing worktrees timed out
```

`PathCompletionService.ListWorktrees` (`server/services/path_completion_service.go:192`) bounds its `git worktree list --porcelain` subprocess with a fixed `listWorktreesTimeout = 5 * time.Second` (`path_completion_service.go:28`). The dev machine this ran on had a load average of 40–100+ from unrelated concurrent processes at the time — plausibly enough contention to blow a 5s subprocess-spawn+run budget.

## Reproduction Steps

Not reliably reproducible standalone: `go test ./server/services/... -run TestListWorktrees_EmptyPath -count=5 -v -timeout 60s` passed 5/5 in isolation on the same machine. Only observed once, inside a full-package run under heavy host load.

## Root Cause

Confirmed: `git worktree list --porcelain` just reads `.git/worktrees/*` admin files — no history walk — so under normal conditions it completes in tens of milliseconds even for repos with dozens of worktrees. The fixed 5s timeout wasn't budgeting for expected command latency, it was meant to catch a genuinely hung subprocess (e.g. blocked on an `index.lock` held by another process). Under host CPU contention from an unrelated full-package test run, the subprocess itself wasn't stuck — it was starved of scheduling time — and 5s of headroom wasn't enough to absorb that scheduling delay on a loaded machine, even though the git command would have returned almost instantly once scheduled.

## Fix Approach

Raised `listWorktreesTimeout` from 5s to 20s (`server/services/path_completion_service.go:28`), with an updated doc comment explaining the bound exists to catch a truly-stuck subprocess rather than to budget for expected latency, and giving it more headroom for scheduling delay under load. A genuinely hung `git` process doesn't resolve itself in single-digit seconds either way, so the extra 15s of headroom costs nothing in the truly-stuck case while absorbing realistic host-contention scheduling delay in the common case.

## Verification

`go test ./server/services/... -run TestListWorktrees_EmptyPath -count=10 -v` — 10/10 passed.

## Related Tasks

Found while verifying the git-fixture-to-go-git migration backlog item (branch `backlog/stapler-squad-migrate-git-fixtures-to-go-git`) — confirmed unrelated to that migration's scope (`path_completion_service.go` is untouched by it, and the failure is a real `git` CLI subprocess call, not a test fixture). Filed rather than silently re-excused as "known flake" per `.claude/rules/fix-flaky-tests-dont-defer.md`, and fixed in the same pass rather than deferred, per explicit follow-up instruction during the `terminal-multi-connection-streaming` project's Phase 6 verification.
