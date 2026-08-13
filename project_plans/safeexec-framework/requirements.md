# safeexec-framework: Requirements

## Context

stapler-squad is a Go web server that manages AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions with git worktrees. It spawns a large number of subprocesses: tmux commands, git operations, shell utilities, and long-running PTY processes. A prior session discovered zombie process accumulation caused by `cmd.WaitDelay` not being set on `exec.CommandContext` calls, leading to goroutines blocked in `cmd.Wait()` indefinitely after context timeout fires SIGKILL.

### What has already been built

- `executor/safeexec`: thin wrapper around `exec.CommandContext` that pre-sets `WaitDelay = 2*time.Second`
- `tools/lint/norawexec`: custom `go/analysis` lint pass that detects direct `os/exec.Command` / `os/exec.CommandContext` calls outside exempt packages, with `//nolint:norawexec <justification>` escape hatch
- All existing violations across `session/` packages have been migrated to `safeexec.CommandContext`

### Problem

The current `safeexec` is minimal — it prevents the WaitDelay zombie footgun but leaves many other sharp edges unaddressed:

- No process group management: child spawns grandchildren; SIGKILL to child leaves grandchildren as orphans
- No structured output capture or streaming API
- No resource limits (CPU, memory, file descriptors)
- No audit logging of subprocess invocations
- Long-running processes (`cmd.Start()`) have no lifecycle management type — callers use nolint comments and manage cleanup manually
- No retry logic for transient failures
- No unified timeout/deadline propagation

---

## Stakeholders

- **Primary user**: Tyler Stapler (solo practitioner), platform engineering background, deep Go expertise
- **Consumers of the library**: all packages in `session/`, `server/`, and future packages that spawn subprocesses

---

## Goals

Build a fully-featured, safe subprocess management library for the stapler-squad codebase that makes the safe path the only easy path. The library lives at `executor/` (already exists as a package) and exposes two clearly-separated APIs:

1. **ShortLivedCmd** — one-shot commands that produce output; analogous to `safeexec.CommandContext` but with more features
2. **ManagedProcess** — long-running processes with explicit Start/Stop/Wait lifecycle

---

## Functional Requirements

### FR-1: Short-Lived Command API
- `safeexec.CommandContext(ctx, name, args...)` must continue to work as today (backwards compatible)
- Extend with a builder API: `safeexec.New(ctx, name, args...).WithTimeout(d).WithEnv(k,v).Run()` (optional — only if it improves ergonomics without ceremony)
- All short-lived commands must have `WaitDelay` set to prevent zombie accumulation
- Output capture: `.Output()`, `.CombinedOutput()`, `.Run()` all work as on `exec.Cmd`

### FR-2: Process Group Management
- Short-lived commands: set `Setpgid: true` so kill signals propagate to all grandchildren
- ManagedProcess: set `Setpgid: true` and expose `Kill()` which sends SIGKILL to the entire process group
- On context cancellation, send SIGTERM to the process group, then SIGKILL after `WaitDelay`

### FR-3: ManagedProcess Lifecycle
- `executor.StartProcess(ctx, name, args...) (*ManagedProcess, error)` — starts the process
- `ManagedProcess.Stop() error` — graceful: SIGTERM → wait up to `GracePeriod` → SIGKILL
- `ManagedProcess.Wait() error` — blocks until process exits
- `ManagedProcess.PID() int` — returns the process PID
- `ManagedProcess.IsAlive() bool` — returns whether the process is still running
- Cleanup on GC via finalizer as a last-resort safety net (not primary mechanism)

### FR-4: Output Capture & Streaming
- Short-lived: existing `.Output()` / `.CombinedOutput()` interface is sufficient
- ManagedProcess: expose `Stdout() io.Reader` and `Stderr() io.Reader` for streaming
- Optional: line-oriented `ScanOutput(func(line string))` callback API

### FR-5: Resource Limits
- Per-command `rlimit` configuration: max CPU time, max memory (RSS), max open file descriptors
- Implemented via `syscall.Setrlimit` in the `SysProcAttr` or via a pre-exec hook
- Reasonable defaults: no hard limits by default; callers opt in
- Linux only for cgroup-based limits; macOS gets rlimit subset only

### FR-6: Audit Logging
- Every subprocess invocation (both short-lived and ManagedProcess) emits a structured log entry:
  - command + args (args may be redacted for secrets)
  - working directory
  - start time, duration, exit code
  - whether it was killed by timeout
- Uses the existing `log.InfoLog` / `log.DebugLog` loggers in the project
- Audit logging is opt-in per call site via a context value or global toggle

### FR-7: Timeout & Deadline Propagation
- Context cancellation and deadline are always respected
- Explicit per-command timeout via `.WithTimeout(d)` overrides context deadline if shorter
- WaitDelay is always set; never left at zero

### FR-8: Lint Enforcement
- `norawexec` lint pass continues to enforce that all subprocess creation goes through the framework
- Add a new lint rule `nounmanagedprocess` that flags `cmd.Start()` without a `//nolint` comment explaining why ManagedProcess isn't used

---

## Non-Functional Requirements

### NFR-1: Backwards Compatibility
- `executor/safeexec.CommandContext` signature unchanged — all existing call sites continue to compile
- New APIs are additive; nothing is removed in this iteration

### NFR-2: Zero External Dependencies
- The framework must have zero new external dependencies beyond the Go standard library and the existing project imports
- `syscall` and `os` are acceptable; no third-party process management libraries

### NFR-3: Platform Support
- Primary: macOS (development) and Linux (production / CI)
- Process group management available on both
- Resource limits: full rlimit set on Linux; partial (CPU, open files) on macOS
- Windows: gracefully degrade (no process groups, no rlimits); build tags if needed

### NFR-4: Testability
- Every type in the framework is interface-backed so tests can use fakes
- The existing `MockCmdExec` pattern in `session/tmux/tmux_test.go` is the reference implementation
- Integration tests use isolated tmux servers (existing pattern) or temp directories

### NFR-5: Performance
- Audit logging must not add >1ms overhead to command invocation in the common case
- No allocations in the hot path beyond what `exec.Cmd` itself allocates

---

## Constraints

- Go 1.22+ (WaitDelay, slices, maps packages available)
- Existing `executor/` package must remain the home for all executor abstractions
- The `executor.CmdExecutor` interface (used throughout `session/tmux/`) must remain compatible
- `executor.TimeoutExecutor` already sets WaitDelay — its callers are safe; don't break them

---

## Success Criteria

1. `make lint-custom` passes with zero norawexec violations across the entire codebase (already true for `session/`)
2. Zero uses of raw `exec.CommandContext` or `exec.Command` in production code outside `executor/`
3. ManagedProcess type exists and replaces at least 2 existing `//nolint:norawexec` sites
4. Process group management is active for all short-lived commands and ManagedProcess
5. Audit logging works and emits structured log entries in integration test runs
6. All new code has unit test coverage > 80%
7. `go vet ./executor/...` and `golangci-lint run ./executor/...` pass cleanly

---

## Out of Scope

- Distributed process management or cross-host execution
- Container/sandbox isolation (seccomp, namespaces, Docker)
- Streaming stdout to the web UI (that's ExternalTmuxStreamer's job)
- Windows process group management (graceful degradation only)
- Replacing the existing `executor.TimeoutExecutor` or `executor.CircuitBreakerExecutor` — they stay
