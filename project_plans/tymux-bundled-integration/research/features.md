# Research: feature landscape for tymux-bundled-integration

## (a) streamhub global-flag mechanics (rollout-safety precedent)

Full chain, in order of evaluation:

1. **Env var read** — `server/services/connectrpc_websocket.go:391-399`'s `useStreamHub()`:
   ```go
   func useStreamHub() bool {
       requested := os.Getenv("STAPLER_SQUAD_USE_STREAM_HUB") == "true"
       effective, err := config.ResolveGlobalStreamHubDefault(config.LoadConfig(), requested)
       if err != nil {
           log.Error("streamhub: refusing to enable global default", "error", err)
           return false
       }
       return effective
   }
   ```
   Called fresh per connection (cheap, `config.LoadConfig()` re-reads disk each time) — safe only because the *result* is never trusted directly; it only feeds `StreamOwnershipLock.Resolve`, which caches per-tmux-session on first call (see (b)).

2. **Rollback-rehearsal gate** — `config/config.go:452-471`'s `ResolveGlobalStreamHubDefault(cfg *Config, requested bool) (bool, error)`:
   - `requested == false` always returns `(false, nil)` — turning the flag *off* is never gated.
   - `requested == true` is refused (`(false, ErrRollbackRehearsalNotCompleted)`) unless `cfg.RollbackRehearsalCompletedAt` is a non-nil, non-zero `*time.Time`.
   - `ErrRollbackRehearsalNotCompleted` (config/config.go:450) names the story (3.3.2) in its message text.
   - The gate applies **only to the global default** — the per-session override path (item (b) below) is explicitly unaffected and remains usable even when this returns an error (doc comment, config/config.go:459-462).

3. **Config field** — `config/config.go:435-443`:
   ```go
   RollbackRehearsalCompletedAt *time.Time `json:"rollback_rehearsal_completed_at,omitempty"`
   ```
   `nil` = never completed. A pointer (not a bool) so "unset" and "completed at the zero time" are distinguishable — same tri-state-pointer idiom the file uses elsewhere (see `AutoSpawnReadyItems`, referenced at config.go:504 in `SetStreamHubSessionOverride`'s doc comment).

4. **Recording completion** — `config/config.go:473-484`'s `(c *Config) RecordRollbackRehearsalCompleted() error`:
   ```go
   func (c *Config) RecordRollbackRehearsalCompleted() error {
       now := time.Now()
       c.RollbackRehearsalCompletedAt = &now
       return SaveConfig(c)
   }
   ```
   Persists `time.Now()` and calls `SaveConfig` — writes straight to `config.json`, no separate audit-log table. This is deliberately meant to be called **exactly once**, by hand, after a human has actually executed the rehearsal (doc comment: "intended to be called exactly once, after manually verifying...").

5. **RPC surface** — `server/services/stream_hub_rollout_service.go`'s `StreamHubRolloutService` (concrete type, no interface — the file's own doc comment cites the interface-pollution rule: "a config-backed handler with no second implementation, so a concrete type"). Three RPCs:
   - `GetStreamHubRolloutStatus` — read-only status (env var set?, rehearsal timestamp, session overrides list).
   - `CompleteStreamHubRollbackRehearsal` — calls `cfg.RecordRollbackRehearsalCompleted()`. This is the operator-facing "record the rehearsal happened" action, reachable from the web UI.
   - `SetStreamHubSessionOverride` — see (b).
   - Notably, **the global env var itself is deliberately NOT settable via RPC** (doc comment, lines 21-28): "it's env-var-gated and requires a process restart by design... keeping the final rollout step a conscious operator action rather than a UI toggle that could silently change live terminal-streaming behavior for every connected session."

6. **What "completing a rehearsal" means operationally** (`project_plans/terminal-multi-connection-streaming/implementation/plan.md:653-670`, Story 3.3.2):
   - Task 3.3.2a: create a disposable session, force `PathHubOwned` via the per-session override (not the global flag).
   - Task 3.3.2b: use it briefly, then remove the override (or flip the global flag off) and verify a **new** connection cleanly reconnects under the legacy path — existing connections are guaranteed not to move mid-flight by the sticky-resolution property (Story 3.1.1).
   - Task 3.3.2c: record the outcome as prose in `implementation/validation.md` (pass/fail, date) **and**, only on a pass, call the code path that persists `RollbackRehearsalCompletedAt`. A passing manual rehearsal that never calls the persist step does **not** unblock the global default — the audit trail is deliberately two-part (human-readable prose record + machine-checked gate), not one or the other.
   - **Audit trail in practice**: `project_plans/terminal-multi-connection-streaming/implementation/validation.md:98-100` has a "Rollback Rehearsal" section with an unchecked checkbox ("Not yet executed... Do not check the box above until the rehearsal has actually been run — this is an execution record, not a design placeholder.") — confirming the rehearsal for streamhub itself has never actually been executed yet in this repo (i.e. `STAPLER_SQUAD_USE_STREAM_HUB` cannot currently resolve `true` globally). This is a live example of the gate doing its job, not hypothetical.

7. **A second, separate prerequisite gate** exists alongside the rehearsal: Story 3.3.3's real multi-session reconnect-storm test must also be green before the global flag is ever flipped — named explicitly in the Risk Control section, not just folded into "tests pass" (plan.md:672-699). This is a second, non-code-enforced (test-suite-enforced, not config-enforced) prerequisite worth knowing about as a *pattern* even though tymux's automated storm-test equivalent is out of this project's immediate scope.

## (b) streamhub per-session override mechanics

1. **Config storage** — `config/config.go:426-434`:
   ```go
   StreamHubSessionOverrides map[string]bool `json:"stream_hub_session_overrides,omitempty"`
   ```
   Keyed by tmux session name; an absent key means "no override, use the global default."

2. **Accessors** — `config/config.go:486-517`:
   ```go
   func (c *Config) GetStreamHubSessionOverride(sessionName string) (forceHub bool, ok bool)
   func (c *Config) SetStreamHubSessionOverride(sessionName string, forceHub *bool) error
   ```
   `GetStreamHubSessionOverride` is nil-safe (nil `*Config` or nil map → `(false, false)`), mirroring `GetFeatureFlag`'s existing nil-safe shape (config.go:1333). `SetStreamHubSessionOverride` follows the file's tri-state `*bool` convention: `nil` deletes the entry (falls back to global default), non-nil `false` pins to the legacy path regardless of the global default, non-nil `true` forces the hub path. Every call persists via `SaveConfig`.

3. **Decoupling mechanism** — `session/streamhub/ownership.go:62-86`:
   ```go
   var sessionOverrideLookup atomic.Pointer[func(sessionName string) (forceHub bool, ok bool)]

   func SetSessionOverrideLookup(lookup func(sessionName string) (forceHub bool, ok bool)) {
       if lookup == nil {
           sessionOverrideLookup.Store(nil)
           return
       }
       sessionOverrideLookup.Store(&lookup)
   }
   ```
   `session/streamhub` never imports `package config` — it only holds a function-pointer indirection, set once by whoever *does* own config access. This is the same one-way-dependency shape ADR-003 establishes for `AcquireOwnershipLock` (ownership.go:46-48 doc comment): package `session/streamhub` must not import package `session` either, to avoid an import cycle (`session` → `session/streamhub`, one-way).

4. **Wiring the lookup** — `server/services/connectrpc_websocket.go:401-412`:
   ```go
   func init() {
       streamhub.SetSessionOverrideLookup(func(sessionName string) (bool, bool) {
           return config.LoadConfig().GetStreamHubSessionOverride(sessionName)
       })
   }
   ```
   Wired in a package-level `init()` inside `server/services` — the package that *does* import both `config` and `session/streamhub` — so this is a one-time process-startup wiring step, not something re-registered per request. Re-reads `config.LoadConfig()` on every lookup call rather than caching (doc comment: "matching `GetFeatureFlag`'s existing re-read-every-time convention... a session's own override can be changed without restarting the process").

5. **Where the lookup is actually consulted** — `session/streamhub/ownership.go:100-126`, `StreamOwnershipLock.resolveLocked`:
   ```go
   func (l *StreamOwnershipLock) resolveLocked(flagValue bool) StreamPath {
       if !l.resolved {
           effective := flagValue
           if lookup := sessionOverrideLookup.Load(); lookup != nil {
               if forceHub, ok := (*lookup)(l.sessionName); ok && forceHub {
                   effective = true
               }
           }
           if effective {
               l.path = PathHubOwned
           } else {
               l.path = PathLegacyPerConnection
           }
           l.resolved = true
       }
       return l.path
   }
   ```
   Consulted **once**, at the first connection's attach time, inside the same mutex-guarded resolve-once-and-cache step that makes the whole design flip-safe — a session's resolution is sticky for its lifetime regardless of what the override or the global flag do afterward. Note: this override can only force the path **on** (`forceHub && ok` sets `effective = true`); it cannot force `false` to override an `effective = true` from the global flag in `resolveLocked`'s literal code — however `SetStreamHubSessionOverride`'s doc comment (config.go:501-504) describes a `false` override as "explicitly pins the session to the legacy path regardless of the global default," which is a **documented mismatch worth flagging**: as literally written, `ok && forceHub` only ever pushes `effective` to `true`, never forces it back to `false` when the global flag is already `true`. This is either a latent bug or the plan.md ACs (which only test override=true-vs-global=false) never exercised the other direction — worth a note for whoever builds the tymux equivalent to get right rather than copy verbatim.

6. **Trigger point for session creation / session-creation-time consultation**: the streamhub override is *not* resolved at session-creation time — it's resolved lazily, at first-*connection*-attach time (`AcquireOwnershipLock(sessionName)` + `Resolve`), not when the session/instance itself is constructed. This differs from what the tymux per-session override needs: the requirements.md explicitly wants the tymux override consulted "at session-creation time" (into `ProcessManagerOptions.Backend`), because the backend choice affects which `ProcessManager` implementation gets constructed inside `NewInstance` — a decision `NewProcessManager` already makes once, at construction (see (c)), not on each new connection. So the tymux per-session override's consultation point structurally has to plug in at `session.NewInstance`/`InstanceOptions` construction (or a shim that populates `InstanceOptions.Backend` from a config lookup before calling `NewInstance`), not by mirroring `SetSessionOverrideLookup`'s per-connection-resolve shape verbatim.

## (c) `ProcessManagerOptions.Backend` / `ProcessManagerBackend` — confirmed dead in practice, but the plumbing already exists end-to-end

This is **not** an unwired field with no consumer — it's fully plumbed from `Instance.Backend` through to backend selection. What's actually missing is a **producer**: nothing in production code ever sets a non-empty value into it.

- **Enum** — `session/process_manager.go:70-77`:
  ```go
  type ProcessManagerBackend string
  const (
      BackendTmux   ProcessManagerBackend = "tmux"
      BackendNative ProcessManagerBackend = "native"
      BackendTymux  ProcessManagerBackend = "tymux"
  )
  ```
- **Field** — `session/process_manager.go:80-92`, `ProcessManagerOptions.Backend ProcessManagerBackend` — doc comment already describes the intended precedence (per-session override wins over process-wide default).
- **Precedence logic** — `session/backend_factory.go:57-79`, `NewProcessManager`:
  ```go
  backend := opts.Backend
  if backend == "" { backend = getSelectedBackend() }   // RegisterBackendProvider's global
  if backend == "" { backend = defaultBackend }
  if backend == "" { backend = BackendTmux }
  switch backend {
  case BackendTmux:  return newTmuxBackendFromOpts(opts), nil
  case BackendNative: return NewNativeProcessManager(opts), nil
  case BackendTymux: return newTymuxBackendFromOpts(opts), nil
  default: return nil, fmt.Errorf("%w: %q", ErrUnrecognizedBackend, backend)
  }
  ```
  This is a fully-implemented, tested precedence chain — `opts.Backend` (per-call/per-session) > `RegisterBackendProvider`'s process-wide global > `defaultBackend` argument > `BackendTmux` fallback. An unrecognized value fails loudly (`ErrUnrecognizedBackend`), not a silent tmux fallback (Story 2.1.3 / UX-9.2, cited in the doc comment).
- **`Instance.Backend` field** — `session/instance.go:208-213`, persists per-instance (`json:"backend,omitempty"`), doc comment: "threaded into `NewProcessManager`'s `ProcessManagerOptions.Backend` at construction. Empty means use the process-wide default."
- **`InstanceOptions.Backend` field** — `session/instance.go:718-720`, copied onto `Instance.Backend` in `NewInstance` (instance.go:865: `Backend: opts.Backend`), which then feeds `NewProcessManager(..., ProcessManagerOptions{Backend: instance.Backend})` at instance.go:913.
- **Every production call site that constructs a `ProcessManager` from an `Instance`** passes `instance.Backend`/`i.Backend` through faithfully:
  - `session/instance.go:913` (construction, inside `NewInstance`)
  - `session/instance_tmux.go:134` (re-selection path, with fallback-to-`BackendTmux`-on-error logging at line 136)
  - `session/external_discovery.go:168`
  - `session/instance_serialization.go:334` (doc comment at line 331: **"`instance.Backend` is not currently persisted in `InstanceData`"** — confirms the backward-compat concern in Task 5 below: on deserialize/restore, `instance.Backend` is always the Go zero value `""`, so every restored session falls through to the process-wide global regardless of what it was created with)
- **Confirmed no production producer** — grepped every non-test `InstanceOptions{` literal in the repo:
  - `server/mcp/tools_github.go:213`, `server/mcp/tools_lifecycle.go:162`
  - `server/services/session_service.go:1310` (`CreateDirectorySession`), `:1362` (`CreateWorktreeSession`), `:2291` (the main `CreateSession` RPC handler)
  - `session/instance_checkpoint.go:189`, `session/import_commit.go:138`
  
  None of these seven set a `Backend:` field. Grep for `.Backend =` (assignment) across the whole non-test tree returns zero hits. So in production, `InstanceOptions.Backend` (and therefore `Instance.Backend`, and therefore `ProcessManagerOptions.Backend`) is **always the empty string**, and `NewProcessManager` always falls through to `getSelectedBackend()` — the process-wide `RegisterBackendProvider` value set once in `main.go:167-174` from `cfg.ProcessManagerBackend`. This matches requirements.md's framing exactly: the selector mechanism exists and is well-designed, but nothing wires a config-backed per-session override into it yet. Building the tymux per-session override is therefore "add one producer" (a config-backed lookup consulted at `InstanceOptions` construction time in the session-creation call sites above), not "invent the precedence chain" — that part is done and tested.

- **Global selection today** — `main.go:167-174`:
  ```go
  backend := session.ProcessManagerBackend(cfg.ProcessManagerBackend)
  if backend == "" {
      backend = session.BackendTmux
  }
  session.RegisterBackendProvider(backend)
  ```
  Read exactly once, at startup, from `cfg.ProcessManagerBackend` (`config/config.go:417-420`, a plain `string`, no env var, no rehearsal gate, no rollback mechanics at all today). This is the "coarser existing selector" requirements.md's Open Questions ask to reconcile with the new rehearsal-gated flag — currently it's a bare string with no safety rails whatsoever, unlike streamhub's env-var + gate + audit-trail design.

## (d) Edge cases / failure modes the plan must address

1. **tymuxd crashes mid-session (existing sessions using it)** — `TymuxBackend` already has some awareness of this class of failure: `session/backend_tymux.go:126-137` forwards `ReconnectState()` (is the standing Attach stream reconnecting, which attempt, what triggered it) and `BackendRestarted()` (was tymuxd detected to have restarted out from under this session — "the pane's original process is orphaned, not reattached", per Story 2.5.3's daemon-restart contract, doc comment at backend_tymux.go:132-134). This means the *transport layer* already has restart-detection machinery; what's missing is the *supervision* layer (detecting the crash and restarting the process itself) — today an operator has to notice and re-run `tymuxd` by hand (Story 2.2.6's explicit scope decision, `session/tymux/transport.go:108-119`). The new supervisor needs to decide: does it poll/health-check tymuxd and auto-restart it, and if so, does `TymuxManager`'s existing `BackendRestarted()` contract already give sessions a clean way to detect "my daemon came back but I need to re-attach" so a supervisor-triggered restart behaves the same as today's manual-restart case? This needs verification against `session/tymux/`'s reconnect-loop tests, not just assumed compatible.

2. **Global flag flips while sessions are running on the old backend** — `main.go:167-174` reads `cfg.ProcessManagerBackend` exactly once at startup into a package-level var (`session/backend_factory.go:22-24`, guarded by `selectedBackendMu`). `RegisterBackendProvider` can technically be called again at runtime (it's exported, mutex-protected), but nothing in production calls it more than once — a config file edit today has zero effect until process restart. Streamhub's own design deliberately keeps the global flag "env-var-gated and requires a process restart by design" (stream_hub_rollout_service.go:21-23) for exactly this reason: a live flip mid-session risks splitting a session's terminal I/O across two owners. The plan should explicitly decide whether the tymux global flag is likewise restart-only (simplest, matches precedent) or hot-reloadable — and if hot-reloadable, what happens to a `*Instance` whose `processManager` field (`session/instance.go`) was already constructed against the old backend when `NewProcessManager` was called at `NewInstance` time. Nothing today re-constructs `instance.processManager` after creation except `instance_tmux.go:134`'s narrower re-selection path (triggered by unrecognized-backend fallback, not by a live flag change) — a hot flip would need a new explicit migration/rebind path, which is out of scope per requirements.md unless research finds it's required.

3. **Per-session override requests tymux but the daemon fails to start** — `NewProcessManager`'s `BackendTymux` case (`session/backend_factory.go:74,89-91`) unconditionally constructs a `TymuxBackend` wrapping `tymux.NewTymuxGRPCSession(tymux.NewRealTransport(""))` — this call **cannot fail synchronously** (it's just building a gRPC client struct, not dialing). The actual failure surfaces later, at `Start(dir string)`/`RestoreWithWorkDir` time, when the manager first tries to call `CreateSession` over the wire. Today (per Story 2.2.6) there's no daemon-liveness check before that RPC is attempted at all. A newly-supervised design needs to decide: does session creation block on "daemon confirmed healthy" before committing to `BackendTymux` for that session (fail fast, maybe fall back to `BackendTmux` with a loud log — mirroring the `ErrUnrecognizedBackend`-fallback pattern already used at `instance_tmux.go:136` for a different failure class), or does it let `Start()` fail normally and surface a session-creation error to the caller? The existing `ErrUnrecognizedBackend` precedent (backend_factory.go:12-20) explicitly rejects "silently fall back to tmux and mask the problem" for a bad *config value* — an analogous principle probably applies to "explicitly requested tymux but the daemon won't come up," i.e. don't silently degrade to tmux without telling the operator.

4. **Shutdown while sessions are still attached to tymuxd** — tmux's own shutdown precedent (main.go:365-392) deliberately does **not** kill the tmux server by default: `--tmux-keep-server` defaults to `true` (main.go:784), `tmux.SetExitEmpty("", false)` keeps the server alive even with no sessions attached, and a keepalive session is explicitly created so an accidental server-empty exit never happens. If bundled tymuxd supervision mirrors this, the natural default is "don't stop tymuxd on graceful shutdown either," with an explicit opt-in flag (`--tymuxd-keep-server` or similar) mirroring `--tmux-keep-server`'s shape — the same reasoning (killing the daemon out from under attached clients on every restart would defeat the purpose of `--tmux-keep-server`-style resilience, and would also contradict `BackendRestarted()`'s existing "detect daemon restarted out from under me" contract, which exists specifically because a daemon restart is a real, handled event, not something to avoid at all costs). Needs an explicit decision either way, not silent inheritance of tmux's default.

5. **Backward compatibility for existing `sessions.json` entries with no recorded backend** — already confirmed as a live gap: `session/instance_serialization.go:331-334`'s doc comment states `instance.Backend` is **not currently persisted** in `InstanceData` at all ("out of Epic 2.1's scope" per the comment). So every session restored from disk today has `instance.Backend == ""` regardless of what backend it was actually created with, and falls through to whatever the process-wide global resolves to at restore time. If bundling work adds *persistence* of `Backend` to `InstanceData` (likely necessary to make a per-session tymux override survive a restart), the migration story is: old entries with no `backend` field in JSON unmarshal to the Go zero value `""` automatically (matches today's implicit behavior) — no explicit migration code needed for the empty-means-default case, but the plan should state this explicitly rather than leave it implicit, since it's exactly the kind of "silently reinterpreted" gap `.claude/rules/fix-flaky-tests-dont-defer.md`'s sibling rules warn about for LLM-authored migrations.

6. **No health-check RPC exists on tymuxd today** — confirmed by reading the proto directly (`/home/tstapler/Programming/tymux/proto/tymux/v1/tymux.proto:8-99`, `service TymuxService`): `CreateSession`, `ListSessions`, `KillSession`, `ReviveSession`, `CapturePane`, `SearchScrollback`, `SplitPane`, `ClosePane`, `CreateWindow`, `WatchWindow` (stream), `Attach` (bidi stream) — no `Health`/`Ping`/`GetStatus` RPC. Requirements.md's Open Question ("what health-check contract does tymuxd expose today") has a concrete answer: **none**. Supervision therefore has two options within this project's stated scope (no changes to tymuxd's own Rust-side behavior): (a) use `ListSessions` (a cheap, side-effect-free RPC) as a de facto liveness probe — the same "borrow an existing read RPC as a health check" pattern many gRPC services use before a dedicated health endpoint exists — or (b) treat process-liveness (is the PID from the supervisor's own launch still alive, mirroring `daemon/daemon.go`'s PID-file pattern below) as the only signal, with RPC-level failures handled reactively by the existing `ReconnectState()`/`BackendRestarted()` machinery rather than proactively polled. A dedicated `Health` RPC on the Rust side would be the cleanest long-term fix but is explicitly out of scope for this project ("Any change to tymuxd's own (Rust-side) behavior" — Out of Scope, requirements.md:97) — worth flagging as a natural follow-up for the sibling `tstapler/tymux` repo rather than something this plan can pull in.

## (e) Existing subprocess-supervision precedent besides tmux

Two clear precedents beyond `session/tmux/tmux.go`:

1. **`daemon/daemon.go`'s `LaunchDaemon`/`StopDaemon`** (daemon.go:335-420) — a full detached-subprocess lifecycle pattern already in this codebase, for the (unrelated) AutoYes background daemon:
   - `LaunchDaemon()`: resolves its own executable path (`os.Executable()`), starts a detached child (`safeexec.CommandContext` + `cmd.Stdin/Stdout/Stderr = nil` + platform `SysProcAttr` to prevent signal propagation via `getSysProcAttr()`), writes the child's PID to a `daemon.pid` file under the config dir, then calls `cmd.Process.Release()` so the OS doesn't require the parent to reap it (avoids zombies).
   - `StopDaemon()`: reads the PID file, `os.FindProcess(pid)` + `proc.Kill()`, removes the PID file. Returns cleanly (no error) if the PID file doesn't exist — idempotent stop.
   - This is directly reusable shape for tymuxd supervision: launch tymuxd the same way (resolve the bundled/embedded binary path instead of `os.Executable()`), track its PID the same way, and reuse the "no-op if not running" idempotent-stop pattern for shutdown. It does not include a health-check/restart-on-crash loop — that part would still need to come from tmux's `EnsureServerRunning`/retry-with-backoff precedent (see below) or be net-new.

2. **`session/tmux/tmux.go`'s `EnsureServerRunning`** (tmux.go:666-708) — the "start-if-not-running + retry with exponential backoff" half of the pattern the daemon precedent above doesn't cover:
   - `checkServerNotRunning(serverSocket)` — a cheap liveness probe (list-sessions against the socket) run *before* attempting to start, so an already-running server is a no-op.
   - `serverStartAttempt` / `ensureServerRunningWithRetry` (tmux.go:624-664) — retries the start-server invocation itself (not just a recheck) up to 8 times with exponential backoff (100ms → 3s cap, ~9.1s worst case), because under sustained system load a single start attempt and a single recheck both proved insufficient in practice (doc comment at tmux.go:600-617 documents the actual production incident that motivated widening these bounds from an initial narrower version).
   - `startServer`/`isNotRunning` are injected as function parameters specifically so tests can simulate the retry logic deterministically without real subprocess timing — a pattern worth reusing for tymuxd's equivalent (`ensureTymuxdRunning`) so its retry/backoff logic is unit-testable the same way.
   - Post-start, tmux also sets a server-wide default option (`remain-on-exit on`) — the equivalent tymuxd setup step, if any exists, is a Rust-side daemon-config concern rather than something Go-side supervision would set via RPC (needs checking against tymuxd's actual startup config surface, out of this research task's grep-only scope).
   - `KillOrphanedControlModeClients` (tmux.go:710+) and `main.go:365-392`'s startup sequence show the **restart-recovery** half: on process (re)start, clean up leftover control-mode clients from a *previous* process instance before restoring any session, and default to keeping the tmux server alive across a stapler-squad restart (`--tmux-keep-server` default `true`) rather than tearing it down — see Edge Case 4 above for why this matters for tymuxd's shutdown behavior specifically.

No other supervised-subprocess pattern was found in the repo (`instrumentation/otelc/` wires an *external*, separately-managed collector process via build-tag auto-instrumentation rather than spawning/supervising its own subprocess; `executor/managed_process.go` and `executor/shortlived.go` are the generic exec-a-command-and-capture-output primitives sessions use to run user programs like Claude Code itself, not a supervision-with-health-check pattern — worth a second look if the plan phase wants confirmation these aren't a closer fit, but based on file names and the daemon/tmux precedents already covering both halves of the "launch + supervise" problem, they appear to be a different concern (running a foreground child under a PTY) rather than a backgrounded, health-checked service).
