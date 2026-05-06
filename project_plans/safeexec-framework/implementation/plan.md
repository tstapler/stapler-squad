# safeexec-framework: Implementation Plan

Plan date: 2026-05-05
Status: Ready for implementation

---

## 1. Package Layout

All new types live in the root `executor` package. No new sub-packages are introduced. The `executor/safeexec` sub-package receives two new files only.

```
executor/
│
│  ── EXISTING (unchanged) ─────────────────────────────────────────────────
├── executor.go                 Executor interface, Exec, MakeExecutor, ToString
├── timeout_executor.go         TimeoutExecutor
├── circuit_breaker.go          CircuitBreakerExecutor and all breaker types
├── registry.go                 CircuitBreakerRegistry
│
│  ── NEW ────────────────────────────────────────────────────────────────────
├── audit.go                    AuditEntry, AuditHook, ctxKey, WithAuditHook,
│                               emitAudit (unexported), LoggingAuditHook
│
├── shortlived.go               config struct, Option type, all WithXxx option
│                               constructors, ShortLivedCmd struct,
│                               New(), Run(), Output(), CombinedOutput(),
│                               ScanOutput() methods
├── shortlived_unix.go          //go:build !windows
│                               applyProcessGroup(cmd) — sets SysProcAttr.Setpgid
├── shortlived_windows.go       //go:build windows
│                               applyProcessGroup(cmd) — no-op stub
│
├── managed_process.go          ManagedProcess struct, StartProcess(),
│                               Stop(), Wait(), PID(), IsAlive(),
│                               Stdout(), Stderr(), ScanLines() methods,
│                               internal reaper goroutine
├── managed_process_unix.go     //go:build !windows
│                               killProcessGroup(pid int, sig syscall.Signal)
│                               stopWithSIGTERM(), stopWithSIGKILL()
├── managed_process_windows.go  //go:build windows
│                               killProcessGroup — cmd.Process.Kill() only
│
├── rlimit.go                   RlimitConfig struct (no build tag — always compiles)
├── rlimit_linux.go             //go:build linux
│                               applyRlimits(cmd, cfg) — syscall.Setrlimit
│                               on RLIMIT_CPU, RLIMIT_AS, RLIMIT_NOFILE
├── rlimit_other.go             //go:build !linux
│                               applyRlimits(cmd, cfg) — no-op, returns nil
│
└── safeexec/
    ├── safeexec.go             (existing, unchanged)
    ├── safeexec_pg.go          //go:build !windows
    │                           CommandContextPG — CommandContext + Setpgid: true
    └── safeexec_pg_windows.go  //go:build windows
                                CommandContextPG — delegates to CommandContext
```

### File count: 8 new files + 2 new files in safeexec/ = 10 new files total

---

## 2. Type Definitions

### 2.1 `audit.go`

```go
// AuditEntry holds structured metadata for one subprocess invocation.
type AuditEntry struct {
    Command      []string      // argv[0..n]; secret positions replaced with "<redacted>"
    WorkDir      string        // cmd.Dir at invocation time; empty string = inherited cwd
    StartTime    time.Time
    Duration     time.Duration // elapsed from Start to Wait returning
    ExitCode     int           // -1 if killed; exit code otherwise
    PID          int
    KilledByCtx  bool          // true if context cancellation triggered the kill
    KilledByStop bool          // true if ManagedProcess.Stop() triggered the kill
}

// AuditHook is implemented by consumers that want to observe subprocess invocations.
// OnExec is called synchronously after cmd.Wait() returns; it must not block.
type AuditHook interface {
    OnExec(entry AuditEntry)
}

// LoggingAuditHook is the default hook; it emits via log/slog at Debug level
// (escalated to Info on non-zero exit or kill).
type LoggingAuditHook struct {
    Logger *slog.Logger // nil falls back to slog.Default()
}

// ctxKey is the unexported context key type.
type ctxKey struct{}

// WithAuditHook returns a context that carries hook. Pass this context to
// ShortLivedCmd or StartProcess to enable audit logging for that invocation.
func WithAuditHook(ctx context.Context, hook AuditHook) context.Context

// emitAudit extracts the hook from ctx (if any) and calls OnExec.
// Called internally by ShortLivedCmd and ManagedProcess after Wait returns.
func emitAudit(ctx context.Context, entry AuditEntry)
```

### 2.2 `shortlived.go`

```go
// config holds all optional parameters for a ShortLivedCmd invocation.
// The zero value is valid: no timeout, inherited env, no rlimits, no audit.
type config struct {
    timeout       time.Duration    // 0 = no override; context deadline governs
    dir           string
    extraEnv      []string         // appended to os.Environ(); format: "KEY=VALUE"
    replaceEnv    []string         // replaces os.Environ() entirely when non-nil
    stdin         io.Reader
    redactIndices []int            // argv positions to replace with "<redacted>" in audit log
    rlimits       RlimitConfig
    noProcGroup   bool             // true = skip Setpgid; use for terminal-owning processes
}

// Option is a functional option for ShortLivedCmd.
type Option func(*config)

// ShortLivedCmd is a configured, not-yet-started one-shot subprocess.
// Construct with New(). Do not reuse after calling Run/Output/CombinedOutput.
type ShortLivedCmd struct {
    ctx  context.Context
    name string
    args []string
    cfg  config
}
```

Exported option constructors:

```go
func WithTimeout(d time.Duration) Option
func WithDir(dir string) Option
func WithEnv(key, val string) Option              // appends one KEY=VALUE pair
func WithReplaceEnv(env []string) Option          // replaces entire environment
func WithStdin(r io.Reader) Option
func WithRedactArgs(indices ...int) Option        // for audit log scrubbing
func WithRlimits(cfg RlimitConfig) Option
func WithoutProcessGroup() Option                 // opt out of Setpgid
```

Exported API:

```go
// New constructs a ShortLivedCmd. ctx governs deadline and audit hook extraction.
func New(ctx context.Context, name string, args []string, opts ...Option) *ShortLivedCmd

// Run runs the command, discarding output. Returns error if the command fails.
func (c *ShortLivedCmd) Run() error

// Output runs the command and returns stdout. Stderr is discarded.
func (c *ShortLivedCmd) Output() ([]byte, error)

// CombinedOutput runs the command and returns stdout+stderr merged.
func (c *ShortLivedCmd) CombinedOutput() ([]byte, error)
```

Internal method (unexported, shared by Run/Output/CombinedOutput):

```go
func (c *ShortLivedCmd) build() *exec.Cmd     // applies all config to a new exec.Cmd
```

### 2.3 `managed_process.go`

```go
// ManagedProcess is a lifecycle handle for a long-running subprocess started with
// cmd.Start(). Construct via StartProcess; do not create directly.
type ManagedProcess struct {
    cmd         *exec.Cmd
    cancel      context.CancelFunc  // cancels the derived context passed to cmd
    stopCh      chan struct{}        // closed by Stop(); triggers graceful shutdown
    done        chan struct{}        // closed by reaper goroutine when cmd.Wait() returns
    waitErr     chan error           // buffered(1); written once by reaper goroutine
    stopped     atomic.Bool         // guards against concurrent Stop() calls
    gracePeriod time.Duration

    stdoutReader io.Reader           // nil if caller used WithConsumeStdout
    stderrReader io.Reader           // nil if caller used WithConsumeStderr

    // audit fields captured at start
    auditCtx context.Context
    name     string
    args     []string
    startAt  time.Time
}

// ProcessOption is a functional option for StartProcess.
type ProcessOption func(*processConfig)

type processConfig struct {
    dir          string
    extraEnv     []string
    replaceEnv   []string
    stdin        io.Reader
    stdout       io.Writer        // when set, used directly as cmd.Stdout; no reader exposed
    stderr       io.Writer        // when set, used directly as cmd.Stderr; no reader exposed
    redactArgs   []int
    rlimits      RlimitConfig
    gracePeriod  time.Duration    // default: 5 * time.Second
    noProcGroup  bool
    noctty       bool             // add Noctty: true to SysProcAttr (for background daemons)
    setsid       bool             // add Setsid: true (implies no controlling terminal)
}
```

Exported ProcessOption constructors:

```go
func WithProcessDir(dir string) ProcessOption
func WithProcessEnv(key, val string) ProcessOption
func WithProcessReplaceEnv(env []string) ProcessOption
func WithProcessStdin(r io.Reader) ProcessOption
func WithConsumeStdout(w io.Writer) ProcessOption  // drain to w; no Stdout() reader
func WithConsumeStderr(w io.Writer) ProcessOption  // drain to w; no Stderr() reader
func WithGracePeriod(d time.Duration) ProcessOption
func WithProcessRlimits(cfg RlimitConfig) ProcessOption
func WithoutProcessGroupMP() ProcessOption         // opt out of Setpgid
func WithNoControllingTerminal() ProcessOption     // sets Noctty: true
func WithNewSession() ProcessOption                // sets Setsid: true (strongest isolation)
func WithProcessRedactArgs(indices ...int) ProcessOption
```

Exported API:

```go
// StartProcess starts name with args, applies opts, sets up process group, pipes,
// and a reaper goroutine. Returns a handle immediately after cmd.Start() succeeds.
func StartProcess(ctx context.Context, name string, args []string, opts ...ProcessOption) (*ManagedProcess, error)

// Stop initiates graceful shutdown: SIGTERM to the process group, then SIGKILL after
// GracePeriod. Blocks until the process has exited. Idempotent.
func (p *ManagedProcess) Stop() error

// Wait blocks until the process exits and returns cmd.Wait()'s error.
// If Stop was called, returns ErrWaitDelay or the process exit error.
func (p *ManagedProcess) Wait() error

// PID returns the process PID. Valid after StartProcess returns.
func (p *ManagedProcess) PID() int

// IsAlive returns true if the process has not yet exited.
func (p *ManagedProcess) IsAlive() bool

// Stdout returns an io.Reader for the process's stdout.
// Returns nil if WithConsumeStdout was used.
// Reads return io.EOF when the process exits.
func (p *ManagedProcess) Stdout() io.Reader

// Stderr returns an io.Reader for the process's stderr.
// Returns nil if WithConsumeStderr was used.
func (p *ManagedProcess) Stderr() io.Reader

// ScanLines reads lines from Stdout() via bufio.Scanner until EOF or ctx is done.
// Calls fn for each line. Blocks until the process exits or ctx is cancelled.
func (p *ManagedProcess) ScanLines(ctx context.Context, fn func(line string)) error
```

### 2.4 `rlimit.go`

```go
// RlimitConfig specifies per-subprocess resource limits.
// Zero values mean "no limit". Limits are applied on Linux via syscall.Setrlimit;
// on other platforms this struct is accepted but has no effect.
type RlimitConfig struct {
    MaxCPUSecs   uint64  // RLIMIT_CPU: max CPU seconds; process receives SIGXCPU at soft limit
    MaxVirtBytes uint64  // RLIMIT_AS: virtual address space in bytes (Linux only enforced)
    MaxOpenFiles uint64  // RLIMIT_NOFILE: open file descriptor count
}
```

Internal API (build-tag-split):

```go
// applyRlimits sets rlimits on cmd before Start(). Linux only; no-op elsewhere.
func applyRlimits(cmd *exec.Cmd, cfg RlimitConfig) error
```

### 2.5 `safeexec/safeexec_pg.go`

```go
// CommandContextPG returns an exec.Cmd with WaitDelay pre-set AND Setpgid: true.
// Use for short-lived commands where SIGKILL must reach grandchildren.
// Do NOT use for processes that require a controlling terminal (control-mode tmux).
func CommandContextPG(ctx context.Context, name string, arg ...string) *exec.Cmd
```

---

## 3. Internal Implementation Notes

### 3.1 `ShortLivedCmd.build()` sequence

1. Derive a context: if `WithTimeout` was set and shorter than ctx deadline, create `context.WithTimeout(ctx, d)`; always `defer cancel()` inside `Run/Output/CombinedOutput`.
2. Call `safeexec.CommandContextPG(ctx, name, args...)` unless `noProcGroup` is true, in which case call `safeexec.CommandContext(ctx, name, args...)`.
3. Set `cmd.Dir`, `cmd.Stdin`, `cmd.Env`.
4. Call `applyRlimits(cmd, cfg.rlimits)` — no-op on non-Linux.
5. Return the configured `*exec.Cmd`.

After `Run()/Output()/CombinedOutput()` returns, call `emitAudit(ctx, entry)`.

### 3.2 `ManagedProcess` startup sequence

1. Build `processConfig` from opts.
2. Derive a context: `ctx, cancel := context.WithCancel(ctx)`. Store `cancel`.
3. Construct `exec.Cmd` via `exec.CommandContext(derivedCtx, name, args...)`.
4. Apply `SysProcAttr`: `Setpgid: true` (via `applyProcessGroup`); optionally `Noctty: true` or `Setsid: true`.
5. Override `cmd.Cancel = func() error { return killProcessGroup(cmd.Process.Pid, syscall.SIGTERM) }`.
6. Set `cmd.WaitDelay = gracePeriod + 1*time.Second` so the runtime sends SIGKILL and closes pipes if SIGTERM is ignored.
7. Wire I/O:
   - If `WithConsumeStdout(w)` was set: `cmd.Stdout = w`.
   - Otherwise: `stdoutR, stdoutW := os.Pipe(); cmd.Stdout = stdoutW`. Store `stdoutR` as `p.stdoutReader`.
   - Same pattern for stderr. If neither Consume nor expose: `cmd.Stderr = io.Discard` in the reaper goroutine fallback (handled below).
8. Call `cmd.Start()`. On error, call `cancel()` and return the error.
9. Close write ends of os.Pipe in the parent (the child inherits them; the parent needs only the read ends).
10. Store `cmd.Process.Pid` for `PID()` and for `killProcessGroup` calls after Start.
11. Set `p.startAt = time.Now()`.
12. Set finalizer via `runtime.SetFinalizer(p, managedProcessFinalizer)`.
13. Launch reaper goroutine (see 3.3).
14. Return `p, nil`.

### 3.3 Reaper goroutine

```
goroutine:
  err := p.cmd.Wait()               // blocks until process exits + pipes drain
  close(p.done)                     // signal to Stop()/Wait() that process is gone
  p.waitErr <- err                  // write once to buffered chan
  emitAudit(p.auditCtx, ...)        // emit structured log entry
```

The reaper goroutine is the ONLY caller of `cmd.Wait()`, guaranteed by construction. No `sync.Once` needed because `StartProcess` creates exactly one goroutine.

### 3.4 `ManagedProcess.Stop()` sequence

```
1. p.stopped.CompareAndSwap(false, true) — if already true, drain p.waitErr and return
2. p.cancel()                          — fires cmd.Cancel (SIGTERM to process group)
3. select:
     case <-p.done: return <-p.waitErr   // clean exit within WaitDelay
     case <-time.After(p.gracePeriod):
         killProcessGroup(p.cmd.Process.Pid, syscall.SIGKILL)  // belt + suspenders
         <-p.done
         return <-p.waitErr
4. runtime.KeepAlive(p)
```

Note: `cmd.Cancel` fires SIGTERM and `cmd.WaitDelay = gracePeriod + 1s` escalates to SIGKILL via the runtime. The explicit `killProcessGroup(SIGKILL)` in step 3 is a belt-and-suspenders guard for the group (WaitDelay only kills `cmd.Process`, not the whole group).

### 3.5 `io.Pipe` vs `os.Pipe` decision

The research identified that `io.Pipe()` deadlocks when no one reads and the buffer fills. Use `os.Pipe()` for `ManagedProcess` stdout/stderr:

- `os.Pipe()` returns `(*os.File, *os.File)` — the OS kernel buffers data, so the process does not block on write even if the Go reader is slow.
- The write end (`*os.File`) is passed to `cmd.Stdout`. After `cmd.Start()`, the parent closes the write end — the child inherits it, and `cmd.Wait()` closes the child's end on exit, delivering `io.EOF` to the reader.
- Unlike `cmd.StdoutPipe()`, the parent reader is not coupled to `cmd.Wait()` ordering.

When neither `WithConsumeStdout` nor the caller reading `Stdout()` is the pattern, drain stderr to `io.Discard` in a goroutine before calling `cmd.Start()`.

### 3.6 Finalizer (last-resort safety net only)

```go
func managedProcessFinalizer(p *ManagedProcess) {
    if p.stopped.Load() {
        return
    }
    if p.cmd.Process != nil {
        pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
        if err == nil {
            _ = syscall.Kill(-pgid, syscall.SIGKILL)
        } else {
            _ = p.cmd.Process.Kill()
        }
    }
    // Do not call cmd.Wait() — reaper goroutine owns that.
    // Do not block — finalizer goroutine is shared.
}
```

`runtime.KeepAlive(p)` at the end of both `Stop()` and `Wait()` prevents premature finalization while those methods are executing.

---

## 4. API Surface Summary

### Package `executor`

| Symbol | Kind | Notes |
|---|---|---|
| `AuditEntry` | struct | Carries one subprocess event; fields see §2.1 |
| `AuditHook` | interface | `OnExec(AuditEntry)` |
| `LoggingAuditHook` | struct | Default slog-based impl |
| `WithAuditHook(ctx, hook)` | func | Returns enriched context |
| `Option` | type | `func(*config)` |
| `New(ctx, name, args, opts...)` | func | Creates `*ShortLivedCmd` |
| `WithTimeout(d)` | func | Returns `Option` |
| `WithDir(dir)` | func | Returns `Option` |
| `WithEnv(k, v)` | func | Returns `Option` |
| `WithReplaceEnv(env)` | func | Returns `Option` |
| `WithStdin(r)` | func | Returns `Option` |
| `WithRedactArgs(indices...)` | func | Returns `Option` |
| `WithRlimits(cfg)` | func | Returns `Option` |
| `WithoutProcessGroup()` | func | Returns `Option` |
| `ShortLivedCmd.Run()` | method | `error` |
| `ShortLivedCmd.Output()` | method | `([]byte, error)` |
| `ShortLivedCmd.CombinedOutput()` | method | `([]byte, error)` |
| `RlimitConfig` | struct | `MaxCPUSecs`, `MaxVirtBytes`, `MaxOpenFiles uint64` |
| `ProcessOption` | type | `func(*processConfig)` |
| `StartProcess(ctx, name, args, opts...)` | func | `(*ManagedProcess, error)` |
| `WithProcessDir(dir)` | func | Returns `ProcessOption` |
| `WithProcessEnv(k, v)` | func | Returns `ProcessOption` |
| `WithProcessReplaceEnv(env)` | func | Returns `ProcessOption` |
| `WithProcessStdin(r)` | func | Returns `ProcessOption` |
| `WithConsumeStdout(w)` | func | Returns `ProcessOption` |
| `WithConsumeStderr(w)` | func | Returns `ProcessOption` |
| `WithGracePeriod(d)` | func | Returns `ProcessOption` |
| `WithProcessRlimits(cfg)` | func | Returns `ProcessOption` |
| `WithoutProcessGroupMP()` | func | Returns `ProcessOption` |
| `WithNoControllingTerminal()` | func | Returns `ProcessOption` |
| `WithNewSession()` | func | Returns `ProcessOption` |
| `WithProcessRedactArgs(indices...)` | func | Returns `ProcessOption` |
| `ManagedProcess.Stop()` | method | `error` |
| `ManagedProcess.Wait()` | method | `error` |
| `ManagedProcess.PID()` | method | `int` |
| `ManagedProcess.IsAlive()` | method | `bool` |
| `ManagedProcess.Stdout()` | method | `io.Reader` |
| `ManagedProcess.Stderr()` | method | `io.Reader` |
| `ManagedProcess.ScanLines(ctx, fn)` | method | `error` |

### Package `executor/safeexec`

| Symbol | Kind | Notes |
|---|---|---|
| `CommandContext(ctx, name, args...)` | func | Existing; unchanged |
| `CommandContextPG(ctx, name, args...)` | func | New; adds `Setpgid: true` |
| `DefaultWaitDelay` | const | Existing; unchanged |

### Unchanged / untouched

- `executor.Executor` interface
- `executor.TimeoutExecutor`
- `executor.CircuitBreakerExecutor`
- `executor.Exec`
- `executor.MakeExecutor`, `MakeTimeoutExecutor`, `ToString`

---

## 5. Migration Strategy: Existing `//nolint:norawexec` Sites

There are 5 production nolint sites. 3 are migration candidates; 2 are out of scope.

### Site 1: `session/external_tmux_streamer.go:190`

**Current pattern:** `exec.CommandContext(s.ctx, "tmux", "-C", "attach-session", "-t", s.tmuxSessionName, "-r")` + manual `StdoutPipe` + `StderrPipe` + `cmd.Start()`

**Migration target:** `executor.StartProcess`

**Options to set:**
- `WithConsumeStderr(io.Discard)` — stderr is currently drained diagnostically; migrate to `LoggingAuditHook` for that
- No `WithConsumeStdout` — caller needs to call `ScanLines` for control-mode events
- `WithNoControllingTerminal()` (sets `Noctty: true`) — this process must not receive SIGHUP from PTY close
- `WithGracePeriod(5 * time.Second)`

**Note:** Do NOT set `WithNewSession()` — tmux control-mode may require session membership. `Noctty: true` is sufficient to prevent SIGTTIN/SIGTTOU. Verify behavior in integration test before committing.

**Key behavior change:** `startControlMode` currently stores `cmd` directly on the struct. After migration it stores `*ManagedProcess`. Callers of `s.controlModeCmd.Process.Kill()` must be replaced with `s.controlModeProcess.Stop()`.

### Site 2: `session/tmux/server_registry.go:266`

**Current pattern:** `exec.CommandContext(r.ctx, "tmux", args...)` + `StdoutPipe` + `StdinPipe` + `cmd.Start()`

**Migration target:** `executor.StartProcess`

**Options to set:**
- `WithProcessStdin(r)` where `r` comes from a pipe the caller controls (stdin must stay open to keep tmux alive — this is a special requirement; see §6.3 below)
- `WithNoControllingTerminal()` — same rationale as Site 1
- `WithGracePeriod(5 * time.Second)`

**Special requirement:** The current code calls `StdinPipe()` because tmux sends `%exit` when stdin reads EOF. After migration, the caller must provide a `*io.PipeWriter` (or `*os.File` from `os.Pipe()`) as `WithProcessStdin`. The hold on stdin is a caller responsibility, not a `ManagedProcess` responsibility.

**After migration:** `server_registry.go` returns `(*ManagedProcess, bufio.Scanner, error)` instead of `(*exec.Cmd, *bufio.Scanner, io.WriteCloser, error)`. The `WriteCloser` for stdin becomes an `*os.File` or `io.PipeWriter` held separately by the caller.

### Site 3: `session/mux/multiplexer.go:276`

**Current pattern:** `exec.CommandContext(m.ctx, "tmux", "attach-session", "-t", m.tmuxSession)` passed to `pty.Start(m.cmd)`.

**Migration status:** NOT migrated to `ManagedProcess`.

**Rationale:** `pty.Start(cmd)` requires a raw `*exec.Cmd` before `cmd.Start()` has been called — it hooks into the cmd's Stdin/Stdout/Stderr and calls `Start()` itself. `ManagedProcess.StartProcess` calls `cmd.Start()` internally; there is no way to intercept it for PTY attachment. This site must keep `//nolint:norawexec` with an updated justification. Add `WaitDelay` manually (currently missing from the raw cmd) and add `SysProcAttr{Setpgid: true}` if compatible with pty behavior.

**Action:** Update the comment to `//nolint:norawexec pty.Start() requires raw *exec.Cmd before cmd.Start(); ManagedProcess cannot be used here`. Add `cmd.WaitDelay = 5 * time.Second` to fix the missing WaitDelay bug.

### Site 4: `session/tmux/testmain_test.go` (test infra)

**Migration status:** Out of scope for this project. Test infra exemptions are intentional.

### Site 5: `daemon/daemon.go`

**Migration status:** Out of scope. The daemon package uses a separate process-lifecycle model (re-exec of the same binary). Migrating it to `ManagedProcess` would require daemon-specific lifecycle semantics not covered by this framework.

---

## 6. Known Issues and Architectural Concerns

### 6.1 `Setpgid` + PTY controlling terminal (CRITICAL)

`Setpgid: true` without `Noctty: true` or `Setsid: true` causes the child to remain in the same session as the Go process, sharing the controlling terminal. In a tmux context, pane close delivers `SIGHUP` to all processes in that session. Background processes must use `WithNoControllingTerminal()` or `WithNewSession()`.

**Impact:** Sites 1 and 2 require `WithNoControllingTerminal()`. Failing to set it will cause the control-mode tmux process to receive `SIGHUP` when a tmux pane is closed, terminating it unexpectedly.

**Mitigation:** The `WithNoControllingTerminal()` option is set by default for `StartProcess`. Callers that legitimately need a controlling terminal (rare) must opt out via `WithProcessTerminal()` (not provided by default — failing safe is the goal).

**Revision to plan:** Invert the default: `processConfig.noctty = true` by default; expose `WithControllingTerminal()` as an opt-in for the rare case. This makes the safe path the default path.

### 6.2 `TOCTOU` race on process group membership

There is a narrow window between `cmd.Start()` returning and the child executing `setpgid`. Signals sent to `-pid` immediately after start may miss the child. The library must not send signals in the startup path. `Stop()` is always called after the process is running, so this window is irrelevant in practice for `ManagedProcess`. For `ShortLivedCmd`, context cancellation may fire immediately, but `cmd.Cancel` is invoked by Go's internal watchCtx goroutine which races correctly.

### 6.3 stdin keep-alive for tmux control-mode (Site 2)

The `server_registry.go` site requires stdin to remain open. `ManagedProcess` exposes `WithProcessStdin(r io.Reader)` which sets `cmd.Stdin`. The caller must provide an `io.Reader` that does not return `io.EOF` until the process should exit. Use `os.Pipe()`: provide the read end as stdin and keep the write end open in the calling goroutine. When the caller wants to terminate, close the write end (triggering EOF to tmux) before calling `Stop()`.

**This is a caller responsibility.** Document it clearly in the migration PR.

### 6.4 `WaitDelay` expiry returns `exec.ErrWaitDelay`

When `WaitDelay` fires before pipes drain, `cmd.Wait()` returns `exec.ErrWaitDelay` instead of the process exit error. `ManagedProcess.Wait()` and `Stop()` must filter this: `ErrWaitDelay` should be treated as a successful shutdown signal (the process was killed as intended), not a returned error. Use `errors.Is(err, exec.ErrWaitDelay)` in the reaper goroutine and convert to `nil` or a typed `ErrKilledByTimeout` sentinel.

### 6.5 `rlimit` on Linux: `RLIMIT_NOFILE` interaction with Go runtime

Go 1.22 raises `RLIMIT_NOFILE` at init for itself. Setting `RLIMIT_NOFILE` via `SysProcAttr.Rlimits` (the `golang.org/x/sys/unix` approach) applies only to the child, not the parent. Using `syscall.Setrlimit` in the parent before `cmd.Start()` (the naive approach) would affect the Go runtime's own open file count. The plan uses `golang.org/x/sys/unix.SysProcAttr.Rlimits` on Linux for child-only application. Confirm the `golang.org/x/sys/unix` struct field exists on the target Linux kernel headers before implementation.

### 6.6 Double-Option naming collision

`Option` (for `ShortLivedCmd`) and `ProcessOption` (for `ManagedProcess`) are separate types that cannot be mixed. The `WithXxx` vs `WithProcessXxx` naming convention distinguishes them but is verbose. If the API proves clunky in practice, consider a unified `Opt` type with a `forProcess bool` discriminator in a future iteration — but do not over-engineer in this pass.

### 6.7 Lint rule `nounmanagedprocess` (FR-8) — deferred

The `nounmanagedprocess` analyzer is specified in requirements but its implementation requires changes to `tools/lint/`. Given the 5 existing nolint sites are well-understood and the 3 migration targets will be eliminated by this project, the lint rule adds limited immediate value. Implement after the migration sites are resolved so the rule starts with zero violations. Track as a follow-on task.

---

## 7. Testing Strategy

### Unit tests (table-driven, in `executor/`)

**`shortlived_test.go`**
- `TestShortLivedCmd_Run_success` — command exits 0, no error
- `TestShortLivedCmd_Run_nonZeroExit` — command exits 1, returns `*exec.ExitError`
- `TestShortLivedCmd_Output_capturesStdout`
- `TestShortLivedCmd_CombinedOutput_mergesStreams`
- `TestShortLivedCmd_WithTimeout_cancelsBeforeCompletion` — timeout shorter than command; verifies context cancellation error
- `TestShortLivedCmd_WithDir_setsWorkingDirectory`
- `TestShortLivedCmd_WithEnv_appendsToEnvironment`
- `TestShortLivedCmd_WithReplaceEnv_replacesEnvironment`
- `TestShortLivedCmd_WithRedactArgs_scrubsAuditLog` — verifies audit hook receives redacted args
- `TestShortLivedCmd_ProcessGroup_grandchildKilled` — starts a process that forks a grandchild; context cancel; verifies grandchild is gone
- `TestShortLivedCmd_WithoutProcessGroup_noSetpgid` — verifies SysProcAttr.Setpgid is false

**`managed_process_test.go`**
- `TestStartProcess_success_PIDIsNonZero`
- `TestStartProcess_commandNotFound_returnsError`
- `TestManagedProcess_Wait_returnsExitError`
- `TestManagedProcess_Stop_sendsTermThenKill` — uses a process that traps SIGTERM and records it; verifies Stop returns after grace period
- `TestManagedProcess_Stop_idempotent` — two concurrent Stop() calls; verifies no panic and both return
- `TestManagedProcess_IsAlive_falseAfterStop`
- `TestManagedProcess_Stdout_readsOutput`
- `TestManagedProcess_Stderr_readsErrors`
- `TestManagedProcess_ScanLines_callsCallback`
- `TestManagedProcess_ScanLines_ctxCancellation_stops`
- `TestManagedProcess_ProcessGroup_grandchildKilledOnStop` — starts process that forks grandchild; Stop(); verifies grandchild is gone
- `TestManagedProcess_NoControllingTerminal_nocttySet` — inspects SysProcAttr after Start (use exec.Cmd reflection or test helper)
- `TestManagedProcess_StdinPipe_eofOnWrite_terminatesTmux` — validates the stdin-hold pattern

**`audit_test.go`**
- `TestWithAuditHook_receivesEntryOnCompletion` — verifies OnExec is called with correct Command, ExitCode, Duration
- `TestWithAuditHook_noHook_noError` — context without hook; no panic
- `TestLoggingAuditHook_emitsAtDebug` — uses slog test handler
- `TestLoggingAuditHook_escalatesToInfoOnNonZeroExit`

**`rlimit_test.go`** (linux build tag)
- `TestApplyRlimits_cpuLimit_SIGXCPUDelivered` — run a CPU-spinning subprocess with 1-second CPU limit; verify it exits with signal
- `TestApplyRlimits_nofileLimit_openFails` — run subprocess that tries to open many files; verify EMFILE

**`shortlived_unix_test.go`** / **`managed_process_unix_test.go`** (build: !windows)
- `TestKillProcessGroup_unix` — lower-level pgid kill test

### Integration tests

Integration tests live alongside existing tests. Use `t.Parallel()` and temp directories; no external service required except where tmux is needed.

**`executor_integration_test.go`** (build tag: `//go:build integration`)
- `TestShortLivedCmd_gitStatus_realGitRepo` — runs `git status` in a temp repo; verifies no WaitDelay zombie
- `TestManagedProcess_longRunning_stopCleans` — starts `sleep 30`; Stop(); verifies process table shows no zombie after 500ms
- `TestManagedProcess_grandchildOrphan_stopped` — starts a shell that forks `sleep 30`; Stop() on shell; verifies grandchild is killed

**Migration site integration tests** (added to their respective packages):
- `TestExternalTmuxStreamer_controlMode_usesStartProcess` — verifies the migrated code creates a ManagedProcess and Stop() cleans it up
- `TestServerRegistry_controlMode_usesStartProcess`

### Coverage target

All new files in `executor/` must reach 80% statement coverage. Run with:

```
go test -cover ./executor/... -run .
```

Use `go test -race ./executor/...` to run all tests under the race detector.

---

## 8. Implementation Epics, Stories, and Tasks

---

### EPIC 1: Foundation — `safeexec.CommandContextPG` and `audit.go`

*Deliverable: the two shared primitives that all other epics depend on.*

#### Story 1.1: `CommandContextPG` in `executor/safeexec/`

- [ ] **Task 1.1.1** Create `safeexec/safeexec_pg.go` (`//go:build !windows`). Implement `CommandContextPG`: call `safeexec.CommandContext`, then set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`. Add doc comment explaining the PTY-incompatibility caveat.
- [ ] **Task 1.1.2** Create `safeexec/safeexec_pg_windows.go` (`//go:build windows`). Implement `CommandContextPG` as a direct delegate to `CommandContext`. Add doc comment explaining the no-op.
- [ ] **Task 1.1.3** Write `safeexec/safeexec_pg_test.go`: table-driven tests for `CommandContextPG` verifying WaitDelay is set, and on !windows that `SysProcAttr.Setpgid == true`.
- [ ] **Task 1.1.4** Run `make lint` and `go vet ./executor/safeexec/...`. Resolve any issues.

#### Story 1.2: `audit.go`

- [ ] **Task 1.2.1** Create `executor/audit.go`. Define `AuditEntry` struct with all fields from §2.1. Define `AuditHook` interface with `OnExec(AuditEntry)`. Define unexported `ctxKey` type. Implement `WithAuditHook(ctx, hook) context.Context` and unexported `emitAudit(ctx, entry)` that extracts hook and calls `OnExec` — no-op if no hook.
- [ ] **Task 1.2.2** Implement `LoggingAuditHook.OnExec`: use `slog.InfoContext` for non-zero exit / kills; `slog.DebugContext` otherwise. Use `slog.Group("subprocess", ...)` with all `AuditEntry` fields. Nil logger falls back to `slog.Default()`.
- [ ] **Task 1.2.3** Write `executor/audit_test.go`: tests per §7 — verify hook is called, verify no-op without hook, verify log level escalation.
- [ ] **Task 1.2.4** Run `make lint` and `go vet ./executor/...`.

---

### EPIC 2: `ShortLivedCmd` builder

*Deliverable: a functional, tested `ShortLivedCmd` that replaces direct `safeexec.CommandContext` for callers wanting options.*

#### Story 2.1: Core builder and options

- [ ] **Task 2.1.1** Create `executor/shortlived.go`. Define `config` struct, `Option` type. Implement all `WithXxx` option constructors from §2.2. Implement `New(ctx, name, args, opts...) *ShortLivedCmd`.
- [ ] **Task 2.1.2** Implement `ShortLivedCmd.build()`: context derivation, cmd construction via `CommandContextPG` (or `CommandContext` for noProcGroup), apply dir/env/stdin/rlimits.
- [ ] **Task 2.1.3** Implement `Run()`, `Output()`, `CombinedOutput()`: call `build()`, delegate to `cmd.Run/Output/CombinedOutput`, call `emitAudit` with correct `AuditEntry` (capture `startAt`, `exitCode`, `killedByCtx` from `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)`).

#### Story 2.2: Platform shims

- [ ] **Task 2.2.1** Create `executor/shortlived_unix.go` (`//go:build !windows`). Implement `applyProcessGroup(cmd *exec.Cmd)` that sets `cmd.SysProcAttr.Setpgid = true` (merging with existing SysProcAttr if non-nil).
- [ ] **Task 2.2.2** Create `executor/shortlived_windows.go` (`//go:build windows`). Implement `applyProcessGroup(cmd *exec.Cmd)` as a no-op.

#### Story 2.3: Unit tests for `ShortLivedCmd`

- [ ] **Task 2.3.1** Write all tests from §7 for `shortlived_test.go`. Use a test binary (`testdata/helper/main.go`) that can be compiled once and reused across tests for predictable behavior (exits on signal, prints to stdout/stderr, etc.).
- [ ] **Task 2.3.2** Run `go test -race -cover ./executor/... -run TestShortLived`. Achieve 80% coverage.

---

### EPIC 3: `RlimitConfig`

*Deliverable: child-only resource limits on Linux; no-op on other platforms.*

#### Story 3.1: `rlimit.go` (platform-independent struct)

- [ ] **Task 3.1.1** Create `executor/rlimit.go`. Define `RlimitConfig` struct from §2.4. No build tag. Add doc comment explaining Linux-only enforcement.

#### Story 3.2: `rlimit_linux.go`

- [ ] **Task 3.2.1** Create `executor/rlimit_linux.go` (`//go:build linux`). Implement `applyRlimits(cmd *exec.Cmd, cfg RlimitConfig) error`. Use `golang.org/x/sys/unix.SysProcAttr.Rlimits` field (already in `go.mod`). Merge with existing `SysProcAttr` if non-nil. Set `RLIMIT_CPU` if `MaxCPUSecs > 0`, `RLIMIT_AS` if `MaxVirtBytes > 0`, `RLIMIT_NOFILE` if `MaxOpenFiles > 0`. Also set `Pdeathsig: syscall.SIGKILL` (Linux-only defense-in-depth: kills child if parent dies).
- [ ] **Task 3.2.2** Verify `golang.org/x/sys/unix.SysProcAttr.Rlimits` field exists for the version in `go.mod` (v0.42.0). If not present, fall back to `syscall.Setrlimit` in the parent with save/restore, documented as an acceptable race.

#### Story 3.3: `rlimit_other.go`

- [ ] **Task 3.3.1** Create `executor/rlimit_other.go` (`//go:build !linux`). Implement `applyRlimits(cmd *exec.Cmd, cfg RlimitConfig) error` as `return nil`.

#### Story 3.4: Tests

- [ ] **Task 3.4.1** Write `executor/rlimit_linux_test.go` (`//go:build linux`): test CPU limit delivers SIGXCPU; test NOFILE limit causes EMFILE. Use short-lived processes via `exec.Command("bash", "-c", ...)` for controlled behavior.

---

### EPIC 4: `ManagedProcess`

*Deliverable: lifecycle-managed long-running processes with SIGTERM/SIGKILL, streaming I/O, and audit.*

#### Story 4.1: Core struct and startup

- [ ] **Task 4.1.1** Create `executor/managed_process.go`. Define `ManagedProcess` struct from §2.3. Define `processConfig` struct. Implement all `WithProcessXxx` and related `ProcessOption` constructors.
- [ ] **Task 4.1.2** Implement `StartProcess(ctx, name, args, opts...) (*ManagedProcess, error)` following the startup sequence in §3.2: derive context, build cmd, apply SysProcAttr (with `noctty = true` default), override `cmd.Cancel`, set `cmd.WaitDelay`, wire I/O via `os.Pipe()`, call `cmd.Start()`, close parent write ends, set finalizer, launch reaper goroutine.
- [ ] **Task 4.1.3** Implement reaper goroutine from §3.3: calls `cmd.Wait()`, handles `exec.ErrWaitDelay` (convert to nil), closes `done`, writes to `waitErr`, calls `emitAudit`.

#### Story 4.2: Lifecycle methods

- [ ] **Task 4.2.1** Implement `Stop()` following the sequence in §3.4. Use `atomic.Bool` for `stopped`. Handle the `ErrWaitDelay` case.
- [ ] **Task 4.2.2** Implement `Wait()`: drains `p.done`, reads from `p.waitErr`, returns error. Annotate with `runtime.KeepAlive(p)`.
- [ ] **Task 4.2.3** Implement `PID() int`, `IsAlive() bool` (select on `p.done` with default case).
- [ ] **Task 4.2.4** Implement `Stdout() io.Reader`, `Stderr() io.Reader`, `ScanLines(ctx, fn)`.

#### Story 4.3: Platform shims

- [ ] **Task 4.3.1** Create `executor/managed_process_unix.go` (`//go:build !windows`). Implement `killProcessGroup(pid int, sig syscall.Signal) error` using `syscall.Getpgid(pid)` then `syscall.Kill(-pgid, sig)` with fallback to `syscall.Kill(pid, sig)`.
- [ ] **Task 4.3.2** Create `executor/managed_process_windows.go` (`//go:build windows`). Implement `killProcessGroup(pid int, sig syscall.Signal) error` as `cmd.Process.Kill()` (no process group concept on Windows).

#### Story 4.4: Unit tests

- [ ] **Task 4.4.1** Write all `managed_process_test.go` tests from §7. Use the same `testdata/helper` binary from Epic 2. Tests covering concurrent Stop(), goroutine leak absence (use `goleak`), and grandchild kill are highest priority.
- [ ] **Task 4.4.2** Run `go test -race -count=10 ./executor/... -run TestManagedProcess` (run 10 times to expose races). All passes green.

---

### EPIC 5: Migration of nolint sites

*Deliverable: 2 of 3 migration candidates replaced; 1 annotated with updated justification.*

#### Story 5.1: Migrate `session/external_tmux_streamer.go`

- [ ] **Task 5.1.1** Replace the `exec.CommandContext` + manual pipe setup in `startControlMode()` with `executor.StartProcess(s.ctx, "tmux", args, executor.WithNoControllingTerminal(), executor.WithGracePeriod(5*time.Second))`. Store `*ManagedProcess` on the struct instead of `*exec.Cmd`.
- [ ] **Task 5.1.2** Update all references from `s.controlModeCmd.Process.Kill()` / `s.controlModeCmd.Wait()` to `s.controlModeProcess.Stop()`.
- [ ] **Task 5.1.3** Expose `Stdout()` reader and replace the `readControlMode(stdout)` goroutine with `p.ScanLines(ctx, s.handleControlModeEvent)` or equivalent.
- [ ] **Task 5.1.4** Run existing `session/` unit and integration tests. Verify no regressions.

#### Story 5.2: Migrate `session/tmux/server_registry.go`

- [ ] **Task 5.2.1** Create an `os.Pipe()` pair for stdin. Wrap `StartProcess` call with `WithProcessStdin(stdinReader)`, `WithNoControllingTerminal()`, `WithGracePeriod(5*time.Second)`. Return `(*ManagedProcess, *bufio.Scanner, *os.File, error)` where `*os.File` is the write end of the stdin pipe.
- [ ] **Task 5.2.2** Update the callsite in `server_registry.go` to store the write end and close it on cleanup.
- [ ] **Task 5.2.3** Update `TrackChildPID` call: use `p.PID()` instead of `cmd.Process.Pid`.
- [ ] **Task 5.2.4** Run `session/tmux/` tests. Verify no regressions.

#### Story 5.3: Update `session/mux/multiplexer.go` comment

- [ ] **Task 5.3.1** Update the `//nolint:norawexec` comment to: `//nolint:norawexec pty.Start() requires a raw *exec.Cmd before cmd.Start(); ManagedProcess cannot be used here`.
- [ ] **Task 5.3.2** Add `cmd.WaitDelay = 5 * time.Second` to the raw cmd (currently missing — this is a bug fix, not just a comment change).
- [ ] **Task 5.3.3** Add `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` if PTY behavior is compatible. Verify with `make install-service` and a manual attach/detach test in tmux.

---

### EPIC 6: Quality gates and lint

*Deliverable: CI passes cleanly; all coverage targets met.*

#### Story 6.1: Coverage and race checks

- [ ] **Task 6.1.1** Run `go test -race -cover ./executor/...`. Achieve 80% coverage for all new files.
- [ ] **Task 6.1.2** Run `make lint` (`golangci-lint run ./executor/...`). Resolve all findings.
- [ ] **Task 6.1.3** Run `go vet ./executor/...`. Zero issues.
- [ ] **Task 6.1.4** Run `make nil-safety` on `executor/`. Resolve NilAway findings if any.

#### Story 6.2: Integration test pass

- [ ] **Task 6.2.1** Run full integration test suite against a local tmux server. Verify Sites 1 and 2 migrations work end-to-end.
- [ ] **Task 6.2.2** Run `make quick-check`. Green.

#### Story 6.3: `nounmanagedprocess` lint rule (deferred)

- [ ] **Task 6.3.1** (deferred — implement after migration complete) Create `tools/lint/nounmanagedprocess/analyzer.go` following the pattern of `norawexec`. Flag `cmd.Start()` calls outside `executor/` that lack a `//nolint:nounmanagedprocess` comment.
- [ ] **Task 6.3.2** Wire into `make lint-custom`.
- [ ] **Task 6.3.3** Verify zero violations after migration. The 1 remaining nolint site (multiplexer) gets `//nolint:nounmanagedprocess` with the existing justification comment.

---

## 9. Dependency and Sequencing

```
Epic 1 (safeexec_pg + audit)
  └─► Epic 2 (ShortLivedCmd)    — depends on CommandContextPG + emitAudit
  └─► Epic 4 (ManagedProcess)   — depends on emitAudit + killProcessGroup
Epic 3 (RlimitConfig)
  └─► Epic 2 (integrated via applyRlimits in build())
  └─► Epic 4 (integrated via applyRlimits in StartProcess)
Epic 2 + Epic 3 + Epic 4
  └─► Epic 5 (Migration)        — depends on ManagedProcess being stable
Epic 5
  └─► Epic 6 (Quality gates)    — final gate before PR
```

Epics 1, 2, 3 can be implemented sequentially in a single session. Epic 4 can begin once Epic 1 is complete. Epic 5 begins only after Epic 4 passes its unit test suite. Epic 6 is the final gate.

---

## 10. Summary

**Epics:** 6
**Stories:** 16
**Tasks:** 45

**Architectural concerns flagged:**

1. `Setpgid + Noctty` default inversion — inverted the default so `noctty = true` is the safe path (§6.1)
2. `ManagedProcess` at multiplexer site cannot be migrated due to `pty.Start()` coupling — annotated and WaitDelay bug fixed instead (§5.3)
3. `os.Pipe()` chosen over `io.Pipe()` for ManagedProcess I/O to prevent deadlocks on backpressure (§3.5)
4. `exec.ErrWaitDelay` must be filtered in reaper goroutine to avoid false error returns (§6.4)
5. `RLIMIT_NOFILE` Linux/runtime interaction requires child-only application via `unix.SysProcAttr.Rlimits` — verify field availability before implementation (§6.5)
6. `nounmanagedprocess` lint rule is deferred until after migration to avoid false positives during the transition (§6.7)
