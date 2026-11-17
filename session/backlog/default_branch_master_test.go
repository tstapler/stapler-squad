package backlog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Regression: a master-default repo (e.g. kibitzer) made every shipped-without-a-PR
// check fail with "reference not found" because "main" was hard-coded. A squash
// merge (work tip not an ancestor of the default branch) is a separate, unsolved case.
func TestMostRecentWorkCommitShippedToMain_should_DetectShipped_When_DefaultBranchIsMaster(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")
	runGitTestCmd(t, dir, "branch", "-M", "master")
	sha := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:    "master-default repo item",
		Status:   string(domain.BacklogStatusInProgress),
		RepoPath: dir,
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID: item.ID, SessionUUID: "master-work-session", SessionRole: instance.SessionRoleWork,
	})
	require.NoError(t, err)
	inst := newTestInstance("master-work-instance")
	inst.UUID = "master-work-session"
	inst.GitManager().Worktree = git.NewGitWorktreeFromStorage(dir, dir, "master-work-instance", "master", sha)
	require.NoError(t, storage.SaveInstances([]*instance.Instance{inst}))

	listener := NewBacklogLifecycleListener(storage)
	gotSHA, shipped := listener.mostRecentWorkCommitShippedToMain(ctx, item.ID, dir)

	assert.Equal(t, sha, gotSHA)
	assert.True(t, shipped, "a commit on master must read as shipped in a master-default repo")
}
