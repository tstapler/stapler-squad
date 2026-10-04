# BUG-110: A tymuxd process that stays alive after its TCP listener dies can never be evicted by EnsureDaemonRunning [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-09-12, investigating a live production incident: the deployed instance's
tymuxd process (holding its Unix-socket lock for 7+ hours) had its TCP listener die at some point,
and every tymux-backed session started failing with `connection refused` — confirmed via
`/proc/<pid>/net/tcp` showing no `LISTEN`-state entry for the daemon's port while the process
itself was still alive and holding a healthy Unix-socket lock (`lsof` showed `LISTEN` on the
control socket, `fuser`/`ps` confirmed one process).

## Problem Description

`EnsureDaemonRunning` (`session/tymux/supervise.go`) only called `stopTymuxdFn()` (`StopTymuxd`)
*after* a failed spawn-and-retry cycle, to clean up the process it had *just* spawned. But
`startDaemonAttempt` overwrites `$configDir/tymuxd.pid` with the new attempt's own PID
immediately on spawn (`writeTymuxdPIDFile`) — so once a *pre-existing* stuck daemon (process
alive, TCP listener dead) causes `checkDaemonHealthyFn` to fail, the sequence was:

1. Spawn a replacement — its child process collides with the stuck predecessor's still-held Unix
   socket lock (`tymuxd.sock`) and exits within milliseconds ("another tymuxd is already starting
   against `<path>`" — tymuxd's own message, previously discarded entirely; see BUG-106).
2. The retry loop polls health for ~9s, always failing (nothing new ever came up).
3. `stopTymuxdFn()` runs — but the PID file now names the just-spawned, *already-dead* replacement,
   not the actual stuck process. Killing it is a harmless no-op ("process already finished").
4. The real stuck process is never touched. Every subsequent `EnsureDaemonRunning` call for the
   same `cfg.Addr` repeats this exact cycle forever — a permanent, unrecoverable-without-manual-
   intervention deadlock.

## Fix

`session/tymux/supervise.go`: `EnsureDaemonRunning` now also calls `stopTymuxdFn()` **before**
`startDaemonAttemptFn`, using the PID file's pre-spawn contents (which still correctly name any
stuck predecessor). `StopTymuxd` is idempotent and a no-op when no PID file exists, so this is
safe on every normal cold start too. Combined with BUG-106's fix landing in the same file, the two
sequential `stopTymuxdFn()` calls (before spawn, and after a failed retry cycle) now each target
the process they're actually meant to.

Verified against the real binary, not just mocked unit tests: seeded a real tymuxd bound to the
wrong `TYMUXD_ADDR` but holding the right `TYMUXD_SOCKET_PATH` (reproducing "alive, unhealthy,
holding the lock" exactly), wrote its PID into the config dir's `tymuxd.pid` as if
`startDaemonAttempt` had spawned it, then called the real `EnsureDaemonRunning` — it killed the
stuck process and spawned a healthy replacement
(`session/tymux/supervise_integration_test.go`'s `TestIntegration_EnsureDaemonRunning_RecoversFromStuckAliveDaemon`).
Also manually verified live on a disposable instance before writing the automated test (built a
manual `stapler-squad` instance, pre-seeded a stuck decoy, created a real session, watched it
recover to `SESSION_STATUS_ACTIVE`).

Coverage: `session/tymux/supervise_test.go`'s
`TestEnsureDaemonRunning_should_StopStuckDaemon_BeforeSpawningReplacement` (mocked, asserts call
order) and the real-binary integration test above.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md` (the
related, distinct collision this shares a root system with — different instances, not the same
daemon degrading over time), `docs/bugs/fixed/BUG-111-tymuxd-socket-path-exceeds-sun-len-limit.md`
(a second real bug found writing the integration test for this one).
