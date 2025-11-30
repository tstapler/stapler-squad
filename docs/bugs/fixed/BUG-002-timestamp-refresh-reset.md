# BUG-002: LastMeaningfulOutput Timestamp Reset on Startup [SEVERITY: Medium]

**Status**: ✅ ALREADY FIXED (Verified 2025-01-17)
**Discovered**: During review queue persistence analysis
**Resolution**: Signature-based change detection already implemented in UpdateTerminalTimestamps()
**Impact**: Originally reported - Loss of historical activity information, inaccurate "Last Activity" timestamps after app restart

## Resolution Summary

**Investigation Result**: This bug was **already fixed** by the existing implementation. The recommended fix (signature-based change detection) is fully implemented in `session/instance.go:1693-1742`.

**Verification**: Created comprehensive test suite in `session/instance_timestamp_signature_test.go` which validates:
- ✅ Timestamp preservation when content unchanged (lines 11-77)
- ✅ Historical timestamp preservation during Preview() refresh (lines 79-130)
- ✅ ForceUpdate signature-based change detection (lines 132-183)
- ✅ Review queue refresh scenario with stale timestamps (lines 211-267)

All tests pass, confirming the signature logic works correctly and BUG-002 does not exist in current code.

**Key Implementation Details**:
- `UpdateTerminalTimestamps()` uses `computeContentSignature()` (MurmurHash3) on lines 1708 and 1728
- Only updates `LastMeaningfulOutput` when `signature != i.LastOutputSignature` (lines 1711, 1731)
- `Preview()` correctly calls `UpdateTerminalTimestamps(content, false)` on line 787
- Signature checking is active for both forceUpdate=true and forceUpdate=false paths

**Test Coverage**: 5 new tests covering all scenarios described in original bug report
- TestUpdateTerminalTimestamps_SignatureBasedPreservation
- TestPreview_PreservesHistoricalTimestamps
- TestForceUpdate_SignatureChangeDetection
- TestSignatureStability
- TestReviewQueueRefreshScenario

**Documentation Notes**: Original bug report remains below for historical reference and describes the expected behavior which is now confirmed working.

## Problem Description

The review queue poller's startup routine calls `Preview()` to refresh terminal timestamps, but this **overwrites the persisted `LastMeaningfulOutput` timestamp with the current time**, causing loss of historical activity information. Users see "Last Activity: 30s ago" instead of the actual last activity time (which could be hours or days old).

## Reproduction

1. Create a session and let it idle for 2+ hours
2. Stop claude-squad application (timestamps persist correctly to disk)
3. Restart claude-squad application
4. Review queue poller runs startup refresh via `refreshAllSessionsInQueue()`
5. **BUG**: Session shows "Last Activity: 30s ago" instead of "Last Activity: 2h ago"
6. Historical timestamp is permanently lost (overwritten in memory and then saved)

**Expected Behavior**: Timestamp should preserve historical value if it's more recent than current terminal content.

**Actual Behavior**: Timestamp is unconditionally overwritten with current time on every app startup.

## Root Cause Analysis

### Timestamp Refresh Logic

**Review queue poller startup** (`session/review_queue_poller.go:315-340`):
```go
func (rqp *ReviewQueuePoller) refreshAllSessionsInQueue(ctx context.Context) error {
    for _, inst := range rqp.sessionManager.GetInstances() {
        // Check if timestamps are stale (> 30 seconds old)
        timeSinceLastUpdate := time.Since(inst.LastTerminalUpdate)
        if timeSinceLastUpdate > 30*time.Second && inst.Status == Running {
            // ❌ PROBLEM: This always overwrites the persisted timestamp!
            if _, err := inst.Preview(); err != nil {
                log.ErrorLog.Printf("Failed to refresh timestamps: %v", err)
            }
        }
    }
}
```

### Why This Causes Data Loss

**Scenario**: Session was last active 2 hours ago, then app restarted

1. **On startup**: `LastMeaningfulOutput = 2 hours ago` (loaded from disk correctly)
2. **Poller checks**: `time.Since(2 hours ago) = 2 hours > 30 seconds` → **refresh triggered**
3. **Preview() called**: Fetches current tmux pane content
4. **Timestamp updated**: `LastMeaningfulOutput = time.Now()` (current time)
5. **Data loss**: Historical "2 hours ago" timestamp is overwritten with "now"
6. **UI displays**: "Last Activity: 30s ago" instead of "Last Activity: 2h ago"

### The Core Issue

The `Preview()` method **unconditionally updates timestamps** whenever it's called:

```go
func (i *Instance) Preview() (string, error) {
    content, err := i.tmuxSession.GetPaneContent()
    // ...

    // ❌ Always updates to current time, ignoring persisted historical value
    i.LastTerminalUpdate = time.Now()
    i.LastMeaningfulOutput = time.Now()  // ← Destroys historical timestamp

    return content, nil
}
```

**Problem**: This design assumes timestamps should always reflect "time of last check", not "time of last actual terminal change". This conflicts with the review queue's need to track **historical activity patterns**.

## Files Affected (2 files - fits context boundary)

1. **session/review_queue_poller.go** (line ~326-339) - Fix refresh logic to only update if truly stale
2. **session/instance.go** (line ~700-720) - Fix `Preview()` to preserve timestamps when content unchanged

## Fix Approaches

### Approach 1: Conditional Refresh (Simple Fix - 1 hour)

Only refresh timestamps if content has **actually changed** since last check.

**Fix in `review_queue_poller.go:326`**:
```go
func (rqp *ReviewQueuePoller) refreshAllSessionsInQueue(ctx context.Context) error {
    for _, inst := range rqp.sessionManager.GetInstances() {
        timeSinceLastUpdate := time.Since(inst.LastTerminalUpdate)

        // Only refresh if timestamps are ZERO (never initialized)
        // Do NOT refresh if timestamps are merely "stale" - preserve historical data
        if inst.LastTerminalUpdate.IsZero() && inst.Status == Running {
            log.DebugLog.Printf("[ReviewQueue] Session '%s': Timestamps uninitialized, refreshing via Preview()", inst.Title)

            if _, err := inst.Preview(); err != nil {
                log.ErrorLog.Printf("[ReviewQueue] Session '%s': Failed to initialize timestamps: %v", inst.Title, err)
            }
        } else if !inst.LastTerminalUpdate.IsZero() {
            log.DebugLog.Printf("[ReviewQueue] Session '%s': Preserving historical timestamps (last update: %v)",
                inst.Title, inst.LastTerminalUpdate)
        }
    }
}
```

**Pros**:
- Simple fix (1 file, ~10 lines changed)
- Preserves all historical timestamps correctly
- Zero risk of data loss

**Cons**:
- Timestamps might be stale if session was active during tmux attach (no WebSocket updates)
- Trade-off: Accurate history vs real-time freshness

### Approach 2: Smart Timestamp Update (Better Fix - 2 hours)

Update `Preview()` to only modify timestamps when content **actually changes**.

**Fix in `session/instance.go` (Preview method)**:
```go
func (i *Instance) Preview() (string, error) {
    content, err := i.tmuxSession.GetPaneContent()
    if err != nil {
        return "", fmt.Errorf("failed to get pane content: %w", err)
    }

    // Calculate content signature
    newSignature := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

    // Only update timestamps if content has CHANGED
    if i.LastOutputSignature != newSignature {
        log.DebugLog.Printf("[Instance] Content changed for '%s' - updating timestamps", i.Title)
        i.LastTerminalUpdate = time.Now()
        i.LastMeaningfulOutput = time.Now()
        i.LastOutputSignature = newSignature
    } else {
        log.DebugLog.Printf("[Instance] Content unchanged for '%s' - preserving timestamps (last: %v)",
            i.Title, i.LastMeaningfulOutput)
        // Timestamps remain unchanged - preserves historical data
    }

    return content, nil
}
```

**Pros**:
- Accurate timestamps reflecting **actual activity** rather than polling time
- Works correctly for both WebSocket streaming and direct tmux attach scenarios
- Preserves historical data when content unchanged
- Enables accurate staleness detection

**Cons**:
- Slightly more complex (content hashing overhead)
- Requires testing with various terminal output scenarios

### Recommended Approach

**Use Approach 2 (Smart Timestamp Update)** because:
1. Aligns with existing `LastOutputSignature` infrastructure (already computed elsewhere)
2. Provides accurate activity tracking in all scenarios (WebSocket + tmux attach)
3. Solves the root cause rather than working around it
4. Low overhead (SHA256 hashing is fast, <1ms for typical terminal content)

## Verification Strategy

### Unit Test
```go
func TestPreviewPreservesTimestampsWhenContentUnchanged(t *testing.T) {
    inst := createTestInstance()

    // Set historical timestamp
    historicalTime := time.Now().Add(-2 * time.Hour)
    inst.LastMeaningfulOutput = historicalTime
    inst.LastOutputSignature = "abc123" // Existing signature

    // Mock tmux to return same content (matching signature)
    mockTmux := &MockTmuxSession{
        Content: "unchanged content",
        Signature: "abc123",
    }
    inst.tmuxSession = mockTmux

    // Call Preview()
    _, err := inst.Preview()
    require.NoError(t, err)

    // Verify timestamp NOT updated (historical value preserved)
    assert.Equal(t, historicalTime, inst.LastMeaningfulOutput,
        "Timestamp should be preserved when content unchanged")
}

func TestPreviewUpdatesTimestampsWhenContentChanged(t *testing.T) {
    inst := createTestInstance()

    // Set historical timestamp
    historicalTime := time.Now().Add(-2 * time.Hour)
    inst.LastMeaningfulOutput = historicalTime
    inst.LastOutputSignature = "abc123" // Old signature

    // Mock tmux to return new content (different signature)
    mockTmux := &MockTmuxSession{
        Content: "new content",
        Signature: "def456",
    }
    inst.tmuxSession = mockTmux

    // Call Preview()
    _, err := inst.Preview()
    require.NoError(t, err)

    // Verify timestamp WAS updated
    assert.True(t, inst.LastMeaningfulOutput.After(historicalTime),
        "Timestamp should be updated when content changes")
}
```

### Integration Test
```go
func TestReviewQueuePreservesHistoricalTimestamps(t *testing.T) {
    storage := NewStorage()

    // Create session with 2-hour old activity
    inst := createTestInstance()
    historicalTime := time.Now().Add(-2 * time.Hour)
    inst.LastMeaningfulOutput = historicalTime
    inst.LastOutputSignature = "old_sig"
    storage.SaveInstances([]*Instance{inst})

    // Simulate app restart
    loaded, _ := storage.LoadInstances()
    assert.Equal(t, historicalTime, loaded[0].LastMeaningfulOutput)

    // Start review queue poller (triggers refresh)
    poller := NewReviewQueuePoller(sessionManager, config)
    poller.Start(context.Background())
    time.Sleep(1 * time.Second) // Let startup refresh complete

    // Verify historical timestamp preserved
    refreshed := sessionManager.GetInstance(inst.Title)
    timeDiff := refreshed.LastMeaningfulOutput.Sub(historicalTime)
    assert.True(t, timeDiff < 10*time.Second,
        "Timestamp should remain close to historical value, not reset to current time")
}
```

### Manual Verification
1. Create a session and generate some terminal output
2. Wait 1+ hours without any terminal activity
3. Note the "Last Activity" timestamp in review queue UI
4. Stop and restart claude-squad
5. **Expected**: "Last Activity" shows original timestamp (~1h ago), not "30s ago"
6. Verify by checking session detail view timestamps

## Related Issues

- **BUG-001**: LastAcknowledged not persisted (different issue, shared root cause: incomplete persistence)
- **Timestamp Design**: Confusion between "time of last check" vs "time of last change"

## Impact Assessment

**Severity**: **Medium** (not High)
- **User-Facing**: Yes - misleading timestamps in UI
- **Data Loss**: Yes - historical activity information lost
- **Workaround**: Users can infer activity from terminal content (but timestamps are misleading)
- **Frequency**: Every application restart (100% reproduction)
- **Scope**: Review queue feature only (does not affect core session management)

**Priority**: Should be fixed to ensure review queue provides accurate information, but does not break functionality. Can be addressed after critical bugs and Web UI Story 3.

**Recommended Timeline**: Fix as part of "persistence quick wins" task after BUG-001 fix.

## Prevention Strategy

**Short-term**: Implement Approach 2 (smart timestamp update based on content signature).

**Long-term Design Principles**:
1. **Timestamps should reflect semantic meaning**: "Last activity" means "last actual change", not "last time we checked"
2. **Preserve historical data**: Never overwrite persisted timestamps without verifying content has changed
3. **Use content signatures**: Rely on `LastOutputSignature` to detect actual changes vs unchanged content
4. **Separate concerns**: Distinguish between "last checked" (ephemeral) and "last changed" (persistent)

## Additional Context

### Why Approach 1 is Insufficient

**Problem with "only refresh if zero"**:
- Doesn't help sessions that were active during tmux attach (no WebSocket)
- Timestamps become permanently stale after first check
- Review queue can't accurately detect idle sessions

**Why Approach 2 is better**:
- Handles both WebSocket streaming AND direct tmux attach scenarios
- Timestamps always reflect actual terminal activity
- Accurate staleness detection for review queue logic
- Aligns with existing signature infrastructure

### Performance Considerations

**Content hashing overhead** (Approach 2):
- SHA256 hash of typical terminal content (~10KB): **~0.5ms**
- Review queue refresh cycle: Every 5 seconds per session
- **Total overhead**: 0.5ms per session per 5s = negligible

**Comparison to current approach**:
- Current: Unconditionally update timestamps (free operation)
- Proposed: Hash + compare + conditional update (~0.5ms)
- **Trade-off**: 0.5ms overhead for accurate historical data = **worthwhile**

## Additional Notes

- This bug has existed since review queue feature was introduced
- The `refreshAllSessionsInQueue()` startup routine was added to handle the "direct tmux attach" scenario (no WebSocket updates)
- The solution inadvertently broke the historical timestamp preservation
- Fix requires balancing real-time freshness vs historical accuracy

---

**Bug Tracking ID**: BUG-002
**Related Feature**: Review Queue (session/review_queue.go, session/review_queue_poller.go)
**Fix Complexity**: Medium (2 hours, 2 files, requires content signature logic)
**Fix Risk**: Low (well-tested signature infrastructure already exists)
**Blocked By**: None
**Blocks**: None (degrades UX but doesn't break functionality)
**Related To**: BUG-001 (shared theme: incomplete persistence consideration)
