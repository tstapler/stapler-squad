# Pitfalls: Switching Session Programs

## Current State Summary

The backend (`UpdateSession` in `server/services/session_service.go:1429-1441`) already handles program changes correctly: it updates `instance.Program`, persists it via `SaveInstances`, and calls `instance.Restart(true)` when the session is `Active`. The bug is almost entirely a **UI gap** — there is no frontend surface that calls `updateSession` with a `program` field.

---

## 1. tmux Interaction Risks

**Restart kills the live terminal.** `Restart(preserveOutput: true)` captures the pane content, kills the tmux session, then relaunches. This means any in-progress agent work is terminated immediately. For long-running agentic tasks, a program change with no warning dialog is destructive.

**`buildLaunchCommand` bakes program-specific flags.** Flags like `--resume`, `--mcp-config`, `--append-system-prompt`, `--dangerously-skip-permissions` are only appended when `strings.Contains(program, "claude")`. Switching from `claude` to `aider` silently drops all Claude-only flags — correct behavior, but the reverse (switching *to* claude on a session whose `claudeSession.ConversationUUID` is set) will attempt `--resume`, which may fail if the history is stored in a path that no longer maps to this session.

**`initTmuxSession` reuses an existing session object.** The `if i.pm().HasSession() { return }` guard in `initTmuxSession` means a program change on a non-started session (e.g. `Stopped` but never fully torn down) may reuse the old session object with the old program string embedded in it.

---

## 2. State Consistency Issues

**Stopped/Paused/Hibernated sessions: program is saved but not applied.** The restart guard (`if instance.Status == session.Active`) means program changes on non-Active sessions are persisted to disk but the change does not take effect until the next manual resume/restart. Users will not know the change is "pending" vs "applied."

**Race between persist and restart.** `instance.Program` is mutated in-memory at line 1431, then `Restart` is called at 1436, then `SaveInstances` is called at 1533. If `Restart` succeeds but `SaveInstances` fails, the running tmux process uses the new program but the stored state still shows the old program. On next load the session will revert.

**`ErrCannotRestart` when `i.started == false`.** `Restart` returns `ErrCannotRestart` if the instance has never been started (e.g. a session in `Creating` or `Stopped` state that was never launched). The `UpdateSession` handler propagates this as a `CodeInternal` error, giving the user no actionable message.

---

## 3. Test Gaps

There are **zero tests** for `UpdateSession` with a `program` field. All existing `TestUpdateSession_*` tests cover tags, title, status transitions, and conflict detection — none test:
- Program change on an Active session (verifying restart occurs)
- Program change on a Stopped session (verifying it saves without restart)
- Program change to an empty string (the guard `*req.Msg.Program != ""` silently no-ops)
- Program change when `Restart` returns an error

---

## 4. UX Confusion Points

**There is no UI to change a session's program.** `SessionWizard` (creation only), `ResumeSessionModal` (shows program read-only), `SessionDetailBar`, and `SessionPeekModal` have no editable program field. The wire is complete on the backend and in the RPC layer but no frontend component surfaces it.

**Program is read-only in `ResumeSessionModal`.** The resume flow shows the current program as context-only text. Users see it but cannot change it — and there is no hint that changing it is possible.

**"System default" empty-string handling is a footgun.** If a user tries to "clear" the program to use the system default by sending an empty string, the guard `*req.Msg.Program != ""` silently ignores the request. The backend never exposes a way to reset to the empty/default value via `UpdateSession`.

**No confirmation dialog before restart.** Changing a program on an Active session immediately kills and restarts the tmux session. There is no frontend warning about work loss, and no acknowledgment in the UI that the session was restarted (only a log-level event on the backend).

---

## 5. Edge Cases

**`bash`/terminal sessions:** Switching to `bash` sets `isTerminalSession` in the frontend (in `SessionWizard`), which hides AI-specific form fields. If a program is changed backend-only (via API), the frontend session list/detail will show `bash` but users will be confused about why AI features disappeared.

**Claude session UUID after program switch.** When switching away from `claude`, the stored `claudeSession.ConversationUUID` is not cleared. If the user later switches back to `claude`, the restart will attempt `--resume <UUID>` — this may succeed or silently fail depending on whether the history file still exists.

**Paused worktree sessions.** `Restart` for a paused session recreates the worktree and clears the UUID (lines 1205-1219). A program change on a paused session that triggers this path would thus also clear the Claude conversation history — a side-effect users would not expect from a "change program" action.
