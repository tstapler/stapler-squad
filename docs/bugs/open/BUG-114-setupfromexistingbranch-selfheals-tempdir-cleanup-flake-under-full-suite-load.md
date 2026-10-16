# BUG-114: `TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner` `t.TempDir()` cleanup flakes under full `session` package suite load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-23, during post-merge verification for `session-teardown-goleak-fix`
(backlog `1cea70ed-3127-48db-8e68-f02eac685510`) — unrelated to that fix (which only touches
`session/claude_controller.go` and `session/detection/ratelimit/integration.go`; this failure is
in `session/git`, untouched by that diff).
**Impact**: Test-only. `go test ./session/... -race` intermittently reports one failure among the
full package run:

```
--- FAIL: TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner (0.66s)
    testing.go:1464: TempDir RemoveAll cleanup: unlinkat /tmp/TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByD4287902662/001/.git: directory not empty
```

Re-running the same test in isolation (`-run TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner
-count=10 -race`) passes reliably (10/10, race-clean, `session/git` 10.123s).

## Problem Description

`TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner`
(`session/git/worktree_ops_test.go:505`) uses `t.TempDir()` for its git repo/worktree fixtures.
`t.Cleanup`'s automatic `RemoveAll` races against something still holding an open file handle or
lock under `.git/` at the moment cleanup runs, only under the concurrent load of a full
`./session/...` `-race` run (many other packages' tests — including `session/tmux`,
`session/unfinished/gogitstore`, `session/tymux` — running real subprocesses concurrently).

Same failure signature (`TempDir RemoveAll cleanup: unlinkat ... directory not empty`) as
`BUG-091` (`server/services`, a different test/package) and `BUG-098` (`server/services`, root
cause: a leaked pipeline goroutine polluting a later test's shared `STAPLER_SQUAD_TEST_DIR`) —
this is a new specific instance of the same disease class, not yet root-caused for this test.

## Reproduction

```
go test ./session/... -race            # intermittent, not every run
go test ./session/git/... -race -count=10 -run TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner
# always passes in isolation
```

## Suspected root cause

Some other `session/git` (or sibling `session/` package) test spawns a real `git`
subprocess/worktree against a path under (or referencing) this test's `t.TempDir()`, or a git
index/pack-file lock from this test's own fixture setup hasn't released by the time `t.Cleanup`
fires under full-suite CPU contention. The log line immediately preceding the failure —
`WARN could not find merge-base for branch with any default branch` — suggests the test's own
git operations may still be in flight relative to `t.Cleanup`'s LIFO ordering. Needs the same
`t.Cleanup`-ordering audit BUG-091's fix applied (verify no background goroutine/subprocess this
test spawns is still running when `t.TempDir()`'s cleanup fires).

## Fix Approach

Audit `TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner` and its
helpers for any git subprocess or worktree operation not explicitly awaited before the test
returns; add an explicit wait/join if one is found, following `BUG-091`'s precedent fix pattern
(explicit awaited teardown scheduled before `t.TempDir()`'s implicit cleanup, not after).

## Verification

After fix: `go test ./session/git/... -race -count=20` and `go test ./session/... -race` run
repeatedly (~10x) with zero `TempDir RemoveAll cleanup` failures for this test.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — filed per this rule rather than re-excused;
  root-causing is deferred to a dedicated fix session since it requires auditing test helpers
  unrelated to the diff that surfaced it.
- `BUG-091`, `BUG-098` — same `TempDir RemoveAll cleanup: unlinkat ... directory not empty`
  failure signature, different specific tests/packages.
