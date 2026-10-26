package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

type fakeProgressPlugin struct {
	fakeNonCloserPlugin
	updatedAt time.Time
	err       error

	mu     sync.Mutex
	bodies []string
}

func (p *fakeProgressPlugin) PostProgressComment(_ context.Context, _ session.PluginConfig, _, body string) (time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bodies = append(p.bodies, body)
	return p.updatedAt, p.err
}

func (p *fakeProgressPlugin) posted() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.bodies...)
}

func newProgressFixture(t *testing.T, fake *fakeProgressPlugin, prURL string) (*session.Storage, *session.PluginRegistry, *session.SyncLoop, *session.BacklogItemData) {
	t.Helper()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, fake.pluginID, false, "")
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:          "progress item",
		Status:         string(session.BacklogStatusInProgress),
		SourceID:       sourceID,
		ExternalID:     "42",
		ExternalURL:    "https://github.com/o/r/issues/42",
		ClaimingHostID: "host-abc",
	})
	require.NoError(t, err)
	if prURL != "" {
		item, err = storage.UpdateBacklogItem(context.Background(), item.ID, session.BacklogItemUpdate{PrURL: &prURL}, nil)
		require.NoError(t, err)
	}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	return storage, registry, session.NewSyncLoop(storage, registry), item
}

func TestProgressComment_InProgressPostsHostAndAdvancesWatermark(t *testing.T) {
	t.Parallel()
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}, updatedAt: ts}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "")

	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "in_progress")

	require.Len(t, fake.posted(), 1)
	require.Contains(t, fake.posted()[0], "host-abc")
	got, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, "in_progress", got.GitHubLastProgressCommentStatus)
	require.NotNil(t, got.GitHubSyncedIssueUpdatedAt)
	require.True(t, ts.Equal(*got.GitHubSyncedIssueUpdatedAt))
}

func TestProgressComment_ReEnteringInProgressDoesNotRepost(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "")

	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "in_progress")
	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "in_progress")

	require.Len(t, fake.posted(), 1)
}

func TestProgressComment_ReworkCycleDoesNotReannounceInProgress(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "https://github.com/o/r/pull/7")

	for _, st := range []string{"in_progress", "review", "in_progress", "review"} {
		handleProgressComment(context.Background(), registry, syncLoop, storage, item, st)
	}

	require.Len(t, fake.posted(), 2, "one in_progress and one review comment")
}

func TestProgressComment_ReviewPostsPRURL(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "https://github.com/o/r/pull/7")

	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "review")

	require.Len(t, fake.posted(), 1)
	require.Contains(t, fake.posted()[0], "https://github.com/o/r/pull/7")
}

func TestProgressComment_ReviewWithoutPRURLSkips(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "")

	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "review")

	require.Empty(t, fake.posted())
}

func TestProgressComment_ReviewWithMalformedPRURLOmitsLink(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "https://evil.example/x) [click](http://bad")

	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "review")

	require.Len(t, fake.posted(), 1)
	require.NotContains(t, fake.posted()[0], "evil.example")
}

func TestProgressComment_ReadBackFailureKeepsWatermarkButRecordsStatus(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "")

	handleProgressComment(context.Background(), registry, syncLoop, storage, item, "in_progress")

	got, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, "in_progress", got.GitHubLastProgressCommentStatus)
	require.Nil(t, got.GitHubSyncedIssueUpdatedAt, "a guessed watermark could skip real remote edits")
}

func TestProgressComment_PostFailureLeavesWatermarkUnset(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}, err: errors.New("token is required")}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "")

	require.NotPanics(t, func() {
		handleProgressComment(context.Background(), registry, syncLoop, storage, item, "in_progress")
	})
	got, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Empty(t, got.GitHubLastProgressCommentStatus)
}

func TestProgressSubscriber_FlagOffPostsNothing_FlagOnPosts(t *testing.T) {
	t.Parallel()
	fake := &fakeProgressPlugin{fakeNonCloserPlugin: fakeNonCloserPlugin{pluginID: "fake_progress"}}
	storage, registry, syncLoop, item := newProgressFixture(t, fake, "")

	bus := events.NewEventBus(10)
	defer bus.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	enabled := false
	flagCalls := 0
	StartBacklogGitHubProgressSyncSubscriber(ctx, bus, func() bool { mu.Lock(); defer mu.Unlock(); flagCalls++; return enabled }, registry, syncLoop, storage)
	calls := func() int { mu.Lock(); defer mu.Unlock(); return flagCalls }

	publish := func() {
		bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
			Kind: events.BacklogChangeStatusTransition, Item: item, NewStatus: "in_progress",
		}))
	}
	publish()
	// The flag is read before any post, so once it has been consulted with the flag off the event is fully handled.
	wait.RequireEventually(t, func() bool { return calls() == 1 }, 2*time.Second, 10*time.Millisecond, "expected the flag to be consulted")
	require.Empty(t, fake.posted())

	mu.Lock()
	enabled = true
	mu.Unlock()
	publish()
	wait.RequireEventually(t, func() bool { return len(fake.posted()) == 1 }, 2*time.Second, 10*time.Millisecond, "expected a progress comment once the flag is on")
}

func TestResolveProgressTarget_SourcelessGitHubIssueURL(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	registry := session.NewPluginRegistry()
	syncLoop := session.NewSyncLoop(storage, registry)

	_, cfg, id, ok := resolveProgressTarget(context.Background(), registry, syncLoop, storage, &session.BacklogItemData{ExternalURL: "https://github.com/o/r/issues/5"})
	require.True(t, ok)
	require.Equal(t, "5", id)
	require.Contains(t, cfg.Raw, `"owner":"o"`)

	_, _, _, ok = resolveProgressTarget(context.Background(), registry, syncLoop, storage, &session.BacklogItemData{ExternalURL: "https://example.com/not-github"})
	require.False(t, ok)
}

func TestForwardSyncClose_PostsCommentWithHostID(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_closer", true, "")
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title: "t", Status: string(session.BacklogStatusDone), SourceID: sourceID, ExternalID: "42", ClaimingHostID: "host-abc",
	})
	require.NoError(t, err)
	fake := &fakeForwardSyncCloserPlugin{pluginID: "fake_closer", closeUpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	registry := session.NewPluginRegistry()
	registry.Register(fake)

	handleForwardSyncClose(context.Background(), registry, session.NewSyncLoop(storage, registry), storage, item)

	require.Equal(t, 1, fake.commentCallCount())
	require.Contains(t, fake.commentCalls[0].body, "`host-abc`")
}

func TestForwardSyncCloseCommentFor_IncludesHostWhenKnown(t *testing.T) {
	t.Parallel()
	require.Equal(t, forwardSyncCloseComment, forwardSyncCloseCommentFor(""))
	require.Contains(t, forwardSyncCloseCommentFor("host-abc"), "`host-abc`")
}

func TestCreateBacklogItem_StampsClaimingHostOnlyWithExternalURL(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	storage.SetClaimRecorder(stubHostRecorder{id: "host-xyz"})

	withURL, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "a", Status: "idea", ExternalURL: "https://github.com/o/r/issues/1"})
	require.NoError(t, err)
	require.Equal(t, "host-xyz", withURL.ClaimingHostID)
	local, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "b", Status: "idea"})
	require.NoError(t, err)
	require.Empty(t, local.ClaimingHostID)
}

type stubHostRecorder struct{ id string }

func (stubHostRecorder) RecordClaim(context.Context, session.ClaimRecord) error { return nil }
func (s stubHostRecorder) LocalHostID() string                                  { return s.id }
