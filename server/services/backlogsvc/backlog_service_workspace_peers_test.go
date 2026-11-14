package backlogsvc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestInitialPromptFor_should_IncludeWorkspacePeersNudge_When_FlagEnabledAndPeerExists
// covers AC0/AC1 (session-start peer nudge, opt-in via feature flag) via the real
// production choke point: BacklogService.initialPromptFor, called by SpawnSessionFromItem
// to build inst.Prompt.
func TestInitialPromptFor_should_IncludeWorkspacePeersNudge_When_FlagEnabledAndPeerExists(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(services.WorkspacePeersNudgeFlagName, true))

	repoPath := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoPath)

	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	// A peer session already working in the same repo (path-based key: no GitHub remote).
	peer := &instance.Instance{
		Title:   "peer-session",
		UUID:    "peer-uuid",
		Path:    repoPath,
		Branch:  "main",
		Status:  domain.Active,
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(peer))
	_, err := storage.SetSessionGoal(t.Context(), "peer-uuid", "refactor the widget loader", instance.GoalStatusWorking, nil, "", "")
	require.NoError(t, err)
	require.NoError(t, storage.SetSessionGoalWorkspaceKey(t.Context(), "peer-uuid", peer.WorkspaceKey()))

	item := &backlogdto.BacklogItemData{
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
	svctest.WithIsolatedStateDir(t)

	repoPath := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoPath)

	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	peer := &instance.Instance{
		Title:   "peer-session",
		UUID:    "peer-uuid",
		Path:    repoPath,
		Branch:  "main",
		Status:  domain.Active,
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(peer))

	item := &backlogdto.BacklogItemData{
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
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(services.WorkspacePeersNudgeFlagName, true))

	repoPath := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoPath)

	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item := &backlogdto.BacklogItemData{
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
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(services.WorkspacePeersNudgeFlagName, true))

	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item := &backlogdto.BacklogItemData{
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
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(services.WorkspacePeersNudgeFlagName, true))

	repoA := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoA)
	repoB := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoB)

	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	peerOnRepoB := &instance.Instance{
		Title: "unrelated-repo-session", UUID: "b-uuid", Path: repoB, Status: domain.Active, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(peerOnRepoB))

	block := svc.workspacePeersBlockFor(t.Context(), repoA)
	assert.Empty(t, block)
}
