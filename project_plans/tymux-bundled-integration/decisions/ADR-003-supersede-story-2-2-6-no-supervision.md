# ADR-003: Supersede Story 2.2.6's "no supervision" decision

**Status**: Accepted (supersedes Story 2.2.6)
**Date**: 2026-08-25

## Context

Story 2.2.6 made a deliberate, documented scope decision: stapler-squad does not start or
supervise `tymuxd` itself. The decision is recorded in two places:

- `session/tymux/transport.go:108-119`'s `tymuxdAddr()` doc comment: *"stapler-squad does not
  start or supervise tymuxd itself (Story 2.2.6's documented scope decision — no
  ensureServerRunning-equivalent)... assumes an out-of-band, already-running daemon."*
- `session/tymux/errors.go:16-19`'s doc comment on `ErrTymuxdUnreachable`, which frames
  connection failures as failures of an externally-managed process.

At the time, this was a reasonable scope cut — Epic 2.2 was about proving the gRPC transport
worked at all, not about process lifecycle. The consequence, confirmed by this project's own
requirements gathering: an operator must `cargo build` the sibling `tstapler/tymux` repo and run
`tymuxd` by hand before the tymux backend is usable, making it effectively untested and unused
outside unit tests that construct `TymuxBackend` directly.

## Decision

**This decision no longer holds.** stapler-squad now supervises `tymuxd`'s lifecycle —
start-if-not-running, health-check, and (opt-out) stop-on-shutdown — whenever the tymux backend
is actually in use: the rehearsal-gated global default resolves to `BackendTymux` (ADR-002), or
any per-session override requests it (Phase 4 of the implementation plan). When nothing in the
current process configuration requests tymux, no daemon is started — this preserves the
zero-dependency property for every operator who never opts in, rather than forcing a `tymuxd`
process onto every stapler-squad instance unconditionally.

**What "health-check" means here, explicitly**: this is **call-before-use at every point tymuxd
is needed**, not an ongoing background poll/restart loop. There are exactly three call sites for
`EnsureDaemonRunning` (updated by ADR-004, which superseded this ADR's original "two call
sites" framing after an adversarial-review pass found the second one as originally placed
created a synchronous-RPC-path latency regression — see ADR-004 for the full mechanics):
`main.go`'s startup phase (tymux needed at process boot — either the global default or a
session override already present before this process started); and, lazily,
`session.TymuxBackend.Start()` and `session.TymuxBackend.RestoreWithWorkDir()`
(`session/backend_tymux.go`), invoked on *every* tymux-backed session's async start/restore —
never from `session.NewProcessManager`, which constructs a `TymuxBackend` cheaply and
synchronously, the same way it already does for `TmuxBackend`. These two `TymuxBackend` methods
cover a per-session override set via `SetTymuxSessionOverride` *after* the process is already
running, with no restart — the scenario the initial version of this plan missed entirely; see
implementation plan Story 2.1.3 and ADR-004. `EnsureDaemonRunning` itself is cheap and
idempotent to call repeatedly — an already-healthy daemon short-circuits in one `ListSessions`
round-trip via `checkDaemonHealthy`, so calling it at every use site costs one bounded RPC per
session start/restore, not a spawn. This is a deliberate choice: no `Health`/`Ping` RPC exists on
tymuxd today to poll against (`research/stack.md` (g)), and `daemon/daemon.go`'s own existing
precedent for this class of problem is start-if-not-running-when-needed, not a standing
supervisor goroutine — introducing one here would add a new goroutine-lifecycle/shutdown-
ordering surface this project has no other reason to need.

**What this means for a crash mid-run**: because supervision is call-before-use rather than an
ongoing poll, a `tymuxd` crash while a session is actively attached is **not proactively
detected or auto-recovered**. What actually happens: the next RPC against the dead daemon (any
of `Start`/`RestoreWithWorkDir`/`Close`/`CapturePane`/`Attach`/`ReviveSession`) fails and is
classified by `session/tymux/errors.go`'s `classifyRPCError` as `ErrTymuxdUnreachable`, with the
underlying transport error preserved via `%w` — a clear, specific failure surfaced to the
caller, never a silent hang and never a generic undiagnosable error. What does *not* happen
automatically: that already-attached session does not get transparently reconnected to a
freshly-restarted daemon. The next brand-new session's async start *will* trigger a fresh
`EnsureDaemonRunning` call (via `TymuxBackend.Start()`/`RestoreWithWorkDir()`, per ADR-004) and
restart `tymuxd` for subsequent sessions — but a live session that was attached at the moment of
the crash surfaces the error and is not itself auto-healed within this plan's scope. See
implementation plan Task 2.1.3c.

The new supervision code (`session/tymux/supervise.go`'s `EnsureDaemonRunning`,
`checkDaemonHealthy`, a PID-file-backed `StopDaemon`-equivalent) is modeled directly on the two
existing precedents this codebase already has for exactly this class of problem:

- `session/tmux/tmux.go`'s `EnsureServerRunning`/`ensureServerRunningWithRetry` — the
  start-if-not-running-with-retry-and-backoff half, including the "this function's contract is
  'a server is running when it returns,' not 'this call started it'" race-tolerant design.
- `daemon/daemon.go`'s `LaunchDaemon`/`StopDaemon` — the detached-subprocess, PID-file,
  `Process.Release()` half.

Both existing citations of Story 2.2.6's decision are updated as part of this work
(`transport.go:108-119`, `errors.go:16-19`) to reference this ADR instead of being left
orphaned — the requirement is to *revisit and supersede with documented reasoning*, not to
silently contradict the prior decision by shipping supervision code next to an unchanged
comment that still claims none exists.

## Consequences

- **New failure classes this project must design against** (full detail in
  `research/pitfalls.md` and the implementation plan's Phase 2): orphan/zombie `tymuxd`
  processes if the Go parent crashes without cleanup (`Pdeathsig` on Linux; no equivalent on
  macOS); port conflicts against a manually-run legacy `tymuxd` (the exact out-of-band workflow
  Story 2.2.6 assumed as the status quo) or another stapler-squad instance; restart tearing down
  live tymux-backed sessions unless the daemon is deliberately kept alive across restarts
  (mirroring `--tmux-keep-server`'s own hard-won lesson,
  `.claude/docs/tmux-keep-server-on-restart.md`).
- **Rollback is not symmetric with streamhub's.** A tymux-backed session cannot be silently
  migrated back to tmux mid-session — the backing process itself would have to change, not just
  an in-process object reference. "Rolling back" the tymux rollout therefore means: the global
  default reverts for *new* sessions only; any session already created under `BackendTymux`
  stays on `BackendTymux` for its lifetime (this falls out of `Instance.Backend`'s existing
  precedence rule once Phase 4/5 of the implementation plan populate and persist it — no new
  migration mechanism needs to be built). The rehearsal procedure for
  `RecordTymuxRollbackRehearsalCompleted` (ADR-002) must exercise *this* definition, not
  streamhub's "flip an override, use it, remove it, confirm clean reconnect" procedure verbatim
  — a tymux-backed session cannot cleanly "reconnect under the legacy path" the way a streamhub
  connection can, so the tymux rehearsal instead verifies: create a disposable session forced
  onto tymux via the per-session override, confirm it works, then flip the *global* default back
  (or remove the override for a *new* disposable session) and confirm the new session is
  created under tmux while the original tymux-backed session is untouched and still functions.
- **First production use of `pkg/warren/App.OnStop`.** A repo-wide grep confirms zero production
  call sites of `OnStop` today — tmux deliberately never uses it (it relies on
  `--tmux-keep-server`/`SetExitEmpty`/the keepalive session to make the tmux server outlive the
  process on purpose). tymuxd's stop-on-shutdown path is genuinely new territory in this
  codebase, not a mirrorable pattern from tmux's own shutdown behavior — the implementation plan
  registers it in the same `"runtime"` phase tmux's own startup supervision runs in
  (`main.go`'s `app.Phase("runtime", ...)`), gated by a new `--tymuxd-keep-server` flag
  defaulting to `true` (matching `--tmux-keep-server`'s default) so the *default* behavior still
  matches tmux's "outlive the process" posture — `OnStop` only actually stops the daemon when an
  operator explicitly opts out.

## Alternatives Considered

**Leave 2.2.6's decision in place; only add a health-check with no start/stop supervision** (an
operator still runs `tymuxd` by hand, but stapler-squad can at least detect whether it's up).
Rejected: does not satisfy the requirements doc's explicit success metric ("An operator can
select the tymux backend... without a manual `cargo build && ./tymuxd &` step") — a health check
alone still requires the exact manual step this project exists to remove.
