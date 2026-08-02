package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// addArchivedInstance persists a session directly into fixture storage with the given
// ArchivedAt timestamp, applying any additional field mutations via opts.
func addArchivedInstance(t *testing.T, fix *forkTestFixture, title string, archivedAt time.Time, opts ...func(*session.Instance)) {
	t.Helper()
	inst := &session.Instance{
		Title:     title,
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	inst.ArchivedAt = &archivedAt
	for _, opt := range opts {
		opt(inst)
	}
	require.NoError(t, fix.storage.AddInstance(inst))
}

// hasStoredTitle reports whether a session with the given title still exists in storage.
func hasStoredTitle(t *testing.T, fix *forkTestFixture, title string) bool {
	t.Helper()
	data, err := fix.storage.ListInstanceData()
	require.NoError(t, err)
	for _, d := range data {
		if d.Title == title {
			return true
		}
	}
	return false
}

func TestSessionRetentionSweeper_DeletesArchivedSessionPastRetention(t *testing.T) {
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addArchivedInstance(t, fix, "old-archived", time.Now().AddDate(0, 0, -20))

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.False(t, hasStoredTitle(t, fix, "old-archived"),
		"expected archived session past the retention window to be deleted")
}

func TestSessionRetentionSweeper_SkipsRecentlyArchivedSession(t *testing.T) {
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addArchivedInstance(t, fix, "recent-archived", time.Now().AddDate(0, 0, -1))

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "recent-archived"),
		"expected recently archived session (inside the retention window) to be retained")
}

func TestSessionRetentionSweeper_SkipsOpenPR(t *testing.T) {
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addArchivedInstance(t, fix, "open-pr-archived", time.Now().AddDate(0, 0, -20), func(inst *session.Instance) {
		inst.GitHubPRNumber = 42
		inst.GitHubPRStatusTerminal = false
	})

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "open-pr-archived"),
		"expected session with an open/unmerged PR to be retained despite passing the retention window")
}

func TestSessionRetentionSweeper_SkipsDirtyWorktree(t *testing.T) {
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("a"), 0o644))
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-q", "-m", "initial")
	// Leave an uncommitted change so IsDirty() reports true.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("uncommitted"), 0o644))

	addArchivedInstance(t, fix, "dirty-worktree-archived", time.Now().AddDate(0, 0, -20), func(inst *session.Instance) {
		wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "dirty-worktree-archived", "test-branch", "0000000000000000000000000000000000000000")
		inst.SetGitWorktree(wt)
	})

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "dirty-worktree-archived"),
		"expected session with uncommitted worktree changes to be retained despite passing the retention window")
}

func TestSessionRetentionSweeper_DeletesCleanWorktreeSession(t *testing.T) {
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("a"), 0o644))
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-q", "-m", "initial")

	addArchivedInstance(t, fix, "clean-worktree-archived", time.Now().AddDate(0, 0, -20), func(inst *session.Instance) {
		wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "clean-worktree-archived", "test-branch", "0000000000000000000000000000000000000000")
		inst.SetGitWorktree(wt)
	})

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.False(t, hasStoredTitle(t, fix, "clean-worktree-archived"),
		"expected session with a clean worktree to be deleted after passing the retention window")
}

// TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound is a regression test
// for the shared-worktree guard: backlog rework/reopen reuses the same branch (and
// therefore the same worktree directory) across rounds, so an old archived round's
// session can point at the exact directory a newer, still-active round's session is
// using. The sweep must not delete that worktree out from under the sibling.
func TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound(t *testing.T) {
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("a"), 0o644))
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-q", "-m", "initial")

	ctx := context.Background()
	item, err := fix.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "shared-worktree item",
		RepoPath: repoDir,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	const sha = "0000000000000000000000000000000000000000"

	// Old round: archived long ago, past retention, otherwise safe to delete.
	oldUUID := "old-round-uuid"
	_, err = fix.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: oldUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	addArchivedInstance(t, fix, "old-round", time.Now().AddDate(0, 0, -20), func(inst *session.Instance) {
		inst.UUID = oldUUID
		wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "old-round", "test-branch", sha)
		inst.SetGitWorktree(wt)
	})

	// New round: still active (never archived), sharing the exact same worktree path —
	// e.g. a rework/reopen that reused the branch per findExistingWorktreeForBranch.
	newUUID := "new-round-uuid"
	_, err = fix.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: newUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	newInst := &session.Instance{
		Title:     "new-round",
		UUID:      newUUID,
		Path:      repoDir,
		Status:    session.Active,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "new-round", "test-branch", sha)
	newInst.SetGitWorktree(wt)
	require.NoError(t, fix.storage.AddInstance(newInst))

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(ctx)

	assert.True(t, hasStoredTitle(t, fix, "old-round"),
		"expected old archived round to be retained because its worktree is shared with an active sibling round")
}
