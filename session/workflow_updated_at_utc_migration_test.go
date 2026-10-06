package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunWorkflowUpdatedAtUTCBackfill_should_NormalizePreExistingLocalRows_When_Called
// mirrors TestRunBacklogItemUpdatedAtUTCBackfill_should_NormalizePreExistingLocalRows_When_Called:
// a row written before the UTC schema fix (session/ent/schema/workflow.go) carries a
// Local-formatted updated_at. Simulates that pre-fix state by explicitly setting
// UpdatedAt to a Local time.Time (bypassing the schema's UTC default), then verifies
// the backfill both normalizes the Location to UTC and preserves the exact instant.
func TestRunWorkflowUpdatedAtUTCBackfill_should_NormalizePreExistingLocalRows_When_Called(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	localTime := time.Date(2026, 1, 15, 10, 30, 0, 123456789, time.FixedZone("PDT", -7*60*60))
	require.NotEqual(t, time.UTC, localTime.Location())

	created, err := repo.client.Workflow.Create().
		SetSlug("pre-fix-wf").
		SetName("Pre Fix WF").
		SetCommand("cmd").
		SetUpdatedAt(localTime).
		Save(ctx)
	require.NoError(t, err)
	require.NotEqual(t, time.UTC, created.UpdatedAt.Location(), "fixture setup must actually produce a non-UTC row")

	require.NoError(t, runWorkflowUpdatedAtUTCBackfill(ctx, repo))

	migrated, err := repo.client.Workflow.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, migrated.UpdatedAt.Location(), "backfill must normalize the row to UTC")
	assert.True(t, migrated.UpdatedAt.Equal(localTime), "backfill must preserve the exact same instant, not just reformat arbitrarily")
}

// TestRunWorkflowUpdatedAtUTCBackfill_should_BeIdempotent_When_RunTwice proves a
// second run is a safe no-op.
func TestRunWorkflowUpdatedAtUTCBackfill_should_BeIdempotent_When_RunTwice(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.client.Workflow.Create().
		SetSlug("fresh-wf").
		SetName("Fresh WF").
		SetCommand("cmd").
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, time.UTC, created.UpdatedAt.Location(), "a freshly created workflow must already be UTC under the fixed schema default")

	require.NoError(t, runWorkflowUpdatedAtUTCBackfill(ctx, repo))
	require.NoError(t, runWorkflowUpdatedAtUTCBackfill(ctx, repo))

	final, err := repo.client.Workflow.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, final.UpdatedAt.Equal(created.UpdatedAt), "idempotent backfill must not perturb an already-UTC row's timestamp")
}
