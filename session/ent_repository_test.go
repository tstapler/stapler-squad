package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ssent "github.com/tstapler/stapler-squad/session/ent"
)

// TestEntRepository_CreateAndGet tests basic create and get operations
func TestEntRepository_CreateAndGet(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create test session
	session := createTestSession("test-session-1")

	// Create session in repository
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve session
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)

	// Verify basic fields
	assert.Equal(t, session.Title, retrieved.Title)
	assert.Equal(t, session.Path, retrieved.Path)
	assert.Equal(t, session.Branch, retrieved.Branch)
	assert.Equal(t, session.Status, retrieved.Status)
	assert.Equal(t, session.Program, retrieved.Program)
}

// TestEntRepository_CreateDuplicate tests duplicate title handling
func TestEntRepository_CreateDuplicate(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("duplicate-test")

	// First create should succeed
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Second create with same title should fail
	err = repo.Create(ctx, session)
	assert.Error(t, err)
}

// TestEntRepository_Update tests updating an existing session
func TestEntRepository_Update(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial session
	session := createTestSession("update-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Modify session
	session.Branch = "feature-branch"
	session.Status = Paused
	session.Category = "Updated"

	// Update session
	err = repo.Update(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.Equal(t, "feature-branch", retrieved.Branch)
	assert.Equal(t, Paused, retrieved.Status)
	assert.Equal(t, "Updated", retrieved.Category)
}

// TestEntRepository_Delete tests session deletion
func TestEntRepository_Delete(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	session := createTestSession("delete-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Delete session
	err = repo.Delete(ctx, session.Title)
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.Get(ctx, session.Title)
	assert.Error(t, err)
}

func TestEntRepository_Delete_WithShells(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("delete-with-shells-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	_, err = repo.CreateShell(ctx, session.Title, ShellData{
		ID:              uuid.NewString(),
		Name:            "shell-1",
		Command:         "bash",
		TmuxSessionName: "delete-with-shells-test-shell-1",
		OrderIndex:      0,
	})
	require.NoError(t, err)

	// Delete session — must not fail with a FOREIGN KEY constraint error even
	// though a Shell row (Shell.session edge is Required()) still references it.
	err = repo.Delete(ctx, session.Title)
	require.NoError(t, err)

	_, err = repo.Get(ctx, session.Title)
	assert.Error(t, err)
}

// TestEntRepository_List tests listing all sessions
func TestEntRepository_List(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple sessions
	session1 := createTestSession("list-test-1")
	session2 := createTestSession("list-test-2")

	err := repo.Create(ctx, session1)
	require.NoError(t, err)
	err = repo.Create(ctx, session2)
	require.NoError(t, err)

	// List all sessions
	sessions, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

// TestEntRepository_ListByStatus tests filtering sessions by status
func TestEntRepository_ListByStatus(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create sessions with different statuses
	running := createTestSession("running-session")
	running.Status = Running
	paused := createTestSession("paused-session")
	paused.Status = Paused

	err := repo.Create(ctx, running)
	require.NoError(t, err)
	err = repo.Create(ctx, paused)
	require.NoError(t, err)

	// Query running sessions
	runningSessions, err := repo.ListByStatus(ctx, Running)
	require.NoError(t, err)
	assert.Len(t, runningSessions, 1)
	assert.Equal(t, "running-session", runningSessions[0].Title)

	// Query paused sessions
	pausedSessions, err := repo.ListByStatus(ctx, Paused)
	require.NoError(t, err)
	assert.Len(t, pausedSessions, 1)
	assert.Equal(t, "paused-session", pausedSessions[0].Title)
}

// TestEntRepository_Tags tests tag operations
func TestEntRepository_Tags(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with tags
	session := createTestSession("tagged-session")
	session.Tags = []string{"frontend", "urgent"}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify tags
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.ElementsMatch(t, session.Tags, retrieved.Tags)

	// Query by tag
	sessions, err := repo.ListByTag(ctx, "frontend")
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "tagged-session", sessions[0].Title)
}

// TestEntRepository_Worktree tests worktree persistence
func TestEntRepository_Worktree(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with worktree
	session := createTestSession("worktree-session")
	session.Worktree = GitWorktreeData{
		RepoPath:      "/path/to/repo",
		WorktreePath:  "/path/to/worktree",
		SessionName:   "test-session",
		BranchName:    "feature-branch",
		BaseCommitSHA: "abc123",
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify worktree
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.Equal(t, session.Worktree.RepoPath, retrieved.Worktree.RepoPath)
	assert.Equal(t, session.Worktree.WorktreePath, retrieved.Worktree.WorktreePath)
	assert.Equal(t, session.Worktree.BranchName, retrieved.Worktree.BranchName)
}

// TestEntRepository_ListWithOptions_RespectsLoadWorktree verifies that
// ListWithOptions honors LoadOptions.LoadWorktree instead of always
// eager-loading every edge regardless of the requested options.
func TestEntRepository_ListWithOptions_RespectsLoadWorktree(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("list-with-options-worktree")
	session.Worktree = GitWorktreeData{
		RepoPath:      "/path/to/repo",
		WorktreePath:  "/path/to/worktree",
		SessionName:   "test-session",
		BranchName:    "feature-branch",
		BaseCommitSHA: "abc123",
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// LoadMinimal should skip worktree loading entirely.
	minimal, err := repo.ListWithOptions(ctx, LoadMinimal)
	require.NoError(t, err)
	require.Len(t, minimal, 1)
	assert.Equal(t, GitWorktreeData{}, minimal[0].Worktree)

	// LoadOptions{LoadWorktree: true} should load it.
	full, err := repo.ListWithOptions(ctx, LoadOptions{LoadWorktree: true})
	require.NoError(t, err)
	require.Len(t, full, 1)
	assert.Equal(t, session.Worktree.RepoPath, full[0].Worktree.RepoPath)
	assert.Equal(t, session.Worktree.WorktreePath, full[0].Worktree.WorktreePath)
	assert.Equal(t, session.Worktree.BranchName, full[0].Worktree.BranchName)
}

// TestEntRepository_DiffStats tests diff stats persistence
func TestEntRepository_DiffStats(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with diff stats
	session := createTestSession("diffstats-session")
	session.DiffStats = DiffStatsData{
		Added:   100,
		Removed: 50,
		Content: "diff content here",
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify diff stats
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.Equal(t, session.DiffStats.Added, retrieved.DiffStats.Added)
	assert.Equal(t, session.DiffStats.Removed, retrieved.DiffStats.Removed)
	assert.Equal(t, session.DiffStats.Content, retrieved.DiffStats.Content)
}

// TestEntRepository_ClaudeSession tests Claude session persistence
func TestEntRepository_ClaudeSession(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with Claude session data
	session := createTestSession("claude-session")
	session.ClaudeSession = ClaudeSessionData{
		ConversationUUID: "claude-123",
		SquadSessionID:   "conv-456",
		ProjectName:      "test-project",
		LastAttached:     time.Now(),
		Settings: ClaudeSettings{
			AutoReattach:          true,
			PreferredSessionName:  "my-session",
			CreateNewOnMissing:    false,
			ShowSessionSelector:   true,
			SessionTimeoutMinutes: 60,
		},
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify Claude session
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.Equal(t, session.ClaudeSession.ConversationUUID, retrieved.ClaudeSession.ConversationUUID)
	assert.Equal(t, session.ClaudeSession.SquadSessionID, retrieved.ClaudeSession.SquadSessionID)
	assert.Equal(t, session.ClaudeSession.Settings.AutoReattach, retrieved.ClaudeSession.Settings.AutoReattach)
	assert.Equal(t, session.ClaudeSession.Metadata["key1"], retrieved.ClaudeSession.Metadata["key1"])
}

// TestEntRepository_UpdateTimestamps tests efficient timestamp updates
func TestEntRepository_UpdateTimestamps(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	session := createTestSession("timestamp-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Update timestamps
	lastTerminal := time.Now()
	lastMeaningful := time.Now().Add(-5 * time.Minute)
	signature := "sha256-hash"

	err = repo.UpdateTimestamps(ctx, session.Title, lastTerminal, lastMeaningful, signature)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.WithinDuration(t, lastTerminal, retrieved.LastTerminalUpdate, time.Second)
	assert.WithinDuration(t, lastMeaningful, retrieved.LastMeaningfulOutput, time.Second)
	assert.Equal(t, signature, retrieved.LastOutputSignature)
}

// TestEntRepository_UpdateTimestamps_NotFound verifies that UpdateTimestamps returns an
// error when the session title does not exist (n==0 from the direct UPDATE).
func TestEntRepository_UpdateTimestamps_NotFound(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.UpdateTimestamps(ctx, "nonexistent-session", time.Now(), time.Now(), "sig")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-session")
}

// TestEntRepository_UUID_PersistAndLoad verifies that the UUID field is stored and
// retrieved correctly. This is the regression test for the "session not found after
// restart" bug: the Ent schema previously had no uuid column, so every restart
// assigned a new random UUID, invalidating all client-stored session IDs.
func TestEntRepository_UUID_PersistAndLoad(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	sess := createTestSession("uuid-persist-test")
	sess.UUID = "fixed-uuid-1234"

	require.NoError(t, repo.Create(ctx, sess))

	retrieved, err := repo.Get(ctx, sess.Title)
	require.NoError(t, err)
	assert.Equal(t, "fixed-uuid-1234", retrieved.UUID)
}

// TestEntRepository_UUID_UpdatePreservesUUID verifies that updating a session
// preserves (or overwrites) the UUID field correctly.
func TestEntRepository_UUID_UpdatePreservesUUID(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	sess := createTestSession("uuid-update-test")
	sess.UUID = "original-uuid"
	require.NoError(t, repo.Create(ctx, sess))

	sess.UUID = "updated-uuid"
	sess.Branch = "new-branch"
	require.NoError(t, repo.Update(ctx, sess))

	retrieved, err := repo.Get(ctx, sess.Title)
	require.NoError(t, err)
	assert.Equal(t, "updated-uuid", retrieved.UUID)
}

// TestEntRepository_UUID_SurvivesDBReopen simulates a server restart by closing
// and reopening the database. The UUID must be the same as before close.
// This is the core regression test: previously the UUID was not stored in the DB,
// so every open assigned a fresh random UUID via the migration path in
// FromInstanceData, breaking all client session references.
func TestEntRepository_UUID_SurvivesDBReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-restart.db")

	// First "run": create a session with a known UUID.
	const wantUUID = "stable-uuid-across-restarts"
	func() {
		repo, err := NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()

		sess := createTestSession("restart-test-session")
		sess.UUID = wantUUID
		require.NoError(t, repo.Create(context.Background(), sess))
	}()

	// Second "run": open the same DB and verify UUID is unchanged.
	repo2, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()

	retrieved, err := repo2.Get(context.Background(), "restart-test-session")
	require.NoError(t, err)
	assert.Equal(t, wantUUID, retrieved.UUID,
		"UUID must survive DB close/reopen (simulated server restart)")
}

// TestEntRepository_UUID_EmptyDefaultDoesNotBreakList verifies that sessions
// created without a UUID (legacy rows that pre-date UUID assignment) are
// listed correctly with an empty UUID rather than causing errors.
func TestEntRepository_UUID_EmptyDefaultDoesNotBreakList(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create two sessions: one with UUID, one without.
	withUUID := createTestSession("with-uuid")
	withUUID.UUID = "has-uuid-value"

	withoutUUID := createTestSession("without-uuid")
	withoutUUID.UUID = "" // legacy session

	require.NoError(t, repo.Create(ctx, withUUID))
	require.NoError(t, repo.Create(ctx, withoutUUID))

	all, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	byTitle := make(map[string]InstanceData)
	for _, d := range all {
		byTitle[d.Title] = d
	}
	assert.Equal(t, "has-uuid-value", byTitle["with-uuid"].UUID)
	assert.Equal(t, "", byTitle["without-uuid"].UUID)
}

// Helper function to create a test Ent repository
// TestUpdateReviewQueueState_SingleStatement enforces that UpdateReviewQueueState
// issues a single direct UPDATE and does NOT perform a SELECT first.
// Regression guard for the SELECT+UPDATE → direct UPDATE refactor (PerfFix from
// 2026-05-30 profiling session). An ent query interceptor counts any SELECT fired
// against the Session table; the count must be 0 after the call.
func TestUpdateReviewQueueState_SingleStatement(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()
	sess := createTestSession("queue-state-test")
	require.NoError(t, repo.Create(ctx, sess))

	// Install interceptor that counts ent Query (SELECT) operations.
	var selectCount int32
	repo.client.Intercept(ssent.InterceptFunc(func(next ssent.Querier) ssent.Querier {
		return ssent.QuerierFunc(func(ctx context.Context, q ssent.Query) (ssent.Value, error) {
			atomic.AddInt32(&selectCount, 1)
			return next.Query(ctx, q)
		})
	}))

	now := time.Now()
	err := repo.UpdateReviewQueueState(ctx, sess.Title, now, time.Time{}, now, "sig-abc")
	require.NoError(t, err)

	got := atomic.LoadInt32(&selectCount)
	require.Equal(t, int32(0), got,
		"UpdateReviewQueueState must issue a single UPDATE, not SELECT+UPDATE (got %d SELECT queries)", got)

	// Verify the update actually landed.
	updated, err := repo.Get(ctx, sess.Title)
	require.NoError(t, err)
	assert.Equal(t, "sig-abc", updated.LastPromptSignature)
	assert.False(t, updated.LastPromptDetected.IsZero())
}

// TestUpdateSessionMetadata_SingleStatement enforces that UpdateSessionMetadata issues a
// single direct UPDATE and does NOT perform a SELECT first, and that it does not create a
// worktree/diffstats/tags/claude_session row as a side effect (unlike the full Update path).
func TestUpdateSessionMetadata_SingleStatement(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()
	sess := createTestSession("metadata-test")
	require.NoError(t, repo.Create(ctx, sess))

	var selectCount int32
	repo.client.Intercept(ssent.InterceptFunc(func(next ssent.Querier) ssent.Querier {
		return ssent.QuerierFunc(func(ctx context.Context, q ssent.Query) (ssent.Value, error) {
			atomic.AddInt32(&selectCount, 1)
			return next.Query(ctx, q)
		})
	}))

	newTitle := "metadata-test-renamed"
	category := "urgent"
	note := "left this waiting on CI"
	workingDir := "/tmp/new-workdir"
	err := repo.UpdateSessionMetadata(ctx, sess.Title, &newTitle, &category, &note, &workingDir)
	require.NoError(t, err)

	got := atomic.LoadInt32(&selectCount)
	require.Equal(t, int32(0), got,
		"UpdateSessionMetadata must issue a single UPDATE, not SELECT+UPDATE (got %d SELECT queries)", got)

	updated, err := repo.Get(ctx, newTitle)
	require.NoError(t, err)
	assert.Equal(t, newTitle, updated.Title)
	assert.Equal(t, category, updated.Category)
	assert.Equal(t, note, updated.Note)
	assert.Equal(t, workingDir, updated.WorkingDir)
	assert.Empty(t, updated.Worktree.RepoPath, "narrow metadata update must not create a worktree row")
	assert.Zero(t, updated.DiffStats.Added+updated.DiffStats.Removed, "narrow metadata update must not touch diff stats")
	assert.Empty(t, updated.Tags, "narrow metadata update must not touch tags")
	assert.Empty(t, updated.ClaudeSession.ConversationUUID, "narrow metadata update must not create a claude_session row")
}

// TestUpdateSessionMetadata_ClearsNoteToEmpty verifies that passing a non-nil empty-string
// note clears the stored note rather than being skipped as "unset" — matching Update's
// unconditional SetNote(data.Note) semantics for the same reason (an empty note is a
// meaningful cleared state).
func TestUpdateSessionMetadata_ClearsNoteToEmpty(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()
	sess := createTestSession("note-clear-test")
	sess.Note = "stale reminder"
	require.NoError(t, repo.Create(ctx, sess))

	empty := ""
	err := repo.UpdateSessionMetadata(ctx, sess.Title, nil, nil, &empty, nil)
	require.NoError(t, err)

	updated, err := repo.Get(ctx, sess.Title)
	require.NoError(t, err)
	assert.Equal(t, "", updated.Note, "note must be cleared to empty, not left stale")
}

// TestUpdateSessionMetadata_NilFieldsLeaveExistingValuesUntouched verifies that a nil
// pointer for category/workingDir leaves the existing DB value untouched — the guarded
// "not provided" semantics the four sibling narrow-update methods all share.
func TestUpdateSessionMetadata_NilFieldsLeaveExistingValuesUntouched(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()
	sess := createTestSession("guarded-fields-test")
	sess.Category = "original-category"
	sess.WorkingDir = "/original/workdir"
	require.NoError(t, repo.Create(ctx, sess))

	note := "only the note changed"
	err := repo.UpdateSessionMetadata(ctx, sess.Title, nil, nil, &note, nil)
	require.NoError(t, err)

	updated, err := repo.Get(ctx, sess.Title)
	require.NoError(t, err)
	assert.Equal(t, note, updated.Note)
	assert.Equal(t, "original-category", updated.Category, "category must be untouched when not part of the request")
	assert.Equal(t, "/original/workdir", updated.WorkingDir, "working_dir must be untouched when not part of the request")
}

// TestUpdateSessionMetadata_SessionNotFound verifies the same not-found error shape as its
// sibling narrow-update methods when the row doesn't exist.
func TestUpdateSessionMetadata_SessionNotFound(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()

	ctx := context.Background()
	note := "irrelevant"
	err := repo.UpdateSessionMetadata(ctx, "does-not-exist", nil, nil, &note, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func createTestEntRepository(t *testing.T) (*EntRepository, func()) {
	// Create temporary database file with unique name using timestamp to avoid conflicts
	tmpDir := t.TempDir()
	// Use nanosecond timestamp to ensure uniqueness even for rapidly running tests
	uniqueName := fmt.Sprintf("test-%d.db", time.Now().UnixNano())
	dbPath := filepath.Join(tmpDir, uniqueName)

	t.Logf("Creating test repository with database at: %s", dbPath)

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	// Verify database is empty
	ctx := context.Background()
	sessions, err := repo.List(ctx)
	require.NoError(t, err, "Failed to list sessions")
	require.Empty(t, sessions, "Database should be empty but has %d sessions", len(sessions))

	cleanup := func() {
		// Close the database connection
		repo.Close()
		// Remove all SQLite files (main db + WAL files)
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}

	return repo, cleanup
}
