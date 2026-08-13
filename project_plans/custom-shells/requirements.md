# Custom Shells — Requirements

## Problem Statement

Users run companion processes and ad-hoc shell commands alongside Claude sessions but currently have no way to view their output or interact with them within the Stapler Squad UI. The primary day-one use case is exploratory testing: spawning an interactive shell in the same workspace directory as a session to run commands, inspect state, and validate Claude's work without leaving the UI.

## Goals

1. Allow users to spawn one or more custom shells/commands attached to a Claude session.
2. Display attached shells as tabs in the session view (alongside the existing "Terminal" tab for Claude output).
3. Provide full PTY interaction: view scrollback, send input, and see real-time output.
4. Show health/status at a glance (running, exited, crashed).
5. Allow shells to keep running when their parent session is paused or stopped, and re-associate them when the session is resumed.

## Non-Goals

- Global shell manager across all sessions (out of scope for v1).
- Saved command presets (out of scope for v1; users enter commands ad-hoc).
- Shell output persistence across server restarts (nice-to-have, not required).

## User Stories

### US-1: Spawn a shell attached to a session
As a user, I can open a session and click "New Shell" (or similar) to spawn an interactive shell tab attached to that session, so I can run exploratory commands in the session's workspace.

**Acceptance Criteria:**
- AC-1.1: A "New Shell" affordance is visible in the session view (e.g., a "+" button in the tab strip).
- AC-1.2: Clicking it opens a dialog/form where I can enter a command (default: `$SHELL` or `bash`) and an optional working directory (default: session working directory).
- AC-1.3: After confirming, a new tab appears in the session's tab strip labelled with the command or a user-supplied name.
- AC-1.4: The shell starts in the specified working directory.

### US-2: Interact with an attached shell
As a user, I can view the shell's output in real time and type commands into it, just like a terminal emulator.

**Acceptance Criteria:**
- AC-2.1: Shell output streams into the tab's terminal widget (same xterm.js component used for session output).
- AC-2.2: Typing in the focused tab sends input to the shell's PTY.
- AC-2.3: Scrollback history is preserved while the tab is open.

### US-3: Shell lifecycle indicators
As a user, I can see at a glance whether a shell is running or has exited.

**Acceptance Criteria:**
- AC-3.1: Running shells show a green status indicator on the tab.
- AC-3.2: Exited/crashed shells show a red indicator and display the exit code.
- AC-3.3: The tab title shows the command (truncated if long).

### US-4: Restart and stop a shell
As a user, I can stop a running shell or restart a stopped one without leaving the UI.

**Acceptance Criteria:**
- AC-4.1: A "Stop" button is available for running shells (sends SIGTERM to the process group).
- AC-4.2: A "Restart" button is available to relaunch the same command in the same directory.
- AC-4.3: A "Close" (remove) button permanently removes the tab and kills the process if running.

### US-5: Shell persistence across session pause/resume
As a user, my attached shells keep running when I pause or stop their parent session, and they reappear in the tab strip when I resume the session.

**Acceptance Criteria:**
- AC-5.1: Pausing or stopping a session does not kill attached shell processes.
- AC-5.2: When a session is resumed, existing attached shell tabs are restored with their current status.
- AC-5.3: Output generated while the session was paused is visible in scrollback after resume (best-effort; limited by tmux scrollback buffer).

### US-6: Multiple shells per session
As a user, I can open more than one shell tab per session to run multiple concurrent commands.

**Acceptance Criteria:**
- AC-6.1: There is no hard limit enforced in the UI (soft limit of ~10 for performance).
- AC-6.2: Each shell has its own independent PTY, working directory, and status.

## Constraints

- Backend: Go, tmux-backed PTY (consistent with existing session management in `session/`).
- Frontend: React SPA, xterm.js terminal widget (already used for session output), ConnectRPC streaming.
- Shells must be scoped to the session's workspace (same isolation boundary as the session's git worktree).
- Working directory must be configurable per shell; defaults to the session's configured path.
- No breaking changes to the existing session terminal tab.

## Out of Scope

- Saved presets / named commands in config files.
- Shell sharing between sessions.
- Shell output export / download.
- Collaborative / multi-user shell viewing.
