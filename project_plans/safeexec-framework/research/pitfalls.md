# Go Subprocess Management: Pitfalls and Sharp Edges

Research date: 2026-05-05
Scope: pitfalls beyond the already-fixed WaitDelay zombie bug

---

## 1. Zombie Processes Beyond WaitDelay

### Double-Wait panic
`cmd.Wait()` must be called exactly once. A second call on the same `*Cmd` panics at runtime with "exec: Wait was already called". This bites in patterns where both a timeout goroutine and the normal call path call `Wait`. The fix is a `sync.Once` guard.

### Goroutine leak → perpetual zombie
If the goroutine responsible for calling `Wait` exits (e.g., due to a panic, a `select` with `default:`, or a context `Done` channel winning the race) before `Wait` returns, the process entry stays in the kernel's process table until the Go process exits. No GC or finalizer reaps it (see Section 7 below).

### Grandchild pipe inheritance (Issue #23019)
When `Cmd.Stdout` or `Cmd.Stderr` is a non-`*os.File` writer (a `*bytes.Buffer`, `io.Pipe`, etc.), `os/exec` creates internal goroutines that copy via `io.Copy`. `Wait` does not return until those goroutines reach EOF. If the direct child C1 spawns a grandchild C2 and passes it an inherited copy of the pipe fd, C2 holds the write end open. `Wait` blocks until C2 exits — even though C1 has long since exited. This is the core motivation for `WaitDelay`. Without it, the caller has no way to bound how long it waits unless it uses `*os.File` pipes directly.

**Concrete scenario in this codebase**: Any `claude` subprocess that forks a background worker (e.g., a build watcher) will hold open the stdout pipe indefinitely, causing the parent Go process to block in `Wait`.

### `os.Process.Release()` does not reap
The docs for `Release` say "It only needs to be called if Wait is not." In practice, calling `Release` without `Wait` does **not** reap the process; it only releases the `os.Process` handle. The zombie remains in the kernel. (golang/go#36534)

### Reaping race with `os.FindProcess`
On Unix, `os.FindProcess` always succeeds (it does not check whether the PID exists). Calling `.Wait()` on a `*Process` obtained via `FindProcess` for a PID that the current process did not start will either fail with `ECHILD` or accidentally reap an unrelated process that recycled the PID.

---

## 2. Process Groups on macOS vs Linux

### `Setpgid` behavior is POSIX-compliant on both, but...
Setting `SysProcAttr{Setpgid: true}` places the child in a new process group with pgid == child's pid. This works identically on macOS and Linux at the kernel level.

### TOCTOU race on pgid membership
The `setpgid` call happens inside the child after `fork` but before `exec`. If the parent immediately tries to signal `syscall.Kill(-pid, syscall.SIGTERM)` (negative pid = process group), there is a narrow window where the child has not yet called `setpgid`, so the signal either targets the parent's group or misses. The POSIX remedy is to call `setpgid(child_pid, child_pid)` in **both** parent and child, but `os/exec` does not expose this. Practical mitigation: add a small synchronization point (e.g., wait for first output) before sending process-group signals.

### Orphaned process group: SIGHUP + SIGCONT (Linux only)
On Linux, when the last process in a process group exits and that group becomes "orphaned" (no remaining member's parent is in a different group within the same session), the kernel automatically delivers `SIGHUP` followed by `SIGCONT` to any stopped processes in the group. macOS and other BSDs do **not** reliably do this. Code that depends on `SIGHUP` cleanup of orphaned groups for graceful shutdown will silently fail on macOS.

### `Setpgid` + tmux: controlling terminal complications
When the Go process runs inside a tmux pane, it already has a controlling terminal (the tmux PTY). A child started with `Setpgid: true` inherits the same controlling terminal unless `Setsid: true` (new session) is also set. This creates two risks:
1. **Job-control signals leak**: If the child reads from or writes to the inherited PTY while the tmux pane loses its controlling terminal (e.g., on detach or window close), the child receives `SIGHUP`. With only `Setpgid: true` (not `Setsid`), the child's process group is different from the shell's, so keyboard `^C` won't reach it — but `SIGHUP` on PTY close **will**.
2. **Background process group SIGTTOU/SIGTTIN**: If a child in a non-foreground process group tries to read from or write to the controlling terminal without `Setsid`, it gets `SIGTTIN`/`SIGTTOU`, which suspends it (SIGSTOP). In a tmux context this is opaque — the process appears hung.

**For this codebase**: subprocesses that should be fully isolated from the tmux controlling terminal need both `Setpgid: true` **and** `Noctty: true` (or `Setsid: true`) to avoid SIGHUP on pane close and SIGTTIN/SIGTTOU on terminal I/O.

### `Pdeathsig` is Linux-only
`SysProcAttr.Pdeathsig` (send signal to child when parent dies) is not available on macOS/Darwin. Code using it must be guarded with a build tag. On macOS the equivalent idiom is a keepalive goroutine that sends `SIGTERM` to the child via a `context.Done` channel.

---

## 3. `io.Pipe` Deadlocks

### The canonical deadlock
```
pr, pw := io.Pipe()
cmd.Stdout = pw
go cmd.Run()            // goroutine A: blocks in Wait until pw is closed
io.Copy(dst, pr)        // goroutine B: blocks until pr sees EOF
// pw is never closed → both goroutines block forever
```
`io.Pipe` has no kernel buffer. Every write blocks until a corresponding read drains it. The write end (pw) must be explicitly closed — it is not closed by `cmd.Wait`. The pattern of assigning an `io.PipeWriter` to `cmd.Stdout` and expecting `Wait` to close it is incorrect and deadlocks.

### StdoutPipe / Wait ordering
The docs state: "It is incorrect to call Wait before all reads from the pipe have completed." Calling `Wait` while a scanner goroutine is still blocked in `scanner.Scan()` on a `StdoutPipe` is a data race (golang/go#19685, golang/go#28461). The only safe pattern: drain the pipe to EOF first, then call `Wait`.

### Non-`*os.File` writer + CommandContext hang (fixed in Go 1.20 with WaitDelay)
Prior to Go 1.20, assigning a non-`*os.File` writer to `cmd.Stdout` with `CommandContext` caused `Wait` to block indefinitely after context cancellation because the internal copy goroutine was still blocked on `Read` from the pipe (golang/go#18874, golang/go#21922). The `WaitDelay` field introduced in Go 1.20 is the official fix. **Do not use pre-1.20 patterns without WaitDelay set.**

### Scanner EOF never arrives after SIGKILL
When the subprocess is `SIGKILL`ed, the kernel closes its end of the pipe. However, if a goroutine is blocked on `scanner.Scan()` and the scanner wraps an `io.PipeReader` (not the raw pipe fd), the `PipeReader.Read` blocks until someone calls `PipeWriter.Close()`. `cmd.Wait` does not close a user-provided `io.PipeWriter`. Result: scanner hangs forever. Use `os.Pipe()` or `cmd.StdoutPipe()` (which `Wait` does close) rather than `io.Pipe()` when reading command output.

### Stderr and stdout goroutine fan-in
If `cmd.Stdout` and `cmd.Stderr` are both set to the same non-`*os.File` writer, and if the writer is not concurrency-safe, the two internal copy goroutines will race on writes. The `os/exec` docs note: "If Stdout and Stderr are the same writer, and have a type that can be compared with ==, at most one goroutine at a time will call Write." But this protection only applies when Go can compare them with `==` — if the writer is wrapped in an interface, the comparison may fail and concurrent writes can occur.

---

## 4. `exec.Cmd` Concurrency Safety

### The rule
`exec.Cmd` is **not concurrency-safe**. Fields must be set before `Start()` and not read concurrently with an ongoing `Run()` or `Wait()`.

### `Process` field race
`cmd.Process` is set by `Start()`. Reading it from another goroutine before `Start()` returns is a data race. This is the "safe access" pattern from Dolt's blog: use `Start()` + `Wait()` explicitly so you hold the `*Process` after `Start` returns; never read `cmd.Process` concurrently with a `Run()` call (golang/go#28461).

### `StdinPipe` + `Wait` race (Issue #9307)
Concurrent writes to `StdinPipe` while `Wait` is in progress produce a data race. The Go race detector flags this. The correct pattern: close `StdinPipe` before calling `Wait`, ensuring the copy goroutine finishes before `Wait` proceeds.

### `Cmd.Err`
`cmd.Err` is set during `Command()` by `LookPath`. Reading it after construction is safe. But if `cmd.Err != nil` and you call `Start()`, `Start()` returns the `LookPath` error immediately. Checking `cmd.Err` before `Start()` avoids a confusing error message.

### Cmd cannot be reused
After `Start`, `Run`, `Output`, or `CombinedOutput`, the `Cmd` is consumed. Calling `Start` a second time returns `"exec: already started"`. A new `*Cmd` must be constructed for each invocation.

---

## 5. Context Cancellation Races

### Cancel arrives between `Start` and first byte
When `CommandContext` is used and the context is already cancelled at the moment `Start` is called, the behavior depends on Go version:
- Go < 1.20: `Start` may succeed and then the `watchCtx` goroutine immediately sends `SIGKILL`. The process may emit zero bytes of output. `Wait` returns a `context.Canceled` error.
- Go >= 1.20: same, but with `WaitDelay` the pipes are forcibly closed after the delay, preventing indefinite blocking.

### Race between SIGKILL and first output byte
`watchCtx` monitors `ctx.Done()` in a separate goroutine after `Start`. There is a window between `Start` returning and `watchCtx` receiving from `ctx.Done()`. During that window the process runs. The caller cannot rely on "no output was produced" as evidence that the process did not start if the context was already expired.

### Default cancel is `os.Process.Kill` (SIGKILL), not SIGTERM
`CommandContext` sets `cmd.Cancel` to `cmd.Process.Kill`. There is no grace period. A long-running subprocess that needs to write a checkpoint before shutdown will be killed with no opportunity to flush. The fix (Go 1.20+) is to set a custom `cmd.Cancel`:
```go
cmd.Cancel = func() error {
    return cmd.Process.Signal(syscall.SIGTERM)
}
cmd.WaitDelay = 5 * time.Second  // escalate to SIGKILL after delay
```
Without `WaitDelay`, a custom `Cancel` that only sends `SIGTERM` will block `Wait` forever if the process ignores it (golang/go#22757, #50436).

### Context cancellation does not kill process group
`cmd.Cancel` targets only `cmd.Process.Pid`. If the subprocess has spawned children (and `Setpgid` was not set), those children are not killed when the context cancels. They become orphans under `init`. To kill the whole tree, `Cancel` must send `-pgid` via `syscall.Kill`.

---

## 6. `Setpgid` and Job Control in a tmux Pane

### The core issue
stapler-squad runs inside tmux. tmux creates a new session for each window/pane. Processes inside a tmux pane have a controlling terminal (the tmux PTY). Subprocesses started with `Setpgid: true` are in a different process group but **share the same session** and therefore the **same controlling terminal** unless `Setsid: true` is also used.

### What `Setpgid: true` alone does (and does not do)
- Protects child from keyboard-generated `SIGINT`/`SIGQUIT` (those go to the foreground process group only). GOOD.
- Does **not** detach from the controlling terminal. The child can still get `SIGHUP` if the PTY closes.
- Does **not** prevent `SIGTTIN`/`SIGTTOU` if the child reads/writes the terminal from background.

### What `Setsid: true` does
Creates a new session. The child has no controlling terminal at all. It will never receive `SIGHUP` from PTY close, `SIGTTIN`, or `SIGTTOU`. This is the correct choice for long-running daemon-style subprocesses in this codebase.

### Orphan group SIGHUP scenario in tmux
When a tmux window is closed, tmux closes the PTY master. All processes with that PTY as controlling terminal receive `SIGHUP`. With `Setpgid: true` (but not `Setsid`), the subprocess is still in the same session and will receive `SIGHUP` on tmux window close. With `Setsid: true`, it will not.

### Recommendation for this codebase
Subprocess categories:
- **claude/aider processes** (need to receive terminal input): use `Setpgid: true` only; they need the controlling terminal.
- **Build watchers, git operations, background helpers**: use `Setsid: true` to fully detach from terminal lifecycle.
- When killing by process group: `syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)` requires that the process started with `Setpgid: true`; with `Setsid: true` the pgid is still the child's pid.

---

## 7. `runtime.SetFinalizer` for Process Cleanup

### Why it seems appealing
A finalizer on `*exec.Cmd` or `*os.Process` could auto-call `Wait` when the struct becomes unreachable, preventing zombie accumulation.

### Why it is dangerous and unreliable

**Finalizer is not guaranteed to run.**
The GC is not obligated to run finalizers before program exit, and on a large heap the GC may run infrequently. A program with a large heap can accumulate hundreds of zombie processes without any finalizer firing. (golang-dev thread: "a sufficiently large heap will not action finalisers at all")

**Finalizer runs in a separate goroutine after GC.**
The finalizer goroutine has no ordering guarantees relative to other goroutines. If a finalizer calls `Wait` and the caller's goroutine has already exited without calling `Wait`, the zombie may have been around for seconds or minutes already.

**Finalizer resurrects the object once.**
`SetFinalizer` runs once; after that, the finalizer is cleared. If the object is re-referenced from within the finalizer (preventing collection), the finalizer never runs again. This makes it easy to write a finalizer that silently does nothing.

**`AddCleanup` (Go 1.24+) is better but still non-deterministic.**
`runtime.AddCleanup` (golang/go#67535) resolves the resurrection problem and allows multiple cleanups per object, but it still cannot guarantee timing. It should not be used for correctness-critical cleanup.

**Correct pattern**: enforce explicit `Wait` via a `defer` at the call site, or use a supervisor goroutine with a `sync.WaitGroup`. If a process handle outlives a known scope, track it in a registry and drain it at shutdown.

---

## 8. `RLIMIT_AS` on macOS

### macOS does not enforce `RLIMIT_AS`
On Linux, `RLIMIT_AS` limits the total virtual address space. A process exceeding it gets `ENOMEM` on `mmap`/`brk`. On macOS/Darwin, `RLIMIT_AS` is defined in `<sys/resource.h>` and accepted by `setrlimit` without error, but the kernel does **not enforce it**. A process will allocate beyond the limit without any error. (Stack Overflow, retdec/issues#379, rdrr.io/cran/unix docs)

### `RLIMIT_AS` aliases `RLIMIT_RSS` on macOS
Some Darwin sources treat `RLIMIT_AS` and `RLIMIT_RSS` as interchangeable. `RLIMIT_RSS` (resident set size) is advisory on modern macOS — the VM subsystem ignores it during memory pressure events. Neither limit reliably kills or throttles a process on macOS.

### Go runtime itself is affected
The Go runtime allocates a large reserved virtual address space at startup (hundreds of MB to several GB of virtual space is normal). If `RLIMIT_AS` were enforced (e.g., in a container with a strict limit), `cmd.Start()` could fail with `ENOMEM` even though actual physical memory usage is low (golang/go#38010). This is observed on Linux; on macOS it is not an issue because the limit is not enforced.

### Safe cross-platform approach
Do not use `RLIMIT_AS` for memory control on macOS. For subprocess memory limiting on macOS, consider:
- cgroups (not available natively on macOS; available in containers)
- `RLIMIT_DATA` (heap limit — partially effective, but Go's allocator bypasses it via `mmap`)
- `proc_pidinfo` / `libproc` for monitoring rather than enforcement
- Running subprocesses inside a Docker container on macOS to get Linux cgroup enforcement

---

## 9. Security: Command Injection via `LookPath`

### The pre-Go 1.19 `.` in PATH vulnerability
Before Go 1.19, if the current directory (`.`) was in `PATH`, `exec.LookPath("prog")` could resolve to `./prog` — an attacker-controlled executable. This was the root of CVE-2022-41716 class issues. Since Go 1.19, `LookPath` returns `exec.ErrDot` if the resolution would land in the current directory, and `exec.Command` refuses to run it.

### `sh -c` with user input
Passing user-controlled strings via `exec.Command("sh", "-c", userInput)` is command injection. `os/exec` does not invoke a shell on its own, but explicitly invoking `sh -c` negates that protection. Arguments passed as separate positional parameters are safe; string concatenation into shell commands is not.

### `SysProcAttr.CmdLine` on Windows
On Windows, processes receive a raw command-line string. `exec.Command` quotes arguments using `CommandLineToArgvW` rules. Programs that parse their own command line differently (batch files, `msiexec.exe`, legacy apps) can be exploited even with correctly quoted args if `SysProcAttr.CmdLine` is set to a user-supplied string.

### LookPath PATH manipulation
If the process's `PATH` environment variable is attacker-controlled (e.g., inherited from a user-supplied environment), `exec.LookPath` will find an attacker's binary. Always use absolute paths for security-sensitive subprocesses, or set `cmd.Env` explicitly rather than inheriting `os.Environ()`.

---

## 10. Real-World Postmortems

### HashiCorp Vault (zombie accumulation)
In earlier versions of Vault's plugin system, plugin subprocesses were started but `Wait` was not reliably called on unhealthy shutdown paths. Under load, hundreds of defunct processes accumulated. Fix: supervisor pattern where every `cmd.Start()` is paired with a dedicated goroutine that calls `Wait` and reports the exit code, regardless of the primary control flow.

### Docker (WaitDelay equivalent)
Docker's `containerd` implements its own `WaitDelay`-like mechanism to bound how long it waits for container process pipes to close after the main process exits. This predates Go's `WaitDelay` and was motivated by exactly the grandchild pipe inheritance problem described in Issue #23019.

### Kubernetes kubelet (context cancel + SIGKILL race)
An early kubelet bug: container lifecycle hooks were run with `CommandContext`, which defaults to SIGKILL on cancellation. A hook that needed to flush state to disk was killed mid-write on timeout, corrupting the output. Resolution: custom `Cancel` with SIGTERM, followed by WaitDelay-based SIGKILL escalation.

---

## Summary for Library Design

| Pitfall | Mitigation |
|---|---|
| Double-Wait panic | `sync.Once` wrapping `Wait` |
| Goroutine leak / zombie | Dedicated reaper goroutine per `Cmd`, never abandon |
| Grandchild pipe hold | Always set `WaitDelay`; prefer `*os.File` pipes for long-running cmds |
| `Setpgid` + tmux SIGHUP | Add `Noctty: true` or `Setsid: true` for background processes |
| `io.Pipe` deadlock | Use `cmd.StdoutPipe()` or `os.Pipe()`, not `io.Pipe()`, for cmd output |
| Concurrent `Process` access | Always use `Start()` + `Wait()`; never read `Process` during `Run()` |
| Context cancel kills only parent | Custom `Cancel` targeting `-pgid` to kill process tree |
| Default cancel is SIGKILL | Set custom `Cancel` + `WaitDelay` for graceful shutdown |
| Finalizer unreliability | Explicit `defer cmd.Wait()` or supervisor goroutine; no finalizers |
| `RLIMIT_AS` on macOS | Do not rely on; use container cgroups or monitoring instead |
| Shell injection via `sh -c` | Never pass user input as shell string; use separate args |
| PATH hijack | Use absolute paths or sanitize `cmd.Env` |
