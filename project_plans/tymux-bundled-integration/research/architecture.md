# Architecture Research: tymux-bundled-integration

## (a) tmux's supervision pattern — the template

All file:line references are against this worktree's current `session/tmux/tmux.go` (3296
lines total) unless noted.

**Server-existence check.** `checkServerNotRunning(serverSocket string) bool`
(`session/tmux/tmux.go:492-500`) runs `tmux list-sessions` with a 5s timeout and classifies
the combined stdout+stderr via `serverNotRunning(output []byte) bool`
(`session/tmux/tmux.go:334-339`), which pattern-matches three tmux error strings
("no server running", "error connecting to", "server exited unexpectedly"). This is the
low-level "is the companion process alive" primitive — analogous to what a tymuxd health
check needs to do, except tmux has no dedicated health RPC and instead infers liveness from
a real command's failure text.

**Start-if-not-running, idempotent.** `EnsureServerRunning(serverSocket string) (TmuxServerReady, error)`
(`session/tmux/tmux.go:669-708`) is the top-level entry point:
1. `checkServerNotRunning` first — if the server is already up, returns immediately
   (`TmuxServerReady{}, nil`), no subprocess spawned.
2. Otherwise runs `tmux start-server` via `ensureServerRunningWithRetry`
   (`session/tmux/tmux.go:647-664`), which retries `serverStartAttempt`
   (`session/tmux/tmux.go:624-640`) up to `serverStartAttempts` (8) times with exponential
   backoff (`serverStartBackoffStart` 100ms → `serverStartBackoffMax` 3s,
   `session/tmux/tmux.go:618-622`). Each retry re-checks `checkServerNotRunning` before
   surfacing a failure, because a `start-server` call can itself race a genuinely-running
   server under load (`startServerSucceededDespiteError`, `session/tmux/tmux.go:592-594`) —
   the function's actual contract is "a server is running when this returns," not "this call
   started it."
3. On success, sets the server-wide `remain-on-exit on` default
   (`session/tmux/tmux.go:696-705`) so sessions don't get destroyed when their wrapped
   program exits.

**Proof-token gating.** `TmuxServerReady` (`session/tmux/tmux.go:580-584`) is a zero-size
struct returned only by `EnsureServerRunning`, and `server.BuildRuntimeDeps` requires it as
its first parameter — a compile-time enforcement that no session gets restored before the
server is confirmed running (see `main.go:358` call site, `runtime` phase). This is a strong
precedent worth reusing verbatim for tymuxd: a `TymuxdReady{}` proof token returned by an
`EnsureDaemonRunning`-equivalent, threaded into whatever tymux-backed restore path needs it.

**Socket/target identity.** `Socket` (`session/tmux/tmux.go:509-537`) is a newtype wrapping
the tmux `-L` socket name, with `ResolveSocket(explicit string) Socket`
(`session/tmux/tmux.go:563-571`) as the single choke point that also handles test-mode
isolation. This is the template for "identify which instance of the companion process a
command targets" — relevant if tymuxd ever needs per-test-isolation the way tmux's `-L` does
(today it doesn't: `TYMUXD_ADDR` is a single fixed/env-overridable address, no isolation
concept — see (b) below).

**Shutdown-adjacent (not App.OnStop).** tmux's server is deliberately *not* stopped via the
warren `App`'s `OnStop` mechanism (see (e) below) — the `--tmux-keep-server` flag
(`main.go:59,385,784`) and `SetExitEmpty`/`CreateKeepaliveSession`
(`session/tmux/tmux.go:772-819`) instead make the tmux server *outlive* the stapler-squad
process on purpose, so that `--tmux-keep-server` restarts don't kill live sessions
(`.claude/docs/tmux-keep-server-on-restart.md`). `KillOrphanedControlModeClients`
(`session/tmux/tmux.go:720-752`) is a *startup*-time reconciliation step (kills leftover `-C`
clients from a prior process instance), not a shutdown step. **This means tmux is not
actually a template for "stop on shutdown"** — that requirement in the tymux project's
success metrics is new territory, not a mirrorable pattern. See (e).

## (b) tymux gRPC client/transport — current shape and gaps vs. the template

`session/backend_tymux.go` (140 lines): `TymuxBackend` is a pure one-line-forward wrapper
around `tymux.TymuxManager` (compile-time check `var _ ProcessManager = (*TymuxBackend)(nil)`
at line 140) — every method just calls `b.mgr.Xxx(...)`. No supervision logic lives here
today.

`session/tymux/transport.go` (149 lines):
- `rpcTransport` interface (`transport.go:26-37`) — narrow seam over the generated Connect-Go
  client, scoped to only the RPCs `tymuxGRPCSession` calls (`CreateSession`, `KillSession`,
  `ListSessions`, `ReviveSession`, `CapturePane`, `Attach`). Explicitly documented
  (`transport.go:23-25`) as *the* real testability seam, not `TymuxManager` — correctly
  scoped per the interface-pollution checklist (defined for/consumed by exactly one real
  concrete need, `tymuxGRPCSession`, with a fake substitutable for tests).
- `tymuxdAddr() string` (`transport.go:108-119`) resolves `TYMUXD_ADDR` env var, else
  `defaultTymuxdAddr = "http://127.0.0.1:7419"` (`transport.go:106`). Doc comment explicitly
  states the current no-supervision decision: *"stapler-squad does not start or supervise
  tymuxd itself (Story 2.2.6's documented scope decision — no ensureServerRunning-equivalent)... assumes an out-of-band, already-running daemon."*
- `newH2CClient()` (`transport.go:128-137`) builds a plaintext-HTTP/2 (h2c) client — tymuxd
  is loopback-only, no TLS, "loopback-trust security model."
- `NewRealTransport(addr string) rpcTransport` (`transport.go:143-149`) is the only
  constructor of a live transport; `addr == ""` falls through to `tymuxdAddr()`.

**Gaps versus tmux's template — nothing to extend, only to add:**
- No equivalent of `checkServerNotRunning`/`serverNotRunning`: no health-check call at all.
  `ErrTymuxdUnreachable` (`session/tymux/errors.go:10-24`) and `classifyRPCError`
  (`errors.go:33-41`) exist, but they classify an RPC's *failure* after the fact (via
  Connect-Go's `connect.CodeUnavailable`) — a passive signal only useful once some other code
  path already tried to talk to tymuxd. There is no analogue of an active
  "is tymuxd alive" probe independent of a real business RPC.
- No equivalent of `EnsureServerRunning`: nothing starts a `tymuxd` process, checks its exit
  status, retries a failed launch, or returns a readiness proof token.
- No supervision-relevant subprocess handling at all (`os/exec`, PID tracking, log capture)
  exists anywhere in `session/tymux/*` — the package is 100% client-side gRPC plumbing today.
- `session/tymux/errors.go:16-19`'s doc comment is the canonical citation for Story 2.2.6's
  "deliberate scope decision" that this project must explicitly supersede — both this file's
  comment and `transport.go:110-113`'s need updating (not silently orphaned) once supervision
  lands, per requirements.md's "Story 2.2.6... explicitly revisited and superseded" success
  metric.
- No per-instance/test-isolation concept analogous to tmux's `Socket`/`-L`: `TYMUXD_ADDR` is
  one address, process-wide. If bundling makes stapler-squad *own* the tymuxd process (rather
  than assume an out-of-band one), a supervised-daemon design should probably pick its own
  listen port (or a unix socket) per stapler-squad instance the way `STAPLER_SQUAD_INSTANCE`
  already isolates tmux socket names and state dirs (see CLAUDE.md's State Isolation
  reference) — worth flagging for the plan phase, not decided here.

## (c) The `ProcessManager` interface and `RegisterBackendProvider` wiring

`session/process_manager.go`:
- `ProcessManager` interface (lines 8-68) — the actual consumer-side interface, correctly
  placed in the *consuming* package (`session`), not next to `TmuxBackend`/`TymuxBackend`'s
  own packages. Two production implementations already exist (`TmuxBackend`,
  `TymuxBackend`) plus `NativeProcessManager` — **this interface is legitimately justified**
  (3 implementations), not a speculative one-off.
- `ProcessManagerBackend` (line 71) is a `string` newtype with three constants: `BackendTmux`,
  `BackendNative`, `BackendTymux` (lines 74-76).
- `ProcessManagerOptions` (lines 80-92) carries `Backend ProcessManagerBackend` (line 91) —
  "when non-empty, overrides both the process-wide default... and NewProcessManager's
  defaultBackend argument for this one call... Zero value means no per-session override."
  **This field is currently dead in production**: a repo-wide grep for `.Backend = ` finds no
  assignment site outside `session/instance.go`'s own struct literal
  (`instance.Backend: opts.Backend` at `instance.go:865`) and `InstanceOptions.Backend` is
  never populated by `server/services/session_service.go`'s `CreateSession` (line 1799) or
  any other request-handling path — confirming requirements.md's claim verbatim.

`session/backend_factory.go`:
- `RegisterBackendProvider(backend ProcessManagerBackend)` (lines 29-33) sets a package-level
  `selectedBackendValue` (mutex-guarded, lines 22-25), called exactly once at process startup:
  `main.go:167-175`, reading `cfg.ProcessManagerBackend` (a plain `string` config field,
  `config/config.go:417-420`, defaulting to `""` → `BackendTmux`).
- `NewProcessManager(ctx, defaultBackend, opts) (ProcessManager, error)` (lines 57-79) is the
  single choke point resolving precedence: `opts.Backend` (per-call override) →
  `getSelectedBackend()` (process-wide global) → `defaultBackend` (caller's fallback) →
  `BackendTmux` (final default). An unrecognized non-empty value returns
  `ErrUnrecognizedBackend` rather than silently falling back (lines 12-20, Story 2.1.3 /
  UX-9.2) — fail loudly on a typo'd/corrupted persisted value.
- `newTymuxBackendFromOpts` (lines 81-91) always constructs a fresh
  `tymux.NewTymuxGRPCSession(tymux.NewRealTransport(""))` — no supervision call anywhere in
  this path today.

**Consumer call sites of `NewProcessManager` that thread `ProcessManagerOptions.Backend`**
(all pass `instance.Backend`, i.e. the per-session persisted field, already correctly wired
through — only *populating* that field is missing):
- `session/instance_tmux.go:134` (primary construction path, `i.pm()`-equivalent)
- `session/instance.go:913`
- `session/instance_serialization.go:334`
- `session/external_discovery.go:168`

**Interface-pollution read on this layer:** `ProcessManager` itself is fine (3 real
implementations, defined in the consumer package). `RegisterBackendProvider` +
`getSelectedBackend()` is a plain package-level var with a mutex, not a speculative
interface — correctly concrete. No new interface is obviously needed here for supervision;
see (f) for the supervision-specific risk.

## (d) Where per-session override and global-default resolution should plug in

**Two distinct "global default" mechanisms already exist in this codebase and this project
must decide which one tymuxd's flag follows — they are not interchangeable:**

1. **`ProcessManagerBackend` (session package)** — a package-level var set *once* at process
   startup (`RegisterBackendProvider`, `main.go:167-175`), read synchronously by
   `NewProcessManager` at each session's construction time. No live env-var re-read, no
   rollback-rehearsal gate, no session-name override map. This is the existing "coarse
   selector" requirements.md refers to.
2. **`STAPLER_SQUAD_USE_STREAM_HUB` (streamhub rollout mechanics)** — read live via
   `os.Getenv` at each resolution point (`server/services/connectrpc_websocket.go:392`,
   duplicated in `session/instance_tmux.go:876` as `effectiveStreamHubFlag()` — duplicated,
   per that function's doc comment, because `package session` cannot import
   `server/services`, the one-way dependency ADR-003 establishes), gated through
   `config.ResolveGlobalStreamHubDefault` (`config/config.go:463-471`) which refuses to let
   the global flip `true` unless `cfg.RollbackRehearsalCompletedAt` is set
   (`config.go:435-443`, `RecordRollbackRehearsalCompleted` at `config.go:480-484`). Resolved
   **once per session, sticky for that session's lifetime**, via
   `StreamOwnershipLock.Resolve` (`session/streamhub/ownership.go:100-124`) — a *deliberate*
   design so a mid-rollout flag flip can't split one session across two owners (see
   `ownership.go:23-33`'s ADR-003 reference).

**Per-session override, streamhub shape:** `config.GetStreamHubSessionOverride`/
`SetStreamHubSessionOverride` (`config/config.go:486-517`) back a
`map[string]bool` (`StreamHubSessionOverrides`, `config.go:426-434`) keyed by session name,
consulted inside `resolveLocked` (`ownership.go:113-118`) via a package-level
`sessionOverrideLookup` function pointer installed once at startup:
`streamhub.SetSessionOverrideLookup(...)` at
`server/services/connectrpc_websocket.go:409` (the *only* production call site).

**Recommended plug-in point for tymux, reconciling both mechanisms:**

Because `ProcessManagerOptions.Backend`/`Instance.Backend` is *already* a per-session,
persisted field (unlike streamhub's separate override map, which exists precisely because
`StreamOwnershipLock` had no persisted per-session field to reuse), the natural design is
**resolve-once-at-creation, write into `Instance.Backend`, never re-resolve** — not a live
lookup function consulted on every `NewProcessManager` call:

- At session-creation time — `server/services/session_service.go`'s `CreateSession`
  (line 1799), the same place that today never populates `InstanceOptions.Backend` — resolve
  the effective backend once, in this precedence order: (1) an explicit per-request
  override (however the plan phase decides to surface it — e.g. a `CreateSessionRequest`
  field, mirroring how `StreamHubSessionOverrides` is keyed by name but decided in advance
  of creation, not after), (2) a new config-backed session-name override map analogous to
  `StreamHubSessionOverrides` (e.g. `TymuxSessionOverrides map[string]bool` +
  `Get/SetTymuxSessionOverride`, same shape as `config.go:486-517`) for the "force this one
  disposable session onto tymux without flipping the global" canary use case named in
  requirements.md, (3) the rehearsal-gated global default (see below), (4) `BackendTmux`.
- Write the resolved value into `InstanceOptions.Backend` → `Instance.Backend` — this makes
  pinning automatic and free: because `NewProcessManager` is only ever called again later with
  `opts.Backend = instance.Backend` (already true at all 4 call sites listed in (c)), a
  session created under `BackendTymux` stays on `BackendTymux` for its entire lifetime even if
  the global default later flips back to tmux. **No new "existing session survives a global
  flip" mechanism needs to be built — it already falls out of the existing
  `ProcessManagerOptions.Backend` precedence rule once the field is actually populated at
  creation.** This answers requirements.md's open question about consistency requirements:
  yes, pin at creation, and the persisted `Instance.Backend` field is sufficient by itself.
- **Global default resolution** — requirements.md's open question ("does the global flag
  become the rehearsal-gated version of `process_manager_backend`, or a new gate that then
  chooses whether `process_manager_backend: tymux` is even honored?"). Given
  `RegisterBackendProvider` is a set-once-at-startup package var (mechanism 1 above) while the
  rehearsal-gate machinery (`ResolveGlobalStreamHubDefault`) is designed around a *live,
  per-resolution* env-var read (mechanism 2), bolting the rehearsal gate directly onto
  `RegisterBackendProvider`/`cfg.ProcessManagerBackend` is the closer fit architecturally: it
  would become `main.go:167-175`'s existing block calling something like
  `config.ResolveGlobalTymuxDefault(cfg, requested)` before `RegisterBackendProvider`, still
  evaluated exactly once at startup — matching how the *backend* selector already behaves,
  rather than importing streamhub's live-per-call resolution model wholesale. This is a
  recommendation for the plan phase to confirm/adopt or override with documented reasoning,
  not a final decision made here.

## (e) Shutdown sequencing hook point

`pkg/warren/app.go` is the actual lifecycle coordinator (not ad hoc `defer` chains in
`main.go`): `App.Phase(name, fn)` (lines 69-77) registers ordered startup phases;
`App.OnStop(name, fn)` (lines 97-106) registers named cleanup functions, called in **reverse
registration order** by `App.Stop(ctx)` (lines 149-171); `App.Run(ctx)` (lines 183-192) is
`Start` → block on `<-ctx.Done()` → `Stop` with a fresh bounded context.

`main.go:326-437` is the one production `warren.App` instantiation: three phases
(`"core-deps"`, `"service-deps"`, `"runtime"`), then `app.Run(ctx)` at line 437 (the
`ctx` from `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)` at line 70). The
`"runtime"` phase (lines 347-435) is exactly where tmux's own startup supervision runs
today — `tmux.EnsureServerRunning("")` (line 358), `KillOrphanedControlModeClients` (line
372), `CreateKeepaliveSession` (line 378), `SetExitEmpty` (line 386) — all before
`server.BuildRuntimeDeps` (line 394) and `srv.Start` (`a.Go("http-server", ...)`, line 427).

**Critical finding: `App.OnStop` has exactly zero production call sites today** — a
repo-wide grep for `OnStop(` outside `pkg/warren/app.go` and its own doc comment
(`pkg/warren/doc.go:31`) finds nothing. tmux registers **no** stop hook; it relies entirely
on `--tmux-keep-server`/`SetExitEmpty`/the keepalive session to make the tmux server outlive
the process on purpose (see (a)'s last paragraph) — there is no "stop tmux on shutdown" code
to mirror, because tmux deliberately never does that.

**This means tymuxd's "stop on shutdown" requirement (an explicit success metric in
requirements.md) would be the first real production use of `App.OnStop`.** The natural hook
point is inside the same `"runtime"` phase, immediately after an `EnsureDaemonRunning`-style
tymuxd-start call (mirroring `EnsureServerRunning`'s placement at line 358): call
`a.OnStop("tymuxd", func(ctx context.Context) error { return <stop the supervised tymuxd
process> })` right after starting it, so `Stop`'s reverse-registration-order guarantee stops
tymuxd before (or after — order depends on what else gets an `OnStop` hook registered later
in the same phase) other cleanup. Whether tymuxd *should* actually stop on shutdown (matching
requirements.md's success metric) or persist across `--tmux-keep-server`-style restarts like
tmux does is a plan-phase decision — the two existing companion-process precedents in this
codebase (tmux: never stop; nothing else: no other precedent exists) do not agree, so this
must be decided and documented, not defaulted silently to either behavior.

## (f) Interface-pollution / primitive-obsession risk notes

**Speculative-interface risk — "ProcessSupervisor" abstraction.** The natural LLM-shaped
temptation here is a generic `type ProcessSupervisor interface { EnsureRunning() (Token,
error); Stop(ctx) error }` meant to unify tmux's `EnsureServerRunning`/(nonexistent) stop
path and a new tymuxd equivalent. **Do not build this.** Per
`.claude/rules/interface-pollution-checklist.md` smell #1 (speculative interface, exactly
one *near-term* real implementation) and #2 (interface next to its implementation): tmux's
`EnsureServerRunning` is a **package-level function**, not a method on any type, and has
never needed an interface in the ~3300 lines of `session/tmux/tmux.go` that depend on it.
tymuxd's equivalent should be the same shape — a concrete `tymux.EnsureDaemonRunning(...)
(TymuxdReady, error)` package-level function in `session/tymux/`, not an interface satisfied
by both `session/tmux` and `session/tymux`. The two backends' supervision needs differ
enough already (tmux: shell out to an external `tmux` binary already on PATH or embedded via
`go:embed`+extract-to-cache; tymuxd: a Rust binary this project would also need to fetch or
compile, with genuinely different health-check semantics — RPC-based vs. subprocess-exit-code
based) that a shared interface would immediately need backend-specific type assertions or an
awkward lowest-common-denominator method set — the "unjustified generic" smell (#5) in
interface form. If a second Rust/gRPC-backed companion process is ever added to this project,
*then* extract a shared interface from the two real implementations (per the checklist's
correct pattern #1: "write the concrete version first; generalize only once 2+ real call
sites need the identical logic"). Until then, concrete functions in each package.

**Speculative-interface risk — supervision as a new `Manager`/`Supervisor` struct.** A second
temptation: wrap "start/health-check/stop tymuxd" in a `TymuxdSupervisor` struct with
`Start()`/`HealthCheck()`/`Stop()` methods that just forward to `os/exec` calls with no added
behavior — smell #4 (forwarding-only wrapper). tmux's template avoids this entirely: every
function in (a) is a standalone package-level function taking `serverSocket string` and
returning typed results, no wrapping struct. A `TymuxdSupervisor` type is only justified if
it needs to *hold state* across calls (e.g. a `*os.Process` handle, a health-check goroutine's
cancel func, a mutex serializing concurrent start attempts the way `recoveryMu`/
`recoveryInFlight` do at `session/tmux/tmux.go:289-292`) — which a bundled/supervised tymuxd
plausibly does need (unlike the current unsupervised gRPC-client-only code, which holds no
process handle at all). If that state need is real, a concrete `type tymuxdSupervisor
struct{...}` (unexported, single production construction site, no interface) is the correct
shape — not an interface, and not a name ending in `-Manager`/`-Service` layered atop it with
no added behavior of its own.

**Primitive-obsession risk — supervision function signatures.** The most likely place this
bites: a function like `func EnsureTymuxdRunning(host, port, binaryPath string) (...)` — three
bare strings, silently swappable, echoing exactly the `.claude/rules/primitive-obsession-checklist.md`
"Wrong" example (`host, owner, repo, username string`). tmux's own template avoids this
naturally because it only ever threads one string primitive
(`serverSocket`) through its supervision functions — there's no multi-string pile to
disambiguate. tymuxd's design will likely need at least two related-but-distinct string/path
concepts (a listen address such as `TYMUXD_ADDR`'s `"http://127.0.0.1:7419"`, and a binary
path if bundling via a downloaded/compiled artifact rather than assuming `tymuxd` is on
`PATH`) — these should become one named type (e.g. `type DaemonConfig struct { Addr string;
BinaryPath string }` or similar, constructed via a validating `NewDaemonConfig`/resolved via a
single `resolveDaemonConfig()` the way `tymuxdAddr()` already resolves just the address) rather
than accreting as separate same-typed string parameters across `EnsureDaemonRunning`,
`newH2CClient`-equivalent, and any future health-check function. Watch specifically for a
second `string` parameter appearing next to an existing one in any new tymuxd-supervision
function signature during the plan/implementation phases — that is the exact trigger
condition the checklist names.

**Where a real interface likely IS justified, unchanged from today:** `rpcTransport`
(`session/tymux/transport.go:26-37`) already is a correctly-scoped interface (narrow,
consumer-defined-and-adjacent since `tymuxGRPCSession` and `rpcTransport` are both in
`package tymux` and the fake substitutes only in tests) — supervision work should not need to
touch or widen this interface; it stays purely about health/lifecycle of the *daemon process*,
which is orthogonal to `rpcTransport`'s RPC-calling concern.
