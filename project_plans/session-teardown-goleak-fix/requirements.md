# Requirements: session-teardown-goleak-fix

## Source

Backlog item `1cea70ed-3127-48db-8e68-f02eac685510`: "Flaky test:
`TestServer_Shutdown_JoinsBackgroundTickers` goleak false positive under full
server suite." Consolidates three prior independent write-ups of the same
flake per `.claude/rules/fix-flaky-tests-dont-defer.md`:
`docs/bugs/open/BUG-083-server-shutdown-goleak-flake-under-full-suite.md`,
`docs/bugs/open/BUG-083-testservershutdownjoinsbackgroundtickers-picks-up-goroutines-from-prior-test.md`,
`docs/bugs/open/BUG-084-server-shutdown-goleak-false-positive-cross-test-tmux-goroutines.md`.
No interactive ideation was run (non-interactive triage session); this document
is derived directly from the item description and the three bug docs.

## Problem Statement

`go test ./server/... -race -count=1` intermittently fails
`TestServer_Shutdown_JoinsBackgroundTickers` (`server/server_test.go`) with a
`goleak.VerifyNone`-style failure (the "found unexpected goroutines" check at
`server/server_test.go:257`), reproducing roughly 1 run in 3-4 in a full
package run and 0/N in isolation (`-run TestServer_Shutdown_JoinsBackgroundTickers`).

## Root Cause (confirmed by three independent investigations)

The immediately-preceding test in the same test binary,
`TestSessionService_CreateThenImmediateDelete_NoDataRace`
(`server/server_integration_test.go:473`), creates a live tmux-backed session,
deletes it, and waits only for **tmux-level** teardown via
`waitForTmuxTeardown` (`server/server_integration_test.go:628`), which polls
`inst.TmuxSessionExists()`. `DeleteSession` tears down the session's component
goroutines asynchronously, so `waitForTmuxTeardown` returns before they have
necessarily exited:

- `session.(*ResponseStream).streamLoop`
- `session.(*CommandExecutor).executionLoop` / `waitForCommandOrDrain`
- `session.(*ClaudeController).runStatusChangeLoop`
- `session/detection/ratelimit.(*PTYConsumer).pollLoop`
- `pkg/analytics.(*MangleCorrelator).StartEviction`'s eviction goroutine
- (BUG-084 recurrence) `session/tmux.(*TmuxSession)`'s diagnostic
  `RestoreWithWorkDir` → `os/exec.(*Cmd).Wait` goroutine

If any of these are still unwinding when `TestServer_Shutdown_JoinsBackgroundTickers`
takes its goleak snapshot, they are misattributed as a leak in
`Server.Shutdown`, which is not actually implicated — confirmed by 100%
pass rate for the target test in isolation and by two independent diff-review
passes (PR #583, and this item) ruling out unrelated code as the cause.

This is a test-isolation/teardown-synchronization defect, not a real leak in
`Server.Shutdown` or the ticker helpers the target test exercises.

## Acceptance Criteria

1. `TestSessionService_CreateThenImmediateDelete_NoDataRace` (or the shared
   teardown helper it uses) blocks until all session component goroutines
   listed above have actually exited before the test returns — not just
   until tmux-level teardown (`TmuxSessionExists() == false`) completes.
2. Running `go test ./server/... -race -count=10` (or an equivalent repeated
   full-package run) no longer reproduces the
   `TestServer_Shutdown_JoinsBackgroundTickers` goleak failure that
   previously reproduced ~1 run in 3-4.
3. `TestServer_Shutdown_JoinsBackgroundTickers` itself is unmodified in
   behavior (still asserts no unexpected goroutines after `Shutdown()`) —
   the fix corrects the preceding test's teardown, not this test's
   assertion strength.
4. The three superseded bug docs (BUG-083 x2, BUG-084) are closed/moved to
   `docs/bugs/fixed/` referencing this item, per this repo's flaky-test
   convention, once the fix lands.
5. No new goroutine leak or deadlock risk is introduced in
   `DeleteSession`'s teardown path (e.g. a wait that can block forever if a
   component goroutine never exits) — any new wait has a bounded timeout
   with a clear test failure message on expiry.

## Out of Scope

- Rewriting `Instance`/`DeleteSession`'s shutdown architecture beyond what's
  needed to expose a deterministic "all component goroutines have exited"
  signal.
- Fixing unrelated flakes tracked elsewhere (BUG-080 through BUG-086 series)
  even though several share the same "full-suite-only, cross-test
  goroutine/state leakage" class.
- Production-path behavior changes — this is test-infrastructure-only; no
  user-facing runtime behavior should change.

## Existing Prior Art in the Codebase

`Stopped() <-chan struct{}` is an established pattern already used for this
exact "wait for a background loop to actually exit" problem:
- `session/history_watcher.go:74` (`HistoryFileWatcher.Stopped()`)
- `session/detection/plugin_watcher.go:43` (`PluginWatcher.Stopped()`)
- `server/services/claude_settings_watcher.go:320` (`ClaudeSettingsWatcher.Stopped()`)

The fix should follow this precedent rather than inventing a new
synchronization primitive.
