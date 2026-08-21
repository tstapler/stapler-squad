# Stack Research: Go-Embeddable Process Supervisor Libraries

Status: Verified | Phase: 2 - Research
Created: 2026-04-29
Last verified: 2026-05-22
Input: project_plans/immortal-migration/requirements.md

---

## 1. Summary

The goal is a config-driven abstraction layer over stapler-squad's session management that lets Tmux be swapped for a process manager supporting auto-restart on crash, while compiling into a single Go binary (no CGO, no external daemons).

The existing codebase already has a clean internal boundary: `TmuxManager` is a Go interface with ~30 methods (`session/tmux_process_manager.go` lines 347–382). `TmuxProcessManager` is the concrete struct that delegates to `*tmux.TmuxSession`. The seam exists; the question is what goes behind it.

Findings in brief:

- **Immortal** (github.com/immortal/immortal) is a standalone daemon, not an embeddable library. Its Go source contains internal packages not designed for external use. Incompatible with single-binary, no-daemon constraint. [TRAINING_ONLY - verify]
- **suture** (github.com/thejerf/suture) is a pure-Go embeddable supervisor tree library (Erlang/OTP model). No PTY support. Solves goroutine supervision, not OS-process supervision.
- **go-supervisor / goproc** — no widely-adopted library with this name solves OS-process + PTY + restart. The space is fragmented across tomb, errgroup, and small retry packages. [TRAINING_ONLY - verify]
- **Custom NativeProcessManager** — the lightest and best-fit option. Uses `os/exec` + `creack/pty` (already in go.mod) + a restart-loop goroutine. Hooks into the existing `LifecycleListener`/`EventExited` seam.
- **s6/runit Go bindings** — none exist in pure Go. All require CGO or exec into a C binary. Out of scope.

**Recommendation**: Implement a custom `NativeProcessManager` backed by `os/exec` + `creack/pty` + backoff restart loop, implementing the existing `TmuxManager` interface. suture is not needed at this layer. See Section 9 for the phased implementation plan.

---

## 2. Options Surveyed

### 2.1 Immortal (github.com/immortal/immortal)

Immortal is a Unix process supervisor written in Go, inspired by daemontools/s6. It runs as a system daemon (`immortald`) and manages child processes through a control socket.

**Architecture**: External daemon model. `immortald` is the parent process; supervised processes are its children. A CLI (`immortalctl`) communicates with a per-process Unix socket. Individual process configurations are YAML files in a watched directory.

**Embeddability**: The Go source is structured as a daemon application with internal packages (`run`, `ctrl`, `scanner`). These are not designed for external import and expose no stable public API. There is no documented Go library interface. [TRAINING_ONLY - verify via code-archaeology]

**PTY support**: Immortal spawns processes via `os/exec` with standard pipe I/O. It does not allocate a PTY or expose a PTY master fd to callers. Terminal output is collected via pipes. This is a hard incompatibility with stapler-squad's architecture, which requires a PTY master fd for ANSI-correct rendering and terminal size propagation. [TRAINING_ONLY - verify]

**Restart policies**: Rich. Configurable restart delay, max restart attempts, wait-for-child-exit, on-signal restart, cgroup-based resource limits (Linux only), environment injection, pre/post hooks.

**Pure Go**: Yes — no CGO in the supervisor code itself. However, the daemon model means the supervisor process is external to the stapler-squad binary.

**License**: MIT [TRAINING_ONLY - verify]

**Maintenance**: Active as of 2024; ~3k GitHub stars. [TRAINING_ONLY - verify]

**Verdict**: Incompatible with the single-binary, no-external-daemons, PTY-streaming constraints. Cannot be imported as a library. Do not adopt.

---

### 2.2 suture (github.com/thejerf/suture)

suture implements the Erlang/OTP supervisor tree pattern in pure Go. A `Supervisor` manages `Service` implementations (goroutines); when a service returns an error or panics, the supervisor restarts it per a configurable backoff policy.

**Architecture**: Pure library. No daemon. No CGO. Compiles directly into the host binary. Services implement a simple `Serve(ctx context.Context) error` interface.

**Embeddability**: Designed for embedding. The core pattern is `supervisor.Add(service)` and `supervisor.Serve(ctx)`. Supervisor trees are hierarchical; a failing sub-tree can be isolated without killing siblings.

**PTY support**: None. suture operates at the goroutine level — it has no concept of PTY file descriptors, terminal size, or stdin/stdout of child OS processes.

**Restart policies**: Configurable via `ServiceSpec`: immediate restart, exponential backoff, jitter, maximum failure count before giving up, full-supervisor-stop on too-many-failures. Clean and battle-tested.

**Pure Go**: Yes [TRAINING_ONLY - verify]

**License**: MIT [TRAINING_ONLY - verify]

**Maintenance**: suture v4 is the current version; actively maintained as of 2024. [TRAINING_ONLY - verify]

**Verdict**: Solves goroutine supervision, not OS-process supervision. Cannot replace Tmux directly. It could be used as the restart-loop engine inside a `NativeProcessManager` — the "service" is a goroutine that calls `pty.Start(cmd)`, waits for exit, and returns a non-nil error to trigger the next restart cycle. Valid composition, but adds a dependency with limited marginal benefit over a hand-written backoff loop.

---

### 2.3 go-supervisor / goproc patterns

There is no widely-adopted Go library named "go-supervisor" or "goproc" that provides OS-process supervision with PTY support. The space is covered by:

- **tomb** (gopkg.in/tomb.v2): Goroutine lifecycle management with clean shutdown. Not a restart supervisor; no PTY.
- **errgroup** (golang.org/x/sync/errgroup): Goroutine fan-out with error collection. Same category as tomb.
- **retry/backoff packages**: Various `retry.Do(fn, ...)` packages. None specifically for PTY process restart.
- **gops** (github.com/google/gops): Runtime diagnostics agent, not a supervisor.

[TRAINING_ONLY - verify via web search: no authoritative PTY-aware Go process supervisor library exists]

**Verdict**: This category has no off-the-shelf answer. The correct solution is custom (Section 2.4).

---

### 2.4 Custom NativeProcessManager (recommended)

The stapler-squad codebase already contains all required primitives:

| Primitive | Where | Role |
|---|---|---|
| `creack/pty v1.1.24` | `go.mod` line 14; `session/tmux/pty.go` | PTY allocation via `pty.Start(cmd)` |
| `os/exec.Cmd` | stdlib | Process lifecycle |
| `LifecycleListener` + `EventExited` | `session/instance.go` lines 64–82 | Hook fires on unexpected exit |
| `TmuxManager` interface | `session/tmux_process_manager.go` lines 353–391 | The abstraction seam to implement (30 methods) |
| `executor.ShortLivedCmd` | `executor/shortlived.go` | One-shot subprocess with timeout — use for metadata queries (CWD, PID checks) |
| `executor.ManagedProcess` | `executor/managed_process.go` | Long-running pipe process — **pipe-based only, NOT PTY-capable** |
| `executor/safeexec.CommandContext` | `executor/safeexec/safeexec.go` | Sets WaitDelay=2s to prevent zombie accumulation |
| scrollback package | `session/scrollback/` | PTY output buffering already abstracted |
| session_restart_test.go | `session/session_restart_test.go` | Restart behavior tests exist |

**norawexec lint rule** (custom go/analysis pass at `tools/lint/norawexec/`): forbids bare `exec.Command`/`exec.CommandContext` outside the `executor` and `executor/safeexec` packages. The PTY launch (`pty.Start(cmd)`) must carry a `//nolint:norawexec long-running PTY process; WaitDelay not applicable` comment. All other subprocess calls in NativeProcessManager (metadata queries, cleanup) should use `executor.ShortLivedCmd` or `safeexec.CommandContext`.

A `NativeProcessManager` implementing `TmuxManager` would:

1. Call `pty.Start(cmd)` to allocate a PTY pair and start the process
2. Store the PTY master `*os.File` in the struct; keep it open for process lifetime
3. Start a goroutine that reads PTY bytes and fans out to registered subscriber channels (replacing `SubscribeToControlModeUpdates`)
4. Call `cmd.Wait()` in a goroutine; on unexpected exit, wait for backoff, then restart
5. Emit `EventExited` between cycles so the `LifecycleListener` chain can update session status

**Embeddability**: Perfect — zero new dependencies.
**PTY support**: Full — uses the same `creack/pty` the tmux package uses.
**Restart policies**: Custom backoff loop; simple and correct for this use case.
**Pure Go**: Yes.

**Verdict**: Highest fit. Most migration cost is in the control-mode pub/sub replacement, not the process management itself. See Section 5 for detailed cost breakdown.

---

### 2.5 s6/runit/daemontools Go bindings

s6, runit, and daemontools are C programs. There are no pure-Go libraries that embed their supervision logic. Known Go wrappers require CGO (e.g., wrapping libskarnet) or exec into an installed binary. Both violate the no-CGO, single-binary constraints.

**Verdict**: Out of scope.

---

### 2.6 systemd socket activation (go-systemd)

Libraries like `github.com/coreos/go-systemd` allow a Go binary to integrate with systemd: socket activation, sd_notify readiness signaling, journal logging. This is external supervisor integration, not embedded supervision.

**Verdict**: Complementary, not a replacement. Useful for production Linux deployments (notify systemd when stapler-squad is ready) but irrelevant to the in-process restart requirement.

---

### 2.7 Process-liveness polling (procfs/kqueue watchdog)

A goroutine that polls `/proc/<pid>/status` (Linux) or uses `kqueue` EVFILT_PROC (macOS) to detect process exit. The tmux package already uses a cruder version of this (DoesSessionExist polls tmux's session list). For a directly-managed child process, `cmd.Wait()` is strictly better: it blocks until exit, consumes the exit status, and prevents zombie accumulation.

**Verdict**: Not a library. Superseded by `cmd.Wait()` goroutine in the custom approach.

---

## 3. Trade-off Matrix

| Candidate | Embeddable | PTY Support | Restart Policies | Pure Go (no CGO) | License | Maintenance | Overall Fit |
|---|---|---|---|---|---|---|---|
| Immortal | No (daemon) | No (pipe only) | Rich | Yes | MIT [T] | Active [T] | Incompatible |
| suture v4 | Yes | No (goroutine level) | Rich (Erlang-style) | Yes | MIT [T] | Active [T] | Wrong layer |
| tomb / errgroup | Yes | No | None (shutdown only) | Yes | BSD/Apache | Stable | Too minimal |
| Custom NativeProcessManager | Yes (in-binary) | Yes (creack/pty) | Custom backoff | Yes | N/A | Owned | Best fit |
| s6/runit bindings | No | N/A | Rich | No (CGO) | GPL [T] | N/A | Out of scope |
| systemd go-systemd | External only | No | External (systemd) | Yes | Apache [T] | Active [T] | Complementary |

[T] = TRAINING_ONLY - verify

---

## 4. Risk and Failure Modes

### 4.1 PTY lifetime and SIGHUP

The PTY master fd must be held open for the child's lifetime. If the fd is garbage-collected or explicitly closed before `cmd.Wait()` returns, the child receives SIGHUP and exits. In a restart loop this creates a subtle livelock: process exits → fd closed → restart → fd created → process starts → fd closed prematurely → repeat.

**Mitigation**: Store `*os.File` in `NativeProcessManager`. Close it only in the `cmd.Wait()` goroutine, after `Wait()` returns.

### 4.2 Zombie process accumulation

A child that exits before its parent calls `Wait()` becomes a zombie. In a fast restart loop with a bug in the wait goroutine, zombies accumulate and eventually exhaust the PID namespace.

**Mitigation**: The `cmd.Wait()` call must be the sole owner of the process lifecycle goroutine. Ensure the goroutine cannot be cancelled before `Wait()` completes (use a separate context that is not cancelled on user-initiated stop; instead, send SIGTERM first, then wait).

### 4.3 Signal group propagation

Tmux creates a new process group for each session. `tmux kill-session` sends signals to all pane processes. Without tmux, `cmd.Process.Signal(syscall.SIGTERM)` only kills the immediate child, leaving grandchild processes orphaned as children of PID 1.

**Mitigation**: Set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` before `cmd.Start()`. This creates a new process group with the child as the group leader. Kill via `syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)` to signal the entire group. [TRAINING_ONLY - verify macOS behavior; Darwin uses `killpg` semantics which may differ]

### 4.4 Control-mode protocol replacement

Four methods in `TmuxManager` implement tmux's binary control-mode protocol (`-CC` flag):

- `StartControlMode` / `StopControlMode`
- `SubscribeToControlModeUpdates() (string, chan []byte)`
- `UnsubscribeFromControlModeUpdates(id string)`

These feed the web UI's real-time terminal stream. The replacement must implement a functionally equivalent pub/sub mechanism over raw PTY bytes. This is the highest-effort item in the migration — not because it is algorithmically complex, but because it sits on the critical path for the UI's terminal rendering.

**Mitigation**: Implement a PTY reader goroutine that reads from the master fd in a loop and fans out to a map of `chan []byte` keyed by subscriber ID. This is ~80 lines of Go. The risk is that the web UI expects tmux-format framing (`%begin` / `%end` markers, etc.) rather than raw bytes. Inspect the server-side stream handler and the React terminal component before implementing.

### 4.5 Session persistence across binary restarts

Tmux sessions survive `stapler-squad` restarts: the tmux server runs independently, and the app re-attaches on startup. A `NativeProcessManager`-owned child process dies when the stapler-squad binary exits, because:

1. The child's PTY master fd is owned by the Go process and is closed on exit.
2. The child's process group receives SIGHUP when its controlling terminal (the PTY) closes.

This is a material UX regression: users lose running AI sessions on every binary update or crash.

**Mitigation options**:
- (a) Extend `claude-mux` to be the persistence shim: claude-mux already holds PTY master fds in a separate process. NativeProcessManager sessions could be bootstrapped through claude-mux.
- (b) Accept the limitation in Phase 1 and document it clearly. Solve in Phase 3.
- (c) Use `Setsid: true` in `SysProcAttr` to disassociate the child from the terminal session. This alone doesn't preserve PTY I/O across restarts but prevents immediate SIGHUP death.

### 4.6 macOS PTY resize

`pty.Start` on macOS allocates via `posix_openpt`. Resize signals require `TIOCSWINSZ` ioctl on the master fd. The `creack/pty` library's `pty.Setsize` handles this, but `SetDetachedSize` (setting a virtual size without an attached client) has no direct equivalent — tmux maintains a virtual screen size independent of any attached terminal. Without tmux, the effective terminal size collapses to whatever the most-recently-attached client set.

**Mitigation**: Track the last-set size in `NativeProcessManager`. On each restart, re-apply the saved size immediately after `pty.Start`.

### 4.7 suture composition overhead (if adopted)

If suture is used as the restart engine, the `Service.Serve(ctx)` contract requires the goroutine to return on both context cancellation (graceful stop) and normal restart scenarios. The PTY read loop and the `cmd.Wait()` goroutine interact in a non-trivial shutdown order. This is solvable but adds state machine complexity.

**Mitigation**: Not using suture in Phase 1 eliminates this entirely.

---

## 5. Migration and Adoption Cost

### 5.1 What already exists (zero additional cost)

| Asset | Location | Relevance |
|---|---|---|
| `TmuxManager` interface | `session/tmux_process_manager.go:353–391` | Already defines the full API surface to implement (30 methods) |
| `creack/pty v1.1.24` | `go.mod:14`; `session/tmux/pty.go` | PTY allocation already used and tested |
| `LifecycleListener` + `EventExited` | `session/instance.go:64–82` | Restart notification hook already tested |
| Scrollback package | `session/scrollback/` | PTY output buffering infrastructure |
| Restart tests | `session/session_restart_test.go` | `HealthCheckerAutoRestart`, `LazyRecoveryRestart` — validate backend-agnostic restart |
| Config system | `config/` | JSON-based config; add `process_manager` key |
| `executor.ShortLivedCmd` | `executor/shortlived.go` | Use for NativeProcessManager metadata queries (CWD, PID checks) — avoids norawexec violations |
| `executor/safeexec.CommandContext` | `executor/safeexec/safeexec.go` | WaitDelay-safe wrapper — use for all non-PTY subprocesses |

### 5.2 New work by component

| Component | Effort | Notes |
|---|---|---|
| `NativeProcessManager` struct: lifecycle methods (`Start`, `Close`, `IsAlive`, `DoesSessionExist`, `SendKeys`, `TapEnter`, `SetWindowSize`, `GetPTY`, `SetOnExitCallback`, `ResetExitOnce`) | Medium (200–300 lines) | Straightforward `os/exec` + `creack/pty` wrappers |
| Backoff restart loop | Low (50–80 lines) | Simple exponential backoff; plug into `SetOnExitCallback` |
| PTY output pub/sub (replaces `StartControlMode` / `SubscribeToControlModeUpdates`) | High (80–150 lines + risk) | The critical path. Must match the byte format the web UI consumes. |
| `CapturePaneContent` / `CapturePaneContentRaw` | Medium (50 lines) | Read from scrollback buffer instead of `tmux capture-pane` |
| `HasUpdated` / `FilterBanners` / `HasMeaningfulContent` | Low-Medium (30–50 lines) | Reimplement against scrollback buffer; banner detection is tmux-specific |
| `GetPaneDimensions` / `GetCursorPosition` | Medium-High | Tmux queries these from virtual screen. Without tmux: either parse VT100 responses or maintain a framebuffer. A fixed 220x50 default may be acceptable initially. |
| `SetDetachedSize` | Low (10 lines) | Store in struct; apply via `pty.Setsize` on next attach |
| `Attach` / `DetachSafely` | Medium | Forward PTY master fd to a terminal emulator; stop forwarding on detach |
| Process group signaling | Low (10 lines) | `SysProcAttr.Setpgid` + `syscall.Kill(-pgid, ...)` |
| Config-driven backend selection | Low (20–30 lines) | Factory function in config; returns `TmuxProcessManager` or `NativeProcessManager` |
| Session persistence across restarts | High (or defer) | Extend claude-mux or accept limitation |

### 5.3 Hardest interface methods to replace

The 30-method `TmuxManager` interface breaks into three tiers by replacement difficulty:

**Tier 1 — Easy (direct `os/exec` + `creack/pty` mappings)**:
`Start`, `Close`, `IsAlive`, `DoesSessionExist`, `GetPTY`, `SendKeys`, `TapEnter`, `SetWindowSize`, `SetOnExitCallback`, `ResetExitOnce`, `RestoreWithWorkDir`

**Tier 2 — Medium (needs scrollback buffer query)**:
`CapturePaneContent`, `CapturePaneContentRaw`, `CapturePaneContentWithOptions`, `CaptureViewport`, `HasUpdated`, `SendPromptWithEnter`, `GetPanePID`, `RefreshClient`

**Tier 3 — Hard (requires either VT100 parsing or fixed fallback)**:
`GetPaneDimensions`, `GetCursorPosition`, `SetDetachedSize`, `FilterBanners`, `HasMeaningfulContent`, `Attach`, `DetachSafely`, `StartControlMode`, `StopControlMode`, `SubscribeToControlModeUpdates`, `UnsubscribeFromControlModeUpdates`

The Tier 3 methods account for roughly 35% of the interface but 70% of the migration effort. A phased approach (Phase 1: Tier 1 only, stub the rest) lets the tmux backend remain the default while the native backend is brought up incrementally.

---

## 6. Operational Concerns

### 6.1 Single-binary goal

`NativeProcessManager` has no runtime dependency on tmux. The existing embedded-tmux path (`make build-embedded`) remains available for the tmux backend but is not required for the native backend. Both backends compile from the same source.

### 6.2 Auto-restart on crash

The native backend enables true auto-restart without polling: `cmd.Wait()` returns immediately when the process exits. The restart loop can fire within milliseconds of a crash. The current tmux backend depends on a health-check poll cycle to detect dead sessions.

### 6.3 Log capture

PTY stdout/stderr is directly accessible from the master fd. The PTY reader goroutine can tee output to both subscriber channels and a `lumberjack` log file simultaneously.

### 6.4 Test isolation

The current test infrastructure uses `TmuxServerSocket` to create isolated tmux servers per test. Native backend tests need no equivalent: each test creates its own `NativeProcessManager` instance with its own PTY and process group. No socket names to coordinate, no tmux server to wait for.

### 6.5 Windows

The existing code has `tmux_windows.go` as a stub. PTY support on Windows requires Windows ConPTY (`golang.org/x/sys/windows` or a CGO bridge). `creack/pty` has limited Windows support. The native backend on Windows is a known gap; document it, do not block the macOS/Linux implementation on it.

---

## 7. Prior Art and Lessons Learned

### 7.1 creack/pty is the right PTY library for this codebase

`creack/pty` is the established pure-Go PTY library used by gotty, ttyd-go, and the original stapler-squad tmux integration. `pty.Start(cmd)` allocates a PTY pair, sets the slave as the command's controlling terminal, and returns the master fd. This is the exact primitive needed for `NativeProcessManager`. No alternatives are needed.

### 7.2 The TmuxManager interface is the correct abstraction boundary

The interface in `tmux_process_manager.go` was already designed with pluggability in mind (comment: "can be implemented by test doubles to avoid requiring a real tmux server"). The interface is complete enough to be a real `ProcessManager` abstraction without renaming. The migration is an implementation swap, not an interface design problem.

### 7.3 The embedded-tmux precedent informs the native approach

The `make build-embedded` path (compile tmux from source, embed in binary) required careful Makefile work, a git submodule, and a separate build tag. The native backend eliminates this complexity entirely. The precedent shows the team is willing to own build complexity to achieve the single-binary goal; the native approach achieves the same goal with less complexity.

### 7.4 The HealthChecker restart path is already tested

`session_restart_test.go` contains `HealthCheckerAutoRestart` and `LazyRecoveryRestart` tests. These tests validate restart behavior at the `Instance` level, independent of whether the backend is tmux or native. Any `NativeProcessManager` implementation that satisfies `TmuxManager` will pass these tests automatically, which is exactly the right validation signal.

### 7.5 claude-mux solves a related problem

The `claude-mux` PTY multiplexer (`scripts/install-mux.sh`) wraps Claude in a PTY and exposes it via a Unix socket, allowing stapler-squad to discover and connect to externally-started sessions. This is architecturally similar to what is needed for session persistence across binary restarts. Extending claude-mux to hold PTY fds for native backend sessions is a viable Phase 3 approach.

### 7.6 Signal handling on macOS requires explicit verification

macOS and Linux diverge in process group signal behavior when `Setpgid` is combined with PTY allocation. The tmux source code has multiple platform-specific workarounds for macOS signal delivery. Any implementation of `NativeProcessManager` must be tested on macOS explicitly, not just Linux.

---

## 8. Open Questions

1. **Immortal library API**: Does `github.com/immortal/immortal` expose any exported types designed for import? Code-archaeology (`findings-features.md`) will answer this definitively before the design phase begins.

2. **Control-mode wire format**: Does the web UI's terminal renderer consume raw PTY bytes or tmux-formatted control-mode output? Inspect `server/services/` streaming handlers and the React terminal component. If raw bytes are acceptable, the Tier 3 migration cost drops significantly.

3. **HealthChecker location**: Where exactly is the production `HealthChecker` struct that `session_restart_test.go` tests? Confirm hook points before designing the restart integration. (Search: `grep -r "HealthChecker\|healthCheck" session/`)

4. **Client reconnection on restart**: When the child process restarts and the PTY master fd changes, what happens to subscribers registered via `SubscribeToControlModeUpdates`? Are channels drained/closed? Must the web UI reconnect automatically?

5. **`GetPaneDimensions` / `GetCursorPosition` usage**: How heavily do the prompt detection and AI response parsing layers depend on accurate cursor position? If these can be stubbed with fixed values initially, Tier 3 migration cost drops.

6. **`SetDetachedSize` semantics**: Tmux maintains a virtual screen size when no client is attached. Without this, the terminal has no defined size while detached. Is a fixed fallback (220x50) acceptable for the native backend? Or does prompt detection require accurate sizing?

7. **macOS process group signal delivery**: What is the correct `SysProcAttr` configuration on macOS Darwin for killing a process subtree via negative PID signal? Requires platform testing.

8. **suture as internal goroutine supervisor**: Is there value in adopting suture for stapler-squad's own internal goroutines (pollers, web server, etc.) as a separate improvement, independent of this migration?

---

## 9. Recommendation

**Adopt the Custom NativeProcessManager approach (Section 2.4).**

### Rationale

All external options are ruled out by the project constraints:

| Constraint | Rules out |
|---|---|
| Single binary, no external daemons | Immortal, systemd integration, all s6/runit paths |
| No CGO | All s6/runit bindings |
| PTY master fd required for terminal streaming | Immortal (pipe-only), suture (goroutine-only) |

The custom approach is optimal because:

1. The abstraction seam (`TmuxManager` interface) already exists and is correct.
2. The PTY primitive (`creack/pty`) is already in go.mod and tested.
3. The restart hook (`LifecycleListener` + `EventExited`) already exists and is tested.
4. The restart behavior tests exist and will validate any new backend automatically.
5. The scroll-back buffering infrastructure already exists.

### Phased plan

**Phase 1 — Process lifecycle (MVP)**
Implement `NativeProcessManager` covering Tier 1 methods. Accept stubs for Tier 3. Gate behind `process_manager: "native"` config key. Existing tests pass with tmux backend (no regressions). New `TestNativeProcessManager*` tests validate Tier 1.

**Phase 2 — Terminal streaming**
Implement the PTY reader pub/sub (`SubscribeToControlModeUpdates` replacement). This unlocks real-time terminal streaming to the web UI. Resolve the wire-format question (Open Question 2) before starting.

**Phase 3 — Persistence and full parity**
Decide the session-persistence strategy (extend claude-mux or document the limitation). Implement `GetPaneDimensions`/`GetCursorPosition` (VT100 query or fixed fallback). Promote native backend to optional default in documentation.

### suture

Do not add suture to this migration. If the team later wants a formal supervisor tree for stapler-squad's internal goroutines (pollers, daemon goroutines), adopt suture at that point as an independent improvement.

---

## 10. Pending Web Searches

Run these when web search is available to validate [TRAINING_ONLY] claims:

1. `"golang embedded library PTY" site:pkg.go.dev OR site:github.com` — verify creack/pty is the dominant pure-Go PTY library; find any newer alternatives
2. `"go suture supervisor restart library" 2024 2025` — verify suture v4 is actively maintained; check for competing projects
3. `"immortal process manager go library embed binary"` — confirm immortal cannot be imported as a library; check for any forks that expose a library API
4. `"go goroutine supervisor watchdog library 2024 2025"` — check if any OS-process + PTY supervisor libraries emerged since 2023
5. `"golang single binary no daemon process manager"` — find prior art for single-binary process supervision in Go
6. `site:github.com/immortal/immortal "import" OR "go get"` — find any evidence of immortal being imported as a Go library
7. `"creack/pty" "os/exec" restart loop golang` — find reference implementations of the custom NativeProcessManager approach
8. `"github.com/thejerf/suture" "os/exec" process supervisor` — find examples of suture composed with os/exec for OS-process supervision
9. `golang PTY "Setpgid" macOS "process group" signal SIGTERM` — verify process group kill behavior on macOS Darwin
10. `immortal process manager golang embed 2025` — catch any post-training-cutoff developments
