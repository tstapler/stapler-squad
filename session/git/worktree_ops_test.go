package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
func TestSetup_SurfacesError_When_BranchRefIsMalformed(t *testing.T) {
	repoDir := setupTestRepo(t)

	branchName := "backlog/fix-typo-abc123"
	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-bad-ref", branchName)
	require.NoError(t, err)

	corruptPackedRefs(t, repoDir)

	err = wt.Setup()
	require.Error(t, err, "Setup() must surface a malformed ref as an error, not silently treat it as branch-absent")
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
// distinguish "never called" from "called but failed before writing."
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
	assert.Contains(t, err.Error(), "failed to check branch reference")

	_, statErr := os.Stat(wt.worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree must not have been created after a ref-check error")

	after, readErr := os.ReadFile(packedRefsPath)
	require.NoError(t, readErr)
	assert.Equal(t, before, after,
		"packed-refs must be untouched after a ref-check error (regression guard, see doc comment above for what this does and doesn't prove)")
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
