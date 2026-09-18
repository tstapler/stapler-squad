# BUG-077: `resumeFromHibernationLocked`'s background goroutine races `Approve`/`Deny` over `Instance.status` [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-15 (validating `make ci -race` for PR #494/the webhook-triggers-followups clean rebase)
**Impact**: Intermittent `-race` failures in `session` package's full test suite (`TestApprove_AllSourceStatuses` / `TestDeny_AllSourceStatuses`), which erodes trust in red CI for this package (see `.claude/rules/fix-flaky-tests-dont-defer.md`). Also a real production race, not just test noise — the same code paths run in the live server.

## Problem Description

`go test -race ./session/...` (full package) intermittently fails with:

```
WARNING: DATA RACE
Write at 0x00c0002fd868 by goroutine N:
  session.(*Instance).loadStatus()
      session/instance_state.go:22
  session.resumeFromHibernationLocked.func1.1()
      session/instance_hibernate.go:119
  session.(*Instance).send()
      session/actor.go:64
  session.resumeFromHibernationLocked.func1()
      session/instance_hibernate.go:113

Previous read at 0x00c0002fd868 by goroutine M:
  session.TestApprove_AllSourceStatuses.func1()
      session/instance_approve_deny_test.go:216
```

`resumeFromHibernationLocked` (`session/instance_hibernate.go:109-119`) spawns a goroutine via
`Instance.send()` (`session/actor.go:64`) that calls `loadStatus()`
(`session/instance_state.go:22`, writes `Instance.status`) — this write is unsynchronized
against a concurrent read of the same field from `approveLocked` → `transitionToLocked`
(`session/instance_state.go:102,111`), which is itself the code path
`TestApprove_AllSourceStatuses`/`TestDeny_AllSourceStatuses` (`session/instance_approve_deny_test.go`)
exercise directly.

Running either test in isolation (`go test -race -run TestDeny_AllSourceStatuses ./session/`, 3x)
passes cleanly every time — the race only surfaces when both tests' Instances happen to interleave
in the full-package `-race` run, consistent with a genuine unsynchronized-write bug rather than a
test-only artifact.

Confirmed unrelated to the webhook-triggers-followups diff: none of `session/instance_hibernate.go`,
`session/instance_state.go`, `session/actor.go`, or `session/instance_approve_deny_test.go` are
touched by that PR's 5 commits — filed per the blast-radius exception in
`.claude/rules/fix-flaky-tests-dont-defer.md` rather than root-caused there, since diagnosing the
hibernation-resume goroutine's locking discipline is out of scope for a webhook-triggers cleanup PR.

## Fix Approach

- `resumeFromHibernationLocked`'s background goroutine (`instance_hibernate.go:109-119`) writes
  `Instance.status` via `loadStatus()` without holding whatever lock `approveLocked`/
  `transitionToLocked` expect callers to hold (the `Locked` suffix on all three functions implies
  an existing locking convention that this goroutine escapes by running detached via `send()`).
- Likely fix: either have the goroutine acquire the same lock before calling `loadStatus()`, or
  route its status write through the same actor-mailbox mechanism (`Instance.send()`'s normal
  synchronous path) that `Approve`/`Deny` already use, instead of firing a raw goroutine that
  touches shared state directly.
- Verify with `go test -race -run 'TestApprove_AllSourceStatuses|TestDeny_AllSourceStatuses' ./session/... -count=20` (loop to increase odds of re-triggering the interleaving) before and after the fix.

## Related Tasks

Found during local `make ci` validation for the webhook-triggers-followups backlog item's clean
rebase onto `main` (post PR #381 merge). Not fixed there — out of scope (unrelated instance-hibernation
locking vs. webhook-trigger follow-ups).
