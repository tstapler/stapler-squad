# Findings: Architecture (Tmux Coupling Audit)

Status: Verified | Last verified: 2026-05-22

## Summary

The stapler-squad codebase has **pervasive but well-isolated Tmux coupling** concentrated in the `session/tmux/` package. Tmux operations span session lifecycle (creation, attachment, resizing), terminal I/O streaming (PTY management + control mode), and configuration (server socket isolation, session naming). The coupling is primarily **unidirectional**: tmux is a concrete implementation detail for `TmuxSession`, which exposes a public interface consumed by `Instance` and server services. The `TmuxManager` interface (30 methods, `session/tmux_process_manager.go:353`) exists and is satisfied by `*TmuxProcessManager`, but **`instance.go` holds the field as the concrete `TmuxProcessManager` type** — direct tmux commands are not swappable without changing the field type to the interface.

Key insight from the stack research: `TmuxManager` is already defined as a 30-method Go interface in `session/tmux_process_manager.go` lines 353–391. The abstraction seam already exists — this is an implementation swap, not an interface design problem.

**CRITICAL (verified 2026-05-22):** `session/instance.go:248` declares `tmuxManager TmuxProcessManager` as a **concrete struct** (not the interface). This is the primary migration target: changing this field from `TmuxProcessManager` to `TmuxManager` (the interface). There are ~85 call sites of `i.tmuxManager.X()` across the session package that must be updated when this field changes type.

**Executor framework (merged main 2026-05-14):** The `executor/` package provides safe subprocess management that NativeProcessManager must use:
- `executor.ShortLivedCmd` — one-shot subprocess with timeout; use for metadata queries (CWD, PID checks)
- `executor.ManagedProcess` — long-running pipe-based subprocess; **pipe-based only, NOT PTY-capable**
- `executor/safeexec.CommandContext` — WaitDelay=2s wrapper; used already in `session/tmux/tmux.go`
- `norawexec` lint rule — custom go/analysis pass at `tools/lint/norawexec/`; forbids bare `exec.Command`/`exec.CommandContext` outside `executor` packages; PTY launch needs `//nolint:norawexec` with justification

## Tmux Coupling Map

### Core Session Lifecycle
- `session/tmux/tmux.go:563–672` — `Start()`: new-session -d, -e (env vars), -c (workdir), polling for creation
- `session/tmux/tmux.go:679–785` — `RestoreWithWorkDir()`: Retry loop for attach-session PTY creation; new-session fallback
- `session/tmux/tmux.go:1214–1258` — `Close()`: kill-session, PTY cleanup
- `session/tmux/tmux.go:1049–1100` — `DetachSafely()`: PTY closure + attach-session process reaping

### Server Lifecycle
- `session/tmux/tmux.go:264–278` — `EnsureServerRunning()`: start-server subprocess
- `session/tmux/tmux.go:312–333` — `CreateKeepaliveSession()`: new-session -d (idle shell to prevent server exit)
- `session/tmux/tmux.go:295–310` — `SetExitEmpty()`: set-option -g exit-empty

### PTY & Streaming
- `session/tmux/tmux.go:515–536` — `AttachToExisting()`: PTY via attach-session
- `session/tmux/tmux.go:521–529` — `buildAttachCommand()`: attach-session command construction
- `session/tmux/tmux.go:1182–1212` — `closePTYAndAttachCmd()`: Process.Kill() for orphaned tmux attach
- `session/tmux/control_mode.go:54–106` — `StartControlMode()`: tmux -C attach-session (control mode streaming)
- `session/tmux/control_mode.go:169–238` — `readControlModeOutput()`: %output / %begin / %end parsing

### Terminal Operations
- `session/tmux/tmux.go:1603–1626` — `CapturePaneContent()`: capture-pane -p -e -J
- `session/tmux/tmux.go:1631–1653` — `CapturePaneContentRaw()`: capture-pane -p -e
- `session/tmux/tmux.go:1655–1677` — `CapturePaneContentWithOptions()`: capture-pane with -S/-E range
- `session/tmux/tmux.go:1702–1734` — `GetCursorPosition()`: display-message #{cursor_x} #{cursor_y}
- `session/tmux/tmux.go:1742–1775` — `GetPaneDimensions()`: display-message #{pane_width} #{pane_height}
- `session/tmux/tmux.go:1887–1910` — `GetPaneCurrentPath()`: display-message #{pane_current_path}
- `session/tmux/tmux.go:1914–1947` — `GetPanePID()`: display-message #{pane_pid}
- `session/tmux/tmux.go:1552–1587` — `RefreshClient()`: refresh-client (or SIGWINCH fallback)

### Windowing & Resizing
- `session/tmux/tmux.go:1260–1264` — `SetDetachedSize()`: pty.Setsize() + resize-window
- `session/tmux/tmux.go:1299–1361` — `SetWindowSize()`: updateWindowSize() + resize-window command

### Session Existence & Recovery
- `session/tmux/tmux.go:216–243` — `ListAllSessions()`: list-sessions -F #{session_name}
- `session/tmux/tmux.go:1405–1426` — `listSessionsRaw()`: list-sessions with circuit-breaker fallback
- `session/tmux/tmux.go:1428–1505` — `DoesSessionExist()`: cached list-sessions check (5s TTL)
- `session/tmux/tmux.go:1517–1547` — `DoesSessionExistNoCache()`: uncached list-sessions
- `session/tmux/tmux.go:1370–1403` — `recoverFromServerFailure()`: EnsureServerRunning + ResetAll + keepalive

## Session Lifecycle Flow

```
Creation (Start() → new-session -d):
  1. NewTmuxSession() constructs TmuxSession with CircuitBreakerExecutor
  2. Start(workDir):
     - Check DoesSessionExist() (cached, list-sessions)
     - Run: tmux new-session -d -s <name> -e CLAUDECODE= -c <workdir> <program>
     - Poll for existence with exponential backoff (5ms→50ms, timeout: 10s)
     - Run: set-option -t <name> history-limit 10000

Attachment (Restore() → attach-session):
  1. Check DoesSessionExist() with retry loop (5 attempts, 100ms→800ms backoff)
  2. If not found: run new-session (cold start fallback)
  3. Retry PTY attach up to 3 times: pty.Start(buildAttachCommand())
  4. buildAttachCommand() → tmux attach-session -t <name>

Streaming (Control Mode):
  1. StartControlMode(): Run tmux -C attach-session -t <name>
  2. Pipe stdout for %output / %begin / %end notifications
  3. readControlModeOutput() processes notifications in goroutine
  4. broadcastControlModeUpdates() sends to WebSocket subscribers

Resizing:
  1. SetWindowSize(cols, rows):
     - pty.Setsize(ptmx, {Rows, Cols})
     - Run: tmux resize-window -t <name> -x <cols> -y <rows>
     - GetPaneDimensions() → display-message #{pane_width} #{pane_height}

Detachment (Detach()):
  1. closePTYAndAttachCmd(): Close PTY + Kill attach-session process
  2. Cancel context to terminate I/O goroutines

Shutdown (Close()):
  1. closePTYAndAttachCmd()
  2. Check DoesSessionExist(); if true: run tmux kill-session -t <name>
  3. Unregister circuit breaker from executor.GetGlobalRegistry()
```

## PTY/Streaming Architecture

**PTY Ownership Model:**
- Owner: `TmuxSession` holds `ptmx *os.File` (PTY master) and `attachCmd *exec.Cmd` (tmux attach-session process)
- Lifecycle: Allocated in `Restore()` via `ptyFactory.Start()`; freed in `DetachSafely()` via `closePTYAndAttachCmd()`
- Critical invariant: PTY FD must be closed AND the attach-session process must be Kill()-ed; closing just the FD orphans the process
- Resizing: `pty.Setsize()` + tmux resize-window command (dual mechanism)

**Two-Channel Streaming Model:**

1. PTY Direct Streaming (Attach()):
   - `session/tmux/tmux.go:967–1046`
   - Goroutine 1: `io.Copy(os.Stdout, t.ptmx)` — streams PTY output to terminal
   - Goroutine 2: Reads stdin; forwards to ptmx (or detaches on Ctrl+Q)
   - Use case: Interactive TUI mode

2. Control Mode Streaming (StartControlMode()):
   - `session/tmux/control_mode.go:42–532`
   - Process: `tmux -C attach-session -t <name>` with bidirectional pipes
   - Output: Reads %output / %begin / %end / %exit notifications
   - Input: Sends tmux commands via stdin (sendCMCommand path)
   - Broadcast: `broadcastControlModeUpdates()` sends bytes to all subscribed WebSocket clients
   - Use case: Web terminal (hybrid streaming: state sync + event-driven updates)

**Terminal Output Capture (subprocess fallback):**
- `CapturePaneContent()`: capture-pane -p -e -J (join wrapped lines)
- `CapturePaneContentRaw()`: capture-pane -p -e (preserve line wrapping)
- `CapturePaneContentWithOptions()`: capture-pane -S <start> -E <end> (scrollback range)

**Banner Filtering (`session/tmux/banner_filter.go`):**
- Removes tmux status line from terminal output (updates every 1s, causes false update detection)
- Used by `HasUpdated()` and `FilterBanners()`

## Config Surface (Tmux-related Config Keys)

| Config Key | Type | Purpose |
|---|---|---|
| `tmux_session_prefix` | string | Custom prefix for session names (default: "staplersquad_") |
| `terminal_streaming_mode` | string | Streaming backend: "raw" (PTY), "state" (MOSH), "hybrid" |
| `session_defaults.env_vars` | map[string]string | ENV vars passed to new sessions |
| `session_defaults.cli_flags` | string | Extra CLI flags for the program |

**Instance fields:** `TmuxPrefix`, `TmuxServerSocket` (socket name for -L flag isolation)

## Proposed ProcessManager Interface

```go
// ProcessManager defines the minimal interface for managing terminal processes.
// Implementations: TmuxProcessManager (current), NativeProcessManager (target).
type ProcessManager interface {
    // Lifecycle
    Start(ctx context.Context, config ProcessConfig) error
    Restore(ctx context.Context) error
    Close(ctx context.Context) error

    // Terminal I/O
    GetPTY() (*os.File, error)
    SendKeys(keys string) (int, error)
    CaptureTerminalOutput(ctx context.Context) (string, error)
    CaptureTerminalOutputWithRange(ctx context.Context, startLine, endLine string) (string, error)

    // Terminal State
    GetCursorPosition(ctx context.Context) (x, y int, err error)
    GetTerminalDimensions(ctx context.Context) (width, height int, err error)
    SetTerminalDimensions(ctx context.Context, cols, rows int) error

    // Process State
    GetCurrentWorkingDirectory(ctx context.Context) (string, error)
    GetProcessID(ctx context.Context) (int32, error)
    IsRunning(ctx context.Context) (bool, error)

    // Streaming
    StartStreaming(ctx context.Context) (StreamingHandle, error)
    StopStreaming(ctx context.Context, handle StreamingHandle) error

    // Exit Notifications
    SetOnExitCallback(fn func(reason string))
}

type ProcessConfig struct {
    Name          string
    Program       string
    WorkDir       string
    Environment   map[string]string
    HistoryFile   string
    HistoryLimit  int
    PTYDimensions *TerminalSize
    Isolated      bool
    IsolationID   string
}
```

## Trade-off Matrix

| Dimension | Coupled (Current) | ProcessManager Abstraction |
|---|---|---|
| Interface size | N/A | 10–12 methods (small enough to implement) |
| Backward compat | 100% | 95% (Instance code changes; public API stable) |
| Test isolation | Moderate (real tmux required) | High (mock ProcessManager) |
| PTY ownership | TmuxSession | ProcessManager (clearer responsibility) |
| Streaming model | Explicit channels | Callbacks (simpler subscriber lifecycle) |
| Server lifecycle mgmt | Per-TmuxSession | Centralized outside manager |
| Config coupling | High (TmuxPrefix, TmuxServerSocket) | ProcessConfig struct |
| Adoption cost | 0 | ~500–1000 LOC |

## Risk and Failure Modes

- **Tmux server crashes**: Detected via `serverNotRunning()`; `recoverFromServerFailure()` handles restart. Risk: If server cannot restart, sessions hang until manual intervention.
- **PTY resource leaks**: Orphaned tmux attach-session processes if `closePTYAndAttachCmd()` is skipped. Detection: `ps aux | grep attach`.
- **Control mode pipe closure**: Fires false-positive session-exit if not guarded with `intentionalStop.Store(true)` before stopping.
- **Session creation poll timeout**: 10s limit; session may actually exist but list-sessions was slow.
- **Concurrent PTY access**: `SendKeys()` and Attach() I/O goroutine can race; relies on OS-level PTY write atomicity.
- **Circuit breaker open on existence check**: Falls back to direct exec, bypassing breaker protection.

## Migration and Adoption Cost

- **Phase 1 (Week 1)**: Extract ProcessManager interface + ProcessConfig; implement TmuxProcessManager adapter. ~300 LOC, zero behavior change.
- **Phase 2 (Week 2)**: Wire ProcessManager into Instance; replace direct tmux calls. ~200 LOC.
- **Phase 3 (Week 3, optional)**: Implement NativeProcessManager (Go PTY + custom supervisor). ~400 LOC.
- **Testing cost**: Mock ProcessManager ~100 LOC; eliminates real-tmux dependency from unit tests.

**Hardest migration items (~70% of effort):**
1. `SubscribeToControlModeUpdates` / `StartControlMode` — tmux binary control-mode protocol feeds web UI real-time stream. Replacement requires a PTY reader goroutine fanning raw bytes to subscriber channels. Critical open question: does the web UI expect tmux-format framing (%begin/%end) or raw PTY bytes?
2. `GetPaneDimensions` / `GetCursorPosition` — tmux queries these from its virtual screen. Without tmux, requires VT100 ANSI response parsing or a fixed fallback.
3. Session persistence across binary restarts — child processes die with stapler-squad when the PTY master fd closes. The claude-mux shim is a candidate solution.

## Operational Concerns

**Monitoring signals:**
1. Circuit breaker trips → tmux server likely down
2. Control mode pipe EOF without %exit → unexpected crash
3. "too many open files" → PTY leak (orphaned processes)
4. list-sessions latency >1s → tmux server under load
5. Restore() retries exhausted → session creation stuck

**Debugging:** `tmux list-sessions`, `ps aux | grep 'tmux attach'`, `~/.stapler_squad_history`

## Open Questions

- [x] ~~Does the web UI expect tmux-format framing (%begin/%end) in the control-mode stream, or raw PTY bytes?~~ **Resolved: raw bytes.** (See synthesis.md)
- [ ] Should ProcessManager own server lifecycle, or remain process-scoped? — blocks: interface finalization
- [ ] Can session persistence across stapler-squad restarts be solved without claude-mux? — blocks: Phase 3 scope
- [ ] Is the existing `TmuxManager` interface in `tmux_process_manager.go` already the right shape, or does it need to be redesigned for the NativeProcessManager case? — blocks: implementation approach
- [ ] `GetTmuxSessionName()` is the current interface method name. Requirements doc suggests `GetSessionIdentifier()` as the backend-agnostic replacement. Decision needed: rename interface method or add new method alongside? — blocks: Phase 1 interface design

## Recommendation

Implement the ProcessManager abstraction in three phases using the adapter pattern. The `TmuxManager` interface that already exists in `session/tmux_process_manager.go` provides a template; a new top-level `ProcessManager` interface should be thinner (hiding tmux server management) and placed in `session/` rather than `session/tmux/`. Phase 1 is zero-risk (no behavior change), Phase 2 is moderate risk (Instance wiring), Phase 3 is where the actual Tmux replacement happens. Control mode streaming is the hardest part and should be spiked before committing to Phase 3.
