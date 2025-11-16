package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		SessionID:      "claude-123",
		ConversationID: "conv-456",
		ProjectName:    "test-project",
		LastAttached:   time.Now(),
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
	assert.Equal(t, session.ClaudeSession.SessionID, retrieved.ClaudeSession.SessionID)
	assert.Equal(t, session.ClaudeSession.ConversationID, retrieved.ClaudeSession.ConversationID)
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

// Helper function to create a test Ent repository
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
