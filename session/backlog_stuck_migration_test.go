package session

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
)

// TestBacklogStuckStateSchema_should_createTableAndUniqueIndex_When_FreshClient
// verifies that a fresh ent client's Schema.Create() creates the
// backlog_stuck_states table with a plain (item_id, reason) unique index —
// NOT a 3-column or partial index — by confirming a duplicate (item_id,
// reason) raw insert conflicts. This is exactly the target MarkStuck's
// OnConflictColumns(item_id, reason) upsert relies on.
func TestBacklogStuckStateSchema_should_createTableAndUniqueIndex_When_FreshClient(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "schema test item",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)
	itemUUID, err := uuid.Parse(item.ID)
	require.NoError(t, err)

	// First insert succeeds — the table exists and accepts writes.
	_, err = repo.client.BacklogStuckState.Create().
		SetItemID(itemUUID).
		SetReason("abandoned_review").
		Save(ctx)
	require.NoError(t, err)

	// A second raw (non-upsert) insert with the SAME (item_id, reason) pair
	// must violate the unique index — proving the index is exactly the
	// 2-column (item_id, reason) pair, not some other shape.
	_, err = repo.client.BacklogStuckState.Create().
		SetItemID(itemUUID).
		SetReason("abandoned_review").
		Save(ctx)
	assert.Error(t, err, "duplicate (item_id, reason) insert must violate the unique index")

	// A different reason for the same item is NOT a conflict — confirms the
	// index is the (item_id, reason) pair, not item_id alone.
	_, err = repo.client.BacklogStuckState.Create().
		SetItemID(itemUUID).
		SetReason("stale_work").
		Save(ctx)
	assert.NoError(t, err, "a different reason for the same item must not conflict")
}

// TestBacklogStuckState_should_cascadeDelete_When_ParentItemDeleted verifies
// that deleting a BacklogItem cascade-deletes its BacklogStuckState rows,
// mirroring the existing status_events cascade.
func TestBacklogStuckState_should_cascadeDelete_When_ParentItemDeleted(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "cascade delete test item",
		Status: string(BacklogStatusPRPending),
	})
	require.NoError(t, err)

	applied, err := repo.MarkStuck(ctx, item.ID, "pr_ready_unmerged", BacklogStatusPRPending, "context")
	require.NoError(t, err)
	require.True(t, applied)

	count, err := repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	require.NoError(t, repo.DeleteBacklogItem(ctx, item.ID))

	count, err = repo.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "stuck-state rows must be cascade-deleted with their parent item")
}

// Test_migration_should_be_reversible is the auto-migration analog of a clean
// up/down migration test for this repo's schema-create-based (no versioned
// migration files) approach: a fresh client's Schema.Create() is idempotent
// when run twice, and the additive backlog_stuck_states table coexists
// cleanly with its sibling tables (backlog_items, backlog_status_events).
func Test_migration_should_be_reversible(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-migration-reversible.db")

	db, err := sql.Open("sqlite", dbPath+"?_fk=1")
	require.NoError(t, err)
	defer db.Close()

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	ctx := context.Background()

	// First Schema.Create call: creates all tables including backlog_stuck_states.
	// Serialized via EntSchemaCreateMu — see its doc comment: Atlas has
	// package-level state that races under t.Parallel() across independent
	// clients.
	EntSchemaCreateMu.Lock()
	require.NoError(t, client.Schema.Create(ctx))

	// Second Schema.Create call must be idempotent — no error, no duplicate index.
	require.NoError(t, client.Schema.Create(ctx))
	EntSchemaCreateMu.Unlock()

	// Sibling tables remain intact and queryable — the additive table did not
	// disturb them.
	itemCount, err := client.BacklogItem.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, itemCount)

	eventCount, err := client.BacklogStatusEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, eventCount)

	stuckCount, err := client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stuckCount)
}
