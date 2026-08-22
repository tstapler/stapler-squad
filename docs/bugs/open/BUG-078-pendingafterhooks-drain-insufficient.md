# BUG-078: `WaitForPendingAfterHooks` doesn't actually prevent the race it was built to fix

## Summary

`session/state_machine.go`'s `pendingAfterHooks sync.WaitGroup` /
`WaitForPendingAfterHooks(timeout time.Duration) bool` mechanism was added to let
`TestTransitionTo_ValidTransitions` and `TestTransitionTo_ChainedTransitions`
(`session/state_machine_test.go`) drain detached `TransitionDef.After` goroutines
(`hibernateProcess`, `resumeFromHibernation`) before `t.TempDir()` cleanup removes
the directory those goroutines are still reading, and before the next test/step
reuses the `Instance`.

It does not work. `go test ./session/... -run 'TestTransitionTo' -race -count=5`
fails deterministically (not intermittently) with two distinct `WARNING: DATA
RACE` reports plus explicit `state_machine_test.go:311: timed out waiting for
pending after-hook goroutines` log lines confirming the timeout is actually
being hit in this environment.

## Root cause A — no draining between chained steps

`TestTransitionTo_ChainedTransitions`'s per-chain loop
(`session/state_machine_test.go:314-317`):

```go
for i, step := range chain.steps {
    if err := inst.transitionTo(ctx, step.to); err != nil {
        t.Fatalf("step %d: transitionTo(%s) from %s: %v", i, step.to, inst.Status, err)
    }
}
```

calls `inst.transitionTo` back-to-back with no drain in between — only once at
the very end, in `t.Cleanup` (`state_machine_test.go:309-313`). Since
`TransitionDef.After` spawns a detached goroutine that outlives the call, step
N+1 can run its synchronous field mutation on `inst` while step N's async
hibernate/resume goroutine is still executing.

Confirmed by race trace: a "previous write" at `session/instance_state.go:43`
inside `TestTransitionTo_ChainedTransitions.func1()` (the next chain step,
`state_machine_test.go:315`) raced against a "read" at `session/instance.go:1101`
(`startLocked`), reached via `resumeFromHibernation.(*Instance).Start.func1()`
(`instance.go:915`) → `sendSyncErr()` (`actor.go:37`) → `Start()`
(`instance.go:914`) → `resumeFromHibernation()` (`instance_hibernate.go:160`) →
the `Hibernated→Active` transition's `After` hook (`state_machine.go:69`) —
i.e. the *prior* step's async hook, still running.

## Root cause B — 5s timeout is too short and non-fatal on expiry

Even in `TestTransitionTo_ValidTransitions`, which calls
`WaitForPendingAfterHooks(5 * time.Second)` exactly once per subtest in
`t.Cleanup` (`state_machine_test.go:228-232`), the wait times out in practice:
real tmux session setup in this environment can exceed 5s end-to-end. Test log:

```
failed to pre-configure tmux server before session creation ...
command timed out after 5s: tmux -L test-isolated-94181 start-server ...
... (~3.6s later) ...
tmux new-session command succeeded
```

`state_machine_test.go:311: timed out waiting for pending after-hook goroutines`
fires, confirming the timeout is hit. Both call sites treat a `false` return as
a `t.Log`-only event — no `t.Fatal`, no retry, no extended wait — so the test
proceeds regardless, and the still-running goroutine goes on to race with
`t.TempDir()` cleanup or, in cases seen this run, with a *different* subtest's
freshly-allocated `Instance` (a second distinct race trace: a goroutine spawned
inside `WaitForPendingAfterHooks` at `state_machine.go:109` for one subtest's
cleanup, still live when `TestTransitionTo_ValidTransitions/Active->Hibernated`'s
`transitionTo` runs at `instance_state.go:50`).

## Reproduction

```bash
go test ./session/... -run 'TestTransitionTo' -race -count=5
```

Fails every run in this environment as of 2026-08-16, with `--- FAIL:` on:
- `TestTransitionTo_ChainedTransitions/hibernate_and_resume`
- `TestTransitionTo_ChainedTransitions` (parent)
- `TestTransitionTo_ValidTransitions/Active->Hibernated`
- `TestTransitionTo_ValidTransitions` (parent)

## Why not fixed in this session

Discovered while validating a WIP fix for a *different*, previously-diagnosed
TempDir-cleanup race (tracked as part of backlog item
`e8d180d4-c4ff-4856-bc43-768365584420`). Fixing this properly likely means one
of:

- Draining `WaitForPendingAfterHooks` after every chain step, not just once at
  the end.
- Making the timeout path a hard test failure (`t.Fatal`) instead of a log, so
  a real timeout surfaces immediately instead of silently letting the test
  proceed into a race.
- Making the `Hibernated→Active` / `Active→Hibernated` transition tests use a
  faster/mocked tmux backend instead of exercising real `tmux start-server` /
  `new-session`, which appears to be the actual source of the multi-second
  delay.
- Reconsidering whether `TransitionDef.After` hooks should be
  synchronous-with-cancellation rather than fire-and-forget-then-drain, since
  nothing today prevents two transitions being requested back-to-back (in
  tests or production) before the first's `After` hook completes.

None of these were evaluated for blast radius before this bug was filed;
picking one is follow-up work. The WIP in `session/state_machine.go` /
`session/instance_hibernate.go` implementing the `pendingAfterHooks` mechanism
was **not committed** as part of this session's other fixes because its own
regression tests fail deterministically under `-race`.

## Recurrence: 2026-08-17

Reproduced again during full-suite verification for an unrelated change (ssh-remote-workspaces
Epic 6.4, `session/sshremote/health_prober.go` + `server/server.go` wiring — none of this bug's
implicated files touched):

```
go test ./session/... ./server/... ./pkg/... -race -count=1 -timeout 300s
```

`--- FAIL: TestComputeCurrentDiffHash_...` (`session` package, collateral -- a different test
whose goroutine happened to be attributed the race by `go test -race`'s reporting) plus a `DATA
RACE` trace matching this bug's Root cause A exactly: `TestTransitionTo_ChainedTransitions.func1()`
(`state_machine_test.go:305`, the "hibernate and resume" chain's next step) racing
`(*Instance).loadStatus()` (`instance_state.go:22`) reached via the prior step's still-running
`resumeFromHibernation` goroutine (`instance_hibernate.go:178`).

Confirms the intermittency this doc already notes: `go test ./session/... -run 'TestTransitionTo'
-race -count=5` (this doc's own repro command) and `-run 'TestTransitionTo_ChainedTransitions'
-count=3` both passed cleanly in isolation immediately after the failure -- consistent with
BUG-077's finding that this class of race "only surfaces when...Instances happen to interleave in
the full-package `-race` run," not with a narrower `-run` filter. Not fixed here per this task's
scope (unrelated subsystem); logged per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than
re-excused silently.
