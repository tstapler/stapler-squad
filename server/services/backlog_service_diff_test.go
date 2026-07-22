package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// attachWorkSessionWithRange is attachWorkSessionWithCommit but takes an explicit
// baseSHA distinct from the final commit, so tests can assert on the actual diff
// content (base..sha) rather than a degenerate empty range.
func attachWorkSessionWithRange(t *testing.T, storage *session.Storage, repo *session.EntRepository, itemID, sessionUUID, repoPath, worktreePath, branchName, baseSHA, sha string) {
	t.Helper()
	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateItemSessionGitActivity(t.Context(), is.ID, sha, "work commit", time.Now(), 1))

	now := time.Now()
	require.NoError(t, repo.Create(t.Context(), session.InstanceData{
		Title:      sessionUUID,
		UUID:       sessionUUID,
		Path:       worktreePath,
		WorkingDir: worktreePath,
		Branch:     branchName,
		Status:     session.Paused,
		Program:    "claude",
		CreatedAt:  now,
		UpdatedAt:  now,
		Worktree: session.GitWorktreeData{
			RepoPath:      repoPath,
			WorktreePath:  worktreePath,
			SessionName:   sessionUUID,
			BranchName:    branchName,
			BaseCommitSHA: baseSHA,
		},
	}))
}

// TestGetBacklogItemDiff_should_ComputeCorrectDiff_When_WorktreeDirectoryNoLongerExists
// is the regression test for the bug this fix closes: once a work session's worktree
// directory has been cleaned up (the normal state for a done item), the diff must
// still reflect what actually shipped — not an empty/wrong diff computed against
// whatever happens to be checked out in item.RepoPath.
func TestGetBacklogItemDiff_should_ComputeCorrectDiff_When_WorktreeDirectoryNoLongerExists(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	baseSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "shipped.txt"), []byte("shipped content\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "shipped.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "add shipped.txt")
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--no-ff", "--no-edit", "feature")

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "done item, worktree cleaned up",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)

	// deletedWorktreePath is never actually created on disk — simulating the
	// normal post-done cleanup where the directory is long gone, while the DB
	// row (BaseCommitSHA, WorktreePath) still remembers it existed.
	deletedWorktreePath := filepath.Join(t.TempDir(), "deleted-worktree")
	attachWorkSessionWithRange(t, storage, repo, item.ID, "cleaned-up-work", repoPath, deletedWorktreePath, "feature", baseSHA, sha)

	resp, err := svc.GetBacklogItemDiff(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemDiffRequest{ItemId: item.ID}))
	require.NoError(t, err, "must not fail just because the worktree directory no longer exists")
	assert.Contains(t, resp.Msg.Diff, "shipped content", "diff must reflect what the branch actually shipped, not an empty/wrong range")
	assert.Equal(t, int32(1), resp.Msg.Added)
	assert.Equal(t, int32(0), resp.Msg.Removed)
}

// TestGetBacklogItemDiff_should_ComputeCorrectDiff_When_WorktreeStillExists is the
// baseline: the live-worktree case (item still in progress/review) must keep working
// exactly as before this fix.
func TestGetBacklogItemDiff_should_ComputeCorrectDiff_When_WorktreeStillExists(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	baseSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "wip.txt"), []byte("work in progress\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "wip.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "wip commit")
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item still in progress",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	attachWorkSessionWithRange(t, storage, repo, item.ID, "live-work", repoPath, repoPath, "feature", baseSHA, sha)

	resp, err := svc.GetBacklogItemDiff(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemDiffRequest{ItemId: item.ID}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.Diff, "work in progress")
	assert.Equal(t, int32(1), resp.Msg.Added)
}

// TestGetBacklogItemDiff_should_ShowRealChanges_When_LastCommitShaNeverRefreshed is
// the regression test for the "View Changes modal is misleading once work is
// committed" UX bug: ItemSession.LastCommitSha is written exactly once, at session
// spawn, to the PRE-work base commit (AttachSessionToItem/SpawnSessionFromItem step
// 12b) — nothing in production ever refreshes it as the agent makes further commits.
// So in the real, common case LastCommitSha == the worktree's own BaseCommitSHA,
// which previously made GetBacklogItemDiff compute an empty base..base range and
// show "No changes to display" for a Review-status item that already has a full
// Gate Verdict against real diff content. The fix prefers the branch's actual tip
// (wt.BranchName) over the stale LastCommitSha.
func TestGetBacklogItemDiff_should_ShowRealChanges_When_LastCommitShaNeverRefreshed(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	baseSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "reviewed.txt"), []byte("already reviewed content\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "reviewed.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "add reviewed.txt")
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--no-ff", "--no-edit", "feature")

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "review item with a real Gate Verdict",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	deletedWorktreePath := filepath.Join(t.TempDir(), "deleted-worktree")
	// LastCommitSha == baseSHA, exactly as production leaves it: never refreshed
	// past the pre-work base commit captured at spawn time.
	attachWorkSessionWithRange(t, storage, repo, item.ID, "stale-last-commit-sha", repoPath, deletedWorktreePath, "feature", baseSHA, baseSHA)

	resp, err := svc.GetBacklogItemDiff(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemDiffRequest{ItemId: item.ID}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.Diff, "already reviewed content", "must show the branch's real committed diff, not an empty base..base range")
	assert.Equal(t, int32(1), resp.Msg.Added)
}

// TestGetBacklogItemDiff_should_ShowEarlierSessionWork_When_LatestReworkSessionMadeNoCommits
// is the regression test for the second, still-live cause of the same empty-diff
// symptom PR #179 only partially fixed: an item's branch can carry more than one work
// session (rework/reopen cycles). The RPC previously derived diffBaseSHA from the
// MOST RECENT work session's own BaseCommitSHA. When that latest session is a reopen
// that makes zero new commits (e.g. it just re-verifies already-complete work and
// exits), its BaseCommitSHA was captured at spawn time — which by definition already
// equals the current branch tip — so base == head and the diff comes back empty, even
// though an earlier work session on the same branch committed substantial real,
// unreviewed work. This mirrors the live repro on item
// de6d7878-9d6e-4081-acfa-02ff545c87b4 ("Add Queued State To Backlog Items"): session 1
// did the real implementation, session 2 was a reopen that made no new commits.
func TestGetBacklogItemDiff_should_ShowEarlierSessionWork_When_LatestReworkSessionMadeNoCommits(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	baseSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	// Session 1: the real implementation, committed to the branch.
	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "implemented.txt"), []byte("real implementation content\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "implemented.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "implement feature")
	branchTipSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--no-ff", "--no-edit", "feature")

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "reopened item, rework session made no commits",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	// Session 1 (earlier, created first): real work, base -> branchTipSHA.
	session1Path := filepath.Join(t.TempDir(), "session-1-worktree")
	attachWorkSessionWithRange(t, storage, repo, item.ID, "work-session-1", repoPath, session1Path, "feature", baseSHA, branchTipSHA)

	// Ensure session 2's CreatedAt is strictly later than session 1's.
	time.Sleep(2 * time.Millisecond)

	// Session 2 (later, a reopen/rework session): made zero new commits. Its
	// BaseCommitSHA was captured at spawn time, which is already the branch
	// tip left by session 1 — base == head from session 2's own perspective.
	session2Path := filepath.Join(t.TempDir(), "session-2-worktree")
	attachWorkSessionWithRange(t, storage, repo, item.ID, "work-session-2-reopen", repoPath, session2Path, "feature", branchTipSHA, branchTipSHA)

	resp, err := svc.GetBacklogItemDiff(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemDiffRequest{ItemId: item.ID}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.Diff, "real implementation content", "diff must still reflect the earlier session's real committed work, not be empty just because the latest rework session made no commits")
	assert.Equal(t, int32(1), resp.Msg.Added)
	assert.Equal(t, int32(0), resp.Msg.Removed)
}
