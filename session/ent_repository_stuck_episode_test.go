package session

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckepisode"
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
	return repo.Client.BacklogStuckEpisode.Create().SetItemID(itemID).SetReason(reason).Exec(ctx)
}

func insertResolvedEpisode(ctx context.Context, repo *EntRepository, itemID uuid.UUID, reason string) error {
	return repo.Client.BacklogStuckEpisode.Create().SetItemID(itemID).SetReason(reason).
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
	_, err = repo.Client.BacklogStuckEpisode.Update().SetResolvedAt(time.Now().UTC()).Save(ctx)
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
	_, err := repo.Client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)

	// Re-running Schema.Create recreates it (additive migration).
	EntSchemaCreateMu.Lock()
	err = repo.Client.Schema.Create(ctx)
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
	after, err := repo.Client.BacklogStuckState.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after, "backlog_stuck_states rows must be untouched")
	t.Logf("sqlite_master partial index: %s", idx)
	t.Logf("backlog_stuck_states rows before=%d after=%d", before, after)
}

// --- Story 5.1.2: episode writer ---

const episodeReason = domain.StuckReasonPRReadyUnmerged

func episodesFor(t *testing.T, repo *EntRepository, itemID string) []*ent.BacklogStuckEpisode {
	t.Helper()
	id, err := uuid.Parse(itemID)
	require.NoError(t, err)
	eps, err := repo.Client.BacklogStuckEpisode.Query().
		Where(backlogstuckepisode.ItemID(id)).
		Order(ent.Asc(backlogstuckepisode.FieldOpenedAt)).
		All(context.Background())
	require.NoError(t, err)
	return eps
}

func markEpisodeItem(t *testing.T, repo *EntRepository, itemID string) {
	t.Helper()
	applied, err := repo.MarkStuck(context.Background(), itemID, episodeReason, BacklogStatusPRPending, "ctx")
	require.NoError(t, err)
	require.True(t, applied)
}

func TestMarkStuck_should_OpenOneEpisodeAndSecondOnRecurrence_When_ResolvedThenMarkedAgain(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

	markEpisodeItem(t, repo, itemID)
	markEpisodeItem(t, repo, itemID) // idempotent refresh: no second episode
	eps := episodesFor(t, repo, itemID)
	require.Len(t, eps, 1)
	assert.Nil(t, eps[0].ResolvedAt)
	state := repo.Client.BacklogStuckState.Query().OnlyX(ctx)
	assert.True(t, eps[0].OpenedAt.Equal(state.FirstDetectedAt), "opened_at %v must equal first_detected_at %v", eps[0].OpenedAt, state.FirstDetectedAt)
	assert.Equal(t, time.UTC, eps[0].OpenedAt.Location())

	resolved, err := repo.ResolveStuck(ctx, itemID, episodeReason)
	require.NoError(t, err)
	require.True(t, resolved)

	markEpisodeItem(t, repo, itemID)
	eps = episodesFor(t, repo, itemID)
	require.Len(t, eps, 2)
	assert.NotNil(t, eps[0].ResolvedAt, "first episode closed")
	assert.Nil(t, eps[1].ResolvedAt, "second episode open")
	state = repo.Client.BacklogStuckState.Query().OnlyX(ctx)
	assert.True(t, eps[1].OpenedAt.Equal(state.FirstDetectedAt))
}

func TestMarkStuck_should_CreateExactlyOneOpenEpisode_When_8GoroutinesMarkConcurrently(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.MarkStuck(ctx, itemID, episodeReason, BacklogStatusPRPending, "ctx")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Len(t, episodesFor(t, repo, itemID), 1)
}

func TestMarkStuckResolveStuck_should_NotHang_When_WriterPoolOneConnection2sTimeout(t *testing.T) {
	t.Parallel()
	// On-disk repositories use the dbpool writer, capped at one connection.
	repo, err := NewEntRepository(WithDatabasePath(filepath.Join(t.TempDir(), "sessions.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	itemID := createStuckTestItem(t, repo, context.Background(), BacklogStatusPRPending)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		if _, err := repo.MarkStuck(ctx, itemID, episodeReason, BacklogStatusPRPending, "ctx"); err != nil {
			done <- fmt.Errorf("mark: %w", err)
			return
		}
		if _, err := repo.ResolveStuck(ctx, itemID, episodeReason); err != nil {
			done <- fmt.Errorf("resolve: %w", err)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("MarkStuck/ResolveStuck hung: a call on r.client inside the tx would deadlock the 1-connection writer")
	}
	eps := episodesFor(t, repo, itemID)
	require.Len(t, eps, 1)
	assert.NotNil(t, eps[0].ResolvedAt)
}

func TestResolveStuck_should_CloseOnlyOpenEpisodeAtomically_When_StateUpdateAffectsRows(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)

	markEpisodeItem(t, repo, itemID)
	_, err := repo.ResolveStuck(ctx, itemID, episodeReason)
	require.NoError(t, err)
	markEpisodeItem(t, repo, itemID)
	firstClosedAt := *episodesFor(t, repo, itemID)[0].ResolvedAt

	resolved, err := repo.ResolveStuck(ctx, itemID, episodeReason)
	require.NoError(t, err)
	assert.True(t, resolved)

	eps := episodesFor(t, repo, itemID)
	require.Len(t, eps, 2)
	assert.True(t, firstClosedAt.Equal(*eps[0].ResolvedAt), "older closed episode untouched")
	require.NotNil(t, eps[1].ResolvedAt)

	// Zero-row state UPDATE (already resolved) touches no episode.
	orphanItem := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	orphan := uuid.MustParse(orphanItem)
	require.NoError(t, insertOpenEpisode(ctx, repo, orphan, string(episodeReason)))
	resolved, err = repo.ResolveStuck(ctx, orphanItem, episodeReason)
	require.NoError(t, err)
	assert.False(t, resolved, "no state row, so zero rows affected")
	resolved, err = repo.ResolveStuck(ctx, itemID, episodeReason)
	require.NoError(t, err)
	assert.False(t, resolved)
	assert.Len(t, episodesFor(t, repo, orphanItem), 1)
	assert.Nil(t, episodesFor(t, repo, orphanItem)[0].ResolvedAt)
	assert.True(t, firstClosedAt.Equal(*episodesFor(t, repo, itemID)[0].ResolvedAt))
}

// Not parallel: swaps the process-wide warning logger's output.
func TestMarkStuck_should_CommitStateAndLogWarning_When_EpisodeTableMissing(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	episodeExec(t, repo, "DROP TABLE backlog_stuck_episodes")

	var buf bytes.Buffer
	l := log.WarningLog()
	prev := l.Writer()
	l.SetOutput(&buf)
	defer l.SetOutput(prev)

	applied, err := repo.MarkStuck(ctx, itemID, episodeReason, BacklogStatusPRPending, "ctx")
	require.NoError(t, err)
	assert.True(t, applied)
	state, err := repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, state.ResolvedAt)
	assert.Contains(t, buf.String(), "stuck episode")

	buf.Reset()
	resolved, err := repo.ResolveStuck(ctx, itemID, episodeReason)
	require.NoError(t, err)
	assert.True(t, resolved)
	assert.Contains(t, buf.String(), "stuck episode")
}

func TestMarkStuck_should_TreatErrNoRowsAndConstraintErrorAsAlreadyOpen_When_EpisodeExists(t *testing.T) {
	t.Parallel()
	assert.True(t, isEpisodeAlreadyOpen(sql.ErrNoRows))
	assert.True(t, isEpisodeAlreadyOpen(fmt.Errorf("wrapped: %w", sql.ErrNoRows)))
	assert.True(t, isEpisodeAlreadyOpen(&ent.ConstraintError{}))
	assert.False(t, isEpisodeAlreadyOpen(errors.New("no such table: backlog_stuck_episodes")))

	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	item := uuid.New()

	open := func() (bool, error) {
		tx, err := repo.Client.Tx(ctx)
		require.NoError(t, err)
		defer tx.Rollback() //nolint:errcheck
		opened, err := openStuckEpisodeTx(ctx, tx, item, string(episodeReason), time.Now())
		if err == nil {
			require.NoError(t, tx.Commit())
		}
		return opened, err
	}
	opened, err := open()
	require.NoError(t, err)
	assert.True(t, opened)
	opened, err = open()
	require.NoError(t, err, "already-open is not an error")
	assert.False(t, opened)

	episodeExec(t, repo, "DROP TABLE backlog_stuck_episodes")
	_, err = open()
	require.Error(t, err, "other errors surface to the non-fatal logging caller")
}

func TestResolveStuck_should_OpenNoTransaction_When_NothingToResolve(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusPRPending)
	drv := &flowMarkerCountingDriver{Driver: repo.driver}
	repo.Client = ent.NewClient(ent.Driver(drv))

	resolved, err := repo.ResolveStuck(ctx, itemID, episodeReason)
	require.NoError(t, err)
	assert.False(t, resolved)
	txs, queries := drv.snapshot()
	assert.Zero(t, txs, "no-op resolve must not BEGIN")
	assert.Len(t, queries, 1, "single cheap existence check")
}
