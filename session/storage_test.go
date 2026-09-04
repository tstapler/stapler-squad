package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git"
)

// testDBCounter guarantees each createTestStorage call gets a uniquely-named
// in-memory database, so unrelated tests running in parallel never share state.
var testDBCounter atomic.Int64

// createTestStorage creates a Storage backed by an in-memory Ent repository.
// The caller should defer cleanup().
//
// This package's fixtures are called 300+ times across its test files; each
// call still pays the full Ent schema-creation + backfill-migration cost, but
// backing it with an in-memory SQLite database (rather than a temp-dir file)
// removes the disk I/O and WAL fsync overhead that made the full package
// suite take ~650s and occasionally exceed `go test`'s 10m timeout.
//
// The DSN uses a named, shared-cache in-memory database (rather than the bare
// ":memory:" literal) because some tests (e.g. forceEmptyBranchNameViaRawSQL in
// review_gate_test.go) open a second, independent *sql.DB against the same
// repo.dbPath to reach the database with raw SQL. A bare ":memory:" gives every
// independent sql.Open call its own private, unmigrated database; "cache=shared"
// makes repeated opens of the same "file:" name attach to the same in-memory DB.
func createTestStorage(t *testing.T) (*Storage, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", testDBCounter.Add(1))
	repo, err := NewEntRepository(WithDatabasePath(dsn))
	require.NoError(t, err)

	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	cleanup := func() {
		repo.Close()
	}
	return storage, cleanup
}

// TestCreateTestStorage_SecondConnectionSeesSameData pins the invariant
// createTestStorage's shared-cache DSN exists for: a second, independent
// *sql.DB opened against the same DSN (as forceEmptyBranchNameViaRawSQL does
// in review_gate_test.go) must see rows written through the first connection.
// A bare ":memory:" DSN would fail this — each independent sql.Open gets its
// own private, unmigrated database — so this test would catch a regression
// that silently reintroduced that DSN shape.
func TestCreateTestStorage_SecondConnectionSeesSameData(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	er := storage.repo

	// newTestInstance alone (bare directory-mode, no gitManager.worktree) never
	// produces a worktrees table row -- SaveInstances only persists a worktree
	// row when the instance actually carries one. A GitWorktree reconstructed
	// via NewGitWorktreeFromStorage (as forceEmptyBranchNameViaRawSQL's callers
	// do, review_gate_test.go) is enough to trigger that row without touching
	// disk, since it returns non-nil whenever any of repoPath/worktreePath/
	// branchName is non-empty.
	inst := newTestInstance("shared-cache-visibility-test")
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		"/tmp/test", "/tmp/test", "shared-cache-visibility-test", "placeholder-branch",
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	db, err := sql.Open("sqlite", er.dbPath)
	require.NoError(t, err)
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM worktrees WHERE session_name = ?", "shared-cache-visibility-test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "second connection against the same DSN must see the row written via the first")
}

// TestStorage_UUID_PersistedThroughAddAndLoad is the primary regression test for
// "session not found after restart".  It verifies that a UUID written via
// AddInstance is returned unchanged by LoadInstances, i.e. it survives the
// full storage round-trip through the Ent SQLite backend.
func TestStorage_UUID_PersistedThroughAddAndLoad(t *testing.T) {
	t.Parallel()
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
	inst.started.Store(true)

	require.NoError(t, storage.AddInstance(inst))

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	assert.Equal(t, "my-stable-uuid", loaded[0].GetStableID(),
		"UUID must survive AddInstance → LoadInstances round-trip")
}

// TestStorage_Backend_PersistedThroughAddAndLoad is the real-persistence
// regression test for Epic 5.1: Instance.Backend must survive a round trip
// through the actual ent-backed repository (EntRepository.Create/Update and
// sessionToInstanceData), not just the in-process ToInstanceData/
// FromInstanceData conversion — session/storage.go's SaveInstances/
// LoadInstances is what a real process restart actually goes through.
func TestStorage_Backend_PersistedThroughAddAndLoad(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{
		Title:     "backend-roundtrip",
		Path:      "/tmp/test",
		Status:    Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Backend:   BackendTymux,
	}
	inst.started.Store(true)

	require.NoError(t, storage.AddInstance(inst))

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	assert.Equal(t, BackendTymux, loaded[0].Backend,
		"Backend must survive AddInstance → LoadInstances round-trip through the ent repository")
}

// TestStorage_UUID_StableAcrossMultipleLoads verifies that the UUID returned by
// LoadInstances is deterministic across repeated calls (no re-generation).
func TestStorage_UUID_StableAcrossMultipleLoads(t *testing.T) {
	t.Parallel()
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
	inst.started.Store(true)
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
	t.Parallel()
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
	inst.started.Store(true)
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
	t.Parallel()
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusMgr, nil)

	inst := &Instance{
		Title:   "my-session-title",
		UUID:    "test-uuid-lookup",
		Status:  Paused,
		Program: "claude",
	}
	inst.started.Store(true)
	poller.SetInstances([]*Instance{inst})

	found := poller.FindInstance("test-uuid-lookup")
	require.NotNil(t, found, "FindInstance must find session by UUID")
	assert.Equal(t, "my-session-title", found.Title)
}

// TestReviewQueuePoller_AddInstanceByUUID verifies that AddInstance (as now used
// by CreateSession) makes the new session findable by UUID without replacing
// pre-existing instances.
func TestReviewQueuePoller_AddInstanceByUUID(t *testing.T) {
	t.Parallel()
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	poller := NewReviewQueuePoller(queue, statusMgr, nil)

	existing := &Instance{
		Title:   "existing-session",
		UUID:    "existing-uuid",
		Status:  Paused,
		Program: "claude",
	}
	existing.started.Store(true)
	poller.SetInstances([]*Instance{existing})

	newcomer := &Instance{
		Title:   "new-session",
		UUID:    "new-uuid",
		Status:  Paused,
		Program: "claude",
	}
	newcomer.started.Store(true)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	inst.started.Store(true)
	return inst
}

// TestStorage_UpdateInstanceTimestampsOnly verifies that calling
// UpdateInstanceTimestampsOnly persists the terminal timestamps and optionally
// LastViewed to the underlying repository.
func TestStorage_UpdateInstanceTimestampsOnly(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("update-session")
	require.NoError(t, storage.AddInstance(inst))

	// Mutate fields that the Ent schema supports.
	inst.Tags = []string{"alpha", "beta"}
	inst.Category = "refactor-tests"

	err := storage.UpdateInstance(inst)
	require.NoError(t, err)

	// ListInstanceData uses LoadMinimal, which intentionally skips the Tags
	// edge (see LoadOptions.LoadTags doc comment) — read back via LoadInstances
	// instead, which eager-loads tags, to verify persistence.
	instances, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Equal(t, []string{"alpha", "beta"}, instances[0].Tags, "Tags should be persisted by UpdateInstance")
	assert.Equal(t, "refactor-tests", instances[0].Category, "Category should be persisted by UpdateInstance")
}

// TestStorage_ListInstanceData verifies that ListInstanceData returns raw
// InstanceData entries without constructing Instance objects.
func TestStorage_ListInstanceData(t *testing.T) {
	t.Parallel()
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

// TestStorage_ArchiveInstanceDataByID_should_setArchivedAt_When_SessionExistsInStorageOnly
// is the regression test for the fix in server/services/session_service.go's
// ArchiveSessionByUUID: a session that is not resident in the live in-memory
// ReviewQueuePoller.instances list (e.g. after a server restart, before this fix existed)
// must still be archivable via a direct storage read-modify-write.
func TestStorage_ArchiveInstanceDataByID_should_setArchivedAt_When_SessionExistsInStorageOnly(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("storage-only-session")
	require.NoError(t, storage.AddInstance(inst))

	archived, err := storage.ArchiveInstanceDataByID("storage-only-session", time.Now())
	require.NoError(t, err)
	assert.True(t, archived, "expected the session to be newly archived")

	data, err := storage.FindInstanceDataByID("storage-only-session")
	require.NoError(t, err)
	require.NotNil(t, data.ArchivedAt, "ArchivedAt should be set after archiving")
	assert.Equal(t, Stopped, data.Status, "Status should transition to Stopped")
}

// TestStorage_ArchiveInstanceDataByID_should_preserveOtherFields_When_Archiving guards the
// doc comment's claim that this is a read-modify-write on the full row, not a partial
// struct: a future refactor that built a bare InstanceData{ID, ArchivedAt, Status} instead
// of mutating a fresh FindInstanceDataByID read would pass every other test here (they only
// assert ArchivedAt/Status) while silently clobbering every other field via
// EntRepository.Update's guarded-optional-field pattern (empty/zero fields get cleared).
func TestStorage_ArchiveInstanceDataByID_should_preserveOtherFields_When_Archiving(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("archive-preserves-fields")
	inst.Note = "do not clobber me"
	inst.Category = "preserve-category"
	require.NoError(t, storage.AddInstance(inst))

	archived, err := storage.ArchiveInstanceDataByID("archive-preserves-fields", time.Now())
	require.NoError(t, err)
	require.True(t, archived)

	data, err := storage.FindInstanceDataByID("archive-preserves-fields")
	require.NoError(t, err)
	assert.Equal(t, "do not clobber me", data.Note, "archiving must not clobber unrelated fields")
	assert.Equal(t, "preserve-category", data.Category, "archiving must not clobber unrelated fields")
}

// TestStorage_ArchiveInstanceDataByID_should_beIdempotent_When_AlreadyArchived matches
// SetArchivedAtIfNilAndStop's CAS semantics: a second archive call on an already-archived
// session is a no-op, not an error, and does not clobber the original ArchivedAt.
func TestStorage_ArchiveInstanceDataByID_should_beIdempotent_When_AlreadyArchived(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("already-archived-session")
	require.NoError(t, storage.AddInstance(inst))

	first, err := storage.ArchiveInstanceDataByID("already-archived-session", time.Now())
	require.NoError(t, err)
	require.True(t, first)

	second, err := storage.ArchiveInstanceDataByID("already-archived-session", time.Now())
	require.NoError(t, err)
	assert.False(t, second, "a second archive call should be a no-op")
}

// TestStorage_ArchiveInstanceDataByID_should_returnFalse_When_SessionNotFound matches
// ArchiveSessionByUUID's existing "unconditional sweep call" contract: archiving an
// unknown ID is not an error.
func TestStorage_ArchiveInstanceDataByID_should_returnFalse_When_SessionNotFound(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	archived, err := storage.ArchiveInstanceDataByID("no-such-session", time.Now())
	require.NoError(t, err)
	assert.False(t, archived)
}

// TestStorage_DeleteAllInstances verifies that DeleteAllInstances removes every
// stored instance, leaving an empty repository.
func TestStorage_DeleteAllInstances(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("sync-session")
	require.NoError(t, storage.AddInstance(inst))

	// Mutate fields that the Ent schema supports.
	inst.Tags = []string{"sync-tag"}
	inst.Category = "sync-category"
	err := storage.SaveInstancesSync([]*Instance{inst})
	require.NoError(t, err)

	// ListInstanceData uses LoadMinimal, which intentionally skips the Tags
	// edge (see LoadOptions.LoadTags doc comment) — read back via LoadInstances
	// instead, which eager-loads tags, to verify persistence.
	instances, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Equal(t, []string{"sync-tag"}, instances[0].Tags, "Tags should be persisted by SaveInstancesSync")
	assert.Equal(t, "sync-category", instances[0].Category, "Category should be persisted by SaveInstancesSync")
}

// TestSaveInstances_WorktreeDataQueryableImmediately is a regression test for the
// backlog review-gate "(no diff available)" bug: a review can fire (via
// request_review, from inside the spawned session) as soon as SpawnSessionFromItem
// returns. If the instance's Worktree row (with BaseCommitSha) isn't persisted
// synchronously at spawn time, GetWorktreeDataBySessionUUID returns not-found and
// the review gate falls back to an unreliable diff. This asserts the underlying
// mechanism SpawnSessionFromItem's synchronous SaveInstances call depends on: a
// started, worktree-backed instance must be immediately queryable by UUID with no
// delay and no intervening periodic save.
func TestSaveInstances_WorktreeDataQueryableImmediately(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("worktree-session")
	inst.UUID = "11111111-1111-1111-1111-111111111111"
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		"/repo", "/repo/../worktrees/worktree-session", "worktree-session", "backlog/some-item", "abc123def")

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	wt, err := storage.GetWorktreeDataBySessionUUID(context.Background(), inst.UUID)
	require.NoError(t, err)
	assert.Equal(t, "abc123def", wt.BaseCommitSHA, "BaseCommitSha must be queryable immediately after SaveInstances, with no delay")
	assert.Equal(t, "backlog/some-item", wt.BranchName)
	assert.NotEmpty(t, wt.WorktreePath)
}

// TestSaveInstances_SkipsNotYetStartedInstance documents the gotcha that makes the
// above regression possible in the first place: SaveInstances silently no-ops for
// any instance where Started() is false, so a caller cannot rely on a freshly
// constructed (but not yet started) Instance being persisted.
func TestSaveInstances_SkipsNotYetStartedInstance(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{Title: "not-started", Path: "/tmp/test", UUID: "22222222-2222-2222-2222-222222222222"}
	require.False(t, inst.Started())

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	instances, err := storage.LoadInstances()
	require.NoError(t, err)
	for _, loaded := range instances {
		assert.NotEqual(t, inst.UUID, loaded.UUID, "SaveInstances must not persist an instance that hasn't been marked Started()")
	}
}

// TestDiffStatsDataRoundTrip verifies that save/load cycle preserves metadata
// but excludes content (the desired behavior for BUG-003 fix).
func TestDiffStatsDataRoundTrip(t *testing.T) {
	t.Parallel()
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

// TestSetBacklogItemPRAndTransition_should_TransitionAndPersistPR_When_ItemInReview
// verifies the happy path of the shared primary-write path used by both
// report_pr_created (Epic 3.1) and the reconciliation backstop (Epic 3.2).
func TestSetBacklogItemPRAndTransition_should_TransitionAndPersistPR_When_ItemInReview(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Ship it",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	err = storage.SetBacklogItemPRAndTransition(ctx, item, "https://github.com/tstapler/stapler-squad/pull/55", 55, "Implemented the feature.", nil)
	require.NoError(t, err)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 55, fetched.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/55", fetched.PrURL)
}

// TestSetBacklogItemPRAndTransition_should_NoOp_When_AlreadyPRPendingSamePR
// verifies the idempotency contract directly at the storage layer.
func TestSetBacklogItemPRAndTransition_should_NoOp_When_AlreadyPRPendingSamePR(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Already shipped",
		Status: string(BacklogStatusPRPending),
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/55"
	prNum := 55
	updated, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNum}, nil)
	require.NoError(t, err)

	err = storage.SetBacklogItemPRAndTransition(ctx, updated, prURL, prNum, "Implemented the feature.", nil)
	require.NoError(t, err, "repeating the same PR on an already-pr_pending item must be a no-op success, not an error — the idempotency short-circuit must run before the reassignment-guard check, so a nil guard here is fine")
}

// TestSetBacklogItemPRAndTransition_should_ReturnError_When_StorageWriteFails
// mirrors BUG-040's own TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies
// test technique: close the real on-disk SQLite connection right after the
// item is created, then call SetBacklogItemPRAndTransition and assert the
// failure is returned to the caller rather than silently swallowed — the
// exact discipline BUG-040 root cause #1 violated.
func TestSetBacklogItemPRAndTransition_should_ReturnError_When_StorageWriteFails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-persist-fail-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Ship it",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	require.NoError(t, repo.Close())

	err = storage.SetBacklogItemPRAndTransition(ctx, item, "https://github.com/tstapler/stapler-squad/pull/55", 55, "Implemented the feature.", nil)
	require.Error(t, err, "a storage write failure must be returned to the caller, not silently swallowed")
}

// TestSetBacklogItemPRAndTransition_should_RejectPrecondition_When_ObservedStatusInvalid
// verifies the invalid-starting-status guard directly at the storage layer:
// only "review" or "pr_pending" are ever accepted, regardless of caller.
func TestSetBacklogItemPRAndTransition_should_RejectPrecondition_When_ObservedStatusInvalid(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Not ready for a PR yet",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	err = storage.SetBacklogItemPRAndTransition(ctx, item, "https://github.com/tstapler/stapler-squad/pull/55", 55, "Too early.", nil)
	require.ErrorIs(t, err, ErrPreconditionFailed)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusIdea), fetched.Status, "an invalid starting status must never be transitioned")
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestSetBacklogItemPRAndTransition_should_RejectStaleObserved_When_ConcurrentWriteWon
// verifies the CAS pinning property directly at the storage layer, isolated
// from the MCP handler/mock scaffolding TestReportPRCreated_LoserPRNeverPersists_WhenCASPreconditionFails
// exercises: a caller's observed snapshot must fail the write once it's
// stale, even for an unrelated field change that only bumped updated_at.
func TestSetBacklogItemPRAndTransition_should_RejectStaleObserved_When_ConcurrentWriteWon(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Ship it",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	// Simulate a concurrent winner landing between this caller's read
	// (item, captured above) and its write below — any field write bumps
	// updated_at (ent's UpdateDefault), which is enough to invalidate a
	// pinned-snapshot CAS.
	title := "Retitled by a concurrent winner"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{Title: &title}, nil)
	require.NoError(t, err)

	err = storage.SetBacklogItemPRAndTransition(ctx, item /* stale */, "https://github.com/tstapler/stapler-squad/pull/55", 55, "Implemented the feature.", nil)
	require.ErrorIs(t, err, ErrPreconditionFailed)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "a stale observed snapshot must never win the CAS")
	assert.Equal(t, 0, fetched.PrNumber)
}

// TestUpdateInstanceIfEpoch_should_ApplyWrite_When_EpochMatches covers Task
// 1.2.4a's happy path: a freshly-added row's creation_epoch defaults to 0, so a
// caller presenting capturedEpoch=0 wins the conditional UPDATE.
func TestUpdateInstanceIfEpoch_should_ApplyWrite_When_EpochMatches(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{
		Title:     "epoch-match",
		UUID:      "uuid-epoch-match",
		Path:      "/tmp/test",
		Status:    Creating,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))

	applied, err := storage.UpdateInstanceIfEpoch(context.Background(), "uuid-epoch-match", 0, Active, "")
	require.NoError(t, err)
	assert.True(t, applied)

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, Active, loaded[0].Status, "the persisted row must now read Active")
}

// TestUpdateInstanceIfEpoch_should_ReturnFalse_When_EpochIsStale covers Task
// 1.2.4a's fencing guarantee: a captured epoch that no longer matches the
// persisted row's creation_epoch (already bumped past it by a cancel/retry) is
// rejected and the row is left unchanged.
func TestUpdateInstanceIfEpoch_should_ReturnFalse_When_EpochIsStale(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{
		Title:     "epoch-stale",
		UUID:      "uuid-epoch-stale",
		Path:      "/tmp/test",
		Status:    Creating,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))

	// The persisted row's creation_epoch defaults to 0; present a stale
	// captured value of 2 (as if a cancel had already bumped it past this
	// caller's captured value).
	applied, err := storage.UpdateInstanceIfEpoch(context.Background(), "uuid-epoch-stale", 2, Active, "")
	require.NoError(t, err)
	assert.False(t, applied)

	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, Creating, loaded[0].Status, "the persisted row must be unchanged when epochs mismatch")
}
