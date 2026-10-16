package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// fakeStateUpdaterPlugin stands in for LinearPlugin: implements
// externalIssueStateUpdater (UpdateIssueState/PostIssueComment).
type fakeStateUpdaterPlugin struct {
	pluginID  string
	updateErr error
	updatedAt time.Time

	mu       sync.Mutex
	targets  []string
	comments []string
}

func (p *fakeStateUpdaterPlugin) PluginID() string { return p.pluginID }
func (p *fakeStateUpdaterPlugin) Fetch(_ context.Context, _ session.PluginConfig, c string) ([]session.ExternalItem, string, error) {
	return nil, c, nil
}
func (p *fakeStateUpdaterPlugin) MapToBacklogItem(_ session.ExternalItem, _ string) session.BacklogItemData {
	return session.BacklogItemData{}
}
func (p *fakeStateUpdaterPlugin) UpdateIssueState(_ context.Context, _ session.PluginConfig, _ string, target string) (time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.targets = append(p.targets, target)
	return p.updatedAt, p.updateErr
}
func (p *fakeStateUpdaterPlugin) PostIssueComment(_ context.Context, _ session.PluginConfig, _ string, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.comments = append(p.comments, body)
	return nil
}

func TestForwardSync_should_UpdateStatePostCommentAndWriteWatermark_ForStateUpdaterPlugin(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_linear", true, "In Review")
	item := createForwardSyncTestItem(t, storage, sourceID, "ENG-1", nil)
	ts := time.Date(2019, 3, 14, 9, 26, 53, 0, time.UTC)
	fake := &fakeStateUpdaterPlugin{pluginID: "fake_linear", updatedAt: ts}
	registry := session.NewPluginRegistry()
	registry.Register(fake)

	handleForwardSyncClose(context.Background(), registry, session.NewSyncLoop(storage, registry), storage, item)

	require.Equal(t, []string{"In Review"}, fake.targets)
	require.Equal(t, []string{fmt.Sprintf(forwardSyncStateComment, "In Review")}, fake.comments, "visible comment must accompany the state change")
	got, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	require.NotNil(t, got.GitHubSyncedIssueUpdatedAt)
	require.True(t, got.GitHubSyncedIssueUpdatedAt.Equal(ts))
}

func TestForwardSync_should_RecordFailureAndSkipComment_When_UpdateIssueStateFails(t *testing.T) {
	t.Parallel()
	storage := newForwardSyncTestStorage(t)
	sourceID := createForwardSyncTestSource(t, storage, "fake_linear", true, "Shipped")
	item := createForwardSyncTestItem(t, storage, sourceID, "ENG-1", nil)
	fake := &fakeStateUpdaterPlugin{pluginID: "fake_linear", updateErr: fmt.Errorf(`no workflow state named "Shipped" found`)}
	registry := session.NewPluginRegistry()
	registry.Register(fake)

	handleForwardSyncClose(context.Background(), registry, session.NewSyncLoop(storage, registry), storage, item)

	require.Empty(t, fake.comments)
	history, _, err := storage.ListSourceSyncEvents(context.Background(), sourceID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Contains(t, history[0].ErrorMessage, "forward-sync state update failed")
}
