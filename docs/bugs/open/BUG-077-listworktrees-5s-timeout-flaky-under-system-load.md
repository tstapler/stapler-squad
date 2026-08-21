# BUG-077: `PathCompletionService.ListWorktrees`'s 5s subprocess timeout flakes under host contention [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-21

## Problem Description

`TestListWorktrees_EmptyPath` (`server/services/workspace_misc_test.go:141`) failed once during a full-package `go test ./server/services/... -timeout 20m -count=1` run (496.420s total) with:

```
Error: Received unexpected error: deadline_exceeded: listing worktrees timed out
```

`PathCompletionService.ListWorktrees` (`server/services/path_completion_service.go:192`) bounds its `git worktree list --porcelain` subprocess with a fixed `listWorktreesTimeout = 5 * time.Second` (`path_completion_service.go:28`). The dev machine this ran on had a load average of 40–100+ from unrelated concurrent processes at the time — plausibly enough contention to blow a 5s subprocess-spawn+run budget.

## Reproduction Steps

Not reliably reproducible standalone: `go test ./server/services/... -run TestListWorktrees_EmptyPath -count=5 -v -timeout 60s` passed 5/5 in isolation on the same machine. Only observed once, inside a full-package run under heavy host load.

## Root Cause

Not fully confirmed — plausible hypothesis is subprocess scheduling delay under host CPU contention exceeding the fixed 5s ceiling; not yet proven with profiling.

## Files Likely Affected

- `server/services/path_completion_service.go` — `listWorktreesTimeout` constant and `ListWorktrees`.

## Fix Approach

Not yet investigated. Candidates: raise `listWorktreesTimeout`, or make it configurable/derived from parent context deadline rather than a fixed constant.

## Verification

`go test ./server/services/... -run TestListWorktrees_EmptyPath -count=20` passes reliably even under artificial host load.

## Related Tasks

Found while verifying the git-fixture-to-go-git migration backlog item (branch `backlog/stapler-squad-migrate-git-fixtures-to-go-git`) — confirmed unrelated to that migration's scope (`path_completion_service.go` is untouched by it, and the failure is a real `git` CLI subprocess call, not a test fixture). Filed rather than silently re-excused as "known flake" per `.claude/rules/fix-flaky-tests-dont-defer.md`; not fixed inline because doing so would mean tuning unrelated production timeout behavior, out of that item's blast radius.
