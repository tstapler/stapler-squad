# Research Synthesis: Session Resumption

_Date: 2026-04-03_
_Sources: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md_

---

## Executive Summary

Session resumption for stapler-squad is achievable with userspace Go on macOS. The key realization from research is that **full process checkpoint (CRIU-style) is not viable on macOS** — but it is also not necessary. The combination of Claude's native JSONL conversation history + tmux scrollback capture + the existing `claude-mux` PTY multiplexer covers all four must-have requirements. Each requirement maps to existing infrastructure that needs extension, not replacement.

---

## Critical Platform Constraint

**macOS userspace cannot re-parent a running process's controlling terminal.** `reptyr` (the Linux tool for this) explicitly does not work on macOS — it requires `PTRACE_GETREGS`, `PTRACE_SETREGS`, `PTRACE_POKEDATA`, and `PTRACE_SYSCALL`, which macOS `ptrace` does not implement. No userspace workaround exists.

**Implication**: The `claude-mux` cooperative wrapper is the ONLY viable PTY adoption path. Sessions started without the wrapper can be identified (read-only metadata) but not controlled. This is a hard constraint that must be communicated in the plan.

---

## Requirement-to-Implementation Mapping

### Req 1: History File Detection

> _Detect which history files a process is writing to and associate with session record_

**Solution**: `gopsutil v3 Process.OpenFiles()` polling at 2-3 second intervals + `fsnotify` (recursive) on `~/.claude/projects/` for fast new-file detection.

**How it works**:
- `Process.OpenFiles()` uses Apple's `proc_pidinfo` via CGo — works for same-user processes, no entitlements
- Returns full paths for all open vnodes; filter for `~/.claude/projects/**/*.jsonl`
- `fsnotify` fires on new file creation; correlate with `gopsutil` scan to find owning PID
- Also grab `Process.Terminal()` (controlling TTY device) and `Process.Cwd()` in the same call

**What to store in session record**: `claude_conversation_uuid` (filename of the JSONL), `claude_project_dir` (encoded cwd directory), `history_file_path` (full absolute path).

**Key pitfall**: Handle partial JSONL lines (use `bufio.Scanner`, skip last line if JSON-unparseable). Use `filepath.EvalSymlinks()` when comparing paths.

---

### Req 2: Process Adoption / Re-parenting

> _Adopt an externally-started TTY process while keeping it available in its original terminal_

**Solution**: Connect to existing `claude-mux` Unix socket as an additional client. This is already implemented — stapler-squad needs to:
1. Discover the socket via `fsnotify` watching `/tmp/` for `claude-mux-*.sock` (already in `session/mux/autodiscover.go`)
2. Connect as a secondary client with retry + backoff (handle startup race)
3. Route `Output` messages from the socket to: scrollback storage + web UI stream
4. Route web UI `Input` messages to the socket

For sessions started WITHOUT `claude-mux`: display as "observed (read-only)" — show metadata from gopsutil but no PTY control.

**New component needed**: `session/bridge/adopted_bridge.go` — a `Run(ctx)` goroutine that handles the socket relay with checkpoint support.

**Key pitfall**: The `claude-mux` socket multiplexer must NOT broadcast input from one client to other clients. The web UI's input should only route to the PTY, not echo back to the original terminal. Verify this in `session/mux/multiplexer.go`.

---

### Req 3: Resume After Restart

> _Restore session state after stapler-squad restarts_

**Two-tier strategy** (from iTerm2/Zellij research):

**Tier 1 — Hot Resume** (tmux session still alive):
- stapler-squad restarts, detects existing tmux session by name
- Reconnects to `claude-mux` socket if present (bridge re-established)
- Web UI stream re-attaches to existing scrollback + new output
- Zero state loss

**Tier 2 — Cold Restore** (tmux session dead, e.g. after OS reboot):
1. Read persisted session metadata (cwd, git branch, `claude_conversation_uuid`)
2. Create new tmux session in saved cwd
3. Replay scrollback from `FileScrollbackStorage` into new pane (replay lines 0..N)
4. Launch `claude --resume <claude_conversation_uuid>` in the pane
5. Conversation context fully restored; scrollback display restored; git state restored via worktree

**What to persist before shutdown** (add to graceful shutdown path):
- `cwd` (from `gopsutil Cwd()` or tmux `pane_current_path`)
- `git_branch` (from `git rev-parse --abbrev-ref HEAD`)
- `claude_conversation_uuid` (from JSONL file open detection or session metadata)
- `scrollback_path` (already persisted in `FileScrollbackStorage`)
- `tmux_layout` (from `tmux display-message -p '#{window_layout}'`)

**Key pitfall**: Never use PID as session identity across restarts. Use `{conversation_uuid, pid_start_time}` composite. PID reuse on macOS is common.

---

### Req 4: Checkpoint / Fork

> _Fork a session from a given historical point into a new independent session_

**Solution**: Three-step fork operation:

1. **Snapshot the checkpoint**: Record `{scrollback_seq, claude_conv_uuid, git_commit_sha, timestamp}` at the fork point — no process interruption needed.

2. **Clone scrollback**: `ForkScrollback(srcPath, upToSeq, dstPath)` — `bufio.Scanner` copy of first N lines into a new JSONL file. O(N) but fast for typical session sizes.

3. **Clone Claude conversation**: Copy `~/.claude/projects/<hash>/<uuid>` truncated to message index at checkpoint time, placed in new UUID path. Start forked session with `claude --resume <new-uuid>`. (Or use `claude --fork <parentMessageUUID>` if Claude CLI exposes this flag.)

4. **Create new git branch**: `git checkout -b fork/<label> <checkpoint_git_sha>` in the worktree.

**Storage schema**: Add `Checkpoint` struct to `session/storage.go`. Adjacency list with `ParentID` — mirrors git commit model. No Ent ORM; no new DB schema.

**Key pitfall**: Claude's JSONL files are large for long conversations. Copy the file before truncating (never truncate in place). Use a temp file + atomic rename.

---

## Cross-Cutting Concerns

### Session Identity Model

```
Stable identity = claude_conversation_uuid  (most stable — UUID never changes)
Liveness check  = claude_mux_socket_path    (if socket exists, session is live)
PID disambiguation = {pid, pid_start_time_ms} (guards against PID reuse)
```

Never trust PID alone. Always verify with `gopsutil Process.CreateTime()`.

### Two-Tier Resume is the Right Mental Model

```
Is the tmux session alive?
  YES → Hot resume: reattach. Free, instant, zero loss.
  NO  → Cold restore:
          conversation: claude --resume <uuid>
          scrollback:   replay FileScrollbackStorage lines into pane
          git:          worktree already has the right branch
```

This mental model maps 1:1 to how tmux-resurrect, Zellij, and iTerm2 all work. It is the correct design.

### History File as the Recovery Anchor

Claude's JSONL conversation file is the most reliable recovery artifact:
- Stored outside stapler-squad's control (in `~/.claude/projects/`)
- Survives stapler-squad crashes, OS reboots, even tmux kills
- Contains the full conversational context needed for `--resume`
- Its UUID filename is the stable session identity anchor

Design around this: **if you have the Claude conversation UUID, you can always recover the session**.

### What CANNOT Be Recovered (Accept This)

After a cold restore:
- The process's open file descriptors (other than the history file)
- In-flight network connections
- Claude's current "thinking" state (it restarts from conversation context, not mid-computation)
- Exact terminal cursor position / application state (vim, etc.)

For an AI coding session, conversation context is what matters. These limitations are acceptable.

---

## Implementation Phasing

### Phase 1 MVP (Minimum viable — highest value)

1. **History file detection + linking**: `gopsutil OpenFiles()` + `fsnotify` poll loop. Store `claude_conversation_uuid` in session record.
2. **Cold restore after restart**: Persist cwd + conversation UUID on graceful shutdown; relaunch with `--resume` on restart.
3. **Checkpoint metadata creation**: UI button to name+capture `{scrollback_seq, timestamp, git_sha}` checkpoint. No fork yet — just bookmarks.

### Phase 2

4. **Checkpoint fork**: Implement `ForkScrollback` + conversation JSONL clone + new git branch creation.
5. **Adopted session bridge improvements**: Retry logic, socket registry file for fast reconnection after restart.

### Phase 3 (Post-MVP)

6. **Read-only external process discovery**: gopsutil scan for non-mux'd Claude sessions; display as "observed" in UI.
7. **OSC 7 cwd tracking**: Parse OSC 7 sequences from PTY output to track cwd changes without polling.
8. **VT state snapshots**: Serialize rendered terminal state at checkpoint time for O(1) cold restore (vs. O(N) replay).

---

## Technology Decisions (For plan.md)

| Decision | Choice | Rationale |
|---|---|---|
| Process inspection | `gopsutil v3` | Only viable macOS userspace option; already widely used |
| File watching | `fsnotify` with recursive flag | Backed by FSEvents on macOS; already used for `/tmp/` socket discovery |
| Process lifecycle | `kqueue EVFILT_PROC` via `golang.org/x/sys/unix` | Zero-latency exit detection for cleanup |
| Session storage | Extend existing JSON structs in `session/storage.go` | No Ent ORM; no new deps |
| Scrollback fork | Full clone (`bufio.Scanner` copy) | Simple, independent, O(MB) not O(GB) |
| Conversation fork | JSONL copy + truncate to new UUID path | Works with existing `claude --resume` |
| PTY adoption | Additional `claude-mux` socket client | Already the correct architecture; no changes to mux protocol |
| Session identity | `claude_conversation_uuid` as primary | Stable across restarts, reboots, PID reuse |
| Process checkpoint | NOT attempted | Not viable on macOS userspace; not needed |
| Ent ORM | NOT introduced | Session storage is JSON-based; Ent is not the primary store |

---

## Open Questions for plan.md

1. **Does `claude-mux` socket multiplexer correctly isolate input from multiple clients?** (Check `session/mux/multiplexer.go` — input from web UI must not echo to original terminal)
2. **Does `claude --fork <parentUUID>` exist as a CLI flag?** (Would simplify conversation forking vs. manual JSONL copy)
3. **What is the socket buffer size in `claude-mux`?** (Determines how much output history a new adopter client receives on connect — affects whether cold-adopted sessions show historical output)
4. **Is the `claude_conversation_uuid` already stored anywhere in current session metadata?** (Check `session/instance.go` for any existing history file linkage)

---

## Research Dimensions — Status

- [x] **Stack** — `gopsutil`, `fsnotify`, `kqueue`, `lsof`; process checkpoint not viable on macOS → `findings-stack.md`
- [x] **Features** — tmux-resurrect, Zellij, Claude JSONL format, Aider, iTerm2 → `findings-features.md`
- [x] **Architecture** — adjacency list checkpoints, full-clone fork, claude-mux socket bridge → `findings-architecture.md`
- [x] **Pitfalls** — macOS ptrace limits, FSEvents coalescing, partial JSONL lines, PID reuse, socket race conditions → `findings-pitfalls.md`
