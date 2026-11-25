package workspacepeers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/instance"
)

// ─── Lifecycle: two independent signals (AC6) ─────────────────────────────────

func TestWorkspacePeer_Lifecycle_distinguishesDeadFromStuck(t *testing.T) {
	t.Parallel()
	dead := WorkspacePeer{InstanceLive: false, StaleGoal: true}
	assert.Equal(t, "gone", dead.Lifecycle())

	stuck := WorkspacePeer{InstanceLive: true, StaleGoal: true}
	assert.Equal(t, "stuck", stuck.Lifecycle())

	active := WorkspacePeer{InstanceLive: true, StaleGoal: false}
	assert.Equal(t, "active", active.Lifecycle())
}

func TestApplyTmuxLiveness_confirmsDeathEvenWhenStatusSaysActive(t *testing.T) {
	t.Parallel()
	peers := []WorkspacePeer{
		{SessionUUID: "live-uuid", InstanceLive: true, Goal: &instance.SessionGoalData{UpdatedAt: time.Now().Add(-time.Hour)}},
		{SessionUUID: "crashed-uuid", InstanceLive: true}, // Status said Active, but tmux disagrees
	}
	ApplyTmuxLiveness(peers, map[string]struct{}{"live-uuid": {}})

	assert.True(t, peers[0].InstanceLive)
	assert.True(t, peers[0].StaleGoal, "goal is an hour old, should be stale")
	assert.False(t, peers[1].InstanceLive, "not in the live set — confirmed dead")
	assert.False(t, peers[1].StaleGoal, "dead sessions aren't 'stuck', they're 'gone'")
}

func TestApplyTmuxLiveness_clearsStaleGoalOfPeerConfirmedDead(t *testing.T) {
	t.Parallel()
	peers := []WorkspacePeer{{SessionUUID: "dead", InstanceLive: true, StaleGoal: true, Goal: &instance.SessionGoalData{UpdatedAt: time.Now().Add(-time.Hour)}}}

	ApplyTmuxLiveness(peers, map[string]struct{}{})

	assert.False(t, peers[0].InstanceLive)
	assert.False(t, peers[0].StaleGoal)
	assert.Equal(t, "gone", peers[0].Lifecycle())
}

func TestApplyTmuxLiveness_staleGoalBoundaryIsExclusive(t *testing.T) {
	t.Parallel()
	// A 1s margin on each side of the threshold, not an exact boundary hit: time.Since is
	// evaluated inside ApplyTmuxLiveness, strictly later than time.Now() here, so testing
	// "elapsed == goalStaleThreshold exactly" is inherently racy against the wall clock.
	const margin = time.Second
	peers := []WorkspacePeer{
		{SessionUUID: "just-under-threshold", InstanceLive: true, Goal: &instance.SessionGoalData{UpdatedAt: time.Now().Add(-goalStaleThreshold + margin)}},
		{SessionUUID: "just-over-threshold", InstanceLive: true, Goal: &instance.SessionGoalData{UpdatedAt: time.Now().Add(-goalStaleThreshold - margin)}},
	}
	live := map[string]struct{}{"just-under-threshold": {}, "just-over-threshold": {}}
	ApplyTmuxLiveness(peers, live)

	assert.False(t, peers[0].StaleGoal, "elapsed just under the threshold is not yet stale")
	assert.True(t, peers[1].StaleGoal, "elapsed just past the threshold is stale")
}

func TestApplyTmuxLiveness_leavesStaleGoalUnchangedForLivePeerWithNoGoal(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	assert.Equal(t, "", BuildWorkspacePeersBlock(nil))
}

func TestBuildWorkspacePeersBlock_includesTitleAndGoal(t *testing.T) {
	t.Parallel()
	peers := []WorkspacePeer{
		{Title: "other-session", Branch: "feature-x", InstanceLive: true, Goal: &instance.SessionGoalData{Goal: "fix the bug"}},
	}
	block := BuildWorkspacePeersBlock(peers)
	assert.Contains(t, block, "other-session")
	assert.Contains(t, block, "feature-x")
	assert.Contains(t, block, "fix the bug")
	assert.Contains(t, block, "list_workspace_peers")
}

func TestBuildWorkspacePeersBlock_capsAtMaxPeers(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
