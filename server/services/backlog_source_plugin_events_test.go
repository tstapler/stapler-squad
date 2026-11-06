package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

func TestBacklogItemMatchesFilters_should_FilterBySourcePlugin(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.WatchBacklogItemsRequest{SourcePluginFilter: []string{"linear"}}
	require.True(t, backlogItemMatchesFilters(&session.BacklogItemData{SourcePluginID: "linear"}, msg))
	require.False(t, backlogItemMatchesFilters(&session.BacklogItemData{SourcePluginID: "github_issues"}, msg))
	require.False(t, backlogItemMatchesFilters(&session.BacklogItemData{}, msg))
	require.True(t, backlogItemMatchesFilters(&session.BacklogItemData{}, &sessionv1.WatchBacklogItemsRequest{}))
}

func TestWithSourcePlugin_should_FillPluginIDWithoutMutatingSharedEvent(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "linear", false, "")
	item := createForwardSyncTestItem(t, storage, sourceID, "ENG-1", nil)
	// Simulate a mutation-path payload re-read without the Source edge.
	stale := *item
	stale.SourcePluginID, stale.SourceID = "", ""
	evt := &events.Event{BacklogItemPayload: &events.BacklogItemEventPayload{Item: &stale}}

	got := withSourcePlugin(context.Background(), storage, evt)

	require.Equal(t, "linear", got.BacklogItemPayload.Item.SourcePluginID)
	require.Empty(t, evt.BacklogItemPayload.Item.SourcePluginID, "shared event must not be mutated")
}
