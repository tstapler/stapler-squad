package svctest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// newForwardSyncTestStorage creates a temporary ent-backed *session.Storage,
// mirroring the pattern in backlog_service_triage_test.go /
// backlog_item_event_publisher_test.go.
func NewForwardSyncTestStorage(t *testing.T) *session.Storage {
	t.Helper()
	repo := session.NewTestEntRepository(t)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// createForwardSyncTestSource creates an ItemSource row and returns its ID.
func CreateForwardSyncTestSource(t *testing.T, storage *session.Storage, pluginID string, forwardSyncEnabled bool, closeLabel string) string {
	t.Helper()
	src, err := storage.CreateItemSource(context.Background(), session.ItemSourceData{
		PluginID:              pluginID,
		DisplayName:           "test source",
		Config:                `{}`,
		Enabled:               true,
		ForwardSyncEnabled:    forwardSyncEnabled,
		ForwardSyncCloseLabel: closeLabel,
	})
	require.NoError(t, err)
	return src.ID
}

// createForwardSyncTestItem creates a BacklogItemData linked to sourceID with
// the given externalID/labels.
func CreateForwardSyncTestItem(t *testing.T, storage *session.Storage, sourceID, externalID string, labels []string) *session.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:      "forward sync test item",
		Status:     string(session.BacklogStatusDone),
		SourceID:   sourceID,
		ExternalID: externalID,
		Labels:     labels,
	})
	require.NoError(t, err)
	return item
}
