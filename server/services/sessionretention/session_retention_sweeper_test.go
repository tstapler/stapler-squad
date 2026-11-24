package sessionretention

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
)

// addArchivedInstance persists a session directly into fixture storage with the given
// ArchivedAt timestamp, applying any additional field mutations via opts.
func addArchivedInstance(t *testing.T, fix *testFixture, title string, archivedAt time.Time, opts ...func(*instance.Instance)) {
	t.Helper()
	inst := &instance.Instance{
		Title:     title,
		Path:      "/tmp/test",
		Status:    domain.Stopped,
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
func hasStoredTitle(t *testing.T, fix *testFixture, title string) bool {
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

func TestSessionRetentionSweeper_SkipsRecentlyArchivedSession(t *testing.T) {
	t.Parallel()
	fix := newFixture(t)

	addArchivedInstance(t, fix, "recent-archived", time.Now().AddDate(0, 0, -1))

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "recent-archived"),
		"expected recently archived session (inside the retention window) to be retained")
}

func TestSessionRetentionSweeper_SkipsOpenPR(t *testing.T) {
	t.Parallel()
	fix := newFixture(t)

	addArchivedInstance(t, fix, "open-pr-archived", time.Now().AddDate(0, 0, -20), func(inst *instance.Instance) {
		inst.GitHubPRNumber = 42
		inst.GitHubPRStatusTerminal = false
	})

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "open-pr-archived"),
		"expected session with an open/unmerged PR to be retained despite passing the retention window")
}

// TestSessionRetentionSweeper_DeletesTerminalPRAfterRestart is the regression test for
// the session-retention-cleanup fix: GitHubPRStatusTerminal must round-trip through
// storage (session/instancestore's sessionToInstanceData and session/instancestore create.go and update.go create+update paths),
// not just live in memory, or every archived PR-linked session would be permanently
// stuck behind SkipsOpenPR's check on every restart regardless of how old or merged the
// PR actually is.
func TestSessionRetentionSweeper_DeletesTerminalPRAfterRestart(t *testing.T) {
	t.Parallel()
	fix := newFixture(t)

	addArchivedInstance(t, fix, "terminal-pr-archived", time.Now().AddDate(0, 0, -20), func(inst *instance.Instance) {
		inst.GitHubPRNumber = 42
		inst.GitHubPRStatusTerminal = true
	})

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.False(t, hasStoredTitle(t, fix, "terminal-pr-archived"),
		"expected session with a merged/closed PR (loaded fresh from storage) to be deleted once past the retention window")
}

func TestSessionRetentionSweeper_SkipsDirtyWorktree(t *testing.T) {
	t.Parallel()
	fix := newFixture(t)

	repoDir := t.TempDir()
	svctest.RunGit(t, repoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("a"), 0o644))
	svctest.RunGit(t, repoDir, "add", ".")
	svctest.RunGit(t, repoDir, "commit", "-q", "-m", "initial")
	// Leave an uncommitted change so IsDirty() reports true.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("uncommitted"), 0o644))

	addArchivedInstance(t, fix, "dirty-worktree-archived", time.Now().AddDate(0, 0, -20), func(inst *instance.Instance) {
		wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "dirty-worktree-archived", "test-branch", "0000000000000000000000000000000000000000")
		inst.SetGitWorktree(wt)
	})

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "dirty-worktree-archived"),
		"expected session with uncommitted worktree changes to be retained despite passing the retention window")
}

// TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound is a regression test
// for the shared-worktree guard: backlog rework/reopen reuses the same branch (and
// therefore the same worktree directory) across rounds, so an old archived round's
// session can point at the exact directory a newer, still-active round's session is
// using. The sweep must not delete that worktree out from under the sibling.
func TestSessionRetentionSweeper_SkipsWorktreeSharedWithSiblingRound(t *testing.T) {
	t.Parallel()
	fix := newFixture(t)

	repoDir := t.TempDir()
	svctest.RunGit(t, repoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("a"), 0o644))
	svctest.RunGit(t, repoDir, "add", ".")
	svctest.RunGit(t, repoDir, "commit", "-q", "-m", "initial")

	ctx := context.Background()
	item, err := fix.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:    "shared-worktree item",
		RepoPath: repoDir,
		Status:   string(domain.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	const sha = "0000000000000000000000000000000000000000"

	// Old round: archived long ago, past retention, otherwise safe to delete.
	oldUUID := "old-round-uuid"
	_, err = fix.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: oldUUID,
		SessionRole: instance.SessionRoleWork,
	})
	require.NoError(t, err)
	addArchivedInstance(t, fix, "old-round", time.Now().AddDate(0, 0, -20), func(inst *instance.Instance) {
		inst.UUID = oldUUID
		wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "old-round", "test-branch", sha)
		inst.SetGitWorktree(wt)
	})

	// New round: still active (never archived), sharing the exact same worktree path —
	// e.g. a rework/reopen that reused the branch per nativeFindExistingWorktreeForBranch.
	newUUID := "new-round-uuid"
	_, err = fix.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: newUUID,
		SessionRole: instance.SessionRoleWork,
	})
	require.NoError(t, err)
	newInst := &instance.Instance{
		Title:     "new-round",
		UUID:      newUUID,
		Path:      repoDir,
		Status:    domain.Active,
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
