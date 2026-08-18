package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// fakeForwardSyncCloserPlugin implements session.ItemSourcePlugin plus the
// externalIssueCloser interface (CloseIssue/PostIssueComment), standing in for
// GitHubIssuesPlugin without making real HTTP calls. All fields are
// mutex-guarded since the subscriber invokes it from its own goroutine while
// tests assert on it from the main test goroutine.
type fakeForwardSyncCloserPlugin struct {
	pluginID string

	closeErr       error
	closeUpdatedAt time.Time
	commentErr     error

	mu           sync.Mutex
	closeCalls   []fakeCloseCall
	commentCalls []fakeCommentCall
}

type fakeCloseCall struct {
	externalID     string
	existingLabels []string
	closeLabel     string
}

type fakeCommentCall struct {
	externalID string
	body       string
}

func (p *fakeForwardSyncCloserPlugin) PluginID() string { return p.pluginID }

func (p *fakeForwardSyncCloserPlugin) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return nil, cursor, nil
}

func (p *fakeForwardSyncCloserPlugin) MapToBacklogItem(_ session.ExternalItem, _ string) session.BacklogItemData {
	return session.BacklogItemData{}
}

func (p *fakeForwardSyncCloserPlugin) CloseIssue(_ context.Context, _ session.PluginConfig, externalID string, existingLabels []string, closeLabel string) (time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeCalls = append(p.closeCalls, fakeCloseCall{externalID: externalID, existingLabels: existingLabels, closeLabel: closeLabel})
	if p.closeErr != nil {
		return time.Time{}, p.closeErr
	}
	return p.closeUpdatedAt, nil
}

func (p *fakeForwardSyncCloserPlugin) PostIssueComment(_ context.Context, _ session.PluginConfig, externalID string, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commentCalls = append(p.commentCalls, fakeCommentCall{externalID: externalID, body: body})
	return p.commentErr
}

func (p *fakeForwardSyncCloserPlugin) closeCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.closeCalls)
}

func (p *fakeForwardSyncCloserPlugin) lastCloseCall() fakeCloseCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeCalls[len(p.closeCalls)-1]
}

func (p *fakeForwardSyncCloserPlugin) commentCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.commentCalls)
}

// fakeNonCloserPlugin implements session.ItemSourcePlugin only — no
// CloseIssue/PostIssueComment — mirroring the shape of GitHubPRsPlugin, which
// does not support forward sync.
type fakeNonCloserPlugin struct{ pluginID string }

func (p *fakeNonCloserPlugin) PluginID() string { return p.pluginID }
func (p *fakeNonCloserPlugin) Fetch(_ context.Context, _ session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	return nil, cursor, nil
}
func (p *fakeNonCloserPlugin) MapToBacklogItem(_ session.ExternalItem, _ string) session.BacklogItemData {
	return session.BacklogItemData{}
}

// newForwardSyncTestStorage creates a temporary ent-backed *session.Storage,
// mirroring the pattern in backlog_service_triage_test.go /
// backlog_item_event_publisher_test.go.
func newForwardSyncTestStorage(t *testing.T) *session.Storage {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("forward-sync-%d.db", time.Now().UnixNano()))

	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// createForwardSyncTestSource creates an ItemSource row and returns its ID.
func createForwardSyncTestSource(t *testing.T, storage *session.Storage, pluginID string, forwardSyncEnabled bool, closeLabel string) string {
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
func createForwardSyncTestItem(t *testing.T, storage *session.Storage, sourceID, externalID string, labels []string) *session.BacklogItemData {
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

// TestForwardSyncSubscriber_ClosesIssueOnDoneTransition_WhenEnabled is the
// integration-level happy path (AC3): a real *events.EventBus delivers a
// BacklogChangeStatusTransition(done) event, and the subscriber calls
// CloseIssue (and the follow-up PostIssueComment) on the source's plugin.
func TestForwardSyncSubscriber_ClosesIssueOnDoneTransition_WhenEnabled(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_closer", true, "shipped")
	item := createForwardSyncTestItem(t, storage, sourceID, "42", []string{"bug"})

	fake := &fakeForwardSyncCloserPlugin{pluginID: "fake_closer", closeUpdatedAt: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	syncLoop := session.NewSyncLoop(storage, registry)

	bus := events.NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartBacklogGitHubForwardSyncSubscriber(ctx, bus, registry, syncLoop, storage)

	bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:      events.BacklogChangeStatusTransition,
		Item:      item,
		OldStatus: "review",
		NewStatus: string(session.BacklogStatusDone),
	}))

	require.Eventually(t, func() bool { return fake.closeCallCount() == 1 }, 2*time.Second, 10*time.Millisecond, "expected CloseIssue to be called")
	call := fake.lastCloseCall()
	require.Equal(t, "42", call.externalID)
	require.Equal(t, []string{"bug"}, call.existingLabels)
	require.Equal(t, "shipped", call.closeLabel)

	require.Eventually(t, func() bool { return fake.commentCallCount() == 1 }, 2*time.Second, 10*time.Millisecond, "expected PostIssueComment to be called")
}

// TestForwardSyncSubscriber_NoOpWhenForwardSyncDisabled is the integration
// guard-path test (AC3): ForwardSyncEnabled=false must result in no GitHub
// call at all, even though the event fires.
func TestForwardSyncSubscriber_NoOpWhenForwardSyncDisabled(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_closer", false, "")
	item := createForwardSyncTestItem(t, storage, sourceID, "42", nil)

	fake := &fakeForwardSyncCloserPlugin{pluginID: "fake_closer"}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	syncLoop := session.NewSyncLoop(storage, registry)

	bus := events.NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartBacklogGitHubForwardSyncSubscriber(ctx, bus, registry, syncLoop, storage)

	bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:      events.BacklogChangeStatusTransition,
		Item:      item,
		NewStatus: string(session.BacklogStatusDone),
	}))

	// Negative assertion: give the subscriber goroutine a window to (wrongly)
	// act, then confirm it never did.
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 0, fake.closeCallCount(), "no GitHub call should be made when ForwardSyncEnabled is false")
}

// TestForwardSyncSubscriber_NoOpWhenPluginDoesNotImplementCloser is the
// interface-pollution guard (AC3): a github_prs-shaped fake plugin (no
// CloseIssue/PostIssueComment) must cause handleForwardSyncClose's type
// assertion to fail cleanly — no panic, no error.
func TestForwardSyncSubscriber_NoOpWhenPluginDoesNotImplementCloser(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_non_closer", true, "")
	item := createForwardSyncTestItem(t, storage, sourceID, "42", nil)

	registry := session.NewPluginRegistry()
	registry.Register(&fakeNonCloserPlugin{pluginID: "fake_non_closer"})
	syncLoop := session.NewSyncLoop(storage, registry)

	require.NotPanics(t, func() {
		handleForwardSyncClose(context.Background(), registry, syncLoop, storage, item)
	})

	// No watermark should have been written — the plugin was never called.
	refreshed, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Nil(t, refreshed.GitHubSyncedIssueUpdatedAt)
}

// TestForwardSyncSubscriber_UsesCloseIssueResponseTimestampForWatermark is the
// regression test for pre-mortem P1 #1: the persisted
// GitHubSyncedIssueUpdatedAt watermark must equal CloseIssue's returned
// timestamp, not the time handleForwardSyncClose happened to run.
func TestForwardSyncSubscriber_UsesCloseIssueResponseTimestampForWatermark(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_closer", true, "")
	item := createForwardSyncTestItem(t, storage, sourceID, "42", nil)

	fixedTimestamp := time.Date(2019, 3, 14, 9, 26, 53, 0, time.UTC)
	fake := &fakeForwardSyncCloserPlugin{pluginID: "fake_closer", closeUpdatedAt: fixedTimestamp}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	syncLoop := session.NewSyncLoop(storage, registry)

	handleForwardSyncClose(context.Background(), registry, syncLoop, storage, item)

	refreshed, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.GitHubSyncedIssueUpdatedAt)
	require.True(t, refreshed.GitHubSyncedIssueUpdatedAt.Equal(fixedTimestamp), "expected watermark %v, got %v (must not be wall-clock time)", fixedTimestamp, refreshed.GitHubSyncedIssueUpdatedAt)
	require.Greater(t, time.Since(*refreshed.GitHubSyncedIssueUpdatedAt), 24*time.Hour, "watermark must be the mocked 2019 timestamp, not wall-clock time")
}

// TestForwardSyncSubscriber_PersistsWatermarkWhenPostCommentFails is the
// regression test for handleForwardSyncClose's comment-is-best-effort
// contract (backlog_github_forward_sync.go's comment above the
// PostIssueComment call): a failed follow-up comment must not prevent the
// loop-prevention watermark from being persisted, since the close itself
// already succeeded by that point. Previously untested — fakeCloserPlugin
// already had commentErr for exactly this, but no test exercised it.
func TestForwardSyncSubscriber_PersistsWatermarkWhenPostCommentFails(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_closer", true, "")
	item := createForwardSyncTestItem(t, storage, sourceID, "42", nil)

	fixedTimestamp := time.Date(2021, 5, 4, 12, 0, 0, 0, time.UTC)
	fake := &fakeForwardSyncCloserPlugin{
		pluginID:       "fake_closer",
		closeUpdatedAt: fixedTimestamp,
		commentErr:     fmt.Errorf("github: comment failed"),
	}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	syncLoop := session.NewSyncLoop(storage, registry)

	handleForwardSyncClose(context.Background(), registry, syncLoop, storage, item)

	require.Equal(t, 1, fake.closeCallCount(), "CloseIssue must still have been called")
	require.Equal(t, 1, fake.commentCallCount(), "PostIssueComment must still have been attempted")

	refreshed, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.GitHubSyncedIssueUpdatedAt, "watermark must be persisted even though the comment failed")
	require.True(t, refreshed.GitHubSyncedIssueUpdatedAt.Equal(fixedTimestamp))
}

// TestForwardSyncSubscriber_NoOpForLocallyCreatedItem verifies the most
// common real-world case has zero-blast-radius coverage: a backlog item with
// no SourceID/ExternalID (the vast majority of items, which were never
// imported from GitHub) must never trigger CloseIssue when it transitions to
// done. This exercises handleForwardSyncClose's
// `current.SourceID == "" || current.ExternalID == ""` early return.
func TestForwardSyncSubscriber_NoOpForLocallyCreatedItem(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)

	// A plain, locally-created backlog item — no source, no external ID.
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:  "locally created item",
		Status: string(session.BacklogStatusDone),
	})
	require.NoError(t, err)

	fake := &fakeForwardSyncCloserPlugin{pluginID: "fake_closer"}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	syncLoop := session.NewSyncLoop(storage, registry)

	require.NotPanics(t, func() {
		handleForwardSyncClose(context.Background(), registry, syncLoop, storage, item)
	})

	require.Equal(t, 0, fake.closeCallCount(), "a locally-created item must never trigger CloseIssue")
	require.Equal(t, 0, fake.commentCallCount())

	refreshed, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Nil(t, refreshed.GitHubSyncedIssueUpdatedAt)
}

// TestForwardSyncSubscriber_RecordsFailureOnCloseError is the regression test
// for pre-mortem P1 #3: a CloseIssue failure must be persisted via
// storage.RecordSourceSyncFailure so it's queryable (Story 4.3.2's row-level
// warning), not just logged.
func TestForwardSyncSubscriber_RecordsFailureOnCloseError(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_closer", true, "")
	item := createForwardSyncTestItem(t, storage, sourceID, "42", nil)

	fake := &fakeForwardSyncCloserPlugin{pluginID: "fake_closer", closeErr: fmt.Errorf("github: rate limited")}
	registry := session.NewPluginRegistry()
	registry.Register(fake)
	syncLoop := session.NewSyncLoop(storage, registry)

	handleForwardSyncClose(context.Background(), registry, syncLoop, storage, item)

	history, truncated, err := storage.ListSourceSyncEvents(context.Background(), sourceID)
	require.NoError(t, err)
	require.False(t, truncated)
	require.Len(t, history, 1)
	require.Contains(t, history[0].ErrorMessage, "rate limited")
	require.Contains(t, history[0].ErrorMessage, item.ID)

	// The watermark must NOT have been advanced on a failed close.
	refreshed, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.Nil(t, refreshed.GitHubSyncedIssueUpdatedAt)
}
