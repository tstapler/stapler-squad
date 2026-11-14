package backlogsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func newItemSessionForStamp(t *testing.T, repoPath string) (*BacklogService, *backlogdto.BacklogItemData, backlogdto.ItemSessionSummary) {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	itemID := createReadyItemForSpawn(t, svc, repoPath, "stamp item")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	is, err := storage.CreateItemSession(t.Context(), backlogdto.ItemSessionData{
		ItemID: itemID, SessionUUID: "00000000-0000-0000-0000-000000000001", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	return svc, item, is
}

func TestStampBaseCommit_should_RecordHeadSHA_When_DirIsGitRepo(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repo)
	svc, item, is := newItemSessionForStamp(t, repo)

	got := svc.stampBaseCommit(t.Context(), baseCommitStamp{Caller: "test", Item: item, Session: is, Dir: repo, RaisesNotification: true})

	assert.Regexp(t, `^[0-9a-f]{40}$`, got)
	sessions, err := svc.storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, got, sessions[0].BaseCommitSha)
}

// A HEAD read failure used to be dropped on the floor, leaving the session with no base
// and the PR-ownership guard with nothing to compare against.
func TestStampBaseCommit_should_NotifyOperator_When_WorkSessionHeadUnreadable(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repo)
	svc, item, is := newItemSessionForStamp(t, repo)
	bus := events.NewEventBus(10)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())

	got := svc.stampBaseCommit(t.Context(), baseCommitStamp{Caller: "test", Item: item, Session: is, Dir: t.TempDir(), RaisesNotification: true})

	assert.Empty(t, got)
	assert.Equal(t, []string{"Work session has no base commit"}, drainNotificationTitles(ch))
	sessions, err := svc.storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Empty(t, sessions[0].BaseCommitSha)
}

func TestStampBaseCommit_should_OnlyLog_When_SessionDoesNotGuardPR(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repo)
	svc, item, is := newItemSessionForStamp(t, repo)
	bus := events.NewEventBus(10)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())

	got := svc.stampBaseCommit(t.Context(), baseCommitStamp{Caller: "test", Item: item, Session: is, Dir: t.TempDir()})

	assert.Empty(t, got)
	assert.Empty(t, drainNotificationTitles(ch))
}

// A git checkout go-git cannot open must not be demoted to a directory session: that
// session has no branch or base commit, which blinds the PR-ownership guard.
func TestResolveSessionPath_should_FailHard_When_DotGitPresentButUnopenable(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))

	_, useWorktree, err := resolveSessionPath(repo, "slug", "")

	require.Error(t, err)
	assert.False(t, useWorktree)
}
