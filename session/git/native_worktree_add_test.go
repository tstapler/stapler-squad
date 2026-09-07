package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestAllocateAdminDirName_FreshName covers Epic 1.2's Story 1.2.1 first acceptance
// criterion: a name with no existing collision gets that exact name, no suffix.
func TestAllocateAdminDirName_FreshName(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()

	got, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(repoPath, ".git", "worktrees", "feature-x"), got)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// TestAllocateAdminDirName_CollidingName_GetsSuffix covers Story 1.2.1's second
// acceptance criterion: a colliding name gets the numeric suffix real git's own
// add_worktree would produce ("<name>1"), confirmed against builtin/worktree.c in
// Task 1.2.1a.
func TestAllocateAdminDirName_CollidingName_GetsSuffix(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()

	first, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)

	second, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Equal(t, filepath.Join(repoPath, ".git", "worktrees", "feature-x1"), second)
	info, statErr := os.Stat(second)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())

	third, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repoPath, ".git", "worktrees", "feature-x2"), third)
}

// TestAllocateAdminDirName_should_ReturnError_When_RetryCapExceeded covers
// validation.md's P1 error-path row: pre-creating "feature-x" and every numeric-suffix
// name up through the retry cap forces AllocateAdminDirName to return a bounded error
// instead of looping forever.
func TestAllocateAdminDirName_should_ReturnError_When_RetryCapExceeded(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")
	require.NoError(t, os.MkdirAll(worktreesDir, 0o777))

	require.NoError(t, os.Mkdir(filepath.Join(worktreesDir, "feature-x"), 0o777))
	for i := 1; i <= maxAllocateAdminDirNameRetries; i++ {
		name := fmt.Sprintf("feature-x%d", i)
		require.NoError(t, os.Mkdir(filepath.Join(worktreesDir, name), 0o777))
	}

	_, err := AllocateAdminDirName(repoPath, "feature-x")
	require.Error(t, err)
}

// TestAllocateAdminDirName_ConcurrentCallers_NeverCollide covers Task 1.2.1c's
// concurrency requirement: N goroutines racing AllocateAdminDirName with the same base
// name against one repo must all get distinct, existing paths — proof the os.Mkdir +
// EEXIST-retry loop is actually race-free, not just correct single-threaded.
func TestAllocateAdminDirName_ConcurrentCallers_NeverCollide(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()

	const goroutines = 20
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = AllocateAdminDirName(repoPath, "feature-x")
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, goroutines)
	for i := range goroutines {
		require.NoError(t, errs[i])
		require.False(t, seen[results[i]], "path %q returned to more than one caller", results[i])
		seen[results[i]] = true

		info, statErr := os.Stat(results[i])
		require.NoError(t, statErr)
		assert.True(t, info.IsDir())
	}
	assert.Len(t, seen, goroutines)
}

// newNativeAddTarget builds a real repo (setupTestRepo) plus a worktree path/branch that
// doesn't exist yet, for calling nativeSetupNewWorktree directly (Epic 2.1, Stories
// 2.1.1/2.1.2). The target directory's basename intentionally matches branchName,
// mirroring real git's own admin-dir-named-after-target-directory-basename behavior
// (confirmed against git 2.53.0's actual `git worktree add -b <branch> <path>` output —
// the admin dir takes <path>'s leaf component, not the branch name).
func newNativeAddTarget(t *testing.T, branchName string) (repoPath, worktreePath string) {
	t.Helper()
	repoPath = setupTestRepo(t)
	worktreePath = filepath.Join(t.TempDir(), branchName)
	return repoPath, worktreePath
}

// runRealGit shells to real git for differential comparison (Epic 2.1's acceptance
// criteria explicitly require this, not a skip), mirroring this package's existing
// safeexec.CommandContext usage in worktree_ops_test.go/worktree_git_test.go.
func runRealGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "real git command failed: %s", string(out))
	return string(out)
}

// TestNativeSetupNewWorktree_WritesRealGitCompatibleAdminFiles covers Story 2.1.1's first
// acceptance criterion: all five admin files exist with real-git-compatible content, and
// locked is absent on success.
func TestNativeSetupNewWorktree_WritesRealGitCompatibleAdminFiles(t *testing.T) {
	t.Parallel()
	branchName := "feature-x"
	repoPath, worktreePath := newNativeAddTarget(t, branchName)

	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "native-add-admin-files", branchName, "")
	require.NoError(t, wt.nativeSetupNewWorktree())

	adminDir := filepath.Join(repoPath, ".git", "worktrees", branchName)

	gitdirContent, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(worktreePath, ".git")+"\n", string(gitdirContent))

	commondirContent, err := os.ReadFile(filepath.Join(adminDir, "commondir"))
	require.NoError(t, err)
	assert.Equal(t, "../..\n", string(commondirContent))

	headContent, err := os.ReadFile(filepath.Join(adminDir, "HEAD"))
	require.NoError(t, err)
	assert.Equal(t, "ref: "+plumbing.NewBranchReferenceName(branchName).String()+"\n", string(headContent))

	_, err = os.Stat(filepath.Join(adminDir, "locked"))
	assert.True(t, os.IsNotExist(err), "locked marker must be removed on success")

	redirectContent, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	require.NoError(t, err)
	assert.Equal(t, "gitdir: "+adminDir+"\n", string(redirectContent))
}

// TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit covers
// Story 2.1.1's second acceptance criterion. Rather than injecting a fault into
// nativeSetupNewWorktree itself, this calls the same lower-level primitives
// (AllocateAdminDirName, AdminFileWriter) it uses and deliberately stops after gitdir —
// mirroring Task 1.1.1c's "call the lower-level steps directly" approach to simulating a
// crash without a real process kill.
//
// Deviation from plan.md's literal wording, verified directly against real git 2.53.0
// (not assumed from gitoxide#2959, whose reproduction never included a `locked` file):
// plan.md states this exact partial state — gitdir written, commondir not yet — should
// show as "prunable". It doesn't. Because LockedMarker is written *first*, per ADR-001's
// mandated order, `locked` already exists by the time `gitdir` is written, and real git
// classifies a locked entry as "locked", never "prunable" (confirmed by shelling to real
// `git worktree list --porcelain` against exactly this on-disk state) — matching the
// separately-stated rule for List's own prunability classification (Story 2.3.1: "a
// worktree with a LockedMarker present is never Prunable"). The two requirements
// (locked-first ordering, and this specific state reading as prunable) are mutually
// exclusive; ordering is the one ADR-001 pins down explicitly, so this test asserts what
// real git actually, verifiably does — recognizes the state as "locked", not corrupt —
// rather than a "prunable" reading plan.md never actually verified against real git for
// this order.
func TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit(t *testing.T) {
	t.Parallel()
	branchName := "feature-crash"
	repoPath, worktreePath := newNativeAddTarget(t, branchName)

	adminDir, err := AllocateAdminDirName(repoPath, filepath.Base(worktreePath))
	require.NoError(t, err)

	w := NewAdminFileWriter(adminDir)
	require.NoError(t, w.WriteFile("locked", []byte("initializing")))
	require.NoError(t, w.WriteFile("gitdir", []byte(filepath.Join(worktreePath, ".git")+"\n")))
	// Deliberately stop here — commondir/HEAD/WorktreeRedirectFile are never written.

	output := runRealGit(t, repoPath, "worktree", "list", "--porcelain")
	assert.Contains(t, output, "locked", "real git must recognize this partial state as locked (not prunable, not corrupt): %s", output)
	assert.NotContains(t, output, "prunable", "a LockedMarker present must take priority over prunable classification: %s", output)
}

// TestNativeSetupNewWorktree_CheckoutMatchesRealGit covers Story 2.1.2's acceptance
// criterion: the natively-checked-out worktree's file contents and status match what
// real git would report for the same commit.
func TestNativeSetupNewWorktree_CheckoutMatchesRealGit(t *testing.T) {
	t.Parallel()
	branchName := "feature-checkout"
	repoPath, worktreePath := newNativeAddTarget(t, branchName)

	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "native-add-checkout", branchName, "")
	require.NoError(t, wt.nativeSetupNewWorktree())

	statusOutput := runRealGit(t, worktreePath, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(statusOutput), "worktree must be clean per real git status")

	showOutput := runRealGit(t, repoPath, "show", "--stat", "--format=", wt.GetBaseCommitSHA())
	for _, line := range strings.Split(strings.TrimSpace(showOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "|") {
			continue
		}
		fileName := strings.TrimSpace(strings.SplitN(line, "|", 2)[0])
		_, statErr := os.Stat(filepath.Join(worktreePath, fileName))
		assert.NoError(t, statErr, "file %q from `git show --stat` must exist in the native checkout", fileName)
	}
}

// TestNativeSetupNewWorktree_should_ReturnError_When_BaseCommitSHADoesNotExist covers
// validation.md's P2 error-path row. Confirmed against real git 2.53.0 that `git worktree
// add -b <branch> <path> <bad-sha>` validates the commit-ish before creating any admin
// dir at all (fatal: invalid reference, no .git/worktrees/ entry created) — this test
// asserts the native path matches that: no admin dir exists after the error, which is a
// strictly stronger guarantee than "nothing is cleaned up" (there is nothing to clean up).
func TestNativeSetupNewWorktree_should_ReturnError_When_BaseCommitSHADoesNotExist(t *testing.T) {
	t.Parallel()
	branchName := "feature-bad-sha"
	repoPath, worktreePath := newNativeAddTarget(t, branchName)

	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "native-add-bad-sha", branchName, strings.Repeat("a", 40))
	err := wt.nativeSetupNewWorktree()
	require.Error(t, err)

	adminDir := filepath.Join(repoPath, ".git", "worktrees", branchName)
	_, statErr := os.Stat(adminDir)
	assert.True(t, os.IsNotExist(statErr), "no admin dir should be created before the base commit is validated")
}

// TestNativeSetupNewWorktree_should_ReturnError_When_BranchNameAlreadyCheckedOutElsewhere
// covers validation.md's P2 error-path row: go-git's Checkout(Create:true) refuses to
// create a branch ref that already exists (git.ErrBranchExists) — the native path's
// equivalent of real git's "already used by worktree" refusal — and the error must
// propagate with locked left in place (Task 2.1.1d).
func TestNativeSetupNewWorktree_should_ReturnError_When_BranchNameAlreadyCheckedOutElsewhere(t *testing.T) {
	t.Parallel()
	branchName := "feature-taken"
	repoPath, worktreePath := newNativeAddTarget(t, branchName)
	runRealGit(t, repoPath, "branch", branchName)

	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "native-add-branch-taken", branchName, "")
	err := wt.nativeSetupNewWorktree()
	require.Error(t, err)

	adminDir := filepath.Join(repoPath, ".git", "worktrees", branchName)
	_, statErr := os.Stat(filepath.Join(adminDir, "locked"))
	assert.NoError(t, statErr, "locked marker must remain in place after a failed Add, matching real git")
}

// TestNativeUnlockWorktree_RemovesLockedMarker covers Story 2.1.4's first acceptance
// criterion.
func TestNativeUnlockWorktree_RemovesLockedMarker(t *testing.T) {
	t.Parallel()
	repoPath, worktreePath, _ := newRealWorktreeFixture(t)

	indexPath, err := resolveWorktreeIndexPath(worktreePath)
	require.NoError(t, err)
	adminDir := filepath.Dir(indexPath)
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "locked"), []byte("initializing"), 0644))

	require.NoError(t, nativeUnlockWorktree(repoPath, worktreePath))

	_, statErr := os.Stat(filepath.Join(adminDir, "locked"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestNativeUnlockWorktree_NoMarkerPresent_NoOp covers Story 2.1.4's second acceptance
// criterion: unlocking an already-unlocked worktree is a no-op, not an error.
func TestNativeUnlockWorktree_NoMarkerPresent_NoOp(t *testing.T) {
	t.Parallel()
	repoPath, worktreePath, _ := newRealWorktreeFixture(t)

	indexPath, err := resolveWorktreeIndexPath(worktreePath)
	require.NoError(t, err)
	adminDir := filepath.Dir(indexPath)
	_, statErr := os.Stat(filepath.Join(adminDir, "locked"))
	require.True(t, os.IsNotExist(statErr), "fixture must not already have a locked marker")

	require.NoError(t, nativeUnlockWorktree(repoPath, worktreePath))

	_, err = os.Stat(filepath.Join(adminDir, "gitdir"))
	assert.NoError(t, err, "admin dir must otherwise be unchanged")
}
