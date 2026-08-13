# State Synchronization Issue: Tmux Sessions vs State.json

**Date**: October 8, 2025
**Issue**: state.json shows sessions as "Ready" but no corresponding tmux sessions exist

## Current State Analysis

### Actual Running Tmux Sessions
```bash
$ tmux ls
claudesquad_new-session: 1 windows (created Wed Oct  8 11:53:02 2025) (attached)
claudesquad_test-stapler-squad: 1 windows (created Wed Oct  8 11:53:02 2025) (attached)
```

**Only 2 tmux sessions exist**

### State.json Claims
```bash
Total instances: 40
Paused (status=3): 25
Ready (status=1): 15
```

**State.json thinks 15 sessions are Ready, but only 2 tmux sessions exist!**

### Active Claude-Squad Process
```bash
PID 66548: ./stapler-squad --web (started at 8:26PM)
```

Running in web mode, actively managing only 2 sessions.

## The Problem

**State Desynchronization**: The application state file (state.json) is out of sync with actual tmux sessions:

1. **15 "Ready" sessions** in state.json have NO tmux backing
2. **25 "Paused" sessions** are correctly marked (no tmux sessions expected)
3. **Only 2 actual sessions** exist and are running

### Root Cause

The sessions marked as "Ready" (status: 1) likely experienced one of these scenarios:
1. **Tmux sessions crashed** but state wasn't updated
2. **Sessions were killed externally** (e.g., `tmux kill-session`)
3. **System restart** without proper cleanup
4. **Previous stapler-squad crash** that didn't save state

When `FromInstanceData()` loads these sessions:
```go
// session/instance.go:246-250
} else {
    // Only non-paused sessions get Start() called
    if err := instance.Start(false); err != nil {
        return nil, err
    }
}
```

For sessions marked as "Ready" (not Paused), it tries to call `Start(false)` which attempts to restore the tmux session. But if the session no longer exists, this would fail... or does it?

Let me check the restore logic in `RestoreWithWorkDir()`.

## Why Empty Preview/Diff?

For sessions marked as "Ready" (status: 1):
- State.json says: status = 1 (Ready)
- `started` flag would be set to `true` during load attempt
- But `TmuxAlive()` returns `false` because tmux session doesn't exist
- `Preview()` returns empty string due to `!TmuxAlive()` check:

```go
// session/instance.go:678-686
func (i *Instance) Preview() (string, error) {
    if !i.started || i.Status == Paused {
        return "", nil
    }

    // This check fails if tmux session doesn't exist
    if !i.TmuxAlive() {
        return "", nil
    }

    return i.tmuxSession.CapturePaneContent()
}
```

## Verification

### Check if "Ready" sessions can actually provide content
```bash
# These sessions are marked as Ready but have no tmux sessions:
incident-analysis-justin - Status: 1
another-one-for-testing - Status: 1
project-management - Status: 1
mcp-ewn - Status: 1
# ... and 11 more

# None of these have corresponding tmux sessions
```

### tmux Session Name Mapping
The tmux session names should be:
- `claudesquad_incident-analysis-justin`
- `claudesquad_another-one-for-testing`
- etc.

But `tmux ls` shows **only**:
- `claudesquad_new-session`
- `claudesquad_test-stapler-squad`

## Solution Approaches

### Option 1: Mark Orphaned Sessions as Paused
When loading sessions, check if tmux session actually exists:
```go
// After Start() call in FromInstanceData()
if instance.Status != Paused && !instance.TmuxAlive() {
    log.WarningLog.Printf("Instance '%s' marked as Ready but tmux session doesn't exist, marking as Paused", instance.Title)
    instance.Status = Paused
}
```

### Option 2: Attempt to Recreate Sessions on Load
When a "Ready" session has no tmux backing, attempt to restore it:
```go
if instance.Status != Paused && !instance.TmuxAlive() {
    log.InfoLog.Printf("Instance '%s' lost its tmux session, attempting to restore", instance.Title)
    if err := instance.tmuxSession.Start(workDir); err != nil {
        log.ErrorLog.Printf("Failed to restore session, marking as Paused: %v", err)
        instance.Status = Paused
    }
}
```

### Option 3: Health Check on Startup
Add a health check pass after loading all sessions:
```go
// In initializeWithSavedData() after loading instances
for _, instance := range instances {
    if instance.Status != Paused && !instance.TmuxAlive() {
        log.WarningLog.Printf("Health check failed for instance '%s', marking as Paused", instance.Title)
        instance.Status = Paused
        // Save updated state
    }
}
```

## Immediate User Workaround

**For the 2 working sessions (new-session, test-stapler-squad):**
- These should show preview/diff content correctly
- Navigate to them in TUI to verify

**For all other sessions:**
- They're either Paused or in inconsistent "Ready" state
- Need to be resumed with `r` key
- Resume will recreate the tmux session and fix the state

## Files to Investigate

1. `session/tmux/tmux.go` - `RestoreWithWorkDir()` method
   - What happens when restoring a non-existent session?
   - Does it fail silently or throw an error?

2. `session/instance.go:246-250` - `FromInstanceData()` else branch
   - How does it handle `Start()` failures?
   - Does it catch and handle missing tmux sessions?

3. `app/app.go:288-310` - `initializeWithSavedData()`
   - Should add health check after loading
   - Verify all "Ready" sessions have actual tmux backing

## Solution Implemented

**Startup Health Checking** has been implemented in `app/app.go`:

```go
// performStartupHealthCheck validates all loaded sessions and marks orphaned ones as Paused
func (h *home) performStartupHealthCheck() {
	orphanedCount := 0
	checkedCount := 0

	for _, instance := range h.getAllInstances() {
		// Skip paused sessions - they're not expected to have tmux backing
		if instance.Paused() {
			continue
		}

		// Skip sessions that haven't been started
		if !instance.Started() {
			continue
		}

		checkedCount++

		// Check if tmux session actually exists
		if !instance.TmuxAlive() {
			log.WarningLog.Printf("Startup health check: Instance '%s' marked as Ready but tmux session doesn't exist, marking as Paused", instance.Title)
			instance.SetStatus(session.Paused)
			orphanedCount++
		}
	}

	if orphanedCount > 0 {
		log.InfoLog.Printf("Startup health check: Found %d orphaned sessions out of %d checked, marked as Paused", orphanedCount, checkedCount)
		// Save the updated state to persist the Paused status
		if err := h.saveAllInstances(); err != nil {
			log.ErrorLog.Printf("Failed to save instances after health check: %v", err)
		}
	} else if checkedCount > 0 {
		log.InfoLog.Printf("Startup health check: All %d active sessions are healthy", checkedCount)
	}
}
```

### How It Works

1. **On Startup**: After loading sessions from `state.json`, the health check runs automatically
2. **Detection**: Checks each session marked as "Ready" (not Paused) to see if its tmux session actually exists
3. **Correction**: Marks orphaned sessions (no tmux backing) as Paused
4. **Persistence**: Saves the corrected state to `state.json` for future runs

### Expected Behavior After Fix

- **First Run After Implementation**: Will detect 13 orphaned sessions and mark them as Paused
- **Logs**: Will show: "Startup health check: Found 13 orphaned sessions out of 15 checked, marked as Paused"
- **TUI**: Orphaned sessions will now correctly show as Paused instead of misleading "Ready" status
- **Working Sessions**: "new-session" and "test-stapler-squad" remain Ready since they have actual tmux backing

### User Impact

**Before Fix:**
- Empty preview/diff panes for 13 orphaned sessions
- Confusion about which sessions are actually running
- state.json out of sync with tmux reality

**After Fix:**
- Orphaned sessions correctly marked as Paused
- Only actual running sessions (2) show as Ready
- Clear indication which sessions need to be resumed with `r` key
- state.json accurately reflects tmux state

## Next Steps

1. **Immediate**: Restart stapler-squad to trigger the startup health check
2. **Verify**: Check logs for "Startup health check" messages showing orphaned sessions detected
3. **Resume**: Use `r` key to resume any sessions you want to work on
4. **Long-term**: Continue monitoring for any new state desynchronization issues
