# ADR-001: Eliminate tmux subprocess forks via control mode command dispatch

**Status**: Proposed
**Date**: 2026-04-24
**Project**: tmux-optimization

## Context

Execution trace profiling revealed ~138 subprocess spawns/second at idle, traced to `os/exec.(*Cmd).Start.func2` (277 goroutines). Each `tmux` CLI invocation forks two goroutines (stdout/stderr pipe copiers) and takes 100–750ms wall time.

The codebase already runs `tmux -C attach-session` per session for real-time output streaming (`readControlModeOutput` in `session/tmux/control_mode.go`). The `controlModeStdin io.WriteCloser` is open and connected to the tmux control mode process, but has never been used to send commands. The tmux control mode protocol documents that commands can be sent over stdin and receive structured `%begin`/`%end` delimited responses on stdout — this is the zero-subprocess path.

Nine call sites in `session/tmux/tmux.go` currently spawn a new `tmux` subprocess for every invocation:
- `CapturePaneContent()`, `CapturePaneContentRaw()`, `CapturePaneContentWithOptions()`
- `GetPaneDimensions()`, `GetCursorPosition()`, `GetPaneCurrentPath()`, `GetPanePID()`
- `RefreshClient()`, `SetWindowSize()`

Three approaches were evaluated for eliminating these forks.

## Decision

We decided to wire a FIFO request/response multiplexer into the existing `controlModeStdin` pipe rather than spawning subprocesses for tmux query operations.

Commands are written as newline-terminated text to `controlModeStdin`. Responses arrive on the already-running stdout reader as `%begin TIME CMDNUM FLAGS` / body lines / `%end TIME CMDNUM FLAGS`. tmux guarantees sequential ordering ("will never mix output for different commands"), so a simple channel queue — no MSGID map — is sufficient for concurrency safety.

New fields on `TmuxSession`:
```go
pendingCmds  []chan cmdResult  // FIFO queue, protected by controlModeSubMu
cmdBodyBuf   bytes.Buffer      // accumulates lines between %begin and %end
inCmdResp    bool              // state machine flag
```

`processControlModeLine()` in `control_mode.go` is extended to detect `%begin`/`%end` and route body content to the head of `pendingCmds`.

All 9 call sites are migrated behind a feature flag `STAPLER_SQUAD_CM_COMMANDS=true`, with fallback to subprocess when control mode is not running.

## Alternatives Considered

- **Raw Unix socket IPC (tmux `imsg` binary protocol)**: Rejected. tmux uses a private binary protocol based on BSD's `imsg` framing. It is undocumented, not guaranteed stable across tmux releases, and has no Go implementation. Implementing it would take 2–4 weeks and break on any tmux upgrade. It would only eliminate the one persistent control-mode attach process — which must exist anyway for streaming — providing no benefit over control mode command dispatch.

- **Continue subprocess per call with TTL caching only**: Rejected as the sole long-term fix. TTL caching (Phase 1) eliminates the majority of redundant subprocess calls at idle, but does not eliminate subprocess overhead for calls that do need to fire. Control mode command dispatch eliminates the per-call fork entirely.

## Rationale

Control mode command dispatch is the documented, production-proven path. iTerm2 uses this exact protocol for all terminal operations. The protocol is stable across tmux versions (documented in the tmux wiki). The `controlModeStdin` pipe is already open — the only work required is wiring the state machine and the FIFO queue. This eliminates 100% of subprocess forks for query operations with no dependency on tmux internals.

## Consequences

**Positive:**
- Eliminates all per-call `tmux` subprocess forks for query operations (~138/second at idle → near zero)
- Uses stable, documented protocol — no risk of breakage on tmux upgrades
- Already-running control mode process is required for streaming anyway; no new persistent process added
- Sequential ordering guarantee from tmux means simple FIFO channel queue; no complex correlation map

**Negative / Risks:**
- Adds a state machine to `processControlModeLine()` (~200–350 LOC)
- Requires fallback-to-subprocess when control mode is not running (e.g., during session startup)
- Feature flag rollout needed to validate correctness before full migration
- Three open verification questions must be answered before migrating all call sites (see Follow-up)

**Follow-up work:**
- Verify `display-message` format string quoting over CM stdin: does `"#{pane_width} #{pane_height}"` parse correctly without shell quoting?
- Verify `refresh-client -t SESSION` behavior when sent from the CM connection itself attached to that session
- Confirm `capture-pane -S/-E` line range flags work over CM stdin
- Implement feature flag `STAPLER_SQUAD_CM_COMMANDS`, start with `GetPaneDimensions` (smallest output), run parallel paths 24h logging discrepancies, then migrate remaining 8 call sites

## Related

- Research: `project_plans/tmux-optimization/research/findings-control-mode-commands.md`
- Research: `project_plans/tmux-optimization/research/findings-tmux-socket-ipc.md`
- Synthesis: `project_plans/tmux-optimization/research/synthesis.md`
- Source: `session/tmux/control_mode.go` (`processControlModeLine`, `readControlModeOutput`, `controlModeStdin`)
- Source: `session/tmux/tmux.go` (9 call sites to migrate)
- Supersedes: (none)
- Related ADRs: ADR-002 (TTL caching — Phase 1 prerequisite)
