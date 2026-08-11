# Stack Research: memory-pressure-ux

## gopsutil — Already a Dependency

`github.com/shirou/gopsutil/v3 v3.24.5` is already in `go.mod`. The project uses it in
`session/procinfo/` for open-file and CWD inspection. **No new dependency is needed.**

### Relevant gopsutil packages for this feature

| Package | Import | Usage |
|---|---|---|
| `mem` | `github.com/shirou/gopsutil/v3/mem` | `mem.VirtualMemory()` → MemTotal, MemAvailable, Used, UsedPercent |
| `process` | `github.com/shirou/gopsutil/v3/process` | `proc.MemoryInfo()` → RSS, VMS, Swap |

`mem.VirtualMemory()` already handles both Linux (`/proc/meminfo`) and macOS (`sysctl` /
`host_statistics64`) internally — **no build-tag branching is required for system memory**.
On Linux it returns `MemAvailable` accurately; on macOS it uses a heuristic.

`proc.MemoryInfo()` wraps `/proc/<pid>/status` (Linux) and `task_info` (macOS). Returns
`*process.MemoryInfoStat` with `RSS` (uint64, bytes), `VMS`, `Swap`.

### Reading /proc/meminfo directly (no gopsutil)

If pure stdlib is preferred (faster, no dependency risk):

```go
// Linux-only: //go:build linux
data, _ := os.ReadFile("/proc/meminfo")
// Parse lines: "MemTotal: 16384000 kB", "MemAvailable: 8000000 kB"
// usedPct = (MemTotal - MemAvailable) / MemTotal * 100
```

Direct file read is ~2-3x faster than gopsutil for `meminfo` since gopsutil parses
the full file. For per-process RSS, gopsutil wraps `/proc/<pid>/status` reading the
`VmRSS` line.

### Build tag pattern: Linux vs macOS

The project's `procinfo/` package already uses this pattern:

```go
// inspector.go:       //go:build darwin
// inspector_other.go: //go:build !darwin
```

For memory reading, the same pattern applies:
- `session/memory/reader_linux.go` — reads `/proc/meminfo` + `/proc/<pid>/status`
- `session/memory/reader_other.go` — calls `gopsutil/mem` (works cross-platform)

However, since gopsutil is already available and handles both platforms, a single
non-tagged file using gopsutil is simpler and fully acceptable.

### tmux list-panes for PID resolution

`tmux list-panes -t <session> -F '#{pane_pid}'` returns the **shell PID** (the process
directly spawned by tmux), not the AI process. The claude/aider process is a child of
the shell.

To get child PIDs:
1. **Linux**: read `/proc/<shell_pid>/task/<tid>/children` or `/proc/<shell_pid>/status`
   then follow the process tree. Alternatively: `pgrep -P <shell_pid>` via exec.
2. **gopsutil**: `process.Children()` method traverses the process tree.
3. **Practical shortcut**: Sum RSS of all descendants using `proc.Children()` recursively,
   or use `proc.MemoryInfoEx()` which includes PSS on Linux.

The project's `session/procinfo/inspector.go` does NOT currently resolve child PIDs —
this is new logic needed for per-session RSS.

### tmux session name for list-panes

The `Instance` struct stores `TmuxPrefix` (string). The tmux session name is constructed
as `<TmuxPrefix><Title>`. The `tmuxManager` field on Instance manages this, and
`KillSession()` knows the exact name. For memory reading, the tmux session name can be
derived via the `tmuxManager` interface or by reconstructing it from `TmuxPrefix + Title`.

Confirmed: `tmux list-panes -t <session_name> -F '#{pane_pid}'` can be called with
`os/exec` or via the existing `safeexec` package the project uses elsewhere.

### os/exec vs direct file reads

For `/proc/meminfo`: direct `os.ReadFile("/proc/meminfo")` — no subprocess, ~50 µs.
For per-process RSS: gopsutil `proc.MemoryInfo()` (reads `/proc/<pid>/status`) — no
subprocess, ~100 µs per PID.
For tmux pane PID: `os/exec` via `safeexec.CommandContext` (same as rest of codebase),
with a short timeout (1-2s).

Avoid `exec` for `/proc` reads — file reads are significantly faster and don't fork.

### Existing procinfo package

`session/procinfo/` already has `ProcessInspector` using gopsutil. The memory reader
should be a **sibling file** in `session/procinfo/` or a new `session/memory/` package.
Given the project already has `procinfo/inspector.go` and `inspector_other.go`, a new
`memreader.go` (cross-platform via gopsutil) fits cleanly there.

### Versions and API notes

- gopsutil v3.24.5: `mem.VirtualMemory()` returns `*VirtualMemoryStat`; access
  `UsedPercent` (float64) directly — no manual calculation needed.
- `process.NewProcess(pid int32)` returns error if PID doesn't exist (race-safe).
- `proc.MemoryInfo()` returns `*MemoryInfoStat{RSS, VMS, Swap uint64}` — RSS is in bytes.

## Summary

- **gopsutil v3.24.5 is already a dependency** and provides `mem.VirtualMemory()` and
  `proc.MemoryInfo()` for both Linux and macOS without build tags.
- **tmux list-panes returns shell PID**, not the AI process; use gopsutil `Children()`
  to traverse the process tree for accurate RSS aggregation.
- **New code belongs in `session/procinfo/`** as a sibling to existing `inspector.go`,
  using direct `/proc` reads (Linux) via gopsutil, fitting the existing build-tag pattern.
