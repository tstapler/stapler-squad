# BUG-105: Leaked `go test` fixture tmux servers exhaust the `stapler-squad.service` cgroup, causing OOM kills and deploy rollbacks (SEVERITY: High)

**Status**: ✅ Fixed
**Discovered**: 2026-09-11, investigating a report that `make install-service` deploys on `onyx`
were intermittently timing out and auto-rolling back.

## Problem Description

`testutil/tmux.go`'s `CreateIsolatedTmuxServer` and `session/session_creation_test.go`'s local
`testTmuxSocket` helper each spin up a real `tmux -L test_<TestName>_<pid>[_<n>]` server per
test. Their `t.Cleanup()` only runs if the `go test` binary reaches normal completion — if the
process is killed first (a stapler-squad session stopped/archived mid-`go test`, an OS signal, a
`-timeout` firing), the fixture tmux server is orphaned.

The repo already has infrastructure for exactly this: `testutil/tmuxreap.ReapLeakedTestServers()`
is called unconditionally from `TestMain` in `session`, `session/tmux`, `session/mux`, and
`server/services`, and kills any tmux socket matching a known test-name prefix whose owning PID
is confirmed dead. **The bug was that its prefix allowlist never covered the generic
`test_<arbitrary-test-name>_<pid>` shape** — it only listed specific literal prefixes from a few
named generators (`test_coldrestore_`, `test_recovery_`, etc., plus `test-isolated-` added for an
earlier instance of this same class of bug). An arbitrary test name between `test_` and the PID
means no fixed literal string can name every case, so both `CreateIsolatedTmuxServer`'s and
`testTmuxSocket`'s sockets were silently invisible to a reaper that was already running on every
test invocation.

These orphaned servers accumulated forever inside the `stapler-squad.service` systemd cgroup
(they're descendants of the service's process tree). On 2026-09-11, `onyx` had **882 leaked
`tmux -L test_*` servers**, the oldest running **8.6 days**, contributing to:

- `Tasks: 5309` in the cgroup (limit 76063)
- `Memory: 40.3G` used against a `MemoryMax=55.7G` ceiling, only ~6.4G headroom
- Repeated kernel OOM kills scoped to `task_memcg=.../stapler-squad.service`, visible via
  `journalctl -k`, on 2026-08-22, 09-01, 09-05, 09-07, 09-08, 09-09, and 09-11 — mostly picking
  off headless Chrome instances (browser-automation MCP sessions) as OOM victims, plus a
  `State 'stop-sigterm' timed out. Killing` / `Failed with result 'timeout'` on 09-01.

`scripts/install-service.sh`'s `health_check_and_rollback` (default `HEALTH_TIMEOUT=300`) gives a
freshly deployed binary 300s to answer `/health`. Under this memory pressure the restart either
gets OOM-killed mid-startup or simply can't come up within the window, so the health check fails
and the script auto-rolls back to the previous binary — which does nothing to relieve the
underlying pressure, so the next deploy hits the same wall.

A second, smaller leak category surfaced while verifying the fix: some tmux server *processes*
survive with **no socket file at all** — almost certainly killed mid-shutdown during the same
memory pressure (self-unlinked their socket as part of exiting, then hung before actually
terminating). A purely socket-file-based reaper can never see these.

## Fix

1. **`testutil/tmuxreap/tmuxreap.go`**: broadened `testSocketPrefixes` from a list of specific
   historical generator prefixes to the bare `"test_"` prefix (plus `"integration_"` and
   `"test-isolated-"`). `extractTestSocketPID` + `isProcessAlive` still gate every kill on the
   owning PID actually being dead, so this stays safe against ever touching a real production
   session (which uses the `staplersquad_...` naming scheme, never `test_...`).
2. **`testutil/tmux.go`**: `CreateIsolatedTmuxServer`'s socket name now embeds `os.Getpid()`
   (`test_<name>_<pid>_<n>`, matching the convention `session_creation_test.go` and
   `server/services/session_service.go` already used) so the PID-liveness check has something to
   extract.
3. **`testutil/tmuxreap/tmuxreap.go`**: added `reapOrphanedTestProcesses`, a process-listing sweep
   (via `ps -eo pid=,args=`, portable to macOS) that catches the socket-file-less orphans from the
   second leak category, using the same PID-liveness rule as the file-based sweep.

No session-lifecycle changes were needed — the existing reap-on-every-`TestMain` mechanism was
already correctly positioned; it just wasn't recognizing these socket names. Verified end-to-end
on `onyx`: after the fix, a single `go test ./session/ -run '^$'` invocation (which only runs
`TestMain`, no actual tests) reaped every one of the pre-existing leaked/orphaned test tmux
processes on the machine, both the file-backed and file-less kind.

Coverage: `testutil/tmuxreap/tmuxreap_test.go` (`TestIsTestSocketName_MatchesGenericTestNamePrefix`)
and `testutil/tmux_test.go` (`TestCreateIsolatedTmuxServer_SocketNameEmbedsPID`).

## Immediate Mitigation (done prior to the code fix)

Killed all 882 then-leaked sockets by iterating `ps` for `tmux -L test_*` and running
`tmux -L <socket> kill-server` on each — reduced cgroup tasks 5309→4825 and freed ~1.7G.
Superseded by the fix above, which makes this self-healing going forward.

## Out of scope

The Chrome-process memory pressure (browser-automation MCP sessions) is a secondary contributor
to the same OOM incidents and may deserve its own investigation, but the leaked test tmux servers
were the dominant, unambiguous, always-growing signal and are now fixed.

A more proactive fix — sending session-stop signals that let `go test`'s own cleanup run instead
of relying on the next test run's reap pass — was considered but isn't necessary: the reaper now
correctly and promptly (see verification above) cleans up on the very next test invocation in any
of the packages that already call `tmuxreap.ReapLeakedTestServers()` from `TestMain`.
