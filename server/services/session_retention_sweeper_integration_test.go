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
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/sessionretention"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/instance"
)

// These tests drive the real SessionService.DeleteSession (tmux/worktree/DB cleanup) through
// the sweeper; the sweeper's own decision logic is unit-tested in package sessionretention
// against a fake deleter.

// addArchivedInstance persists a session directly into fixture storage with the given
// ArchivedAt timestamp, applying any additional field mutations via opts.
func addArchivedInstance(t *testing.T, fix *forkTestFixture, title string, archivedAt time.Time, opts ...func(*instance.Instance)) {
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
func addArchivedInstanceLive(t *testing.T, fix *forkTestFixture, title string, archivedAt time.Time, opts ...func(*instance.Instance)) {
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

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err, "addArchivedInstanceLive: failed to reload after persist")
	for _, li := range loaded {
		if li.Title == title {
			svctest.AddInstanceToPoller(fix.poller, li)
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

	sweeper := sessionretention.NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.SweepOnce(context.Background())

	assert.False(t, hasStoredTitle(t, fix, "old-archived"),
		"expected archived session past the retention window to be deleted")
}

func TestSessionRetentionSweeper_DeletesCleanWorktreeSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	repoDir := t.TempDir()
	svctest.RunGit(t, repoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("a"), 0o644))
	svctest.RunGit(t, repoDir, "add", ".")
	svctest.RunGit(t, repoDir, "commit", "-q", "-m", "initial")

	addArchivedInstance(t, fix, "clean-worktree-archived", time.Now().AddDate(0, 0, -20), func(inst *instance.Instance) {
		wt := git.NewGitWorktreeFromStorage(repoDir, repoDir, "clean-worktree-archived", "test-branch", "0000000000000000000000000000000000000000")
		inst.SetGitWorktree(wt)
	})

	sweeper := sessionretention.NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	sweeper.SweepOnce(context.Background())

	assert.False(t, hasStoredTitle(t, fix, "clean-worktree-archived"),
		"expected session with a clean worktree to be deleted after passing the retention window")
}

// TestSessionRetentionSweeper_ConvergesWhenAllSiblingsBecomeEligible is a regression test
// for the group-convergence fix (PR #303 review): a naive "skip if ANY sibling still
// references this worktree path" check never converges for an item reopened 2+ times,
// because every round shares the identical path (see nativeFindExistingWorktreeForBranch) --
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
	svctest.RunGit(t, mainRepoDir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(mainRepoDir, "committed.txt"), []byte("a"), 0o644))
	svctest.RunGit(t, mainRepoDir, "add", ".")
	svctest.RunGit(t, mainRepoDir, "commit", "-q", "-m", "initial")
	headSHA := strings.TrimSpace(svctest.RunGitTestCmd(t, mainRepoDir, "rev-parse", "HEAD"))

	// A single real linked worktree, shared by every round below -- mirroring
	// nativeFindExistingWorktreeForBranch's reuse-by-branch-name behavior: reopen/rework
	// never creates a second worktree, it reuses this exact directory.
	worktreeDir := filepath.Join(t.TempDir(), "shared-worktree")
	svctest.RunGit(t, mainRepoDir, "worktree", "add", "-q", "-b", "shared-branch", worktreeDir, "HEAD")

	ctx := context.Background()
	item, err := fix.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:    "converged item",
		RepoPath: mainRepoDir,
		// The backlog item itself has also reached a terminal state -- consistent
		// with every round being done, though the sweep does not read item status
		// directly (it derives group eligibility purely from sibling session data).
		Status: string(domain.BacklogStatusDone),
	})
	require.NoError(t, err)

	roundTitles := []string{"round-1", "round-2", "round-3"}
	roundUUIDs := []string{"round-1-uuid", "round-2-uuid", "round-3-uuid"}
	for i, title := range roundTitles {
		_, err := fix.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: roundUUIDs[i],
			SessionRole: instance.SessionRoleWork,
		})
		require.NoError(t, err)

		// Stagger ArchivedAt so they're independently, not just coincidentally, past
		// retention -- all well past the 14-day default.
		archivedAt := time.Now().AddDate(0, 0, -(20 + i))
		addArchivedInstanceLive(t, fix, title, archivedAt, func(inst *instance.Instance) {
			inst.UUID = roundUUIDs[i]
			wt := git.NewGitWorktreeFromStorage(mainRepoDir, worktreeDir, title, "shared-branch", headSHA)
			inst.SetGitWorktree(wt)
		})
	}

	sweeper := sessionretention.NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)

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
		sweeper.SweepOnce(ctx)
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
