# Startup Health Check Implementation

**Date**: October 8, 2025
**Issue**: State desynchronization between state.json and actual tmux sessions
**Solution**: Automatic startup health checking to detect and correct orphaned sessions

## Problem Summary

The application's state file (`state.json`) was out of sync with actual running tmux sessions:

- **40 total sessions** in state.json
- **15 sessions marked as "Ready"** (status: 1)
- **Only 2 actual tmux sessions** running (`claudesquad_new-session`, `claudesquad_test-stapler-squad`)
- **13 orphaned sessions** marked as Ready but lacking tmux backing

This caused:
- Empty preview/diff panes when navigating to orphaned sessions
- Confusion about which sessions are actually running
- Misleading session status indicators

## Root Cause

Sessions can lose their tmux backing through:
1. Tmux sessions crashed
2. Sessions killed externally (`tmux kill-session`)
3. System restarts without proper cleanup
4. Application crashes that didn't update state
5. Failed session restoration attempts

When `FromInstanceData()` loads these orphaned sessions, it sets `started=true` but the tmux session doesn't actually exist. The `TmuxAlive()` check returns false, causing `Preview()` to return empty strings.

## Solution Implemented

### Code Changes

**File**: `app/app.go`

**Added** `performStartupHealthCheck()` method (lines 355-390):
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

**Modified** `initializeWithSavedData()` (line 316):
```go
// Perform startup health check to detect orphaned sessions
h.performStartupHealthCheck()
```

### How It Works

1. **Timing**: Runs immediately after loading sessions from storage, before restoring user selection
2. **Detection**: Checks each non-paused session to verify its tmux session actually exists using `TmuxAlive()`
3. **Correction**: Marks orphaned sessions as Paused by setting status to `session.Paused`
4. **Persistence**: Saves the corrected state to `state.json` to prevent re-detection on next startup
5. **Logging**: Reports how many orphaned sessions were found and corrected

### Design Decisions

**Why not attempt recovery?**
- Session restoration can fail for many reasons (missing working directory, invalid program path, etc.)
- Marking as Paused is safer and gives users explicit control via the `r` (resume) key
- Failed recovery attempts would slow down startup

**Why check on startup vs lazily?**
- Provides immediate accurate state representation in the TUI
- Prevents user confusion when first navigating sessions
- Complements existing lazy health checking during attachment

**Why save immediately?**
- Prevents re-detection on every startup
- Maintains state consistency across restarts
- Minimal performance impact (only runs once per orphaned session)

## Expected Behavior

### First Run After Implementation

**Console Output**:
```
[INFO] Startup health check: Found 13 orphaned sessions out of 15 checked, marked as Paused
```

**State Changes**:
- 13 orphaned sessions: Status changed from Ready (1) → Paused (3)
- 2 working sessions: Status remains Ready (new-session, test-stapler-squad)

### Subsequent Runs

**If no new orphaned sessions**:
```
[INFO] Startup health check: All 2 active sessions are healthy
```

**If new orphaned sessions detected** (e.g., after crash):
```
[WARN] Startup health check: Instance 'session-name' marked as Ready but tmux session doesn't exist, marking as Paused
[INFO] Startup health check: Found X orphaned sessions out of Y checked, marked as Paused
```

## User Experience

### Before Fix
- Navigate to session → Empty preview/diff panes
- No indication why content is missing
- 13 sessions misleadingly shown as "Ready"

### After Fix
- Navigate to orphaned session → Shows "Session is paused" message
- Clear visual indication of session state
- Only 2 sessions show as Ready (accurately reflects tmux reality)
- Press `r` to resume any paused session when needed

## Verification Steps

1. **Check Logs** (in `~/.stapler-squad/logs/stapler-squad.log`):
   ```bash
   tail -50 ~/.stapler-squad/logs/stapler-squad.log | grep "Startup health check"
   ```

2. **Verify State File**:
   ```bash
   # Count sessions by status after health check
   jq '.instances[] | .status' ~/.stapler-squad/state.json | sort | uniq -c
   ```

3. **Verify Tmux Sessions**:
   ```bash
   # Should match the number of Ready sessions
   tmux ls | grep claudesquad | wc -l
   ```

4. **TUI Navigation**:
   - Launch stapler-squad
   - Navigate to previously orphaned sessions
   - Verify they show as Paused with appropriate message
   - Navigate to working sessions (new-session, test-stapler-squad)
   - Verify preview/diff panes show content

## Testing

The health check has been tested to handle:
- ✅ Sessions with tmux backing (correctly remains Ready)
- ✅ Sessions without tmux backing (correctly marked as Paused)
- ✅ Already paused sessions (skipped, no duplicate processing)
- ✅ Sessions that haven't been started (skipped)
- ✅ Empty instance list (no crashes)
- ✅ State persistence after health check

## Future Enhancements

**Potential improvements** (not currently implemented):

1. **Automatic Recovery Attempt**: Try `Start(false)` before marking as Paused
   - Risk: Could slow startup if many sessions fail to restore
   - Benefit: Would auto-recover sessions with transient failures

2. **Health Check Report in TUI**: Show startup health check results in status bar
   - Benefit: Provides immediate user feedback about state corrections

3. **Periodic Background Health Checks**: Already implemented via `startBackgroundHealthChecker()`
   - Runs every 5 minutes if `PerformBackgroundHealthChecks` config is enabled
   - Complements startup health checking

4. **Session Recovery Queue**: Track failed sessions for user review
   - Could integrate with existing review queue system
   - Benefit: Helps users identify and fix problematic sessions

## Related Files

- `app/app.go` - Implementation of startup health check
- `session/instance.go` - `TmuxAlive()` method used for detection
- `docs/state-synchronization-issue.md` - Detailed problem analysis
- `docs/tui-empty-preview-verification.md` - Original investigation

## Conclusion

The startup health check provides automatic detection and correction of state desynchronization between `state.json` and actual tmux sessions. This ensures the TUI accurately reflects which sessions are actually running, eliminating confusion and empty preview/diff panes for orphaned sessions.

Users can now trust the session status indicators, and orphaned sessions are clearly marked for manual resumption when needed.
