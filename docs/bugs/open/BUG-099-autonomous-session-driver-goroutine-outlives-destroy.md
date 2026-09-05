# BUG-099: SessionDriver goroutine outlives `Instance.Destroy()` for a stub/non-ready program, reliably [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-05, while investigating an intermittent `TempDir RemoveAll` "directory not empty"
failure in `TestCreateSession_Autonomous_CreatesDirectoryInBaseDir` and
`TestCreateSession_EmptyPath_Autonomous_PassesPathValidation` (`server/services/session_service_create_test.go`)
surfaced by a full `make ready` run.
**Impact**: Test-only as currently understood (see Fix Approach — production impact unconfirmed). Reliably
reproducible, unlike BUG-098's "not reproducible in isolation" — see Reproduction Steps.

## Problem Description

`destroyCreatedSession` (the shared test-cleanup helper both affected tests use) already does the right
things in the right order: `DeleteSession` RPC, `svc.waitForPendingCleanup()`, then
`session.JoinSessionDriver(inst)` before the test's `t.TempDir()` is torn down. Despite that,
`JoinSessionDriver`'s bounded wait (`stopJoinTimeout`, borrowed from `session/pty_discovery.go`, 10s)
times out on **every single run** of these two tests, not occasionally:

```
WARN JoinSessionDriver: driver goroutine did not exit within timeout; it may still be running session=autonomous-session timeout=10s
```

I initially assumed this was CPU-scheduling contention under a full `-race` suite and widened the
timeout to 30s, then to `driverReadyTimeout + 15s` (45s) as a supposedly-safe derived margin. Both were
wrong: at every timeout value tried (10s, 30s, 45s), the same warning fired on **100% of runs**, with
no variance in timing (each iteration took almost exactly the timeout value, every time). This rules out
"occasionally slow under load" — the driver goroutine does not exit via the normal stop signal at all
for these two tests; something else (a much longer deadline, or a genuine stuck wait) is what eventually
lets it finish, and no timeout short of that actually-governing bound will make the join reliable.

Whether this actually corrupts a *later* test's `t.TempDir()` (the user-visible flake) is a second-order
timing coincidence on top of the first bug: it only manifests when some other test's temp dir happens to
still be in the leaked goroutine's path at the moment it finally touches the tmux exec-gate directory.
Running only these two tests in a tight `-count=N` loop in isolation almost never reproduces the
TempDir corruption (no other test's dir to collide with) even though the underlying non-exit is 100%
reproducible — this is why earlier isolated-retest verification during the same session looked "clean"
(13-27 consecutive passes) despite the real bug being present every time.

## Reproduction Steps

```sh
env -u STAPLER_SQUAD_TEST_DIR -u STAPLER_SQUAD_INSTANCE -u STAPLER_SQUAD_USE_STREAM_HUB \
  go test -race -short -timeout=10m ./server/services/... \
  -run 'TestCreateSession_Autonomous_CreatesDirectoryInBaseDir|TestCreateSession_EmptyPath_Autonomous_PassesPathValidation' \
  -count=8 -v
```

Every iteration logs the `JoinSessionDriver` timeout warning, regardless of the constant's value.
To see the actual TempDir corruption (not just the underlying non-exit), run the *full* package suite
repeatedly instead of just these two tests in isolation, so a later test's temp dir is available to
collide with.

## Fix Approach (not attempted — needs dedicated investigation)

1. Confirm whether the driver goroutine started for these tests is the regular one
   (`session.StartSessionDriver`, called unconditionally from
   `server/services/session_creation_pipeline.go:306`) — confirmed via code reading that the
   Autonomous-specific `AutonomousDriver`/`autonomousSvc` path is *not* involved here, since these
   tests' `headlessPool` is nil (logged: `"autonomous_mode requested but headlessPool is nil"`), so
   only the regular driver starts.
2. Instrument (temporarily) or trace exactly which branch inside `runSessionDriverWithPrompt`'s loop
   the goroutine is blocked in when `stop` should already be closed — `Instance.Destroy()` calls
   `StopSessionDriver(i)` unconditionally and sets `i.destroyed` first (session/instance.go:1790-1821),
   and the poll loop checks `i.destroyed` every `driverPollInterval` (2s), so a clean stop should be
   visible within ~2s, not >45s. Something is preventing either the stop signal from reaching this
   specific goroutine, or the goroutine from reaching its stop-check at all.
3. Consider whether `CreateSession`'s async background-resolution pipeline (which is what actually
   calls `StartSessionDriver`, per `session_creation_pipeline.go:306`, from a `trackCleanup`-tracked
   goroutine started by `server/services/session_service.go:2636`) races the test's immediate
   `DeleteSession` call: if `StartSessionDriver` runs *after* `Destroy()` already set
   `driverDestroyed`, the CAS guard should refuse to start a new driver goroutine at all — worth
   confirming this guard is actually reached before assuming the timing race is elsewhere.
4. Once root-caused, fix at the actual blocking point rather than widening any timeout further —
   confirmed during this investigation that "wait longer" does not fix this class of failure, it only
   changes which timeout value happens to also not be enough.

## Related

- `docs/bugs/open/BUG-098-crosstest-testdir-pollution-from-leaked-tmux-pipeline-goroutine.md` — same
  underlying "goroutine outlives its test, touches a later test's TempDir" symptom class, different
  leak site (an untracked async pipeline goroutine vs. this bug's SessionDriver goroutine, which *is*
  joined via `JoinSessionDriver` but still doesn't exit in time).
- `.claude/skills/fix-flaky-tests-dont-defer/SKILL.md` — filed per this repo's standing rule: reliable
  reproduction was achieved (unlike BUG-098), but the actual root cause inside the driver goroutine's
  stop path needs dedicated investigation beyond what fit in the session that surfaced it.
- `session/session_driver.go`'s `JoinSessionDriver`/`stopJoinTimeout`, `session/instance.go`'s
  `Destroy()`, `server/services/session_creation_pipeline.go:300-322`.
