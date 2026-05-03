package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestStorage creates a temporary Storage backed by an Ent repository.
// The caller should defer cleanup().
func createTestStorage(t *testing.T) (*Storage, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	cleanup := func() {
		repo.Close()
		os.RemoveAll(tmpDir)
	}
	return storage, cleanup
}

// TestStorage_UUID_PersistedThroughAddAndLoad is the primary regression test for
// "session not found after restart".  It verifies that a UUID written via
// AddInstance is returned unchanged by LoadInstances, i.e. it survives the
// full storage round-trip through the Ent SQLite backend.
func TestStorage_UUID_PersistedThroughAddAndLoad(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{
		Title:     "uuid-roundtrip",
		UUID:      "my-stable-uuid",
		Path:      "/tmp/test",
		Status:    Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	// Paused sets started=true internally, which is required for SaveInstances.
	// We use the same fast-path that FromInstanceData takes.
	inst.started = true

	require.NoError(t, storage.AddInstance(inst))

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	assert.Equal(t, "my-stable-uuid", loaded[0].GetStableID(),
		"UUID must survive AddInstance → LoadInstances round-trip")
}

// TestStorage_UUID_StableAcrossMultipleLoads verifies that the UUID returned by
// LoadInstances is deterministic across repeated calls (no re-generation).
func TestStorage_UUID_StableAcrossMultipleLoads(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{
		Title:     "multi-load",
		UUID:      "consistent-uuid",
		Path:      "/tmp/test",
		Status:    Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.started = true
	require.NoError(t, storage.AddInstance(inst))

	first, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, first, 1)

	second, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, second, 1)

	assert.Equal(t, first[0].GetStableID(), second[0].GetStableID(),
		"GetStableID must return the same value on repeated LoadInstances calls")
}

// TestStorage_UUID_MigrationAssignsAndPersists covers the upgrade path for
// legacy sessions (UUID="" in storage). On the first LoadInstances the
// migration in FromInstanceData assigns a new UUID.  That UUID is then saved
// back by the caller (as happens in the startup background goroutine), and
// the second LoadInstances should return the same UUID.
func TestStorage_UUID_MigrationAssignsAndPersists(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	// Insert a legacy session with no UUID (simulating pre-UUID schema data).
	inst := &Instance{
		Title:     "legacy-no-uuid",
		UUID:      "", // explicitly empty — legacy session
		Path:      "/tmp/test",
		Status:    Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.started = true
	require.NoError(t, storage.AddInstance(inst))

	// First load: migration assigns a new UUID.
	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	assignedUUID := loaded[0].GetStableID()
	assert.NotEmpty(t, assignedUUID, "migration must assign a non-empty UUID to legacy sessions")

	// Simulate the startup goroutine persisting the migrated UUID.
	require.NoError(t, storage.SaveInstances(loaded))

	// Second load (simulated restart): must return the same UUID.
	reloaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)

	assert.Equal(t, assignedUUID, reloaded[0].GetStableID(),
		"migrated UUID must be stable after SaveInstances + second LoadInstances")
}

// TestReviewQueuePoller_FindInstanceByUUID verifies that FindInstance resolves a
// session by its UUID (the path used by WebSocket stream reconnections).
// This is the specific lookup that was failing with "session not found" after restart.
func TestReviewQueuePoller_FindInstanceByUUID(t *testing.T) {
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusMgr, nil)

	inst := &Instance{
		Title:   "my-session-title",
		UUID:    "test-uuid-lookup",
		Status:  Paused,
		Program: "claude",
	}
	inst.started = true
	poller.SetInstances([]*Instance{inst})

	found := poller.FindInstance("test-uuid-lookup")
	require.NotNil(t, found, "FindInstance must find session by UUID")
	assert.Equal(t, "my-session-title", found.Title)
}

// TestReviewQueuePoller_AddInstanceByUUID verifies that AddInstance (as now used
// by CreateSession) makes the new session findable by UUID without replacing
// pre-existing instances.
func TestReviewQueuePoller_AddInstanceByUUID(t *testing.T) {
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusMgr, nil)

	existing := &Instance{
		Title:   "existing-session",
		UUID:    "existing-uuid",
		Status:  Paused,
		Program: "claude",
	}
	existing.started = true
	poller.SetInstances([]*Instance{existing})

	newcomer := &Instance{
		Title:   "new-session",
		UUID:    "new-uuid",
		Status:  Paused,
		Program: "claude",
	}
	newcomer.started = true
	poller.AddInstance(newcomer)

	// Both sessions must be findable.
	assert.NotNil(t, poller.FindInstance("existing-uuid"),
		"AddInstance must preserve pre-existing instances in the poller")
	assert.NotNil(t, poller.FindInstance("new-uuid"),
		"AddInstance must make the new session findable by UUID")
}

// TestDiffStatsDataSerializationExcludesContent verifies that the Content field
// is excluded from JSON serialization to reduce state file size.
// This is the fix for BUG-003: Large State File Size.
func TestDiffStatsDataSerializationExcludesContent(t *testing.T) {
	// Create DiffStatsData with content
	stats := DiffStatsData{
		Added:   10,
		Removed: 5,
		Content: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1,5 +1,10 @@\n+new line\n",
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(stats)
	require.NoError(t, err)

	// Parse JSON into a map to check field presence
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	// Verify metadata fields are present
	assert.Contains(t, parsed, "added", "added field should be present in JSON")
	assert.Contains(t, parsed, "removed", "removed field should be present in JSON")
	assert.Equal(t, float64(10), parsed["added"], "added value should be correct")
	assert.Equal(t, float64(5), parsed["removed"], "removed value should be correct")

	// Verify content field is NOT present (excluded via json:"-" tag)
	assert.NotContains(t, parsed, "content", "content field should be excluded from JSON")
}

// TestDiffStatsDataBackwardCompatibility verifies that old state files with
// diff_stats.content field can still be loaded correctly.
// The content field will be silently ignored during deserialization.
func TestDiffStatsDataBackwardCompatibility(t *testing.T) {
	// Simulate old state file JSON with content field
	oldJSON := `{
		"added": 10,
		"removed": 5,
		"content": "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1,5 +1,10 @@\n+new line"
	}`

	// Parse into DiffStatsData
	var stats DiffStatsData
	err := json.Unmarshal([]byte(oldJSON), &stats)
	require.NoError(t, err)

	// Verify metadata loaded correctly
	assert.Equal(t, 10, stats.Added, "added should be loaded correctly")
	assert.Equal(t, 5, stats.Removed, "removed should be loaded correctly")

	// Content field should be empty (ignored during deserialization due to json:"-" tag)
	assert.Empty(t, stats.Content, "content should be empty after deserialization (excluded field)")
}

// TestInstanceDataSaveExcludesDiffContent verifies that when an Instance
// is converted to InstanceData for serialization, the diff content is excluded.
func TestInstanceDataSaveExcludesDiffContent(t *testing.T) {
	// Create InstanceData with diff stats including content
	data := InstanceData{
		Title: "test-session",
		Path:  "/test/path",
		DiffStats: DiffStatsData{
			Added:   25,
			Removed: 10,
			Content: "large diff content that should not be persisted to reduce file size...",
		},
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(data)
	require.NoError(t, err)

	// Parse JSON into a map to check diff_stats structure
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	// Verify diff_stats is present
	diffStats, ok := parsed["diff_stats"].(map[string]interface{})
	require.True(t, ok, "diff_stats should be present in JSON")

	// Verify metadata is present
	assert.Contains(t, diffStats, "added", "diff_stats.added should be present")
	assert.Contains(t, diffStats, "removed", "diff_stats.removed should be present")
	assert.Equal(t, float64(25), diffStats["added"])
	assert.Equal(t, float64(10), diffStats["removed"])

	// Verify content is NOT present
	assert.NotContains(t, diffStats, "content", "diff_stats.content should be excluded from JSON")
}

// TestInstanceDataLoadWithDiffContent verifies backward compatibility when
// loading old state files that contain diff_stats.content.
func TestInstanceDataLoadWithDiffContent(t *testing.T) {
	// Simulate old state file JSON with diff content
	oldJSON := `{
		"title": "legacy-session",
		"path": "/old/path",
		"working_dir": "",
		"branch": "main",
		"status": 0,
		"height": 0,
		"width": 0,
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": "2025-01-01T00:00:00Z",
		"auto_yes": false,
		"prompt": "",
		"program": "claude",
		"worktree": {
			"repo_path": "",
			"worktree_path": "",
			"session_name": "",
			"branch_name": "",
			"base_commit_sha": ""
		},
		"diff_stats": {
			"added": 100,
			"removed": 50,
			"content": "This is legacy diff content that should be ignored on load..."
		}
	}`

	// Parse into InstanceData
	var data InstanceData
	err := json.Unmarshal([]byte(oldJSON), &data)
	require.NoError(t, err)

	// Verify basic fields loaded correctly
	assert.Equal(t, "legacy-session", data.Title)
	assert.Equal(t, "/old/path", data.Path)
	assert.Equal(t, "claude", data.Program)

	// Verify diff stats metadata loaded correctly
	assert.Equal(t, 100, data.DiffStats.Added)
	assert.Equal(t, 50, data.DiffStats.Removed)

	// Verify content is empty (ignored due to json:"-" tag)
	assert.Empty(t, data.DiffStats.Content, "diff content should be empty after loading old state")
}

// newTestInstance returns a minimal Paused Instance ready for AddInstance.
func newTestInstance(title string) *Instance {
	inst := &Instance{
		Title:     title,
		Path:      "/tmp/test",
		Status:    Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.started = true
	return inst
}

// TestStorage_UpdateInstanceTimestampsOnly verifies that calling
// UpdateInstanceTimestampsOnly persists the terminal timestamps and optionally
// LastViewed to the underlying repository.
func TestStorage_UpdateInstanceTimestampsOnly(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("ts-session")
	require.NoError(t, storage.AddInstance(inst))

	lastTerminal := time.Now().Add(-5 * time.Second).Truncate(time.Millisecond)
	lastMeaningful := time.Now().Add(-3 * time.Second).Truncate(time.Millisecond)
	lastViewed := time.Now().Add(-1 * time.Second).Truncate(time.Millisecond)
	sig := "abc123sig"

	err := storage.UpdateInstanceTimestampsOnly("ts-session", lastTerminal, lastMeaningful, sig, lastViewed)
	require.NoError(t, err)

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)

	got := rows[0]
	assert.Equal(t, sig, got.LastOutputSignature, "LastOutputSignature should be updated")
	assert.WithinDuration(t, lastViewed, got.LastViewed, time.Second, "LastViewed should be updated")
}

// TestStorage_UpdateInstanceTimestampsOnly_ZeroLastViewed verifies that
// passing a zero LastViewed does NOT overwrite the existing value.
func TestStorage_UpdateInstanceTimestampsOnly_ZeroLastViewed(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("ts-zero-session")
	require.NoError(t, storage.AddInstance(inst))

	// Set an initial LastViewed via a first call.
	original := time.Now().Add(-10 * time.Second).Truncate(time.Millisecond)
	require.NoError(t, storage.UpdateInstanceTimestampsOnly("ts-zero-session", time.Time{}, time.Time{}, "", original))

	// Now call again with zero LastViewed — it must not overwrite.
	require.NoError(t, storage.UpdateInstanceTimestampsOnly("ts-zero-session", time.Time{}, time.Time{}, "", time.Time{}))

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.WithinDuration(t, original, rows[0].LastViewed, time.Second,
		"LastViewed must not be overwritten by a zero value")
}

// TestStorage_UpdateInstanceLastAddedToQueue verifies the partial-field update
// for LastAddedToQueue.
func TestStorage_UpdateInstanceLastAddedToQueue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("queue-session")
	require.NoError(t, storage.AddInstance(inst))

	queueTime := time.Now().Truncate(time.Millisecond)
	err := storage.UpdateInstanceLastAddedToQueue("queue-session", queueTime)
	require.NoError(t, err)

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.WithinDuration(t, queueTime, rows[0].LastAddedToQueue, time.Second,
		"LastAddedToQueue should be updated")
}

// TestStorage_UpdateInstanceLastUserResponse verifies the partial-field update
// for LastUserResponse.
//
// NOTE: LastUserResponse is not yet in the Ent schema, so it cannot be
// persisted to SQLite by the current EntRepository backend. This test verifies
// that the call succeeds without error (the mutation is a no-op at the DB
// level). Persistence will be enabled once the Ent schema is extended with
// this column.
func TestStorage_UpdateInstanceLastUserResponse(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("response-session")
	require.NoError(t, storage.AddInstance(inst))

	responseTime := time.Now().Truncate(time.Millisecond)
	err := storage.UpdateInstanceLastUserResponse("response-session", responseTime)
	require.NoError(t, err, "UpdateInstanceLastUserResponse must not return an error")

	// Verify the call does not corrupt the existing record.
	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1, "session must still exist after the update call")
	assert.Equal(t, "response-session", rows[0].Title)
}

// TestStorage_UpdateInstanceAcknowledged verifies that UpdateInstanceAcknowledged
// sets LastAcknowledged to a non-zero time.
func TestStorage_UpdateInstanceAcknowledged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("ack-session")
	require.NoError(t, storage.AddInstance(inst))

	// Confirm it starts at zero.
	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].LastAcknowledged.IsZero(), "LastAcknowledged should be zero before acknowledging")

	before := time.Now()
	err = storage.UpdateInstanceAcknowledged("ack-session")
	require.NoError(t, err)
	after := time.Now()

	rows, err = storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].LastAcknowledged.IsZero(), "LastAcknowledged should be non-zero after acknowledging")
	assert.True(t, !rows[0].LastAcknowledged.Before(before) && !rows[0].LastAcknowledged.After(after),
		"LastAcknowledged should be within the before/after window")
}

// TestStorage_UpdateInstanceProcessingGrace verifies the partial-field update
// for ProcessingGraceUntil.
//
// NOTE: ProcessingGraceUntil is not yet in the Ent schema, so it cannot be
// persisted to SQLite by the current EntRepository backend. This test verifies
// that the call succeeds without error (the mutation is a no-op at the DB
// level). Persistence will be enabled once the Ent schema is extended with
// this column.
func TestStorage_UpdateInstanceProcessingGrace(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("grace-session")
	require.NoError(t, storage.AddInstance(inst))

	graceTime := time.Now().Add(30 * time.Second).Truncate(time.Millisecond)
	err := storage.UpdateInstanceProcessingGrace("grace-session", graceTime)
	require.NoError(t, err, "UpdateInstanceProcessingGrace must not return an error")

	// Verify the call does not corrupt the existing record.
	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1, "session must still exist after the update call")
	assert.Equal(t, "grace-session", rows[0].Title)
}

// TestStorage_UpdateInstance verifies that UpdateInstance replaces all fields
// (not a partial update) for an existing instance.
func TestStorage_UpdateInstance(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("update-session")
	require.NoError(t, storage.AddInstance(inst))

	// Mutate fields that the Ent schema supports.
	inst.Tags = []string{"alpha", "beta"}
	inst.Category = "refactor-tests"

	err := storage.UpdateInstance(inst)
	require.NoError(t, err)

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"alpha", "beta"}, rows[0].Tags, "Tags should be persisted by UpdateInstance")
	assert.Equal(t, "refactor-tests", rows[0].Category, "Category should be persisted by UpdateInstance")
}

// TestStorage_ListInstanceData verifies that ListInstanceData returns raw
// InstanceData entries without constructing Instance objects.
func TestStorage_ListInstanceData(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst1 := newTestInstance("list-session-1")
	inst2 := newTestInstance("list-session-2")
	require.NoError(t, storage.AddInstance(inst1))
	require.NoError(t, storage.AddInstance(inst2))

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	assert.Len(t, rows, 2, "ListInstanceData should return both added instances")

	titles := make(map[string]bool)
	for _, r := range rows {
		titles[r.Title] = true
	}
	assert.True(t, titles["list-session-1"], "list-session-1 should be present")
	assert.True(t, titles["list-session-2"], "list-session-2 should be present")
}

// TestStorage_DeleteAllInstances verifies that DeleteAllInstances removes every
// stored instance, leaving an empty repository.
func TestStorage_DeleteAllInstances(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	require.NoError(t, storage.AddInstance(newTestInstance("del-session-1")))
	require.NoError(t, storage.AddInstance(newTestInstance("del-session-2")))

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 2, "precondition: 2 instances should exist before delete")

	err = storage.DeleteAllInstances()
	require.NoError(t, err)

	rows, err = storage.ListInstanceData()
	require.NoError(t, err)
	assert.Empty(t, rows, "ListInstanceData should return empty after DeleteAllInstances")
}

// TestStorage_SaveInstancesSync verifies that SaveInstancesSync persists
// mutated instance state to the repository synchronously.
func TestStorage_SaveInstancesSync(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("sync-session")
	require.NoError(t, storage.AddInstance(inst))

	// Mutate fields that the Ent schema supports.
	inst.Tags = []string{"sync-tag"}
	inst.Category = "sync-category"
	err := storage.SaveInstancesSync([]*Instance{inst})
	require.NoError(t, err)

	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, []string{"sync-tag"}, rows[0].Tags, "Tags should be persisted by SaveInstancesSync")
	assert.Equal(t, "sync-category", rows[0].Category, "Category should be persisted by SaveInstancesSync")
}

// TestDiffStatsDataRoundTrip verifies that save/load cycle preserves metadata
// but excludes content (the desired behavior for BUG-003 fix).
func TestDiffStatsDataRoundTrip(t *testing.T) {
	// Original data with content
	original := DiffStatsData{
		Added:   42,
		Removed: 17,
		Content: "This content should not survive the round trip...",
	}

	// Serialize
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	var loaded DiffStatsData
	err = json.Unmarshal(jsonBytes, &loaded)
	require.NoError(t, err)

	// Metadata should be preserved
	assert.Equal(t, original.Added, loaded.Added, "added should be preserved")
	assert.Equal(t, original.Removed, loaded.Removed, "removed should be preserved")

	// Content should NOT be preserved (this is the desired behavior)
	assert.Empty(t, loaded.Content, "content should be empty after round trip")
}
