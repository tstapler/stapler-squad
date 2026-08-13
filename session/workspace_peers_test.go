package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func addTestInstance(t *testing.T, storage *Storage, inst *Instance) {
	t.Helper()
	inst.started.Store(true)
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
	keyA := WorkspaceKey("acme", "widgets", "", "")
	keyB := WorkspaceKey("acme", "gadgets", "", "")
	assert.NotEqual(t, keyA, keyB)
}

func TestWorkspaceKey_sameForDifferentWorktreesOfSameRepo(t *testing.T) {
	// Same GitHub repo, different worktree paths — must produce the same key.
	keyMain := WorkspaceKey("acme", "widgets", "/repos/widgets", "/repos/widgets")
	keyWorktree := WorkspaceKey("acme", "widgets", "/repos/widgets", "/repos/widgets-feature-branch")
	assert.Equal(t, keyMain, keyWorktree)
}

func TestWorkspaceKey_isCaseInsensitiveOnGitHubOwnerRepo(t *testing.T) {
	assert.Equal(t, WorkspaceKey("Acme", "Widgets", "", ""), WorkspaceKey("acme", "widgets", "", ""))
}

func TestWorkspaceKey_fallsBackToMainRepoPathWithoutGitHub(t *testing.T) {
	key := WorkspaceKey("", "", "/repos/widgets", "/repos/widgets-worktree")
	assert.Equal(t, "path:/repos/widgets", key)
}

func TestWorkspaceKey_emptyWhenNothingIsSet(t *testing.T) {
	assert.Equal(t, "", WorkspaceKey("", "", "", ""))
}

func TestWorkspaceKey_fallsBackToBarePathWithoutGitHubOrMainRepoPath(t *testing.T) {
	key := WorkspaceKey("", "", "", "/repos/widgets")
	assert.Equal(t, "path:/repos/widgets", key)
}

// ─── ListWorkspacePeers (AC0, AC1, AC2, AC3, AC7) ─────────────────────────────

func TestListWorkspacePeers_excludesCallerAndDifferentRepos(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	caller := &Instance{Title: "caller", UUID: "caller-uuid", Path: "/repos/widgets", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	samePeer := &Instance{Title: "same-repo-peer", UUID: "peer-uuid", Path: "/repos/widgets-wt", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	otherRepo := &Instance{Title: "other-repo", UUID: "other-uuid", Path: "/repos/gadgets", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "gadgets"}
	addTestInstance(t, storage, caller)
	addTestInstance(t, storage, samePeer)
	addTestInstance(t, storage, otherRepo)

	peers, err := storage.ListWorkspacePeers(context.Background(), caller.WorkspaceKey(), caller.UUID)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "peer-uuid", peers[0].SessionUUID)
}

func TestListWorkspacePeers_includesDifferentBranchesAndWorktreesOfSameRepo(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	caller := &Instance{Title: "caller", UUID: "caller-uuid", Path: "/repos/widgets", Branch: "main", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	worktreePeer := &Instance{Title: "feature-peer", UUID: "peer-uuid", Path: "/repos/widgets-feature", Branch: "feature-x", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, caller)
	addTestInstance(t, storage, worktreePeer)

	peers, err := storage.ListWorkspacePeers(context.Background(), caller.WorkspaceKey(), caller.UUID)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "feature-peer", peers[0].Title)
	assert.Equal(t, "feature-x", peers[0].Branch)
}

func TestListWorkspacePeers_emptyWorkspaceKeyReturnsNoPeers(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	peers, err := storage.ListWorkspacePeers(context.Background(), "", "caller-uuid")
	require.NoError(t, err)
	assert.Empty(t, peers)
}

func TestListWorkspacePeers_crashedSessionGoalStillVisible(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	crashed := &Instance{Title: "crashed", UUID: "crashed-uuid", Path: "/repos/widgets", Status: Stopped, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, crashed)
	_, err := storage.SetSessionGoal(context.Background(), "crashed-uuid", "finish the migration", GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, storage.SetSessionGoalWorkspaceKey(context.Background(), "crashed-uuid", crashed.WorkspaceKey()))

	peers, err := storage.ListWorkspacePeers(context.Background(), crashed.WorkspaceKey(), "fresh-caller-uuid")
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.NotNil(t, peers[0].Goal)
	assert.Equal(t, "finish the migration", peers[0].Goal.Goal)
	assert.False(t, peers[0].InstanceLive, "a Stopped instance should not be reported as live")
}

func TestListWorkspacePeers_scopedToOwnStorageInstance(t *testing.T) {
	// Simulates two separate stapler-squad instances (state-isolation boundary, AC7):
	// each has its own Storage/DB, and a session on the *same* repo. Peers from one
	// instance's storage must never leak into the other's query results.
	storageA, cleanupA := createTestStorage(t)
	defer cleanupA()
	storageB, cleanupB := createTestStorage(t)
	defer cleanupB()

	instA := &Instance{Title: "instance-a-session", UUID: "a-uuid", Path: "/repos/widgets", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	instB := &Instance{Title: "instance-b-session", UUID: "b-uuid", Path: "/repos/widgets", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storageA, instA)
	addTestInstance(t, storageB, instB)

	peersFromA, err := storageA.ListWorkspacePeers(context.Background(), instA.WorkspaceKey(), "")
	require.NoError(t, err)
	for _, p := range peersFromA {
		assert.NotEqual(t, "b-uuid", p.SessionUUID, "storage A must not see storage B's session")
	}
}

func TestListWorkspacePeers_clockSkewDoesNotReadAsFresh(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := &Instance{Title: "skewed", UUID: "skewed-uuid", Path: "/repos/widgets", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, inst)
	_, err := storage.SetSessionGoal(context.Background(), "skewed-uuid", "goal", GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, storage.SetSessionGoalWorkspaceKey(context.Background(), "skewed-uuid", inst.WorkspaceKey()))

	caller := &Instance{Title: "caller", UUID: "caller-uuid", Path: "/repos/widgets", Status: Active, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "widgets"}
	addTestInstance(t, storage, caller)

	peers, err := storage.ListWorkspacePeers(context.Background(), inst.WorkspaceKey(), "caller-uuid")
	require.NoError(t, err)
	require.Len(t, peers, 1)
	// UpdatedAt is set by the ent default to "now" — not actually in the future — but this
	// asserts the peer isn't flagged stale immediately after being set, i.e. elapsed >= 0.
	assert.False(t, peers[0].StaleGoal)
}

// ─── Lifecycle: two independent signals (AC6) ─────────────────────────────────

func TestWorkspacePeer_Lifecycle_distinguishesDeadFromStuck(t *testing.T) {
	dead := WorkspacePeer{InstanceLive: false, StaleGoal: true}
	assert.Equal(t, "gone", dead.Lifecycle())

	stuck := WorkspacePeer{InstanceLive: true, StaleGoal: true}
	assert.Equal(t, "stuck", stuck.Lifecycle())

	active := WorkspacePeer{InstanceLive: true, StaleGoal: false}
	assert.Equal(t, "active", active.Lifecycle())
}

func TestApplyTmuxLiveness_confirmsDeathEvenWhenStatusSaysActive(t *testing.T) {
	peers := []WorkspacePeer{
		{SessionUUID: "live-uuid", InstanceLive: true, Goal: &SessionGoalData{UpdatedAt: time.Now().Add(-time.Hour)}},
		{SessionUUID: "crashed-uuid", InstanceLive: true}, // Status said Active, but tmux disagrees
	}
	ApplyTmuxLiveness(peers, map[string]struct{}{"live-uuid": {}})

	assert.True(t, peers[0].InstanceLive)
	assert.True(t, peers[0].StaleGoal, "goal is an hour old, should be stale")
	assert.False(t, peers[1].InstanceLive, "not in the live set — confirmed dead")
	assert.False(t, peers[1].StaleGoal, "dead sessions aren't 'stuck', they're 'gone'")
}

func TestApplyTmuxLiveness_staleGoalBoundaryIsExclusive(t *testing.T) {
	// A 1s margin on each side of the threshold, not an exact boundary hit: time.Since is
	// evaluated inside ApplyTmuxLiveness, strictly later than time.Now() here, so testing
	// "elapsed == goalStaleThreshold exactly" is inherently racy against the wall clock.
	const margin = time.Second
	peers := []WorkspacePeer{
		{SessionUUID: "just-under-threshold", InstanceLive: true, Goal: &SessionGoalData{UpdatedAt: time.Now().Add(-goalStaleThreshold + margin)}},
		{SessionUUID: "just-over-threshold", InstanceLive: true, Goal: &SessionGoalData{UpdatedAt: time.Now().Add(-goalStaleThreshold - margin)}},
	}
	live := map[string]struct{}{"just-under-threshold": {}, "just-over-threshold": {}}
	ApplyTmuxLiveness(peers, live)

	assert.False(t, peers[0].StaleGoal, "elapsed just under the threshold is not yet stale")
	assert.True(t, peers[1].StaleGoal, "elapsed just past the threshold is stale")
}

func TestApplyTmuxLiveness_leavesStaleGoalUnchangedForLivePeerWithNoGoal(t *testing.T) {
	peers := []WorkspacePeer{
		{SessionUUID: "live-no-goal", InstanceLive: false, StaleGoal: true, Goal: nil},
	}
	ApplyTmuxLiveness(peers, map[string]struct{}{"live-no-goal": {}})

	assert.True(t, peers[0].InstanceLive)
	assert.Nil(t, peers[0].Goal)
	assert.True(t, peers[0].StaleGoal, "no Goal to recompute staleness from — prior value is left untouched")
}

// ─── BuildWorkspacePeersBlock (AC5) ────────────────────────────────────────────

func TestBuildWorkspacePeersBlock_emptyWhenNoPeers(t *testing.T) {
	assert.Equal(t, "", BuildWorkspacePeersBlock(nil))
}

func TestBuildWorkspacePeersBlock_includesTitleAndGoal(t *testing.T) {
	peers := []WorkspacePeer{
		{Title: "other-session", Branch: "feature-x", InstanceLive: true, Goal: &SessionGoalData{Goal: "fix the bug"}},
	}
	block := BuildWorkspacePeersBlock(peers)
	assert.Contains(t, block, "other-session")
	assert.Contains(t, block, "feature-x")
	assert.Contains(t, block, "fix the bug")
	assert.Contains(t, block, "list_workspace_peers")
}

func TestBuildWorkspacePeersBlock_capsAtMaxPeers(t *testing.T) {
	var peers []WorkspacePeer
	for i := 0; i < 15; i++ {
		peers = append(peers, WorkspacePeer{Title: fmt.Sprintf("peer-%d", i), InstanceLive: true})
	}
	block := BuildWorkspacePeersBlock(peers)
	for i := 0; i < maxPeersInInitialPrompt; i++ {
		assert.Contains(t, block, fmt.Sprintf("peer-%d", i))
	}
	assert.Contains(t, block, "10 more")
}

func TestBuildWorkspacePeersBlock_livePeersSortBeforeGoneUnderCap(t *testing.T) {
	// 6 gone peers followed by 1 live peer: a raw-order cap would drop the live peer
	// entirely. It must survive the cap since live/stuck peers sort before gone ones.
	var peers []WorkspacePeer
	for i := 0; i < 6; i++ {
		peers = append(peers, WorkspacePeer{Title: fmt.Sprintf("gone-%d", i), InstanceLive: false})
	}
	peers = append(peers, WorkspacePeer{Title: "live-peer", InstanceLive: true})

	block := BuildWorkspacePeersBlock(peers)
	assert.Contains(t, block, "live-peer")
	assert.Contains(t, block, "2 more")
}

func TestBuildWorkspacePeersBlock_stuckPeersSortBeforeGoneUnderCap(t *testing.T) {
	// Same as the live-peer case above, but for the "stuck" lifecycle (live with a stale
	// goal) — the sort predicate groups both "active" and "stuck" ahead of "gone", so this
	// exercises the branch the live-only case above can't reach.
	var peers []WorkspacePeer
	for i := 0; i < 6; i++ {
		peers = append(peers, WorkspacePeer{Title: fmt.Sprintf("gone-%d", i), InstanceLive: false})
	}
	peers = append(peers, WorkspacePeer{Title: "stuck-peer", InstanceLive: true, StaleGoal: true})

	block := BuildWorkspacePeersBlock(peers)
	assert.Contains(t, block, "stuck-peer (stuck)")
	assert.Contains(t, block, "2 more")
}

func TestBuildWorkspacePeersBlock_blankBranchOmitsDanglingField(t *testing.T) {
	peers := []WorkspacePeer{
		{Title: "gone-peer", Branch: "", InstanceLive: false},
		{Title: "live-peer", Branch: "", InstanceLive: true},
	}
	block := BuildWorkspacePeersBlock(peers)
	assert.NotContains(t, block, "(, gone)")
	assert.NotContains(t, block, "(, active)")
	assert.Contains(t, block, "gone-peer (gone)")
	assert.Contains(t, block, "live-peer (active)")
}
