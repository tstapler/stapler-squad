# TUI Empty Preview/Diff Root Cause Analysis

## Problem
Preview and diff panes are empty when viewing sessions through the TUI interface, but Web UI shows sessions.

## Root Cause

### Session Loading Issue

When sessions are loaded from storage, they go through `FromInstanceData()` in `session/instance.go`:

```go
// Line 139-146: Auto-pause check
if !instance.Paused() && instance.gitWorktree != nil {
    worktreePath := instance.gitWorktree.GetWorktreePath()
    if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
        // Worktree has been deleted, mark instance as paused
        instance.Status = Paused  // ← Sessions auto-paused if worktree missing
    }
}

// Line 148-166: Branch on Paused status
if instance.Paused() {
    instance.started = true  // ← Sets flag manually
    // Creates tmux session OBJECT but doesn't start actual tmux session
    instance.tmuxSession = tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix)
} else {
    if err := instance.Start(false); err != nil {  // ← Properly restores tmux session
        return nil, err
    }
}
```

### Preview() Returns Empty

When TUI calls `instance.Preview()` (line 594-605):

```go
func (i *Instance) Preview() (string, error) {
    if !i.started || i.Status == Paused {
        return "", nil  // ← Returns empty
    }

    if !i.TmuxAlive() {  // ← Checks if tmux session exists in tmux server
        return "", nil  // ← Returns empty
    }

    return i.tmuxSession.CapturePaneContent()
}
```

### The Cascade

1. **Sessions saved with worktree paths** that no longer exist
2. **On load**: `FromInstanceData()` detects missing worktree → auto-pauses session
3. **Paused sessions get**:
   - `started = true` ✓
   - `tmuxSession` Go object created ✓
   - But NO actual tmux session in tmux server ✗
4. **When TUI calls `Preview()`**:
   - Check `!i.started` → passes (started=true)
   - Check `i.Status == Paused` → returns empty string OR
   - Check `!i.TmuxAlive()` → returns empty string (no actual tmux session exists)
5. **Result**: Empty preview/diff panes

## Why Web UI Might Work

The Web UI uses different access patterns:

### Web UI Session List
- Shows session metadata (title, status, category, etc.)
- Doesn't require terminal content
- Works for paused sessions

### Web UI Terminal Streaming
Uses `StreamTerminal()` RPC which has same checks:
```go
if !instance.Started() {
    return error("session not started")
}
if instance.Paused() {
    return error("session is paused")  // ← Should also fail!
}
```

**So Web UI terminal streaming should ALSO fail for paused sessions.**

The user might be:
- Only viewing session list (not terminal content)
- Looking at different sessions that aren't paused
- Experiencing a different issue

## Solution

### Immediate Fix: Resume Sessions

Sessions need to be resumed before content can be displayed. In the TUI:

1. **Check session status**: Press `f` to show all sessions including paused
2. **Resume paused sessions**: Select a paused session and press `r` to resume
3. **This will**:
   - Recreate the git worktree
   - Start the tmux session
   - Enable preview/diff content

### Code Changes Made

We added comprehensive handling for paused sessions in the async preview/diff workers:

1. **`ui/preview.go`** (lines 149-195):
   - Detects paused sessions
   - Shows appropriate fallback message: "Session is paused. Press 'r' to resume."
   - Differentiates between worktree-based and directory-based sessions

2. **`ui/diff.go`** (lines 210-232):
   - Detects paused sessions
   - Shows fallback message: "Session is paused - Diff unavailable - worktree directory not found"

3. **`ui/tabbed_window.go`**:
   - Removed early returns for paused sessions
   - Allows async workers to handle paused state

### But Why Still Empty?

If preview/diff are STILL empty after these changes, it means:

1. **Sessions aren't reaching the async workers** - Debug logging added would show if workers are being called
2. **Result processing isn't working** - The `ProcessResults()` ticker might not be firing
3. **Different issue entirely** - Sessions might not be paused but have another problem

## Debugging Steps

### 1. Check Session Status
```bash
# Look at your state file
cat ~/.claude-squad/state.json | jq '.instances[] | {title, status}'
```

Expected statuses:
- `0` = Running
- `1` = Ready
- `2` = Loading
- `3` = Paused ← This is your problem
- `4` = NeedsApproval

### 2. Check Worktree Paths
```bash
# See if worktrees exist
cat ~/.claude-squad/state.json | jq '.instances[] | {title, worktree: .worktree.worktree_path}'
```

Then check if those paths exist:
```bash
ls -la ~/.claude-squad/worktrees/
```

### 3. Check Debug Logs
```bash
# Run TUI and check logs in another terminal
tail -f ~/.claude-squad/logs/debug.log | grep -i "preview\|diff"
```

Look for:
- `[PREVIEW] UpdateContentAsync called`
- `[PREVIEW] Worker processing request`
- `[PREVIEW] ProcessResults: received result`
- `[DIFF] UpdateDiffAsync called`
- Similar debug messages

### 4. Verify Tmux Sessions
```bash
# List all tmux sessions
tmux ls

# Should see sessions like: claudesquad_session-name: ...
```

## Expected Behavior After Fix

With the async fixes in place:

### For Paused Sessions
- Preview pane should show:
  ```
  Session is paused. Press 'r' to resume.

  Branch: fix-rendering-markdown | Directory not found
  ```
- Diff pane should show:
  ```
  Session is paused

  Diff unavailable - worktree directory not found
  ```

### For Active Sessions
- Preview pane shows terminal output via `tmux capture-pane`
- Diff pane shows git diff stats and content

## Next Steps

1. **Resume a session**: Select any session and press `r`
2. **Check if preview/diff work**: After resume completes, preview/diff should populate
3. **If still empty**: Run debug logging steps above and examine output
4. **Report findings**: Share debug log output showing what happens during navigation

## Architecture Notes

### TUI Access Path
```
TUI Navigation
  ↓
instanceChanged()
  ↓
UpdatePreview(instance) / UpdateDiff(instance)
  ↓
preview.UpdateContentAsync() / diff.UpdateDiffAsync()
  ↓
Background worker: processPreviewRequest() / processDiffRequest()
  ↓
Checks: instance.Paused() || !instance.TmuxAlive()
  ↓
If paused: Send fallback content
If alive: Call instance.Preview() → tmux capture-pane
  ↓
Send result to previewResultCh / diffResultCh
  ↓
ProcessResults() ticker (50ms) reads channel
  ↓
Updates preview/diff state
  ↓
View() renders content
```

### Web UI Access Path
```
Web UI Connect
  ↓
StreamTerminal RPC
  ↓
Checks: instance.Started() && !instance.Paused()
  ↓
instance.GetPTYReader()
  ↓
Direct PTY file descriptor access
  ↓
Stream output to WebSocket
```

**Key Difference**: TUI uses `tmux capture-pane` (requires live tmux session), Web UI uses direct PTY access (different requirements).

## Verification

To confirm the fix works, after resuming a session:

```bash
# Check tmux session exists
tmux ls | grep claudesquad

# Check worktree exists
ls -la ~/.claude-squad/worktrees/

# Navigate to session in TUI and verify preview/diff show content
```
