package native

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/internal/gittest"
)

func setupTestRepo(t *testing.T) string { return gittest.SetupTestRepo(t) }

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.RunGit(t, dir, args...)
}

func runRealGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.RunGit(t, dir, args...)
}

// testWorktree stands in for session/git's *GitWorktree in tests: it carries the inputs
// SetupNewWorktree needs and caches the resolved base SHA the way the wrapper does.
type testWorktree struct {
	repoPath, worktreePath, branchName, baseCommitSHA string
}

// newTestWorktree mirrors the argument order of git.NewGitWorktreeFromStorageWithExecutor;
// the session-name argument is unused here.
func newTestWorktree(repoPath, worktreePath, _, branchName, baseCommitSHA string) *testWorktree {
	return &testWorktree{repoPath, worktreePath, branchName, baseCommitSHA}
}

func (w *testWorktree) nativeSetupNewWorktree() error {
	sha, err := SetupNewWorktree(SetupParams{
		RepoPath:      w.repoPath,
		WorktreePath:  w.worktreePath,
		BranchName:    w.branchName,
		BaseCommitSHA: w.baseCommitSHA,
	})
	if sha != "" {
		w.baseCommitSHA = sha
	}
	return err
}

func (w *testWorktree) GetBaseCommitSHA() string { return w.baseCommitSHA }

func setupBenchRepo(b *testing.B) string { return gittest.SetupBenchRepo(b) }

// worktreeIsDirty is the go-git Worktree.Status() reference implementation the fast dirty
// check is compared against; it mirrors session/git's worktreeIsDirty/worktreeIsDirtyWithFS.
func worktreeIsDirty(path string) (bool, error) { return worktreeIsDirtyWithFS(path, nil) }

func worktreeIsDirtyWithFS(path string, cache *GitignoreFSCache) (bool, error) {
	repo, err := OpenRepo(path)
	if err != nil {
		return false, fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree at %s: %w", path, err)
	}
	worktree.Filesystem = NewCachedFilesystem(worktree.Filesystem, cache)
	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree status at %s: %w", path, err)
	}
	return !status.IsClean(), nil
}

// testMergeDeps wires the real git CLI fetch, a plain go-git HEAD read and a process-local
// mutex per repo path, standing in for session/git's production MergeDeps.
var testMergeDeps = MergeDeps{ //nolint:gochecknoglobals // immutable test function table
	WorktreeLock: testWorktreeLock,
	HeadSHA: func(path string) (string, error) {
		repo, err := OpenRepo(path)
		if err != nil {
			return "", err
		}
		ref, err := repo.Head()
		if err != nil {
			return "", err
		}
		return ref.Hash().String(), nil
	},
	Fetch: func(repoPath, branch string) error {
		cmd := safeexec.CommandContext(context.Background(), "git", "-C", repoPath, "fetch", "origin", "--", branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to fetch branch: %s", out)
		}
		return nil
	},
}

var testWorktreeLocks sync.Map //nolint:gochecknoglobals // per-repo test mutexes

func testWorktreeLock(repoPath string, fn func() error) error {
	mu, _ := testWorktreeLocks.LoadOrStore(repoPath, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	return fn()
}

func nativeMergeMainIntoWorktree(worktreePath, mainBranch string) (*MergeMainResult, error) {
	return mergeMainIntoWorktree(testMergeDeps, worktreePath, mainBranch)
}

func cloneTestRepo(t *testing.T, originDir string) string { return gittest.CloneTestRepo(t, originDir) }
