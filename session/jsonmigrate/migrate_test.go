package jsonmigrate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

type fakeRepo struct {
	rows      []domain.InstanceData
	createErr map[string]error
	listErr   error
	closed    bool
}

func (f *fakeRepo) Create(_ context.Context, d domain.InstanceData) error {
	if err := f.createErr[d.Title]; err != nil {
		return err
	}
	f.rows = append(f.rows, d)
	return nil
}

func (f *fakeRepo) List(context.Context) ([]domain.InstanceData, error) { return f.rows, f.listErr }

func (f *fakeRepo) Close() error { f.closed = true; return nil }

// opener returns an OpenRepository that hands out repo and records the paths it was asked for.
func opener(repo *fakeRepo, opened *[]string) OpenRepository {
	return func(dbPath string) (Repository, error) {
		*opened = append(*opened, dbPath)
		return repo, nil
	}
}

func failingOpener(err error) OpenRepository {
	return func(string) (Repository, error) { return nil, err }
}

func session(title string) domain.InstanceData {
	return domain.InstanceData{Title: title, Path: "/p/" + title, Branch: "b-" + title, Program: "claude", Status: domain.Active}
}

func writeJSON(t *testing.T, path string, sessions []domain.InstanceData) {
	t.Helper()
	data, err := json.Marshal(sessions)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestRollbackMigration_should_RemoveDBAndKeepBackup_When_BackupExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup.json")
	db := filepath.Join(dir, "sessions.db")
	require.NoError(t, os.WriteFile(backup, []byte("[]"), 0o600))
	require.NoError(t, os.WriteFile(db, []byte("stub"), 0o600))

	require.NoError(t, RollbackMigration(backup, db))

	_, err := os.Stat(db)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(backup)
	assert.NoError(t, err)
}

func TestRollbackMigration_should_Succeed_When_DBAlreadyGone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup.json")
	require.NoError(t, os.WriteFile(backup, []byte("[]"), 0o600))
	assert.NoError(t, RollbackMigration(backup, filepath.Join(dir, "absent.db")))
}

func TestRollbackMigration_should_Fail_When_BackupMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	db := filepath.Join(dir, "sessions.db")
	require.NoError(t, os.WriteFile(db, []byte("stub"), 0o600))

	err := RollbackMigration(filepath.Join(dir, "none.json"), db)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup file not found")
	_, statErr := os.Stat(db)
	assert.NoError(t, statErr, "DB must be untouched when the backup is missing")
}

func TestRollbackMigration_should_Fail_When_DBPathIsNonEmptyDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	backup := filepath.Join(dir, "backup.json")
	require.NoError(t, os.WriteFile(backup, []byte("[]"), 0o600))
	db := filepath.Join(dir, "dbdir")
	require.NoError(t, os.MkdirAll(filepath.Join(db, "child"), 0o750))

	err := RollbackMigration(backup, db)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove SQLite database")
}

func TestMigrateJSONToEnt_should_CreateEverySessionAndBackup_When_JSONValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a"), session("b"), session("c")})
	dbPath := filepath.Join(dir, "nested", "ent.db")
	repo := &fakeRepo{}
	var opened []string

	res, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: dbPath}, opener(repo, &opened))

	require.NoError(t, err)
	assert.Equal(t, 3, res.TotalSessions)
	assert.Equal(t, 3, res.MigratedSessions)
	assert.Zero(t, res.SkippedSessions)
	assert.Empty(t, res.Errors)
	assert.True(t, res.BackupCreated)
	assert.Equal(t, dbPath, res.SQLiteDatabasePath)
	assert.Equal(t, []string{dbPath}, opened)
	assert.True(t, repo.closed, "repository must be closed")
	assert.Len(t, repo.rows, 3)
	_, statErr := os.Stat(filepath.Dir(dbPath))
	assert.NoError(t, statErr, "the database directory must be created before opening")

	orig, origErr := os.ReadFile(jsonPath)
	require.NoError(t, origErr)
	backup, readErr := os.ReadFile(res.BackupPath)
	require.NoError(t, readErr)
	assert.Equal(t, orig, backup)
	assert.True(t, strings.HasPrefix(filepath.Base(res.BackupPath), "sessions.json.backup_"), res.BackupPath)
	assert.True(t, strings.HasSuffix(res.BackupPath, ".json"))
}

func TestMigrateJSONToEnt_should_UseExplicitBackupPath_When_Set(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})
	backup := filepath.Join(dir, "my.bak")

	res, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: filepath.Join(dir, "e.db"), BackupPath: backup},
		opener(&fakeRepo{}, new([]string)))

	require.NoError(t, err)
	assert.Equal(t, backup, res.BackupPath)
	_, statErr := os.Stat(backup)
	assert.NoError(t, statErr)
}

func TestMigrateJSONToEnt_should_SkipAndRecordError_When_CreateFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("good"), session("bad"), session("good2")})
	repo := &fakeRepo{createErr: map[string]error{"bad": errors.New("constraint")}}

	res, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: filepath.Join(dir, "e.db")}, opener(repo, new([]string)))

	require.NoError(t, err)
	assert.Equal(t, 2, res.MigratedSessions)
	assert.Equal(t, 1, res.SkippedSessions)
	require.Len(t, res.Errors, 1)
	assert.Equal(t, "Failed to migrate session 'bad': constraint", res.Errors[0])
}

func TestMigrateJSONToEnt_should_ReturnZeroResultWithoutOpening_When_JSONEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "empty.json")
	writeJSON(t, jsonPath, []domain.InstanceData{})
	var opened []string

	res, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: filepath.Join(dir, "e.db")}, opener(&fakeRepo{}, &opened))

	require.NoError(t, err)
	assert.Zero(t, res.TotalSessions)
	assert.False(t, res.BackupCreated)
	assert.Empty(t, opened)
}

func TestMigrateJSONToEnt_should_ChangeNothingOnDisk_When_DryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a"), session("b")})
	dbPath := filepath.Join(dir, "e.db")
	var opened []string

	res, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: dbPath, DryRun: true}, opener(&fakeRepo{}, &opened))

	require.NoError(t, err)
	assert.Equal(t, 2, res.MigratedSessions, "a dry run reports every session as would-migrate")
	assert.False(t, res.BackupCreated)
	assert.Empty(t, opened)
	entries, dirErr := os.ReadDir(dir)
	require.NoError(t, dirErr)
	assert.Len(t, entries, 1, "only the source JSON may exist")
}

func TestMigrateJSONToEnt_should_RefuseExistingDB_When_NoForceOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})
	dbPath := filepath.Join(dir, "e.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("old"), 0o600))
	var opened []string

	_, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: dbPath}, opener(&fakeRepo{}, &opened))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Empty(t, opened)
	data, readErr := os.ReadFile(dbPath)
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(data), "an existing database must survive a refused migration")
}

func TestMigrateJSONToEnt_should_RemoveExistingDB_When_ForceOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})
	dbPath := filepath.Join(dir, "e.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("old"), 0o600))

	_, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: dbPath, ForceOverwrite: true}, opener(&fakeRepo{}, new([]string)))

	require.NoError(t, err)
	_, statErr := os.Stat(dbPath)
	assert.True(t, os.IsNotExist(statErr), "the fake opener never recreates it, so absence proves the old file was removed")
}

func TestMigrateJSONToEnt_should_KeepExistingDB_When_ForceOverwriteDryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})
	dbPath := filepath.Join(dir, "e.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("old"), 0o600))

	_, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: dbPath, ForceOverwrite: true, DryRun: true}, opener(&fakeRepo{}, new([]string)))

	require.NoError(t, err)
	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr)
}

func TestMigrateJSONToEnt_should_FailBeforeOpening_When_FilesystemStepFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("file"), 0o600))
	nonEmptyDir := filepath.Join(dir, "occupied")
	require.NoError(t, os.MkdirAll(filepath.Join(nonEmptyDir, "child"), 0o750))

	cases := map[string]struct {
		opts MigrationOptions
		want string
	}{
		"backup path unwritable":          {MigrationOptions{JSONPath: jsonPath, SQLitePath: filepath.Join(dir, "e.db"), BackupPath: filepath.Join(blocker, "x.bak")}, "failed to create backup"},
		"database dir uncreatable":        {MigrationOptions{JSONPath: jsonPath, SQLitePath: filepath.Join(blocker, "sub", "e.db"), BackupPath: filepath.Join(dir, "ok.bak")}, "failed to create database directory"},
		"existing database not removable": {MigrationOptions{JSONPath: jsonPath, SQLitePath: nonEmptyDir, BackupPath: filepath.Join(dir, "ok2.bak"), ForceOverwrite: true}, "failed to remove existing database"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var opened []string

			_, err := MigrateJSONToEnt(tc.opts, opener(&fakeRepo{}, &opened))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, opened)
		})
	}
}

func TestMigrateJSONToEnt_should_Fail_When_OpenFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "sessions.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})

	_, err := MigrateJSONToEnt(MigrationOptions{JSONPath: jsonPath, SQLitePath: filepath.Join(dir, "e.db")}, failingOpener(errors.New("locked")))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize Ent repository: locked")
}

func TestMigrateJSONToEnt_should_Fail_When_JSONMissingOrMalformed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	open := opener(&fakeRepo{}, new([]string))

	_, err := MigrateJSONToEnt(MigrationOptions{JSONPath: filepath.Join(dir, "none.json")}, open)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read JSON file")

	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{not json"), 0o600))
	_, err = MigrateJSONToEnt(MigrationOptions{JSONPath: bad}, open)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal JSON data")
}

func TestValidateEntMigration_should_Pass_When_RowsMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "s.json")
	rows := []domain.InstanceData{session("a"), session("b")}
	writeJSON(t, jsonPath, rows)
	repo := &fakeRepo{rows: rows}
	var opened []string

	require.NoError(t, ValidateEntMigration(jsonPath, "db-path", opener(repo, &opened)))
	assert.Equal(t, []string{"db-path"}, opened)
	assert.True(t, repo.closed)
}

func TestValidateEntMigration_should_ReportCountMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "s.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a"), session("b")})

	err := ValidateEntMigration(jsonPath, "db", opener(&fakeRepo{rows: []domain.InstanceData{session("a")}}, new([]string)))

	require.Error(t, err)
	assert.Equal(t, "session count mismatch: JSON has 2, Ent has 1", err.Error())
}

func TestValidateEntMigration_should_CountMissingAndMismatchedSessions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "s.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a"), session("b"), session("c")})
	changed := session("b")
	changed.Branch = "other"
	repo := &fakeRepo{rows: []domain.InstanceData{session("a"), changed, session("z")}}

	err := ValidateEntMigration(jsonPath, "db", opener(repo, new([]string)))

	require.Error(t, err)
	assert.Equal(t, "validation failed: 1 missing, 1 mismatched", err.Error())
}

func TestValidateEntMigration_should_DetectEachComparedField(t *testing.T) {
	t.Parallel()
	mutators := map[string]func(*domain.InstanceData){
		"path":    func(d *domain.InstanceData) { d.Path = "/x" },
		"branch":  func(d *domain.InstanceData) { d.Branch = "x" },
		"status":  func(d *domain.InstanceData) { d.Status = domain.Paused },
		"program": func(d *domain.InstanceData) { d.Program = "x" },
	}
	for name, mutate := range mutators {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			jsonPath := filepath.Join(t.TempDir(), "s.json")
			writeJSON(t, jsonPath, []domain.InstanceData{session("a")})
			got := session("a")
			mutate(&got)

			err := ValidateEntMigration(jsonPath, "db", opener(&fakeRepo{rows: []domain.InstanceData{got}}, new([]string)))

			require.Error(t, err)
			assert.Equal(t, "validation failed: 0 missing, 1 mismatched", err.Error())
		})
	}
}

func TestValidateEntMigration_should_PropagateOpenListAndReadErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "s.json")
	writeJSON(t, jsonPath, []domain.InstanceData{session("a")})

	err := ValidateEntMigration(jsonPath, "db", failingOpener(errors.New("nope")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open Ent database: nope")

	err = ValidateEntMigration(jsonPath, "db", opener(&fakeRepo{listErr: errors.New("boom")}, new([]string)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list Ent sessions: boom")

	err = ValidateEntMigration(filepath.Join(dir, "none.json"), "db", opener(&fakeRepo{}, new([]string)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read JSON file")

	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{"), 0o600))
	err = ValidateEntMigration(bad, "db", opener(&fakeRepo{}, new([]string)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal JSON data")
}
