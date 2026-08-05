package session

// backlog_lifecycle_superseded_test.go — regression tests for BUG-047's
// collateral damage: closeIfSupersededByMain closing a real, unmerged PR
// because ItemSession.LastCommitSha held the session's spawn-time *base* SHA
// rather than any commit the session actually authored.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git"
)

// supersededFixture builds the exact live shape of the incident: a repo whose
// main branch is at mainSHA, plus an unmerged work branch carrying one real
// commit (featureSHA) that the agent authored after spawning. The repo is left
// checked out on the work branch, so the session's worktree HEAD is featureSHA
// while its spawn-time base is mainSHA.
func supersededFixture(t *testing.T) (repoPath, mainSHA, featureSHA, branch string) {
	t.Helper()
	repoPath, mainSHA = setupBounceMainRepo(t)
	branch = "backlog/stapler-squad-fix-idle-reviewer-wedge"
	runGitTestCmd(t, repoPath, "checkout", "-b", branch)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "fix.txt"), []byte("the real, reviewed, unmerged fix\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "fix.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "fix(backlog): idle reviewer wedge")
	featureSHA = strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))
	return repoPath, mainSHA, featureSHA, branch
}

// supersededItem creates a pr_pending item with an open PR and one work
// session seeded exactly the way production spawn seeding leaves it, then
// returns the item and its work session.
func supersededItem(t *testing.T, storage *Storage, repoPath, baseSHA string, prNumber int) (*BacklogItemData, ItemSessionSummary) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "bug: reviewer session that submits a verdict but never exits wedges the item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)

	prURL := "https://github.com/tstapler/stapler-squad/pull/342"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "bug047-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// What real spawn seeding writes (SpawnSessionFromItem step 12b): the
	// pre-work base SHA into BaseCommitSha. LastCommitSha is additionally
	// seeded with the same value here to reproduce every pre-fix row already
	// sitting in production databases — the fix must hold for those too, not
	// only for rows written after it lands.
	require.NoError(t, storage.SetItemSessionBaseCommit(ctx, workIS.ID, baseSHA))
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, workIS.ID, baseSHA, "", time.Now(), 0))

	return item, workIS
}

// attachWorktree registers a worktree row for the work session so
// resolveLatestWorkCommit can find the session's live HEAD.
func attachWorktree(t *testing.T, storage *Storage, repoPath, branch, headSHA string) {
	t.Helper()
	inst := newTestInstance("bug047-instance")
	inst.UUID = "bug047-work-session"
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "bug047-instance", branch, headSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
}

func failingCIChecker() *fakePRPendingChecker {
	return &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	}
}

// TestReconcilePRPending_should_NotCloseUnmergedPRAsSuperseded_When_LastCommitShaIsOnlyTheSpawnTimeBase
// is the direct regression test for BUG-047's live incident on 2026-08-05.
//
// ItemSession.LastCommitSha was written exactly once — at session spawn, with
// the worktree's pre-work HEAD — and never refreshed as the agent committed.
// A session's own base commit is by construction already an ancestor of main,
// so git.IsCommitOnMain on it is unconditionally true. closeIfSupersededByMain
// read that field, concluded the item's work had "already shipped through
// another path", closed PR #342 (a real, reviewed, CI-green fix) WITHOUT
// merging it, and marked the backlog item done. The fix never shipped, and two
// further items that depended on it stayed wedged.
//
// The assertion that matters is the negative one: an item whose session has
// authored real, unmerged work must not be closed out just because no commit
// has landed since spawn. It must instead follow the normal CI-failing path
// and spawn a fix session.
func TestReconcilePRPending_should_NotCloseUnmergedPRAsSuperseded_When_LastCommitShaIsOnlyTheSpawnTimeBase(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoPath, mainSHA, featureSHA, branch := supersededFixture(t)
	item, _ := supersededItem(t, storage, repoPath, mainSHA, 342)
	attachWorktree(t, storage, repoPath, branch, featureSHA)

	listener := NewBacklogLifecycleListener(storage)
	checker := failingCIChecker()
	overridePRPendingChecker(t, listener, checker)
	spawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(spawner)

	listener.ReconcilePRPending(ctx, storage.repo.(*EntRepository))

	assert.False(t, checker.closeCalled,
		"PR #342 carries real unmerged work; it must never be closed as superseded on the strength of the session's own base commit")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status,
		"the item must stay in pr_pending, not be marked done against a base SHA it never authored")
	assert.Equal(t, 342, fetched.PrNumber, "the PR reference must not be cleared")
	assert.True(t, spawner.spawnCalled,
		"with the bogus superseded exit closed off, a CI-failing PR must follow the normal fix-spawn path")
}

// TestReconcilePRPending_should_NotCloseUnmergedPRAsSuperseded_When_SessionHeadCannotBeResolved
// covers the belt-and-braces half of the fix. resolveLatestWorkCommit can
// legitimately fail — the worktree directory is gone and the branch ref no
// longer resolves — in which case closeIfSupersededByMain falls back to the
// stored LastCommitSha. Without an explicit guard, that fallback walks straight
// back into the original bug for every pre-fix row still in the database. The
// explicit BaseCommitSha comparison is what makes the fallback safe.
func TestReconcilePRPending_should_NotCloseUnmergedPRAsSuperseded_When_SessionHeadCannotBeResolved(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoPath, mainSHA, _, _ := supersededFixture(t)
	// Deliberately no worktree row — resolveLatestWorkCommit returns "" and the
	// stored (base-seeded) LastCommitSha is all that remains.
	item, _ := supersededItem(t, storage, repoPath, mainSHA, 342)

	listener := NewBacklogLifecycleListener(storage)
	checker := failingCIChecker()
	overridePRPendingChecker(t, listener, checker)
	listener.SetPRFixSpawner(&fakePRFixSpawner{})

	listener.ReconcilePRPending(ctx, storage.repo.(*EntRepository))

	assert.False(t, checker.closeCalled,
		"an unresolvable HEAD must not license closing the PR against the session's base commit")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status,
		"fail closed: when the session's real tip is unknown, the item stays put")
}

// TestReconcilePRPending_should_StillCloseSupersededPR_When_SessionsRealTipIsOnMain
// is the positive control for the two tests above: the BUG-032 behaviour they
// constrain must still fire when the session's *actual authored* commit —
// not its base — is confirmed on main. Without this, "never close anything"
// would pass the regression tests while silently deleting the feature.
func TestReconcilePRPending_should_StillCloseSupersededPR_When_SessionsRealTipIsOnMain(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoPath, mainSHA, featureSHA, branch := supersededFixture(t)
	// The work genuinely shipped through another path: main now contains the
	// session's own tip commit.
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--ff-only", branch)
	runGitTestCmd(t, repoPath, "checkout", branch)

	item, _ := supersededItem(t, storage, repoPath, mainSHA, 342)
	attachWorktree(t, storage, repoPath, branch, featureSHA)

	listener := NewBacklogLifecycleListener(storage)
	checker := failingCIChecker()
	overridePRPendingChecker(t, listener, checker)
	spawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(spawner)

	listener.ReconcilePRPending(ctx, storage.repo.(*EntRepository))

	assert.True(t, checker.closeCalled,
		"a PR whose session's real tip commit is already on main is genuinely superseded and must still be closed")
	assert.Contains(t, checker.closeComment, featureSHA,
		"the close comment must cite the commit actually verified on main, not the session's base SHA")
	assert.False(t, spawner.spawnCalled, "a genuinely superseded PR must not also spawn a fix cycle")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status)
}

// TestRefreshWorkSessionGitActivity_should_ReplaceSpawnTimeBaseWithSessionsRealTip
// covers the other half of the fix: the field itself. LastCommitSha only ever
// held the spawn-time base because nothing refreshed it; this sweep — wired
// into the existing reconciliation tick — is what makes the field's name true.
func TestRefreshWorkSessionGitActivity_should_ReplaceSpawnTimeBaseWithSessionsRealTip(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoPath, mainSHA, featureSHA, branch := supersededFixture(t)
	item, workIS := supersededItem(t, storage, repoPath, mainSHA, 342)
	attachWorktree(t, storage, repoPath, branch, featureSHA)

	before, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Equal(t, mainSHA, before[0].LastCommitSha, "precondition: the row starts base-seeded, as production leaves it")

	NewBacklogLifecycleListener(storage).refreshWorkSessionGitActivity(ctx)

	after, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, featureSHA, after[0].LastCommitSha,
		"LastCommitSha must hold the session's real tip after a refresh tick")
	assert.Equal(t, mainSHA, after[0].BaseCommitSha,
		"BaseCommitSha must keep the spawn-time baseline the review-gate diff depends on")
	assert.Equal(t, "fix(backlog): idle reviewer wedge", after[0].LastCommitMessage)
	assert.Equal(t, 1, after[0].CommitCountSinceSpawn,
		"one commit authored since the base")
	assert.Equal(t, workIS.ID, after[0].ID)
}

// TestUpdateItemSessionGitActivity_should_RecordProgressAtObservationTime_When_CommitIsBackdated
// guards a regression the live-refresh sweep would otherwise have introduced.
//
// last_progress_at drives reconcileStaleWorkSessions' staleness clock. Git author
// timestamps survive rebases and cherry-picks, and this repo rebases session
// worktrees onto main routinely — so recording the *commit's author date* as
// progress would push that clock backwards whenever a session rebased an older
// commit, falsely flagging a healthy, actively-committing session as stale_work
// and handing it to automated remediation. Progress is recorded when it is
// observed; last_commit_at keeps the true author time for display.
func TestUpdateItemSessionGitActivity_should_RecordProgressAtObservationTime_When_CommitIsBackdated(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "item whose session rebased a backdated commit",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "backdated-commit-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// A commit authored 3 days ago — well past the staleness threshold — but
	// observed by the refresh sweep right now.
	authoredAt := time.Now().Add(-72 * time.Hour)
	before := time.Now()
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, workIS.ID, "deadbeef", "rebased work", authoredAt, 1))

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	require.NotNil(t, sessions[0].LastCommitAt)
	assert.WithinDuration(t, authoredAt, *sessions[0].LastCommitAt, time.Second,
		"last_commit_at must preserve the commit's real author time for display")

	require.NotNil(t, sessions[0].LastProgressAt)
	assert.False(t, sessions[0].LastProgressAt.Before(before.Add(-time.Second)),
		"last_progress_at must be observation time, not the backdated author time — otherwise a rebase instantly marks a healthy session stale")
	assert.True(t, staleWork(authoredAt, time.Now()),
		"precondition: the author date really is old enough to trip the staleness threshold, so this test would fail if the author date leaked through")
	assert.False(t, staleWork(*sessions[0].LastProgressAt, time.Now()),
		"the session must not be considered stale after just reporting a commit")
}
