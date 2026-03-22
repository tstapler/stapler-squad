# TUI Empty Preview/Diff - Current State Verification

**Investigation Date**: October 8, 2025
**Status**: Root Cause Confirmed

## Current State Analysis

### Session Status Check
```bash
# All sessions in state.json have status=3 (Paused)
✓ fix-erndering-makdrown: Paused (status: 3)
✓ fix-achivre-erorr: Paused (status: 3)
✓ brainstorming: Paused (status: 3)
✓ gradle-ueqstions: Paused (status: 3)
```

### Worktree Directory Check
```bash
# Worktree directories exist on disk
✓ /Users/tylerstapler/.stapler-squad/worktrees/fix-erndering-makdrown_185b1adf34df22c0/ EXISTS
```

### Tmux Session Check
```bash
# No tmux sessions exist for paused sessions
✓ Only found: claudesquad_new-session, claudesquad_test-stapler-squad
✗ Missing: claudesquad_fix-erndering-makdrown (and all other paused sessions)
```

## Root Cause Confirmed

**Why Preview/Diff Panes are Empty:**

1. **Sessions are Paused (Status = 3)**
   - All user sessions have `status: 3` (Paused status)
   - Could have been manually paused or auto-paused in the past

2. **No Active Tmux Sessions**
   - Paused sessions have `started=true` but no actual tmux session running
   - `TmuxAlive()` returns false because `tmuxSession.DoesSessionExist()` fails

3. **Preview() Returns Empty String**
   ```go
   // session/instance.go:678-680
   func (i *Instance) Preview() (string, error) {
       if !i.started || i.Status == Paused {
           return "", nil  // ← Returns empty for paused sessions
       }
   ```

4. **Diff Operations Also Fail**
   - Same logic applies to diff pane
   - Returns empty/fallback content for paused sessions

## Code Path Analysis

### TUI Access Path
```
User navigates to session in TUI
  ↓
UpdatePreview() / UpdateDiff() called
  ↓
Async workers check: instance.Paused() || !instance.TmuxAlive()
  ↓
Returns fallback/empty content
  ↓
Empty preview/diff panes displayed
```

### Why It Happens
```go
// session/instance.go:117-122
func (i *Instance) TmuxAlive() bool {
    if i.Status == Paused || !i.started || i.tmuxSession == nil {
        return false  // ← Fails for paused sessions
    }
    return i.tmuxSession.DoesSessionExist()
}
```

## Solution: Resume Sessions

### Step 1: Verify Current State
```bash
# Check session statuses
jq '.instances[] | {title, status, paused: (.status == 3)}' ~/.stapler-squad/state.json | head -20

# Check tmux sessions
tmux ls | grep claudesquad

# Verify worktrees exist
ls -la ~/.stapler-squad/worktrees/ | grep fix-erndering-makdrown
```

### Step 2: Resume Sessions in TUI

**In the stapler-squad TUI:**

1. **Launch stapler-squad**:
   ```bash
   ./stapler-squad
   ```

2. **Show all sessions** (including paused):
   - Press `f` to toggle filter
   - This shows paused sessions

3. **Select a paused session**:
   - Navigate with arrow keys
   - Select "fix-erndering-makdrown" or any paused session

4. **Resume the session**:
   - Press `r` to resume
   - This will:
     - Verify worktree exists (✓ already confirmed)
     - Start tmux session for the worktree
     - Set status to Running

5. **Verify preview/diff work**:
   - Navigate back to the resumed session
   - Check if preview pane shows terminal output
   - Check if diff pane shows git changes

### Step 3: Expected Behavior After Resume

**Before Resume:**
- Preview pane: Empty or fallback message
- Diff pane: Empty or fallback message
- Status: Paused (3)

**After Resume:**
- Preview pane: Shows terminal output from tmux capture-pane
- Diff pane: Shows git diff stats and content
- Status: Running (0)
- Tmux session: `claudesquad_fix-erndering-makdrown` exists

## Why Previous Fixes Didn't Work

The previous attempts focused on:
1. Async preview/diff rendering logic
2. Worker channel processing
3. Result processing timing

**These were all correct and working!** The issue was at a different layer:
- The `Preview()` and diff methods themselves return empty for paused sessions
- No amount of async rendering fixes will help if the underlying data source returns empty

## Verification Commands

### Before Resuming
```bash
# Should show status=3 (Paused)
jq '.instances[] | select(.title == "fix-erndering-makdrown") | {title, status}' \
  ~/.stapler-squad/state.json

# Should NOT find tmux session
tmux ls | grep "fix-erndering-makdrown" || echo "No tmux session found"
```

### After Resuming
```bash
# Should show status=0 (Running)
jq '.instances[] | select(.title == "fix-erndering-makdrown") | {title, status}' \
  ~/.stapler-squad/state.json

# Should find tmux session
tmux ls | grep "fix-erndering-makdrown" && echo "Tmux session found!"
```

## Technical Details

### Session Loading Process
```go
// app/app.go:288-310 - initializeWithSavedData()
// Loads sessions from storage but does NOT call instance.Start()
for _, instance := range instances {
    h.addInstanceToList(instance)  // ← No Start() call
}
```

### Paused Session Handling
```go
// session/instance.go:232-246 - FromInstanceData()
if instance.Paused() {
    instance.started = true  // ← Sets flag
    // Creates tmux Go object but NOT actual tmux session
    instance.tmuxSession = tmux.NewTmuxSessionWithPrefix(...)
} else {
    // Only non-paused sessions get Start() called
    if err := instance.Start(false); err != nil {
        return nil, err
    }
}
```

### Why This Design Exists
- **Performance**: Starting all tmux sessions on app startup would be slow
- **Resource Management**: Paused sessions don't need active tmux processes
- **Intent Preservation**: Sessions remain paused until user explicitly resumes them

## Next Steps

1. **User Action**: Resume paused sessions using `r` key in TUI
2. **Verify Fix**: Check if preview/diff panes populate after resume
3. **Report Back**: If still empty after resume, we need to investigate further
4. **Alternative**: If many sessions need resuming, consider adding "resume all paused" feature

## Files Examined

- `/Users/tylerstapler/.stapler-squad/state.json` - Session state (32.9MB, all sessions paused)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/session/instance.go` - Preview() and TmuxAlive() logic
- `/Users/tylerstapler/IdeaProjects/stapler-squad/app/app.go` - Session loading logic
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/preview.go` - Async preview worker (working correctly)
- `/Users/tylerstapler/IdeaProjects/stapler-squad/ui/diff.go` - Async diff worker (working correctly)

## Related Documentation

- [TUI Empty Preview Root Cause](./tui-empty-preview-root-cause.md) - Comprehensive root cause analysis
- Previous async rendering fixes were correct but addressed a different issue
