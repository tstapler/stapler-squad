# BUG-089: `TestServer_Shutdown_JoinsBackgroundTickers` flakes when run alongside the full `server` package suite [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-23
**Impact**: Intermittent CI failure in the `server` package's test suite — no production impact. Not caused by, or related to, PR #605's diff; this test is pre-existing code from `main` (merged into the PR #605 branch via a `main`-sync merge), not something that PR added or touched.

## Problem Description

`TestServer_Shutdown_JoinsBackgroundTickers` (in the `server` package — exact file not yet identified, found via `go test ./server/... -run 'TestServer_Shutdown_JoinsBackgroundTickers'` failing once) failed once when run as part of the full `go test ./server/... ./testutil/... -count=1` suite, but passed cleanly 20/20 times in isolation (`go test ./server/... -run 'TestServer_Shutdown_JoinsBackgroundTickers' -race -count=20`). This is the same *symptom shape* as BUG-087 (a test that's flaky only in the presence of other concurrently-running tests in the same package, not on its own) — likely another instance of shared-state interference (global logger, global config, or timing-sensitive ticker/shutdown-ordering assertions racing against unrelated tests in the same binary), though the specific mechanism hasn't been root-caused yet.

## Reproduction Steps

1. Run the full package: `go test ./server/... ./testutil/... -count=1`
2. Occasionally (observed once), `TestServer_Shutdown_JoinsBackgroundTickers` fails.
3. Run it in isolation: `go test ./server/... -run 'TestServer_Shutdown_JoinsBackgroundTickers' -race -count=20` — passes every time.

## Root Cause

**2026-08-25 update**: root cause identified. Confirmed reproducing 3/3 with `go test ./server/... -count=3` (independent of the diff being tested at the time — reproduces identically with or without an unrelated in-flight change, ruling out interference from that change specifically). The failure is `server_test.go:257`'s `verifyNoLeaksTolerant` reporting one unexpected goroutine:

```
Goroutine ... in state syscall, with syscall.Syscall6 on top of the stack:
...
os/exec.(*Cmd).Wait(...)
github.com/tstapler/stapler-squad/session/tmux.(*TmuxSession).RestoreWithWorkDir.gowrap1.(*TmuxSession).RestoreWithWorkDir.func2.1()
	session/tmux/tmux.go:1758
sync.(*Once).doSlow(...)
github.com/tstapler/stapler-squad/session/tmux.(*TmuxSession).RestoreWithWorkDir.func2(...)
	session/tmux/tmux.go:1758
created by github.com/tstapler/stapler-squad/session/tmux.(*TmuxSession).RestoreWithWorkDir in goroutine ...
	session/tmux/tmux.go:1756
```

This is a real subprocess-reaping goroutine spawned by `TestSessionService_CreateThenImmediateDelete_NoDataRace` (visible immediately before it in the log: that test creates a session named `staplersquad_ptmx-race-repro-<ts>` and its `RestoreWithWorkDir` call spawns a `sync.Once`-guarded goroutine that calls `exec.Cmd.Wait()` on the tmux subprocess). That test doesn't wait for the spawned goroutine to finish reaping before returning, so it's still alive (blocked in the `Waitid` syscall) when `TestServer_Shutdown_JoinsBackgroundTickers` runs its own `ignoreCurrentTolerant`/`verifyNoLeaksTolerant` baseline shortly after — the leak is attributed to the wrong test because of *when* it's observed, not *where* it originates. This is a Go-test-suite ordering artifact (both tests are in the `server` package's binary and share the process-wide goroutine pool goleak inspects), not a defect in `TestServer_Shutdown_JoinsBackgroundTickers`'s own logic, matching this doc's original suspicion.

A second, consistently co-occurring failure was also observed in the same runs: `TestHandleActuatorHealth_ReturnsOK_InNormalConditions` fails every time `TestServer_Shutdown_JoinsBackgroundTickers` does (same 3/3 pattern) — not yet confirmed whether it shares the same root cause or is a second, independent ordering artifact; worth checking together.

## Files Likely Affected

- `server/server_test.go` (`TestServer_Shutdown_JoinsBackgroundTickers`, `verifyNoLeaksTolerant`)
- `server/services/session_service_test.go` (`TestSessionService_CreateThenImmediateDelete_NoDataRace` — the actual leak source)
- `session/tmux/tmux.go:1750-1760` (`RestoreWithWorkDir`'s `sync.Once`-guarded `exec.Cmd.Wait()` goroutine)

## Fix Approach

`TestSessionService_CreateThenImmediateDelete_NoDataRace` needs to block until `RestoreWithWorkDir`'s spawned wait-goroutine has actually exited before the test returns — e.g. if `TmuxSession` exposes (or can expose) a way to wait on that `sync.Once` completing, or the test's own cleanup kills/waits on the tmux subprocess directly before returning. This is the same shape as the fix already applied to `session/streamhub`'s tests this session (2026-08-25, `withCursorSync`'s `slowCursorPositioner` test waiting on a `calledCh` before returning, in `session/streamhub/snapshot_prepare_test.go`) — don't return from a test until every goroutine it spawned (directly or via the code under test) has actually finished, not just handed off its result.

## Verification

After the fix: run `go test ./server/... -count=20` (full package, not just the isolated test) with no failures involving `TestServer_Shutdown_JoinsBackgroundTickers`.

## Related Tasks

Discovered while running PR #605's (`stapler-squad-web-transport` branch, project `web-transport-architecture-review`) local test gate, immediately after merging latest `main` into the branch (this test is `main`'s code, not PR #605's). See also `docs/bugs/open/BUG-087-captureLogs-global-slog-swap-races-under-t-parallel.md` for the same symptom shape found in the same investigation, and `docs/bugs/open/BUG-088-credential-chain-bypasses-test-dir-isolation.md` for a related test-isolation gap.
