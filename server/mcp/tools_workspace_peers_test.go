package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// TestListWorkspacePeersMCP_returnsOtherSessionsOnSameRepo covers AC0: the MCP tool
// returns other active sessions sharing the caller's workspace, excluding the caller.
func TestListWorkspacePeersMCP_returnsOtherSessionsOnSameRepo(t *testing.T) {
	callerUUID := "caller-uuid"
	caller := makeTestInstance("caller-session", callerUUID)
	caller.GitHubOwner, caller.GitHubRepo = "acme", "widgets"
	peer := makeTestInstance("peer-session", "peer-uuid")
	peer.GitHubOwner, peer.GitHubRepo = "acme", "widgets"
	unrelated := makeTestInstance("unrelated-session", "other-uuid")
	unrelated.GitHubOwner, unrelated.GitHubRepo = "acme", "other-repo"

	store := &stubStore{instances: []*session.Instance{caller, peer, unrelated}}
	h, _ := makeGoalHandlers(t, store)
	// Storage.ListWorkspacePeers queries its own real ent-backed ListInstanceData, so the
	// same instances need to be persisted there too (mirrors how caller resolution uses
	// `store` while goal/peer data lives in `storage` throughout this file).
	require.NoError(t, h.storage.AddInstance(caller))
	require.NoError(t, h.storage.AddInstance(peer))
	require.NoError(t, h.storage.AddInstance(unrelated))
	_, err := h.storage.SetSessionGoal(context.Background(), "peer-uuid", "fix the bug", session.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, h.storage.SetSessionGoalWorkspaceKey(context.Background(), "peer-uuid", peer.WorkspaceKey()))

	ctx := WithSessionUUID(context.Background(), callerUUID)
	result, err := h.listWorkspacePeers(ctx, makeToolReq(nil))
	require.NoError(t, err)

	m := parseResult(t, result)
	assert.True(t, m["success"].(bool))
	peers, ok := m["peers"].([]interface{})
	require.True(t, ok)
	require.Len(t, peers, 1)
	peerMap := peers[0].(map[string]interface{})
	assert.Equal(t, "peer-session", peerMap["title"])
	assert.Equal(t, "fix the bug", peerMap["goal_text"])
	// No real tmux server is running in this test process, so LiveTmuxSessionUUIDs
	// authoritatively reports every peer as not-live — documents current (tmux-absent)
	// test-environment behavior for the two-signal lifecycle fields this tool exists to
	// expose (AC6), rather than leaving them unasserted.
	assert.Equal(t, "gone", peerMap["lifecycle"])
	assert.False(t, peerMap["instance_live"].(bool))
}

func TestListWorkspacePeersMCP_returnsEmptyWhenCallerHasNoWorkspace(t *testing.T) {
	callerUUID := "caller-uuid"
	caller := makeTestInstance("caller-session", callerUUID)
	// No GitHubOwner/Repo/Path set beyond the default "/tmp/test" from makeTestInstance,
	// which does yield a path-based key — clear it to simulate a truly bare session.
	caller.Path = ""

	store := &stubStore{instances: []*session.Instance{caller}}
	h, _ := makeGoalHandlers(t, store)

	ctx := WithSessionUUID(context.Background(), callerUUID)
	result, err := h.listWorkspacePeers(ctx, makeToolReq(nil))
	require.NoError(t, err)

	m := parseResult(t, result)
	assert.True(t, m["success"].(bool))
	peers, ok := m["peers"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, peers)
}

func TestListWorkspacePeersMCP_errorsWithoutCallerSessionUUID(t *testing.T) {
	store := &stubStore{}
	h, _ := makeGoalHandlers(t, store)

	result, err := h.listWorkspacePeers(context.Background(), makeToolReq(nil))
	require.NoError(t, err)

	m := parseResult(t, result)
	assert.False(t, m["success"].(bool))
}
