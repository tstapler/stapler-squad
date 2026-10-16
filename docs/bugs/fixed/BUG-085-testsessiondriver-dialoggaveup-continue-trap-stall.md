# BUG-085: TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation stalls in continue trap [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-08-21
**Fixed**: 2026-08-21
**Impact**: One `session` package test failed reproducibly in isolation, not just under load. Confirmed to be a test-isolation defect, not a production logic bug — `session_driver.go`'s `dialogGaveUp` fall-through and initial-prompt-send logic were never broken.

## Problem Description

`TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation` (`session/session_driver_test.go:1230`) failed with:

```
SendKeys count never exceeded 3 — the dialogGaveUp fall-through never reached the initial-prompt-send step (stuck in the continue trap)
```

This reproduced in an isolated single-test run (`go test ./session/... -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -race -v -count=1`, ~68s), not just inside a large concurrent full-suite `-race` run — ruling out simple resource-contention flakiness as the sole cause.

## Reproduction Steps

1. `go test ./session/... -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -race -v -count=1`
2. Expected: test passes — the simulated dialog-gave-up condition falls through to the inactivity-escalation initial-prompt-send step, and `SendKeys` is called more than 3 times.
3. Actual (before fix): `SendKeys` count never exceeds 3.

## Root Cause

Confirmed test-isolation defect, not a `session_driver.go` logic bug.

`sendInitialPromptTick` (`session/session_driver.go:513`) calls `FindConversationFilePath(inst.GetStableID())` (`session/history.go:394`) before ever sending the initial prompt, to check whether a conversation already started underneath the driver. `FindConversationFilePath` walks `$HOME/.claude/projects` on real disk, searching JSONL files for the session's stable ID.

The test never sandboxed `$HOME`. On a real dev machine, `~/.claude/projects` holds genuine, large session history, and walking/reading through it for every driver-loop tick stalled real wall-clock seconds per tick — enough to consume the test's entire timing budget before `sendInitialPromptTick` ever reached its `SendKeys` call, producing exactly this failure for a reason completely unrelated to the `dialogGaveUp` fall-through logic under test.

This was previously masked: an earlier version of the test wrapped the body in `synctest.Test(...)` and included `t.Setenv("HOME", t.TempDir())` specifically to sandbox this walk, with a comment explaining why (real, large `~/.claude/projects` history stalls the search and makes the outcome depend on whatever's in that history). A prior fix attempt at this same bug rewrote the test to drop the `synctest` wrapper (to fix an unrelated timing-margin concern) and, in the process, silently dropped the `t.Setenv("HOME", ...)` line along with it — reintroducing the exact hazard the original comment warned about.

## Fix Approach

Restored `t.Setenv("HOME", t.TempDir())` at the top of the test (with a comment on why it's required), so `FindConversationFilePath`'s walk resolves instantly and deterministically to "not found," independent of the real session history on the machine running the test. Since `t.Setenv` cannot be used on a parallel test (`testing.T.Setenv` panics if called on/after `t.Parallel()`), also removed the `t.Parallel()` call this test had gained during the same prior rewrite.

## Verification

- `go test ./session/... -run TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation -count=5 -v -timeout 250s` — 5/5 passed, each consistently ~32s (down from the prior flaky ~68s+ real-disk-walk runs).
- `go test ./session -timeout 10m` (full package) — passed clean, 0 failures.

## Related Tasks

Discovered during `sdd:5-implement`'s final full-repo verification pass for `project_plans/terminal-multi-connection-streaming/` (see that project's `implementation/plan.md`) — this bug is unrelated to that project's own scope and does not block it. Per `.claude/rules/fix-flaky-tests-dont-defer.md`, root-caused and fixed rather than re-deferred, after an earlier fix attempt on this same bug reintroduced the underlying hazard while addressing a different concern.
