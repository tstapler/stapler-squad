package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// corruptPackedRefs overwrites repoDir's .git/packed-refs with malformed content, forcing
// repo.Reference() to return a non-ErrReferenceNotFound error for any branch that has no
// loose ref (falls through to the packed-refs parser and fails there). A malformed *loose*
// ref file does NOT work for this fixture: go-git's plumbing.NewHash() silently discards
// hex-decode failures and returns a zero hash with a nil error, which is indistinguishable
// from "branch exists" — verified against go-git v5.14.0 (see project_plans research notes).
func corruptPackedRefs(t *testing.T, repoDir string) {
	t.Helper()
	packedRefsPath := filepath.Join(repoDir, ".git", "packed-refs")
	require.NoError(t, os.WriteFile(packedRefsPath, []byte("not a valid packed-refs file\nrandom garbage"), 0644))
}

func TestBranchRefExists_ReturnsFalseNil_When_BranchAbsent(t *testing.T) {
	repoDir := setupTestRepo(t)
	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)

	exists, err := branchRefExists(repo, plumbing.NewBranchReferenceName("does-not-exist"))
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestBranchRefExists_ReturnsTrueNil_When_BranchExists(t *testing.T) {
	repoDir := setupTestRepo(t)
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", "existing-feature")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)

	exists, err := branchRefExists(repo, plumbing.NewBranchReferenceName("existing-feature"))
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestBranchRefExists_ReturnsError_When_PackedRefsCorrupted(t *testing.T) {
	repoDir := setupTestRepo(t)
	corruptPackedRefs(t, repoDir)

	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)

	exists, err := branchRefExists(repo, plumbing.NewBranchReferenceName("backlog/fix-typo-abc123"))
	require.Error(t, err)
	assert.False(t, exists)
	assert.False(t, errors.Is(err, plumbing.ErrReferenceNotFound),
		"a genuine ref-read failure must not be classified as ErrReferenceNotFound")
	assert.Contains(t, err.Error(), "failed to check branch reference")
}

// TestSetup_SurfacesError_When_BranchRefIsMalformed is the regression test for the bug: a
// non-ErrReferenceNotFound repo.Reference() error must surface from Setup() instead of being
// silently treated as "branch does not exist" and falling through to worktree creation.
// Both the sentinel check and the message check matter here: cleanupExistingBranch() (see
// worktree_branch.go) also touches the same corrupted packed-refs file and fails with its
// own, differently-worded error if the misclassification bug is reintroduced, so the
// sentinel check is what actually pins this test to branchRefExists's own classification
// rather than to whichever call site happens to fail first on this fixture.
func TestSetup_SurfacesError_When_BranchRefIsMalformed(t *testing.T) {
	repoDir := setupTestRepo(t)

	branchName := "backlog/fix-typo-abc123"
	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-bad-ref", branchName)
	require.NoError(t, err)

	corruptPackedRefs(t, repoDir)

	err = wt.Setup()
	require.Error(t, err, "Setup() must surface a malformed ref as an error, not silently treat it as branch-absent")
	assert.False(t, errors.Is(err, plumbing.ErrReferenceNotFound),
		"a genuine ref-read failure must not be classified as ErrReferenceNotFound")
	assert.Contains(t, err.Error(), "failed to check branch reference")

	// Must not have fallen through to worktree creation.
	_, statErr := os.Stat(wt.worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not have been created after a ref-check error")
}

// TestSetupNewWorktree_SurfacesError_When_BranchRefIsMalformed covers setupNewWorktree()'s
// own re-check call site directly (not reachable through Setup() alone, since Setup()'s
// upfront goroutine check would already have surfaced the same error and never called
// setupNewWorktree() at all — this exercises the second call site's own use of
// branchRefExists independently, e.g. as it would be reached via a stale/racy upstream
// read). It also guards against data loss: setupNewWorktree()'s ref check is the only gate
// before cleanupExistingBranch() unconditionally calls RemoveReference on the ref store, so
// a misclassified error here previously fell through into that call. The source shows the
// early return on a non-nil branchRefExists error precedes the cleanupExistingBranch() call
// (worktree_ops.go), so cleanupExistingBranch() cannot run here; the packed-refs-unchanged
// assertion below is a best-effort regression guard on top of that, not independent proof —
// for this specific corrupted-packed-refs fixture, an errantly-invoked RemoveReference would
// also fail to parse the file and leave it unchanged, so the assertion alone can't
// distinguish "never called" from "called but failed before writing." See
// TestBranchRefExists_LeavesRealRefIntact_When_UnderlyingReadFails below for a test that
// proves, via a fault injected below the packed-refs layer, that a real branch ref actually
// still resolves after the classification helper both call sites share returns this error.
func TestSetupNewWorktree_SurfacesError_When_BranchRefIsMalformed(t *testing.T) {
	repoDir := setupTestRepo(t)

	branchName := "backlog/fix-typo-abc123"
	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-bad-ref-direct", branchName)
	require.NoError(t, err)

	corruptPackedRefs(t, repoDir)
	packedRefsPath := filepath.Join(repoDir, ".git", "packed-refs")
	before, readErr := os.ReadFile(packedRefsPath)
	require.NoError(t, readErr)

	err = wt.setupNewWorktree()
	require.Error(t, err, "setupNewWorktree() must surface a malformed ref as an error")
	assert.False(t, errors.Is(err, plumbing.ErrReferenceNotFound),
		"a genuine ref-read failure must not be classified as ErrReferenceNotFound")
	assert.Contains(t, err.Error(), "failed to check branch reference")

	_, statErr := os.Stat(wt.worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not have been created after a ref-check error")

	after, readErr := os.ReadFile(packedRefsPath)
	require.NoError(t, readErr)
	assert.Equal(t, before, after,
		"packed-refs must be untouched after a ref-check error (regression guard, see doc comment above for what this does and doesn't prove)")
}

// refFailStorer wraps a real storage.Storer and forces Reference() to return a fabricated
// non-ErrReferenceNotFound error for one specific ref, while passing every other operation
// through to the real underlying storer unchanged. This lets a test force branchRefExists's
// error path against a branch that has a real, resolvable ref — something the
// corrupted-packed-refs fixture used elsewhere in this file cannot do, since corrupting
// packed-refs necessarily makes the whole ref store unreadable (including for independent
// post-hoc verification), not just the one branch under test.
type refFailStorer struct {
	storage.Storer
	failRef plumbing.ReferenceName
	err     error
}

func (s *refFailStorer) Reference(name plumbing.ReferenceName) (*plumbing.Reference, error) {
	if name == s.failRef {
		return nil, s.err
	}
	return s.Storer.Reference(name)
}

// TestBranchRefExists_LeavesRealRefIntact_When_UnderlyingReadFails proves the literal
// requirement behind AC5 (worktree-branch-exists-race): when branchRefExists — the single
// helper both Setup() and setupNewWorktree() call — encounters a non-ErrReferenceNotFound
// error, the real branch ref on disk is left completely untouched. Unlike the
// corrupted-packed-refs fixture, this uses a wrapped storer to fail only the read for the
// target branch, leaving the rest of the real filesystem-backed ref store fully intact and
// independently verifiable: a freshly-opened, unwrapped repo confirms the branch ref still
// resolves to its original commit after the forced error.
func TestBranchRefExists_LeavesRealRefIntact_When_UnderlyingReadFails(t *testing.T) {
	repoDir := setupTestRepo(t)

	branchName := "existing-feature-fault-injected"
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", branchName)
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	realRepo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)

	branchRef := plumbing.NewBranchReferenceName(branchName)
	beforeRef, err := realRepo.Reference(branchRef, false)
	require.NoError(t, err)
	expectedHash := beforeRef.Hash()

	fakeErr := errors.New("simulated ref-read I/O error")
	faultyRepo := &git.Repository{
		Storer: &refFailStorer{Storer: realRepo.Storer, failRef: branchRef, err: fakeErr},
	}

	exists, err := branchRefExists(faultyRepo, branchRef)
	require.Error(t, err)
	assert.False(t, exists)
	assert.False(t, errors.Is(err, plumbing.ErrReferenceNotFound),
		"a genuine ref-read failure must not be classified as ErrReferenceNotFound")
	assert.ErrorIs(t, err, fakeErr)

	// Prove the branch ref genuinely still exists and resolves to the same commit, via a
	// completely independent, freshly-opened repo untouched by the fault injection above.
	freshRepo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)
	afterRef, err := freshRepo.Reference(branchRef, false)
	require.NoError(t, err, "branch ref must still exist and resolve after a ref-check error")
	assert.Equal(t, expectedHash, afterRef.Hash())
}

// TestSetupNewWorktree_RespectsPreSetBaseCommitSHA is the regression test for the
// stale-HEAD backlog-spawn bug: setupNewWorktree() used to unconditionally overwrite
// baseCommitSHA with `rev-parse HEAD` of repoPath, silently discarding any base a caller
// had already selected (e.g. NewGitWorktreeFromCommitSHA, or CreateBacklogWorktree
// resolving origin/main's fetched tip). This asserts the worktree is branched from the
// pre-set commit even though repoPath's own HEAD has since moved past it.
func TestSetupNewWorktree_RespectsPreSetBaseCommitSHA(t *testing.T) {
	repoDir := setupTestRepo(t)

	firstCommit, err := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "rev-parse", "HEAD").CombinedOutput()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(firstCommit))

	// Advance repoPath's HEAD past baseSHA, simulating a shared checkout that has drifted
	// ahead of (or independently of) the commit the caller actually wants to branch from.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "later.txt"), []byte("later"), 0644))
	for _, args := range [][]string{
		{"-C", repoDir, "add", "."},
		{"-C", repoDir, "commit", "-m", "second commit"},
	} {
		out, cmdErr := safeexec.CommandContext(context.Background(), "git", args...).CombinedOutput()
		require.NoError(t, cmdErr, "git %v failed: %s", args, out)
	}
	headCommit, err := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "rev-parse", "HEAD").CombinedOutput()
	require.NoError(t, err)
	require.NotEqual(t, baseSHA, strings.TrimSpace(string(headCommit)), "test setup must advance HEAD past baseSHA")

	branchName := "backlog/pre-set-base-sha"
	wt, _, err := NewGitWorktreeFromCommitSHA(repoDir, "test-pre-set-base", branchName, baseSHA)
	require.NoError(t, err)

	err = wt.setupNewWorktree()
	require.NoError(t, err)
	defer func() { _ = wt.Cleanup() }()

	assert.Equal(t, baseSHA, wt.GetBaseCommitSHA(), "baseCommitSHA must remain the pre-set commit, not be overwritten by repoPath's HEAD")

	worktreeHead, err := safeexec.CommandContext(context.Background(), "git", "-C", wt.worktreePath, "rev-parse", "HEAD").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, baseSHA, strings.TrimSpace(string(worktreeHead)), "worktree must be checked out at the pre-set base commit, not repoPath's current HEAD")

	_, statErr := os.Stat(filepath.Join(wt.worktreePath, "later.txt"))
	assert.True(t, os.IsNotExist(statErr), "worktree must not contain changes made after the pre-set base commit")
}

// TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists covers setupNewWorktree()'s
// own reuse path directly, independent of Setup()'s upfront goroutine (which would normally
// short-circuit straight to setupFromExistingBranch and never reach setupNewWorktree() at
// all when the branch already exists).
func TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists(t *testing.T) {
	repoDir := setupTestRepo(t)

	branchName := "existing-feature-direct"
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", branchName)
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-existing-direct", branchName)
	require.NoError(t, err)

	err = wt.setupNewWorktree()
	require.NoError(t, err)
	defer func() { _ = wt.Cleanup() }()

	assert.NotEmpty(t, wt.GetBaseCommitSHA())

	out, statErr := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", "--list", branchName).CombinedOutput()
	require.NoError(t, statErr)
	assert.True(t, strings.Contains(string(out), branchName), "branch must still exist after reuse")
}

// TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate is the regression
// test for the self-heal fallback in setupNewWorktree's "worktree add -b" error handling.
// Unlike TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists — which pre-creates the
// branch before calling setupNewWorktree, so branchRefExists is already true at the
// function's own upfront check and setupFromExistingBranch is reached via that early path —
// this test starts with the branch absent for both callers and races two real
// setupNewWorktree calls against the identical branch name, the same shape as two concurrent
// backlog spawns for the same item computing the same deterministic branchWorkSlug. Both
// callers' upfront branchRefExists checks can observe "false" before either has created the
// branch; the loser's "git worktree add -b" fails with "a branch named '<branch>' already
// exists", triggering setupNewWorktree's fallback into setupFromExistingBranch — which then
// hits its own second race window: by the time it runs, the winner has often already checked
// out the branch into its worktree, so setupFromExistingBranch's own "worktree add <path>
// <branch>" (no -b) fails too, with git 2.50.1's "'<branch>' is already used by worktree at
// '<path>'" (older git instead says "already checked out") — which setupFromExistingBranch
// must also recognize to find and reuse the winner's worktree rather than hard-failing. This
// test exercises both layers together. Which of the two callers wins vs. loses is inherently
// nondeterministic (real git subprocess timing), so this test does not assert on which one
// self-healed — only on the invariant the fix guarantees: neither concurrent caller may
// hard-fail.
func TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate(t *testing.T) {
	repoDir := setupTestRepo(t)
	branchName := "backlog/concurrent-race-fixture"

	wt1, _, err := NewGitWorktreeWithBranch(repoDir, "test-race-1", branchName)
	require.NoError(t, err)
	wt2, _, err := NewGitWorktreeWithBranch(repoDir, "test-race-2", branchName)
	require.NoError(t, err)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs[0] = wt1.setupNewWorktree()
	}()
	go func() {
		defer wg.Done()
		<-start
		errs[1] = wt2.setupNewWorktree()
	}()
	close(start)
	wg.Wait()

	require.NoError(t, errs[0], "first concurrent setup must not hard-fail on a lost branch-create race")
	require.NoError(t, errs[1], "second concurrent setup must not hard-fail on a lost branch-create race")
	defer func() { _ = wt1.Cleanup() }()
	defer func() { _ = wt2.Cleanup() }()

	out, statErr := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", "--list", branchName).CombinedOutput()
	require.NoError(t, statErr)
	assert.True(t, strings.Contains(string(out), branchName), "branch must exist once the race resolves")
}

// TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo models the
// real-world failure this fix addresses: several independent worktree creations (e.g. multiple
// backlog-triage spawns, or duplicate server processes) hitting the same repo's shared
// .git/worktrees/ administrative metadata at once, each for a distinct branch/worktree path.
// Unlike TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate (which calls the
// unlocked setupNewWorktree() directly to exercise same-branch self-heal logic), this test calls
// the public, now-lock-wrapped Setup() to verify withRepoWorktreeLock actually prevents the
// metadata race rather than merely tolerating one branch-create collision.
func TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo(t *testing.T) {
	repoDir := setupTestRepo(t)
	const n = 8

	worktrees := make([]*GitWorktree, n)
	for i := 0; i < n; i++ {
		branchName := fmt.Sprintf("backlog/concurrent-setup-%d", i)
		wt, _, err := NewGitWorktreeWithBranch(repoDir, fmt.Sprintf("test-concurrent-setup-%d", i), branchName)
		require.NoError(t, err)
		worktrees[i] = wt
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			errs[i] = worktrees[i].Setup()
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "concurrent Setup() for worktree %d must not fail due to a .git/worktrees/ metadata race", i)
	}

	for _, wt := range worktrees {
		wt := wt
		defer func() { _ = wt.Cleanup() }()
	}
}
