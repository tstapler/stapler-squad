package analytics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func escapeIndexNames(t *testing.T, dir string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "analytics.db")+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='escape_events' AND name LIKE 'escapeevent_%'")
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())
	return names
}

// A legacy database carries the old indexes; reopening must drop exactly those and keep the live three.
func TestOpenAnalyticsDB_DropsObsoleteEscapeIndexes(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	require.NoError(t, client.Close())

	legacy, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "analytics.db"))
	require.NoError(t, err)
	for _, ddl := range []string{
		"CREATE INDEX escapeevent_session_id ON escape_events (session_id)",
		"CREATE INDEX escapeevent_session_id_stage ON escape_events (session_id, stage)",
		"CREATE INDEX escapeevent_mangled ON escape_events (mangled)",
		"CREATE INDEX escapeevent_sequence_type ON escape_events (sequence_type)",
		"CREATE INDEX escapeevent_project_path ON escape_events (project_path)",
	} {
		_, err := legacy.Exec(ddl)
		require.NoError(t, err)
	}
	require.NoError(t, legacy.Close())
	require.Len(t, escapeIndexNames(t, dir), 8)

	client, err = OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()
	assert.ElementsMatch(t, []string{
		"escapeevent_session_id_session_seq",
		"escapeevent_session_id_wall_time",
		"escapeevent_wall_time",
	}, escapeIndexNames(t, dir))

	// Idempotent on a second open.
	require.NoError(t, client.Close())
	client, err = OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()
	assert.Len(t, escapeIndexNames(t, dir), 3)
}
