package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreviewWorktreePath_ReturnsPrefixWithoutRandomSuffix verifies the preview matches
// the deterministic prefix NewGitWorktreeWithBranchAndExecutor computes, without the
// non-deterministic "_<random-suffix>" it appends at actual creation time.
func TestPreviewWorktreePath_ReturnsPrefixWithoutRandomSuffix(t *testing.T) {
	repoDir := setupTestRepo(t)

	path, err := PreviewWorktreePath(repoDir, "My Feature Branch")
	require.NoError(t, err)

	worktreeDir, err := getWorktreeDirectory()
	require.NoError(t, err)

	expectedPrefix := filepath.Join(worktreeDir, sanitizeBranchName("My Feature Branch"))
	assert.Equal(t, expectedPrefix, path)
}

// TestPreviewWorktreePath_ErrorsOnNonGitRepo verifies preview fails fast (no filesystem
// mutation, no git subprocess calls) when repoPath isn't inside a git repository.
func TestPreviewWorktreePath_ErrorsOnNonGitRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := PreviewWorktreePath(dir, "some-session")
	assert.Error(t, err)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "PreviewWorktreePath must not create a directory or .git when repoPath is not a git repo")
}

// TestPreviewWorktreePath_DoesNotCreateRepoWhenPathIsMissing guards against a preview
// (called on every Omnibar keystroke) silently materializing a real git repository on
// disk — the mutating fallback findGitRepoRoot uses for the actual creation path must
// never be reachable from PreviewWorktreePath.
func TestPreviewWorktreePath_DoesNotCreateRepoWhenPathIsMissing(t *testing.T) {
	parent := t.TempDir()
	missingPath := filepath.Join(parent, "does-not-exist-yet")

	_, err := PreviewWorktreePath(missingPath, "some-session")
	assert.Error(t, err)

	_, statErr := os.Stat(missingPath)
	assert.True(t, os.IsNotExist(statErr), "PreviewWorktreePath must not create %s", missingPath)
}

// TestPreviewWorktreePath_SanitizesSessionNameConsistently verifies the preview applies
// the same sanitization NewGitWorktreeWithBranchAndExecutor uses, so the prefix shown to
// the user matches what actual creation would produce.
func TestPreviewWorktreePath_SanitizesSessionNameConsistently(t *testing.T) {
	repoDir := setupTestRepo(t)

	path, err := PreviewWorktreePath(repoDir, "Weird!!! Name///Here")
	require.NoError(t, err)

	worktreeDir, err := getWorktreeDirectory()
	require.NoError(t, err)

	expectedPrefix := filepath.Join(worktreeDir, sanitizeBranchName("Weird!!! Name///Here"))
	assert.Equal(t, expectedPrefix, path)
}
