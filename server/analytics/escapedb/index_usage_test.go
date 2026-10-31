package escapedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func queryPlan(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN "+q, args...)
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	return strings.Join(plan, "; ")
}

// Every escape_events index is the one a real query plans onto, on ~50k synthetic
// rows without sqlite_stat1 (the production state: PRAGMA optimize never runs), so none is droppable: removing one turns its query into a table scan.
func TestEscapeEventsIndexes_should_EachServeARealQuery(t *testing.T) {
	ctx := context.Background()
	_, dbh, err := Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbh.Close() })
	db := dbh.Writer()

	tx, err := db.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare("INSERT INTO escape_events (id, session_id, stage, sequence_type, byte_length, mangled, wall_time, session_seq) VALUES (?,?,?,?,?,?,?,?)")
	require.NoError(t, err)
	base := time.Now().Add(-48 * time.Hour).Round(0)
	for i := 0; i < 50_000; i++ {
		_, err := stmt.Exec(fmt.Sprintf("%016x", i*7919), fmt.Sprintf("s%d", i%40), "pty_read", "SGR", 4, false, base.Add(time.Duration(i)*time.Second), i)
		require.NoError(t, err)
	}
	require.NoError(t, stmt.Close())
	require.NoError(t, tx.Commit())

	cases := []struct {
		name, index, query string
		args               []any
	}{
		{"timeline paging", "escapeevent_session_id_session_seq", "SELECT id FROM escape_events WHERE session_id = ? AND session_seq > ? ORDER BY session_seq LIMIT 100", []any{"s1", 10}},
		{"per-session oldest-N eviction", "escapeevent_session_id_wall_time", "SELECT id FROM escape_events WHERE session_id = ? ORDER BY wall_time LIMIT 250", []any{"s1"}},
		{"age retention delete", "escapeevent_wall_time", "SELECT id FROM escape_events WHERE wall_time < ?", []any{base.Add(time.Hour)}},
		{"global time-range summary", "escapeevent_wall_time", "SELECT session_id, project_path, sequence_type, stage, mangled, mangle_type, COUNT(*) FROM escape_events WHERE wall_time >= ? AND wall_time <= ? GROUP BY session_id, project_path, sequence_type, stage, mangled, mangle_type", []any{base, base.Add(time.Hour)}},
	}
	for _, c := range cases {
		plan := queryPlan(t, db, c.query, c.args...)
		assert.Contains(t, plan, c.index, "%s: %s", c.name, plan)
		assert.NotContains(t, plan, "SCAN escape_events", "%s: %s", c.name, plan)
	}
}
