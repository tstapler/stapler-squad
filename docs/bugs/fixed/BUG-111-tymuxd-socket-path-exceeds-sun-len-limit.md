# BUG-111: BUG-106's per-instance socket path can exceed Unix domain sockets' SUN_LEN limit [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-09-12, writing `session/tymux/supervise_integration_test.go` (BUG-110's
integration-test coverage): a real `EnsureDaemonRunning` call against a `DaemonConfig.SocketPath`
derived from a test's own (deeply-nested) temp directory failed with the real binary reporting
`Error: failed to create Unix socket at <path>: path must be shorter than SUN_LEN`.

## Problem Description

`resolveSocketPath` (`session/tymux/daemon_config.go`, added fixing BUG-106) placed the socket at
`<config.GetConfigDir()>/tymuxd-sock/tymuxd.sock` — nested directly inside the per-instance config
dir. Unix domain socket paths are kernel-length-limited (`sun_path`, ~108 bytes on Linux,
including the null terminator). `config.GetConfigDir()`'s own length is unbounded in practice: a
long `STAPLER_SQUAD_INSTANCE` name, or (the case that actually surfaced this) a deeply-nested
`STAPLER_SQUAD_TEST_DIR` under a test's own `t.TempDir()` — Go's `t.TempDir()` embeds the full,
often long, test name — can easily push `<configDir>/tymuxd-sock/tymuxd.sock` over that limit.
The failure is silent from stapler-squad's own perspective without BUG-106's output-log capture:
`startDaemonAttempt` doesn't wait for or inspect the child's exit, so this surfaced only as "did
not become healthy after N attempts," indistinguishable from any other spawn failure, until the
real error was readable in `tymuxd-output.log`.

## Fix

`resolveSocketPath` now hashes `configDir` (SHA-256, first 8 bytes hex) into a short, fixed-length
token and places the socket under `os.TempDir()` instead of nesting inside `configDir` itself —
`os.TempDir()/ssq-tymux-<16 hex chars>/tymuxd.sock`, comfortably under the limit regardless of how
long or deep `configDir` is, while staying deterministic (same `configDir` always hashes to the
same path — required for `EnsureDaemonRunning`'s reuse case) and still instance-unique (different
`configDir`s hash to different paths). The directory keeps the same `0700`-owned-by-caller
requirement tymuxd itself enforces (BUG-106); `os.TempDir()` being world-writable doesn't weaken
this, since the specific subdirectory under it is not.

Coverage: `session/tymux/daemon_config_test.go`'s
`TestResolveDaemonConfig_should_StaySafelyShort_When_ConfigDirIsVeryLong` (asserts the resolved
path stays well under the limit even when `configDir` is deliberately very long) plus the existing
`TestResolveDaemonConfig_should_DeriveSocketPathUnderInstanceConfigDir_When_InstanceSet` (updated
to assert the new hash-based shape). The integration tests in
`supervise_integration_test.go` also had to adopt the same short-path pattern for their own
manually-constructed `SocketPath` values (a `t.TempDir()`-based path hits the identical limit) —
see that file's `shortSocketPath` helper.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md` (introduced
the socket path this fixes), `docs/bugs/fixed/BUG-110-stuck-alive-tymuxd-can-never-be-evicted-by-ensuredaemonrunning.md`
(found alongside, during the same integration-test-writing session).
