package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// addArchivedInstanceLive persists a session directly into fixture storage AND registers
// it with the live review-queue poller (reloading first, mirroring addPausedSession in
// session_service_test.go), so DeleteSession's live-instance branch runs and actually
// calls Destroy() -> CleanupWorktree() instead of just killing tmux by name.
func addArchivedInstanceLive(t *testing.T, fix *forkTestFixture, title string, archivedAt time.Time, opts ...func(*session.Instance)) {
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

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err, "addArchivedInstanceLive: failed to reload after persist")
	for _, li := range loaded {
		if li.Title == title {
			addInstanceToPoller(fix.poller, li)
			return
		}
	}
	t.Fatalf("addArchivedInstanceLive: could not find %q after reload", title)
}

func TestSessionRetentionSweeper_DeletesArchivedSessionPastRetention(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addArchivedInstance(t, fix, "old-archived", time.Now().AddDate(0, 0, -20))

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.False(t, hasStoredTitle(t, fix, "old-archived"),
		"expected archived session past the retention window to be deleted")
}

func TestSessionRetentionSweeper_SkipsRecentlyArchivedSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addArchivedInstance(t, fix, "recent-archived", time.Now().AddDate(0, 0, -1))

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "recent-archived"),
		"expected recently archived session (inside the retention window) to be retained")
}

func TestSessionRetentionSweeper_SkipsOpenPR(t *testing.T) {
	t.Parallel()
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

// TestSessionRetentionSweeper_DeletesTerminalPRAfterRestart is the regression test for
// the session-retention-cleanup fix: GitHubPRStatusTerminal must round-trip through
// storage (session/ent_repository.go's sessionToInstanceData / create+update paths),
// not just live in memory, or every archived PR-linked session would be permanently
// stuck behind SkipsOpenPR's check on every restart regardless of how old or merged the
// PR actually is.
func TestSessionRetentionSweeper_DeletesTerminalPRAfterRestart(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addArchivedInstance(t, fix, "terminal-pr-archived", time.Now().AddDate(0, 0, -20), func(inst *session.Instance) {
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
	t.Parallel()
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
	t.Parallel()
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

// TestSessionRetentionSweeper_ConvergesWhenAllSiblingsBecomeEligible is a regression test
// for the group-convergence fix (PR #303 review): a naive "skip if ANY sibling still
// references this worktree path" check never converges for an item reopened 2+ times,
// because every round shares the identical path (see findExistingWorktreeForBranch) --
// each round would always find some OTHER round still referencing it and block forever,
// even once the whole group is independently archived, past retention, clean, and
// PR-terminal. This test builds exactly that group (3 sibling rounds sharing one real
// git worktree, all independently eligible) and asserts the sweep eventually reclaims all
// of them -- both the DB rows and the physical worktree directory -- across however many
// sweep cycles it actually takes (see the comment on the assert.Eventually call below for
// why a stronger single-pass guarantee isn't the right thing to assert here).
func TestSessionRetentionSweeper_ConvergesWhenAllSiblingsBecomeEligible(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	mainRepoDir := t.TempDir()
	runGit(t, mainRepoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(mainRepoDir, "committed.txt"), []byte("a"), 0o644))
	runGit(t, mainRepoDir, "add", ".")
	runGit(t, mainRepoDir, "commit", "-q", "-m", "initial")
	headSHA := strings.TrimSpace(runGitTestCmd(t, mainRepoDir, "rev-parse", "HEAD"))

	// A single real linked worktree, shared by every round below -- mirroring
	// findExistingWorktreeForBranch's reuse-by-branch-name behavior: reopen/rework
	// never creates a second worktree, it reuses this exact directory.
	worktreeDir := filepath.Join(t.TempDir(), "shared-worktree")
	runGit(t, mainRepoDir, "worktree", "add", "-q", "-b", "shared-branch", worktreeDir, "HEAD")

	ctx := context.Background()
	item, err := fix.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "converged item",
		RepoPath: mainRepoDir,
		// The backlog item itself has also reached a terminal state -- consistent
		// with every round being done, though the sweep does not read item status
		// directly (it derives group eligibility purely from sibling session data).
		Status: string(session.BacklogStatusDone),
	})
	require.NoError(t, err)

	roundTitles := []string{"round-1", "round-2", "round-3"}
	roundUUIDs := []string{"round-1-uuid", "round-2-uuid", "round-3-uuid"}
	for i, title := range roundTitles {
		_, err := fix.storage.CreateItemSession(ctx, session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: roundUUIDs[i],
			SessionRole: session.SessionRoleWork,
		})
		require.NoError(t, err)

		// Stagger ArchivedAt so they're independently, not just coincidentally, past
		// retention -- all well past the 14-day default.
		archivedAt := time.Now().AddDate(0, 0, -(20 + i))
		addArchivedInstanceLive(t, fix, title, archivedAt, func(inst *session.Instance) {
			inst.UUID = roundUUIDs[i]
			wt := git.NewGitWorktreeFromStorage(mainRepoDir, worktreeDir, title, "shared-branch", headSHA)
			inst.SetGitWorktree(wt)
		})
	}

	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)

	// DeleteSession's own worktree cleanup (Destroy()) runs in a fire-and-forget
	// goroutine, so once the whole group becomes eligible, more than one sibling can be
	// dispatched for deletion within the same sweep() call while they all point at the
	// identical physical directory -- one sibling's in-flight `git worktree remove` can
	// transiently make another sibling's own dirty-check error out and get skipped for
	// that cycle (a safe, conservative outcome, not a bug: the worktree it was guarding
	// is gone moments later, so it converges on the very next tick). Re-invoking sweep()
	// here models exactly that next tick, rather than asserting a stronger "always
	// single-pass" guarantee this fix doesn't need to make.
	// Inlined rather than calling hasStoredTitle(t, ...): that helper's
	// require.NoError is safe at every OTHER call site in this file (all
	// synchronous, single-shot assertions) but not here -- assert.Eventually
	// runs this func on its own ticker goroutine, and does not wait for an
	// in-flight tick to return before giving up at the deadline. If a tick's
	// sweep(ctx) (real worktree/git + DB work) is still running when the 5s
	// ceiling passes, Eventually returns and the test finishes, t.Cleanup
	// closes fix.storage's DB, and the still-running stale tick then hits
	// "sql: database is closed" -- require.NoError on a *testing.T that has
	// already completed panics with "Fail in goroutine after test has
	// completed" instead of just being one more false-y tick (confirmed:
	// this exact panic, this exact test, session_service_fork_test.go's
	// forkTestFixture doc comment already names it as a known recurrence).
	// A query error here just means "not converged yet" -- return false and
	// let the next tick (or the timeout) decide, never fail the test from
	// this goroutine.
	assert.Eventually(t, func() bool {
		sweeper.sweep(ctx)
		data, err := fix.storage.ListInstanceData()
		if err != nil {
			return false
		}
		for _, d := range data {
			for _, title := range roundTitles {
				if d.Title == title {
					return false
				}
			}
		}
		_, statErr := os.Stat(worktreeDir)
		return os.IsNotExist(statErr)
	}, 15*time.Second, 100*time.Millisecond,
		"expected every sibling round's DB row and the shared worktree directory to eventually be reclaimed once the whole group became independently eligible")
}
