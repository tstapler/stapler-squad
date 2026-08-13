# ADR-002: Two-Tier Resume Strategy (Hot / Cold)

**Status**: Accepted
**Date**: 2026-04-03
**Context**: Session Resumption feature — how to restore sessions after stapler-squad restarts or system reboots.

## Decision

Implement a two-tier resume strategy modeled after tmux-resurrect and iTerm2's session restoration:

### Tier 1: Hot Resume (tmux session alive)

- **Trigger**: Stapler-squad restarts but tmux sessions survive (most common case)
- **Action**: Reattach to existing tmux session by name; reconnect to claude-mux socket if present
- **Cost**: Zero state loss, instant
- **Already works**: The existing `FromInstanceData` -> `Start(false)` path calls `tmuxManager.RestoreWithWorkDir()` which reconnects to alive tmux sessions

### Tier 2: Cold Restore (tmux session dead)

- **Trigger**: OS reboot, tmux server crash, or explicit tmux kill
- **Action**:
  1. Read persisted session metadata (cwd, git branch, conversation UUID)
  2. Create new tmux session in saved cwd
  3. Launch `claude --resume <uuid>` in the pane
  4. Git worktree already preserves branch state
  5. Scrollback from FileScrollbackStorage available for display in web UI
- **Cost**: Conversation context restored via Claude's native `--resume`; terminal visual state starts fresh

### Graceful Shutdown Capture

Before shutdown, persist:
- `cwd` (from the tmux pane or gopsutil)
- `git_branch` (from worktree manager)
- `claude_conversation_uuid` (from history file detection or existing ClaudeSessionData)
- `history_file_path` (absolute path to the JSONL)

This data is written to existing InstanceData fields during the normal `SaveInstances` path.

## Alternatives Considered

1. **CRIU-style process checkpoint**: Rejected. Not viable on macOS userspace.

2. **Full scrollback replay into tmux pane**: Deferred to Phase 3 (VT state snapshots). For MVP, the web UI displays historical scrollback from FileScrollbackStorage. The tmux pane starts fresh with `claude --resume`.

3. **Single-tier resume (always cold restore)**: Rejected. Hot resume is free and preserves full state.

## Consequences

- `FromInstanceData` must detect whether the tmux session is alive and branch accordingly (already partially implemented via `tmuxManager.DoesSessionExist()`)
- Cold restore path must construct the claude command with `--resume <uuid>` (already supported via `ClaudeCommandBuilder`)
- Graceful shutdown must capture cwd and conversation UUID before process exit
- The web UI must handle the case where scrollback comes from persisted storage (historical) vs live stream (current)

## Risks

- If the user kills tmux with `tmux kill-server` while stapler-squad is running, the hot resume check will fail at next startup. This correctly falls through to cold restore.
- Cold restore does not recover in-flight Claude "thinking" state. Claude restarts from the last completed message. This is acceptable for AI coding sessions.
