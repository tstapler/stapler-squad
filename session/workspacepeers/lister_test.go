package workspacepeers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

type fakeStore struct {
	data    []domain.InstanceData
	listErr error
}

func (f fakeStore) ListInstanceData() ([]domain.InstanceData, error) { return f.data, f.listErr }

// GetEntClient returns nil: goal lookups are covered against a real ent schema in package session.
func (fakeStore) GetEntClient() *ent.Client { return nil }

func peerData(uuid, title string, status domain.Status) domain.InstanceData {
	return domain.InstanceData{UUID: uuid, Title: title, Branch: "br-" + uuid, Path: "/repos/widgets", GitHubOwner: "acme", GitHubRepo: "widgets", Status: status}
}

func TestListWorkspacePeers_should_FilterByKeyExcludeAndBlankUUID(t *testing.T) {
	t.Parallel()
	other := peerData("other", "other-repo", domain.Active)
	other.GitHubRepo = "elsewhere"
	l := New(fakeStore{data: []domain.InstanceData{
		peerData("caller", "caller", domain.Active),
		peerData("peer", "peer", domain.Active),
		peerData("", "no-uuid", domain.Active),
		other,
	}})
	key := domain.WorkspaceKey("acme", "widgets", "", "")

	peers, err := l.ListWorkspacePeers(context.Background(), key, "caller")

	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "peer", peers[0].SessionUUID)
	assert.Equal(t, "peer", peers[0].Title)
	assert.Equal(t, "br-peer", peers[0].Branch)
	assert.Equal(t, "/repos/widgets", peers[0].Path)
	assert.Nil(t, peers[0].Goal, "no ent client means no goal rows")
	assert.False(t, peers[0].StaleGoal)
}

func TestListWorkspacePeers_should_MarkStoppedSessionsNotLive(t *testing.T) {
	t.Parallel()
	l := New(fakeStore{data: []domain.InstanceData{peerData("a", "a", domain.Stopped), peerData("b", "b", domain.Paused)}})

	peers, err := l.ListWorkspacePeers(context.Background(), domain.WorkspaceKey("acme", "widgets", "", ""), "")

	require.NoError(t, err)
	require.Len(t, peers, 2)
	assert.False(t, peers[0].InstanceLive)
	assert.True(t, peers[1].InstanceLive)
}

func TestListWorkspacePeers_should_ReturnNothing_When_KeyEmpty(t *testing.T) {
	t.Parallel()
	peers, err := New(fakeStore{data: []domain.InstanceData{peerData("a", "a", domain.Active)}}).ListWorkspacePeers(context.Background(), "", "")
	require.NoError(t, err)
	assert.Nil(t, peers)
}

func TestListWorkspacePeers_should_WrapListError(t *testing.T) {
	t.Parallel()
	_, err := New(fakeStore{listErr: errors.New("db down")}).ListWorkspacePeers(context.Background(), "k", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list instances: db down")
}

func TestSetSessionGoalWorkspaceKey_should_Fail_When_BackendHasNoEntClient(t *testing.T) {
	t.Parallel()
	err := New(fakeStore{}).SetSessionGoalWorkspaceKey(context.Background(), "u", "k")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "goal storage not supported by this backend")
}

func TestBlockForPath_should_RenderPeersSharingThePathWorkspace(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	peer := domain.InstanceData{UUID: "peer", Title: "peer-session", Path: repo, Status: domain.Active}
	l := New(fakeStore{data: []domain.InstanceData{peer}})

	// tmux is not running in the test environment, so every peer reads "gone", which
	// proves ApplyTmuxLiveness ran on the result before rendering.
	block := l.BlockForPath(context.Background(), repo)

	assert.Contains(t, block, "## Other Active Sessions In This Workspace")
	assert.Contains(t, block, "peer-session (gone)")
}

func TestBlockForPath_should_ReturnEmpty_When_NothingToResolve(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var nilLister *Lister
	assert.Equal(t, "", nilLister.BlockForPath(ctx, "/x"))
	assert.Equal(t, "", New(nil).BlockForPath(ctx, "/x"))
	assert.Equal(t, "", New(fakeStore{}).BlockForPath(ctx, ""))
	assert.Equal(t, "", New(fakeStore{}).BlockForPath(ctx, t.TempDir()), "no peers renders nothing")
	assert.Equal(t, "", New(fakeStore{listErr: errors.New("x")}).BlockForPath(ctx, t.TempDir()), "list failures are swallowed")
}
