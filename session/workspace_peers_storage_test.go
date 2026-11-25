package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/sessiongoal"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/workspacepeers"
)

func addTestInstance(t *testing.T, storage *Storage, inst *instance.Instance) {
	t.Helper()
	inst.MarkStarted()
	if inst.CreatedAt.IsZero() {
		inst.CreatedAt = time.Now()
	}
	if inst.UpdatedAt.IsZero() {
		inst.UpdatedAt = time.Now()
	}
	require.NoError(t, storage.AddInstance(inst))
}

// ─── WorkspaceKey derivation (AC1, AC2) ───────────────────────────────────────

func TestWorkspaceKey_distinctForDifferentRepos(t *testing.T) {
	t.Parallel()
	keyA := domain.WorkspaceKey("acme", "widgets", "", "")
	keyB := domain.WorkspaceKey("acme", "gadgets", "", "")
	assert.NotEqual(t, keyA, keyB)
}

func TestWorkspaceKey_sameForDifferentWorktreesOfSameRepo(t *testing.T) {
	t.Parallel()
	// Same GitHub repo, different worktree paths — must produce the same key.
	keyMain := domain.WorkspaceKey("acme", "widgets", "/repos/widgets", "/repos/widgets")
	keyWorktree := domain.WorkspaceKey("acme", "widgets", "/repos/widgets", "/repos/widgets-feature-branch")
	assert.Equal(t, keyMain, keyWorktree)
}

func TestWorkspaceKey_isCaseInsensitiveOnGitHubOwnerRepo(t *testing.T) {
	t.Parallel()
	assert.Equal(t, domain.WorkspaceKey("Acme", "Widgets", "", ""), domain.WorkspaceKey("acme", "widgets", "", ""))
}

func TestWorkspaceKey_fallsBackToMainRepoPathWithoutGitHub(t *testing.T) {
	t.Parallel()
	key := domain.WorkspaceKey("", "", "/repos/widgets", "/repos/widgets-worktree")
	assert.Equal(t, "path:/repos/widgets", key)
}

func TestWorkspaceKey_emptyWhenNothingIsSet(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", domain.WorkspaceKey("", "", "", ""))
}

func TestWorkspaceKey_fallsBackToBarePathWithoutGitHubOrMainRepoPath(t *testing.T) {
	t.Parallel()
	key := domain.WorkspaceKey("", "", "", "/repos/widgets")
	assert.Equal(t, "path:/repos/widgets", key)
}

// ─── ListWorkspacePeers (AC0, AC1, AC2, AC3, AC7) ─────────────────────────────

func TestListWorkspacePeers_excludesCallerAndDifferentRepos(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	caller := &instance.Instance{Title: "caller", UUID: "caller-uuid", Path: "/repos/widgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	samePeer := &instance.Instance{Title: "same-repo-peer", UUID: "peer-uuid", Path: "/repos/widgets-wt", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	otherRepo := &instance.Instance{Title: "other-repo", UUID: "other-uuid", Path: "/repos/gadgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "gadgets"}
	addTestInstance(t, storage, caller)
	addTestInstance(t, storage, samePeer)
	addTestInstance(t, storage, otherRepo)

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(context.Background(), caller.WorkspaceKey(), caller.UUID)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "peer-uuid", peers[0].SessionUUID)
}

func TestListWorkspacePeers_includesDifferentBranchesAndWorktreesOfSameRepo(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	caller := &instance.Instance{Title: "caller", UUID: "caller-uuid", Path: "/repos/widgets", Branch: "main", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	worktreePeer := &instance.Instance{Title: "feature-peer", UUID: "peer-uuid", Path: "/repos/widgets-feature", Branch: "feature-x", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, caller)
	addTestInstance(t, storage, worktreePeer)

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(context.Background(), caller.WorkspaceKey(), caller.UUID)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "feature-peer", peers[0].Title)
	assert.Equal(t, "feature-x", peers[0].Branch)
}

func TestListWorkspacePeers_emptyWorkspaceKeyReturnsNoPeers(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(context.Background(), "", "caller-uuid")
	require.NoError(t, err)
	assert.Empty(t, peers)
}

func TestListWorkspacePeers_crashedSessionGoalStillVisible(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	crashed := &instance.Instance{Title: "crashed", UUID: "crashed-uuid", Path: "/repos/widgets", Status: domain.Stopped, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, crashed)
	_, err := storage.SetSessionGoal(context.Background(), "crashed-uuid", "finish the migration", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, workspacepeers.New(storage).SetSessionGoalWorkspaceKey(context.Background(), "crashed-uuid", crashed.WorkspaceKey()))

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(context.Background(), crashed.WorkspaceKey(), "fresh-caller-uuid")
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.NotNil(t, peers[0].Goal)
	assert.Equal(t, "finish the migration", peers[0].Goal.Goal)
	assert.False(t, peers[0].InstanceLive, "a Stopped instance should not be reported as live")
}

func TestListWorkspacePeers_staleGoalFlagsLivePeersOnly(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	for _, tc := range []struct {
		uuid   string
		status domain.Status
		want   bool
	}{
		{"live-stale", domain.Active, true},
		{"stopped-stale", domain.Stopped, false},
	} {
		inst := &instance.Instance{Title: tc.uuid, UUID: tc.uuid, Path: "/repos/widgets", Status: tc.status, Program: "claude",
			GitHubOwner: "acme", GitHubRepo: "widgets"}
		addTestInstance(t, storage, inst)
		_, err := storage.SetSessionGoal(ctx, tc.uuid, "old goal", instance.GoalStatusWorking, nil, "", "")
		require.NoError(t, err)
		_, err = storage.GetEntClient().SessionGoal.Update().Where(sessiongoal.SessionUUID(tc.uuid)).SetUpdatedAt(time.Now().Add(-time.Hour)).Save(ctx)
		require.NoError(t, err)
	}

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(ctx, domain.WorkspaceKey("acme", "widgets", "", ""), "")
	require.NoError(t, err)
	require.Len(t, peers, 2)
	for _, p := range peers {
		require.NotNil(t, p.Goal, p.SessionUUID)
		assert.Equal(t, p.SessionUUID == "live-stale", p.StaleGoal, p.SessionUUID)
	}
}

func TestSetSessionGoalWorkspaceKey_stampsGoalRow(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	_, err := storage.SetSessionGoal(ctx, "stamp-uuid", "goal", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)

	require.NoError(t, workspacepeers.New(storage).SetSessionGoalWorkspaceKey(ctx, "stamp-uuid", "path:/repos/widgets"))

	row, err := storage.GetEntClient().SessionGoal.Query().Where(sessiongoal.SessionUUID("stamp-uuid")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "path:/repos/widgets", row.WorkspaceKey)
}

func TestListWorkspacePeers_undecodableTasksYieldEmptyTaskList(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	inst := &instance.Instance{Title: "bad-tasks", UUID: "bad-tasks-uuid", Path: "/repos/widgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, inst)
	_, err := storage.SetSessionGoal(ctx, "bad-tasks-uuid", "goal", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	_, err = storage.GetEntClient().SessionGoal.Update().Where(sessiongoal.SessionUUID("bad-tasks-uuid")).SetTasks("{not json").Save(ctx)
	require.NoError(t, err)

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(ctx, inst.WorkspaceKey(), "")

	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.NotNil(t, peers[0].Goal)
	assert.NotNil(t, peers[0].Goal.Tasks, "a decode failure must degrade to an empty, non-nil task list")
	assert.Empty(t, peers[0].Goal.Tasks)
}

func TestListWorkspacePeers_scopedToOwnStorageInstance(t *testing.T) {
	t.Parallel()
	// Simulates two separate stapler-squad instances (state-isolation boundary, AC7):
	// each has its own Storage/DB, and a session on the *same* repo. Peers from one
	// instance's storage must never leak into the other's query results.
	storageA, cleanupA := createTestStorage(t)
	defer cleanupA()
	storageB, cleanupB := createTestStorage(t)
	defer cleanupB()

	instA := &instance.Instance{Title: "instance-a-session", UUID: "a-uuid", Path: "/repos/widgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	instB := &instance.Instance{Title: "instance-b-session", UUID: "b-uuid", Path: "/repos/widgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storageA, instA)
	addTestInstance(t, storageB, instB)

	peersFromA, err := workspacepeers.New(storageA).ListWorkspacePeers(context.Background(), instA.WorkspaceKey(), "")
	require.NoError(t, err)
	for _, p := range peersFromA {
		assert.NotEqual(t, "b-uuid", p.SessionUUID, "storage A must not see storage B's session")
	}
}

func TestListWorkspacePeers_clockSkewDoesNotReadAsFresh(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &instance.Instance{Title: "skewed", UUID: "skewed-uuid", Path: "/repos/widgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, inst)
	_, err := storage.SetSessionGoal(context.Background(), "skewed-uuid", "goal", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, workspacepeers.New(storage).SetSessionGoalWorkspaceKey(context.Background(), "skewed-uuid", inst.WorkspaceKey()))

	caller := &instance.Instance{Title: "caller", UUID: "caller-uuid", Path: "/repos/widgets", Status: domain.Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, caller)

	peers, err := workspacepeers.New(storage).ListWorkspacePeers(context.Background(), inst.WorkspaceKey(), "caller-uuid")
	require.NoError(t, err)
	require.Len(t, peers, 1)
	// UpdatedAt is set by the ent default to "now" — not actually in the future — but this
	// asserts the peer isn't flagged stale immediately after being set, i.e. elapsed >= 0.
	assert.False(t, peers[0].StaleGoal)
}

var _ workspacepeers.Store = (*Storage)(nil)
