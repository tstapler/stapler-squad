package services

// backlog_sources_preview_test.go — tests for PreviewBackwardSyncImpact
// (Epic 4.4, Story 4.4.1). Reuses fakeSourcePlugin from
// backlog_service_test.go's TriggerSync test block.

import (
	"context"
	"errors"
	"fmt"
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

// fakePaginatedSourcePlugin is a session.ItemSourcePlugin stub that also
// implements session.PaginatedFetcher, so tests can exercise
// PreviewBackwardSyncImpact's FetchAll path (used when a plugin supports
// pagination) rather than the single-page Fetch fallback fakeSourcePlugin
// exercises.
type fakePaginatedSourcePlugin struct {
	items              []session.ExternalItem
	possiblyIncomplete bool
}

func (f *fakePaginatedSourcePlugin) PluginID() string { return "fake_paginated_source" }

func (f *fakePaginatedSourcePlugin) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	// Deliberately return nothing on the plain single-page Fetch path, so a
	// test asserting on FetchAll's results would fail loudly if
	// PreviewBackwardSyncImpact ever regressed to calling Fetch instead of
	// FetchAll for a plugin that implements PaginatedFetcher.
	return nil, cursor, nil
}

func (f *fakePaginatedSourcePlugin) FetchAll(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, bool, error) {
	return f.items, cursor, f.possiblyIncomplete, nil
}

func (f *fakePaginatedSourcePlugin) MapToBacklogItem(item session.ExternalItem, sourceID string) session.BacklogItemData {
	return session.BacklogItemData{
		Title:      item.Title,
		Status:     string(session.BacklogStatusIdea),
		ExternalID: item.ExternalID,
		SourceID:   sourceID,
	}
}

// TestPreviewBackwardSyncImpact_UsesFetchAllAndSurfacesPossiblyIncomplete
// verifies two things that guard the CRITICAL under-reporting finding: (1)
// PreviewBackwardSyncImpact prefers a plugin's FetchAll (PaginatedFetcher)
// over its single-page Fetch when both are available, and (2) a true
// possiblyIncomplete from the plugin propagates all the way out to the RPC
// response, so the UI can show the "may be incomplete" caveat rather than
// implying an exhaustive count.
func TestPreviewBackwardSyncImpact_UsesFetchAllAndSurfacesPossiblyIncomplete(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakePaginatedSourcePlugin{
		items: []session.ExternalItem{
			{ExternalID: "e1", Title: "Eligible One", State: "closed"},
			{ExternalID: "e2", Title: "Eligible Two", State: "closed"},
		},
		possiblyIncomplete: true,
	}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Paginated Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "Eligible One", Status: string(session.BacklogStatusReady), SourceID: src.ID, ExternalID: "e1",
	})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "Eligible Two", Status: string(session.BacklogStatusIdea), SourceID: src.ID, ExternalID: "e2",
	})
	require.NoError(t, err)

	resp, previewErr := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{
		SourceId: src.ID,
	}))
	require.NoError(t, previewErr)
	assert.Equal(t, int32(2), resp.Msg.ItemCount, "should have counted items from FetchAll, not the empty Fetch fallback")
	assert.True(t, resp.Msg.PossiblyIncomplete)
}

// TestPreviewBackwardSyncImpact_BatchedLookupCountsCorrectlyAtScale verifies
// the batched GetBacklogItemsByExternalIDs lookup (replacing an N+1
// per-issue query loop) still produces the correct eligible count and
// sample titles across many closed issues, not just that it avoids
// crashing. Mixes eligible and ineligible items so a batching bug that
// dropped or mismatched rows (e.g. a map keyed wrong) would show up as a
// wrong count rather than passing by accident.
func TestPreviewBackwardSyncImpact_BatchedLookupCountsCorrectlyAtScale(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	registry := session.NewPluginRegistry()

	const numClosedIssues = 20
	var items []session.ExternalItem
	for i := 0; i < numClosedIssues; i++ {
		items = append(items, session.ExternalItem{
			ExternalID: fmt.Sprintf("e%d", i),
			Title:      fmt.Sprintf("Issue %d", i),
			State:      "closed",
		})
	}
	plugin := &fakeSourcePlugin{items: items}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	// Even-indexed issues get an eligible local status; odd-indexed get a
	// non-eligible one. Half of numClosedIssues should end up counted.
	wantTitles := make([]string, 0, numClosedIssues/2)
	for i := 0; i < numClosedIssues; i++ {
		status := string(session.BacklogStatusInProgress)
		if i%2 == 0 {
			status = string(session.BacklogStatusReady)
			wantTitles = append(wantTitles, fmt.Sprintf("Issue %d", i))
		}
		_, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
			Title: fmt.Sprintf("Issue %d", i), Status: status, SourceID: src.ID, ExternalID: fmt.Sprintf("e%d", i),
		})
		require.NoError(t, err)
	}

	resp, previewErr := svc.PreviewBackwardSyncImpact(t.Context(), connect.NewRequest(&sessionv1.PreviewBackwardSyncImpactRequest{
		SourceId: src.ID,
	}))
	require.NoError(t, previewErr)
	assert.Equal(t, int32(numClosedIssues/2), resp.Msg.ItemCount)
	// sampleTitles is capped (maxBackwardSyncPreviewSamples), so only assert
	// every returned title is a genuinely-eligible one.
	for _, title := range resp.Msg.SampleTitles {
		assert.Contains(t, wantTitles, title)
	}
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
