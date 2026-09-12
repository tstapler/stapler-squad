# BUG-106: tymuxd's process-wide Unix-socket lock blocks concurrent `STAPLER_SQUAD_INSTANCE`s from both using the tymux backend [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-09-12, during an operational-readiness review of the tymux-backend rollout
(feature flag `tymux`, `config.TymuxFeatureFlag`) ahead of considering it for a global default.

## Problem Description

Flipping `SetTymuxGlobalOverride(true)` live on a second, independently-instanced
(`STAPLER_SQUAD_INSTANCE=<name>`) stapler-squad process — exactly the manual-testing workflow this
repo's own `CLAUDE.md` documents ("Manual/interactive testing without touching the live deployed
instance") — made every new session on that instance fail outright:

```
ERROR [session pipeline] async start failed session=<title> err="failed to start new session:
start: tymux backend requested but daemon unavailable: tymux: tymuxd at http://127.0.0.1:7888
did not become healthy after 8 attempts"
```

`session/tymux/daemon_config.go`'s `resolveDaemonAddr` already derives a distinct `TYMUXD_ADDR`
TCP port per `STAPLER_SQUAD_INSTANCE` (CRC32-based, `instanceDaemonPortBase`/`Span`) specifically
so two instances' daemons don't collide — and this *is* effective for the TCP port. But tymuxd's
own control-plane Unix socket is a **separate** concept the Go side wasn't touching at all: running
the binary directly reproduced the real failure immediately:

```
$ TYMUXD_ADDR=127.0.0.1:7888 tymuxd
Error: another tymuxd is already starting against /run/user/1000/tymuxd/tymuxd.sock
(lock file: /run/user/1000/tymuxd/tymuxd.sock.lock)
```

tymuxd defaults this socket to one fixed path per OS user under `$XDG_RUNTIME_DIR`/`$TMPDIR`,
independent of `--socket-addr`/`TYMUXD_ADDR`. A tymuxd already running for the default/shared
instance (bound to `127.0.0.1:7419`) holds that lock, so a second tymuxd for a named instance
(bound to a different port, e.g. `127.0.0.1:7888`) still refuses to start — it spawns, immediately
hits this error, and exits, which is exactly what
`project_plans/tymux-bundled-integration/research/pitfalls.md`'s "State isolation must extend to
the daemon" section predicted could happen ("nothing in it currently isolates a second listening
process... would collide today") — the mitigation it named (derive the daemon's address from the
instance) was implemented for the TCP port but not for this second, independent collision surface.

## Reproduction

1. Have any tymuxd already running for the default/shared instance (`TYMUXD_ADDR` unset →
   `127.0.0.1:7419`) — e.g. from a prior manual test with the `tymux` flag or a
   `TymuxSessionOverrides` entry set.
2. Build and run a second, named instance: `STAPLER_SQUAD_INSTANCE=foo PORT=<other> ./stapler-squad --tmux-keep-server`.
3. `curl -X POST .../session.v1.TymuxRolloutService/SetTymuxGlobalOverride -d '{"forceTymux":true}'`
   against the named instance.
4. Create any session on it. It fails with `SESSION_STATUS_FAILED` and the log line above.

## Fix

`session/tymux/daemon_config.go`: added `DaemonConfig.SocketPath` and `resolveSocketPath()`,
mirroring `resolveDaemonAddr`'s exact instance-scoping shape — `TYMUXD_SOCKET_PATH` env var wins
if set, `""`/`"shared"` instance leaves it empty (tymuxd's own default, unchanged), otherwise
derives `<per-instance config dir>/tymuxd-sock/tymuxd.sock` (using the existing
`config.GetConfigDir()`, the same directory `startDaemonAttempt`'s PID file already lives in — not
tymuxd's own `XDG_RUNTIME_DIR`/`TMPDIR` logic, which isn't instance-scoped).

`session/tymux/supervise.go`'s `startDaemonAttempt` now sets `TYMUXD_SOCKET_PATH` on the spawned
process's environment when `cfg.SocketPath` is non-empty, using tymuxd's own documented override
(`--socket-path`/`TYMUXD_SOCKET_PATH`, confirmed via `tymuxd --help`'s error-message text — it has
no working `--help` output, but the flag names appear in its own error strings).

A second, smaller issue surfaced fixing this: tymuxd refuses to bind a socket whose parent
directory isn't owned by the caller at exactly mode `0700` ("expected uid ... at mode 700"). The
per-instance config dir itself is `0750` (shared with `config.json`/session state), so the socket
lives in a dedicated `tymuxd-sock/` subdirectory created (and `chmod`'d, in case it pre-existed at
the wrong mode) at `0700`.

Verified end-to-end: with a leftover tymuxd already bound to `127.0.0.1:7419` for the shared
instance, a fresh named instance with the fix applied now spawns its own tymuxd on its
instance-derived port, both processes coexist, and session creation reaches `SESSION_STATUS_ACTIVE`
(previously `SESSION_STATUS_FAILED` after 8 failed health-check attempts, ~9s).

Coverage: `session/tymux/daemon_config_test.go` —
`TestResolveDaemonConfig_should_LeaveSocketPathEmpty_When_InstanceUnsetOrShared`,
`TestResolveDaemonConfig_should_DeriveSocketPathUnderInstanceConfigDir_When_InstanceSet` (asserts
both the path and the `0700` directory mode), `TestResolveDaemonConfig_should_PreferTymuxdSocketPathEnvVar_When_InstanceAlsoSet`.

## Out of scope

- The `tymux` global feature flag still defaults to `false` and has no rollback-rehearsal
  enforcement (see the adjacent stale-comment cleanup in `config.go` — `ResolveGlobalTymuxDefault`,
  the function that used to enforce this, was removed in `d0ab13c30` and several comments still
  referenced it). Whether the rehearsal gate should be re-added as actual enforcement, rather than
  an operator-trusted historical record, is a separate product decision, not fixed here.
- A pre-existing, unrelated startup race was also observed while reproducing this (a brand-new
  named instance's first `SaveConfig`/`SaveDiscoveryConfig` calls log a one-time `WARN` because
  `config.GetConfigDirForDir`'s named-instance branch doesn't `MkdirAll` the directory the way its
  test-mode branch does — self-heals once anything else creates the directory). Filed separately,
  not fixed here: see `docs/bugs/open/BUG-107-named-instance-config-dir-not-created-on-first-boot.md`.
