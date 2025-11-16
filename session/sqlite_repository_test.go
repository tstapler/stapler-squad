package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteRepository_CreateAndGet tests basic create and get operations
func TestSQLiteRepository_CreateAndGet(t *testing.T) {
	repo, cleanup := createTestRepository(t)
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

// TestSQLiteRepository_CreateDuplicate tests duplicate title handling
func TestSQLiteRepository_CreateDuplicate(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("duplicate-test")

	// First create should succeed
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Second create with same title should fail
	err = repo.Create(ctx, session)
	assert.Error(t, err, "Should not allow duplicate titles")
}

// TestSQLiteRepository_Update tests session update operations
func TestSQLiteRepository_Update(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial session
	session := createTestSession("update-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Modify session data
	session.Branch = "feature-updated"
	session.Status = Paused
	session.Category = "Updated Category"

	// Update session
	err = repo.Update(ctx, session)
	require.NoError(t, err)

	// Verify updates
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.Equal(t, "feature-updated", retrieved.Branch)
	assert.Equal(t, Paused, retrieved.Status)
	assert.Equal(t, "Updated Category", retrieved.Category)
}

// TestSQLiteRepository_UpdateNonExistent tests updating non-existent session
func TestSQLiteRepository_UpdateNonExistent(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	session := createTestSession("nonexistent")
	err := repo.Update(ctx, session)
	assert.Error(t, err, "Should fail to update non-existent session")
}

// TestSQLiteRepository_Delete tests session deletion
func TestSQLiteRepository_Delete(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	session := createTestSession("delete-test")
	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Verify session exists
	_, err = repo.Get(ctx, session.Title)
	require.NoError(t, err)

	// Delete session
	err = repo.Delete(ctx, session.Title)
	require.NoError(t, err)

	// Verify session no longer exists
	_, err = repo.Get(ctx, session.Title)
	assert.Error(t, err, "Session should not exist after deletion")
}

// TestSQLiteRepository_List tests listing all sessions
func TestSQLiteRepository_List(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple sessions
	sessions := []InstanceData{
		createTestSession("list-test-1"),
		createTestSession("list-test-2"),
		createTestSession("list-test-3"),
	}

	for _, session := range sessions {
		err := repo.Create(ctx, session)
		require.NoError(t, err)
	}

	// List all sessions
	retrieved, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, retrieved, 3)

	// Verify all sessions present
	titles := make(map[string]bool)
	for _, s := range retrieved {
		titles[s.Title] = true
	}
	assert.True(t, titles["list-test-1"])
	assert.True(t, titles["list-test-2"])
	assert.True(t, titles["list-test-3"])
}

// TestSQLiteRepository_ListByStatus tests filtering by status
func TestSQLiteRepository_ListByStatus(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create sessions with different statuses
	running1 := createTestSession("running-1")
	running1.Status = Running
	running2 := createTestSession("running-2")
	running2.Status = Running
	paused := createTestSession("paused-1")
	paused.Status = Paused

	require.NoError(t, repo.Create(ctx, running1))
	require.NoError(t, repo.Create(ctx, running2))
	require.NoError(t, repo.Create(ctx, paused))

	// List only running sessions
	runningSessions, err := repo.ListByStatus(ctx, Running)
	require.NoError(t, err)
	assert.Len(t, runningSessions, 2)

	// Verify all are running
	for _, s := range runningSessions {
		assert.Equal(t, Running, s.Status)
	}

	// List only paused sessions
	pausedSessions, err := repo.ListByStatus(ctx, Paused)
	require.NoError(t, err)
	assert.Len(t, pausedSessions, 1)
	assert.Equal(t, "paused-1", pausedSessions[0].Title)
}

// TestSQLiteRepository_Tags tests tag management
func TestSQLiteRepository_Tags(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with tags
	session := createTestSession("tag-test")
	session.Tags = []string{"Frontend", "Urgent", "React"}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify tags
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Frontend", "Urgent", "React"}, retrieved.Tags)

	// Update tags
	session.Tags = []string{"Backend", "Low-Priority"}
	err = repo.Update(ctx, session)
	require.NoError(t, err)

	// Verify updated tags
	retrieved, err = repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Backend", "Low-Priority"}, retrieved.Tags)
}

// TestSQLiteRepository_ListByTag tests filtering by tag
func TestSQLiteRepository_ListByTag(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create sessions with tags
	frontend1 := createTestSession("frontend-1")
	frontend1.Tags = []string{"Frontend", "React"}
	frontend2 := createTestSession("frontend-2")
	frontend2.Tags = []string{"Frontend", "Vue"}
	backend := createTestSession("backend-1")
	backend.Tags = []string{"Backend", "Go"}

	require.NoError(t, repo.Create(ctx, frontend1))
	require.NoError(t, repo.Create(ctx, frontend2))
	require.NoError(t, repo.Create(ctx, backend))

	// List sessions with "Frontend" tag
	frontendSessions, err := repo.ListByTag(ctx, "Frontend")
	require.NoError(t, err)
	assert.Len(t, frontendSessions, 2)

	// Verify all have Frontend tag
	for _, s := range frontendSessions {
		assert.Contains(t, s.Tags, "Frontend")
	}

	// List sessions with "Backend" tag
	backendSessions, err := repo.ListByTag(ctx, "Backend")
	require.NoError(t, err)
	assert.Len(t, backendSessions, 1)
	assert.Equal(t, "backend-1", backendSessions[0].Title)
}

// TestSQLiteRepository_UpdateTimestamps tests efficient timestamp updates
func TestSQLiteRepository_UpdateTimestamps(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	session := createTestSession("timestamp-test")
	originalTime := time.Now().Add(-1 * time.Hour)
	session.LastTerminalUpdate = originalTime
	session.LastMeaningfulOutput = originalTime

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Update timestamps
	newTime := time.Now()
	newSignature := "new-signature-abc123"
	err = repo.UpdateTimestamps(ctx, session.Title, newTime, newTime, newSignature)
	require.NoError(t, err)

	// Verify timestamp updates
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.WithinDuration(t, newTime, retrieved.LastTerminalUpdate, time.Second)
	assert.WithinDuration(t, newTime, retrieved.LastMeaningfulOutput, time.Second)
	assert.Equal(t, newSignature, retrieved.LastOutputSignature)

	// Verify other fields unchanged
	assert.Equal(t, session.Title, retrieved.Title)
	assert.Equal(t, session.Path, retrieved.Path)
}

// TestSQLiteRepository_Worktree tests worktree data persistence
func TestSQLiteRepository_Worktree(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with worktree
	session := createTestSession("worktree-test")
	session.Worktree = GitWorktreeData{
		RepoPath:      "/home/user/main-repo",
		WorktreePath:  "/home/user/worktrees/feature-branch",
		SessionName:   "feature-session",
		BranchName:    "feature/new-feature",
		BaseCommitSHA: "abc123def456",
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Retrieve and verify worktree
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.Equal(t, session.Worktree.RepoPath, retrieved.Worktree.RepoPath)
	assert.Equal(t, session.Worktree.WorktreePath, retrieved.Worktree.WorktreePath)
	assert.Equal(t, session.Worktree.BranchName, retrieved.Worktree.BranchName)
	assert.Equal(t, session.Worktree.BaseCommitSHA, retrieved.Worktree.BaseCommitSHA)
}

// TestSQLiteRepository_DiffStats tests diff stats persistence
func TestSQLiteRepository_DiffStats(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with diff stats
	session := createTestSession("diffstats-test")
	session.DiffStats = DiffStatsData{
		Added:   150,
		Removed: 75,
		Content: "diff --git a/file.go b/file.go\n...",
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

// TestSQLiteRepository_ClaudeSession tests Claude session data persistence
func TestSQLiteRepository_ClaudeSession(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with Claude data
	session := createTestSession("claude-test")
	session.ClaudeSession = ClaudeSessionData{
		SessionID:      "claude-session-123",
		ConversationID: "conv-456",
		ProjectName:    "my-project",
		LastAttached:   time.Now(),
		Settings: ClaudeSettings{
			AutoReattach:          true,
			PreferredSessionName:  "preferred-session",
			CreateNewOnMissing:    true,
			ShowSessionSelector:   false,
			SessionTimeoutMinutes: 30,
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
	assert.Equal(t, session.ClaudeSession.ProjectName, retrieved.ClaudeSession.ProjectName)
	assert.Equal(t, session.ClaudeSession.Settings.AutoReattach, retrieved.ClaudeSession.Settings.AutoReattach)
	assert.Equal(t, session.ClaudeSession.Settings.SessionTimeoutMinutes, retrieved.ClaudeSession.Settings.SessionTimeoutMinutes)
	assert.Equal(t, session.ClaudeSession.Metadata["key1"], retrieved.ClaudeSession.Metadata["key1"])
	assert.Equal(t, session.ClaudeSession.Metadata["key2"], retrieved.ClaudeSession.Metadata["key2"])
}

// TestSQLiteRepository_CascadeDelete tests cascading deletes
func TestSQLiteRepository_CascadeDelete(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create session with all child data
	session := createTestSession("cascade-test")
	session.Tags = []string{"Frontend", "React"}
	session.Worktree = GitWorktreeData{
		RepoPath:     "/repo",
		WorktreePath: "/worktree",
		SessionName:  "session",
		BranchName:   "branch",
	}
	session.DiffStats = DiffStatsData{
		Added:   10,
		Removed: 5,
		Content: "diff content",
	}
	session.ClaudeSession = ClaudeSessionData{
		SessionID: "claude-123",
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Verify session and child data exist
	retrieved, err := repo.Get(ctx, session.Title)
	require.NoError(t, err)
	assert.NotEmpty(t, retrieved.Tags)
	assert.NotEmpty(t, retrieved.Worktree.RepoPath)
	assert.NotEmpty(t, retrieved.DiffStats.Content)
	assert.NotEmpty(t, retrieved.ClaudeSession.SessionID)

	// Delete session
	err = repo.Delete(ctx, session.Title)
	require.NoError(t, err)

	// Verify session and all child data deleted (no orphaned records)
	_, err = repo.Get(ctx, session.Title)
	assert.Error(t, err, "Session should be deleted")
}

// TestSQLiteRepository_EmptyDatabase tests operations on empty database
func TestSQLiteRepository_EmptyDatabase(t *testing.T) {
	repo, cleanup := createTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// List should return empty slice
	sessions, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, sessions)

	// Get non-existent session should error
	_, err = repo.Get(ctx, "nonexistent")
	assert.Error(t, err)

	// Delete non-existent session should error
	err = repo.Delete(ctx, "nonexistent")
	assert.Error(t, err)

	// ListByStatus should return empty slice
	sessions, err = repo.ListByStatus(ctx, Running)
	require.NoError(t, err)
	assert.Empty(t, sessions)

	// ListByTag should return empty slice
	sessions, err = repo.ListByTag(ctx, "nonexistent-tag")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

// Helper functions

func createTestRepository(t *testing.T) (*SQLiteRepository, func()) {
	// Create temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	repo, err := NewSQLiteRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
	}

	return repo, cleanup
}

func createTestSession(title string) InstanceData {
	now := time.Now()
	return InstanceData{
		Title:      title,
		Path:       "/home/user/project",
		WorkingDir: "/home/user/project",
		Branch:     "main",
		Status:     Running,
		Height:     24,
		Width:      80,
		CreatedAt:  now,
		UpdatedAt:  now,
		Program:    "claude",
		Category:   "Test",
		Tags:       []string{},
	}
}
