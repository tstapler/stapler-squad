package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
)

// TestInitialPromptFor_should_IncludeWorkspacePeersNudge_When_FlagEnabledAndPeerExists
// covers AC0/AC1 (session-start peer nudge, opt-in via feature flag) via the real
// production choke point: BacklogService.initialPromptFor, called by SpawnSessionFromItem
// to build inst.Prompt.
func TestInitialPromptFor_should_IncludeWorkspacePeersNudge_When_FlagEnabledAndPeerExists(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(workspacePeersNudgeFlagName, true))

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	// A peer session already working in the same repo (path-based key: no GitHub remote).
	peer := &session.Instance{
		Title:   "peer-session",
		UUID:    "peer-uuid",
		Path:    repoPath,
		Branch:  "main",
		Status:  session.Active,
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(peer))
	_, err := storage.SetSessionGoal(t.Context(), "peer-uuid", "refactor the widget loader", session.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, storage.SetSessionGoalWorkspaceKey(t.Context(), "peer-uuid", peer.WorkspaceKey()))

	item := &session.BacklogItemData{
		ID:                 "item-1",
		Title:              "new backlog item",
		Description:        "do the thing",
		AcceptanceCriteria: "[]",
		RepoPath:           repoPath,
	}

	prompt := svc.initialPromptFor(t.Context(), item, nil)

	assert.Contains(t, prompt, "Other Active Sessions In This Workspace")
	assert.Contains(t, prompt, "peer-session")
	assert.Contains(t, prompt, "refactor the widget loader")
}

// TestInitialPromptFor_should_OmitWorkspacePeersNudge_When_FlagDisabledByDefault covers
// AC0: even with a peer present, the nudge must not appear unless the feature flag is
// explicitly enabled.
func TestInitialPromptFor_should_OmitWorkspacePeersNudge_When_FlagDisabledByDefault(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	peer := &session.Instance{
		Title:   "peer-session",
		UUID:    "peer-uuid",
		Path:    repoPath,
		Branch:  "main",
		Status:  session.Active,
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(peer))

	item := &session.BacklogItemData{
		ID:                 "item-1",
		Title:              "new backlog item",
		Description:        "do the thing",
		AcceptanceCriteria: "[]",
		RepoPath:           repoPath,
	}

	prompt := svc.initialPromptFor(t.Context(), item, nil)

	assert.NotContains(t, prompt, "Other Active Sessions In This Workspace")
}

func TestInitialPromptFor_should_OmitWorkspacePeersNudge_When_NoPeersExist(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(workspacePeersNudgeFlagName, true))

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item := &session.BacklogItemData{
		ID:                 "item-1",
		Title:              "new backlog item",
		Description:        "do the thing",
		AcceptanceCriteria: "[]",
		RepoPath:           repoPath,
	}

	prompt := svc.initialPromptFor(t.Context(), item, nil)

	assert.NotContains(t, prompt, "Other Active Sessions In This Workspace")
}

func TestInitialPromptFor_should_OmitWorkspacePeersNudge_When_RepoPathIsEmpty(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(workspacePeersNudgeFlagName, true))

	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item := &session.BacklogItemData{
		ID:                 "item-1",
		Title:              "one-off item",
		Description:        "do the thing",
		AcceptanceCriteria: "[]",
		RepoPath:           "",
	}

	prompt := svc.initialPromptFor(t.Context(), item, nil)

	assert.NotContains(t, prompt, "Other Active Sessions In This Workspace")
}

// TestWorkspacePeersBlockFor_should_ExcludeSessionsOnDifferentRepos guards against a
// regression where every backlog item on the box would see every other item's sessions
// as "peers" (AC1).
func TestWorkspacePeersBlockFor_should_ExcludeSessionsOnDifferentRepos(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(workspacePeersNudgeFlagName, true))

	repoA := t.TempDir()
	initGitRepoWithCommit(t, repoA)
	repoB := t.TempDir()
	initGitRepoWithCommit(t, repoB)

	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	peerOnRepoB := &session.Instance{
		Title: "unrelated-repo-session", UUID: "b-uuid", Path: repoB, Status: session.Active, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(peerOnRepoB))

	block := svc.workspacePeersBlockFor(t.Context(), repoA)
	assert.Empty(t, block)
}
