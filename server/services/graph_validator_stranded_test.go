package services

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func seededTransition(t *testing.T, svc *BacklogService, from, to session.BacklogStatus) *sessionv1.StageTransition {
	t.Helper()
	f := string(from)
	resp, err := svc.ListStageTransitions(t.Context(), connect.NewRequest(&sessionv1.ListStageTransitionsRequest{FromStageSlug: &f}))
	require.NoError(t, err)
	for _, tr := range resp.Msg.Items {
		if tr.ToStageSlug == string(to) {
			return tr
		}
	}
	t.Fatalf("seeded edge %s->%s not found", from, to)
	return nil
}

func TestUpdateStageTransition_should_RejectDisable_When_EdgeIsSystemRequired(t *testing.T) {
	t.Parallel()
	edges := [][2]session.BacklogStatus{
		{session.BacklogStatusInProgress, session.BacklogStatusReview},
		{session.BacklogStatusReady, session.BacklogStatusQueued},
		{session.BacklogStatusQueued, session.BacklogStatusInProgress},
		{session.BacklogStatusRefining, session.BacklogStatusArchived},
	}
	for _, e := range edges {
		t.Run(string(e[0])+"->"+string(e[1]), func(t *testing.T) {
			t.Parallel()
			svc, _, _, _ := newTransitionCRUDTestService(t)
			tr := seededTransition(t, svc, e[0], e[1])

			_, err := svc.UpdateStageTransition(t.Context(), connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{
				Id: tr.Id, Enabled: boolPtr(false),
			}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
			assert.Contains(t, err.Error(), "system-required")
			assert.Contains(t, err.Error(), string(e[0])+"->"+string(e[1]))

			got, err := svc.GetStageTransition(t.Context(), connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: tr.Id}))
			require.NoError(t, err)
			assert.True(t, got.Msg.Item.Enabled, "stored edge must stay enabled")
		})
	}
}

func TestDeleteStageTransition_should_RejectDelete_When_EdgeIsSystemRequired(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	tr := seededTransition(t, svc, session.BacklogStatusReady, session.BacklogStatusQueued)

	_, err := svc.DeleteStageTransition(t.Context(), connect.NewRequest(&sessionv1.DeleteStageTransitionRequest{Id: tr.Id}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	_, err = svc.GetStageTransition(t.Context(), connect.NewRequest(&sessionv1.GetStageTransitionRequest{Id: tr.Id}))
	require.NoError(t, err, "edge must still exist")
}

func TestUpdateStageTransition_should_AllowDisable_When_EdgeIsNotSystemRequired(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTransitionCRUDTestService(t)
	tr := seededTransition(t, svc, session.BacklogStatusDone, session.BacklogStatusRefining)

	resp, err := svc.UpdateStageTransition(t.Context(), connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{
		Id: tr.Id, Enabled: boolPtr(false),
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Item.Enabled)
}

func TestUpdateStageTransition_should_NameStatusAndItemIDs_When_DisableWouldStrandLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, storage := newTransitionCRUDTestService(t)
	ctx := t.Context()

	// A custom stage has no system-required outgoing edge, so its configured
	// edges are its only way out.
	const slug = "design-review-strand"
	_, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{Slug: slug, Name: slug, IsEntry: true, Enabled: true}))
	require.NoError(t, err)
	targets := []session.BacklogStatus{session.BacklogStatusReady, session.BacklogStatusRefining}
	edges := make([]*sessionv1.StageTransition, 0, len(targets))
	for _, to := range targets {
		resp, err := svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
			FromStageSlug: slug, ToStageSlug: string(to), Enabled: true,
		}))
		require.NoError(t, err)
		edges = append(edges, resp.Msg.Item)
	}
	created, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "stuck", Status: slug})
	require.NoError(t, err)

	_, err = svc.UpdateStageTransition(ctx, connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{Id: edges[0].Id, Enabled: boolPtr(false)}))
	require.NoError(t, err, "removing one of several outgoing edges is allowed")

	_, err = svc.UpdateStageTransition(ctx, connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{Id: edges[1].Id, Enabled: boolPtr(false)}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), slug)
	assert.Contains(t, err.Error(), created.ID)
	assert.Contains(t, err.Error(), "override_reason")
}

// strandableStages creates custom stage "from" whose only edge goes to custom
// stage "to", with one live item on "from".
func strandableStages(t *testing.T, svc *BacklogService, storage *session.Storage, from, to string) (fromID, toID, itemID string) {
	t.Helper()
	ctx := t.Context()
	fr, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{Slug: from, Name: from, IsEntry: true, Enabled: true}))
	require.NoError(t, err)
	tr, err := svc.CreateStage(ctx, connect.NewRequest(&sessionv1.CreateStageRequest{Slug: to, Name: to, IsTerminal: true, Enabled: true}))
	require.NoError(t, err)
	_, err = svc.CreateStageTransition(ctx, connect.NewRequest(&sessionv1.CreateStageTransitionRequest{FromStageSlug: from, ToStageSlug: to, Enabled: true}))
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "stuck", Status: from})
	require.NoError(t, err)
	return fr.Msg.Item.Id, tr.Msg.Item.Id, item.ID
}

func TestUpdateStage_should_RejectDisable_When_ItWouldStrandLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, storage := newTransitionCRUDTestService(t)
	fromID, toID, itemID := strandableStages(t, svc, storage, "strand-a", "strand-b")

	for name, id := range map[string]string{"own stage": fromID, "target stage": toID} {
		_, err := svc.UpdateStage(t.Context(), connect.NewRequest(&sessionv1.UpdateStageRequest{Id: id, Enabled: boolPtr(false)}))
		require.Error(t, err, name)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), name)
		assert.Contains(t, err.Error(), "strand-a", name)
		assert.Contains(t, err.Error(), itemID, name)
	}
	got, err := svc.GetStage(t.Context(), connect.NewRequest(&sessionv1.GetStageRequest{Slug: "strand-b"}))
	require.NoError(t, err)
	assert.True(t, got.Msg.Item.Enabled, "no partial write")
}

func TestDeleteStage_should_RejectForce_When_ItWouldStrandLiveItems(t *testing.T) {
	t.Parallel()
	svc, _, _, storage := newTransitionCRUDTestService(t)
	_, toID, itemID := strandableStages(t, svc, storage, "strand-c", "strand-d")

	_, err := svc.DeleteStage(t.Context(), connect.NewRequest(&sessionv1.DeleteStageRequest{Id: toID, Force: true}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), itemID)

	_, err = svc.GetStage(t.Context(), connect.NewRequest(&sessionv1.GetStageRequest{Slug: "strand-d"}))
	require.NoError(t, err, "stage must still exist")
}

func TestDeleteStage_should_NotSuggestForce_When_StageHostsSystemRequiredEdges(t *testing.T) {
	t.Parallel()
	svc, _, _, storage := newTransitionCRUDTestService(t)
	stage, err := svc.GetStage(t.Context(), connect.NewRequest(&sessionv1.GetStageRequest{Slug: string(session.BacklogStatusReview)}))
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "in review", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.DeleteStage(t.Context(), connect.NewRequest(&sessionv1.DeleteStageRequest{Id: stage.Msg.Item.Id}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "force")
	assert.Contains(t, err.Error(), "system-required")
}

