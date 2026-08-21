# Features Research: Session Resumption

_Researched: 2026-04-03_

---

## tmux-resurrect / tmux-continuum

### tmux-resurrect

**What is saved** (via `tmux list-panes`, `tmux list-windows`):
- Session names and active window index
- Window layout string (e.g. `even-horizontal,220x50,0,0`)
- Pane current working directory (`pane_current_path`)
- Pane current command / running process (`pane_current_command`)
- Optionally: pane scrollback content (disabled by default)

**File format**: Tab-separated text in `~/.tmux/resurrect/last` (with timestamped copies):
```
pane	session_name	window_idx	win_name	win_active	win_flags	pane_idx	pane_dir	pane_active	pane_cmd	pane_full_cmd
window	session_name	window_idx	win_name	win_active	win_flags	layout_string
state	app_version
```

**Scrollback persistence** (optional): Saves pane content to `~/.tmux/resurrect/pane_contents.txt.gz`. Restores by piping content into pane via `tmux send-keys`. Fragile — it's sending keystrokes, not actual terminal data.

**Restore mechanism**:
1. Creates sessions/windows/panes with `tmux new-session`, `new-window`, `split-window`
2. Applies layout string via `tmux select-layout`
3. Changes directory with `tmux send-keys "cd <dir>" Enter`
4. Optionally re-runs saved command (configurable per-program via hook)

**Key limitation**: Cannot restore process state (open FDs, network connections). Programs like vim/nvim can be re-launched; SSH/DB connections cannot.

**Source**: https://github.com/tmux-plugins/tmux-resurrect

### tmux-continuum

Adds interval-based auto-save (default: 15 minutes) via tmux's `after-new-session` hook. Creates timestamped save files:
```
~/.tmux/resurrect/tmux_resurrect_2026-04-03T14:30:00.txt
~/.tmux/resurrect/last → (symlink to most recent)
```

**Point-in-time restore**: The multiple timestamped files enable restoring to any previous checkpoint — just re-symlink `last`. This is the simplest possible checkpoint model.

**Relevance**: The timestamped snapshot model (multiple files, `last` symlink) is directly applicable to session forking. Each "checkpoint" is a snapshot file.

**Source**: https://github.com/tmux-plugins/tmux-continuum

---

## Zellij Session Management

### Persistent Sessions

Unlike tmux, Zellij sessions survive client disconnects — the session daemon keeps running. `zellij list-sessions` shows all running sessions with name, creation time, and connected client count. `zellij attach <name>` reconnects instantly (hot resume, zero state loss).

### Layout Serialization (KDL)

Zellij uses KDL (Cuddly Data Language) for layouts, saved to `~/.cache/zellij/layouts/<session-name>.kdl`:

```kdl
layout {
    tab name="editor" {
        pane split_direction="vertical" {
            pane command="nvim" { args "." }
            pane cwd="/home/user/project"
        }
    }
    tab name="shell" { pane }
}
```

Captures: tab names, pane splits + sizes, commands, cwds. Does NOT capture scrollback or process state.

### Session Resurrection (v0.38+)

`zellij action dump-layout` writes the current live layout to KDL. When attaching to a dead session by name, Zellij prompts: "Session <name> was not found. Would you like to resurrect it?" — then recreates from the last saved KDL.

**Relevance**:
- The dump-and-restore cycle is the correct model for session resume
- The "resurrect by name" UX pattern is worth copying
- KDL layout format is elegant and human-readable

**Source**: https://zellij.dev/news/session-resurrection/

---

## Claude CLI Conversation History

### File System Structure

```
~/.claude/projects/<cwd-encoded>/<session-uuid>
```

Where `<cwd-encoded>` is the absolute working directory with `/` → `-` (e.g., `/Users/tyler/myproject` → `-Users-tyler-myproject`).

Each session is a **single UUID-named JSONL file**. Confirmed by direct inspection of `~/.claude/projects/-Users-tylerstapler-IdeaProjects-claude-squad/` — ~27 session files observed for this project.

### JSONL Entry Structure

Each line is a JSON object:

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `"user"` or `"assistant"` |
| `uuid` | string | Unique message ID |
| `parentUuid` | string\|null | Parent message ID (implements conversation tree) |
| `sessionId` | string | Session UUID (matches filename) |
| `timestamp` | string | ISO 8601 |
| `cwd` | string | Working directory at message time |
| `version` | string | Claude CLI version |
| `isSidechain` | bool | True if this is a forked conversation |
| `message` | object | Role + content array |

**Message content** follows the Anthropic API format: array of `{type: "text", text: "..."}`, `{type: "tool_use", ...}`, `{type: "tool_result", ...}`.

### Conversation Tree Model

`parentUuid` implements a **linked-list / DAG**: each message points to its parent. Branching is supported — multiple messages can share the same `parentUuid`. `isSidechain: true` marks entries that originated as a fork.

### `--resume` and Forking

- `claude --resume <uuid>`: resumes from end of conversation UUID. Replays conversation into model context. Does NOT restore terminal state, only conversational context.
- Claude supports forking from a specific message UUID, creating a new session file with `isSidechain: true` entries.

**Key insight for stapler-squad**: The filename IS the session ID. The cwd-encoded directory IS the project key. stapler-squad should:
1. Store the `claude_conversation_uuid` in its session metadata
2. Pass `--resume <uuid>` when relaunching a cold-resumed session
3. Detect all sessions for a project by scanning `~/.claude/projects/<cwd-encoded>/`

**Source**: Direct inspection of `~/.claude/projects/` + Anthropic Claude Code documentation

---

## Aider Session Persistence

### History Files

Aider writes two files per working directory:

1. **Chat history**: `.aider.chat.history.md` — append-only Markdown log (full conversation, human-readable)
2. **Input history**: `.aider.input.history` — readline history format (user inputs only)

**Resume behavior**: Implicit — running `aider` in the same directory picks up the existing history file. No session IDs, no formal `--resume` flag. Custom history file path via `--chat-history-file`.

**Limitation**: History file is for human reference only. Aider does NOT replay it into model context by default — each session starts fresh. The model context is rebuilt via the repo map (a compact codebase representation injected on every prompt).

**Relevance**: Aider's append-only Markdown log confirms the "append, never overwrite" pattern. The implicit session detection (same dir = same history) is simpler but less robust than Claude's UUID-based model.

**Source**: https://aider.chat/docs/usage/persistent-history.html

---

## iTerm2 Session Restoration

### Daemon Model

When iTerm2 launches a shell, it actually launches `iTermServer` (a background daemon). When iTerm2 quits, `iTermServer` keeps running. On next launch, iTerm2 reconnects to the existing daemon and reattaches to all still-running shell processes — **hot resume, zero state loss**.

This is the same model that tmux uses and what stapler-squad already implements via tmux.

### Cold Restore (After Reboot)

If `iTermServer` was killed (reboot), iTerm2:
1. Reads last-known cwd from `~/Library/Saved Application State/com.googlecode.iterm2.savedState/`
2. Launches a new shell with `cd <last-cwd>` in the startup command

No process state is restored — just geometry and cwd.

### Shell Integration (OSC Escape Sequences)

iTerm2 uses shell hooks emitting escape sequences:
- `OSC 7 ; file://<hostname><path> ST` — report current directory (fires on each prompt)
- `OSC 133 ; A ST` / `B ST` / `C ST` / `D ST` — semantic prompt markers (start/end of prompt and command output)

These allow iTerm2 to track cwd changes without polling.

**Relevance for stapler-squad**: OSC 7 sequences are emitted by modern shells and can be parsed from the PTY output stream (the claude-mux output) to track cwd changes in real time, avoiding periodic polling.

**Source**: https://iterm2.com/documentation-restoration.html

---

## Key Patterns & Takeaways

### Pattern 1: Separate conversation state from process state

Every tool treats these distinctly:
- **Conversation state** (Claude JSONL, Aider .md): Dialogue. Reproducible — can be replayed or resumed via `--resume`.
- **Process state** (tmux scrollback, running programs): Terminal environment. Partially recoverable via cwd + relaunch; full process state is never restored.

**Implication**: Track both layers independently. Claude UUID handles conversation; tmux snapshot handles display state.

### Pattern 2: UUID-named file per session, cwd-encoded directory

Claude's model: `~/.claude/projects/<cwd-encoded>/<uuid>` gives O(1) lookup by project path. The UUID filename IS the session ID.

**Implication**: stapler-squad session records should store `claude_conversation_uuid` and use it for `--resume`. Discovery of existing sessions = scan `~/.claude/projects/<cwd-encoded>/`.

### Pattern 3: parentUuid tree for conversation branching

Claude's `parentUuid` + `isSidechain` = conversation DAG. Fork = new UUID file starting from parent message.

**Implication**: Checkpoint-fork in stapler-squad maps to Claude's native fork mechanism. No new storage format needed for conversation branching.

### Pattern 4: Two-tier hot/cold resume

- **Hot resume** (tmux session alive): reattach — instant, zero state loss
- **Cold restore** (tmux session dead): relaunch `claude --resume <uuid>` — conversation restored, scrollback from persisted file

**Implication**: Check tmux session liveness first; only do cold restore if tmux session is gone.

### Pattern 5: Daemon outlives client

iTerm2's `iTermServer` and Zellij's daemon: session outlives the managing UI. When UI reconnects, reattach instantly.

**Implication**: stapler-squad already implements this via tmux. No additional work needed for the hot-resume tier.

### Pattern 6: OSC 7 for push-based cwd tracking

Instead of polling for cwd changes, parse `OSC 7` escape sequences from the PTY stream.

**Implication**: Add OSC 7 parsing to the scrollback capture pipeline to track cwd changes with zero polling overhead.

---

## Sources

| URL | Description |
|-----|-------------|
| https://github.com/tmux-plugins/tmux-resurrect | tmux-resurrect source |
| https://github.com/tmux-plugins/tmux-continuum | tmux-continuum source |
| https://zellij.dev/news/session-resurrection/ | Zellij session resurrection |
| https://zellij.dev/documentation/session-management | Zellij session management docs |
| https://zellij.dev/documentation/layouts | Zellij KDL layout format |
| https://docs.anthropic.com/en/docs/claude-code/cli-reference | Claude Code CLI reference |
| https://aider.chat/docs/usage/persistent-history.html | Aider session persistence docs |
| https://iterm2.com/documentation-restoration.html | iTerm2 session restoration |
| https://iterm2.com/documentation-shell-integration.html | iTerm2 shell integration / OSC sequences |
| Direct inspection: `~/.claude/projects/` | Local JSONL format verification |
