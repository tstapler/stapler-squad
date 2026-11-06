package session

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
)

// episodeExec runs DDL/DML straight on the repo's writer connection.
func episodeExec(t *testing.T, repo *EntRepository, query string) {
	t.Helper()
	require.NoError(t, repo.driver.Exec(context.Background(), query, []any{}, nil), query)
}

// sqliteMasterRows returns (type, name, sql) rows from sqlite_master whose
// name or tbl_name matches table, via the repo's own connection.
func sqliteMasterRows(t *testing.T, repo *EntRepository, table string) [][3]string {
	t.Helper()
	var rows entsql.Rows
	require.NoError(t, repo.driver.Query(context.Background(),
		"SELECT type, name, COALESCE(sql, '') FROM sqlite_master WHERE tbl_name = ? ORDER BY type, name",
		[]any{table}, &rows))
	defer rows.Close()
	var out [][3]string
	for rows.Next() {
		var r [3]string
		require.NoError(t, rows.Scan(&r[0], &r[1], &r[2]))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func openEpisodeIndexSQL(t *testing.T, repo *EntRepository) string {
	t.Helper()
	for _, r := range sqliteMasterRows(t, repo, "backlog_stuck_episodes") {
		if r[0] == "index" && strings.Contains(r[2], "UNIQUE") {
			return r[2]
		}
	}
	return ""
}

func insertOpenEpisode(ctx context.Context, repo *EntRepository, itemID uuid.UUID, reason string) error {
	return repo.client.BacklogStuckEpisode.Create().SetItemID(itemID).SetReason(reason).Exec(ctx)
}

func insertResolvedEpisode(ctx context.Context, repo *EntRepository, itemID uuid.UUID, reason string) error {
	return repo.client.BacklogStuckEpisode.Create().SetItemID(itemID).SetReason(reason).
		SetResolvedAt(time.Now().UTC()).Exec(ctx)
}

func TestStuckEpisodeSchema_should_RejectSecondOpenRowAcceptResolvedPlusOpen_When_InsertedDirectly(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	item := uuid.New()

	require.NoError(t, insertOpenEpisode(ctx, repo, item, "pr_needs_fix"))

	err := insertOpenEpisode(ctx, repo, item, "pr_needs_fix")
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "second open row must hit the partial unique index: %v", err)

	// Different reason, and resolved rows, are not blocked.
	require.NoError(t, insertOpenEpisode(ctx, repo, item, "other_reason"))
	require.NoError(t, insertResolvedEpisode(ctx, repo, item, "pr_needs_fix"))
	require.NoError(t, insertResolvedEpisode(ctx, repo, item, "pr_needs_fix"))

	// Close the open one; a new open row is then accepted.
	_, err = repo.client.BacklogStuckEpisode.Update().SetResolvedAt(time.Now().UTC()).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, insertOpenEpisode(ctx, repo, item, "pr_needs_fix"))

	assert.Contains(t, openEpisodeIndexSQL(t, repo), "WHERE resolved_at IS NULL")
}

func TestMigration_should_BeReversible_When_StuckEpisodeTableAndIndexCreatedAndDropped(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	rows := sqliteMasterRows(t, repo, "backlog_stuck_episodes")
	require.NotEmpty(t, rows, "Schema.Create must create backlog_stuck_episodes")
	assert.Contains(t, openEpisodeIndexSQL(t, repo), "WHERE resolved_at IS NULL")

	episodeExec(t, repo, "DROP TABLE backlog_stuck_episodes")
	assert.Empty(t, sqliteMasterRows(t, repo, "backlog_stuck_episodes"), "table and its indexes are gone")

	// The rest of the schema still works with the episode table absent.
	_, err := repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)

	// Re-running Schema.Create recreates it (additive migration).
	EntSchemaCreateMu.Lock()
	err = repo.client.Schema.Create(ctx)
	EntSchemaCreateMu.Unlock()
	require.NoError(t, err)
	assert.Contains(t, openEpisodeIndexSQL(t, repo), "WHERE resolved_at IS NULL")
}

// Runs only against a `sqlite3 .backup` copy of a live-shaped DB named by
// SSQ_FLOW_PERF_DB; never point it at a live file.
func TestStuckEpisodeSchema_should_CreatePartialIndexOnLiveCopy_When_SchemaCreateRunsOnBackup(t *testing.T) {
	path := os.Getenv("SSQ_FLOW_PERF_DB")
	if path == "" {
		t.Skip("SSQ_FLOW_PERF_DB not set")
	}
	countStuck := func(db *sql.DB) int {
		var n int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM backlog_stuck_states").Scan(&n))
		return n
	}
	pre, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	before := countStuck(pre)
	require.NoError(t, pre.Close())

	repo, err := NewEntRepository(WithDatabasePath(path))
	require.NoError(t, err)
	defer repo.Close()

	idx := openEpisodeIndexSQL(t, repo)
	assert.Contains(t, idx, "WHERE resolved_at IS NULL")
	after, err := repo.client.BacklogStuckState.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after, "backlog_stuck_states rows must be untouched")
	t.Logf("sqlite_master partial index: %s", idx)
	t.Logf("backlog_stuck_states rows before=%d after=%d", before, after)
}
