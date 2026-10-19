package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// denyAllEngine answers every transition question with "no"; it proves the
// SyncLoop consults the engine it was given rather than a private default.
type denyAllEngine struct{ DefaultWorkflowEngine }

func (denyAllEngine) CanTransition(BacklogStatus, BacklogStatus, *StageConfigSnapshot) bool {
	return false
}

func TestSyncLoop_SetWorkflowEngine_UsesInjectedEngineForBackwardSync(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id:    "fake",
		items: []ExternalItem{{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: time.Now()}},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	denier := &denyAllEngine{}
	sl.SetWorkflowEngine(nil) // nil must not clear the engine
	require.NotNil(t, sl.WorkflowEngine())
	sl.SetWorkflowEngine(denier)
	require.Same(t, WorkflowEngine(denier), sl.WorkflowEngine())

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title: "Issue", Status: string(BacklogStatusReady), ExternalID: "ext-1", SourceID: sourceID,
	})
	require.NoError(t, err)
	entSrc, err := storage.repo.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	got, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReady), got.Status, "the denying engine must keep the closed-issue archive from happening")
}
