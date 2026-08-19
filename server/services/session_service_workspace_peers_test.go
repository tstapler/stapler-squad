package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// addTestPeer creates a peer session sharing repoPath's workspace, with a goal set, for
// use by the workspace-peers-nudge tests below.
func addTestPeer(t *testing.T, storage *session.Storage, repoPath string) {
	t.Helper()
	peer := &session.Instance{
		Title: "peer-session", UUID: "peer-uuid", Path: repoPath, Branch: "main",
		Status: session.Active, Program: "claude", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(peer))
	_, err := storage.SetSessionGoal(t.Context(), "peer-uuid", "fix the widget loader", session.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, storage.SetSessionGoalWorkspaceKey(t.Context(), "peer-uuid", peer.WorkspaceKey()))
}

// TestSessionServiceWorkspacePeersBlockFor_should_IncludePeerNudge_When_FlagEnabledAndPeerExists
// covers AC0/AC1 for the regular CreateSession RPC path (as opposed to BacklogService's
// initialPromptFor, covered separately in backlog_service_workspace_peers_test.go): the nudge
// is opt-in via the workspacePeersNudgeFlagName feature flag.
func TestSessionServiceWorkspacePeersBlockFor_should_IncludePeerNudge_When_FlagEnabledAndPeerExists(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(workspacePeersNudgeFlagName, true))

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))
	t.Cleanup(func() { svc.Shutdown() })

	addTestPeer(t, storage, repoPath)

	block := svc.workspacePeersBlockFor(t.Context(), repoPath)

	assert.Contains(t, block, "Other Active Sessions In This Workspace")
	assert.Contains(t, block, "peer-session")
	assert.Contains(t, block, "fix the widget loader")
}

// TestSessionServiceWorkspacePeersBlockFor_should_OmitPeerNudge_When_FlagDisabledByDefault
// covers AC0: the nudge must not appear by default even when a peer exists, since the flag
// defaults to off/unset.
func TestSessionServiceWorkspacePeersBlockFor_should_OmitPeerNudge_When_FlagDisabledByDefault(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))
	t.Cleanup(func() { svc.Shutdown() })

	addTestPeer(t, storage, repoPath)

	assert.Empty(t, svc.workspacePeersBlockFor(t.Context(), repoPath))
}

func TestSessionServiceWorkspacePeersBlockFor_should_ReturnEmpty_When_NoPeersExist(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(workspacePeersNudgeFlagName, true))

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))
	t.Cleanup(func() { svc.Shutdown() })

	assert.Empty(t, svc.workspacePeersBlockFor(t.Context(), repoPath))
}

func TestSessionServiceWorkspacePeersBlockFor_should_ReturnEmpty_When_RepoPathIsEmpty(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))
	t.Cleanup(func() { svc.Shutdown() })

	assert.Empty(t, svc.workspacePeersBlockFor(t.Context(), ""))
}
