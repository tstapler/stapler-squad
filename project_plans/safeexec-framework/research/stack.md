# safeexec-framework: Technology Stack Research

Researched: 2026-05-05

---

## 1. `os/exec` stdlib — Process Group Management

### `SysProcAttr` fields relevant to process group management

`exec.Cmd.SysProcAttr` is typed `*syscall.SysProcAttr` and is passed directly to `os.StartProcess`. The fields that matter for this library:

**On both Linux and macOS:**

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true, // Put child in its own process group (PGID == child PID)
    Pgid:    0,    // 0 means use child's own PID as PGID
}
```

Setting `Setpgid: true` is the critical operation: the child and all its own children will share a process group, making it possible to signal the entire group at once.

**Linux only:**

```go
Pdeathsig: syscall.SIGKILL, // Send signal to child when the creating thread dies
```

`Pdeathsig` is not available on macOS. The field does not exist in `darwin` `SysProcAttr`. macOS has no direct equivalent.

**Also Linux only (relevant for resource limits via pre-exec hooks):**

`Cloneflags`, `Unshareflags`, `UidMappings`, `GidMappings`, `CgroupFD` — all Linux-only namespace and cgroup features. Not relevant to the immediate plan but available if isolation escalates to cgroup-based resource limits.

**macOS `SysProcAttr` is a strict subset of Linux's.** The darwin struct has only: `Chroot`, `Credential`, `Ptrace`, `Setsid`, `Setpgid`, `Setctty`, `Noctty`, `Ctty`, `Foreground`, `Pgid`. No `Pdeathsig`, no `Cloneflags`, no `CgroupFD`.

### `cmd.Cancel` field (Go 1.20+)

`exec.CommandContext` sets `cmd.Cancel` to call `cmd.Process.Kill()` by default. The `Cancel` field is a `func() error` that fires when the context is done. This is the right hook to send SIGTERM to the process **group**:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
cmd.Cancel = func() error {
    // Send SIGTERM to the whole process group
    pgid, err := syscall.Getpgid(cmd.Process.Pid)
    if err != nil {
        return cmd.Process.Kill() // fallback
    }
    return syscall.Kill(-pgid, syscall.SIGTERM)
}
cmd.WaitDelay = 5 * time.Second // After WaitDelay: os.Process.Kill() (single process only)
```

**Critical finding:** `WaitDelay` expiry calls `c.Process.Kill()` (line 828 of `exec.go`), which kills only the process, not the group. To kill the group on WaitDelay expiry, the `Cancel` func approach with SIGTERM + `WaitDelay` as a backstop is the right pattern — but if WaitDelay fires, it only kills the parent. The library must explicitly send SIGKILL to `-pgid` in the `ManagedProcess.Stop()` flow after the grace period.

---

## 2. Killing a Process Group — Correct Pattern

### The canonical idiom

```go
// Setup (before Start):
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

// Kill entire group (after Start):
pgid, err := syscall.Getpgid(cmd.Process.Pid)
if err != nil {
    // fallback: kill just the process
    cmd.Process.Kill()
    return
}
syscall.Kill(-pgid, syscall.SIGKILL) // negative PGID = signal the group
```

The negative sign on `pgid` is the POSIX convention: `kill(-pgid, sig)` sends `sig` to every process in group `pgid`. This works identically on macOS and Linux.

### Why `syscall.Getpgid` instead of using `cmd.Process.Pid` directly

When `Setpgid: true` and `Pgid: 0`, the child's PGID equals its own PID. So `-cmd.Process.Pid` is equivalent to `-pgid` in the simple case. However, `syscall.Getpgid` is safer if the child was placed into an existing group or if the PID was reused between Start and the kill call. The `Getpgid` call is a system call, not free, but acceptable in the non-hot path.

### SIGTERM before SIGKILL — the two-phase stop

The library must implement:
1. `syscall.Kill(-pgid, syscall.SIGTERM)` — give the group a chance to shut down cleanly
2. Wait up to `GracePeriod` (e.g. 5s)
3. `syscall.Kill(-pgid, syscall.SIGKILL)` — force terminate

This is the standard supervisor pattern. Do **not** rely on `WaitDelay` alone for group kill; it only kills the process, not the group.

### `golang/go#53199` — negative PID issue

There is a known Go issue (golang/go#53199) noting that `syscall.Kill` on some platforms casts the argument to `uint32`, making negative values wrong. The correct approach is to use `-pgid` as `int` through the `syscall.Kill(pid int, sig Signal)` signature, not to cast to `uint32`. Go's `syscall.Kill` has signature `func Kill(pid int, sig Signal) error` and handles the negation correctly.

---

## 3. `WaitDelay` Interaction with Context Cancellation and Process Groups (Go 1.20+)

`WaitDelay` was added in Go 1.20 (proposal #50436, milestone Go1.20). Its semantics:

1. **Timer starts when**: the context is done OR `cmd.Wait` observes the process has already exited — whichever comes first.
2. **On expiry**: `os.Process.Kill()` is called on the **single process** (not the group), then all I/O pipes are forcibly closed.
3. **Return value**: if `WaitDelay` fires and the process had otherwise exited successfully, `cmd.Wait` returns `exec.ErrWaitDelay` instead of `nil`.

**Key interaction with process groups:**

- `WaitDelay` only kills `cmd.Process`. If the child spawned grandchildren that hold the stdout/stderr pipes open, `WaitDelay` will close the pipes but the grandchildren remain alive as orphans.
- Setting `Setpgid: true` + a custom `cmd.Cancel` that sends SIGTERM to `-pgid` + `WaitDelay` as a backstop is the correct three-layer defense. The custom Cancel sends SIGTERM to the group; if the group doesn't die in `WaitDelay`, the pipes are closed and Wait returns; but the group is still alive. A separate explicit SIGKILL to `-pgid` must be sent after `WaitDelay` for full cleanup.
- The stdlib does not provide this automatically. The library must implement it.

**`cmd.Cancel` vs context.Done:**

```go
cmd.Cancel = func() error {
    // This fires when ctx.Done() fires
    pgid, _ := syscall.Getpgid(cmd.Process.Pid)
    return syscall.Kill(-pgid, syscall.SIGTERM) // SIGTERM to group
    // WaitDelay then acts as the deadline for pipes + kills cmd.Process
}
cmd.WaitDelay = gracePeriod
```

After `WaitDelay` fires, a goroutine watching for pipe closure + `ErrWaitDelay` must follow up with `syscall.Kill(-pgid, syscall.SIGKILL)`.

---

## 4. Resource Limits (`rlimit` syscalls)

### Constants available per platform

**Linux** (`syscall` package, `zerrors_linux_amd64.go`):
```
RLIMIT_AS     = 0x9  // Virtual address space
RLIMIT_CORE   = 0x4  // Core dump size
RLIMIT_CPU    = 0x0  // CPU time (seconds)
RLIMIT_DATA   = 0x2  // Data segment size
RLIMIT_FSIZE  = 0x1  // Max file size
RLIMIT_NOFILE = 0x7  // Open file descriptors
RLIMIT_STACK  = 0x3  // Stack size
```

**Linux** (`golang.org/x/sys/unix`, `zerrors_linux_amd64.go`) additionally has:
```
RLIMIT_MEMLOCK = 0x8  // Locked memory
RLIMIT_NPROC   = 0x6  // Subprocesses
RLIMIT_RSS     = 0x5  // Resident set size (soft limit; kernel does NOT enforce on modern Linux)
```

**macOS** (`golang.org/x/sys/unix`, `zerrors_darwin_amd64.go`):
```
RLIMIT_AS     = 0x5  // Virtual address space
RLIMIT_CORE   = 0x4
RLIMIT_CPU    = 0x0  // CPU time
RLIMIT_DATA   = 0x2
RLIMIT_FSIZE  = 0x1
RLIMIT_MEMLOCK = 0x6
RLIMIT_NOFILE = 0x8  // Open file descriptors
RLIMIT_NPROC  = 0x7
RLIMIT_RSS    = 0x5  // Resident set size (DEFINED but NOT enforced on macOS; Apple removed enforcement)
RLIMIT_STACK  = 0x3
RLIMIT_CPU_USAGE_MONITOR = 0x2  // macOS-specific
```

### Critical platform differences

- **`RLIMIT_RSS`**: Defined on both Linux (as `0x5`) and macOS (as `0x5`). However:
  - Linux: Defined in `syscall` package but also **not enforced** on modern Linux kernels (kernel docs: "This limit has effect only in Linux 2.4.x, x < 30"). The constant is present in `golang.org/x/sys/unix` but NOT in the standard `syscall` package.
  - macOS: Defined in `golang.org/x/sys/unix` but Apple has not enforced RSS limits since macOS 10.x. `setrlimit(RLIMIT_RSS, ...)` returns 0 (success) but has no effect.
  - **Conclusion**: `RLIMIT_RSS` is not a viable cross-platform memory limit. Use `RLIMIT_AS` (virtual address space) instead if memory bounding is needed; it is enforced on Linux and has some effect on macOS.

- **`RLIMIT_CPU`**: Works on both Linux and macOS — process gets SIGXCPU when it hits the soft limit, SIGKILL at the hard limit. This is the most portable resource constraint.

- **`RLIMIT_NOFILE`**: Works on both. Note: Go's runtime already adjusts `RLIMIT_NOFILE` upward at init (in `syscall/rlimit.go`) and stores the original in `origRlimitNofile`. Calling `syscall.Setrlimit(RLIMIT_NOFILE, ...)` stores nil in `origRlimitNofile`, disabling the runtime's automatic child adjustment. This is a documented behavior in Go 1.22+.

### Applying rlimits to child only (not current process)

The naive approach (set rlimit, Start(), restore rlimit) is racy with the Go runtime's goroutine scheduler. The correct approach uses a pre-exec callback. As of Go 1.22, the standard library does **not** expose a `SysProcAttr.Rlimits` field (unlike some OS-level fork/exec implementations). The correct pattern:

```go
// Option A: Use golang.org/x/sys/unix which has Rlimit in SysProcAttr on Linux
// unix.SysProcAttr has an Rlimits []unix.Rlimit field (Linux-only)

// Option B (stdlib only): set rlimits on the parent process, Start, restore
// This is racy and not recommended.

// Option C: Small wrapper executable that calls setrlimit before execing the real target
```

Given the NFR-2 (zero external deps), the project should use build-tagged files:
- `executor_linux.go`: Use `golang.org/x/sys/unix.SysProcAttr.Rlimits` (already in `go.mod` via `golang.org/x/sys v0.42.0`)
- `executor_darwin.go`: Set process-level `RLIMIT_CPU` and `RLIMIT_NOFILE` at call site with save/restore (accept the race risk, or skip rlimits on darwin)

---

## 5. Notable Go Subprocess Libraries and Patterns

### `github.com/creack/pty`

**Purpose**: Unix pseudo-terminal (PTY) creation and management. Not a general subprocess management library.

**Patterns worth learning:**
- `pty.Start(cmd)` returns `(*os.File, error)` — the PTY master fd. The command's Stdin/Stdout/Stderr are all connected to the PTY slave. This is a clean "single file handle for all I/O" pattern.
- Uses build constraints (`//go:build linux`, `//go:build darwin`) for platform-specific ioctl calls (TIOCPTYGNAME on macOS, ptsname on Linux).
- Zero external dependencies — pure stdlib + `syscall`.
- Does **not** manage process lifecycle (no Stop, no Kill, no WaitDelay) — that is explicitly left to the caller.
- Uses `pty.Open()` to get a PTY/TTY pair, then attaches to a cmd via `cmd.SysProcAttr.Setctty = true`.

**Patterns to adopt:**
- Build-tagged platform files for OS-specific syscall variants (the library should do the same for `Pdeathsig` on Linux vs no-op on darwin).
- Exposing raw `*os.File` handles for streaming rather than wrapping them — gives callers full control.

### `github.com/mattn/go-shellwords`

**Purpose**: Parses shell command strings into `[]string` argument lists (like Python's `shlex`). Not a process management library. Relevant only if the library needs to accept string-form commands.

**Patterns:** stateless parser, no subprocess management. Not relevant to the library's design.

### `mvdan.cc/sh/v3`

**Purpose**: POSIX shell interpreter and runner in pure Go. Relevant if the library needs to run shell scripts.

**Patterns worth noting:**
- Uses `runner.Run(context.Context, *syntax.File)` — always context-propagating.
- Handles process group management internally when running pipelines.
- External dependency; not usable under NFR-2 but useful as a reference for "how to propagate context through a shell pipeline."

### The `ionrock/procs` library pattern

**Purpose**: Lightweight process management wrapper. Key insight: exposes `Process.Start()`, `Process.Stop()`, and a `Done` channel — a clean lifecycle interface that maps directly to the `ManagedProcess` API target.

**Patterns worth adopting:**
- `Done chan struct{}` for async process completion notification.
- Separate `OutputHandler func([]byte)` for streaming stdout without blocking Wait.
- Not importing this library (NFR-2), but the interface design is a good model.

---

## 6. `runtime.SetFinalizer` Pitfalls and the Correct Pattern

### `runtime.SetFinalizer` pitfalls

`runtime.SetFinalizer` is officially deprecated in favor of `runtime.AddCleanup` (Go 1.24). Key pitfalls:

1. **No ordering guarantee**: Finalizers run in unspecified order. If two objects reference each other and become unreachable simultaneously, neither finalizer may run.
2. **Resurrection**: `SetFinalizer` resurrects the object (it becomes reachable again during finalization), which delays GC by at least one additional GC cycle. This can interact badly with process resources.
3. **Tiny allocation coalescence**: Small objects (< 16 bytes) may be coalesced into a single allocation block. The Go GC may not run the finalizer for the coalesced object at the expected time.
4. **Not guaranteed to run**: The GC may never call a finalizer if the process exits. This is a hard constraint for subprocess cleanup: finalizers are a safety net only, not a correctness mechanism.
5. **Single finalizer per object**: `SetFinalizer` can only attach one function per object. Adding a second call replaces the first.
6. **Object must be at the start of a heap block**: Pointers into the middle of allocations are silently ignored.
7. **Cycle prevention**: If `ManagedProcess` holds references that form a cycle back to itself, the finalizer will never run.

### `runtime.AddCleanup` (Go 1.24) — the replacement

```go
func AddCleanup[T, S any](ptr *T, cleanup func(S), arg S) Cleanup
```

`AddCleanup` resolves most of the above:
- Multiple cleanups can be attached to the same object.
- Cleanups can be attached to objects the caller doesn't own.
- Does **not** resurrect the object (no extra GC cycle).
- Cleanups may run concurrently with each other and with user goroutines.
- **Still not guaranteed to run** — same fundamental GC constraint as SetFinalizer.

**This project targets Go 1.22+.** `AddCleanup` requires Go 1.24. Since the project is on Go 1.22, `SetFinalizer` must be used, with the following constraints:

### Correct finalizer pattern for `ManagedProcess`

The finalizer is a **last-resort safety net**, not a primary cleanup mechanism. The correct pattern:

```go
type ManagedProcess struct {
    cmd     *exec.Cmd
    done    chan struct{}
    stopped int32 // atomic flag
}

func newManagedProcess(cmd *exec.Cmd) *ManagedProcess {
    mp := &ManagedProcess{cmd: cmd, done: make(chan struct{})}
    runtime.SetFinalizer(mp, func(p *ManagedProcess) {
        // Only attempt cleanup if Stop was never called
        if atomic.LoadInt32(&p.stopped) == 0 {
            // Best-effort: kill the process group; do not block
            if p.cmd.Process != nil {
                pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
                if err == nil {
                    syscall.Kill(-pgid, syscall.SIGKILL)
                } else {
                    p.cmd.Process.Kill()
                }
            }
        }
        // Do NOT call runtime.SetFinalizer(p, nil) here — that's not valid during finalization
    })
    return mp
}
```

**Rules for the finalizer:**
- Never block in a finalizer (no `cmd.Wait()`, no channel ops with backpressure).
- Set `stopped` atomically in `Stop()` so the finalizer can detect it was called.
- Use `runtime.KeepAlive(mp)` in `Stop()` and `Wait()` to prevent premature finalization while those methods are executing with `mp.cmd`.
- Accept that the finalizer may never run. The primary Stop path must work correctly without it.

**For Go 1.24+ compatibility note (future upgrade):** Migrate to `runtime.AddCleanup` when the project upgrades. `AddCleanup` signature:

```go
var cleanup runtime.Cleanup = runtime.AddCleanup(mp, func(pid int) {
    pgid, err := syscall.Getpgid(pid)
    if err == nil {
        syscall.Kill(-pgid, syscall.SIGKILL)
    }
}, mp.cmd.Process.Pid) // arg is the PID, not the *ManagedProcess (avoids cycle)
```

---

## Summary of Key Findings

- **Process group kill pattern**: Set `SysProcAttr{Setpgid: true}` before `Start()`, then `syscall.Kill(-pgid, syscall.SIGKILL)` via `Getpgid`. Works identically on macOS and Linux. `Pdeathsig` is Linux-only and not a replacement.

- **WaitDelay does not kill the group**: `WaitDelay` expiry calls `cmd.Process.Kill()` (single process). Process group kill must be explicit: use a custom `cmd.Cancel` for SIGTERM-to-group and a follow-up goroutine for SIGKILL-to-group after the grace period. The two-phase `Stop()` in `ManagedProcess` is the correct design.

- **rlimit platform reality**: `RLIMIT_CPU` and `RLIMIT_NOFILE` work on both Linux and macOS. `RLIMIT_RSS` is defined on both but effectively unenforced on modern kernels/OS. For child-only rlimits on Linux, `golang.org/x/sys/unix.SysProcAttr.Rlimits` (already in `go.mod`) is the correct zero-extra-dep approach. On macOS, use RLIMIT_CPU only via `syscall.Setrlimit` before `cmd.Start()` with save/restore (or skip).

- **Finalizer pattern**: Use `runtime.SetFinalizer` as a best-effort last-resort only (project targets Go 1.22). Never block in the finalizer. Use `runtime.KeepAlive` in `Stop()`/`Wait()`. Plan to migrate to `runtime.AddCleanup` on Go 1.24 upgrade (avoids resurrection, supports multiple cleanups, prevents cycles).
