# safeexec-framework: Architecture Research

## Existing Executor Package Map

```
executor/                          (package executor)
├── executor.go                    Executor interface + Exec + MakeExecutor + MakeTimeoutExecutor + ToString
├── timeout_executor.go            TimeoutExecutor — wraps Executor, creates its own context+WaitDelay per call
├── circuit_breaker.go             CircuitBreakerExecutor — wraps Executor with per-command-class breakers
├── registry.go                    CircuitBreakerRegistry — global singleton for observability
└── safeexec/                      (package safeexec)
    └── safeexec.go                CommandContext — thin exec.CommandContext + WaitDelay pre-setter
```

### Core Interface

```go
// executor package
type Executor interface {
    Run(cmd *exec.Cmd) error
    Output(cmd *exec.Cmd) ([]byte, error)
    CombinedOutput(cmd *exec.Cmd) ([]byte, error)
}
```

All decorator types (`TimeoutExecutor`, `CircuitBreakerExecutor`) implement this interface and accept an `Executor`
delegate. Consumers depend on the interface, not concrete types.

### Key Observations

- `TimeoutExecutor` does NOT use `safeexec.CommandContext` internally — it rebuilds the cmd from `cmd.Args`
  and sets `WaitDelay = 2*time.Second` directly. This is a known inconsistency: the `norawexec` lint exempts
  the `executor` package itself (`/executor` suffix), so it compiles cleanly.
- `safeexec.CommandContext` is consumed directly by `session/tmux/` (non-timeout code paths) and by
  `session/tmux/server_registry.go` for keepalive commands.
- Long-running `cmd.Start()` processes (control-mode tmux, server registry attach) carry
  `//nolint:norawexec` comments with justifications. There are exactly **5 production nolint sites**:
  - `session/mux/multiplexer.go`
  - `session/external_tmux_streamer.go`
  - `session/tmux/server_registry.go`
  - `session/tmux/testmain_test.go` (test infra)
  - `daemon/daemon.go` (daemon launch — daemon package, not executor)
- The `norawexec` analyzer exempts `strings.HasSuffix(pkgPath, "/executor")` and
  `strings.HasSuffix(pkgPath, "/executor/safeexec")`. Any new sub-package under `executor/` that calls
  `exec.CommandContext` directly must be added to this exempt list.

---

## Question 1: How Should ShortLivedCmd and ManagedProcess Relate to the Existing Executor Interface?

**Answer: Complementary types, not replacements. Do not change the Executor interface.**

The `Executor` interface is a thin adapter seam for `*exec.Cmd` execution; it is used by `CircuitBreakerExecutor`,
`TimeoutExecutor`, and the tmux session management layer. It solves command dispatch, not process lifecycle.

`ShortLivedCmd` and `ManagedProcess` operate at a higher level of abstraction: they own the full lifecycle of
a subprocess, including construction, resource limits, audit emission, and cleanup. They should NOT implement
`Executor` and should not be forced into the `Run/Output/CombinedOutput` mold.

Proposed relationship:

- `ShortLivedCmd` is a builder/runner for one-shot commands. It owns `*exec.Cmd` internally and returns
  output directly. It does NOT implement `Executor`. Callers who need circuit breaking or timeout wrapping
  compose it with the existing decorators at the boundary — or use `TimeoutExecutor` for legacy code.
- `ManagedProcess` is a lifecycle handle for long-running `cmd.Start()` processes. It is not an `Executor`.
  It is the type that replaces `//nolint:norawexec` manual management.
- The `Executor` interface stays unchanged. `TimeoutExecutor` and `CircuitBreakerExecutor` stay as-is.
  They continue to be the right tool when callers already hold an `*exec.Cmd` and want pluggable dispatch.

This avoids two anti-patterns: (1) over-extending `Executor` with lifecycle methods that don't fit
`Run/Output/CombinedOutput`, and (2) requiring all new code to be funneled through the `*exec.Cmd`
pass-by-value style of the existing interface.

---

## Question 2: Should ShortLivedCmd Be a New Package Under executor/ or in the Root executor Package?

**Answer: Add to the root `executor` package. Keep `safeexec/` as the low-level primitive only.**

Rationale:

1. **Cohesion**: `executor.go`, `timeout_executor.go`, and `circuit_breaker.go` already share the `executor`
   package. `ShortLivedCmd` and `ManagedProcess` are logical additions to the same abstraction tier.
2. **Avoid import cycles**: `ShortLivedCmd` will need to call `safeexec.CommandContext` internally.
   `safeexec` is a sub-package of `executor`. If `ShortLivedCmd` lived in its own sub-package (e.g.
   `executor/shortlived`), it would import `executor/safeexec` — fine — but would also likely import
   `executor` itself for the `Executor` interface, creating a cycle. Staying in the `executor` package
   avoids this.
3. **`norawexec` exemption**: The lint rule already exempts `"/executor"` suffix. No change needed.
4. **Sub-package is justified only for isolation**: `safeexec/` exists because it is the lowest-level
   primitive and has no dependencies on the rest of `executor`. There is no equivalent isolation benefit
   for `ShortLivedCmd`.

File placement within `executor/`:

```
executor/
├── executor.go              (existing — Executor interface, Exec, MakeExecutor, ToString)
├── timeout_executor.go      (existing)
├── circuit_breaker.go       (existing)
├── registry.go              (existing)
├── shortlived.go            NEW — ShortLivedCmd builder + runner
├── shortlived_unix.go       NEW — process group Setpgid=true (build: !windows)
├── shortlived_windows.go    NEW — no-op Setpgid stub (build: windows)
├── managed_process.go       NEW — ManagedProcess type + StartProcess
├── managed_process_unix.go  NEW — SIGTERM/SIGKILL pgid kill (build: !windows)
├── managed_process_windows.go NEW — graceful-only kill (build: windows)
├── rlimit.go                NEW — RlimitConfig type (always compiles; values only)
├── rlimit_linux.go          NEW — applyRlimits via syscall.Setrlimit (build: linux)
├── rlimit_other.go          NEW — applyRlimits no-op (build: !linux)
├── audit.go                 NEW — AuditHook interface + structured log emitter
└── safeexec/
    └── safeexec.go          (existing — unchanged)
```

---

## Question 3: How Should the Audit Logging Hook Integrate Without Breaking the Existing Interface?

**Answer: Use a context-carried hook, not an interface method addition.**

Two viable patterns were considered:

**Option A — Middleware/decorator (rejected)**: Add an `AuditingExecutor` that wraps `Executor` and emits
logs before/after `Run/Output/CombinedOutput`. This is consistent with the existing decorator chain but
only covers the `Executor` interface path. `ShortLivedCmd` and `ManagedProcess` would each need to
separately invoke audit logic, causing duplication.

**Option B — Context-carried hook (preferred)**: Define an `AuditHook` interface and store it in the
context via a private key. `ShortLivedCmd` and `ManagedProcess` both call a `emitAudit(ctx, entry)` helper
that extracts the hook from context (if present) and calls it. The hook is opt-in: no hook in context
means no audit overhead.

```go
// audit.go
type AuditEntry struct {
    Command     []string
    WorkDir     string
    StartTime   time.Time
    Duration    time.Duration
    ExitCode    int
    KilledByCtx bool
}

type AuditHook interface {
    OnExec(entry AuditEntry)
}

// WithAuditHook stores hook in ctx. Pass the returned context to ShortLivedCmd/ManagedProcess.
func WithAuditHook(ctx context.Context, hook AuditHook) context.Context

// LoggingAuditHook is the default hook that writes to log.InfoLog / log.DebugLog.
type LoggingAuditHook struct{}
```

This keeps the `Executor` interface unchanged, keeps audit logic in one place (`audit.go`), and
makes opt-in per-callsite via context — consistent with how the codebase already uses context for
cancellation and deadline propagation. The `AuditingExecutor` decorator can still be provided as a
convenience wrapper over the `Executor` interface for legacy callsites, but it becomes thin: it just
extracts the hook from context and calls `emitAudit`.

---

## Question 4: Adding Process Group Management to safeexec.CommandContext Without Breaking the Call Signature

**Answer: Do not change `safeexec.CommandContext`. Add `safeexec.CommandContextPG` as a parallel function.**

`safeexec.CommandContext(ctx, name, args...)` has the identical signature as `exec.CommandContext`.
This is intentional — the entire point of `safeexec` is to be a transparent drop-in. Changing the
returned type or adding behavior that the caller might not expect violates that contract.

Process group setup requires setting `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` before
`cmd.Start()`. This is inherently platform-specific (no-op on Windows). It also increases kill latency
slightly and can interact with terminal ownership assumptions in control-mode tmux processes — which is
exactly why some callers need PG and some don't.

Proposed approach:

```go
// safeexec/safeexec.go (existing, unchanged)
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd

// safeexec/safeexec_pg.go (new — build: !windows)
// CommandContextPG returns a Cmd with WaitDelay AND Setpgid=true.
// Use for short-lived commands where kill signals must reach grandchildren.
// Do NOT use for control-mode tmux processes that expect to share the terminal session.
func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd

// safeexec/safeexec_pg_windows.go (new — build: windows)
// CommandContextPG falls back to CommandContext on Windows (no process group concept).
func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd
```

`ShortLivedCmd` calls `safeexec.CommandContextPG` internally by default unless the caller opts out
via `.WithoutProcessGroup()`. `ManagedProcess` similarly calls `CommandContextPG` and additionally
tracks the pgid for its `Kill()` implementation.

Adding the `norawexec` exemption: `executor/safeexec` is already exempt. `CommandContextPG` lives
inside that package, so no change to the analyzer's exempt list is needed.

---

## Question 5: Build Tags for Platform-Specific rlimit Code

**Answer: Use the `linux` / `!linux` pattern established by `subreaper_linux.go` / `subreaper_other.go`.**

The codebase already has two well-established patterns for platform-specific code:

1. `!windows` / `windows` — for POSIX vs Windows differences (daemon, mux signals, exec_unix/exec_windows)
2. `linux` / `!linux` — for Linux-specific syscall features (subreaper) where macOS and Windows share a no-op

For rlimits, the correct split is `linux` / `!linux`:

- Linux: full `syscall.Setrlimit` support including `RLIMIT_AS` (virtual memory), `RLIMIT_CPU`, `RLIMIT_NOFILE`
- macOS: `RLIMIT_CPU` and `RLIMIT_NOFILE` are available via `syscall.Setrlimit`, but `RLIMIT_AS` behaves
  differently and `RLIMIT_RSS` is not enforced. The safe approach is to treat macOS as "partial support"
  and only apply CPU + nofile limits, or treat it as a no-op to avoid platform-specific correctness bugs.
- Windows: no rlimit concept; always no-op.

Given the requirements (production runs on Linux; macOS is development), using `linux` / `!linux` gives
the clearest semantics:

```
executor/rlimit_linux.go    //go:build linux        — full syscall.Setrlimit implementation
executor/rlimit_other.go    //go:build !linux       — no-op applyRlimits; RlimitConfig type still defined
executor/rlimit.go          (no build tag)          — RlimitConfig struct definition only (always compiles)
```

The `RlimitConfig` type lives in `rlimit.go` (no build tag) so callers can always construct configs
portably. Only `applyRlimits(cmd *exec.Cmd, cfg RlimitConfig) error` is split by build tag. This is
the same pattern used for `SetSubreaper()` in `session/tmux/`.

For `Setpgid` / process group management, use `!windows` / `windows` since macOS fully supports
`Setpgid` and `killpg` via standard POSIX syscalls. The split is cleaner:

```
executor/safeexec/safeexec_pg.go         //go:build !windows   — Setpgid: true
executor/safeexec/safeexec_pg_windows.go //go:build windows    — no-op fallback
```

---

## Question 6: Concrete Package Layout

### Full File Inventory

```
executor/
│
│  ── EXISTING (unchanged) ──────────────────────────────────────────────────
├── executor.go              Executor interface, Exec, MakeExecutor, MakeTimeoutExecutor, ToString
├── timeout_executor.go      TimeoutExecutor
├── circuit_breaker.go       CircuitBreakerExecutor, CircuitBreakerConfig, Clock, circuitBreaker
├── registry.go              CircuitBreakerRegistry, globalRegistry
│
│  ── NEW ────────────────────────────────────────────────────────────────────
├── audit.go                 AuditHook interface, AuditEntry, WithAuditHook, LoggingAuditHook, emitAudit
│
├── shortlived.go            ShortLivedCmd builder type, Run/Output/CombinedOutput methods
│                              Uses safeexec.CommandContextPG internally.
│                              Calls emitAudit on completion.
│
├── managed_process.go       ManagedProcess struct, StartProcess func, Stop/Wait/PID/IsAlive methods
│                              Calls emitAudit on exit.
├── managed_process_unix.go  //go:build !windows
│                              killProcessGroup — sends SIGTERM then SIGKILL to entire pgid
├── managed_process_windows.go //go:build windows
│                              killProcessGroup — cmd.Process.Kill() only (no process groups)
│
├── rlimit.go                RlimitConfig struct (MaxCPUSecs, MaxMemBytes, MaxOpenFiles uint64)
│                              Always compiles; values are passed to applyRlimits.
├── rlimit_linux.go          //go:build linux
│                              applyRlimits — sets RLIMIT_CPU, RLIMIT_AS, RLIMIT_NOFILE via syscall.Setrlimit
│                              Uses SysProcAttr.Pdeathsig = syscall.SIGKILL as defense-in-depth
├── rlimit_other.go          //go:build !linux
│                              applyRlimits — no-op; returns nil
│
└── safeexec/
    ├── safeexec.go          (existing, unchanged) CommandContext + DefaultWaitDelay
    ├── safeexec_pg.go       //go:build !windows   CommandContextPG — CommandContext + Setpgid: true
    └── safeexec_pg_windows.go //go:build windows  CommandContextPG — delegates to CommandContext
```

### Type Responsibilities

| Type | Package | Responsibility |
|------|---------|----------------|
| `Executor` interface | `executor` | Thin adapter for pluggable `*exec.Cmd` dispatch; unchanged |
| `TimeoutExecutor` | `executor` | Context-per-call timeout; unchanged |
| `CircuitBreakerExecutor` | `executor` | Per-command-class failure tracking; unchanged |
| `ShortLivedCmd` | `executor` | Builder for one-shot commands; owns WaitDelay + PG + rlimits + audit |
| `ManagedProcess` | `executor` | Lifecycle handle for `cmd.Start()` processes; owns PG kill + audit |
| `AuditHook` | `executor` | Interface for audit consumers; `LoggingAuditHook` is the default impl |
| `RlimitConfig` | `executor` | Value type for per-command resource constraints |
| `safeexec.CommandContext` | `executor/safeexec` | Lowest-level primitive; WaitDelay only; unchanged |
| `safeexec.CommandContextPG` | `executor/safeexec` | CommandContext + Setpgid; used by ShortLivedCmd |

### norawexec Lint Impact

- `executor` and `executor/safeexec` are already exempt. No changes to `analyzer.go` needed.
- The 5 production `//nolint:norawexec` sites in `session/` become candidates for migration to
  `ManagedProcess` (3 sites: multiplexer, external_tmux_streamer, server_registry). The `daemon/daemon.go`
  site is exempt from migration (daemon package, intentionally separate lifecycle).
- A new `nounmanagedprocess` analyzer (FR-8) would flag `cmd.Start()` calls outside `executor/` that
  lack a `//nolint:nounmanagedprocess` comment. The 5 existing nolint sites would need their comment
  updated or replaced by actual `ManagedProcess` usage.

### Integration Points for ShortLivedCmd vs Existing Executor Interface

Callers that currently use `executor.Executor` (e.g., `session/tmux/tmux.go` stores `cmdExec executor.Executor`)
should NOT need to change. The `Executor` interface remains the correct abstraction for:

- `tmux` subcommand calls (new-session, kill-session, capture-pane, etc.) — these are short-lived and
  already managed via `TimeoutExecutor` + `CircuitBreakerExecutor`
- Any code path that needs circuit breaking or swappable mock executors for testing

`ShortLivedCmd` is the correct choice for:

- New callsites that need rlimits, audit, or explicit PG management
- Refactoring away from `//nolint:norawexec` sites where the process is short-lived

`ManagedProcess` replaces:

- Any `cmd.Start()` call followed by manual `cmd.Wait()` / `cmd.Process.Kill()` + `//nolint:norawexec`

---

## Summary of Key Architectural Decisions

1. `ShortLivedCmd` and `ManagedProcess` live in the root `executor` package (not a sub-package) to avoid
   import cycles with `executor/safeexec` and to share the `norawexec` lint exemption already in place.
   They do NOT implement the existing `Executor` interface — they operate at a higher lifecycle abstraction
   that does not fit the `Run/Output/CombinedOutput` seam.

2. Audit logging integrates via a context-carried `AuditHook` interface rather than a decorator wrapping
   the `Executor` interface. This keeps `Executor` unchanged, puts audit logic in one place, and makes it
   genuinely opt-in per-callsite via `executor.WithAuditHook(ctx, hook)`.

3. Process group management (`Setpgid: true`) is added as `safeexec.CommandContextPG` — a new function
   in the existing `safeexec` package with a `!windows` / `windows` build tag split — rather than
   modifying `safeexec.CommandContext`'s existing behavior, which is relied upon by control-mode tmux
   processes that must NOT use process groups.

4. Platform-specific rlimit code follows the `linux` / `!linux` build tag pattern established by
   `session/tmux/subreaper_linux.go`. The `RlimitConfig` struct has no build tag so callers can build
   portably; only `applyRlimits` is split. Process group code follows the `!windows` / `windows` split
   since macOS fully supports POSIX process groups.
