package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

// hotQuery is a query a polled or per-request path runs against the core
// database. The SQL is a hand-written mirror of what ent emits, so it can drift
// from the real query; keep it in step when the repository query changes. The plan must SEARCH an index; a SCAN fails the test unless the
// query is listed as an index-ordered walk bounded by LIMIT or as a known gap.
type hotQuery struct {
	name string
	sql  string
	args []any
	// orderedWalk allows `SCAN ... USING INDEX` (an index-ordered walk) because LIMIT bounds it.
	orderedWalk bool
	// knownGap documents a query whose plan is not yet acceptable; it is logged, never failed,
	// so a PR that closes the gap does not break this test on merge.
	knownGap string
}

var coreHotQueries = []hotQuery{
	{name: "classification since", sql: "SELECT * FROM classification_analytics WHERE created_at >= ? ORDER BY created_at", args: []any{"2026-01-01"}},
	{name: "classification by program since", sql: "SELECT * FROM classification_analytics WHERE command_program = ? AND created_at >= ?", args: []any{"git", "2026-01-01"}},
	{name: "classification newest N", sql: "SELECT * FROM classification_analytics ORDER BY created_at DESC LIMIT 100", orderedWalk: true},
	{name: "context samples by session", sql: "SELECT * FROM context_samples WHERE session_uuid = ? ORDER BY turn_index", args: []any{"u"}},
	{name: "context samples since", sql: "SELECT * FROM context_samples WHERE sampled_at >= ?", args: []any{"2026-01-01"}},
	{name: "status events by item", sql: "SELECT * FROM backlog_status_events WHERE item_id = ? ORDER BY created_at", args: []any{1}},
	{name: "stuck states by item", sql: "SELECT * FROM backlog_stuck_states WHERE item_id = ?", args: []any{1}},
	{name: "diff stats by session", sql: "SELECT * FROM diff_stats WHERE session_diff_stats = ?", args: []any{1}},
	{name: "session by uuid", sql: "SELECT * FROM sessions WHERE uuid = ?", args: []any{"u"}},
	{name: "backlog by status", sql: "SELECT * FROM backlog_items WHERE status = ? ORDER BY priority", args: []any{"queued"}},
	{name: "item sessions by backlog item", sql: "SELECT * FROM item_sessions WHERE backlog_item_item_sessions = ?", args: []any{1}},
	{name: "status events newest N across items", sql: "SELECT * FROM backlog_status_events ORDER BY created_at DESC LIMIT 50",
		knownGap: "SCAN + TEMP B-TREE; audit callers before adding a created_at index (ADR-036 index audit)"},
}

func queryPlan(t *testing.T, repo *EntRepository, q string, args []any) []string {
	t.Helper()
	var rows entsql.Rows
	require.NoError(t, repo.driver.Query(context.Background(), "EXPLAIN QUERY PLAN "+q, args, &rows))
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	return plan
}

func TestCoreHotQueries_should_SearchAnIndex_When_RunOnAMigratedDatabase(t *testing.T) {
	t.Parallel()
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "sessions.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.client.Close() })

	for _, hq := range coreHotQueries {
		plan := strings.Join(queryPlan(t, repo, hq.sql, hq.args), " | ")
		scans := strings.Contains(plan, "SCAN ") && (!hq.orderedWalk || !strings.Contains(plan, "USING INDEX"))
		tempSort := strings.Contains(plan, "TEMP B-TREE FOR ORDER BY")
		switch {
		case hq.knownGap != "":
			if strings.Contains(plan, "SEARCH") && !strings.Contains(plan, "SCAN ") {
				t.Logf("known gap %q looks closed (%s): remove knownGap. Plan: %s", hq.name, hq.knownGap, plan)
				break
			}
			t.Logf("known gap %q (%s): %s", hq.name, hq.knownGap, plan)
		case scans || tempSort:
			t.Errorf("hot query %q has a table scan or sort: %s", hq.name, plan)
		case hq.orderedWalk:
			require.Contains(t, plan, "USING INDEX", hq.name)
		default:
			require.Contains(t, plan, "SEARCH", hq.name)
		}
	}
}
