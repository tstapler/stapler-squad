# PTY Initialization Fix

## Issue Summary

Sessions without git worktrees (directory sessions) and all restored sessions were showing "(error)" status in stapler-squad due to "PTY is not initialized" errors.

## Root Cause

The `RestoreWithWorkDir()` method in `session/tmux/tmux.go` was refactored to remove PTY initialization. The code comment stated "The PTY will be created later when actually needed for attachment", but this never happened. This caused:

1. `SetDetachedSize()` failures - Unable to resize terminal for preview pane
2. `SendKeys()` failures - Unable to send commands to Claude
3. Direct Claude Command Interface failures - Newly implemented system requires PTY

## Solution

Restored PTY initialization in `RestoreWithWorkDir()` using the pattern from commit 8676c3e, with improvements:

### Changes Made

1. **Added `buildAttachCommand()` method** (`session/tmux/tmux.go:188-191`)
   - Creates tmux attach-session command for PTY operations
   - Respects server socket isolation

2. **Updated `RestoreWithWorkDir()`** (`session/tmux/tmux.go:299-344`)
   - Creates PTY connection using `tmux attach-session`
   - Implements graceful degradation if PTY initialization fails
   - Session continues with limited functionality rather than failing completely

3. **Updated test expectations** (`session/tmux/restore_test.go:40-44`)
   - Tests now expect PTY creation during restore
   - Validates attach-session command is used

### Architecture Decisions

Following recommendations from software-planner agent analysis:

**Phase 1 (Implemented)**:
- Eager PTY initialization during restore
- Graceful degradation on failure
- Session works with limited functionality if PTY unavailable

**Future Phases**:
- Phase 2: Lazy PTY initialization with fallback to tmux commands
- Phase 3: Connection pooling for scaling to 100s of sessions

### Code Example

```go
func (t *TmuxSession) RestoreWithWorkDir(workDir string) error {
    // ... session existence check ...

    // Create PTY connection for detached operations
    if t.ptmx == nil {
        ptmx, err := t.ptyFactory.Start(t.buildAttachCommand())
        if err != nil {
            // Graceful degradation - log warning but continue
            log.WarningLog.Printf("PTY initialization failed for session '%s': %v (session will work with limited functionality)", t.sanitizedName, err)
            // Continue without PTY - operations that require it will fail gracefully
        } else {
            t.ptmx = ptmx
            log.InfoLog.Printf("Successfully restored PTY connection for tmux session '%s'", t.sanitizedName)
        }
    }

    t.monitor = newStatusMonitor()
    return nil
}

func (t *TmuxSession) buildAttachCommand() *exec.Cmd {
    return t.buildTmuxCommand("attach-session", "-t", t.sanitizedName)
}
```

## Testing

All tests pass:
- `go test ./session/tmux` - All tmux package tests ✅
- `go test ./session` - All session package tests ✅

## Impact

**Fixed**:
- ✅ Directory sessions (no worktree) now work correctly
- ✅ Preview pane resizing works for all sessions
- ✅ Direct Claude Command Interface has PTY access
- ✅ `SendKeys()` operations work correctly

**Improved**:
- ✅ Graceful degradation if PTY creation fails
- ✅ Better error messages for debugging
- ✅ Session continues with limited functionality vs failing completely

## Validation

Users can verify the fix by:
1. Creating a session without selecting a git worktree
2. Session should show Running status (not error)
3. Preview pane should resize correctly
4. No "PTY is not initialized" warnings in logs

## Related Files

- `session/tmux/tmux.go` - Core PTY initialization logic
- `session/tmux/restore_test.go` - Test expectations updated
- `session/instance.go` - Instance lifecycle using tmux sessions
- `session/pty_access.go` - Direct Claude Command Interface PTY layer

## Future Improvements

As recommended by architectural analysis:

1. **Lazy PTY initialization** - Create on first use rather than eagerly
2. **Connection pooling** - Reuse PTYs across operations
3. **Fallback mechanisms** - Use tmux commands when PTY unavailable
4. **Resource monitoring** - Track PTY usage and health
5. **Graceful cleanup** - Proper PTY lifecycle management at scale
