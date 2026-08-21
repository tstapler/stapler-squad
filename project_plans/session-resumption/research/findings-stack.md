# Stack Research: Session Resumption

_Researched: 2026-04-03_

---

## File Descriptor / History File Tracking

### The Constraint

macOS provides no unprivileged real-time per-process file-open event stream. There is no `/proc` filesystem. Options range from cooperative wrappers to polling:

| Approach | Mechanism | Privilege Required | Latency |
|---|---|---|---|
| `gopsutil OpenFiles()` | `proc_pidinfo` (libproc) via CGo | None (same user) | Polling ~2-3s |
| `lsof -p PID` | Shell out to lsof | None (same user) | ~50-100ms per call |
| FSEvents + lsof correlation | Watch directory, then query | None | 1-3s FSEvents delay + lsof |
| dtrace | Kernel-level syscall tracing | Root | Real-time |
| Endpoint Security Framework | Apple kernel extension | Apple entitlement | Real-time |

**dtrace and Endpoint Security are NOT viable** for a standard userspace application.

### Recommended: `gopsutil v3 Process.OpenFiles()`

`github.com/shirou/gopsutil/v3/process` wraps Apple's `libproc` (`proc_pidinfo` with `PROC_PIDLISTFDS` + `PROC_PIDFDVNODEPATHINFO`) via CGo. Returns full paths for all open vnodes for any same-user process without special privileges.

```go
import "github.com/shirou/gopsutil/v3/process"

p, _ := process.NewProcess(int32(claudePID))
files, _ := p.OpenFiles()
for _, f := range files {
    if strings.HasPrefix(f.Path, claudeProjectsDir) && strings.HasSuffix(f.Path, ".jsonl") {
        // This process has a Claude history file open
        linkSessionToHistory(p.Pid, f.Path)
    }
}
```

Also provides:
- `Process.Terminal()` → `/dev/ttys004` — the controlling terminal device
- `Process.Cwd()` → working directory
- `Process.Environ()` → environment variables (via `sysctl KERN_PROCARGS2`)

Works for same-user processes. Requires CGo (acceptable — gopsutil is a widely-used dependency).

**Source**: https://pkg.go.dev/github.com/shirou/gopsutil/v3/process

### FSEvents + lsof Correlation (Complementary)

Watch `~/.claude/projects/` with `fsnotify` (backed by FSEvents on macOS) for new `.jsonl` file creation. On `CREATE` event, immediately call `gopsutil OpenFiles()` scan to find the owning PID.

```go
watcher, _ := fsnotify.NewWatcher()
watcher.Add(claudeProjectsDir, fsnotify.WithRecursive)

for event := range watcher.Events {
    if event.Op&fsnotify.Create != 0 && strings.HasSuffix(event.Name, ".jsonl") {
        go correlateJSONLToPID(event.Name)  // gopsutil scan
    }
}
```

This gives fast detection (push-based for new files) + reliable PID resolution (pull-based).

---

## Checkpoint / Restore on macOS (Userspace)

### Full Process Checkpoint: Not Viable on macOS

| Tool | Status on macOS |
|---|---|
| **CRIU** | Linux-only. Requires `PTRACE_SYSCALL`, `PTRACE_GETREGS`, `/proc`, netlink. None exist on macOS. |
| **DMTCP** | Experimental macOS support. Relies on `LD_PRELOAD` (restricted by SIP). Not production-ready. |
| **Mach `task_for_pid` + `vm_read/write`** | Requires `com.apple.security.cs.debugger` entitlement (Apple-granted). SIP blocks on system processes. |

**Conclusion**: Full process state checkpoint is not achievable in standard macOS userspace.

### Practical Alternative: Structured State Serialization (tmux model)

tmux demonstrates the correct approach: checkpoint is NOT a full process snapshot — it's structured state serialization:

1. **Scrollback buffer**: already in `FileScrollbackStorage` (JSONL). A checkpoint is `(line_index, timestamp)` into this file.
2. **Working directory**: `gopsutil Process.Cwd()` captured before shutdown.
3. **Git branch**: read from git APIs.
4. **Linked JSONL history file**: detected via `OpenFiles()` scan.
5. **Terminal device**: `gopsutil Process.Terminal()`.

On restart: re-render saved scrollback in new tmux pane + relaunch `claude --resume <uuid>` in saved cwd. Conversation context recovered from JSONL history. Terminal scrollback history visible but not interactive-resumable.

---

## Process Re-parenting / TTY Adoption

### Hard Constraint on macOS

**reptyr** (Linux tool for re-parenting a running process to a new terminal) explicitly **does not work on macOS**:
- Uses `PTRACE_GETREGS`, `PTRACE_SETREGS`, `PTRACE_POKEDATA`, `PTRACE_SYSCALL` — none exist on macOS
- `PT_ATTACH` requires being the parent process or holding `com.apple.security.cs.debugger` entitlement

**Source**: https://github.com/nelhage/reptyr — "macOS: NOT supported"

### Viable Pattern: Cooperative Wrapper (claude-mux — Already Implemented)

The `claude-mux` binary in `cmd/claude-mux/main.go` is the correct and only viable approach on macOS:
1. User installs wrapper via shell alias or PATH override
2. When user runs `claude`, the wrapper intercepts, creates PTY master/slave pair, forks `claude` as child with PTY slave as controlling terminal
3. Wrapper holds PTY master, exposes via Unix socket at `/tmp/claude-mux-<PID>.sock`
4. stapler-squad connects as additional socket client — no disruption to original terminal

For already-running sessions without the wrapper: **read-only adoption only** — display metadata from gopsutil but no PTY control.

### Read-Only Process Discovery

```go
procs, _ := process.Processes()
for _, p := range procs {
    exe, _ := p.Exe()
    if !isClaudeProcess(exe) { continue }

    tty, _ := p.Terminal()      // "/dev/ttys004"
    cwd, _ := p.Cwd()
    files, _ := p.OpenFiles()   // find JSONL history file
    env, _ := p.Environ()       // check for relevant env vars

    // Can build read-only session record even without PTY control
    registerExternalSession(p.Pid, tty, cwd, findHistoryFile(files))
}
```

---

## Go-Specific Implementation Stack

### Process Discovery + Monitoring

```go
// Primary: gopsutil for per-process metadata
"github.com/shirou/gopsutil/v3/process"

// File watching: fsnotify with recursive flag
"github.com/fsnotify/fsnotify"

// Process lifecycle: kqueue EVFILT_PROC (fire on exit/fork/exec)
"golang.org/x/sys/unix"
```

### kqueue Process Lifecycle Watcher

```go
kq, _ := unix.Kqueue()
ev := unix.Kevent_t{
    Ident:  uint64(pid),
    Filter: unix.EVFILT_PROC,
    Flags:  unix.EV_ADD | unix.EV_ONESHOT,
    Fflags: unix.NOTE_EXIT,
}
unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil)
// Blocks until process exits → trigger scrollback capture + session cleanup
```

Use to know immediately when a Claude process exits, triggering final scrollback flush and session state persistence.

### Polling Architecture (Recommended Approach)

```go
func (m *SessionMonitor) pollLoop(ctx context.Context) {
    ticker := time.NewTicker(3 * time.Second)
    for {
        select {
        case <-ticker.C:
            m.scanProcesses()
        case event := <-m.fswatcher.Events:
            if isClaideHistoryFile(event.Name) {
                go m.correlateFileToProcess(event.Name)
            }
        case <-ctx.Done():
            return
        }
    }
}
```

---

## Recommendations

### 1. Use `gopsutil` polling + `fsnotify` for JSONL correlation (Best for macOS)

Poll at 2-3 second intervals for process detection. Use fsnotify for faster new-file detection. Both approaches are userspace, no entitlements, no new language runtimes.

**Tradeoffs**: CGo required; 2-3s detection latency (acceptable for session discovery).

### 2. Keep and enhance `claude-mux` as the PTY bridge

The only viable full-control adoption path on macOS. Enhancement: add a persistent socket registry (`~/.claude-squad/mux-registry.json`) so stapler-squad can reconnect after restart without re-running filesystem discovery.

### 3. Structured state serialization (not process checkpoint) for resume

Before shutdown: serialize cwd (gopsutil), tmux layout (`tmux display-message`), linked JSONL path, git branch. On restart: relaunch `claude --resume <uuid>` + restore scrollback display. No CRIU or full process state needed.

---

## Sources

| URL | Topic |
|-----|-------|
| https://pkg.go.dev/github.com/shirou/gopsutil/v3/process | gopsutil API reference |
| https://github.com/shirou/gopsutil/blob/master/process/process_darwin.go | gopsutil macOS impl (proc_pidinfo) |
| https://stackoverflow.com/questions/18965743/golang-getting-all-open-file-handles-of-a-running-process | lsof / proc_pidinfo in Go |
| https://github.com/nelhage/reptyr | reptyr Linux-only; macOS non-support confirmed |
| https://stackoverflow.com/questions/46975068/how-to-track-file-creation-of-a-process-on-macOS | FSEvents/dtrace tracking comparison |
| https://github.com/fsnotify/fsnotify | fsnotify Go library (FSEvents on macOS) |
| https://pkg.go.dev/golang.org/x/sys/unix | Go kqueue/kevent bindings (EVFILT_PROC) |
| https://github.com/checkpoint-restore/criu/issues/1036 | CRIU macOS non-support |
| https://dmtcp.sourceforge.net/ | DMTCP experimental macOS |
| https://stackoverflow.com/questions/53055100/macos-ptrace-pt-attach-fails-with-permission-denied | macOS ptrace restrictions |
