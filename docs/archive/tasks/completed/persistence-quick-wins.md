# Persistence Layer Quick Wins - Bug Fixes

**Epic**: Persistence Layer Reliability
**Status**: Ready to Start
**Priority**: P2 - After Web UI Story 3, before Story 4
**Total Estimated Effort**: 3 hours (2 tasks)
**Progress**: 0% complete

---

## Epic Overview

This epic addresses **critical data loss bugs** discovered during persistence layer analysis. These are **quick wins** (1-2 hours each) that fix broken functionality with minimal risk and high impact.

**Strategic Value**:
- **Restores broken features**: Review queue snooze functionality
- **Prevents data loss**: Preserves historical activity timestamps
- **Low risk**: Trivial code changes (3-10 lines per fix)
- **High impact**: Fixes user-facing issues that degrade UX

**Related Bugs**:
- BUG-001 [HIGH]: LastAcknowledged field not persisted
- BUG-002 [MEDIUM]: LastMeaningfulOutput timestamp reset on startup
- BUG-003 [LOW]: 34MB state file (deferred - investigation required)

---

## Story 1: Fix LastAcknowledged Persistence (1 hour)

**Goal**: Persist review queue snooze/dismiss actions across application restarts

**Current Behavior**: When users dismiss a session from review queue, it immediately re-appears after restarting the application (snooze functionality completely broken).

**Root Cause**: `LastAcknowledged` field exists in `Instance` struct but is **not included** in `InstanceData` serialization struct.

**Success Criteria**:
- Dismissed sessions remain dismissed for configured snooze duration (4 hours default)
- Snooze state survives application restarts
- Backward compatible with existing state.json files
- Unit and integration tests validate persistence

---

### Task 1.1: Add LastAcknowledged to Persistence Layer (1h)

**Scope**: Add missing field to serialization structs and methods

**Files** (3 files, ~600 lines total context):
- `session/storage.go` (line ~56) - Add field to InstanceData struct
- `session/instance.go` (line ~166) - Add field to ToInstanceData() method
- `session/instance.go` (line ~210+) - Add field to FromInstanceData() method

**Context**:
- `LastAcknowledged` tracks when user last dismissed/snoozed a session
- Review queue uses this to prevent re-adding dismissed sessions too soon
- Field already exists in `Instance` struct but never persisted
- Uses `time.Time` type with `omitempty` JSON tag → backward compatible

**Implementation**:

**Step 1**: Add to InstanceData struct (`session/storage.go:56`):
```go
type InstanceData struct {
    // ... existing fields ...
    LastViewed time.Time `json:"last_viewed,omitempty"`

    // Review queue snooze/dismiss tracking
    // Tracks when user last acknowledged (dismissed/snoozed) this session
    // Used to prevent re-adding sessions to review queue too soon after dismissal
    LastAcknowledged time.Time `json:"last_acknowledged,omitempty"`
}
```

**Step 2**: Serialize field (`session/instance.go:~166`):
```go
func (i *Instance) ToInstanceData() InstanceData {
    data := InstanceData{
        // ... existing field copies ...
        LastViewed:           i.LastViewed,
        LastAcknowledged:     i.LastAcknowledged,  // ← Add this line
    }
    return data
}
```

**Step 3**: Deserialize field (`session/instance.go:~210+`):
```go
func FromInstanceData(data InstanceData) (*Instance, error) {
    inst := &Instance{
        // ... existing field assignments ...
        LastViewed:       data.LastViewed,
        LastAcknowledged: data.LastAcknowledged,  // ← Add this line
    }
    // ... rest of function ...
    return inst, nil
}
```

**Step 4**: Add tests (`session/storage_test.go`):
```go
func TestLastAcknowledgedPersistence(t *testing.T) {
    // Create instance with LastAcknowledged set
    inst := &Instance{
        Title: "test-session",
        LastAcknowledged: time.Now().Add(-2 * time.Hour),
    }

    // Serialize to InstanceData
    data := inst.ToInstanceData()

    // Verify field is present in serialized form
    assert.False(t, data.LastAcknowledged.IsZero(),
        "LastAcknowledged should be serialized")
    assert.Equal(t, inst.LastAcknowledged, data.LastAcknowledged,
        "LastAcknowledged value should match")

    // Deserialize back to Instance
    restored, err := FromInstanceData(data)
    require.NoError(t, err)

    // Verify field survives round-trip
    assert.Equal(t, inst.LastAcknowledged, restored.LastAcknowledged,
        "LastAcknowledged should survive serialization round-trip")
}

func TestReviewQueueDismissPersistence(t *testing.T) {
    storage := NewStorage()

    // Create session and dismiss from review queue
    inst := createTestInstance()
    inst.LastAcknowledged = time.Now()
    err := storage.SaveInstances([]*Instance{inst})
    require.NoError(t, err)

    // Simulate app restart - reload from disk
    loaded, err := storage.LoadInstances()
    require.NoError(t, err)
    require.Len(t, loaded, 1)

    // Verify dismiss timestamp persisted
    assert.False(t, loaded[0].LastAcknowledged.IsZero(),
        "LastAcknowledged should survive restart")

    // Verify review queue respects dismiss (within snooze window)
    timeSinceAck := time.Since(loaded[0].LastAcknowledged)
    assert.True(t, timeSinceAck < 4*time.Hour,
        "Should still be within default 4-hour snooze window")
}
```

**Success Criteria**:
- Code compiles without errors
- All existing tests still pass
- New tests verify field persistence
- Backward compatible with existing state.json (omitempty tag)
- Manual test: Dismiss session, restart app, session remains dismissed

**Testing**: Comprehensive unit and integration tests included above

**Dependencies**: None

**Status**: Pending

---

## Story 2: Fix Timestamp Refresh Logic (2 hours)

**Goal**: Preserve historical activity timestamps when terminal content hasn't changed

**Current Behavior**: After restarting the application, sessions show "Last Activity: 30s ago" instead of the actual historical timestamp (e.g., "2 hours ago"). Historical data is permanently lost.

**Root Cause**: `Preview()` method unconditionally updates timestamps to current time, even when terminal content is unchanged. Review queue poller calls `Preview()` on startup to refresh stale timestamps, inadvertently destroying historical data.

**Success Criteria**:
- Historical timestamps preserved when terminal content unchanged
- Timestamps updated only when actual terminal activity detected
- Accurate activity tracking for both WebSocket streaming and direct tmux attach
- Backward compatible with existing timestamp logic

---

### Task 2.1: Implement Content-Based Timestamp Update (2h)

**Scope**: Modify `Preview()` to only update timestamps when terminal content actually changes

**Files** (2 files, ~500 lines total context):
- `session/instance.go` (line ~700-720) - Modify Preview() method
- `session/review_queue_poller.go` (line ~326-339) - Update refresh logic (optional)

**Context**:
- `Preview()` fetches current tmux pane content and updates timestamps
- `LastOutputSignature` (SHA256 hash) detects actual content changes
- Review queue poller calls `Preview()` on startup for sessions with stale timestamps
- Need to preserve historical timestamps when content hasn't changed since last check

**Implementation**:

**Step 1**: Modify Preview() to use content signature (`session/instance.go:~700-720`):

```go
func (i *Instance) Preview() (string, error) {
    // Fetch current terminal content from tmux
    content, err := i.tmuxSession.GetPaneContent()
    if err != nil {
        return "", fmt.Errorf("failed to get pane content: %w", err)
    }

    // Calculate content signature (SHA256 hash of terminal content)
    newSignature := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

    // Only update timestamps if content has CHANGED since last check
    // This preserves historical timestamps when content is unchanged
    if i.LastOutputSignature != newSignature {
        // Content changed - update timestamps to reflect actual activity
        log.DebugLog.Printf("[Instance] Terminal content changed for '%s' - updating timestamps", i.Title)
        i.LastTerminalUpdate = time.Now()
        i.LastMeaningfulOutput = time.Now()
        i.LastOutputSignature = newSignature
    } else {
        // Content unchanged - preserve existing timestamps (historical data)
        log.DebugLog.Printf("[Instance] Terminal content unchanged for '%s' - preserving timestamps (last activity: %v)",
            i.Title, i.LastMeaningfulOutput)
        // Note: LastTerminalUpdate and LastMeaningfulOutput remain unchanged
    }

    return content, nil
}
```

**Step 2** (Optional): Improve poller refresh logic (`session/review_queue_poller.go:~326-339`):

```go
func (rqp *ReviewQueuePoller) refreshAllSessionsInQueue(ctx context.Context) error {
    for _, inst := range rqp.sessionManager.GetInstances() {
        // Only refresh timestamps if they're truly uninitialized (zero value)
        // Don't refresh merely "stale" timestamps - Preview() will preserve historical data
        if inst.LastTerminalUpdate.IsZero() && inst.Status == Running && inst.TmuxAlive() {
            log.DebugLog.Printf("[ReviewQueue] Session '%s': Timestamps uninitialized, refreshing via Preview()", inst.Title)

            if _, err := inst.Preview(); err != nil {
                log.ErrorLog.Printf("[ReviewQueue] Session '%s': Failed to initialize timestamps: %v", inst.Title, err)
                // Continue processing even if Preview fails
            }
        } else if !inst.LastTerminalUpdate.IsZero() {
            // Timestamps already initialized - Preview() will be called by normal polling
            // and will preserve historical data if content unchanged
            log.DebugLog.Printf("[ReviewQueue] Session '%s': Timestamps initialized (last: %v), will refresh via normal polling",
                inst.Title, inst.LastTerminalUpdate)
        }
    }
    return nil
}
```

**Step 3**: Add tests (`session/instance_test.go`):

```go
func TestPreviewPreservesTimestampsWhenContentUnchanged(t *testing.T) {
    // Create instance with historical timestamp
    inst := createTestInstance()
    historicalTime := time.Now().Add(-2 * time.Hour)
    inst.LastMeaningfulOutput = historicalTime
    inst.LastOutputSignature = "abc123def456" // Existing content signature

    // Mock tmux to return same content (matching signature)
    mockTmux := &MockTmuxSession{
        PaneContent: "unchanged terminal content",
    }
    inst.tmuxSession = mockTmux

    // Calculate signature for mock content (should match existing signature)
    expectedSig := fmt.Sprintf("%x", sha256.Sum256([]byte("unchanged terminal content")))
    inst.LastOutputSignature = expectedSig // Simulate previous content

    // Call Preview()
    content, err := inst.Preview()
    require.NoError(t, err)
    assert.Equal(t, "unchanged terminal content", content)

    // Verify timestamp NOT updated (historical value preserved)
    assert.Equal(t, historicalTime, inst.LastMeaningfulOutput,
        "Timestamp should be preserved when content unchanged")
    assert.Equal(t, expectedSig, inst.LastOutputSignature,
        "Signature should remain unchanged")
}

func TestPreviewUpdatesTimestampsWhenContentChanged(t *testing.T) {
    // Create instance with historical timestamp
    inst := createTestInstance()
    historicalTime := time.Now().Add(-2 * time.Hour)
    inst.LastMeaningfulOutput = historicalTime
    inst.LastOutputSignature = "old_signature_abc123"

    // Mock tmux to return NEW content (different signature)
    mockTmux := &MockTmuxSession{
        PaneContent: "NEW terminal content with changes",
    }
    inst.tmuxSession = mockTmux

    // Call Preview()
    _, err := inst.Preview()
    require.NoError(t, err)

    // Verify timestamp WAS updated (content changed)
    assert.True(t, inst.LastMeaningfulOutput.After(historicalTime),
        "Timestamp should be updated when content changes")

    // Verify signature was updated
    newSignature := fmt.Sprintf("%x", sha256.Sum256([]byte("NEW terminal content with changes")))
    assert.Equal(t, newSignature, inst.LastOutputSignature,
        "Signature should be updated to reflect new content")
}

func TestReviewQueuePreservesHistoricalTimestamps(t *testing.T) {
    storage := NewStorage()

    // Create session with 2-hour old activity
    inst := createTestInstance()
    historicalTime := time.Now().Add(-2 * time.Hour)
    inst.LastMeaningfulOutput = historicalTime
    inst.LastOutputSignature = "historical_signature"
    storage.SaveInstances([]*Instance{inst})

    // Simulate app restart
    loaded, err := storage.LoadInstances()
    require.NoError(t, err)
    assert.Equal(t, historicalTime, loaded[0].LastMeaningfulOutput,
        "Historical timestamp should be loaded from disk")

    // Start review queue poller (triggers startup refresh)
    poller := NewReviewQueuePoller(sessionManager, config)
    err = poller.refreshAllSessionsInQueue(context.Background())
    require.NoError(t, err)

    // Verify historical timestamp preserved (within tolerance for test execution time)
    refreshed := sessionManager.GetInstance(inst.Title)
    timeDiff := refreshed.LastMeaningfulOutput.Sub(historicalTime).Abs()
    assert.True(t, timeDiff < 10*time.Second,
        "Timestamp should remain close to historical value, not reset to current time")
}
```

**Success Criteria**:
- Code compiles without errors
- All existing tests still pass
- New tests verify timestamp preservation logic
- Manual test: Session idle for 1+ hours, restart app, "Last Activity" shows historical time
- Performance: SHA256 hashing adds <1ms overhead per Preview() call (negligible)

**Testing**: Comprehensive unit and integration tests included above

**Dependencies**: None (uses existing `LastOutputSignature` infrastructure)

**Status**: Pending

---

## Dependencies and Sequencing

### Task Dependencies

**Story 1** (LastAcknowledged):
- No dependencies - can start immediately
- Unblocked by: Nothing
- Blocks: Nothing (independent fix)

**Story 2** (Timestamp Refresh):
- No dependencies - can start immediately
- Unblocked by: Nothing
- Blocks: Nothing (independent fix)

**Parallel Execution**: Both stories can be implemented in parallel (no shared files or dependencies).

### Integration with Other Work

**Relationship to TODO.md priorities**:

1. **P1 - Web UI Story 3**: Unblocked - continue as highest priority
2. **P2 - Persistence Quick Wins** (this epic): After Story 3, before Story 4
3. **P3 - SQLite Migration**: Deferred until Web UI MVP complete

**Recommended Sequencing**:
1. Complete Web UI Story 3 (Session Creation Wizard) - 7 hours
2. Fix BUG-001 (LastAcknowledged) - 1 hour
3. Fix BUG-002 (Timestamp Refresh) - 2 hours
4. Continue Web UI Stories 4-5 (Bulk Operations, Mobile/A11y) - 16 hours
5. Investigate BUG-003 (State File Size) - 1 hour analysis
6. Apply targeted fix for BUG-003 if needed - 2-4 hours

---

## Progress Tracking

### Story 1: Fix LastAcknowledged Persistence
- [ ] Task 1.1: Add field to persistence layer (1h)

### Story 2: Fix Timestamp Refresh Logic
- [ ] Task 2.1: Implement content-based timestamp update (2h)

**Total Progress**: 0 of 2 tasks complete (0%)

---

## Risk Assessment

### Overall Risk: Very Low

**Technical Risks**:
- **Data Loss**: None - both fixes are additive (don't remove existing functionality)
- **Backward Compatibility**: Excellent - `omitempty` tags and zero value semantics prevent issues
- **Performance**: Negligible - SHA256 hashing adds <1ms per operation
- **Testing**: Comprehensive unit and integration tests provided

**Mitigation Strategies**:
- Thorough testing before deployment
- Both fixes are independent - can be rolled back individually if issues arise
- No database migration required (JSON format extensible)

---

## Success Metrics

### Feature Restoration
- Review queue snooze functionality works across restarts (BUG-001 fixed)
- Historical activity timestamps preserved correctly (BUG-002 fixed)

### Quality Metrics
- Zero test regressions (all existing tests pass)
- New tests achieve >95% code coverage of modified functions
- Manual testing confirms user-facing fixes

### Performance Metrics
- Startup time unchanged (within ±10ms tolerance)
- Memory usage unchanged
- SHA256 hashing overhead <1ms per Preview() call

---

## Post-Implementation Actions

After completing these fixes:

1. **Update bug statuses** in `docs/bugs/`:
   - Mark BUG-001 as "Fixed" with verification notes
   - Mark BUG-002 as "Fixed" with verification notes

2. **Update TODO.md**:
   - Mark "Persistence Quick Wins" as complete
   - Update bug tracking section

3. **Announce fixes** in project documentation:
   - Add to CHANGELOG.md or release notes
   - Note: "Review queue snooze now persists across restarts"
   - Note: "Historical activity timestamps now preserved correctly"

4. **Consider long-term improvements**:
   - Add validation tests to prevent future field omissions
   - Investigate BUG-003 (state file size) if performance degrades
   - Consider SQLite migration if query complexity increases

---

## Related Documentation

- **Bug Reports**: `docs/bugs/BUG-001-*.md`, `docs/bugs/BUG-002-*.md`
- **SQLite Migration Plan**: `docs/tasks/repository-pattern-sqlite-migration.md` (deferred)
- **TODO.md**: Current priorities and context
- **Review Queue**: `session/review_queue.go`, `session/review_queue_poller.go`

---

**Epic Status**: Ready to Start
**Next Task**: Task 1.1 - Add LastAcknowledged to Persistence Layer (after Web UI Story 3)
**Estimated Completion**: 3 hours total (both stories can run in parallel if desired)
