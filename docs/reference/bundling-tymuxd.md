# Bundling tymuxd (Single-Binary, Supervised)

`tymuxd` is the terminal-multiplexer daemon from the sibling repo
[`tstapler/tymux`](https://github.com/tstapler/tymux), consumed by the tymux
`ProcessManager` backend (`session/tymux/`, `session/backend_tymux.go`) as an
alternative to the tmux backend. This doc covers fetching/embedding the
binary, how stapler-squad supervises it, the rollout-safety flags that gate
switching to it, and the accepted security tradeoffs of doing so. It
supersedes the placeholder pointer left in `docs/how-to/bundle-tmux.md`'s
"Bundling tymuxd alongside tmux" section.

See `project_plans/tymux-bundled-integration/decisions/` for the full
ADRs (ADR-001 through ADR-004) referenced throughout.

## Fetching and embedding the binary

Unlike tmux (compiled from a pinned git submodule — see
`docs/how-to/bundle-tmux.md`), `tymuxd` is a multi-crate Rust workspace
this repo has no reason to require a Cargo/rustc toolchain for. Instead, a
pinned prebuilt release binary is fetched from `tstapler/tymux`'s GitHub
Releases and checksum-verified (ADR-001):

```bash
make fetch-tymuxd          # Download the pinned tymuxd release binary (no cargo/rustc needed)
make build-tymuxd-embed    # Confirm it landed in session/tymux/embed/ for -tags embed_tymux
make build-embedded-tymux  # Build stapler-squad with BOTH tmux and tymuxd embedded
```

- The release version is pinned by `TYMUX_VERSION` in the `Makefile`
  (see the file for the current value), with its release-asset SHA-256s in
  `scripts/tymuxd-checksums.txt`. Bump it, the
  `github.com/tstapler/tymux/clients/go` require in `go.mod`, and the cache
  key in `.github/actions/fetch-tymuxd/action.yml` together.
  `tools/ci/check-tymux-pin.sh` (the lint workflow's pin check) fails when the
  Makefile tag and the `go.mod` client pin are not the same tymux commit; the
  checksums file and the action's cache key are not checked, so update them
  by hand.
- `make fetch-tymuxd` shells out to `scripts/fetch-tymuxd.sh`, which resolves
  the release asset for the current `uname -s`/`uname -m`, downloads it, and
  verifies its SHA-256 against a pinned value before extracting
  `session/tymux/embed/tymuxd`.
- `make build-embedded-tymux` is a separate target from `make build-embedded`
  (which embeds tmux alone, tag `embed_tmux`) — it builds with both
  `-tags "embed_tmux embed_tymux"` so existing `embed_tmux`-only consumers are
  unaffected.
- **Windows has no `tymux` release target today** — an `embed_tymux` build on
  Windows simply isn't possible until upstream adds one; this is a scope
  decision (ADR-001), not a defect.
- At runtime, `session/tymux/binary_embedded.go`'s `TymuxdBinary()` extracts
  the embedded bytes to the user's cache directory on first use (SHA-256
  checked against the cached copy so a corrupted/tampered cache file is never
  silently re-executed), mirroring `session/tmux/binary_embedded.go`'s
  `Binary()` shape for tmux.
- `TYMUXD_BIN` overrides the resolved path unconditionally, exactly like
  `TMUX_BIN` does for tmux — see "Escape hatches" below.

## Turning it on: the rollout-safety flow

Two independent mechanisms gate the tymux backend, mirroring the streamhub
rollout's shape (ADR-002 explains why they're evaluated once at startup
rather than live per-call):

### 1. The global default — `STAPLER_SQUAD_USE_TYMUX`

```bash
STAPLER_SQUAD_USE_TYMUX=true ./stapler-squad
```

This is read once at startup (`main.go`'s `resolveStartupBackend`), alongside
`process_manager_backend: "tymux"` in `config.json` — both paths are funneled
through the same `config.ResolveGlobalTymuxDefault` gate, so hand-editing
`config.json` cannot bypass it (this is the exact bypass `ADR-002` closes).
That gate refuses to let the global default resolve to `true` unless a
rollback rehearsal has been recorded:

```go
// config.go
cfg.RecordTymuxRollbackRehearsalCompleted()  // sets TymuxRollbackRehearsalCompletedAt
```

until then, requesting the global default logs a warning and falls back to
tmux — it never silently fails closed without a trace. Restart-only, by
design: no hot-reload, no risk of splitting an in-flight session's backend
mid-life from a live config edit.

### 2. The per-session override — `TymuxSessionOverrides`

For flipping a single canary session onto (or off of) tymux without
restarting the whole process or the rehearsal gate:

```go
// config.go — persisted, keyed by the sanitized tmux session name
// (tmux.NewSessionName), NOT the raw request title
cfg.SetTymuxSessionOverride(sessionName, &forceTymux) // *bool: nil clears, true forces tymux, false forces tmux
```

`session.ResolveSessionBackend` consults this in precedence order: an
explicit per-request override (`CreateSession`'s `BackendOverride` field)
first, then this persisted per-session override (in either direction — it
deliberately does not repeat the streamhub `resolveLocked` bug where a
`false` override only ever pushed the resolved value toward `true`), then the
already-gated global default, then `BackendTmux`. This override path is
**not** subject to the rollback-rehearsal gate — that gate applies only to
the global default (same property as `StreamHubSessionOverrides`).

`TymuxRolloutService.SetTymuxSessionOverride` (the ConnectRPC handler in
`server/services/tymux_rollout_service.go`) is a real, working implementation:
it forwards its request's tri-state `force_tymux` field straight to
`config.SetTymuxSessionOverride`, and `GetTymuxRolloutStatus` reports the
current `TymuxSessionOverrides` map back in full — so an operator-facing
RPC path to set, clear, and read back a per-session override exists end to
end today, not just via a direct `config.SetTymuxSessionOverride` Go call or
the per-request, non-persisted `BackendOverride` field on `CreateSession`.
There is still no React UI surfacing this RPC — `grpcurl`/a future UI pass
remains the reachability path, per requirements.md's scope note that UI work
is only in-scope if needed to make the override "reachable at all."

### Rollback semantics

Rolling back the tymux backend is not symmetric with streamhub's — a
tymux-backed session's underlying process can't be silently migrated back to
tmux mid-session. "Rolling back" means: the global default reverts for *new*
sessions only; any session already created under `BackendTymux` stays there
for its lifetime (falls out of `Instance.Backend`'s existing precedence rule,
no separate migration mechanism). See ADR-003's Consequences for the full
rehearsal procedure this implies.

## Daemon supervision

stapler-squad supervises `tymuxd`'s lifecycle whenever the tymux backend is
actually in use — no operator-run `cargo build && ./tymuxd &` step (ADR-003
supersedes the earlier "no supervision" scope decision from Story 2.2.6).
Supervision is **call-before-use, not an ongoing background poll**: there is
no `Health`/`Ping` RPC on tymuxd to poll against today, so
`tymux.EnsureDaemonRunning` is invoked at exactly three points, each cheap
and idempotent (an already-healthy daemon short-circuits in one
`ListSessions` round-trip via `checkDaemonHealthy`):

1. `main.go`'s `"runtime"` startup phase, when tymux is needed at boot.
2. `TymuxBackend.Start()` (`session/backend_tymux.go`), on every tymux-backed
   session's async start.
3. `TymuxBackend.RestoreWithWorkDir()`, on every tymux-backed session's async
   restore.

Sites 2 and 3 run inside the same async goroutine `CreateSession` already
uses to absorb tmux's own startup cost — never inside `NewProcessManager`'s
synchronous construction path — so a cold-start `BackendTymux` session's
`CreateSession` RPC returns in milliseconds, identical to `BackendTmux`
(ADR-004; this closed a real regression an adversarial review caught in an
earlier version of this plan, where the daemon's ~9–11s worst-case
spawn/retry loop sat directly on the RPC path). A `context.WithTimeout` of
15s bounds each call; concurrent cold-starts against the same `cfg.Addr`
coalesce onto one spawn attempt via a `singleflight.Group`, so two sessions
racing to start tymuxd never both try to bind the same port.

**What this does *not* do**: a `tymuxd` crash mid-session is not proactively
detected. The next RPC against the dead daemon fails and is classified as
`ErrTymuxdUnreachable`; a brand-new session's next async start *will*
trigger a fresh `EnsureDaemonRunning` and restart `tymuxd`, but an
already-attached session is not transparently reconnected to it.

### Keeping the daemon alive across restarts

```bash
./stapler-squad --tymuxd-keep-server=false   # opt out of the default; stop tymuxd on shutdown
```

`--tymuxd-keep-server` defaults to `true`, mirroring `--tmux-keep-server`'s
own default and rationale (see
`docs/explanation/tmux-keep-server-on-restart.md`) — a restart should not tear
down live tymux-backed sessions unless an operator explicitly opts out. When
`false`, the daemon is stopped via `pkg/warren`'s `App.OnStop` hook
(`tymux.StopTymuxd()`) — this is the first production use of `OnStop` in
this codebase; tmux itself never uses it, relying instead on
`--tmux-keep-server`/`SetExitEmpty` to outlive the process on purpose.

If this process only *reused* an already-healthy `tymuxd` it didn't start
(another process, or a previous run, is the owner), no stop hook is
registered for it at all, regardless of the flag — `StopTymuxd()` has no
ownership check beyond a PID file, so stopping a daemon this process doesn't
own could kill one a sibling process still depends on.

### Transport: Unix control socket first, TCP fallback

stapler-squad's tymux client (`session/tymux/transport_select.go`) talks to tymuxd over its Unix control socket by default: the daemon authenticates socket peers (SO_PEERCRED), while its TCP listener on `127.0.0.1:7419` is deprecated and unauthenticated. One selector is shared by every session of a daemon (keyed by address and socket path), so a switch is logged once, not per session.

- **Fallback.** A call refused on the socket (`ENOENT`/`ECONNREFUSED`) moves to TCP if TCP answers, and a call refused on TCP moves back only after the socket answers a probe. The choice is sticky; the socket is re-probed every 30s by one caller at a time, bounded to 500ms. Attach streams already open are not migrated and reconnect through the same choice.
- **`EACCES`/`EPERM` are hard errors.** If the kernel denies `connect()` on the socket, a daemon is listening and does not authorize this user; the client returns `ErrTymuxdSocketAccessDenied` and never falls back to TCP (same rule as upstream `udsdialer.DialWithFallback`). A re-probe that sees EACCES/EPERM while sticky on TCP switches back to the socket so the error surfaces.
- **Overlong socket path.** A path over the platform `sun_path` limit (104 on macOS, 108 on Linux, including the NUL) can never be dialed (`EINVAL`). Squad logs one Warn naming the path and uses TCP only for that daemon; an `EINVAL` seen at dial time also falls back, loudly.
- **Which socket.** Only the socket that belongs to this instance's daemon is used (`STAPLER_SQUAD_INSTANCE`-scoped path, or the shared default daemon's per-user path). With no paired socket the client is TCP-only.
- **Supervisor classification** (`probeDaemon`, socket first):

  | Socket | TCP | Result | Meaning |
  |---|---|---|---|
  | answers | any | Healthy | squad does not need TCP |
  | no socket file (`ENOENT`) | answers | Healthy, one Info "no Unix control socket (older daemon?)" | daemon predates the socket or has not created it |
  | socket file exists, refuses (`ECONNREFUSED`) | answers | Degraded, Warn every 10 min | the control socket died; squad is on the unauthenticated fallback |
  | accepts but silent | any | Busy | inconclusive, never killed; reported as wedged only after 10 min |
  | `EACCES`/`EPERM`, or a `Unauthenticated`/`PermissionDenied` reply | not consulted | AccessDenied | alive, never killed, never "wedged"; one Warn with the socket path and the permission diagnosis |
  | no socket file or refused | refused | NotListening | down |

  AccessDenied counts as alive: `EnsureDaemonRunning` returns ready (never spawns or kills), and callers then see `ErrTymuxdSocketAccessDenied` on their RPCs. A socket path the kernel cannot dial at all (over the `sun_path` limit, `ENOTDIR`, `ELOOP`, `ENAMETOOLONG`) is treated like a missing socket, so TCP is consulted and a dead daemon is respawned rather than reported as unresponsive.

  Any live socket or TCP listener still vetoes stopping the daemon. Watch `tymux_daemon_degraded` (1 while Degraded), `tymux_tcp_listener_up`, `tymux_transport{daemon,transport}` and `tymux_transport_transitions_total{daemon,from,to}`.
- **TCP liveness is informational.** The monitor probes the TCP listener once per tick (only while sessions need the daemon) and publishes `tymux_tcp_listener_up`. It never affects health, and only a definite refusal counts as down (a busy or forbidden listener leaves the gauge alone); a Warn every 10 minutes says other TCP-only clients of the daemon will fail while it is down.
- **Follow-up: `--disable-tcp-loopback`.** tymuxd has `--disable-tcp-loopback` (`TYMUXD_DISABLE_TCP_LOOPBACK`) to run socket-only. Squad does not set it, and enabling it later breaks things that are still keyed on the TCP port:
  - `waitPortFree`/`stopListener` (`session/tymux/version.go`) would see the port "free" immediately, skip the SIGKILL escalation and race the respawn.
  - The `portListening`-based squatter and "control socket unresponsive" classification in `EnsureDaemonRunning` would misfire.
  - Any process that grabs `127.0.0.1:7419` becomes an unauthenticated squatter that squad's TCP fallback could reach.
  - Every TCP-only client of the daemon stops working.

  Re-key those checks on the socket first (socket-gone, not port-free) before turning it on.

## Upgrading and downgrading the running daemon

A running `tymuxd` keeps serving until it is replaced, so a new build that embeds a
different `TYMUX_VERSION` does not change the daemon you are talking to. Settings → Features →
"tymuxd version" (and `GetTymuxRolloutStatus.daemon`) shows both versions and whether replacing
would be an upgrade or a downgrade; `ReplaceTymuxd` does it.

- **Detection** (`session/tymux/version.go`): asks the daemon with the `GetServerInfo` RPC
  (tymuxd 1.4+), which also reports its protocol revision. A daemon that answers `UNIMPLEMENTED`
  (older than 1.4) or is unreachable over gRPC falls back to finding the listener on the
  configured address (`lsof`, then `ss`), resolving its executable, and running `--version`.
  The pid file is deliberately not used: it records only the last daemon this process spawned
  and goes stale.
- **Replacement**: SIGTERM (tymuxd flushes its session records), wait for the port to free, SIGKILL
  as a last resort, then `EnsureDaemonRunning` spawns the configured binary. The direction is
  whatever the configured binary is relative to the running one, so a downgrade is: pin an older
  `TYMUX_VERSION` (or `TYMUXD_BIN`), rebuild, then replace.
- **Live sessions**: a revived pane respawns its program, so replacement is refused with
  `FAILED_PRECONDITION` while any session is live unless `allow_interrupt_live_sessions` is set.
- The embedded binary is extracted write-then-rename, so a newer build can replace the cached file
  while the old daemon still runs from it.

## Debugging daemon churn: who stopped tymuxd, and why

Every stop squad issues is logged at Warn before the first signal, and again when it ends
(`session/tymux/stop_reason.go`). Search `staplersquad.log` for `stopping tymuxd`; an unattributed
SIGTERM with no such line did not come from squad.

| Field | Meaning |
|---|---|
| `pid`, `daemon_age` | The process being stopped and how long it had run (`ps -o etime=`; `unknown` if unreadable). A young age means a recent respawn loop. |
| `reason_kind` | `monitor_recovery`, `squad_shutdown`, `version_replace`, `manual_stop`, `stale_daemon` (EnsureDaemonRunning clearing a dead predecessor), `failed_cold_start` (reaping a replacement that never became healthy). |
| `reason` | Human text plus detail, for example `monitor recovery: 6 consecutive failed probes (threshold 6)` or `version upgrade rollout: from "1.12.0" to "1.13.1"`. |
| `tcp_probe`, `unix_probe` | The last probe results the decision rested on: `healthy`, `degraded`, `busy`, `unhealthy`, `access_denied`, `no_socket_file`, `not_listening`, `not_probed` (the other transport answered first), or `unknown` (none recorded). |

The follow-up line `tymuxd stop finished` carries `outcome` (`exited_after_sigterm`,
`escalated_to_sigkill` after `ignored SIGTERM, escalating to SIGKILL`, `already_gone`,
`signal_failed`) and `elapsed`. The last 50 stops are also kept in memory
(`tymux.RecentDaemonStops()`); no `/actuator` endpoint exposes them yet.

When EnsureDaemonRunning declines to kill a daemon whose listener is still up, the error names
each dial: `TCP dial ... was refused and the Unix control socket accepts connections but does not
answer` (`ErrTymuxdControlSocketUnresponsive`), `TCP dial ... accepts connections but nothing
answered as tymuxd, and the Unix control socket is not listening` (`ErrTymuxdPortSquatted`), or
`TCP dial ... and the Unix control socket both accept connections but neither answered`
(`ErrTymuxdUnresponsive`). A dial only proves a connection was accepted, so "squatted" is claimed
only when the control socket is not listening.

## Escape hatches

Two environment variables let a developer bypass the normal binary
resolution — both are **deliberate, unvalidated escape hatches**, not
validated inputs:

- **`TYMUXD_BIN`** — overrides `TymuxdBinary()`'s resolved path outright. As
  documented on `TymuxdBinary()` itself (`session/tymux/binary_embedded.go`):
  anyone who can set env vars for this process can point it at an arbitrary
  binary. This is an accepted risk for local dev/testing, mirroring
  `TMUX_BIN`'s identical, already-accepted shape in
  `session/tmux/binary_embedded.go` — not a new gap this project introduces.
- **`TYMUXD_ADDR`** — overrides the daemon address/port
  (`session/tymux/transport.go`, `daemon_config.go`), used both to dial an
  existing daemon and, when stapler-squad spawns one itself, to set the
  child's bind address explicitly (never relying on inheritance, since
  tymuxd's own hardcoded default is `127.0.0.1:7419` if `TYMUXD_ADDR` isn't
  set at all). If `TYMUXD_ADDR` is unset but `STAPLER_SQUAD_INSTANCE` is set
  to a named (non-`""`/non-`"shared"`) instance, `ResolveDaemonConfig`
  (`session/tymux/daemon_config.go`) derives a distinct port instead of
  `:7419` — `7420 + crc32(instanceName) % 1000` — mirroring the state
  directory's own per-instance isolation, so two manual/isolated instances'
  `tymuxd`s never collide on the same port by default.

## Security: TCP loopback with no TLS/auth

`tymuxd` is dialed over plain HTTP/2 (`connect.NewClient`) at
`http://127.0.0.1:7419` by default (`session/tymux/transport.go`'s
`defaultTymuxdAddr`) — **no TLS, no credential/token exchange**. This is a
materially weaker boundary than a Unix domain socket: a `0600`-permissioned
socket scopes access to the owning user via the filesystem; a TCP port bound
to `127.0.0.1` is reachable by **any local user on a shared/multi-user
machine**, not just the process that started it.

**Accepted risk, not an oversight.** This mirrors an existing precedent
already accepted in this repo: stapler-squad's own HTTP server on
`localhost:8543` has the same TCP-loopback-no-TLS shape. On a single-user
development machine, this is low risk. It is explicitly **not** safe to treat
as equivalent on a shared dev box or CI runner with multiple concurrent
local users — on such a machine, any other local user can connect to
`127.0.0.1:7419` and issue tymux RPCs (start/attach/capture a pane, etc.)
just as stapler-squad itself does.

**A Unix-socket alternative is out of scope for this project.** Closing this
gap the way ssq-mux's own Unix socket does (filesystem-permission boundary
instead of TCP) would require a change to `tymuxd` itself — the Rust
daemon's own listener. `requirements.md` explicitly excludes changes to
`tymux`'s own upstream behavior from this project's scope. This is a natural
follow-up for the `tstapler/tymux` project to pick up, not something this
plan resolves.

**Port squatting is guarded against, not ignored.** Because nothing stops
another local process from binding `127.0.0.1:7419` first,
`EnsureDaemonRunning` health-checks the actual gRPC protocol (a real
`ListSessions` call), not just whether something is listening on the TCP
port — a non-tymuxd process squatting on the port surfaces as
`ErrTymuxdPortSquatted` rather than stapler-squad silently talking gRPC to
an unverified peer.

## Reference

| Concept | Where it lives |
|---|---|
| Fetch/embed the binary | `scripts/fetch-tymuxd.sh`, `Makefile` (`fetch-tymuxd`, `build-tymuxd-embed`, `build-embedded-tymux`), `session/tymux/binary_embedded.go` |
| Global rollout gate | `config.ResolveGlobalTymuxDefault`, `config.RecordTymuxRollbackRehearsalCompleted`, `STAPLER_SQUAD_USE_TYMUX`, `main.go`'s `resolveStartupBackend` |
| Per-session override | `config.SetTymuxSessionOverride`/`GetTymuxSessionOverride`, `config.TymuxSessionOverrides`, `session.ResolveSessionBackend` |
| Operator RPC surface | `server/services/tymux_rollout_service.go` (`TymuxRolloutService`) |
| Supervision | `session/tymux/supervise.go` (`EnsureDaemonRunning`, `checkDaemonHealthy`, `StopTymuxd`), `session/backend_tymux.go` |
| Keep-alive across restarts | `--tymuxd-keep-server` (`main.go`) |
| Decisions | `project_plans/tymux-bundled-integration/decisions/ADR-001` through `ADR-004` |
