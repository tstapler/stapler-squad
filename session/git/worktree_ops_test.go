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
	"time"

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
	t.Parallel()
	repoDir := setupTestRepo(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)

	exists, err := branchRefExists(repo, plumbing.NewBranchReferenceName("does-not-exist"))
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestBranchRefExists_ReturnsTrueNil_When_BranchExists(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", "existing-feature")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)

	exists, err := branchRefExists(repo, plumbing.NewBranchReferenceName("existing-feature"))
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestBranchRefExists_ReturnsError_When_PackedRefsCorrupted(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	corruptPackedRefs(t, repoDir)

	repo, err := OpenRepo(repoDir)
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
func TestSetup_SurfacesError_When_BranchRefIsMalformed(t *testing.T) {
	t.Parallel()
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

// TestSetupNewWorktree_SurfacesError_When_BranchRefIsMalformed covers setupLocked's own
// re-check call site directly (not reachable through Setup() alone, since Setup()'s
// upfront goroutine check would already have surfaced the same error and never reached
// this second call site at all) — this exercises that second call site's own use of
// branchRefExists independently, e.g. as it would be reached via a stale/racy upstream
// read. See TestBranchRefExists_LeavesRealRefIntact_When_UnderlyingReadFails below for a
// test that proves, via a fault injected below the packed-refs layer, that a real branch
// ref actually still resolves after the classification helper both call sites share
// returns this error.
func TestSetupNewWorktree_SurfacesError_When_BranchRefIsMalformed(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	branchName := "backlog/fix-typo-abc123"
	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-bad-ref-direct", branchName)
	require.NoError(t, err)

	corruptPackedRefs(t, repoDir)
	packedRefsPath := filepath.Join(repoDir, ".git", "packed-refs")
	before, readErr := os.ReadFile(packedRefsPath)
	require.NoError(t, readErr)

	err = wt.setupLocked()
	require.Error(t, err, "setupLocked() must surface a malformed ref as an error")
	assert.False(t, errors.Is(err, plumbing.ErrReferenceNotFound),
		"a genuine ref-read failure must not be classified as ErrReferenceNotFound")
	assert.Contains(t, err.Error(), "failed to check branch reference")

	_, statErr := os.Stat(wt.worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not have been created after a ref-check error")

	after, readErr := os.ReadFile(packedRefsPath)
	require.NoError(t, readErr)
	assert.Equal(t, before, after,
		"packed-refs must be untouched after a ref-check error")
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
// helper both Setup() and setupLocked() call — encounters a non-ErrReferenceNotFound
// error, the real branch ref on disk is left completely untouched. Unlike the
// corrupted-packed-refs fixture, this uses a wrapped storer to fail only the read for the
// target branch, leaving the rest of the real filesystem-backed ref store fully intact and
// independently verifiable: a freshly-opened, unwrapped repo confirms the branch ref still
// resolves to its original commit after the forced error.
func TestBranchRefExists_LeavesRealRefIntact_When_UnderlyingReadFails(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	branchName := "existing-feature-fault-injected"
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", branchName)
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	realRepo, err := OpenRepo(repoDir)
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
	freshRepo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	afterRef, err := freshRepo.Reference(branchRef, false)
	require.NoError(t, err, "branch ref must still exist and resolve after a ref-check error")
	assert.Equal(t, expectedHash, afterRef.Hash())
}

// TestSetupNewWorktree_RespectsPreSetBaseCommitSHA is the regression test for the
// stale-HEAD backlog-spawn bug: the new-worktree setup path used to unconditionally overwrite
// baseCommitSHA with `rev-parse HEAD` of repoPath, silently discarding any base a caller
// had already selected (e.g. NewGitWorktreeFromCommitSHA, or CreateBacklogWorktree
// resolving origin/main's fetched tip). This asserts the worktree is branched from the
// pre-set commit even though repoPath's own HEAD has since moved past it.
func TestSetupNewWorktree_RespectsPreSetBaseCommitSHA(t *testing.T) {
	t.Parallel()
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

	err = wt.setupLocked()
	require.NoError(t, err)
	defer func() { _ = wt.Cleanup() }()

	assert.Equal(t, baseSHA, wt.GetBaseCommitSHA(), "baseCommitSHA must remain the pre-set commit, not be overwritten by repoPath's HEAD")

	worktreeHead, err := safeexec.CommandContext(context.Background(), "git", "-C", wt.worktreePath, "rev-parse", "HEAD").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, baseSHA, strings.TrimSpace(string(worktreeHead)), "worktree must be checked out at the pre-set base commit, not repoPath's current HEAD")

	_, statErr := os.Stat(filepath.Join(wt.worktreePath, "later.txt"))
	assert.True(t, os.IsNotExist(statErr), "worktree must not contain changes made after the pre-set base commit")
}

// TestInitBaseCommitSHA_UsesWorktreePath_NotRepoPathAmbientCheckout is a direct
// regression test for backlog item e7664cbf: initBaseCommitSHA used to run
// `git merge-base HEAD <candidate>` with cwd=g.repoPath, so "HEAD" resolved to
// whatever branch the shared parent repoPath's checkout happened to be on —
// not this worktree's own branch — making the computed base wrong (or, for an
// orphan branch, unresolvable) whenever repoPath had drifted. This constructs a
// GitWorktree pointing at a real, separate worktree directory (not repoPath
// itself) with "feature" checked out, deliberately leaves repoPath on an
// unrelated orphan branch sharing no history with "feature", and confirms
// initBaseCommitSHA still computes the correct merge-base with main by running
// inside the worktree — mirroring the already-correct sibling
// resolveBaseCommitSHA (session/git/diff.go), which this fix now matches.
func TestInitBaseCommitSHA_UsesWorktreePath_NotRepoPathAmbientCheckout(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	mainSHA, err := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "rev-parse", "main").CombinedOutput()
	require.NoError(t, err)
	expectedBaseSHA := strings.TrimSpace(string(mainSHA))

	worktreeDir := t.TempDir()
	for _, args := range [][]string{
		{"-C", repoDir, "worktree", "add", "-b", "feature", worktreeDir, "main"},
		{"-C", worktreeDir, "config", "user.email", "test@example.com"},
		{"-C", worktreeDir, "config", "user.name", "Test"},
	} {
		out, cmdErr := safeexec.CommandContext(context.Background(), "git", args...).CombinedOutput()
		require.NoError(t, cmdErr, "git %v failed: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "feature.txt"), []byte("real work"), 0644))
	for _, args := range [][]string{
		{"-C", worktreeDir, "add", "."},
		{"-C", worktreeDir, "commit", "-m", "real fix"},
	} {
		out, cmdErr := safeexec.CommandContext(context.Background(), "git", args...).CombinedOutput()
		require.NoError(t, cmdErr, "git %v failed: %s", args, out)
	}

	// Simulate a concurrent process leaving the shared repoPath on an unrelated
	// orphan branch with no common history with "feature" — the worst case for the
	// old repoPath-cwd implementation (would fail outright with "no merge base").
	for _, args := range [][]string{
		{"-C", repoDir, "checkout", "--orphan", "unrelated"},
		{"-C", repoDir, "commit", "--allow-empty", "-m", "unrelated history"},
	} {
		out, cmdErr := safeexec.CommandContext(context.Background(), "git", args...).CombinedOutput()
		require.NoError(t, cmdErr, "git %v failed: %s", args, out)
	}

	wt := NewGitWorktreeFromStorage(repoDir, worktreeDir, "test-init-base-sha", "feature", "")
	require.NotNil(t, wt)

	wt.initBaseCommitSHA()

	assert.Equal(t, expectedBaseSHA, wt.GetBaseCommitSHA(),
		"initBaseCommitSHA must find feature's real merge-base with main by running inside the worktree, unaffected by repoPath's ambient checkout")
}

// TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists covers setupLocked's own
// reuse path directly, independent of Setup()'s upfront goroutine (which would normally
// short-circuit straight to setupFromExistingBranch and never reach that call site at all
// when the branch already exists).
func TestSetupNewWorktree_UsesExistingBranch_When_BranchRefExists(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	branchName := "existing-feature-direct"
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", branchName)
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-existing-direct", branchName)
	require.NoError(t, err)

	err = wt.setupLocked()
	require.NoError(t, err)
	defer func() { _ = wt.Cleanup() }()

	assert.NotEmpty(t, wt.GetBaseCommitSHA())

	out, statErr := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", "--list", branchName).CombinedOutput()
	require.NoError(t, statErr)
	assert.True(t, strings.Contains(string(out), branchName), "branch must still exist after reuse")
}

// TestWorktreeAlreadyRegisteredForBranch_MatchesRawAgainstCanonicalPath verifies AC3: when
// g.worktreePath holds a raw, not-yet-canonicalized spelling of the same directory git
// reports (realpath'd) via 'worktree list --porcelain', the two must still be recognized as
// the same worktree rather than spuriously mismatching and triggering an unnecessary
// remove+recreate.
func TestWorktreeAlreadyRegisteredForBranch_MatchesRawAgainstCanonicalPath(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	branchName := "already-registered-raw-vs-canonical"
	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-raw-vs-canonical", branchName)
	require.NoError(t, err)
	require.NoError(t, wt.setupLocked())
	defer func() { _ = wt.Cleanup() }()

	// Build a symlink alias to the real worktree directory and point g.worktreePath at
	// the alias (the "raw" spelling) instead of the canonical path setupLocked
	// actually created on disk. git itself only ever knows about the real directory, so
	// 'worktree list --porcelain' will report the canonical path — exercising exactly the
	// raw-vs-canonicalized mismatch this fix must tolerate.
	rawAlias := filepath.Join(t.TempDir(), "raw-alias-to-worktree")
	if err := os.Symlink(wt.worktreePath, rawAlias); err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}
	wt.worktreePath = rawAlias

	assert.True(t, wt.worktreeAlreadyRegisteredForBranch(),
		"raw alias path %q for the same worktree as git's canonical report must still match", rawAlias)
}

// TestBranchExistsAfterAddFailure_ReturnsTrue_When_BranchAlreadyExists covers
// branchExistsAfterAddFailure's (the Ground-Truth Re-Query loop, ADR-001) fast path: a
// branch that already exists by the very first check (attempt 0, no sleep) is found
// immediately.
func TestBranchExistsAfterAddFailure_ReturnsTrue_When_BranchAlreadyExists(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "backlog/immediate-branch-exists"
	cmd := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", branchName)
	require.NoError(t, cmd.Run())

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-immediate-exists", branchName)
	require.NoError(t, err)

	assert.True(t, wt.branchExistsAfterAddFailure(plumbing.NewBranchReferenceName(branchName)))
}

// TestBranchExistsAfterAddFailure_ReturnsTrue_When_BranchAppearsDuringRetryWindow is the
// pre-mortem Failure #5 addendum: the test above finds the branch on attempt 0, so the
// retry loop's actual looping/backoff behavior is never exercised. This test delays branch
// creation until after the loop's first two checks would have observed "not found", so it
// only passes if the loop genuinely retries rather than checking once.
func TestBranchExistsAfterAddFailure_ReturnsTrue_When_BranchAppearsDuringRetryWindow(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "backlog/delayed-race-winner-layer1"

	go func() {
		// Past the loop's first two checks (attempt 0 has no pre-sleep; attempt 1
		// sleeps once) but well within its total budget.
		time.Sleep(2 * worktreeAddRetryDelay)
		cmd := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", branchName)
		_ = cmd.Run()
	}()

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-delayed-race-winner-layer1", branchName)
	require.NoError(t, err)

	assert.True(t, wt.branchExistsAfterAddFailure(plumbing.NewBranchReferenceName(branchName)),
		"must self-heal once the delayed race winner's branch appears within the retry window")
}

// TestBranchExistsAfterAddFailure_IncrementsRetryCounter_When_ItRetries is Task 4.4.2b's
// validation.md test: a branch-creation race that forces the Ground-Truth Re-Query loop to
// actually retry must increment git_worktree_retry_total. Deliberately not t.Parallel():
// git_worktree_retry_total has no attribute dimension to filter a before/after delta by,
// and Go's testing package runs every non-parallel test in this package to completion
// before any t.Parallel() test resumes, so this avoids racing against this package's other
// (parallel) self-heal race tests that also exercise this same retry loop.
func TestBranchExistsAfterAddFailure_IncrementsRetryCounter_When_ItRetries(t *testing.T) {
	repoDir := setupTestRepo(t)
	branchName := "backlog/retry-counter-fixture"

	go func() {
		time.Sleep(2 * worktreeAddRetryDelay)
		cmd := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "branch", branchName)
		_ = cmd.Run()
	}()

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-retry-counter-fixture", branchName)
	require.NoError(t, err)

	before := collectGitMetric(t, "git_worktree_retry_total")
	baseline := sumGitCounter(t, before)

	assert.True(t, wt.branchExistsAfterAddFailure(plumbing.NewBranchReferenceName(branchName)),
		"must self-heal once the delayed race winner's branch appears within the retry window")

	after := collectGitMetric(t, "git_worktree_retry_total")
	require.NotNil(t, after)
	assert.Greater(t, sumGitCounter(t, after), baseline, "expected at least one Ground-Truth Re-Query retry to be recorded")
}

// TestBranchExistsAfterAddFailure_ReturnsFalse_When_BranchNeverAppears is Story 1.1.1's
// negative case: Ground-Truth Re-Query must not spuriously report success (masking a
// genuine failure, e.g. disk full, permissions) when the branch never actually appears.
func TestBranchExistsAfterAddFailure_ReturnsFalse_When_BranchNeverAppears(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "backlog/never-created-layer1"

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-never-created-layer1", branchName)
	require.NoError(t, err)

	assert.False(t, wt.branchExistsAfterAddFailure(plumbing.NewBranchReferenceName(branchName)),
		"must not self-heal when the branch genuinely never appears")
}

// isWorktreeAddNoBranchCall reports whether a recorded gitSpyRunCall is
// setupFromExistingBranch's "git worktree add <path> <branch>" call (no "-b"), as opposed
// to the "worktree unlock"/"worktree remove -f" cleanup calls that also route through the
// same spy.
func isWorktreeAddNoBranchCall(call gitSpyRunCall) bool {
	return len(call.args) == 4 && call.args[0] == "worktree" && call.args[1] == "add"
}

// newWorktreeAddFailureSpy returns a gitSpyCommandRunner that fails
// setupFromExistingBranch's "worktree add <path> <branch>" call with an error string
// neither of the old "already checked out"/"already used by worktree" literals would have
// matched, letting every other call through unchanged.
func newWorktreeAddFailureSpy() *gitSpyCommandRunner {
	spy := &gitSpyCommandRunner{}
	spy.runFunc = func() ([]byte, error) {
		call := spy.runCalls[len(spy.runCalls)-1]
		if isWorktreeAddNoBranchCall(call) {
			return nil, errors.New("signal: killed")
		}
		return nil, nil
	}
	return spy
}

// TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError is the
// deterministic regression test for Ground-Truth Re-Query (ADR-001) at
// setupFromExistingBranch's layer: an error string neither of the old
// "already checked out"/"already used by worktree" literals would have matched must still
// self-heal by adopting the winner's worktree path once findLiveWorktreeForBranch reports
// it registered there.
func TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "backlog/unrecognized-error-layer2"
	winnerPath := CanonicalizeWorktreePath(filepath.Join(t.TempDir(), "winner-worktree"))

	winnerWt := NewGitWorktreeFromStorageWithExecutor(repoDir, winnerPath, "test-unrecognized-error-layer2-winner", branchName, "")
	require.NoError(t, winnerWt.nativeSetupNewWorktree())

	spy := newWorktreeAddFailureSpy()
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, filepath.Join(t.TempDir(), "loser-worktree"), "test-unrecognized-error-layer2", branchName, "", WithCommandRunner(spy))

	err := wt.setupFromExistingBranch()
	require.NoError(t, err, "must self-heal by adopting the winner's registered worktree path")
	assert.Equal(t, winnerPath, wt.worktreePath)
}

// TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner is the
// pre-mortem Failure #5 addendum for layer 2: the happy-path test above registers the
// winner before setupFromExistingBranch ever runs, so findLiveWorktreeForBranch's retry
// loop is never actually exercised. This test registers the winner only after a delay well
// into the retry window, so it only passes if the loop genuinely retries rather than
// checking once.
func TestSetupFromExistingBranch_SelfHeals_When_WorktreeRegisteredByDelayedRaceWinner(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "backlog/delayed-race-winner-layer2"
	winnerPath := CanonicalizeWorktreePath(filepath.Join(t.TempDir(), "winner-worktree"))

	go func() {
		time.Sleep(2 * worktreeAddRetryDelay)
		winnerWt := NewGitWorktreeFromStorageWithExecutor(repoDir, winnerPath, "test-delayed-race-winner-layer2-winner", branchName, "")
		_ = winnerWt.nativeSetupNewWorktree()
	}()

	spy := newWorktreeAddFailureSpy()
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, filepath.Join(t.TempDir(), "loser-worktree"), "test-delayed-race-winner-layer2", branchName, "", WithCommandRunner(spy))

	err := wt.setupFromExistingBranch()
	require.NoError(t, err, "must self-heal once the delayed race winner's registration appears within the retry window")
	assert.Equal(t, winnerPath, wt.worktreePath)
}

// TestSetupFromExistingBranch_HardFails_When_WorktreeAddErrorsAndBranchNotFoundAnywhere is
// Story 1.1.2's negative case: the unconditional re-query must still correctly hard-fail
// when the branch genuinely isn't registered to any worktree.
func TestSetupFromExistingBranch_HardFails_When_WorktreeAddErrorsAndBranchNotFoundAnywhere(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "backlog/never-registered-layer2"

	spy := newWorktreeAddFailureSpy()
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, filepath.Join(t.TempDir(), "loser-worktree"), "test-never-registered-layer2", branchName, "", WithCommandRunner(spy))

	err := wt.setupFromExistingBranch()
	require.Error(t, err, "must not self-heal when the branch is genuinely registered nowhere")
	assert.Contains(t, err.Error(), "failed to create worktree from branch")
}

// TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo models the
// real-world failure this fix addresses: several independent worktree creations (e.g. multiple
// backlog-triage spawns, or duplicate server processes) hitting the same repo's shared
// .git/worktrees/ administrative metadata at once, each for a distinct branch/worktree path.
// Unlike TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate (which calls the
// unlocked setupLocked() directly to exercise same-branch self-heal logic), this test calls
// the public, now-lock-wrapped Setup() to verify WithRepoWorktreeLock actually prevents the
// metadata race rather than merely tolerating one branch-create collision.
func TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo(t *testing.T) {
	t.Parallel()
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

// TestSetup_UsesNativeImplementation_ZeroSubprocessCalls covers Story 2.1.3's acceptance
// criterion.
func TestSetup_UsesNativeImplementation_ZeroSubprocessCalls(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "feature-native-on"
	worktreePath := filepath.Join(t.TempDir(), branchName)

	spy := &gitSpyCommandRunner{}
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-flag-on", branchName, "", WithCommandRunner(spy))

	require.NoError(t, wt.Setup())

	assert.Empty(t, spy.runCalls, "native path must issue zero git subprocess invocations for the worktree-add step")

	// Confirm the native implementation actually ran, not merely that no subprocess
	// call happened to occur: the admin dir and worktree redirect file exist with
	// nativeSetupNewWorktree's exact layout.
	adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
	_, err := os.Stat(filepath.Join(adminDir, "gitdir"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(worktreePath, ".git"))
	assert.NoError(t, err)
}

// TestSetupFromExistingBranch_UnlockUsesNativeImplementation covers Story 2.1.4's dispatch
// test: setupFromExistingBranch's unlock step routes through nativeUnlockWorktree (a
// direct filesystem removal, no subprocess), while the surrounding remove/re-add
// subprocess calls are unaffected (Task 2.1.4b's explicitly scoped-down unlock-only
// dispatch).
func TestSetupFromExistingBranch_UnlockUsesNativeImplementation(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "feature-native-unlock"
	worktreePath := filepath.Join(t.TempDir(), branchName)

	// Seed a real, native-created worktree, then simulate an interrupted subsequent
	// operation leaving a stale LockedMarker behind — the exact state
	// setupFromExistingBranch's cleanup path exists to recover from.
	seedWt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-unlock-seed", branchName, "")
	require.NoError(t, seedWt.nativeSetupNewWorktree())

	indexPath, err := resolveWorktreeIndexPath(worktreePath)
	require.NoError(t, err)
	adminDir := filepath.Dir(indexPath)
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "locked"), []byte("initializing"), 0644))

	spy := &gitSpyCommandRunner{}
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-unlock", branchName, "", WithCommandRunner(spy))

	require.NoError(t, wt.setupFromExistingBranch())

	for _, call := range spy.runCalls {
		if len(call.args) >= 2 && call.args[0] == "worktree" && call.args[1] == "unlock" {
			t.Fatalf("unlock step must not shell out to git, got call: %+v", call)
		}
	}

	_, statErr := os.Stat(filepath.Join(adminDir, "locked"))
	assert.True(t, os.IsNotExist(statErr), "nativeUnlockWorktree must have removed the locked marker directly")
}

// TestRemove_UsesNativeImplementation_ZeroSubprocessCalls covers Story 2.2.2's acceptance
// criterion: Remove() dispatches to nativeRemoveWorktree with zero subprocess calls
// recorded.
func TestRemove_UsesNativeImplementation_ZeroSubprocessCalls(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "feature-native-remove"
	worktreePath := filepath.Join(t.TempDir(), branchName)

	seedWt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-remove-seed", branchName, "")
	require.NoError(t, seedWt.nativeSetupNewWorktree())

	spy := &gitSpyCommandRunner{}
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-remove", branchName, "", WithCommandRunner(spy))

	require.NoError(t, wt.Remove())

	assert.Empty(t, spy.runCalls, "native path must issue zero git subprocess invocations for the worktree-remove step")

	_, err := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(err), "native remove must have deleted the working directory")

	adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
	_, err = os.Stat(adminDir)
	assert.True(t, os.IsNotExist(err), "native remove must have deleted the admin dir")
}

// TestFindLiveWorktreeForBranch_ZeroSubprocessCalls covers Epic 2.3's Story 2.3.2
// acceptance criterion: findLiveWorktreeForBranch finds a matching branch's live worktree
// via nativeListWorktrees, with zero subprocess calls.
func TestFindLiveWorktreeForBranch_ZeroSubprocessCalls(t *testing.T) {
	t.Parallel()
	branchName := "feature-native-find-live"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	spy := &gitSpyCommandRunner{}
	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "test-native-find-live", branchName, "", WithCommandRunner(spy))

	foundPath, found := wt.findLiveWorktreeForBranch()

	require.True(t, found)
	assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(foundPath))
	assert.Empty(t, spy.runCalls, "native path must issue zero git subprocess invocations")
}

// TestFindLiveWorktreeForBranch_NotFound_ZeroSubprocessCalls covers the symmetric miss
// case: no worktree registered for the branch, still zero subprocess calls, and the retry
// loop's sleeps don't apply forever (bounded by worktreeAddRetryAttempts).
func TestFindLiveWorktreeForBranch_NotFound_ZeroSubprocessCalls(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	spy := &gitSpyCommandRunner{}
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, filepath.Join(t.TempDir(), "unused"), "test-native-find-live-miss", "does-not-exist", "", WithCommandRunner(spy))

	_, found := wt.findLiveWorktreeForBranch()

	assert.False(t, found)
	assert.Empty(t, spy.runCalls, "native path must issue zero git subprocess invocations even on a miss")
}

// TestPrune_UsesNativeImplementation covers Story 2.4.2's acceptance criterion: Prune()
// dispatches to nativeWorktreePrune with zero subprocess calls recorded, removing a
// prunable entry's admin dir.
func TestPrune_UsesNativeImplementation(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	branchName := "feature-native-prune"
	worktreePath := filepath.Join(t.TempDir(), branchName)

	seedWt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-prune-seed", branchName, "")
	require.NoError(t, seedWt.nativeSetupNewWorktree())
	require.NoError(t, os.RemoveAll(worktreePath))

	spy := &gitSpyCommandRunner{}
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-prune", branchName, "", WithCommandRunner(spy))

	require.NoError(t, wt.Prune())

	assert.Empty(t, spy.runCalls, "native path must issue zero git subprocess invocations for the worktree-prune step")

	adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
	_, err := os.Stat(adminDir)
	assert.True(t, os.IsNotExist(err), "native prune must have removed the prunable admin dir")
}

// TestCleanupWorktreesPrune_UsesNativeImplementation covers the Epic 2.2 gap this epic
// closes (CleanupWorktrees' doc comment): its trailing prune step dispatches to
// nativeWorktreePrune against the process's current working directory. Deliberately not
// t.Parallel(): t.Chdir forbids it.
func TestCleanupWorktreesPrune_UsesNativeImplementation(t *testing.T) {
	repoDir := setupTestRepo(t)
	branchName := "feature-native-cleanup-prune"
	worktreePath := filepath.Join(t.TempDir(), branchName)

	seedWt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-native-cleanup-prune-seed", branchName, "")
	require.NoError(t, seedWt.nativeSetupNewWorktree())
	require.NoError(t, os.RemoveAll(worktreePath))

	t.Chdir(repoDir)

	require.NoError(t, cleanupWorktreesPrune())

	adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
	_, err := os.Stat(adminDir)
	assert.True(t, os.IsNotExist(err), "native cleanup prune must have removed the prunable admin dir")
}

// TestSetup_ConcurrentCalls_SerializeThroughSameLock covers Story 2.5.1's acceptance
// criterion: two Setup() calls against the same repoPath, started concurrently, must
// serialize through the same repoWorktreeLock registry entry.
//
// Proof strategy: the test acquires the shared repoWorktreeLock's intra-process mutex
// itself, *before* launching either Setup() call, then confirms both calls are still
// blocked (neither has returned) after a generous wait. Since WithRepoWorktreeLock's mu
// is a genuine sync.Mutex, this is not a timing heuristic — if either call resolved to a
// *different* lock (or skipped WithRepoWorktreeLock entirely), that call would complete
// immediately instead of blocking, and the test would fail deterministically rather than
// flakily. This avoids measuring the calls' own wall-clock windows (which naturally
// "overlap" whenever one is blocked waiting on the other — not evidence of a race, just
// proof a wait happened).
func TestSetup_ConcurrentCalls_SerializeThroughSameLock(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt1, _, err := NewGitWorktreeWithBranch(repoDir, "sess-1", "backlog/concurrent-lock-1")
	require.NoError(t, err)
	wt2, _, err := NewGitWorktreeWithBranch(repoDir, "sess-2", "backlog/concurrent-lock-2")
	require.NoError(t, err)

	lockInstance, err := lockForRepo(repoDir)
	require.NoError(t, err)

	lockInstance.mu.Lock()

	errs := make([]error, 2)
	done := make(chan int, 2)
	go func() {
		errs[0] = wt1.Setup()
		done <- 0
	}()
	go func() {
		errs[1] = wt2.Setup()
		done <- 1
	}()

	select {
	case which := <-done:
		t.Fatalf("Setup() call %d completed while the test held repoWorktreeLock.mu externally -- it is bypassing WithRepoWorktreeLock", which)
	case <-time.After(150 * time.Millisecond):
		// Expected: both goroutines are blocked on mu.Lock() inside WithRepoWorktreeLock.
	}

	lockInstance.mu.Unlock()

	<-done
	<-done
	require.NoError(t, errs[0], "first Setup() must not fail")
	require.NoError(t, errs[1], "second Setup() must not fail")
	defer func() { _ = wt1.Cleanup() }()
	defer func() { _ = wt2.Cleanup() }()

	lockAfter, err := lockForRepo(repoDir)
	require.NoError(t, err)
	assert.Same(t, lockInstance, lockAfter, "both calls must resolve to the same repoWorktreeLock singleton for repoPath")

	out := runRealGit(t, repoDir, "worktree", "list", "--porcelain")
	assert.Contains(t, out, "backlog/concurrent-lock-1", "first worktree must be registered and clean")
	assert.Contains(t, out, "backlog/concurrent-lock-2", "second worktree must be registered and clean")
	assert.NotContains(t, out, "prunable", "neither worktree may be left prunable/corrupted")
}

// TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate covers Task
// 2.5.2b: two concurrent, unlocked setupLocked() calls for the identical branch name (the
// same shape as two concurrent backlog spawns for the same item computing the same
// deterministic branchWorkSlug) must not hard-fail — nativeSetupNewWorktreeWithSelfHeal's
// Ground-Truth Re-Query (Task 2.5.2a) must close the race for whichever call loses it.
func TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate(t *testing.T) {
	repoDir := setupTestRepo(t)
	branchName := "backlog/native-concurrent-race-fixture"

	wt1, _, err := NewGitWorktreeWithBranch(repoDir, "test-native-race-1", branchName)
	require.NoError(t, err)
	wt2, _, err := NewGitWorktreeWithBranch(repoDir, "test-native-race-2", branchName)
	require.NoError(t, err)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs[0] = wt1.setupLocked()
	}()
	go func() {
		defer wg.Done()
		<-start
		errs[1] = wt2.setupLocked()
	}()
	close(start)
	wg.Wait()

	require.NoError(t, errs[0], "first concurrent native setup must not hard-fail on a lost branch-create race")
	require.NoError(t, errs[1], "second concurrent native setup must not hard-fail on a lost branch-create race")
	defer func() { _ = wt1.Cleanup() }()
	defer func() { _ = wt2.Cleanup() }()

	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	_, err = repo.Reference(plumbing.NewBranchReferenceName(branchName), false)
	require.NoError(t, err, "branch must exist once the race resolves")
}
