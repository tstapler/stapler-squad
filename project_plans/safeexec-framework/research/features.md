# safeexec-framework: Feature Design Research

## Research Questions Addressed

1. Builder pattern for exec.Cmd
2. Graceful shutdown of long-running subprocesses
3. Streaming subprocess output without buffering
4. Audit logging structure
5. "Stop if context cancels OR explicitly stopped" pattern
6. Safe io.Reader exposure when process may exit mid-read

---

## 1. Builder Pattern for exec.Cmd

### Conclusion: Functional Options (WithXxx variadic)

The idiomatic Go choice is **functional options** (the Dave Cheney pattern), not method chaining or a separate builder struct. Method chaining (returning `*ShortLivedCmd` from every setter) is valid but adds a separate type that must propagate all exec.Cmd fields. A bare struct literal requires all callsites to be updated when fields change.

Functional options win because:
- The zero value of the builder is valid ("works out of the box" with sensible defaults).
- New options never change the constructor signature — all existing callsites compile unchanged.
- Options are first-class values; callers can build option slices, share them, and compose them.
- The pattern is already in wide use in the Go stdlib-adjacent ecosystem (grpc, zap, otel).

**Idiomatic shape:**

```go
type Option func(*config)

func New(ctx context.Context, name string, args ...string, opts ...Option) *ShortLivedCmd
func WithTimeout(d time.Duration) Option
func WithEnv(key, val string) Option
func WithDir(dir string) Option
func WithStdin(r io.Reader) Option
func WithRedactArgs(indices ...int) Option  // for audit log arg scrubbing
```

The returned `*ShortLivedCmd` should expose `.Run()`, `.Output()`, `.CombinedOutput()` to mirror `exec.Cmd`. It is not a builder itself — all configuration happens through options before construction.

**Alternative considered: method chaining (fluent builder)**

```go
safeexec.New(ctx, "git", "push").Timeout(5*time.Second).Dir(wd).Run()
```

This is popular (e.g., `testcontainers-go` uses it) but requires every method to return the same type, and has an ergonomic pitfall: if `Run()` returns an error, the chain is awkward to break for error handling. Rejected for this codebase because the functional options pattern is already consistent with how grpc-go and otel configure their clients, which Tyler already works with.

**Reference:** The kubernetes/utils `exec.Interface` (reviewed above) uses a thin wrapper struct + setter methods (`SetDir`, `SetStdin`, etc.) — this is the testability-first variant. It's workable but less ergonomic than functional options for callsites that want to compose options.

---

## 2. Graceful Shutdown of Long-Running Subprocesses

### Conclusion: SIGTERM → GracePeriod timer → SIGKILL process group

The canonical Go pattern for graceful subprocess shutdown (from Kubernetes, DoltHub, and the stdlib docs themselves) is a two-phase kill with process group targeting.

**Phase 1: SIGTERM to the process group**

```go
// Setpgid must be set before Start()
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

// In Stop():
syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
```

Sending to `-pid` (negative PID) targets the entire process group, so grandchildren also receive SIGTERM. This is critical when tmux control-mode spawns additional tmux helper processes.

**Phase 2: Timed SIGKILL after grace period**

```go
func (p *ManagedProcess) Stop() error {
    p.mu.Lock()
    if p.stopped {
        p.mu.Unlock()
        return nil
    }
    p.stopped = true
    p.mu.Unlock()

    _ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)

    select {
    case <-p.done:          // exited cleanly
        return <-p.waitErr
    case <-time.After(p.gracePeriod):
        // Force-kill the group
        _ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
        return <-p.waitErr
    }
}
```

**Key design decisions:**
- `p.done` is a `chan struct{}` closed by the goroutine running `cmd.Wait()`.
- `p.waitErr` is a buffered `chan error` (capacity 1) written by that same goroutine exactly once.
- `p.stopped` guard prevents double-stop races.
- Default `GracePeriod` should be **5 seconds** — generous for shell processes, tight enough not to leave zombies long.

**Real-world reference:** The kubernetes/utils `Stop()` method uses `SIGTERM` + `time.AfterFunc(10*time.Second, SIGKILL)`. The envconsul runner uses `stopLock` + `close(DoneCh)` as the notification channel. The DoltHub blog pattern uses a goroutine waiting on a signal channel.

**macOS vs Linux note:** `Pdeathsig` (send signal to child when parent dies) is Linux-only. macOS does not support it. Use only `Setpgid: true` for portability. The grace period + SIGKILL pattern works on both.

---

## 3. Streaming Subprocess Output Without Buffering

### Conclusion: cmd.StdoutPipe() into a goroutine; never ReadAll() for long-lived processes

**Short-lived commands:** `Output()` / `CombinedOutput()` are correct. They buffer output in memory but the process is expected to terminate promptly. For short-lived commands that may emit large output, expose a streaming variant with a caller-provided `io.Writer`.

**Long-lived processes (ManagedProcess):** The stdlib `StdoutPipe()` returns an `io.ReadCloser` connected to the process's stdout via an OS pipe. Reads block until data arrives; the pipe is closed by `cmd.Wait()` when the process exits. This is the right primitive.

**The concurrent-safe wrapper pattern:**

```go
type ManagedProcess struct {
    stdoutR io.Reader   // returned by cmd.StdoutPipe()
    stderrR io.Reader   // returned by cmd.StderrPipe()
    // ... lifecycle fields
}

func (p *ManagedProcess) Stdout() io.Reader { return p.stdoutR }
func (p *ManagedProcess) Stderr() io.Reader { return p.stderrR }
```

Callers must consume from a goroutine. The stdlib guarantees the pipe is not closed until `Wait()` is called, so readers are safe to run concurrently with `Stop()`. When the process exits, reads will return `io.EOF`.

**Critical constraint (from stdlib docs):** You cannot call `Wait()` before all reads from `StdoutPipe()` / `StderrPipe()` are complete. Design implication: `ManagedProcess.Wait()` must not call `cmd.Wait()` directly — it should wait on the goroutines draining pipes first, then call `cmd.Wait()`. Or, set `cmd.Stdout`/`cmd.Stderr` to an `io.Pipe` writer and expose the reader side, so pipes drain independently.

**Preferred approach for ManagedProcess:** Use `io.Pipe()` rather than `cmd.StdoutPipe()`. Set `cmd.Stdout = pipeWriter`. The goroutine inside `ManagedProcess` copies from the process's output into the pipe writer and closes the pipe writer when the process exits. This decouples the process exit lifecycle from the read lifecycle and gives the caller a clean `io.Reader` that returns `io.EOF` exactly once.

```go
stdoutR, stdoutW := io.Pipe()
cmd.Stdout = stdoutW
// goroutine: io.Copy to stdoutW, then stdoutW.CloseWithError(err)
```

**Draining stderr (when not exposed):** Always drain stderr to `io.Discard` in a goroutine, even when the caller only cares about stdout. Leaving stderr undrained causes `Wait()` to block if the OS pipe buffer fills (typically 64KB on Linux).

**Line-oriented callback API:** Expose an optional `ScanLines(ctx context.Context, fn func(line string)) error` method that uses `bufio.Scanner` over `Stdout()`. This is the ergonomic choice for callers who process output line-by-line (e.g., tmux control-mode event parsing). The caller blocks until the process exits or ctx is cancelled.

---

## 4. Audit Logging Structure

### Conclusion: `log/slog` with a structured `SubprocessEvent` group

Go 1.21 added `log/slog` to the stdlib. It is the standard for structured logging going forward. The existing project uses `log.InfoLog` / `log.DebugLog` (which appear to be `*log.Logger` wrappers). The audit log should emit via `slog` so log aggregators can filter by field.

**Event shape:**

```go
slog.InfoContext(ctx, "subprocess.start",
    slog.String("cmd", name),
    slog.Any("args", redactedArgs),
    slog.String("dir", dir),
    slog.Int("pid", cmd.Process.Pid),
)

slog.InfoContext(ctx, "subprocess.exit",
    slog.String("cmd", name),
    slog.Int("pid", pid),
    slog.Int("exit_code", exitCode),
    slog.Duration("duration", elapsed),
    slog.Bool("killed_by_timeout", timedOut),
    slog.Bool("killed_by_context", ctxKilled),
)
```

**Design decisions:**
- Emit at `slog.LevelDebug` by default; only escalate to `Info` on non-zero exit or kill.
- Use `slog.Group("subprocess", ...)` to namespace fields so log aggregators can index `subprocess.exit_code`.
- Arg redaction: accept a `WithRedactArgs(indices ...int)` option that replaces those positional args with `"<redacted>"` in the log. Secrets (tokens, passwords) at known positions (e.g., `git -c credential.helper=...`) must never appear in logs.
- Context propagation: `slog.InfoContext(ctx, ...)` passes the context so that trace IDs injected into the context appear in the log entry automatically (when a slog handler is configured to extract them).
- Opt-in toggle: a `WithAuditLog(logger *slog.Logger)` option on both `ShortLivedCmd` and `ManagedProcess` enables audit logging. A package-level `SetDefaultLogger(*slog.Logger)` enables it globally with `slog.Default()` as the fallback.

**Why not a custom format:** The project will likely integrate with Datadog (per `.claude/docs/opentelemetry.md`). Structured slog output maps cleanly to Datadog log attributes. A custom format would require a separate parser.

---

## 5. "Stop if context cancels OR if explicitly stopped" Pattern

### Conclusion: `select` on `ctx.Done()` and a `stopCh chan struct{}`

The standard Go pattern for "stop on either condition" is a `select` with two channels. This is used throughout Kubernetes, HashiCorp's envconsul, and consul-template.

**Shape inside ManagedProcess:**

```go
type ManagedProcess struct {
    ctx     context.Context
    cancel  context.CancelFunc   // derived context for the cmd
    stopCh  chan struct{}         // closed by Stop()
    done    chan struct{}         // closed by the Wait goroutine
    waitErr chan error            // buffered(1), written by Wait goroutine
    // ...
}

func (p *ManagedProcess) run() {
    defer close(p.done)
    go func() {
        select {
        case <-p.ctx.Done():  // parent context cancelled
            p.stopWithSIGTERM()
        case <-p.stopCh:      // explicit Stop() called
            p.stopWithSIGTERM()
        case <-p.done:        // process exited on its own
            return
        }
    }()
    p.waitErr <- p.cmd.Wait()
}

func (p *ManagedProcess) Stop() error {
    once.Do(func() { close(p.stopCh) })
    <-p.done
    return <-p.waitErr
}
```

**Why not just use context cancel on the cmd directly?**

`exec.CommandContext` sets `cmd.Cancel = cmd.Process.Kill` by default (SIGKILL immediately). For ManagedProcess we want SIGTERM first. The solution is to construct the cmd with `exec.CommandContext` (so the ctx plumbing is in place) but override `cmd.Cancel` to our SIGTERM-then-SIGKILL function before calling `cmd.Start()`. This is the `cmd.Cancel` field introduced in Go 1.20.

```go
cmd := exec.CommandContext(derivedCtx, name, args...)
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
cmd.WaitDelay = gracePeriod + 1*time.Second // ensure Wait() returns after SIGKILL
cmd.Cancel = func() error {
    return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
    // SIGKILL is the WaitDelay fallback
}
```

When `derivedCtx` is cancelled (either because the parent context cancelled, or because `Stop()` calls `cancel()`), `cmd.Cancel` fires SIGTERM. If the process hasn't exited within `WaitDelay`, the runtime sends SIGKILL and closes pipes. `cmd.Wait()` then returns `ErrWaitDelay` (not an error worth propagating in most cases).

This eliminates the need for the manual goroutine-with-select approach in most cases, but the `stopCh` is still needed to trigger the `cancel()` from `Stop()` without depending on the parent context.

---

## 6. Safe io.Reader When Process May Exit Mid-Read

### Conclusion: io.Pipe() decouples process lifetime from read lifetime

The core hazard: if a caller holds an `io.ReadCloser` from `cmd.StdoutPipe()` and the process exits, `cmd.Wait()` closes the pipe. If the caller is blocked in a `Read()` call, it unblocks with `io.EOF`. This is correct behavior — the caller handles EOF and exits its read loop. The hazard is if `Wait()` is called before the reader goroutine exits, which causes a race.

**Safe pattern:** The goroutine that calls `cmd.Wait()` must not return until all goroutines reading from `StdoutPipe()` / `StderrPipe()` have exited. Use a `sync.WaitGroup`:

```go
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    io.Copy(dst, stdoutPipe) // blocks until pipe closed (EOF or error)
}()
cmd.Wait()     // WRONG: may close pipe before goroutine exits
wg.Wait()
cmd.Wait()     // correct: all readers done before Wait
```

**Preferred: io.Pipe() for ManagedProcess**

Use `io.Pipe()` so the exposed `io.Reader` is not the raw OS pipe:

```go
pr, pw := io.Pipe()
cmd.Stdout = pw
// goroutine:
go func() {
    err := cmd.Wait()
    if err != nil {
        pw.CloseWithError(err)
    } else {
        pw.Close()
    }
}()
// caller:
p.stdoutReader = pr  // safe to read from; gets io.EOF when process exits
```

This provides correct EOF semantics regardless of when the caller reads. The `io.Pipe` is concurrency-safe. The `pw.CloseWithError(err)` propagates the process's exit error to any reader blocked in `Read()`.

**What if no one reads from stdout?** If `ManagedProcess` is started but the caller never reads from `Stdout()`, the OS pipe buffer fills and the process blocks on writes. Always drain in a goroutine even if the caller doesn't call `Stdout()`:

```go
func (p *ManagedProcess) Start() error {
    // ...
    if p.stdoutReader == nil {
        cmd.Stdout = io.Discard  // drain automatically
    }
    // ...
}
```

Or expose a `ConsumeOutput(io.Writer)` option that sets `cmd.Stdout` before start and does not expose a reader.

---

## Summary of Design Recommendations

| Concern | Recommendation | Key Reference |
|---|---|---|
| Builder pattern | Functional options (`WithXxx ...Option`), zero-value valid | Dave Cheney pattern; gRPC style |
| Graceful stop | `cmd.Cancel` = SIGTERM to pgid; `WaitDelay` = grace + 1s for SIGKILL fallback | Go 1.20 `cmd.Cancel` field; DoltHub blog |
| Output streaming | `io.Pipe()` bridging `cmd.Stdout` → caller reader; drain in goroutine | stdlib StdoutPipe docs; io.Pipe internals |
| Audit logging | `log/slog` with `slog.Group("subprocess", ...)`, opt-in per call site | Go 1.21 slog; OTel context propagation |
| Dual-stop pattern | `cmd.Cancel` driven by derived context; `Stop()` calls `cancel()` on that derived context | Go 1.20 `exec.CommandContext` + `cmd.Cancel` |
| Safe reader | `io.Pipe()` writer closed in Wait goroutine; `pw.CloseWithError(err)` propagates exit error | io.Pipe docs; sync.WaitGroup ordering |

---

## Existing Codebase Patterns (Baseline)

The current `external_tmux_streamer.go` and `session/tmux/server_registry.go` demonstrate the **pre-ManagedProcess** pattern:
- Raw `exec.CommandContext` with `//nolint:norawexec` comments
- `cmd.StdoutPipe()` + `cmd.StderrPipe()` set up before `cmd.Start()`
- Output consumed in goroutines (`go s.readControlMode(stdout)`, `go s.drainStderr(stderr)`)
- No `Setpgid`, no SIGTERM, process terminated only when the context cancels (which sends SIGKILL)

These are the primary candidates to be replaced by `ManagedProcess` in the implementation phase.

The `TimeoutExecutor` demonstrates the **correct WaitDelay pattern** for short-lived commands: it re-wraps with `exec.CommandContext` and always sets `WaitDelay = 2*time.Second`. The new `ShortLivedCmd` builder should consolidate this logic and add `Setpgid: true` to prevent grandchild orphans.
