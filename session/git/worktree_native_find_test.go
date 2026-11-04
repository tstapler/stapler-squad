package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/internal/gittest"
)

// newNativeRemoveFixture creates a real native-created worktree for branchName, ready for
// removal tests and for tests of session/git code that consumes native worktree state.
func newNativeRemoveFixture(t *testing.T, branchName string) (repoPath, worktreePath string) {
	t.Helper()
	repoPath = gittest.SetupTestRepo(t)
	worktreePath = filepath.Join(t.TempDir(), branchName)
	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "native-remove-fixture-"+branchName, branchName, "")
	require.NoError(t, wt.nativeSetupNewWorktree())
	return repoPath, worktreePath
}

// TestNativeFindExistingWorktreeForBranch_FindsLiveWorktree_ReportsNotFoundForOtherBranch
// covers PR #730 Gate 2's noted gap: nativeFindExistingWorktreeForBranch (worktree.go),
// called from worktree.go's findOrCreateWorktree, had no direct test of its own.
func TestNativeFindExistingWorktreeForBranch_FindsLiveWorktree_ReportsNotFoundForOtherBranch(t *testing.T) {
	t.Parallel()
	branchName := "feature-native-find-existing"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	path, found := nativeFindExistingWorktreeForBranch(repoPath, branchName)
	require.True(t, found)
	assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(path))

	_, found = nativeFindExistingWorktreeForBranch(repoPath, "some-other-branch")
	assert.False(t, found, "a branch with no registered worktree must report not-found, not a stale match")
}

func TestFindWorktreeForBranch_PreservesRealConflictedRebase(t *testing.T) {
	t.Parallel()
	branch := "backlog/real-rebase"
	repoPath, worktreePath := newNativeRemoveFixture(t, branch)
	runGit(t, repoPath, "checkout", "-b", "upstream")
	file := filepath.Join(repoPath, "conflict.txt")
	require.NoError(t, os.WriteFile(file, []byte("upstream\n"), 0600))
	runGit(t, repoPath, "add", "conflict.txt")
	runGit(t, repoPath, "commit", "-m", "upstream change")
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "conflict.txt"), []byte("topic\n"), 0600))
	runGit(t, worktreePath, "add", "conflict.txt")
	runGit(t, worktreePath, "commit", "-m", "topic change")
	cmd := safeexec.CommandContext(t.Context(), "git", "-C", worktreePath, "rebase", "upstream")
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "rebase must stop at conflict: %s", output)
	index := runGit(t, worktreePath, "ls-files", "--unmerged")
	require.NotEmpty(t, index)
	conflict, err := os.ReadFile(filepath.Join(worktreePath, "conflict.txt"))
	require.NoError(t, err)
	for _, targetPath := range []string{worktreePath, filepath.Join(t.TempDir(), "replacement")} {
		wt := NewGitWorktreeFromStorageWithExecutor(repoPath, targetPath, "real-rebase", branch, "")
		require.NoError(t, wt.setupFromExistingBranch())
		assert.Equal(t, index, runGit(t, worktreePath, "ls-files", "--unmerged"))
		content, err := os.ReadFile(filepath.Join(worktreePath, "conflict.txt"))
		require.NoError(t, err)
		assert.Equal(t, conflict, content)
	}
	attached, err := NewGitWorktreeFromExisting(worktreePath, "attach-rebase")
	require.NoError(t, err)
	assert.Equal(t, branch, attached.GetBranchName())
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "conflict.txt"), []byte("resolved\n"), 0600))
	runGit(t, worktreePath, "add", "conflict.txt")
	runGit(t, worktreePath, "-c", "core.editor=true", "rebase", "--continue")
	assert.Equal(t, branch, strings.TrimSpace(runGit(t, worktreePath, "branch", "--show-current")))
}

func TestFindWorktreeForBranch_ResumesInterruptedRebase(t *testing.T) {
	t.Parallel()
	for _, stateDir := range []string{"rebase-merge", "rebase-apply"} {
		t.Run(stateDir, func(t *testing.T) {
			t.Parallel()
			branch := "backlog/interrupted-rebase"
			repoPath, worktreePath := newNativeRemoveFixture(t, branch)
			adminDir := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "--absolute-git-dir"))
			head := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "HEAD"))
			require.NoError(t, os.WriteFile(filepath.Join(adminDir, "HEAD"), []byte(head+"\n"), 0600))
			require.NoError(t, os.Mkdir(filepath.Join(adminDir, stateDir), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(adminDir, stateDir, "head-name"), []byte("refs/heads/"+branch+"\n"), 0600))
			marker := filepath.Join(worktreePath, "unfinished-work.txt")
			require.NoError(t, os.WriteFile(marker, []byte("preserve me\n"), 0600))

			path, found := nativeFindExistingWorktreeForBranch(repoPath, branch)
			require.True(t, found, "a detached rebase still reserves its original branch")
			assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(path))

			for _, targetPath := range []string{worktreePath, filepath.Join(t.TempDir(), "replacement")} {
				wt := NewGitWorktreeFromStorageWithExecutor(repoPath, targetPath, "rebase-recovery", branch, "")
				require.NoError(t, wt.setupFromExistingBranch())
				assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(wt.worktreePath))
			}
			content, err := os.ReadFile(marker)
			require.NoError(t, err)
			assert.Equal(t, "preserve me\n", string(content))
			_, err = os.Stat(filepath.Join(adminDir, stateDir, "head-name"))
			require.NoError(t, err, "recovery must preserve the interrupted rebase")
		})
	}
}
