package session

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEntRepository_ShouldCreateIncrementalAutoVacuumDB_When_DatabaseIsNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	repo, err := NewEntRepository(WithDatabasePath(path))
	require.NoError(t, err)
	require.NotNil(t, repo.MaintenanceDriver(), "driver exposed for dbmaint")
	require.NoError(t, repo.Close())

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	var mode int
	require.NoError(t, db.QueryRow("PRAGMA auto_vacuum").Scan(&mode))
	require.Equal(t, 2, mode, "auto_vacuum=INCREMENTAL must be set before the first table is created")
}
