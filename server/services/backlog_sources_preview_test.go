package services

// backlog_sources_preview_test.go — tests for PreviewBackwardSyncImpact
// (Epic 4.4, Story 4.4.1). Reuses fakeSourcePlugin from
// backlog_service_test.go's TriggerSync test block.

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// TestPreviewBackwardSyncImpact_CountsOnlyEligibleItems verifies the preview
// only counts already-imported items in idea/refining/ready/queued whose
// linked (matching ExternalID) GitHub issue is closed — mirroring
// determineBackwardSyncTarget's exact eligibility rule, reused rather than
// re-derived. Items excluded for each of the distinct reasons a real source
// could have: an open issue, a non-eligible local status, and a closed issue
// with no locally-imported counterpart at all.
func TestPreviewBackwardSyncImpact_CountsOnlyEligibleItems(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakeSourcePlugin{items: []session.ExternalItem{
		{ExternalID: "e1", Title: "Eligible Ready", State: "closed"},
		{ExternalID: "e2", Title: "Eligible Idea", State: "closed"},
		{ExternalID: "e3", Title: "Open Issue", State: "open"},
		{ExternalID: "e4", Title: "In Progress", State: "closed"},
		{ExternalID: "e5", Title: "No Local Item", State: "closed"},
	}}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "Eligible Ready", Status: string(session.BacklogStatusReady), SourceID: src.ID, ExternalID: "e1",
	})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "Eligible Idea", Status: string(session.BacklogStatusIdea), SourceID: src.ID, ExternalID: "e2",
	})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "Open Issue", Status: string(session.BacklogStatusReady), SourceID: src.ID, ExternalID: "e3",
	})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "In Progress", Status: string(session.BacklogStatusInProgress), SourceID: src.ID, ExternalID: "e4",
	})
	require.NoError(t, err)
	// e5 deliberately has no matching locally-imported item.

	resp, previewErr := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{
		SourceId: src.ID,
	}))
	require.NoError(t, previewErr)
	assert.Equal(t, int32(2), resp.Msg.ItemCount)
	assert.ElementsMatch(t, []string{"Eligible Ready", "Eligible Idea"}, resp.Msg.SampleTitles)
}

// TestPreviewBackwardSyncImpact_ReturnsErrorOnFetchFailure verifies a Fetch
// failure (rate limit, auth, etc.) surfaces as a typed error rather than a
// false "itemCount: 0" — a silent "nothing will change" would be worse than
// an explicit failure here (plan.md Story 4.4.1 AC2).
func TestPreviewBackwardSyncImpact_ReturnsErrorOnFetchFailure(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakeSourcePlugin{fetchErr: errors.New("upstream boom")}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	_, previewErr := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{
		SourceId: src.ID,
	}))
	require.Error(t, previewErr)
	var connErr *connect.Error
	require.ErrorAs(t, previewErr, &connErr)
	assert.Equal(t, connect.CodeInternal, connErr.Code())
}

func TestPreviewBackwardSyncImpact_ReturnsUnimplementedWithoutPluginRegistry(t *testing.T) {
	svc := newBacklogService(t)
	_, err := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{SourceId: "any"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnimplemented, connErr.Code())
}

func TestPreviewBackwardSyncImpact_ReturnsInvalidArgumentWhenSourceIDEmpty(t *testing.T) {
	svc := newBacklogService(t)
	svc.SetPluginRegistry(session.NewPluginRegistry())
	_, err := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{SourceId: ""}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestPreviewBackwardSyncImpact_ReturnsNotFoundForMissingSource(t *testing.T) {
	svc := newBacklogService(t)
	registry := session.NewPluginRegistry()
	registry.Register(&fakeSourcePlugin{})
	svc.SetPluginRegistry(registry)

	_, err := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{
		SourceId: "00000000-0000-0000-0000-000000000000",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}
