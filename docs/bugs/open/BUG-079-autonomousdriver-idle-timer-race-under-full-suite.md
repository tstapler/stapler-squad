# BUG-079: `TestAutonomousDriver_Stop_CancelsLoop_DuringNudgeSuppression` data races under full `session` package run

## Summary

`go test ./session/... -race -count=1` occasionally fails with a `WARNING: DATA
RACE` in `TestAutonomousDriver_Stop_CancelsLoop_DuringNudgeSuppression`
(`session/autonomous_driver_test.go`), but only when run as part of the full
`session` package suite — running it in isolation
(`go test ./session/ -run TestAutonomousDriver -race`) passes cleanly, with or
without unrelated changes present in the tree.

## Reproduction

Intermittent: seen once during a `go test ./session/... ./server/services/...
-race -count=1 -timeout 300s` run reviewing an unrelated change (SSH
remote-workspaces, Phase 5 approval-relay correction); did not reproduce on a
subsequent full-suite rerun in the same session. Not yet captured with a
reliable repro command — `-count=N` on the isolated test alone does not
trigger it, so the race appears to depend on cross-test state left behind by
some other `session` package test running earlier in the same process.

## Suspected root cause

`withShrunkIdleSettleTimers`/`waitForIdle`-style package-level test-timer
state (grep `session/autonomous_driver_test.go` and any sibling test file
that overrides the same package-level timer variables) appears to be shared,
mutable state read/written across subtests with no synchronization — i.e. a
leaked `AutonomousDriver.run()` goroutine from one subtest still reading a
package-level idle-settle timer variable while a later subtest's own
`t.Cleanup` concurrently restores/mutates it. Not confirmed by a captured
race trace (the triggering run's full output wasn't preserved) — this is an
inference from the symptom (full-suite-only reproduction) and from reading
the timer-override pattern, not a verified root cause.

## Why not fixed here

Discovered incidentally while reviewing an unrelated diff
(`session/sshremote/`, `server/services/approval_handler.go`,
`server/services/session_service.go` — none of which touch
`autonomous_driver.go`/`autonomous_driver_test.go`) as part of backlog item
`3461c8dd-a3b5-4543-8055-204c183ae396` (ssh-remote-workspaces). Root-causing
and fixing this properly requires:

- Capturing a reliable reproduction (may need `-count` on the full package,
  not just the one test, or a specific test-execution order).
- Reading every test in `session/` that touches the same package-level
  idle-settle timer state to find the actual leaked-goroutine / shared-state
  interaction, which is a wider blast radius than this backlog item's scope
  (SSH remote workspaces) justifies taking on mid-implementation.

Per `.claude/rules/fix-flaky-tests-dont-defer.md`, filing rather than
silently re-excusing as "known pre-existing, unrelated" — this is the first
time it's been captured in writing rather than waved off in review.
