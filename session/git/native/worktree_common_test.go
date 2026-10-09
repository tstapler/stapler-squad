package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRealWorktreeFixture builds a real repo (setupTestRepo) plus a real, git-CLI-created
// linked worktree (via SetupNewWorktree) — a
// minimal local stand-in for plan.md's WorktreeAdminFixture (Epic 5.1), used here per
// Task 1.2.2c's "implemented early here as a minimal local helper if Epic 5.1 hasn't
// landed yet" instruction. Returns the main repo path, the worktree path, and the base
// commit SHA the worktree's branch was created at.
func newRealWorktreeFixture(t *testing.T) (repoPath, worktreePath, baseCommitSHA string) {
	t.Helper()
	repoPath = setupTestRepo(t)
	worktreePath = filepath.Join(t.TempDir(), "native-worktree-common-fixture")

	wt := newTestWorktree(repoPath, worktreePath, "native-worktree-common-fixture", "native-worktree-common-fixture", "")
	require.NoError(t, wt.nativeSetupNewWorktree())

	return repoPath, worktreePath, wt.GetBaseCommitSHA()
}

// TestOpenWorktreeRepo_LinkedWorktree_ResolvesCorrectHead covers Epic 1.2's Story 1.2.2
// first acceptance criterion: opening a linked worktree via openWorktreeRepo resolves
// HEAD to the correct, live commit — the bug EnableDotGitCommonDir's absence causes
// (util.go's getHeadCommitSHA doc comment).
func TestOpenWorktreeRepo_LinkedWorktree_ResolvesCorrectHead(t *testing.T) {
	_, worktreePath, baseCommitSHA := newRealWorktreeFixture(t)

	repo, err := openWorktreeRepo(worktreePath)
	require.NoError(t, err)

	head, err := repo.Head()
	require.NoError(t, err)

	assert.Equal(t, baseCommitSHA, head.Hash().String())
}

// TestResolveWorktreeIndexPath_LinkedWorktree covers Story 1.2.2's second acceptance
// criterion for a linked worktree: the returned path is "<admin dir>/index", read from
// the worktree's own `.git` redirect file.
func TestResolveWorktreeIndexPath_LinkedWorktree(t *testing.T) {
	repoPath, worktreePath, _ := newRealWorktreeFixture(t)

	got, err := ResolveWorktreeIndexPath(worktreePath)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	require.NoError(t, err)
	adminDir := string(content)
	// Strip the "gitdir: " prefix and trailing newline the same way the function under
	// test does, to derive the expected path independently of its internals.
	adminDir = adminDir[len("gitdir: "):]
	for len(adminDir) > 0 && (adminDir[len(adminDir)-1] == '\n' || adminDir[len(adminDir)-1] == '\r') {
		adminDir = adminDir[:len(adminDir)-1]
	}

	assert.Equal(t, filepath.Join(adminDir, "index"), got)
	assert.Contains(t, got, filepath.Join(repoPath, ".git", "worktrees"))
}

// TestResolveWorktreeIndexPath_CLICreatedWorktree resolves the index path of a worktree
// created by the real git CLI (not the native code under test) and checks it against
// `git rev-parse --git-path index`.
func TestResolveWorktreeIndexPath_CLICreatedWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)
	worktreePath := filepath.Join(t.TempDir(), "cli-created-wt")
	runGit(t, repoPath, "worktree", "add", "-b", "cli-created-wt", worktreePath)

	got, err := ResolveWorktreeIndexPath(worktreePath)
	require.NoError(t, err)

	want := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "--path-format=absolute", "--git-path", "index"))
	assert.Equal(t, CanonicalizeWorktreePath(want), CanonicalizeWorktreePath(got))
	assert.Contains(t, got, filepath.Join(".git", "worktrees"))
}

// TestResolveWorktreeIndexPath_MainWorktree covers Story 1.2.2's second acceptance
// criterion for the main working copy: `.git` is a real directory, so the index path is
// simply "<repo>/.git/index".
func TestResolveWorktreeIndexPath_MainWorktree(t *testing.T) {
	repoPath := setupTestRepo(t)

	got, err := ResolveWorktreeIndexPath(repoPath)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(repoPath, ".git", "index"), got)
}

// TestResolveWorktreeIndexPath_should_ReturnError_When_GitFileIsMalformed covers
// validation.md's P1 error-path row: a `.git` file present but whose content isn't a
// "gitdir: <path>" line must return an error, not a wrong path.
func TestResolveWorktreeIndexPath_should_ReturnError_When_GitFileIsMalformed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("not a gitdir line\n"), 0644))

	_, err := ResolveWorktreeIndexPath(dir)
	require.Error(t, err)
}
