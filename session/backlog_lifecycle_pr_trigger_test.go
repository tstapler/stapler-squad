package session

import (
	"context"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepoWithRemote creates a real (empty) git repository at a fresh temp
// directory with an "origin" remote pointing at remoteURL, so
// github.GetOwnerRepoFromRemote(dir) resolves exactly as it would against a
// real worktree. Each call uses a unique t.TempDir() so github.GetRemoteURL's
// process-wide remoteURLCache (keyed by repoPath) never sees a stale entry
// from a different test.
func newTestRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}})
	require.NoError(t, err)
	return dir
}

func TestFindPRPendingItemForEvent_should_ReturnItemAndTrue_When_PrNumberAndRepoMatch(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	got, found := findPRPendingItemForEvent(ctx, storage.repo, "tstapler/stapler-squad", 189)

	require.True(t, found)
	assert.Equal(t, item.ID, got.ID.String())
}

func TestFindPRPendingItemForEvent_should_ReturnNilFalse_When_PrNumberMatchesButRepoDiffers(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/someone-else/unrelated-fork.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/someone-else/unrelated-fork/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	_, found := findPRPendingItemForEvent(ctx, storage.repo, "tstapler/stapler-squad", 189)

	assert.False(t, found, "PR-number collision across two tracked repos must not match on number alone")
}

func TestFindPRPendingItemForEvent_should_ReturnNilFalse_When_ZeroPrPendingItemsExist(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	_, found := findPRPendingItemForEvent(context.Background(), storage.repo, "tstapler/stapler-squad", 189)

	assert.False(t, found)
}

func TestTriggerPRFixForEvent_should_ReconcileItemAndReturnMatchedTrue_When_ItemFound(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/trigger-pr-fix-event", "")

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(listener, "backlog/trigger-pr-fix-event")

	matched, err := listener.TriggerPRFixForEvent(ctx, "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.True(t, matched)

	refreshed, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), refreshed.Status, "reconcilePRPendingItem should have run for the matched item (merged PR -> done)")
}

func TestTriggerPRFixForEvent_should_ReturnFalseNilWithoutQuerying_When_ListenerDisabled(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage) // enabled defaults to false

	matched, err := listener.TriggerPRFixForEvent(ctx, "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.False(t, matched)

	refreshed, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), refreshed.Status, "disabled listener must not touch the item")
}

func TestTriggerPRFixForEvent_should_ReturnFalseNil_When_NoPrPendingItemMatches(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	matched, err := listener.TriggerPRFixForEvent(context.Background(), "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.False(t, matched)
}
