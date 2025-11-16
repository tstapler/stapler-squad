package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrateJSONToSQLite_Success tests successful migration
func TestMigrateJSONToSQLite_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test JSON file with sessions
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("session-1"),
		createTestSession("session-2"),
		createTestSession("session-3"),
	}
	writeJSONFile(t, jsonPath, sessions)

	// Prepare SQLite path
	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Run migration
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalSessions)
	assert.Equal(t, 3, result.MigratedSessions)
	assert.Equal(t, 0, result.SkippedSessions)
	assert.Empty(t, result.Errors)
	assert.True(t, result.BackupCreated)
	assert.NotEmpty(t, result.BackupPath)

	// Verify backup created
	_, err = os.Stat(result.BackupPath)
	assert.NoError(t, err, "Backup file should exist")

	// Verify SQLite database created
	_, err = os.Stat(sqlitePath)
	assert.NoError(t, err, "SQLite database should exist")

	// Verify data in SQLite
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()
	migratedSessions, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, migratedSessions, 3)
}

// TestMigrateJSONToSQLite_EmptyJSON tests migration with empty JSON
func TestMigrateJSONToSQLite_EmptyJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty JSON file
	jsonPath := filepath.Join(tmpDir, "empty.json")
	writeJSONFile(t, jsonPath, []InstanceData{})

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Run migration
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalSessions)
	assert.Equal(t, 0, result.MigratedSessions)
	assert.False(t, result.BackupCreated, "No backup needed for empty file")
}

// TestMigrateJSONToSQLite_DryRun tests dry run mode
func TestMigrateJSONToSQLite_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test JSON file
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("session-1"),
		createTestSession("session-2"),
	}
	writeJSONFile(t, jsonPath, sessions)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Run dry run migration
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
		DryRun:     true,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalSessions)
	assert.Equal(t, 2, result.MigratedSessions) // In dry run, assumes all would migrate
	assert.False(t, result.BackupCreated, "No backup in dry run")

	// Verify no backup created
	assert.Empty(t, result.BackupPath)

	// Verify no SQLite database created
	_, err = os.Stat(sqlitePath)
	assert.Error(t, err, "SQLite database should not exist in dry run")
}

// TestMigrateJSONToSQLite_CustomBackupPath tests custom backup path
func TestMigrateJSONToSQLite_CustomBackupPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test JSON file
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{createTestSession("session-1")}
	writeJSONFile(t, jsonPath, sessions)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")
	customBackupPath := filepath.Join(tmpDir, "custom-backup.json")

	// Run migration with custom backup path
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
		BackupPath: customBackupPath,
	})

	require.NoError(t, err)
	assert.True(t, result.BackupCreated)
	assert.Equal(t, customBackupPath, result.BackupPath)

	// Verify backup at custom location
	_, err = os.Stat(customBackupPath)
	assert.NoError(t, err, "Backup should exist at custom path")
}

// TestMigrateJSONToSQLite_ForceOverwrite tests overwriting existing database
func TestMigrateJSONToSQLite_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test JSON file
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{createTestSession("session-new")}
	writeJSONFile(t, jsonPath, sessions)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Create existing database with old data
	oldRepo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	ctx := context.Background()
	oldSession := createTestSession("session-old")
	require.NoError(t, oldRepo.Create(ctx, oldSession))
	oldRepo.Close()

	// First migration without force should fail
	_, err = MigrateJSONToSQLite(MigrationOptions{
		JSONPath:       jsonPath,
		SQLitePath:     sqlitePath,
		ForceOverwrite: false,
	})
	assert.Error(t, err, "Should fail without ForceOverwrite")

	// Second migration with force should succeed
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:       jsonPath,
		SQLitePath:     sqlitePath,
		ForceOverwrite: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.MigratedSessions)

	// Verify old data replaced with new data
	newRepo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	defer newRepo.Close()

	// Old session should not exist
	_, err = newRepo.Get(ctx, "session-old")
	assert.Error(t, err, "Old session should not exist")

	// New session should exist
	newSession, err := newRepo.Get(ctx, "session-new")
	require.NoError(t, err)
	assert.Equal(t, "session-new", newSession.Title)
}

// TestMigrateJSONToSQLite_InvalidJSON tests handling of invalid JSON
func TestMigrateJSONToSQLite_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid JSON file
	jsonPath := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(jsonPath, []byte("invalid json {{{"), 0644)
	require.NoError(t, err)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Migration should fail
	_, err = MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})
	assert.Error(t, err, "Should fail with invalid JSON")
}

// TestMigrateJSONToSQLite_NonExistentJSON tests handling of missing JSON file
func TestMigrateJSONToSQLite_NonExistentJSON(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "nonexistent.json")
	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Migration should fail
	_, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})
	assert.Error(t, err, "Should fail with nonexistent JSON file")
}

// TestMigrateJSONToSQLite_WithTags tests migrating sessions with tags
func TestMigrateJSONToSQLite_WithTags(t *testing.T) {
	tmpDir := t.TempDir()

	// Create JSON with tagged sessions
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	session := createTestSession("tagged-session")
	session.Tags = []string{"Frontend", "Urgent", "React"}
	writeJSONFile(t, jsonPath, []InstanceData{session})

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Run migration
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.MigratedSessions)

	// Verify tags migrated correctly
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()
	migrated, err := repo.Get(ctx, "tagged-session")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Frontend", "Urgent", "React"}, migrated.Tags)
}

// TestMigrateJSONToSQLite_WithWorktree tests migrating sessions with worktree data
func TestMigrateJSONToSQLite_WithWorktree(t *testing.T) {
	tmpDir := t.TempDir()

	// Create JSON with worktree session
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	session := createTestSession("worktree-session")
	session.Worktree = GitWorktreeData{
		RepoPath:      "/repo",
		WorktreePath:  "/worktree",
		SessionName:   "session",
		BranchName:    "feature",
		BaseCommitSHA: "abc123",
	}
	writeJSONFile(t, jsonPath, []InstanceData{session})

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Run migration
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.MigratedSessions)

	// Verify worktree data migrated
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()
	migrated, err := repo.Get(ctx, "worktree-session")
	require.NoError(t, err)
	assert.Equal(t, "/repo", migrated.Worktree.RepoPath)
	assert.Equal(t, "/worktree", migrated.Worktree.WorktreePath)
	assert.Equal(t, "feature", migrated.Worktree.BranchName)
}

// TestValidateMigration_Success tests successful validation
func TestValidateMigration_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and migrate sessions
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("session-1"),
		createTestSession("session-2"),
	}
	writeJSONFile(t, jsonPath, sessions)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	_, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})
	require.NoError(t, err)

	// Validate migration
	err = ValidateMigration(jsonPath, sqlitePath)
	assert.NoError(t, err, "Validation should pass for successful migration")
}

// TestValidateMigration_CountMismatch tests validation with count mismatch
func TestValidateMigration_CountMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create JSON with 3 sessions
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("session-1"),
		createTestSession("session-2"),
		createTestSession("session-3"),
	}
	writeJSONFile(t, jsonPath, sessions)

	// Create SQLite with only 2 sessions
	sqlitePath := filepath.Join(tmpDir, "sessions.db")
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, createTestSession("session-1")))
	require.NoError(t, repo.Create(ctx, createTestSession("session-2")))
	repo.Close()

	// Validation should fail
	err = ValidateMigration(jsonPath, sqlitePath)
	assert.Error(t, err, "Validation should fail with count mismatch")
	assert.Contains(t, err.Error(), "session count mismatch")
}

// TestValidateMigration_MissingSession tests validation with missing session
func TestValidateMigration_MissingSession(t *testing.T) {
	tmpDir := t.TempDir()

	// Create JSON with sessions
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("session-1"),
		createTestSession("session-2"),
	}
	writeJSONFile(t, jsonPath, sessions)

	// Create SQLite with different sessions (same count, different names)
	sqlitePath := filepath.Join(tmpDir, "sessions.db")
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, createTestSession("session-1")))
	require.NoError(t, repo.Create(ctx, createTestSession("different-session")))
	repo.Close()

	// Validation should fail
	err = ValidateMigration(jsonPath, sqlitePath)
	assert.Error(t, err, "Validation should fail with missing session")
	assert.Contains(t, err.Error(), "missing")
}

// TestRollbackMigration_Success tests successful rollback
func TestRollbackMigration_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create backup and SQLite database
	backupPath := filepath.Join(tmpDir, "backup.json")
	sessions := []InstanceData{createTestSession("session-1")}
	writeJSONFile(t, backupPath, sessions)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	repo.Close()

	// Verify SQLite exists
	_, err = os.Stat(sqlitePath)
	require.NoError(t, err)

	// Rollback migration
	err = RollbackMigration(backupPath, sqlitePath)
	assert.NoError(t, err, "Rollback should succeed")

	// Verify SQLite database removed
	_, err = os.Stat(sqlitePath)
	assert.Error(t, err, "SQLite database should be removed")

	// Verify backup still exists
	_, err = os.Stat(backupPath)
	assert.NoError(t, err, "Backup should still exist")
}

// TestRollbackMigration_NonExistentBackup tests rollback with missing backup
func TestRollbackMigration_NonExistentBackup(t *testing.T) {
	tmpDir := t.TempDir()

	backupPath := filepath.Join(tmpDir, "nonexistent-backup.json")
	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Rollback should fail
	err := RollbackMigration(backupPath, sqlitePath)
	assert.Error(t, err, "Should fail with nonexistent backup")
	assert.Contains(t, err.Error(), "backup file not found")
}

// TestMigration_EndToEnd tests complete migration workflow
func TestMigration_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Create JSON with complex session data
	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		{
			Title:      "complex-session",
			Path:       "/project",
			Branch:     "main",
			Status:     Running,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Program:    "claude",
			Tags:       []string{"Frontend", "React"},
			Worktree: GitWorktreeData{
				RepoPath:     "/repo",
				WorktreePath: "/worktree",
				BranchName:   "feature",
			},
			DiffStats: DiffStatsData{
				Added:   100,
				Removed: 50,
				Content: "diff content",
			},
			ClaudeSession: ClaudeSessionData{
				SessionID: "claude-123",
				Settings: ClaudeSettings{
					AutoReattach: true,
				},
			},
		},
	}
	writeJSONFile(t, jsonPath, sessions)

	sqlitePath := filepath.Join(tmpDir, "sessions.db")

	// Step 2: Migrate
	result, err := MigrateJSONToSQLite(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: sqlitePath,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.MigratedSessions)

	// Step 3: Validate
	err = ValidateMigration(jsonPath, sqlitePath)
	assert.NoError(t, err)

	// Step 4: Verify all data integrity
	repo, err := NewSQLiteRepository(WithDatabasePath(sqlitePath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()
	migrated, err := repo.Get(ctx, "complex-session")
	require.NoError(t, err)

	// Verify main fields
	assert.Equal(t, "complex-session", migrated.Title)
	assert.Equal(t, "/project", migrated.Path)
	assert.Equal(t, "main", migrated.Branch)
	assert.Equal(t, Running, migrated.Status)

	// Verify tags
	assert.ElementsMatch(t, []string{"Frontend", "React"}, migrated.Tags)

	// Verify worktree
	assert.Equal(t, "/repo", migrated.Worktree.RepoPath)
	assert.Equal(t, "feature", migrated.Worktree.BranchName)

	// Verify diff stats
	assert.Equal(t, 100, migrated.DiffStats.Added)
	assert.Equal(t, 50, migrated.DiffStats.Removed)

	// Verify Claude session
	assert.Equal(t, "claude-123", migrated.ClaudeSession.SessionID)
	assert.True(t, migrated.ClaudeSession.Settings.AutoReattach)
}

// Helper function to write JSON file
func writeJSONFile(t *testing.T, path string, sessions []InstanceData) {
	data, err := json.MarshalIndent(sessions, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	require.NoError(t, err)
}
