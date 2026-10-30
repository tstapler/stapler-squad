package analytics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAnalyticsDBWithDriver_ShouldCreateIncrementalAutoVacuumDB_When_DatabaseIsNew(t *testing.T) {
	dir := t.TempDir()
	client, drv, err := OpenAnalyticsDBWithDriver(context.Background(), dir)
	require.NoError(t, err)
	require.NotNil(t, drv)
	require.NoError(t, client.Close())

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "analytics.db")+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	var mode int
	require.NoError(t, db.QueryRow("PRAGMA auto_vacuum").Scan(&mode))
	require.Equal(t, 2, mode)
}
