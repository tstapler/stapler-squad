package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunBacklogItemUpdatedAtUTCBackfill_should_NormalizePreExistingLocalRows_When_Called
// is the regression test for the CRITICAL rollout gap found in code review:
// rows written before the UTC schema fix (session/ent/schema/backlog_item.go)
// still carry a Local-formatted updated_at TEXT value on disk. Simulates a
// pre-fix row by explicitly setting UpdatedAt to a Local time.Time (bypassing
// the schema's UTC default via an explicit Set call, exactly as the OLD bare
// time.Now default would have produced), then verifies the backfill both
// normalizes the Location to UTC AND preserves the exact same instant.
func TestRunBacklogItemUpdatedAtUTCBackfill_should_NormalizePreExistingLocalRows_When_Called(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	// A Local time.Time — e.g. time.Now() in a non-UTC server environment,
	// exactly what the old bare `time.Now` schema default produced.
	localTime := time.Date(2026, 1, 15, 10, 30, 0, 123456789, time.FixedZone("PDT", -7*60*60))
	require.NotEqual(t, time.UTC, localTime.Location())

	created, err := repo.client.BacklogItem.Create().
		SetTitle("pre-fix item with Local updated_at").
		SetStatus("review").
		SetUpdatedAt(localTime).
		Save(ctx)
	require.NoError(t, err)
	require.NotEqual(t, time.UTC, created.UpdatedAt.Location(), "fixture setup must actually produce a non-UTC row")

	require.NoError(t, runBacklogItemUpdatedAtUTCBackfill(ctx, repo))

	migrated, err := repo.client.BacklogItem.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, migrated.UpdatedAt.Location(), "backfill must normalize the row to UTC")
	assert.True(t, migrated.UpdatedAt.Equal(localTime), "backfill must preserve the exact same instant, not just reformat arbitrarily")
}

// TestRunBacklogItemUpdatedAtUTCBackfill_should_BeIdempotent_When_RunTwice proves
// a second run is a safe no-op — already-UTC rows (including ones the first
// backfill run just normalized, and freshly-created rows under the fixed
// schema default) are left untouched.
func TestRunBacklogItemUpdatedAtUTCBackfill_should_BeIdempotent_When_RunTwice(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "freshly created item"})
	require.NoError(t, err)
	require.Equal(t, time.UTC, created.UpdatedAt.Location(), "a freshly created item must already be UTC under the fixed schema default")

	require.NoError(t, runBacklogItemUpdatedAtUTCBackfill(ctx, repo))
	require.NoError(t, runBacklogItemUpdatedAtUTCBackfill(ctx, repo))

	final, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, final.UpdatedAt.Equal(created.UpdatedAt), "idempotent backfill must not perturb an already-UTC row's timestamp")
}
