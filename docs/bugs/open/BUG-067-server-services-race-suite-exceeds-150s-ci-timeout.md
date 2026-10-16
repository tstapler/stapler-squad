# BUG-067: `server/services` race-enabled test suite exceeds the 150s MCP Integration Tests CI timeout [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-07
**Impact**: Intermittent CI red on the "MCP Integration Tests" workflow for any PR, unrelated to what that PR actually changes — erodes trust in CI red as a signal and can block unrelated PRs from shipping.

## Problem Description

`.github/workflows/mcp-integration.yml` runs `go test -tags=integration -timeout=150s -v -race ./server/mcp/... ./server/services/...`. The `server/services` package's cumulative wall-clock time under `-race` is now borderline against (and sometimes over) that fixed 150s ceiling, independent of the code change in whatever PR triggers the run. This is not one test being slow — the individual test that gets caught mid-flight when the alarm fires (`TestCreateSession_StatusManagerWiredBeforeDriver`) passes standalone in 0.42s. It's the accumulated total across the package's many tmux/session-driver integration tests, likely worsened by race-detector instrumentation overhead and/or ordinary test-suite growth over time.

## Reproduction Steps

1. On branch `backlog/stapler-squad-detector-plugins` (PR [#376](https://github.com/tstapler/stapler-squad/pull/376)) at commit `e6b6e8d5f` (a merge of `origin/main` bringing in ~24 unrelated commits), the "MCP Integration Tests" CI job passed: [run 31140616806](https://github.com/tstapler/stapler-squad/actions/runs/31140616806).
2. The very next push, commit `a6e570233`, changed *only* `session/detection/plugins.go` (an unrelated package refactor, extracting two helper functions from `LoadPluginDir` — no touch to `server/services` or `session/session_driver.go`). The same job failed: [run 31141249399](https://github.com/tstapler/stapler-squad/actions/runs/31141249399/job/92751434063), `panic: test timed out after 2m30s`, stuck mid-run in `TestCreateSession_StatusManagerWiredBeforeDriver` (`server/services/session_service_create_test.go:401`), blocked inside `session.runSessionDriverWithPrompt` (`session/session_driver.go:183`).
3. Reproduced locally and deterministically on the same commit (`a6e570233`): `go test -tags=integration -timeout=150s -race -count=1 ./server/services/...` → `FAIL github.com/tstapler/stapler-squad/server/services 150.108s`. Same wall-clock boundary, not a fluke.
4. Expected: the CI job passes reliably regardless of unrelated changes elsewhere in the repo.
5. Actual: pass/fail is a coin flip driven by total suite wall-clock time relative to a fixed timeout, not by test correctness.

## Root Cause

Unknown — needs profiling to identify which tests dominate wall-clock time under `-race` in `server/services`. Candidates: race-detector instrumentation overhead compounding across many real-tmux-session integration tests in one package binary; ordinary accretion of new tests over time without a corresponding timeout bump; possible serialization/lock contention across tests sharing package-level state (`TestMain` in `server/services/backlog_service_test.go:42`) that gets worse under `-race`'s slower scheduling.

Confirmed NOT caused by the detector-plugins feature diff itself — `server/services/*.go` is not among that PR's changed files, and the same package+flags passed cleanly against an earlier commit on the same branch differing only in an unrelated package.

## Files Likely Affected

- `.github/workflows/mcp-integration.yml` — the fixed `-timeout=150s` budget for `./server/mcp/... ./server/services/...`.
- `server/services/` — the package whose cumulative test wall-clock time is the actual bottleneck (not any single test).
- `server/services/session_service_create_test.go` — where the timeout happened to land when it fired; not itself the cause.

## Fix Approach

Unknown — needs one of: (a) profile which tests in `server/services` are slowest under `-race` and speed up or parallelize them, (b) split `server/services` integration tests into a separate, longer-timeout CI job, or (c) raise `mcp-integration.yml`'s `-timeout` budget if the suite's growth is expected and acceptable.

## Verification

`go test -tags=integration -timeout=150s -race -count=1 ./server/services/...` completes comfortably under 150s wall-clock, repeatably (run it 3+ times to rule out remaining marginal-timing flakiness).

## Related Tasks

Discovered while shipping PR [#376](https://github.com/tstapler/stapler-squad/pull/376) (`backlog/stapler-squad-detector-plugins`). Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than silently re-excused as "known pre-existing, unrelated."

**2026-08-14 update**: A separate backlog item root-caused a related-but-distinct contributor while investigating `TestDeleteSession_PublishesDeletedEvent` tripping the (unrelated) 10-minute default `go test` timeout on `./server/services/...`. That investigation found `DeleteSession`'s background cleanup goroutines (`liveInst.Destroy()` and the `KillTmuxSessionByTitle` fallback) were unbounded and un-awaited: nothing tracked them, so they could outlive the test that spawned them, and `TmuxSession.Close()`'s `kill-session` subprocess had no *real* bound (a wrapped `context.WithTimeout` never reached the actual subprocess — `buildTmuxCommand` bakes in `context.Background()`). Under contention (e.g. many tests hammering the same tmux server), a hung `kill-session` call could pin a goroutine indefinitely, adding to `server/services`'s cumulative wall-clock time — a plausible partial explanation for this bug's still-"Unknown" root cause, alongside the other candidates already listed above (race-detector overhead, package-level `TestMain` state, ordinary suite growth). Fixed via:
- `SessionService.deleteCleanupWG` (a `sync.WaitGroup`) tracking both `DeleteSession` cleanup goroutines, awaited by `Shutdown()`.
- `session/tmux/tmux.go`'s `TmuxSession.Close()` now runs `kill-session` through `executor.MakeTimeoutExecutor(killSessionTimeout)` (5s, matching `KillTmuxSessionByTitle`'s existing cap) instead of the unbounded `t.cmdExec`, so a hung tmux server is genuinely killed rather than merely having its *wait* time-boxed.
- `.github/workflows/build.yml` and `Makefile`'s `test`/`test-race` targets now pass an explicit `-timeout=20m` instead of relying on Go's silent 10m default, so a future regression surfaces as a clear timeout message with goroutine context instead of an opaque 10m cutoff.

This does not close this bug — the 150s MCP Integration Tests ceiling (a different, tighter CI job) may still need one of the fix approaches listed above even with the goroutine leak fixed.

**2026-08-21 update**: A separate backlog item (branch `backlog/stapler-squad-migrate-git-fixtures-to-go-git`) migrated `server/services`' git-subprocess-heavy test fixture helpers (`initGitRepoForTest`, `initGitRepoWithCommit`, and the drift-loop commit setup in `TestTriggerReReview_should_BlockOnBranchDriftInsteadOfMisleadingFailVerdict`) from real `git` CLI forks to `go-git`, and consolidated three duplicate helper implementations into `server/services/git_fixture_test.go`. That work targets the same "ordinary suite growth" contributor named above (real subprocess forks in fixture setup), alongside [[BUG-073]]'s specific `initGitRepoWithCommit` hang. Not expected to fully close this bug on its own — see that item for scope.

**2026-08-27 update**: New manifestation on PR #645's CI run (job 98612778356): `TestTriggerHandoffSummary_SynthesizesPendingResponse_When_NoRowExistsYet` (`server/services/handoff_summary_service_test.go:259`) failed with `[]sessionv1.HandoffSummaryStatus{1, 2} does not contain 4`, in a `server/services` run clocking 320.7s (vs. ~74s locally for the same package). Passed 10/10 in local isolation. PR #645 never touches `handoff_summary_service.go`, so this is the same load/contention pattern, not a new defect — logged per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than re-excused, then re-ran the CI job.
