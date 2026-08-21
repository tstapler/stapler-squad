# BUG-083: `TestServer_Shutdown_JoinsBackgroundTickers` intermittently fails on leaked goroutines from the preceding test's session teardown [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-21, while verifying `go build ./...` / `go test ./server ...` after merging `main` into `backlog/stapler-squad-dynamic-rule-reload` (PR #538)

## Problem Description

`TestServer_Shutdown_JoinsBackgroundTickers` (`server/server_test.go:310`) asserts no unexpected goroutines are running after `Shutdown()` (see the "found unexpected goroutines" check at `server/server_test.go:257`). It intermittently fails in a full `go test ./server` run — reproduced 1 failure in 3 full-package runs, but passes consistently when run in isolation (`-run TestServer_Shutdown_JoinsBackgroundTickers`, 4/4 passes).

Root cause: the immediately-preceding test, `TestSessionService_CreateThenImmediateDelete_NoDataRace` (`server/server_integration_test.go:473`), creates a live session, deletes it, then only waits for **tmux-level** teardown via `waitForTmuxTeardown` (`server/server_integration_test.go:628`), which polls `inst.TmuxSessionExists()`. It does not wait for the session `Instance`'s component goroutines (`session.ResponseStream.streamLoop`, `session.CommandExecutor.executionLoop`/`waitForCommandOrDrain`, `session.ClaudeController.runStatusChangeLoop`, `session/detection/ratelimit.PTYConsumer.pollLoop`, `pkg/analytics.MangleCorrelator.StartEviction`) to actually exit. `DeleteSession` tears these down asynchronously, so they can still be alive — and get captured in `TestServer_Shutdown_JoinsBackgroundTickers`'s goroutine snapshot — by the time the next test in the same process starts.

`Instance.IsStopped()` (`session/instance_state.go:144`) checks a state-machine status flag, not goroutine completion, so it isn't a drop-in fix for `waitForTmuxTeardown`'s wait condition without confirming that status transition is itself gated on all of the above goroutines joining.

## Reproduction Steps

```
cd server && for i in 1 2 3; do go test . -count=1 2>&1 | grep -E "^--- FAIL|^FAIL"; done
```
Typically fails within 2-3 runs. Isolated runs of the failing test alone do not reproduce it — the failure requires `TestSessionService_CreateThenImmediateDelete_NoDataRace` (or another session-creating test) to run immediately before it in the same test binary.

## Suggested Fix Approaches

- Extend `waitForTmuxTeardown` (or add a sibling helper) to also wait for the session `Instance`'s component goroutines to exit — e.g. a `Stopped() <-chan struct{}` on `Instance` (mirroring the pattern already used by `session.HistoryFileWatcher.Stopped()` and `server/services.ClaudeSettingsWatcher.Stopped()`) that closes only once `ResponseStream`, `CommandExecutor`, `ClaudeController`, and any `PTYConsumer` have all confirmed their run loops have returned.
- Confirm whether `Instance.IsStopped()`'s `Stopped` status transition is already gated on this goroutine set; if so, `waitForTmuxTeardown` (or callers of it) could simply also poll `inst.IsStopped()`.

## Why Not Fixed Here

Surfaced as a side effect of merging `main` into PR #538's branch (a `server/services` claude-settings-reload feature) and re-verifying `go test ./server`; the actual fault is in `session` package component shutdown ordering and an integration test's teardown wait, an unrelated subsystem, so fixing it here would expand this PR's blast radius into code neither authored nor reviewed as part of it. Filed per this repo's "fix flaky tests when found, don't just defer" convention (`.claude/rules/fix-flaky-tests-dont-defer.md`).

## Related

Same class of full-suite-only, cross-test goroutine/state leakage as [[BUG-081]] (`docs/bugs/fixed/BUG-081-defaultratelimiter-global-poisoned-across-session-package-tests.md`) and [[BUG-082]] (`docs/bugs/open/BUG-082-findconversationfilepath-walks-real-home-projects-dir-in-tests.md`).
