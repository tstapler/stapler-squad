package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

func TestSeedFingerprint_MatchesFreshlySeededDatabase_WhenInMemoryEntSeeded(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	require.NoError(t, workflowengine.EnsureBuiltInWorkflowStages(ctx, repo.Client))

	live, err := workflowengine.StageConfigFingerprint(ctx, repo.Client)
	require.NoError(t, err)
	require.Equal(t, workflowengine.SeedFingerprint(), live)

	// An edit that touches no edge still reads as edited.
	//nolint:entfullscan test-scoped in-memory DB.
	stage, err := repo.Client.BacklogStage.Query().First(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.Client.BacklogStage.UpdateOneID(stage.ID).SetEnabled(false).Exec(ctx))
	edited, err := workflowengine.StageConfigFingerprint(ctx, repo.Client)
	require.NoError(t, err)
	require.NotEqual(t, workflowengine.SeedFingerprint(), edited)
}
