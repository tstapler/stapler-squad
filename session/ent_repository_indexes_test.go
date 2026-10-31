package session

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func indexNames(t *testing.T, dbPath, table string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND tbl_name=? AND sql IS NOT NULL", table)
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

func execOnDisk(t *testing.T, dbPath string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath)
	require.NoError(t, err)
	defer db.Close()
	for _, s := range stmts {
		_, err := db.Exec(s)
		require.NoError(t, err, s)
	}
}

// A database created before the index cleanup carries four unused
// classification_analytics indexes and lacks the item_sessions FK index;
// reopening must fix both and leave the two live analytics indexes in place.
func TestNewEntRepository_should_MigrateIndexes_When_OpeningLegacyDatabase(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	execOnDisk(t, dbPath,
		"CREATE INDEX classificationanalytics_session_id ON classification_analytics (session_id)",
		"CREATE INDEX classificationanalytics_decision ON classification_analytics (decision)",
		"CREATE INDEX classificationanalytics_risk_level ON classification_analytics (risk_level)",
		"CREATE INDEX classificationanalytics_rule_id ON classification_analytics (rule_id)",
		"DROP INDEX itemsession_backlog_item_item_sessions",
	)
	require.Contains(t, indexNames(t, dbPath, "classification_analytics"), "classificationanalytics_rule_id")
	require.NotContains(t, indexNames(t, dbPath, "item_sessions"), "itemsession_backlog_item_item_sessions")

	repo, err = NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	assert.ElementsMatch(t, []string{
		"classification_analytics_analytics_id_key",
		"classificationanalytics_created_at",
		"classificationanalytics_command_program_created_at",
	}, indexNames(t, dbPath, "classification_analytics"))
	assert.Contains(t, indexNames(t, dbPath, "item_sessions"), "itemsession_backlog_item_item_sessions")

	// Idempotent on a third open.
	repo, err = NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	require.NoError(t, repo.Close())
}

// ListItemSessions' lookup must seek on the FK index instead of scanning item_sessions.
func TestItemSessions_should_SeekByBacklogItem_When_QueryPlanned(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.Query("EXPLAIN QUERY PLAN SELECT id FROM item_sessions WHERE backlog_item_item_sessions = ? ORDER BY created_at", "x")
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	joined := strings.Join(plan, "; ")
	assert.Contains(t, joined, "USING INDEX itemsession_backlog_item_item_sessions", "plan: %s", joined)
	assert.NotContains(t, joined, "SCAN item_sessions", "plan: %s", joined)
}
