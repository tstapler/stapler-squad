# Requirements: Immortal Migration (Process Manager Modularization)

Status: Active | Phase: 1 - Ideation complete
Created: 2026-04-29
Updated: 2026-05-22

## Problem Statement

Stapler Squad's hard dependency on Tmux creates two compounding problems:

1. **Deployment complexity**: Tmux must be installed separately on the host; the project goal of shipping a single self-contained binary is undermined.
2. **Feature limitations**: Tmux has no native process supervision — if a session crashes, it stays dead. Users have to manually restart it. A real process manager would detect the crash and restart the process automatically.

Together these push toward replacing Tmux with a process manager that can be embedded in the binary and provides supervision semantics.

## Success Criteria

A clean abstraction layer exists between stapler-squad's session management logic and the underlying process manager, such that:
- The process manager backend can be changed in `config.json` without modifying session logic
- A native Go backend is implemented and **demonstrates actual crash-restart behavior** (not just exposes the interface hook)
- The Tmux backend continues to work as the default — no regressions
- The interface is documented well enough that a third backend could be added without touching core session logic

## Scope

### Phase 1: Interface + TmuxBackend adapter (zero behavior change)
- ProcessManager interface in `session/process_manager.go`
- TmuxBackend adapter wrapping TmuxProcessManager
- OpenFeature SDK wired for backend selection (seeded from config.json)
- Backend factory routing "tmux" → TmuxBackend
- `session/instance.go` field changed from concrete `TmuxProcessManager` to `ProcessManager`
- All existing tests pass

### Phase 2: NativeProcessManager skeleton (PTY + restart)
- NativeProcessManager using `creack/pty` for PTY launch (not executor.ManagedProcess — pipe-based only)
- `executor.ShortLivedCmd` for metadata queries (tmux list-panes equivalent)
- Restart goroutine: monitors process exit, relaunches with backoff
- `//nolint:norawexec` with justification comment for the PTY exec call
- Config flag `process_manager_backend: "native"` routes to NativeProcessManager
- Crash + restart is demonstrable via test or manual verification

### Must Have (MoSCoW)
- Session lifecycle abstraction: create, stop, pause, resume sessions through the interface
- Terminal streaming abstraction: PTY/output streaming to the web UI regardless of backend
- Process supervision & restart capability: native backend must actually restart crashed processes
- Config-driven backend selection: `config.json` selects the backend at startup

### Out of Scope
- **Removing Tmux support**: Tmux stays as the default; this work adds pluggability, not a forced migration
- **Live session migration between backends**: switching backend mid-run is not required
- **Remote/cluster process managers**: Kubernetes, Nomad, and other distributed schedulers are out of scope
- **Full production readiness of NativeProcessManager**: skeleton + restart loop is the goal; edge cases (resize events, scrollback, etc.) are follow-on

## Constraints

- **Language**: Pure Go only — no cgo, no separate language runtimes, no external daemons
- **Binary model**: Anything embedded must compile into the stapler-squad binary (single binary deployment goal)
- **Backwards compatibility**: Existing `config.json` files must continue to work without forced migration
- **norawexec lint rule**: Bare `exec.Command` is forbidden; PTY launch in NativeProcessManager requires `//nolint:norawexec` with justification
- **executor.ManagedProcess is pipe-based**: Cannot be used for PTY sessions; `creack/pty` must be used directly

## Context

### Existing Work (post main-merge 2026-05-22)
- Research complete: stack, features, architecture, pitfalls files in `project_plans/immortal-migration/research/`
- Initial plan.md and validation.md exist but need refresh to reflect this updated scope
- `executor/` package (safeexec, ManagedProcess, ShortLivedCmd) now in main — available for use
- `creack/pty` already in go.mod
- `session/tmux_process_manager.go` has existing TmuxManager interface (lines 347–382) that leaks tmux types — must be refactored
- `session/instance.go:248` field is `tmuxManager TmuxProcessManager` (concrete) — must become `processManager ProcessManager`
- ~85 call sites of `i.tmuxManager.X()` must be renamed to `i.processManager.X()`
- GetPaneDimensions is on the hot path (5× per resize in connectrpc_websocket.go)
- GetSessionIdentifier() needed to replace GetTmuxSessionName() for review_queue_poller.go:398 and pty_discovery.go:275,291

### Stakeholders
- Tyler Stapler (sole developer and user)

## Open Questions
- All major questions resolved in research phase (see research/synthesis.md)
