package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestWorkspacePeersBlockFor_should_IncludePeerNudge_When_AnotherSessionSharesTheRepo covers
// AC5 for the regular CreateSession RPC path (as opposed to BacklogService's initialPromptFor,
// covered separately in backlog_service_workspace_peers_test.go).
func TestSessionServiceWorkspacePeersBlockFor_should_IncludePeerNudge_When_AnotherSessionSharesTheRepo(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	peer := &session.Instance{
		Title: "peer-session", UUID: "peer-uuid", Path: repoPath, Branch: "main",
		Status: session.Active, Program: "claude", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(peer))
	_, err := storage.SetSessionGoal(t.Context(), "peer-uuid", "fix the widget loader", session.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, storage.SetSessionGoalWorkspaceKey(t.Context(), "peer-uuid", peer.WorkspaceKey()))

	block := svc.workspacePeersBlockFor(t.Context(), repoPath)

	assert.Contains(t, block, "Other Active Sessions In This Workspace")
	assert.Contains(t, block, "peer-session")
	assert.Contains(t, block, "fix the widget loader")
}

func TestSessionServiceWorkspacePeersBlockFor_should_ReturnEmpty_When_NoPeersExist(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	assert.Empty(t, svc.workspacePeersBlockFor(t.Context(), repoPath))
}

func TestSessionServiceWorkspacePeersBlockFor_should_ReturnEmpty_When_RepoPathIsEmpty(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(10))

	assert.Empty(t, svc.workspacePeersBlockFor(t.Context(), ""))
}
