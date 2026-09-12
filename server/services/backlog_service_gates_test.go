package services

// backlog_service_gates_test.go — tests for GetPendingGates, the read-only
// preview RPC backing the item-detail "what's blocking this" checklist
// (Epic 2.10). Added alongside the checklist UI once it surfaced that no RPC
// exposed WorkflowEngine.PendingGates to the frontend at all.

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func TestGetPendingGates_should_ReturnNotFound_When_ItemDoesNotExist(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newStageCRUDTestService(t)
	ctx := t.Context()

	_, err := svc.GetPendingGates(ctx, connect.NewRequest(&sessionv1.GetPendingGatesRequest{
		ItemId:   "00000000-0000-0000-0000-000000000000",
		ToStatus: string(session.BacklogStatusRefining),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetPendingGates_should_ReturnEmpty_When_TransitionHasNoConfiguredGates(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()
	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, storage.GetEntClient()))

	repo := session.NewEntStageConfigRepository(storage.GetEntClient())
	gateSatisfactionRepo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	engine, err := session.NewConfiguredWorkflowEngine(repo, gateSatisfactionRepo, nil)
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, engine, nil, nil)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "an item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	resp, err := svc.GetPendingGates(ctx, connect.NewRequest(&sessionv1.GetPendingGatesRequest{
		ItemId:   item.ID,
		ToStatus: string(session.BacklogStatusRefining),
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Gates, "the built-in idea->refining edge has no configured gates")
}
