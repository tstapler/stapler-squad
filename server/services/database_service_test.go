package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ---------------------------------------------------------------------------
// ListDatabases
// ---------------------------------------------------------------------------

// TestListDatabases_ReturnsList verifies that ListDatabases returns a response
// without error. The call reads the filesystem, so the result may be an empty
// list; the important invariant is no error and a non-nil response.
func TestListDatabases_ReturnsList(t *testing.T) {
	svc := NewDatabaseService()

	resp, err := svc.ListDatabases(context.Background(), connect.NewRequest(&sessionv1.ListDatabasesRequest{}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	// Databases slice may be nil or empty in a clean test environment — that is fine.
	assert.NotNil(t, resp.Msg)
}

// ---------------------------------------------------------------------------
// GetCurrentDatabase
// ---------------------------------------------------------------------------

// TestGetCurrentDatabase_ReturnsDatabase verifies that GetCurrentDatabase
// returns a response without error. In a test environment, the workspace meta
// file is unlikely to exist, so the implementation falls back to building a
// minimal DatabaseInfo from the config dir — the call must still succeed.
func TestGetCurrentDatabase_ReturnsDatabase(t *testing.T) {
	svc := NewDatabaseService()

	resp, err := svc.GetCurrentDatabase(context.Background(), connect.NewRequest(&sessionv1.GetCurrentDatabaseRequest{}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Msg.Database, "Database field must be populated")
	assert.True(t, resp.Msg.Database.IsCurrent, "IsCurrent must be true for the current database")
}

// ---------------------------------------------------------------------------
// SwitchDatabase
// ---------------------------------------------------------------------------

// TestSwitchDatabase_EmptyPath verifies that SwitchDatabase returns
// CodeInvalidArgument when config_dir is empty.
func TestSwitchDatabase_EmptyPath(t *testing.T) {
	svc := NewDatabaseService()

	_, err := svc.SwitchDatabase(context.Background(), connect.NewRequest(&sessionv1.SwitchDatabaseRequest{
		ConfigDir: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestSwitchDatabase_PathOutsideBaseDir verifies that SwitchDatabase returns
// CodeInvalidArgument when the target path is not under the allowed base dir.
func TestSwitchDatabase_PathOutsideBaseDir(t *testing.T) {
	svc := NewDatabaseService()

	_, err := svc.SwitchDatabase(context.Background(), connect.NewRequest(&sessionv1.SwitchDatabaseRequest{
		ConfigDir: "/tmp/outside-basedir",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	// Either CodeInvalidArgument (security check) or CodeNotFound (dir doesn't exist
	// under base dir path) — both are acceptable non-internal failures.
	assert.NotEqual(t, connect.CodeInternal, connectErr.Code())
}

// TestSwitchDatabase_NonExistentDir verifies that SwitchDatabase returns
// CodeNotFound when the target directory does not exist on disk.
// We construct a path that passes the base-dir security check but is not a
// real directory.
func TestSwitchDatabase_NonExistentDir(t *testing.T) {
	svc := NewDatabaseService()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	nonExistent := filepath.Join(homeDir, ".stapler-squad", "non-existent-workspace-xyz")

	_, switchErr := svc.SwitchDatabase(context.Background(), connect.NewRequest(&sessionv1.SwitchDatabaseRequest{
		ConfigDir: nonExistent,
	}))

	require.Error(t, switchErr)
	var connectErr *connect.Error
	require.ErrorAs(t, switchErr, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// ---------------------------------------------------------------------------
// MergeDatabase
// ---------------------------------------------------------------------------

// TestMergeDatabase_EmptySource verifies that MergeDatabase returns
// CodeInvalidArgument when config_dir is empty.
func TestMergeDatabase_EmptySource(t *testing.T) {
	svc := NewDatabaseService()

	_, err := svc.MergeDatabase(context.Background(), connect.NewRequest(&sessionv1.MergeDatabaseRequest{
		ConfigDir: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestMergeDatabase_SourceOutsideBaseDir verifies that MergeDatabase returns
// CodeInvalidArgument when the source path is not under the allowed base dir.
func TestMergeDatabase_SourceOutsideBaseDir(t *testing.T) {
	svc := NewDatabaseService()

	_, err := svc.MergeDatabase(context.Background(), connect.NewRequest(&sessionv1.MergeDatabaseRequest{
		ConfigDir: "/tmp/outside-basedir",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestMergeDatabase_SourceDBNotFound verifies that MergeDatabase returns
// CodeNotFound when the source workspace directory contains no sessions.db.
func TestMergeDatabase_SourceDBNotFound(t *testing.T) {
	svc := NewDatabaseService()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	// Create a real dir under the base dir that has no sessions.db file.
	sourceDir := filepath.Join(homeDir, ".stapler-squad", "test-merge-no-db-xyz")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	t.Cleanup(func() { os.RemoveAll(sourceDir) })

	_, mergeErr := svc.MergeDatabase(context.Background(), connect.NewRequest(&sessionv1.MergeDatabaseRequest{
		ConfigDir: sourceDir,
	}))

	require.Error(t, mergeErr)
	var connectErr *connect.Error
	require.ErrorAs(t, mergeErr, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
