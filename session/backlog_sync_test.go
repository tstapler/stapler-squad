package session

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeSyncPlugin lets tests control exactly what Fetch returns and inspect the
// config it was called with (e.g. to assert token decryption happened).
type fakeSyncPlugin struct {
	id          string
	items       []ExternalItem
	newCursor   string
	fetchErr    error
	lastConfig  PluginConfig
	lastCursor  string
	fetchCalled int
}

func (f *fakeSyncPlugin) PluginID() string { return f.id }

func (f *fakeSyncPlugin) Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error) {
	f.fetchCalled++
	f.lastConfig = config
	f.lastCursor = cursor
	if f.fetchErr != nil {
		return nil, cursor, f.fetchErr
	}
	return f.items, f.newCursor, nil
}

func (f *fakeSyncPlugin) MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData {
	return BacklogItemData{
		Title:       item.Title,
		Description: item.Description,
		Priority:    item.Priority,
		Status:      string(BacklogStatusIdea),
		ExternalID:  item.ExternalID,
		SourceID:    sourceID,
	}
}

func newTestSyncSetup(t *testing.T, plugin ItemSourcePlugin) (*Storage, func(), *SyncLoop, string) {
	t.Helper()
	storage, cleanup := createTestStorage(t)

	registry := NewPluginRegistry()
	registry.Register(plugin)
	sl := NewSyncLoop(storage, registry)

	src, err := storage.CreateItemSource(context.Background(), ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Test Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	return storage, cleanup, sl, src.ID
}

func TestSyncOne_CreatesNewItemsFromPlugin(t *testing.T) {
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Item One", Priority: 2},
			{ExternalID: "ext-2", Title: "Item Two", Priority: 3},
		},
		newCursor: "cursor-2",
	}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	er := storage.repo.(*EntRepository)
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)

	err = sl.SyncOne(ctx, entSrc)
	require.NoError(t, err)

	item1, err := er.GetBacklogItemByExternalID(ctx, sourceID, "ext-1")
	require.NoError(t, err)
	require.Equal(t, "Item One", item1.Title)

	updatedSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, "cursor-2", updatedSrc.SyncCursor)

	events, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 2, events[0].ItemsCreated)
}

func TestSyncOne_LocalWinsSkipsUserModifiedFields(t *testing.T) {
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Remote Title", Description: "Remote Desc", Priority: 5},
		},
	}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:       "Local Title",
		Description: "Remote Desc",
		Priority:    1,
		Status:      string(BacklogStatusIdea),
		ExternalID:  "ext-1",
		SourceID:    sourceID,
	})
	require.NoError(t, err)

	er := storage.repo.(*EntRepository)
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["title"]`).
		Save(ctx)
	require.NoError(t, err)
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Local Title", refetched.Title, "user-modified title must not be overwritten")
	require.Equal(t, 5, refetched.Priority, "priority was not user-modified, so remote wins")

	events, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, 1, events[0].ItemsUpdated)
	require.Equal(t, 0, events[0].ItemsCreated)
	require.Equal(t, 0, events[0].ItemsSkipped)
}

// SyncOne only skips an existing item when every syncable field (title,
// description, priority) is locked via UserModifiedFields — it does not
// compare old vs. new values, so a field not present in UserModifiedFields
// is always re-applied even if the fetched value is unchanged.
func TestSyncOne_SkipsWhenAllFieldsAreUserModified(t *testing.T) {
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Remote", Description: "Remote", Priority: 5},
		},
	}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:       "Local",
		Description: "Local",
		Priority:    2,
		Status:      string(BacklogStatusIdea),
		ExternalID:  "ext-1",
		SourceID:    sourceID,
	})
	require.NoError(t, err)

	er := storage.repo.(*EntRepository)
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["title","description","priority"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	events, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, 1, events[0].ItemsSkipped)
	require.Equal(t, 0, events[0].ItemsCreated)
	require.Equal(t, 0, events[0].ItemsUpdated)

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Local", refetched.Title, "all fields locked, so nothing should change")
}

func TestSyncOne_ReturnsErrorForUnregisteredPlugin(t *testing.T) {
	plugin := &fakeSyncPlugin{id: "fake"}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	er := storage.repo.(*EntRepository)
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	entSrc.PluginID = "does-not-exist"

	err = sl.SyncOne(ctx, entSrc)
	require.Error(t, err)
}

func TestSyncOne_DecryptsEncryptedConfigTokenBeforeFetch(t *testing.T) {
	plugin := &fakeSyncPlugin{id: "fake"}
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	registry := NewPluginRegistry()
	registry.Register(plugin)

	key := make([]byte, 32)
	keyFunc := func() ([]byte, error) { return key, nil }
	sl := NewSyncLoopWithKeyProvider(storage, registry, keyFunc)

	encToken, err := EncryptToken(key, "plaintext-token")
	require.NoError(t, err)

	ctx := context.Background()
	src, err := storage.CreateItemSource(ctx, ItemSourceData{
		PluginID:    "fake",
		DisplayName: "Encrypted Source",
		Enabled:     true,
		Config:      `{"encrypted":true,"token":"` + encToken + `"}`,
	})
	require.NoError(t, err)

	er := storage.repo.(*EntRepository)
	entSrc, err := er.GetItemSourceByID(ctx, src.ID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	require.Contains(t, plugin.lastConfig.Raw, "plaintext-token")
	require.NotContains(t, plugin.lastConfig.Raw, "encrypted")
}

func TestSyncOne_FetchErrorPropagates(t *testing.T) {
	plugin := &fakeSyncPlugin{id: "fake", fetchErr: errors.New("boom")}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	er := storage.repo.(*EntRepository)
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)

	err = sl.SyncOne(ctx, entSrc)
	require.Error(t, err)

	// A failed fetch must still leave a trace in sync history — otherwise the
	// failure vanishes silently instead of surfacing in GetSyncHistory.
	events, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 1, events[0].ItemsErrored)
	require.Contains(t, events[0].ErrorMessage, "boom")
}

// Two different sources can both have items numbered e.g. "1" (GitHub issue/PR
// numbers are per-repo, not globally unique). Syncing source B must not match
// against — and overwrite — an item that actually belongs to source A.
func TestSyncOne_DoesNotCollideAcrossSourcesWithSameExternalID(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	registry := NewPluginRegistry()
	pluginA := &fakeSyncPlugin{id: "fake-a", items: []ExternalItem{{ExternalID: "1", Title: "Repo A Issue 1"}}}
	pluginB := &fakeSyncPlugin{id: "fake-b", items: []ExternalItem{{ExternalID: "1", Title: "Repo B Issue 1"}}}
	registry.Register(pluginA)
	registry.Register(pluginB)
	sl := NewSyncLoop(storage, registry)

	srcA, err := storage.CreateItemSource(ctx, ItemSourceData{PluginID: "fake-a", DisplayName: "Repo A", Enabled: true})
	require.NoError(t, err)
	srcB, err := storage.CreateItemSource(ctx, ItemSourceData{PluginID: "fake-b", DisplayName: "Repo B", Enabled: true})
	require.NoError(t, err)

	require.NoError(t, sl.SyncByID(ctx, srcA.ID))
	require.NoError(t, sl.SyncByID(ctx, srcB.ID))

	er := storage.repo.(*EntRepository)
	itemA, err := er.GetBacklogItemByExternalID(ctx, srcA.ID, "1")
	require.NoError(t, err)
	require.Equal(t, "Repo A Issue 1", itemA.Title)

	itemB, err := er.GetBacklogItemByExternalID(ctx, srcB.ID, "1")
	require.NoError(t, err)
	require.Equal(t, "Repo B Issue 1", itemB.Title)

	require.NotEqual(t, itemA.ID, itemB.ID, "sources with colliding external_id must produce distinct backlog items")

	eventsA, err := er.ListSourceSyncEvents(ctx, srcA.ID)
	require.NoError(t, err)
	require.Equal(t, 1, eventsA[0].ItemsCreated, "source B's sync must not count as an update to source A's item")
}

func TestSyncByID_SyncsEvenWhenSourceDisabled(t *testing.T) {
	plugin := &fakeSyncPlugin{
		id:    "fake",
		items: []ExternalItem{{ExternalID: "ext-1", Title: "Item"}},
	}
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	registry := NewPluginRegistry()
	registry.Register(plugin)
	sl := NewSyncLoop(storage, registry)

	ctx := context.Background()
	src, err := storage.CreateItemSource(ctx, ItemSourceData{
		PluginID:    "fake",
		DisplayName: "Disabled Source",
		Enabled:     false,
	})
	require.NoError(t, err)

	require.NoError(t, sl.SyncByID(ctx, src.ID))
	require.Equal(t, 1, plugin.fetchCalled)
}

func TestSyncByID_ReturnsErrNotFoundForMissingSource(t *testing.T) {
	plugin := &fakeSyncPlugin{id: "fake"}
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	registry := NewPluginRegistry()
	registry.Register(plugin)
	sl := NewSyncLoop(storage, registry)

	err := sl.SyncByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
}
