package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/git"
)

// newTrackedWorkSession creates a real ItemSession (SessionRoleWork) backed
// by a genuine Session+Worktree row (via SaveInstances, mirroring
// newOrphanedAgentPRTestItem's identical recipe) so
// GetWorktreeDataBySessionUUID resolves branchName as itemID's tracked
// branch — the fixture shape Story 6's verifyPRHeadBranchMatchesTracked
// guard needs to see a non-empty tracked branch instead of failing closed on
// an empty one. lastCommitSHA, if non-empty, is also recorded via
// UpdateItemSessionGitActivity (needed by closeIfSupersededByMain's separate
// LastCommitSha/IsCommitOnMain check).
func newTrackedWorkSession(t *testing.T, storage *Storage, itemID, repoPath, branchName, lastCommitSHA string) {
	t.Helper()
	ctx := context.Background()

	inst := newTestInstance("tracked-work-session")
	inst.UUID = uuid.New().String()
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath+"/../wt", "tracked-work-session", branchName, "abc123")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      itemID,
		SessionUUID: inst.UUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	if lastCommitSHA != "" {
		require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, is.ID, lastCommitSHA, "work", time.Now(), 1))
	}
}

// stubMatchingPRByNumberFinder installs a PRByNumberFinder on listener that
// reports headBranch as the head ref of whatever PR number is looked up —
// the "verified" stub shape a pre-existing mutation-path test needs so
// Story 6's new guard confirms the match instead of failing closed.
func stubMatchingPRByNumberFinder(listener *BacklogLifecycleListener, headBranch string) {
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: headBranch}, nil
	})
}

// --- Task 6.6: verifyPRHeadBranchMatchesTracked unit tests -----------------

func TestVerifyPRHeadBranchMatchesTracked_should_ReturnTrue_When_HeadBranchMatches(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: "backlog/x"}, nil
	})

	matches, err := listener.verifyPRHeadBranchMatchesTracked(ctx, "/tmp/fake-repo", "backlog/x", 42)

	require.NoError(t, err)
	assert.True(t, matches)
}

func TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_HeadBranchDiffers(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: "feature/y"}, nil
	})

	matches, err := listener.verifyPRHeadBranchMatchesTracked(ctx, "/tmp/fake-repo", "backlog/x", 42)

	require.NoError(t, err)
	assert.False(t, matches)
}

// TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_TrackedBranchEmpty
// proves the fail-closed path short-circuits before any GitHub call: an
// empty tracked branch (the caller couldn't resolve the item's own tracked
// branch) must never be treated as "nothing to check, so it matches".
func TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_TrackedBranchEmpty(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	listener := NewBacklogLifecycleListener(storage)
	finderCalled := false
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		finderCalled = true
		return &github.PRInfo{HeadRef: "backlog/x"}, nil
	})

	matches, err := listener.verifyPRHeadBranchMatchesTracked(ctx, "/tmp/fake-repo", "", 42)

	assert.False(t, matches)
	require.Error(t, err)
	assert.False(t, finderCalled, "the finder must never be called when there is no tracked branch to verify against")
}

func TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_FinderErrors(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return nil, errors.New("transient GitHub failure")
	})

	matches, err := listener.verifyPRHeadBranchMatchesTracked(ctx, "/tmp/fake-repo", "backlog/x", 42)

	assert.False(t, matches, "a lookup failure must never be read as a verified match")
	require.Error(t, err)
}

// --- Task 6.6: integration tests — the 3 mutation sites skip on guard failure ---

// TestCloseIfSupersededByMain_should_NotClosePR_When_HeadBranchMismatchDetected
// is the direct proof of adversarial-review.md's Blocker fix for
// closeIfSupersededByMain: an item whose PR head branch no longer matches
// its tracked branch (the override_reason-linked shape) must never be
// auto-closed on the strength of item.PrNumber alone, even when its last
// work-session commit is confirmed already on main.
func TestCloseIfSupersededByMain_should_NotClosePR_When_HeadBranchMismatchDetected(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shipped.txt"), []byte("already shipped\n"), 0o644))
	runGitTestCmd(t, dir, "add", "shipped.txt")
	runGitTestCmd(t, dir, "commit", "-m", "the fix, already on main via a different PR")
	shippedSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Branch-mismatch superseded test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/999"
	prNumber := 999
	updated, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	newTrackedWorkSession(t, storage, item.ID, dir, "backlog/tracked-branch", shippedSHA)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		// Mismatched: report_pr_created's override_reason path would attach
		// a PR whose real head branch differs from the item's tracked one.
		return &github.PRInfo{HeadRef: "some-other-branch"}, nil
	})
	checker := &fakePRPendingChecker{status: &git.PRStatus{IsClosed: true}}

	closed := listener.closeIfSupersededByMain(ctx, checker, updated)

	assert.False(t, closed, "a head-branch mismatch must prevent the auto-close")
	assert.False(t, checker.closeCalled, "ClosePR must never be called when the guard trips")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "the item must remain pr_pending, not be auto-completed")
	assert.Equal(t, prNumber, fetched.PrNumber, "PrNumber must remain set — not cleared by a guard-blocked close")
}

// TestReconcilePRPending_should_NotTransitionToDone_When_HeadBranchMismatchDetected
// is the direct proof of adversarial-review.md's Blocker fix for
// ReconcilePRPending's merge-detected done transition (Task 6.4): a merged
// PR whose head branch doesn't match the item's tracked branch must not
// auto-complete the item.
func TestReconcilePRPending_should_NotTransitionToDone_When_HeadBranchMismatchDetected(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 148)
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/tracked-branch", "")

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: "some-other-branch"}, nil
	})

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status,
		"a merged PR whose head branch no longer matches the tracked branch must not auto-complete the item")
}

// TestReconcileBouncingItems_should_NotTransitionToDone_When_HeadBranchMismatchDetected
// is the direct proof of adversarial-review.md's Blocker fix for
// reconcileBouncingItems' IsPRMerged-driven done transition (Task 6.5).
func TestReconcileBouncingItems_should_NotTransitionToDone_When_HeadBranchMismatchDetected(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item with merged PR, mismatched head branch",
		Status:   string(BacklogStatusInProgress),
		RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	prNumber := 172
	prURL := "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/172"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/tracked-branch", "")

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: "some-other-branch"}, nil
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.NotEqual(t, string(BacklogStatusDone), fetched.Status,
		"a merged PR whose head branch no longer matches the tracked branch must not auto-complete the item")
}

// --- Task 6.3a: fix-spawn disclaimer tests ----------------------------------

// TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_ClosedPRHeadBranchMismatchDetected
// covers the closed-without-merging fixCtx-building call site (~line 4045):
// closeIfSupersededByMain falls through for its own ordinary reason (the
// work session's last commit isn't on main yet), and Task 6.3a's independent
// re-check must still catch the head-branch mismatch and disclose it.
func TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_ClosedPRHeadBranchMismatchDetected(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")
	runGitTestCmd(t, dir, "checkout", "-b", "backlog/closed-mismatch")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("wip\n"), 0o644))
	runGitTestCmd(t, dir, "add", "feature.txt")
	runGitTestCmd(t, dir, "commit", "-m", "feature work, not yet on main")
	featureSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))
	runGitTestCmd(t, dir, "checkout", "main")

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Closed PR, unverified association",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/501"
	prNumber := 501
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	newTrackedWorkSession(t, storage, item.ID, dir, "backlog/closed-mismatch", featureSHA)

	listener := NewBacklogLifecycleListener(storage)
	checker := &fakePRPendingChecker{status: &git.PRStatus{IsClosed: true}}
	overridePRPendingChecker(t, listener, checker)
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)
	// Mismatched: the finder reports a different head branch than this
	// item's actually-tracked branch (backlog/closed-mismatch).
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: "some-other-branch"}, nil
	})

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	require.True(t, fakeSpawner.spawnCalled, "closeIfSupersededByMain must have fallen through to the normal reopen path (commit not yet on main)")
	assert.Contains(t, fakeSpawner.lastFixContext, "NOTE: this PR's association with this backlog item could not be verified",
		"an unverified PR association must be disclosed to the spawned fix session")
}

// TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_CIFailingPRHeadBranchMismatchDetected
// covers the CI-failing/blocked/conflicting fixCtx-building call site
// (~line 4158).
func TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_CIFailingPRHeadBranchMismatchDetected(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")
	runGitTestCmd(t, dir, "checkout", "-b", "backlog/ci-mismatch")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("wip\n"), 0o644))
	runGitTestCmd(t, dir, "add", "feature.txt")
	runGitTestCmd(t, dir, "commit", "-m", "feature work, not yet on main")
	featureSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))
	runGitTestCmd(t, dir, "checkout", "main")

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "CI-failing PR, unverified association",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/502"
	prNumber := 502
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	newTrackedWorkSession(t, storage, item.ID, dir, "backlog/ci-mismatch", featureSHA)

	listener := NewBacklogLifecycleListener(storage)
	checker := &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	}
	overridePRPendingChecker(t, listener, checker)
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)
	listener.SetPRByNumberFinder(func(context.Context, string, int) (*github.PRInfo, error) {
		return &github.PRInfo{HeadRef: "some-other-branch"}, nil
	})

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	require.True(t, fakeSpawner.spawnCalled, "closeIfSupersededByMain must have fallen through to the normal fix-spawn path (commit not yet on main)")
	assert.Contains(t, fakeSpawner.lastFixContext, "NOTE: this PR's association with this backlog item could not be verified",
		"an unverified PR association must be disclosed to the spawned fix session")
}

// TestReconcilePRPending_should_OmitUnverifiedDisclaimer_When_CIFailingPRHeadBranchMatchesTracked
// is the control case: when the PR's head branch genuinely matches the
// item's tracked branch, fixCtx must be byte-for-byte identical to what the
// pre-Task-6.3a behavior already produced — this task must not change the
// normal, verified CI-fix path.
func TestReconcilePRPending_should_OmitUnverifiedDisclaimer_When_CIFailingPRHeadBranchMatchesTracked(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")
	runGitTestCmd(t, dir, "checkout", "-b", "backlog/ci-matches")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("wip\n"), 0o644))
	runGitTestCmd(t, dir, "add", "feature.txt")
	runGitTestCmd(t, dir, "commit", "-m", "feature work, not yet on main")
	featureSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))
	runGitTestCmd(t, dir, "checkout", "main")

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "CI-failing PR, verified association",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/503"
	prNumber := 503
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	newTrackedWorkSession(t, storage, item.ID, dir, "backlog/ci-matches", featureSHA)

	listener := NewBacklogLifecycleListener(storage)
	feedbackText := "## Failing CI checks\n- build FAILED\n"
	checker := &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: feedbackText,
		},
	}
	overridePRPendingChecker(t, listener, checker)
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)
	// Verified: the finder reports the same head branch this item is
	// actually tracking.
	stubMatchingPRByNumberFinder(listener, "backlog/ci-matches")

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	require.True(t, fakeSpawner.spawnCalled)
	assert.NotContains(t, fakeSpawner.lastFixContext, "NOTE:", "a verified PR association must not carry the unverified disclaimer")
	expected := fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", prNumber, prURL, feedbackText)
	assert.Equal(t, expected, fakeSpawner.lastFixContext, "the normal, verified CI-fix fixCtx must be byte-for-byte unchanged by Task 6.3a")
}
