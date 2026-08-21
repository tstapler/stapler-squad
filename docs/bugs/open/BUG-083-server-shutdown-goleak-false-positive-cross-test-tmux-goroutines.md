# BUG-083: `TestServer_Shutdown_JoinsBackgroundTickers` fails only in the full `server` package suite — goleak catches another test's still-teardown-in-flight tmux/PTY goroutines [SEVERITY: Low]

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

## Related Tasks

Found during `make test` verification for the `session.Repository`/`session.PipelineModeRepository` interface-pollution audit task (`.claude/rules/interface-pollution-checklist.md`). Not fixed as part of that task — fixing cross-test goroutine-teardown synchronization in `CommandExecutor`/`ClaudeController`/`ResponseStream` is unrelated to, and substantially larger than, the interface-collapse work that surfaced it.
