# Requirements: Session Resumption

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-03

## Problem Statement

Two related problems make AI coding sessions fragile and disposable:

1. **Context loss on restart**: When stapler-squad restarts, previously managed sessions lose their state. Users must re-establish context manually.

2. **External process orphaning**: Processes started outside stapler-squad (e.g., in IntelliJ or VS Code terminals) run unmanaged — no lifecycle tracking, no history capture, and no integration with the squad dashboard.

Primary users: solo developers using Claude Code and similar AI coding tools across multiple terminal contexts (IDE-embedded terminals, tmux, standalone terminals).

## Success Criteria

1. After a stapler-squad restart, any previously managed session can be resumed — scrollback history, working directory, and program-specific conversation history intact.
2. An externally-started AI coding process (Claude, Aider, etc.) can be adopted into stapler-squad management while continuing to run in its original terminal location (e.g., IntelliJ's terminal pane).
3. A session can be forked from a specific point in its history, creating a new managed session that starts from that state.
4. History files written by managed programs are detected, tracked, and linked to their session metadata.

## Scope

### Must Have (MoSCoW — Phase 1 MVP)
- History file detection: detect which program-specific history files (e.g., `~/.claude/projects/*.jsonl`, shell REPL history) a process is writing to, and associate them with the session record
- Process adoption / re-parenting: adopt an externally-started TTY process into stapler-squad management via the existing claude-mux socket infrastructure, while keeping the process available in its original terminal
- Resume after restart: restore session state (scrollback buffer, working directory, linked history file paths) after stapler-squad restarts
- Checkpoint/fork: fork a session from a given historical point into a new independent session branch

### Nice to Have (Post-MVP, phased rollout)
- Remote sessions (SSH/cloud)
- Non-AI generic shell session management
- Cross-machine session sync
- Automatic conflict resolution when the same session is adopted by multiple instances

### Out of Scope
- Kernel-level, eBPF, or OS-level changes — userspace only
- Breaking changes to the existing claude-mux protocol or socket format

## Constraints

- **Tech stack**: Go backend only; no new language runtimes
- **Compatibility**: Must remain backward-compatible with existing claude-mux PTY multiplexer (`session/mux/`)
- **Platform**: macOS-first; Linux support is secondary
- **Userspace only**: No kernel modules, eBPF, or OS-level patches

## Context

### Existing Work

The following components are already built and should be extended rather than replaced:

| Component | Location | Relevance |
|---|---|---|
| `ClaudeSessionHistory` | `session/history.go` | Reads `~/.claude/projects/*.jsonl`; parses Claude conversation files; indexes by project path |
| `FileScrollbackStorage` | `session/scrollback/storage.go` | Persists terminal scrollback as line-delimited JSON with optional zstd/gzip compression |
| claude-mux PTY multiplexer | `session/mux/multiplexer.go`, `cmd/claude-mux/main.go` | Unix domain socket bridge for external terminal sessions; already handles I/O routing and auto-discovery |
| `session/mux/autodiscover.go` | `session/mux/autodiscover.go` | Filesystem-watching discovery of claude-mux sockets in `/tmp/` |
| Ent schema | `session/ent/schema/` | Existing DB schema for sessions, tags, worktrees; will need new fields for history file paths and checkpoints |

### Key Architecture Facts
- Server runs on `localhost:8543`; ConnectRPC endpoints in `server/services/`
- Proto definitions in `proto/session/v1/`; `make proto-gen` regenerates Go + TS code
- Session state persisted via Ent ORM (SQLite or Postgres)
- `session/instance.go` manages the lifecycle of individual sessions

### Stakeholders
- Primary: Tyler Stapler (sole developer and user)
- Secondary: Any future users of claude-squad / stapler-squad

## Research Dimensions Needed

- [ ] **Stack** — evaluate technology options: file descriptor tracking (lsof/procfs/kqueue), checkpoint/restore approaches (CRIU alternatives on macOS), Unix process re-parenting techniques
- [ ] **Features** — survey comparable tools: tmux session persistence (tmux-resurrect, tmux-continuum), iTerm2 session restoration, Zellij session management, how Claude CLI stores and resumes conversation context
- [ ] **Architecture** — design patterns: how to model session checkpoints in Ent schema, how to bridge adopted processes without breaking the existing claude-mux socket model, forking strategy (copy-on-write vs. full clone of conversation state)
- [ ] **Pitfalls** — known failure modes: TTY re-parenting limitations on macOS (no `reparent` syscall), race conditions between scrollback capture and process adoption, history file locking conflicts, PIDs vs. stable session IDs across restarts
