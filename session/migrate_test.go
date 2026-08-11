package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRollbackMigration_Success tests successful rollback
func TestRollbackMigration_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create backup and a stub database file (simulating any DB file on disk)
	backupPath := filepath.Join(tmpDir, "backup.json")
	sessions := []InstanceData{createTestSession("session-1")}
	writeJSONFile(t, backupPath, sessions)

	dbPath := filepath.Join(tmpDir, "sessions.db")
	err := os.WriteFile(dbPath, []byte("stub"), 0644)
	require.NoError(t, err)

	// Verify DB file exists
	_, err = os.Stat(dbPath)
	require.NoError(t, err)

	// Rollback migration
	err = RollbackMigration(backupPath, dbPath)
	assert.NoError(t, err, "Rollback should succeed")

	// Verify DB file removed
	_, err = os.Stat(dbPath)
	assert.Error(t, err, "DB file should be removed after rollback")

	// Verify backup still exists
	_, err = os.Stat(backupPath)
	assert.NoError(t, err, "Backup should still exist")
}

// TestRollbackMigration_NonExistentBackup tests rollback with missing backup
func TestRollbackMigration_NonExistentBackup(t *testing.T) {
	tmpDir := t.TempDir()

	backupPath := filepath.Join(tmpDir, "nonexistent-backup.json")
	dbPath := filepath.Join(tmpDir, "sessions.db")

	// Rollback should fail
	err := RollbackMigration(backupPath, dbPath)
	assert.Error(t, err, "Should fail with nonexistent backup")
	assert.Contains(t, err.Error(), "backup file not found")
}

// TestMigrateJSONToEnt_Success tests successful migration to Ent
func TestMigrateJSONToEnt_Success(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("ent-session-1"),
		createTestSession("ent-session-2"),
		createTestSession("ent-session-3"),
	}
	writeJSONFile(t, jsonPath, sessions)

	entDBPath := filepath.Join(tmpDir, "ent_sessions.db")

	result, err := MigrateJSONToEnt(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: entDBPath,
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

	// Verify Ent database created with sessions
	repo, err := NewEntRepository(WithDatabasePath(entDBPath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()
	migratedSessions, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, migratedSessions, 3)
}

// TestMigrateJSONToEnt_EmptyJSON tests migration with empty JSON
func TestMigrateJSONToEnt_EmptyJSON(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "empty.json")
	writeJSONFile(t, jsonPath, []InstanceData{})

	entDBPath := filepath.Join(tmpDir, "ent_sessions.db")

	result, err := MigrateJSONToEnt(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: entDBPath,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalSessions)
	assert.Equal(t, 0, result.MigratedSessions)
	assert.False(t, result.BackupCreated, "No backup needed for empty file")
}

// TestMigrateJSONToEnt_DryRun tests dry run mode
func TestMigrateJSONToEnt_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("ent-session-1"),
		createTestSession("ent-session-2"),
	}
	writeJSONFile(t, jsonPath, sessions)

	entDBPath := filepath.Join(tmpDir, "ent_sessions.db")

	result, err := MigrateJSONToEnt(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: entDBPath,
		DryRun:     true,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalSessions)
	assert.Equal(t, 2, result.MigratedSessions) // In dry run, assumes all would migrate
	assert.False(t, result.BackupCreated, "No backup in dry run")

	// Verify no Ent database created
	_, err = os.Stat(entDBPath)
	assert.Error(t, err, "Ent database should not exist in dry run")
}

// TestMigrateJSONToEnt_ForceOverwrite tests overwriting existing Ent database
func TestMigrateJSONToEnt_ForceOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{createTestSession("new-ent-session")}
	writeJSONFile(t, jsonPath, sessions)

	entDBPath := filepath.Join(tmpDir, "ent_sessions.db")

	// Pre-populate the Ent database with old data
	oldRepo, err := NewEntRepository(WithDatabasePath(entDBPath))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, oldRepo.Create(ctx, createTestSession("old-ent-session")))
	oldRepo.Close()

	// Migration without force should fail
	_, err = MigrateJSONToEnt(MigrationOptions{
		JSONPath:       jsonPath,
		SQLitePath:     entDBPath,
		ForceOverwrite: false,
	})
	assert.Error(t, err, "Should fail without ForceOverwrite")

	// Migration with force should succeed
	result, err := MigrateJSONToEnt(MigrationOptions{
		JSONPath:       jsonPath,
		SQLitePath:     entDBPath,
		ForceOverwrite: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.MigratedSessions)

	// Verify only new session in DB
	newRepo, err := NewEntRepository(WithDatabasePath(entDBPath))
	require.NoError(t, err)
	defer newRepo.Close()

	newSession, err := newRepo.Get(ctx, "new-ent-session")
	require.NoError(t, err)
	assert.Equal(t, "new-ent-session", newSession.Title)
}

// TestValidateEntMigration_Success tests successful Ent migration validation
func TestValidateEntMigration_Success(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("ent-validate-1"),
		createTestSession("ent-validate-2"),
	}
	writeJSONFile(t, jsonPath, sessions)

	entDBPath := filepath.Join(tmpDir, "ent_sessions.db")

	_, err := MigrateJSONToEnt(MigrationOptions{
		JSONPath:   jsonPath,
		SQLitePath: entDBPath,
	})
	require.NoError(t, err)

	err = ValidateEntMigration(jsonPath, entDBPath)
	assert.NoError(t, err, "Validation should pass for successful Ent migration")
}

// TestValidateEntMigration_CountMismatch tests Ent validation with count mismatch
func TestValidateEntMigration_CountMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	jsonPath := filepath.Join(tmpDir, "sessions.json")
	sessions := []InstanceData{
		createTestSession("ent-s1"),
		createTestSession("ent-s2"),
		createTestSession("ent-s3"),
	}
	writeJSONFile(t, jsonPath, sessions)

	// Create Ent DB with only 2 sessions
	entDBPath := filepath.Join(tmpDir, "ent_sessions.db")
	repo, err := NewEntRepository(WithDatabasePath(entDBPath))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, createTestSession("ent-s1")))
	require.NoError(t, repo.Create(ctx, createTestSession("ent-s2")))
	repo.Close()

	err = ValidateEntMigration(jsonPath, entDBPath)
	assert.Error(t, err, "Validation should fail with count mismatch")
	assert.Contains(t, err.Error(), "session count mismatch")
}

// Helper function to write JSON file
func writeJSONFile(t *testing.T, path string, sessions []InstanceData) {
	data, err := json.MarshalIndent(sessions, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	require.NoError(t, err)
}
