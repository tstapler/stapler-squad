# BUG-099: SessionDriver goroutine outlives `Instance.Destroy()` for a stub/non-ready program, reliably [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-09-05, while investigating an intermittent `TempDir RemoveAll` "directory not empty"
failure in `TestCreateSession_Autonomous_CreatesDirectoryInBaseDir` and
`TestCreateSession_EmptyPath_Autonomous_PassesPathValidation` (`server/services/session_service_create_test.go`)
surfaced by a full `make ready` run.
**Fixed in**: `refactor/deterministic-fast-tests` (2026-09-05)

## Root Cause

`DeleteSession` → `cleanupPartialCreation(instance)` (`server/services/session_service.go`) only called
`instance.Destroy()` — the only place that called `session.StopSessionDriver(instance)` — when
`instance.Started()` was already `true`:

```go
func cleanupPartialCreation(instance *session.Instance) error {
	if instance.Started() {
		return instance.Destroy()
	}
	// ...defense-in-depth guard: KillSession()/CleanupWorktree(), never StopSessionDriver...
}
```

For an autonomous session, `CreateSession`'s async background-resolution pipeline can still be racing to
call `Instance.Start()` (which sets `started`) and `StartSessionDriver` when the test's `DeleteSession`
call runs. If `DeleteSession` observes `Started() == false` at that instant, it takes the
"defense-in-depth liveness guard" fallback branch instead — which never calls `Destroy()`, and therefore
never calls `StopSessionDriver`, under any circumstance. If the racing pipeline goroutine then goes on to
actually call `StartSessionDriver` (it isn't blocked by anything — `driverDestroyed` was never set), that
driver goroutine is now completely orphaned: nothing will ever close its `stop` channel or set
`inst.destroyed`, so it just keeps polling `PreviewContext` every `driverPollInterval` (2s) until its own
25-minute `driverTotalTimeout` safety net.

Confirmed via a goroutine dump (`runtime.Stack`, temporarily added to `JoinSessionDriver`) taken at the
moment a `stopJoinTimeout` (10s) wait expired: the driver goroutine was still actively polling
`inst.PreviewContext(ctx)` every ~2s, `st` reading `Active` the whole time (never `Stopped`) — and no
`StopSessionDriver` call, and no `cleanupPartialCreation` log line of any kind, appeared anywhere in the
test's log output. That absence is what pointed at the `!Started()` fallback branch: it can succeed
silently (no error, no log) on both its `KillSession()`/`CleanupWorktree()` sub-branches when there's
nothing left to kill/clean up, which was true here since the toy `program: "true"` command's tmux window
(and, in this case, the whole session) had already exited on its own by the time `DeleteSession` ran.

This is why every earlier attempt at widening `JoinSessionDriver`'s timeout (10s → 30s → 45s) failed
identically at 100% of runs with zero variance: there was no stop signal in flight to eventually observe —
`StopSessionDriver` was never called at all for these tests, so no amount of waiting would help.

## Fix

`cleanupPartialCreation` now calls `session.StopSessionDriver(instance)` at the top of its
`!instance.Started()` fallback branch — the one path that previously never called it — so every exit from
`cleanupPartialCreation` now calls `StopSessionDriver` exactly once, either directly here or via
`Destroy()`'s own top-of-function call in the `Started()==true` branch. This mirrors `Destroy()`'s
documented rationale verbatim: marking `driverDestroyed` here guarantees that a `StartSessionDriver` call
arriving after this point — even one already racing in flight — refuses to start a driver goroutine that
nothing would ever be able to stop.

```go
func cleanupPartialCreation(instance *session.Instance) error {
	if instance.Started() {
		return instance.Destroy()
	}
	session.StopSessionDriver(instance)
	// ...defense-in-depth guard, unchanged...
}
```

Verified: the two originally-flaky tests (already decoupled from this lifecycle path in a separate,
smaller change — see the extracted `requiresExplicitPath`/`needsGeneratedOneOffPath`/`generateOneOffPath`
pure functions in `server/services/session_service.go`) plus every other test that still exercises the
full `CreateSession` → `destroyCreatedSession` → `JoinSessionDriver` round trip
(`TestCreateSession_Autonomous_EmptyPath_GeneratesDirectoryInBaseDir`,
`TestCreateSession_Autonomous_ExplicitPath_DoesNotGenerateScratchDir`,
`TestCreateSession_OneOff_CreatesDirectoryInBaseDir`,
`TestCreateSession_OneOff_TwoCallsCreateTwoDistinctDirectories`,
`TestCreateSession_EmptyPath_OneOff_PassesPathValidation`) now complete in well under a second each (down
from ~10s apiece) with zero `JoinSessionDriver` timeout warnings, 5x under `-race` with no flakes. Full
`server/services`, `session`, and `config` packages pass under `-race` (one unrelated, pre-existing
full-suite-parallel-load race — see the many other `docs/bugs/{open,fixed}/*-flaky-under-full-suite*.md`
entries — was observed in 1 of 4 full-package runs and reproduced regardless of this fix; not something
this change introduced or is responsible for fixing).

## Related fix that surfaced along the way (not the root cause, but worth keeping)

While investigating, `config.GetConfigDirForDir`'s test-mode branch was found to call
`pruneStaleTestDirs` — an `os.ReadDir` + potentially several `os.RemoveAll` calls over the shared
`~/.stapler-squad/test/` directory — on **every single call**, and that function is on the hot path of
every `tmux.AcquireExecSlot`/`AcquireResyncExecSlot`/`AcquireInputExecSlot` call (i.e. every tmux
subprocess spawn, including every `SessionDriver` poll tick). On a machine with many accumulated
`test-<pid>` directories from prior runs, this made an early goroutine dump during this investigation
briefly show the driver goroutine busy inside `pruneStaleTestDirs`'s scan rather than at its `stop`-aware
select — not the actual root cause (the goroutine was never told to stop at all, so this was never the
full explanation), but a real, independent inefficiency on a very hot path. Fixed by wrapping the call in
a `sync.Once` (`config/config.go`'s `pruneStaleTestDirsOnce`) so it runs at most once per process instead
of once per tmux subprocess spawn.

## Related

- `docs/bugs/open/BUG-098-crosstest-testdir-pollution-from-leaked-tmux-pipeline-goroutine.md` — same
  underlying "goroutine outlives its test, touches a later test's TempDir" symptom class, different leak
  site (an untracked async pipeline goroutine, not this bug's SessionDriver goroutine).
- `.claude/skills/fix-flaky-tests-dont-defer/SKILL.md` — this bug was root-caused and fixed in the same
  session it was surfaced in, per this repo's standing rule.
- `session/session_driver.go`'s `StopSessionDriver`/`JoinSessionDriver`, `session/instance.go`'s
  `Destroy()`, `server/services/session_service.go`'s `cleanupPartialCreation`.
