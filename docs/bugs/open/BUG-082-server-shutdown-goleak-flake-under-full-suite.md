# BUG-082: `TestServer_Shutdown_JoinsBackgroundTickers` goleak flake under full `server` package run

## Summary

`go test ./server/ -race -tags integration -count=1` intermittently fails
`TestServer_Shutdown_JoinsBackgroundTickers` (`server/server_test.go:206`)
with a `goleak.VerifyNone` failure. Running the test in isolation — including
10x with `-race -tags integration` — passes reliably every time. Reproduced
3 times out of roughly 10 full-package attempts (both with and without `-v`),
so it depends on cross-test ordering/timing, not this test alone.

## Reproduction

```
go test ./server/ -race -tags integration -count=1 -v
# --- FAIL: TestServer_Shutdown_JoinsBackgroundTickers (0.72s)

go test ./server/ -run TestServer_Shutdown_JoinsBackgroundTickers -race -tags integration -count=10
# always passes
```

## Leaked goroutines (from a captured failure)

```
Goroutine ... github.com/tstapler/stapler-squad/session.(*ClaudeController).runStatusChangeLoop
  session/claude_controller.go:1180
Goroutine ... github.com/tstapler/stapler-squad/session.(*CommandExecutor).waitForCommandOrDrain
  session/command_executor.go:256
Goroutine ... github.com/tstapler/stapler-squad/session.(*ResponseStream).streamLoop
  session/response_stream.go:237
Goroutine ... github.com/tstapler/stapler-squad/pkg/analytics.(*MangleCorrelator).StartEviction
  pkg/analytics/mangle_correlator.go:184
Goroutine ... os/exec.(*Cmd).Wait (real tmux child process)
  session/tmux/tmux.go:1638, via (*TmuxSession).RestoreWithWorkDir
```

None of these are owned by `TestServer_Shutdown_JoinsBackgroundTickers` itself
(it only exercises `server.Server`'s ticker/HTTP shutdown path) or by
`session/sshremote`'s `RemoteHealthProber` (ruled out: `RemoteHealthProber`'s
own watcher goroutines were stress-tested 50x under `-race` in isolation with
zero leaks as part of the ssh-remote-workspaces change that surfaced this).
They are all real session lifecycle goroutines (`ClaudeController`,
`CommandExecutor`, `ResponseStream`, `MangleCorrelator`) plus a real forked
tmux process from an *earlier* test in the same `server` package binary that
hadn't finished tearing down by the time this later test's `goleak.VerifyNone`
ran. Under the heavier scheduling pressure of a full `-race`, multi-package
`make test-integration` run, that earlier test's cleanup apparently doesn't
always complete before this later test's leak check fires.

## Suspected root cause

Some `server` package test (candidates: anything constructing a real
`session.Instance`/`ClaudeController`/`ResponseStream`/`TmuxSession` for an
integration-style test, per `tests/e2e/sshd` and `*_remote_test.go` additions
in this same package) does not synchronously wait for its
goroutines/child tmux process to fully exit in its own cleanup before
returning control to the test runner. `TestServer_Shutdown_JoinsBackgroundTickers`
is just the unlucky goleak checkpoint that happens to run soon after and
observes the still-unwinding state — not the source of the leak itself. Mirrors
BUG-079's and BUG-080's diagnosis pattern (cross-test state/goroutine bleed
under full-suite-only `-race` runs), all three discovered during work on
ssh-remote-workspaces without being caused by it.

## Fix Approach

Identify which `server` package test constructs a real session/tmux
controller stack and audit its `t.Cleanup`/teardown for a missing
`wg.Wait()`/process-reap step (likely needs a `goleak.IgnoreCurrent()`
baseline or an explicit join before the test returns, matching the pattern
`TestServer_Shutdown_JoinsBackgroundTickers` itself already uses correctly).

## Verification

After fix: `go test ./server/ -race -tags integration -count=1` run
repeatedly (~20x) with zero `TestServer_Shutdown_JoinsBackgroundTickers`
goleak failures.

## Related

- `.claude/rules/fix-flaky-tests-dont-defer.md` — filed per this rule's
  exception clause (fixing requires auditing session/tmux teardown across
  `server` package tests unrelated to the change that surfaced it).
- BUG-079, BUG-080 — same full-suite-only flake shape, discovered in the same
  ssh-remote-workspaces session.
