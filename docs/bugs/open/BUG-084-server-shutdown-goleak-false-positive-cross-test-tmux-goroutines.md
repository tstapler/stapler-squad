# BUG-084: `TestServer_Shutdown_JoinsBackgroundTickers` fails only in the full `server` package suite — goleak catches another test's still-teardown-in-flight tmux/PTY goroutines [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-21, while verifying `make test` for the `session.Repository`/`session.PipelineModeRepository` interface-pollution cleanup (`session/repository.go`, `session/storage.go`).
**Impact**: Intermittent CI noise on `go test ./server/...` — the failure is not reproducible on demand (passes reliably in isolation and in most full-package runs), which erodes trust in red CI for this package per `.claude/rules/fix-flaky-tests-dont-defer.md`.

## Problem Description

`go test ./server/ -count=1` fails roughly 1 run in 3-4 with:

```
--- FAIL: TestServer_Shutdown_JoinsBackgroundTickers (0.49s)
    server_test.go:257: found unexpected goroutines:
        [Goroutine ... session.(*CommandExecutor).waitForCommandOrDrain ...
         Goroutine ... session/detection/ratelimit.(*PTYConsumer).pollLoop ...
         Goroutine ... session.(*ClaudeController).runStatusChangeLoop ...
         Goroutine ... pkg/analytics.(*MangleCorrelator).StartEviction ...
         Goroutine ... session/tmux.(*TmuxSession).RestoreWithWorkDir ... os/exec.(*Cmd).Wait ...
         Goroutine ... session.(*ResponseStream).streamLoop ...
```

Running `go test ./server/ -run TestServer_Shutdown_JoinsBackgroundTickers -v` in isolation passes cleanly every time (5/5 in local repro).

The leaked goroutines all trace back to `TestSessionService_CreateThenImmediateDelete_NoDataRace` (`server/services` — imported into the `server` package's test binary), which spawns a real tmux session (`ptmx-race-repro-*`) with a live `CommandExecutor`/`ClaudeController`/`ResponseStream`/`PTYConsumer` stack. When that test's teardown (killing the tmux session, draining the PTY, stopping the controller) has not fully unwound by the time `TestServer_Shutdown_JoinsBackgroundTickers` runs its `verifyNoLeaksTolerant` goleak snapshot, the still-exiting goroutines from the *other* test are misattributed as a leak in this one. This is a test-isolation/ordering issue (shared process-wide goroutine population across `go test`'s single binary), not a real leak in `Server.Shutdown` or the ticker helpers `TestServer_Shutdown_JoinsBackgroundTickers` actually exercises.

Confirmed unrelated to the diff in progress: that change only touches `session/repository.go` (deleted the `Repository` interface, collapsed `Storage.repo` to a concrete `*EntRepository`) and callers of `Storage`/`BacklogLifecycleListener`/`SyncLoop` internals — none of `CommandExecutor`, `ClaudeController`, `ResponseStream`, `PTYConsumer`, or `TmuxSession` teardown were touched.

## Fix Approach

- Either give `TestSessionService_CreateThenImmediateDelete_NoDataRace` (and similar tests that spawn a real tmux/PTY-backed session) a synchronous, fully-blocking teardown (`t.Cleanup` that waits for `CommandExecutor`/`ClaudeController`/`ResponseStream` goroutines to actually exit, not just signals cancellation) before the test returns, or
- Scope `verifyNoLeaksTolerant`'s goleak snapshot in `server_test.go` to ignore goroutines whose stack roots in packages exercised by *other* concurrently-running session-lifecycle tests (fragile), or
- Serialize `TestServer_Shutdown_JoinsBackgroundTickers`-style goleak-sensitive tests away from tests that spawn real tmux sessions (e.g. via a build tag / `-run` split in CI, or `t.Parallel()` grouping) so the snapshot never races a concurrent teardown.

The first option is the most durable fix — it closes the same class of gap for any other goleak-sensitive test in the package, not just this one pairing.

## Recurrence log

- 2026-08-21, during the `make test` gate for PR #583 (`fix(tmux): eliminate AttachToExisting/RestoreWithWorkDir PTY-triple TOCTOU`): recurred with the identical signature under the second, fully-parallel `make test` invocation — same leaked set (`CommandExecutor.waitForCommandOrDrain`, `PTYConsumer.pollLoop`, `MangleCorrelator.StartEviction`, `RestoreWithWorkDir`'s diagnostic `os/exec.(*Cmd).Wait` goroutine), same `ptmx-race-repro-*` attribution. It was the only failure in the whole run. Confirmed **not** caused by that PR: the PR's diff touches no file under `server/`, and `RestoreWithWorkDir`'s diagnostic `waitOnce`/`cmd.Wait()` goroutine — the one tmux frame in the leak set — is byte-for-byte identical to `origin/main`'s (verified with `git show origin/main:session/tmux/tmux.go`). Passed 5/5 in isolation and passed the `server` package standalone (`go test -short ./server -count=1`, 16.899s) on the same tree, matching this bug's "not reproducible on demand" description. Logged rather than re-excused, per `.claude/rules/fix-flaky-tests-dont-defer.md`; the durable fix is still option 1 above (synchronous teardown for the tmux/PTY-backed session tests).

- 2026-08-23, during `github:pr-ship` for PR #609 (`fix(session): durable signal + badge for lost-history cold restore`, session-revive-uuid-loss AC3/AC5): recurred with the identical signature (`TestServer_Shutdown_JoinsBackgroundTickers` failing only in the full `server` package run, passing 3/3 in isolation) after merging `main` into the PR branch. Confirmed **not** caused by that PR: the diff touches `proto/session/v1/types.proto`, `server/adapters/instance_adapter*.go`, `server/services/session_service*.go`, `session/instance*.go`, `session/storage.go`, and `web-app/src/components/sessions/*` only — no file under `server/server*.go`, `session/tmux/`, or the `CommandExecutor`/`ClaudeController`/`ResponseStream`/`PTYConsumer` teardown paths named in the leak set. `go test ./server -run TestServer_Shutdown_JoinsBackgroundTickers -v -count=1` passed 3/3 in isolation on the same tree. Logged per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than re-excused silently; not fixed here since the durable fix (option 1 above) is a synchronous-teardown refactor to shared session-lifecycle test infrastructure, substantially larger than and unrelated to the AC3 notification/badge work this PR ships.
- 2026-08-25, during `make test` verification for backlog item e7664cbf (`fix(review-gate): recorded base-commit SHA missing from repo`, `session/review_gate.go` + `session/backlog_review.go` + `server/services/backlog_service_triage.go`): recurred with the identical signature — same `RestoreWithWorkDir`/`os/exec.(*Cmd).Wait` leak attributed from `TestSessionService_CreateThenImmediateDelete_NoDataRace`'s `ptmx-race-repro-*` session. Confirmed **not** caused by that change: reproduced identically on a clean `git stash` checkout of the same tree (before the fix's diff was applied), and the diff touches only `session/review_gate.go`, `session/backlog_review.go`, `session/git/worktree_ops.go`, and `server/services/backlog_service_triage.go` — none of `CommandExecutor`, `ClaudeController`, `ResponseStream`, `PTYConsumer`, or `TmuxSession` teardown. Passed in isolation (`go test ./server/ -run TestServer_Shutdown_JoinsBackgroundTickers -v`, 1/1). Logged rather than re-excused, per `.claude/rules/fix-flaky-tests-dont-defer.md`.

## Related Tasks

Found during `make test` verification for the `session.Repository`/`session.PipelineModeRepository` interface-pollution audit task (`.claude/rules/interface-pollution-checklist.md`). Not fixed as part of that task — fixing cross-test goroutine-teardown synchronization in `CommandExecutor`/`ClaudeController`/`ResponseStream` is unrelated to, and substantially larger than, the interface-collapse work that surfaced it.
