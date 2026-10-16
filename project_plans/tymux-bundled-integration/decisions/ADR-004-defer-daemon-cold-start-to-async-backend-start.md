# ADR-004: Defer tymuxd cold-start off the synchronous `CreateSession` RPC path

**Status**: Accepted
**Date**: 2026-08-25

## Context

`implementation/adversarial-review.md`'s re-review of the patched plan (verdict CONCERNS) flagged
that Task 2.1.3a, as originally written, wired `EnsureDaemonRunning` into `NewProcessManager`'s
`case BackendTymux:` branch (`session/backend_factory.go:74`, confirmed at
`session/backend_factory.go:57-79` in this worktree). Three related problems, all from the same
review pass:

1. **Synchronous latency regression on the RPC path itself**
   (`adversarial-review.md:70-95`). `NewProcessManager` is called *synchronously* on the
   `CreateSession` path: `server/services/session_service.go`'s `CreateSession` handler calls
   `session.CreateManagedInstance` synchronously (its own comment at
   `server/services/session_service.go:2344-2347` states "This does NOT start tmux/the process;
   that happens in the async goroutine below"), which calls `NewInstance` → `NewProcessManager`
   synchronously, all *before* the async goroutine (`s.trackCleanup(func() {...})` starting at
   `server/services/session_service.go:2397`, which calls `instance.Start(true)` at line 2405)
   that the tmux backend already relies on to absorb slow work. Once `EnsureDaemonRunning`'s
   spawn-and-retry loop (Task 2.1.2c's tmux-derived constants: 8 attempts, 100ms→3s backoff,
   ~9.1s worst case per `session/tmux/tmux.go:609-617`'s own doc comment) sat inside
   `NewProcessManager`'s `BackendTymux` branch, a cold-start `BackendTymux` session paid that
   entire ~9-11s cost synchronously inside the `CreateSession` RPC call — precisely the "operator
   flips a canary session onto tymux without restarting" scenario Story 2.1.3 exists to fix, and
   a regression against tmux's own behavior, which only ever pays this cost once, at process
   startup, never per session.
2. **No real deadline/cancellation on the `ctx` passed to `EnsureDaemonRunning`**
   (`adversarial-review.md:96-107`). Every non-test call site feeding `NewProcessManager` today
   passes `context.Background()` (`session/instance.go:912`,
   `session/external_discovery.go:168`, `session/instance_serialization.go:334`,
   `session/instance_tmux.go:134,137`) — so a caller giving up could not bound or cancel the
   retry loop, and there was no deadline shorter than the ~9-11s worst case.
3. **Concurrent cold-start race undiscussed and untested**
   (`adversarial-review.md:108-117`). Two sessions created concurrently that both observe a cold
   daemon could both call `startDaemonAttempt`, racing to bind the same port. The design likely
   self-heals (the loser's spawn fails to bind; its own retry loop then finds the winner's daemon
   healthy) but nothing in the plan named or tested this, and it depends on `tymuxd` exiting
   cleanly on a bind failure rather than leaving a wedged process.

## Decision

`EnsureDaemonRunning`'s spawn-and-retry call moves **out of** `NewProcessManager`'s synchronous
`BackendTymux` branch (`session/backend_factory.go`) and **into** `TymuxBackend.Start()` and
`TymuxBackend.RestoreWithWorkDir()` (`session/backend_tymux.go`) — the two `ProcessManager`
interface methods that are only ever invoked from inside the existing async goroutine
`CreateSession` already uses to absorb "starting tmux/the process" (traced concretely:
`server/services/session_service.go:2405`'s `instance.Start(true)` →
`session/instance.go`'s several `i.pm().Start(startPath)` /
`i.pm().RestoreWithWorkDir(startPath)` call sites, e.g. `session/instance.go:1245,1249`). This
mirrors tmux's own contract exactly: `TmuxBackend.Start()`/`RestoreWithWorkDir()`
(`session/tmux_backend.go:45-46`) are the analogous forwarding methods for the tmux backend, and
they too only ever run inside this same async path — `CreateSession` already returns in
milliseconds regardless of backend today; this decision makes that true for `BackendTymux` again
instead of only for `BackendTmux`/`BackendNative`.

Concretely, three changes:

1. **`NewProcessManager`'s `BackendTymux` case reverts to cheap construction only** — symmetric
   with what it already does for `BackendTmux` today (`newTmuxBackendFromOpts` never starts a
   tmux server; it only builds the wrapper struct). It calls `newTymuxBackendFromOpts(opts)` and
   returns immediately, with no daemon health-check or spawn attempt on this path. The `ctx`
   parameter (renamed to `ctx` and used by the now-superseded Task 2.1.3a) reverts to unused
   (`_`), since nothing in this function needs it once the daemon check moves downstream.
2. **`TymuxBackend.Start(dir string) error` and `TymuxBackend.RestoreWithWorkDir(w string) error`
   each call `tymux.EnsureDaemonRunning` before delegating to the wrapped `tymux.TymuxManager`.**
   Neither method takes a `context.Context` today (matching the `ProcessManager` and
   `TymuxManager` interfaces, `session/process_manager.go:12` / `session/tymux/manager.go:24-25`
   — no interface signature change is needed or wanted, since every other backend's
   `Start`/`RestoreWithWorkDir` shares the same signature). Each method constructs its own
   `context.WithTimeout(context.Background(), 15*time.Second)` internally, closing the
   ctx-deadline sub-concern: 15s sits comfortably above tmux's own ~9.1s worst-case constant
   (Task 2.1.2c) reused for the daemon's spawn/retry budget, so a healthy cold-start always
   completes inside the deadline, while a genuinely wedged daemon fails this specific `Start`/
   `RestoreWithWorkDir` call (and therefore this specific session's async creation) at ~15s
   instead of hanging the goroutine forever. On failure, the wrapped error
   (`fmt.Errorf("tymux backend requested but daemon unavailable: %w", err)`) is returned from
   `Start`/`RestoreWithWorkDir` exactly as before — the async goroutine's existing failure path
   (`server/services/session_service.go:2405-2412`: log, `SetCreationProgress` with the error
   text, `ForceStatus(session.Stopped)`, persist, publish a `SessionUpdatedEvent`) already
   handles any `instance.Start(true)` failure this way, tmux-backed or tymux-backed alike — no
   new error-surfacing mechanism is introduced for tymux specifically.
3. **A `golang.org/x/sync/singleflight.Group` guards the spawn attempt inside
   `EnsureDaemonRunning`'s implementation** (`session/tymux/supervise.go`), keyed by
   `cfg.Addr` (not a single shared key — two `DaemonConfig`s with different addresses, e.g. two
   `STAPLER_SQUAD_INSTANCE`s per Task 1.3.1a's port derivation, must not coalesce with each
   other). `golang.org/x/sync` is already a direct, non-indirect dependency of this module
   (`go.mod:208`, `v0.22.0`) and `singleflight` already has a working precedent in this exact
   package family: `session/tmux/tmux.go`'s `existsSF`/`noCacheSF` singleflight groups
   (`session/tmux/tmux.go:146,156`, used at `session/tmux/tmux.go:2589` to coalesce concurrent
   `DoesSessionExist` misses onto one subprocess call) — the same coalesce-concurrent-callers
   shape, applied here to coalesce concurrent daemon-spawn attempts onto one `startDaemonAttempt`
   instead of a plain mutex, since `singleflight.Group.Do` already gives every coalesced caller
   the same result (the spawned daemon's readiness, or the shared error) without extra plumbing.
   Two concurrent `TymuxBackend.Start()`/`RestoreWithWorkDir()` calls that both observe a cold
   daemon now share one spawn attempt and one retry loop, rather than each independently calling
   `startDaemonAttempt` and racing to bind `cfg.Addr`.

**Adaptation note**: the task that produced this ADR anticipated `TymuxBackend.Start`/
`RestoreWithWorkDir` might not exist under those names, or that the async goroutine might call
into the backend differently. Verified against the actual code in this worktree
(`session/backend_tymux.go:30-31`, `server/services/session_service.go:2397-2413`,
`session/instance.go`'s `i.pm().Start`/`i.pm().RestoreWithWorkDir` call sites): both methods
exist under exactly these names, are currently one-line forwards to the wrapped
`tymux.TymuxManager`, and are reached exclusively through `instance.Start(true)` inside the
existing async goroutine — no adaptation to the core mechanism was needed beyond what's described
above.

## Consequences

- **`CreateSession`'s RPC latency for a `BackendTymux` session is now identical to
  `BackendTmux`'s** — fast return (milliseconds), with the actual backend start (and any tymuxd
  cold-start cost) happening in the async goroutine exactly as it already does for tmux. The
  "operator flips a canary session onto tymux without restarting" scenario Story 2.1.3 exists to
  support now gets the same fast-RPC-response UX as every other session-creation path, instead of
  blocking the RPC for up to ~9-11s on first use.
- **A cold tymuxd start failure surfaces through the exact same path an existing tmux async-start
  failure already uses** — `server/services/session_service.go`'s async goroutine transitions the
  session to `session.Stopped`, records the error text via `SetCreationProgress`, persists, and
  publishes a `SessionUpdatedEvent` the UI already listens for (`status`/`creation_progress`
  fields). No new notification/status-transition mechanism was built for tymux specifically.
- **A genuinely wedged daemon now fails at a bounded ~15s instead of hanging the async goroutine
  indefinitely** — the `context.WithTimeout` inside `Start()`/`RestoreWithWorkDir()` bounds the
  worst case to comfortably above the documented ~9.1s retry-budget ceiling, surfaced through the
  same async-failure path above; no live RPC caller was ever blocked on this even before this
  ADR, since the RPC has already returned by the time this code runs.
- **Concurrent cold-starts across two sessions coalesce onto one spawn attempt** via the
  `cfg.Addr`-keyed `singleflight.Group`, closing the previously-untested race: both callers
  observe the same outcome (the coalesced spawn's success or failure) rather than independently
  racing to bind the same port.
- **`NewProcessManager`'s `BackendTymux` branch is symmetric with `BackendTmux` again** — both
  are cheap, non-blocking construction, matching the doc comment `NewProcessManager` already
  carries for every other backend.
- ADR-003's "two call sites" framing (`session/tymux/transport.go:108-119`,
  `errors.go:16-19`'s superseded citations, and ADR-003's own "What health-check means" section)
  is updated by this ADR to name the three real call sites `EnsureDaemonRunning` is invoked from
  once this ships: `main.go`'s `"runtime"` phase (Task 2.2.1b, unchanged — already used a
  phase-scoped `ctx`, not `context.Background()`, so it was never the ctx-deadline concern's
  target), `TymuxBackend.Start()`, and `TymuxBackend.RestoreWithWorkDir()` (both new, replacing
  the single `NewProcessManager` site Task 2.1.3a originally used).

## Alternatives Considered

**(a) Keep the call in `NewProcessManager`, synchronous, and just document/accept the RPC
latency.** Rejected — this is exactly the undiscussed tradeoff the adversarial review flagged
(`adversarial-review.md:94-95`'s own recommendation), and it leaves `BackendTymux` sessions with
a UX regression against `BackendTmux`/`BackendNative` that this project has no reason to
introduce: nothing about tymuxd's cold-start cost requires it to be paid on the RPC path, since
the async goroutine that would otherwise absorb it already exists and is already used by every
other backend.

**(b) Pre-warm tymuxd unconditionally at stapler-squad startup, regardless of the global
flag.** Rejected on two independent grounds. First, it imposes the Rust daemon's startup cost on
every operator, including the large majority who never touch the tymux backend at all — directly
contradicting Story 2.2.1's "zero footprint for everyone who never opts in" property and Epic
2.1's stated goal of call-before-use only when tymux is actually needed. Second, and more
fundamentally, it does not even solve the problem for the specific scenario Story 2.1.3 exists
to fix: an operator whose global default is `BackendTmux` (so unconditional startup pre-warming
never fires for them) but who flips a *per-session* canary override to `BackendTymux` at runtime
via `SetTymuxSessionOverride`, without restarting the process. That operator's daemon still needs
a first-use cold start regardless of whether pre-warming happens at startup for *other*
operators — pre-warming only masks the cold-start cost in the one case where the global default
already happens to request tymux at boot, which is not the case this ADR's fix is for. The
correct fix has to work at the point of actual need (session start), which is exactly what moving
the call into `TymuxBackend.Start()`/`RestoreWithWorkDir()` does, independent of whether the
daemon was already warm from process startup or not.

**(c) A mutex instead of `singleflight.Group` for the concurrent-cold-start guard.** Considered
per the task's "or equivalent" allowance, but rejected in favor of `singleflight`: a bare mutex
around `startDaemonAttempt` would serialize concurrent callers but still leave each one repeating
its own retry/health-check loop after the lock releases (each one has to re-verify health itself,
since a mutex carries no result), whereas `singleflight.Group.Do` gives every coalesced caller
the *same* result (the one spawn attempt's readiness token or error) directly, with less code —
and this codebase already has a working, precedented `singleflight` pattern for exactly this
"coalesce concurrent callers, one of them does the real work" shape
(`session/tmux/tmux.go`'s `existsSF`), so introducing a second, different concurrency primitive
for the same problem shape would be the inconsistency, not the fit.
