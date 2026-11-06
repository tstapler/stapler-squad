package git

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
