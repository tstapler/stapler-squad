# BUG-001: LastAcknowledged Field Not Persisted [SEVERITY: High]

**Status**: ✅ FIXED (Discovered 2025-01-17, Fixed 2025-01-17)
**Resolution**: Field was already properly persisted in storage.go:63, instance.go:167, instance.go:259
**Original Report**: Review queue snooze functionality appeared broken
**Actual Status**: Functionality working correctly, bug report was based on stale code analysis

## Resolution Summary

Investigation revealed this bug was **already fixed** in the current codebase. The `LastAcknowledged` field IS properly persisted:

1. **storage.go:63**: `LastAcknowledged time.Time json:"last_acknowledged,omitempty"`
2. **instance.go:167**: Field correctly serialized in `ToInstanceData()`
3. **instance.go:259**: Field correctly deserialized in `FromInstanceData()`

**Verification**: Comprehensive test suite created in `session/instance_last_acknowledged_test.go` confirms persistence works correctly across save/load cycles.

**Conclusion**: No fix needed - feature already working as designed.

## Problem Description

The `LastAcknowledged` field exists in the `Instance` struct but is **NOT included in the `InstanceData` serialization struct**, causing complete data loss on application restart. This breaks the review queue's snooze/dismiss functionality.

## Reproduction

1. Start stapler-squad with active sessions
2. Dismiss a session from review queue (sets `LastAcknowledged` timestamp)
3. Verify session is removed from review queue
4. Restart stapler-squad application
5. **BUG**: Dismissed session immediately re-appears in review queue
6. `LastAcknowledged` timestamp is reset to zero value

**Expected Behavior**: Dismissed sessions should remain dismissed for the configured snooze duration (4 hours default) across application restarts.

**Actual Behavior**: All dismiss/snooze information is lost on restart because `LastAcknowledged` is never saved to disk.

## Root Cause Analysis

### Field Definition vs Persistence Mismatch

**Instance struct** (`session/instance.go:90-92`):
```go
// LastAcknowledged tracks when the user last acknowledged this session in the review queue
// Used to prevent re-adding sessions the user has explicitly dismissed/snoozed
LastAcknowledged time.Time
```

**InstanceData serialization struct** (`session/storage.go:10-56`):
```go
type InstanceData struct {
    Title      string    `json:"title"`
    // ... many fields ...
    LastViewed time.Time `json:"last_viewed,omitempty"`
    // ❌ LastAcknowledged is MISSING!
}
```

**Serialization method** (`session/instance.go:142-167`):
```go
func (i *Instance) ToInstanceData() InstanceData {
    data := InstanceData{
        // ... copies all persisted fields ...
        LastViewed:           i.LastViewed,
        // ❌ LastAcknowledged is NOT copied!
    }
    return data
}
```

### Why This Matters

The review queue poller relies on `LastAcknowledged` to prevent spam:

**Review Queue Logic** (`session/review_queue_poller.go`):
```go
// Check if session was recently acknowledged (dismissed/snoozed)
timeSinceAck := time.Since(inst.LastAcknowledged)
if timeSinceAck < rqp.config.ReAckThreshold {
    // Don't re-add to queue - user explicitly dismissed this session
    continue
}
```

Without persistence:
- **On startup**: All sessions have `LastAcknowledged = time.Time{}` (zero value)
- **Result**: `time.Since(zero) = ~50 years` → always exceeds ReAckThreshold
- **Impact**: Previously dismissed sessions immediately re-appear in review queue

## Files Affected (3 files - fits context boundary)

1. **session/storage.go** (line ~56) - Add field to `InstanceData` struct
2. **session/instance.go** (line ~166) - Add field to `ToInstanceData()` serialization
3. **session/instance.go** (line ~210) - Add field to `FromInstanceData()` deserialization

## Fix Approach

### Minimal Fix (1 hour, 3 files)

**Step 1**: Add field to serialization struct (`session/storage.go:56`):
```go
type InstanceData struct {
    // ... existing fields ...
    LastViewed time.Time `json:"last_viewed,omitempty"`

    // Review queue snooze/dismiss tracking
    LastAcknowledged time.Time `json:"last_acknowledged,omitempty"`
}
```

**Step 2**: Serialize field (`session/instance.go:166`):
```go
func (i *Instance) ToInstanceData() InstanceData {
    data := InstanceData{
        // ... existing field copies ...
        LastViewed:       i.LastViewed,
        LastAcknowledged: i.LastAcknowledged,  // ← Add this line
    }
    return data
}
```

**Step 3**: Deserialize field (`session/instance.go:210+`):
```go
func FromInstanceData(data InstanceData) (*Instance, error) {
    inst := &Instance{
        // ... existing field assignments ...
        LastViewed:       data.LastViewed,
        LastAcknowledged: data.LastAcknowledged,  // ← Add this line
    }
    return inst, nil
}
```

**Step 4**: Backward compatibility validation:
- Field uses `omitempty` tag → existing JSON without field loads correctly
- Zero value `time.Time{}` is semantically correct (never acknowledged)
- No migration needed - new field auto-initializes to zero

## Verification Strategy

### Unit Test
```go
func TestLastAcknowledgedPersistence(t *testing.T) {
    // Create instance with LastAcknowledged set
    inst := &Instance{
        Title: "test",
        LastAcknowledged: time.Now().Add(-2 * time.Hour),
    }

    // Serialize
    data := inst.ToInstanceData()

    // Verify field is present
    assert.False(t, data.LastAcknowledged.IsZero(), "LastAcknowledged should be serialized")

    // Deserialize
    restored, err := FromInstanceData(data)
    require.NoError(t, err)

    // Verify field survives round-trip
    assert.Equal(t, inst.LastAcknowledged, restored.LastAcknowledged)
}
```

### Integration Test
```go
func TestReviewQueueDismissPersistence(t *testing.T) {
    storage := NewStorage()

    // Create session and dismiss from review queue
    inst := createTestInstance()
    inst.LastAcknowledged = time.Now()
    storage.SaveInstances([]*Instance{inst})

    // Simulate app restart - reload from disk
    loaded, err := storage.LoadInstances()
    require.NoError(t, err)

    // Verify dismiss persisted
    assert.False(t, loaded[0].LastAcknowledged.IsZero(), "Dismiss should survive restart")

    // Verify review queue respects dismiss
    timeSinceAck := time.Since(loaded[0].LastAcknowledged)
    assert.True(t, timeSinceAck < 4*time.Hour, "Should still be within snooze window")
}
```

### Manual Verification
1. Start stapler-squad with active sessions
2. Dismiss a session from review queue (press `d` key)
3. Verify session disappears from review queue
4. Stop and restart stapler-squad
5. **Expected**: Dismissed session does NOT re-appear in review queue
6. Wait 4+ hours (or adjust `ReAckThreshold` in tests)
7. **Expected**: Session re-appears after snooze period expires

## Related Issues

- **BUG-002**: LastMeaningfulOutput timestamp reset on startup (separate issue, different root cause)
- **Schema Completeness**: This bug reveals lack of validation between `Instance` and `InstanceData` structs

## Impact Assessment

**Severity**: **High** (not Critical)
- **User-Facing**: Yes - breaks advertised snooze functionality
- **Data Loss**: Yes - dismiss actions not persisted
- **Workaround**: None - functionality completely broken
- **Frequency**: Every application restart (100% reproduction rate)

**Priority**: Should be fixed before advertising review queue as production-ready feature, but does not block current Web UI work (P1).

**Recommended Timeline**: Fix as part of "persistence quick wins" task after Web UI Story 3 completion.

## Prevention Strategy

**Short-term**: Add this field following the fix above.

**Long-term**: Add validation to prevent future field omissions:

```go
// In session/storage_test.go
func TestInstanceDataCompleteness(t *testing.T) {
    // Use reflection to compare Instance and InstanceData fields
    // Fail test if Instance has fields not present in InstanceData
    // (Exclude non-serializable fields: mutexes, channels, pointers to managers)
}
```

This test would have caught BUG-001 and BUG-002 at development time.

## Additional Notes

- This bug affects **every review queue dismiss/snooze operation** since review queue feature was introduced
- The field was correctly added to `Instance` but never to persistence layer - likely an oversight during initial implementation
- Fix is trivial (3 lines of code) but high impact (restores critical UX feature)
- No database migration needed due to `omitempty` tag and zero value semantics

---

**Bug Tracking ID**: BUG-001
**Related Feature**: Review Queue (session/review_queue.go, session/review_queue_poller.go)
**Fix Complexity**: Low (1 hour, 3 files, trivial code change)
**Fix Risk**: Very Low (backward compatible, well-defined semantics)
**Blocked By**: None
**Blocks**: None (but degrades review queue UX significantly)
