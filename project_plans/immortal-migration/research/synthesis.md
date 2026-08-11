# Research Synthesis: Process Manager Modularization

Created: 2026-04-29
Last verified: 2026-05-22
Input: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

## Decision Required

Decide how to add a config-driven process manager abstraction to stapler-squad so the Tmux dependency can be replaced with a Go-native in-process supervisor — or determine that Tmux cannot be replaced at this time.

## Context (updated 2026-05-14: post-main-merge)

Stapler-squad hard-codes Tmux as its process manager, preventing single-binary deployment and offering no auto-restart on crash. The goal is a `ProcessManager` interface that lets Tmux remain the default while enabling a future `NativeProcessManager` backed by PTY + a restart goroutine. Immortal was the candidate evaluated first.

**Constraints:** Pure Go (no CGO), single-binary, Tmux stays as default, backwards-compatible config.

### Executor Framework — Now Available (merged from main 2026-05-14)

Main now contains `executor/` — a safe subprocess management layer:

| Type | What it does | Applicable to NativeProcessManager? |
|---|---|---|
| `safeexec.CommandContext` | Wraps `exec.CommandContext` with `WaitDelay=2s` | Already used in `session/tmux/tmux.go`; TmuxBackend inherits transparently |
| `executor.ShortLivedCmd` | One-shot subprocess with timeout, env, process group | Yes — use for metadata queries (CWD, PID checks) |
| `executor.ManagedProcess` | Long-running pipe-based subprocess with SIGTERM/SIGKILL | **No** — pipe-based only, cannot allocate a PTY |
| `norawexec` lint rule | Custom go/analysis pass at `tools/lint/norawexec/`; forbids bare `exec.Command`/`exec.CommandContext` outside `executor`/`executor/safeexec` packages | PTY start must carry `//nolint:norawexec long-running PTY process; WaitDelay not applicable` |

**Key constraint from norawexec:** NativeProcessManager's PTY launch (`pty.StartWithAttrs(cmd)`) must add `//nolint:norawexec` with justification. All other subprocess calls (metadata, cleanup) should use `executor.ShortLivedCmd` or `safeexec.CommandContext`.

## Options Considered

| Option | Embeddable | PTY Support | Restart Logic | Effort | Verdict |
|---|---|---|---|---|---|
| **Immortal** | No (daemon-shaped, spawns supDir + sockets) | No (pipes only) | Retries + wait only | High (6 deps, 7–11 days) | **Ruled out** |
| **suture** | Yes (goroutine supervisor) | No (goroutine-level, not OS processes) | Yes (OTP-style) | Medium | **Wrong layer** |
| **Custom NativeProcessManager** | Yes (in-process goroutine) | Yes (creack/pty already in go.mod) | Yes (exponential backoff goroutine, ~50 LOC) | Low-Medium (~250 LOC core) | **Recommended** |
| **Tmux (status quo)** | Via embedded binary | Yes | No | 0 | Keep as default backend |

## Dominant Trade-off

**Daemon architecture vs. in-process library.** All existing purpose-built process supervisors (Immortal, s6, supervisord) are designed to be the init-like root process for a service tree, not a library callable from inside another process. They carry filesystem artifacts (lock files, sockets, supDirs) and process model assumptions (setsid, daemon fork) that are incompatible with embedding in a long-running Go binary.

The only viable path for single-binary embedding is a custom in-process supervisor: a ~250-line goroutine that calls `os/exec` + `creack/pty` and implements exponential backoff restart on process exit.

## Recommendation

**Choose: Custom NativeProcessManager** (phased, behind feature flag)

**Because:**
- Immortal is daemon-shaped (spawns supDir + Unix socket per process) and has no PTY support — the two hardest constraints are both blockers, not trade-offs
- `creack/pty` is already in `go.mod`; the restart logic is ~50 lines of well-understood Go; no new dependencies needed
- A `TmuxManager` interface already exists in `session/tmux_process_manager.go` — the codebase has the abstraction seam; this is an implementation swap, not an interface design problem
- Pitfalls are known and solvable with established patterns: `Wait()` discipline, `startMu` double-checked locking (already present for the double-start race), `Setpgid: true` in SysProcAttr, SIGTERM forwarding in Shutdown()

**Accept these costs:**
- Control mode streaming (the tmux `-C` protocol) has no direct equivalent — the hardest 70% of the migration is replacing `%begin`/`%output`/`%end` framing with a raw PTY fan-out goroutine. This requires a spike to confirm the web UI can consume raw PTY bytes instead of tmux-framed events.
- Session persistence across stapler-squad restarts is not solved — child processes die when the PTY master fd closes. The claude-mux shim is a candidate future solution; out of scope for this phase.
- `GetCursorPosition()` and `GetPaneDimensions()` query tmux's virtual screen — the native equivalent requires VT100 ANSI response parsing or a fixed fallback.

**Reject these alternatives:**
- Immortal: No PTY, daemon-shaped, on-disk runtime artifacts per session — all three are hard blockers given the single-binary + terminal-streaming requirements
- suture: Goroutine supervisor (Erlang OTP model), not OS process supervisor — solves a different problem; could compose inside NativeProcessManager as the retry engine but adds a dependency without meaningful benefit over a hand-written backoff loop

## Phased Implementation Plan

**Phase 1 — Interface extraction (zero risk)**
- Create `session/process_manager.go`: thin `ProcessManager` interface + `ProcessConfig` struct
- Create `session/tmux/tmux_process_manager.go`: `TmuxProcessManager` adapter wrapping existing `TmuxSession`
- Wire `Instance` to use `ProcessManager` (replaces direct tmux calls)
- Gate: `process_manager: "tmux"` in config (default); `"native"` for new backend
- Estimated effort: ~500 LOC changes, zero behavior change, all existing tests pass

**Phase 2 — NativeProcessManager core (moderate risk)**
- Implement `session/native/native_process_manager.go`:
  - `os/exec.Cmd` + `creack/pty` for PTY allocation
  - Exponential backoff restart goroutine (`Wait()` goroutine + `startMu` locking)
  - `Setpgid: true` in `SysProcAttr` + SIGTERM forwarding in `Close()`
  - PTY fan-out goroutine replacing control mode (raw byte broadcast to WebSocket subscribers)
- Estimated effort: ~400 LOC new code
- Spike required: Confirm web UI can consume raw PTY bytes (see Open Questions)

**Phase 3 — Session persistence (optional, deferred)**
- Investigate whether claude-mux or a named-pipe approach can survive stapler-squad restarts
- Out of scope until Phase 2 is validated in production

## Open Questions Before Committing to Phase 2

- [x] ~~Does the web UI terminal renderer expect tmux-framed events (`%begin`/`%output`/`%end`) or can it consume raw PTY bytes?~~ **Resolved: raw bytes.** The tmux octal encoding is stripped in `decodeControlModeOutput()` before bytes reach the subscriber channel. `connectrpc_websocket.go` wraps the raw bytes directly into `TerminalOutput.Data` protobuf and ships them to xterm.js, which handles VT/ANSI natively. NativeProcessManager just reads from `ptmx` and broadcasts to the same `chan []byte` — no protocol changes needed anywhere in the stack.
- [x] ~~Does the existing `TmuxManager` interface cover all methods?~~ **Resolved: ~85 call sites confirmed** (68+ unique grep matches in `session/instance.go` plus ~20 more). The field `tmuxManager` at `instance.go:248` is declared as **concrete** `TmuxProcessManager` — Phase 1 must change this to the `TmuxManager` interface type. `GetTmuxSessionName()` is the current interface method; requirements suggest renaming to `GetSessionIdentifier()` for backend-agnostic naming — decision needed before Phase 1 finalization.
- [ ] Is `GetCursorPosition()` / `GetPaneDimensions()` used in the rendering hot path, or only for metadata? — blocks: whether VT100 response parsing is required or a static fallback is acceptable
- [ ] `open-feature/go-sdk` is **not** in `go.mod` — requirements mention "OpenFeature wiring" for config-driven backend selection. Decision needed: use OpenFeature SDK, use the existing JSON config system directly (`config/`), or a simpler build tag? — blocks: Phase 1 feature flag design

If the first question cannot be answered from code inspection alone, a 1-day prototype is required before writing the planning ADR.

## Sources

- `project_plans/immortal-migration/research/findings-stack.md`
- `project_plans/immortal-migration/research/findings-features.md`
- `project_plans/immortal-migration/research/findings-architecture.md`
- `project_plans/immortal-migration/research/findings-pitfalls.md`
- Immortal source: https://github.com/immortal/immortal (pkg.go.dev confirmed: `Daemon`, `Supervise`, `New`, `Process`, `Config` exported; `Fork()` calls `setsid(2)`; no PTY fd exposed)
