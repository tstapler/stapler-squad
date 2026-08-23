package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
		ExternalURL: item.URL,
		Labels:      item.Labels,
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

// newTestBackwardSyncSetup mirrors newTestSyncSetup but lets the caller
// control BackwardSyncEnabled on the created ItemSource — every Phase 2/3
// backward-sync test needs this opted in (or deliberately left off, for the
// disabled-gate regression tests).
func newTestBackwardSyncSetup(t *testing.T, plugin ItemSourcePlugin, backwardSyncEnabled bool) (*Storage, func(), *SyncLoop, string) {
	t.Helper()
	storage, cleanup := createTestStorage(t)

	registry := NewPluginRegistry()
	registry.Register(plugin)
	sl := NewSyncLoop(storage, registry)

	src, err := storage.CreateItemSource(context.Background(), ItemSourceData{
		PluginID:            plugin.PluginID(),
		DisplayName:         "Test Source",
		Enabled:             true,
		BackwardSyncEnabled: backwardSyncEnabled,
	})
	require.NoError(t, err)

	return storage, cleanup, sl, src.ID
}

func TestSyncOne_CreatesNewItemsFromPlugin(t *testing.T) {
	t.Parallel()
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
	er := storage.repo
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

	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 2, events[0].ItemsCreated)
}

// TestListSourceSyncEvents_ReportsTruncatedWhenOverCap verifies that querying more
// than maxSourceSyncEventsHistory rows returns truncated=true and caps the slice at
// the limit, while a source at or under the cap reports truncated=false — so the
// UI can show a "history not fully shown" indicator instead of silently hiding rows.
func TestListSourceSyncEvents_ReportsTruncatedWhenOverCap(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	src, err := storage.CreateItemSource(ctx, ItemSourceData{
		PluginID:    "fake_source",
		DisplayName: "Chatty Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	start := time.Now()
	for i := 0; i < maxSourceSyncEventsHistory+1; i++ {
		require.NoError(t, er.CreateSourceSyncEvent(ctx, src.ID, "", 1, 0, 0, 0, "", start, start))
	}

	events, truncated, err := er.ListSourceSyncEvents(ctx, src.ID)
	require.NoError(t, err)
	require.True(t, truncated, "expected truncated=true when more rows exist than the cap")
	require.Len(t, events, maxSourceSyncEventsHistory)
}

// TestListSourceSyncEvents_NotTruncatedAtOrUnderCap guards against the off-by-one
// edge case where a source with exactly maxSourceSyncEventsHistory rows would be
// misreported as truncated.
func TestListSourceSyncEvents_NotTruncatedAtOrUnderCap(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	src, err := storage.CreateItemSource(ctx, ItemSourceData{
		PluginID:    "fake_source",
		DisplayName: "Exactly At Cap",
		Enabled:     true,
	})
	require.NoError(t, err)

	start := time.Now()
	for i := 0; i < maxSourceSyncEventsHistory; i++ {
		require.NoError(t, er.CreateSourceSyncEvent(ctx, src.ID, "", 1, 0, 0, 0, "", start, start))
	}

	events, truncated, err := er.ListSourceSyncEvents(ctx, src.ID)
	require.NoError(t, err)
	require.False(t, truncated, "exactly at the cap must not be reported as truncated")
	require.Len(t, events, maxSourceSyncEventsHistory)
}

func TestSyncOne_LocalWinsSkipsUserModifiedFields(t *testing.T) {
	t.Parallel()
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

	er := storage.repo
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

	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
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
	t.Parallel()
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

	er := storage.repo
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["title","description","priority"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, 1, events[0].ItemsSkipped)
	require.Equal(t, 0, events[0].ItemsCreated)
	require.Equal(t, 0, events[0].ItemsUpdated)

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Local", refetched.Title, "all fields locked, so nothing should change")
}

func TestSyncOne_ReturnsErrorForUnregisteredPlugin(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{id: "fake"}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	entSrc.PluginID = "does-not-exist"

	err = sl.SyncOne(ctx, entSrc)
	require.Error(t, err)
}

func TestSyncOne_DecryptsEncryptedConfigTokenBeforeFetch(t *testing.T) {
	t.Parallel()
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

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, src.ID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	require.Contains(t, plugin.lastConfig.Raw, "plaintext-token")
	require.NotContains(t, plugin.lastConfig.Raw, "encrypted")
}

func TestSyncOne_FetchErrorPropagates(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{id: "fake", fetchErr: errors.New("boom")}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)

	err = sl.SyncOne(ctx, entSrc)
	require.Error(t, err)

	// A failed fetch must still leave a trace in sync history — otherwise the
	// failure vanishes silently instead of surfacing in GetSyncHistory.
	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 1, events[0].ItemsErrored)
	require.Contains(t, events[0].ErrorMessage, "boom")
}

// Two different sources can both have items numbered e.g. "1" (GitHub issue/PR
// numbers are per-repo, not globally unique). Syncing source B must not match
// against — and overwrite — an item that actually belongs to source A.
func TestSyncOne_DoesNotCollideAcrossSourcesWithSameExternalID(t *testing.T) {
	t.Parallel()
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

	er := storage.repo
	itemA, err := er.GetBacklogItemByExternalID(ctx, srcA.ID, "1")
	require.NoError(t, err)
	require.Equal(t, "Repo A Issue 1", itemA.Title)

	itemB, err := er.GetBacklogItemByExternalID(ctx, srcB.ID, "1")
	require.NoError(t, err)
	require.Equal(t, "Repo B Issue 1", itemB.Title)

	require.NotEqual(t, itemA.ID, itemB.ID, "sources with colliding external_id must produce distinct backlog items")

	eventsA, _, err := er.ListSourceSyncEvents(ctx, srcA.ID)
	require.NoError(t, err)
	require.Equal(t, 1, eventsA[0].ItemsCreated, "source B's sync must not count as an update to source A's item")
}

// TestGetBacklogItemsByExternalIDs_ScopesToSourceAndIgnoresMissing verifies
// the batched lookup (replacing PreviewBackwardSyncImpact's former N+1
// per-issue query loop) preserves GetBacklogItemByExternalID's two
// invariants at once: results are scoped per-source (no cross-source
// collision on numerically-identical external IDs), and external IDs with no
// local match are simply absent from the returned map rather than erroring.
func TestGetBacklogItemsByExternalIDs_ScopesToSourceAndIgnoresMissing(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	srcA, err := storage.CreateItemSource(ctx, ItemSourceData{PluginID: "fake-a", DisplayName: "Repo A", Enabled: true})
	require.NoError(t, err)
	srcB, err := storage.CreateItemSource(ctx, ItemSourceData{PluginID: "fake-b", DisplayName: "Repo B", Enabled: true})
	require.NoError(t, err)

	_, err = storage.CreateBacklogItem(ctx, BacklogItemData{Title: "A1", Status: string(BacklogStatusIdea), SourceID: srcA.ID, ExternalID: "1"})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, BacklogItemData{Title: "A2", Status: string(BacklogStatusIdea), SourceID: srcA.ID, ExternalID: "2"})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, BacklogItemData{Title: "B1", Status: string(BacklogStatusIdea), SourceID: srcB.ID, ExternalID: "1"})
	require.NoError(t, err)

	// "3" doesn't exist at all; "1" exists on both sources but this query is
	// scoped to srcA, so only srcA's item must come back.
	results, err := er.GetBacklogItemsByExternalIDs(ctx, srcA.ID, []string{"1", "2", "3"})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "A1", results["1"].Title)
	require.Equal(t, "A2", results["2"].Title)
	_, missingFound := results["3"]
	require.False(t, missingFound, "external ID with no local match must be absent from the map, not an error")

	// Empty input returns an empty map, not an error.
	empty, err := er.GetBacklogItemsByExternalIDs(ctx, srcA.ID, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

// Two concurrent syncs of the SAME source (e.g. a manual TriggerSync racing
// the periodic tick) must not both miss the same not-yet-created item's
// external_id lookup and both create it — the per-source lock in SyncOne
// (syncSourceLocks) must serialize them.
func TestSyncOne_ConcurrentSyncsOfSameSourceDoNotDuplicateItems(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id:    "fake",
		items: []ExternalItem{{ExternalID: "ext-1", Title: "Item", Priority: 3}},
	}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- sl.SyncOne(ctx, entSrc)
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}

	require.Equal(t, 2, plugin.fetchCalled, "both goroutines should have run (sequentially, not concurrently)")

	items, err := storage.ListBacklogItems(ctx, BacklogItemFilter{})
	require.NoError(t, err)
	matching := 0
	for _, it := range items {
		if it.ExternalID == "ext-1" {
			matching++
		}
	}
	require.Equal(t, 1, matching, "concurrent syncs of the same source must not create duplicate items")
}

func TestSyncByID_SyncsEvenWhenSourceDisabled(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// TestMergeUserModifiedFields pins the exact merge semantics MergeUserModifiedFields
// must provide (Epic 0.3, Story 0.3.1): order-independent set union, no
// duplicates when re-adding a field already present.
func TestMergeUserModifiedFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		newFields []string
		want      []string
	}{
		{
			name:      "empty raw plus one new field",
			raw:       "",
			newFields: []string{"title"},
			want:      []string{"title"},
		},
		{
			name:      "existing fields plus a duplicate of an existing field",
			raw:       `["title"]`,
			newFields: []string{"title"},
			want:      []string{"title"},
		},
		{
			name:      "existing fields plus a genuinely new field",
			raw:       `["title"]`,
			newFields: []string{"priority"},
			want:      []string{"title", "priority"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			merged, err := MergeUserModifiedFields(tt.raw, tt.newFields...)
			require.NoError(t, err)

			got := ParseUserModifiedFields(merged)
			require.ElementsMatch(t, tt.want, got)
			require.Len(t, got, len(tt.want), "must not contain duplicates")
		})
	}
}

// TestSyncOne_UserEditedTitleSurvivesSubsequentBackwardSync exercises the
// actual regression Epic 0.3 exists to fix (pitfalls research §1): a user
// edit to Title, once recorded in UserModifiedFields via MergeUserModifiedFields
// (the same helper the UpdateBacklogItem RPC handler uses in
// server/services/backlog_service_lifecycle.go — session cannot import
// server/services, so this seeds UserModifiedFields via the production merge
// helper rather than a live RPC call), must survive a subsequent backward
// sync tick that fetches a different title from the plugin. This proves the
// pre-existing containsField/ContainsModifiedField gate in SyncOne is
// actually reachable in production now that Epic 0.3 wires up the RPC path.
func TestSyncOne_UserEditedTitleSurvivesSubsequentBackwardSync(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Remote Title (Changed)", Description: "Remote Desc", Priority: 5},
		},
	}
	storage, cleanup, sl, sourceID := newTestSyncSetup(t, plugin)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:       "Original Title",
		Description: "Remote Desc",
		Priority:    1,
		Status:      string(BacklogStatusIdea),
		ExternalID:  "ext-1",
		SourceID:    sourceID,
	})
	require.NoError(t, err)

	// Simulate the UpdateBacklogItem RPC handler's value-diff + merge flow:
	// the user edits Title, so touchedFields = ["title"], merged into the
	// item's (currently empty) UserModifiedFields via the production helper.
	merged, err := MergeUserModifiedFields(created.UserModifiedFields, "title")
	require.NoError(t, err)
	newTitle := "User Edited Title"
	_, err = storage.UpdateBacklogItem(ctx, created.ID, BacklogItemUpdate{
		Title:              &newTitle,
		UserModifiedFields: &merged,
	}, nil)
	require.NoError(t, err)

	entSrc, err := storage.repo.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "User Edited Title", refetched.Title, "user-edited title must survive backward sync")
	require.Equal(t, 5, refetched.Priority, "priority was not user-modified, so remote wins")
}

// TestDetermineBackwardSyncTarget_ReturnsArchivedForPreWorkStatuses_And_NoTargetOtherwise
// pins ADR-002's decision table: pre-work statuses (idea/refining/ready/queued)
// map to archived; everything else (mid-flight in_progress/review/pr_pending,
// and the terminal done/archived statuses) has no valid target.
func TestDetermineBackwardSyncTarget_ReturnsArchivedForPreWorkStatuses_And_NoTargetOtherwise(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current    BacklogStatus
		wantTarget BacklogStatus
		wantOK     bool
	}{
		{BacklogStatusIdea, BacklogStatusArchived, true},
		{BacklogStatusRefining, BacklogStatusArchived, true},
		{BacklogStatusReady, BacklogStatusArchived, true},
		{BacklogStatusQueued, BacklogStatusArchived, true},
		{BacklogStatusInProgress, "", false},
		{BacklogStatusReview, "", false},
		{BacklogStatusPRPending, "", false},
		{BacklogStatusDone, "", false},
		{BacklogStatusArchived, "", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.current), func(t *testing.T) {
			t.Parallel()
			target, ok := determineBackwardSyncTarget(tt.current)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.wantTarget, target)
		})
	}
}

// TestSyncOne_BackwardSync_ClosedIssueArchivesReadyItem is the AC4 happy path:
// a pre-work item whose linked issue is observed closed transitions to
// archived, triggered via TriggeredByGitHubSync.
func TestSyncOne_BackwardSync_ClosedIssueArchivesReadyItem(t *testing.T) {
	t.Parallel()
	issueUpdatedAt := time.Now()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: issueUpdatedAt},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusReady),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusArchived), refetched.Status)
	require.NotNil(t, refetched.GitHubSyncedIssueUpdatedAt)
	require.True(t, refetched.GitHubSyncedIssueUpdatedAt.Equal(issueUpdatedAt))
}

// TestSyncOne_BackwardSync_ClosedIssueSkipsInProgressItem asserts that a
// mid-flight item's closed issue does not force a transition — archived is
// not reachable from in_progress under ADR-002's policy.
func TestSyncOne_BackwardSync_ClosedIssueSkipsInProgressItem(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: time.Now()},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusInProgress),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), refetched.Status, "no valid target for in_progress under ADR-002")
	// Regression guard: nothing changed locally on this skip branch, so the
	// loop-prevention watermark must NOT advance — otherwise a later manual
	// revert to a pre-work status (with no further GitHub-side change) would
	// have alreadyReconciled short-circuit before determineBackwardSyncTarget
	// is even consulted again, permanently suppressing an otherwise-
	// legitimate auto-archive. Mirrors the transition-failure branch's fix.
	require.Nil(t, refetched.GitHubSyncedIssueUpdatedAt, "watermark must not advance on a no-valid-target skip")

	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, 0, events[0].ItemsErrored)
	require.Greater(t, events[0].ItemsSkipped, 0, "should be counted as skipped, not errored")
}

// TestSyncOne_BackwardSync_NoValidTargetSkipAllowsLaterReprocessing proves the
// watermark fix end-to-end: a closed issue synced against a mid-flight item
// (no valid backward-sync target) must not permanently suppress
// reprocessing. If the item is later manually reverted to a pre-work status
// with no further GitHub-side change (same IssueUpdatedAt), the next sync
// tick must still archive it — which only happens if the first tick left the
// watermark unadvanced.
func TestSyncOne_BackwardSync_NoValidTargetSkipAllowsLaterReprocessing(t *testing.T) {
	t.Parallel()
	issueUpdatedAt := time.Now()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: issueUpdatedAt},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusInProgress),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)

	// Tick 1: closed issue vs. in_progress item — no valid target, skipped.
	require.NoError(t, sl.SyncOne(ctx, entSrc))
	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), refetched.Status)
	require.Nil(t, refetched.GitHubSyncedIssueUpdatedAt)

	// Simulate a manual revert to a pre-work status (e.g. the in-progress
	// session was abandoned) — no GitHub-side change accompanies this.
	_, err = storage.TransitionBacklogItemStatus(ctx, created.ID, BacklogStatusReady, nil, TriggeredBySystem)
	require.NoError(t, err)

	// Tick 2: same closed issue, same IssueUpdatedAt. If the watermark had
	// wrongly advanced on tick 1, alreadyReconciled would short-circuit here
	// and the item would never be archived.
	require.NoError(t, sl.SyncOne(ctx, entSrc))
	refetched, err = storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusArchived), refetched.Status, "tick 2 must still reprocess and archive")
}

// TestSyncOne_BackwardSync_NoOpWhenBackwardSyncDisabled proves the master
// gate: with BackwardSyncEnabled == false, a closed issue never triggers a
// status transition even for an otherwise-eligible pre-work item.
func TestSyncOne_BackwardSync_NoOpWhenBackwardSyncDisabled(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: time.Now()},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, false)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusReady),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReady), refetched.Status, "backward sync disabled — no transition")
	require.Nil(t, refetched.GitHubSyncedIssueUpdatedAt, "watermark must not advance when backward sync is disabled")
}

// TestSyncOne_BackwardSync_DoesNotReArchiveAlreadyDoneItem pins ADR-002's
// explicit "done is left alone" decision: forward sync closing the issue on
// the transition into done is the expected, common path — not something
// backward sync should react to by auto-archiving.
func TestSyncOne_BackwardSync_DoesNotReArchiveAlreadyDoneItem(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: time.Now()},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusDone),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), refetched.Status, "done items are never auto-archived")
}

// TestSyncOne_BackwardSync_ReopenedIssueOnArchivedItemLogsNoOp pins ADR-002's
// documented no-op for reopened issues: archived's only outgoing edge is to
// idea, and this policy deliberately never fires that transition
// automatically. The watermark still advances so the reopen isn't re-logged
// every tick.
func TestSyncOne_BackwardSync_ReopenedIssueOnArchivedItemLogsNoOp(t *testing.T) {
	t.Parallel()
	issueUpdatedAt := time.Now()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "open", IssueUpdatedAt: issueUpdatedAt},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusArchived),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusArchived), refetched.Status, "reopen is log-only; no automatic re-triage")
	require.NotNil(t, refetched.GitHubSyncedIssueUpdatedAt, "watermark must advance so the reopen isn't re-logged every tick")
	require.True(t, refetched.GitHubSyncedIssueUpdatedAt.Equal(issueUpdatedAt))
}

// TestSyncOne_BackwardSync_UpdatesLabelsWhenNotUserLocked is the AC4 Labels
// happy path: an item with existing labels picks up the remote's updated
// label set when BackwardSyncEnabled is on and "labels" isn't user-locked.
func TestSyncOne_BackwardSync_UpdatesLabelsWhenNotUserLocked(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, Labels: []string{"bug", "p1"}},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusIdea),
		ExternalID: "ext-1",
		SourceID:   sourceID,
		Labels:     []string{"bug"},
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"bug", "p1"}, refetched.Labels)
}

// TestSyncOne_BackwardSync_SkipsLabelsWhenUserLocked exercises Epic 0.3's
// local-wins gate for the "labels" field name, seeded directly since no
// production UI path sets it today (documented in the Domain Glossary).
func TestSyncOne_BackwardSync_SkipsLabelsWhenUserLocked(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, Labels: []string{"bug", "p1"}},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusIdea),
		ExternalID: "ext-1",
		SourceID:   sourceID,
		Labels:     []string{"bug"},
	})
	require.NoError(t, err)

	er := storage.repo
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["labels"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"bug"}, refetched.Labels, "user-locked labels must not be overwritten")
}

// TestSyncOne_BackwardSync_SkipsLabelsWhenBackwardSyncDisabled is the
// regression test for the 2026-08-03 validation-pass gating fix on Task
// 2.3.1a: without the BackwardSyncEnabled gate, Labels would sync
// unconditionally regardless of the per-source opt-in.
func TestSyncOne_BackwardSync_SkipsLabelsWhenBackwardSyncDisabled(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, Labels: []string{"bug", "p1"}},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, false)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusIdea),
		ExternalID: "ext-1",
		SourceID:   sourceID,
		Labels:     []string{"bug"},
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"bug"}, refetched.Labels, "backward sync disabled — remote labels must not overwrite local")
}

// TestSyncOne_BackfillsLabelsOnExistingItemWithNoLabels is AC6's happy path:
// even with every other field locally locked, an item that has never had
// labels gets them backfilled from the remote once (BackwardSyncEnabled on,
// "labels" not itself locked).
func TestSyncOne_BackfillsLabelsOnExistingItemWithNoLabels(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Remote Title", Description: "Remote Desc", Priority: 5, Labels: []string{"bug"}},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:       "Local Title",
		Description: "Local Desc",
		Priority:    1,
		Status:      string(BacklogStatusIdea),
		ExternalID:  "ext-1",
		SourceID:    sourceID,
		Labels:      nil,
	})
	require.NoError(t, err)

	er := storage.repo
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["title","description","priority"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"bug"}, refetched.Labels, "labels must be backfilled even with other fields locked")
	require.Equal(t, "Local Title", refetched.Title, "locked title must survive")
}

// TestSyncOne_BackfillsLabelsRespectsUserModifiedFieldsGate pins the
// deliberate asymmetry AC6 calls out: unlike ExternalURL, Labels backfill IS
// gated by UserModifiedFields — if "labels" is itself locked, it stays locked
// even though the item has no labels yet.
func TestSyncOne_BackfillsLabelsRespectsUserModifiedFieldsGate(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, Labels: []string{"bug"}},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusIdea),
		ExternalID: "ext-1",
		SourceID:   sourceID,
		Labels:     nil,
	})
	require.NoError(t, err)

	er := storage.repo
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["labels"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Empty(t, refetched.Labels, "labels locked via UserModifiedFields must not be backfilled")
}

// TestSyncOne_BackfillsExternalURLEvenWhenAllOtherFieldsAreUserModified pins
// ADR-001 Decision 1: ExternalURL backfills unconditionally, independent of
// both UserModifiedFields and BackwardSyncEnabled — unlike the Labels block,
// which is gated by both.
func TestSyncOne_BackfillsExternalURLEvenWhenAllOtherFieldsAreUserModified(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Remote Title", Description: "Remote Desc", Priority: 5, URL: "https://github.com/example/repo/issues/1"},
		},
	}
	// BackwardSyncEnabled deliberately false — ExternalURL backfill must still fire.
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, false)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:       "Local Title",
		Description: "Local Desc",
		Priority:    1,
		Status:      string(BacklogStatusIdea),
		ExternalID:  "ext-1",
		SourceID:    sourceID,
		ExternalURL: "",
	})
	require.NoError(t, err)

	er := storage.repo
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["title","description","priority","labels"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/example/repo/issues/1", refetched.ExternalURL)
	require.Equal(t, "Local Title", refetched.Title, "locked title must survive")
}

// TestSyncOne_BackwardSync_DoneItemClosedIssueIsNoOpEvenWithoutWatermark pins
// AC7 Risk A: a done item's closed issue is structurally impossible to
// re-archive/re-close via determineBackwardSyncTarget alone — proven without
// relying on the watermark at all (GitHubSyncedIssueUpdatedAt deliberately
// left nil).
func TestSyncOne_BackwardSync_DoneItemClosedIssueIsNoOpEvenWithoutWatermark(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: time.Now()},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusDone),
		ExternalID: "ext-1",
		SourceID:   sourceID,
		// GitHubSyncedIssueUpdatedAt deliberately left nil.
	})
	require.NoError(t, err)
	require.Nil(t, created.GitHubSyncedIssueUpdatedAt)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), refetched.Status, "done→archived self-edge must never fire, watermark or not")
}

// TestSyncOne_BackwardSync_ManualReopenAfterForwardSyncCloseIsNotReClosed
// pins AC7 Risk B — the watermark's actual job: after forward sync closed the
// issue and recorded the watermark, and the item was then manually
// transitioned back to in_progress locally, the next tick's fetch returning
// the SAME (unchanged) IssueUpdatedAt must be treated as already-reconciled
// and must not push the item back toward archived/done.
func TestSyncOne_BackwardSync_ManualReopenAfterForwardSyncCloseIsNotReClosed(t *testing.T) {
	t.Parallel()
	t1 := time.Now().Add(-1 * time.Hour)
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: t1},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:                      "Issue",
		Status:                     string(BacklogStatusInProgress),
		ExternalID:                 "ext-1",
		SourceID:                   sourceID,
		GitHubSyncedIssueUpdatedAt: &t1,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), refetched.Status, "exact-echo of an already-reconciled watermark must not re-trigger a transition")
}

// TestSyncOne_BackwardSync_GenuinelyNewerExternalCloseIsProcessed is AC7's
// companion happy path: once the fetched IssueUpdatedAt genuinely advances
// past the stored watermark, backward sync evaluates the state fresh again —
// proving the watermark only suppresses the exact-echo case.
func TestSyncOne_BackwardSync_GenuinelyNewerExternalCloseIsProcessed(t *testing.T) {
	t.Parallel()
	t1 := time.Now().Add(-1 * time.Hour)
	t2 := time.Now()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: t2},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:                      "Issue",
		Status:                     string(BacklogStatusReady),
		ExternalID:                 "ext-1",
		SourceID:                   sourceID,
		GitHubSyncedIssueUpdatedAt: &t1,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusArchived), refetched.Status, "genuinely newer external state must be processed")
	require.NotNil(t, refetched.GitHubSyncedIssueUpdatedAt)
	require.True(t, refetched.GitHubSyncedIssueUpdatedAt.Equal(t2))
}

// alwaysDenyWorkflowEngine is a WorkflowEngine stub whose CanTransition
// always returns false, used to force GuardedTransitionAllowed(...) == false
// even when determineBackwardSyncTarget finds a structurally valid target —
// exercising SyncOne's guard-denied branch (backlog_sync.go's `if
// GuardedTransitionAllowed(...) { ... } else { ...skipped++ }`), which no
// existing test reached: the no-valid-target skip
// (TestSyncOne_BackwardSync_ClosedIssueSkipsInProgressItem) only covers the
// determineBackwardSyncTarget-returns-false branch, a different code path.
type alwaysDenyWorkflowEngine struct{}

func (alwaysDenyWorkflowEngine) CanTransition(from, to BacklogStatus) bool { return false }
func (alwaysDenyWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error {
	return nil
}
func (alwaysDenyWorkflowEngine) AllowedTransitions(from BacklogStatus) []BacklogStatus { return nil }

// TestSyncOne_BackwardSync_GuardDeniedTransitionIsSkippedNotApplied is the
// regression test for the GuardedTransitionAllowed-returns-false branch
// inside SyncOne's closed-issue block: determineBackwardSyncTarget finds a
// structurally valid target (ready -> archived), but the injected workflow
// engine's guard denies it. The item must stay at its original status
// (never transitioned) and be counted as skipped, not updated or errored.
func TestSyncOne_BackwardSync_GuardDeniedTransitionIsSkippedNotApplied(t *testing.T) {
	t.Parallel()
	issueUpdatedAt := time.Now()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: issueUpdatedAt},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()
	sl.workflowEngine = alwaysDenyWorkflowEngine{}

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusReady),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReady), refetched.Status, "a guard-denied transition must never be applied")

	// Note: ItemsUpdated can still be non-zero here — Title/Description/
	// Priority field-sync (unconditional, separate from the backward-sync
	// status block) may independently count as an update in the same tick.
	// The status-block-specific assertion is refetched.Status staying Ready.
	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, 0, events[0].ItemsErrored)
	require.Greater(t, events[0].ItemsSkipped, 0, "guard-denied transition should be counted as skipped")
}

// TestSyncOne_BackwardSync_ZeroIssueUpdatedAtDoesNotFalselyReconcile is the
// regression test for the Copilot review finding on IssueUpdatedAt: a fetched
// item with a zero (unparsed/missing) IssueUpdatedAt must not be treated as
// "already reconciled" against a real existing watermark — a zero time.Time
// is never After() anything, so without the IsZero() guard, alreadyReconciled
// would short-circuit forever and the closed issue would never be archived,
// even though the real watermark is a well-defined, older timestamp.
func TestSyncOne_BackwardSync_ZeroIssueUpdatedAtDoesNotFalselyReconcile(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			// IssueUpdatedAt deliberately left at its zero value, simulating a
			// GitHub updated_at that failed to parse.
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed"},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	existingWatermark := time.Now().Add(-1 * time.Hour)
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:                      "Issue",
		Status:                     string(BacklogStatusReady),
		ExternalID:                 "ext-1",
		SourceID:                   sourceID,
		GitHubSyncedIssueUpdatedAt: &existingWatermark,
	})
	require.NoError(t, err)

	er := storage.repo
	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReady), refetched.Status, "a zero IssueUpdatedAt must not falsely short-circuit as already-reconciled or apply any transition")
	require.NotNil(t, refetched.GitHubSyncedIssueUpdatedAt)
	require.True(t, refetched.GitHubSyncedIssueUpdatedAt.Equal(existingWatermark), "the real existing watermark must not be overwritten with the zero value")
}

// TestSyncOne_BackwardSync_ClosedIssueTransitionCountsAsUpdatedOnce is the
// regression test for the Copilot review finding on double-counting: an item
// archived via the closed-issue backward-sync block must be counted in
// ItemsUpdated exactly once — not also in ItemsSkipped — since status
// transitions bypass BacklogItemUpdate/anyField entirely and used to always
// fall through to the generic `if !anyField { skipped++ }` fallback too.
// Title/description/priority are locked via UserModifiedFields so anyField
// stays false — isolating the status-transition-only path the finding
// describes (an item with unlocked content fields also gets a second,
// legitimate updated++ from the field-sync path below, which is a separate,
// pre-existing concern this fix does not change).
func TestSyncOne_BackwardSync_ClosedIssueTransitionCountsAsUpdatedOnce(t *testing.T) {
	t.Parallel()
	plugin := &fakeSyncPlugin{
		id: "fake",
		items: []ExternalItem{
			{ExternalID: "ext-1", Title: "Issue", Priority: 3, State: "closed", IssueUpdatedAt: time.Now()},
		},
	}
	storage, cleanup, sl, sourceID := newTestBackwardSyncSetup(t, plugin, true)
	defer cleanup()

	ctx := context.Background()
	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:      "Issue",
		Status:     string(BacklogStatusReady),
		ExternalID: "ext-1",
		SourceID:   sourceID,
	})
	require.NoError(t, err)

	er := storage.repo
	createdUUID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	// Also lock labels — otherwise Labels' own unconditional-under-
	// BackwardSyncEnabled write (Epic 2.3) would set anyField=true itself,
	// masking the status-transition-only path under test.
	_, err = er.client.BacklogItem.UpdateOneID(createdUUID).
		SetUserModifiedFields(`["title","description","priority","labels"]`).
		Save(ctx)
	require.NoError(t, err)

	entSrc, err := er.GetItemSourceByID(ctx, sourceID)
	require.NoError(t, err)
	require.NoError(t, sl.SyncOne(ctx, entSrc))

	refetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusArchived), refetched.Status)

	events, _, err := er.ListSourceSyncEvents(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, 1, events[0].ItemsUpdated, "the archived item must be counted in updated exactly once")
	require.Equal(t, 0, events[0].ItemsSkipped, "the archived item must not ALSO be counted as skipped")
	require.Equal(t, 0, events[0].ItemsErrored)
	require.Equal(t, 1, events[0].ItemsCreated+events[0].ItemsUpdated+events[0].ItemsSkipped+events[0].ItemsErrored, "aggregate counts must partition the single synced item exactly once")
}
