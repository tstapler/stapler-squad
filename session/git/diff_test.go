package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestGitWorktreeDiff_ModifiedTrackedFile_ReturnsAddedAndRemovedLines verifies that
// changing the content of a file already committed in the base commit shows up as
// both added and removed lines (a content replacement), exercising the
// baseFiles-and-disk-both-exist-but-differ path in diffOneFile.
func TestGitWorktreeDiff_ModifiedTrackedFile_ReturnsAddedAndRemovedLines(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-modified")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	readmePath := filepath.Join(wt.GetWorktreePath(), "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# Test\n\nModified content.\n"), 0644))

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.Greater(t, stats.Added, 0, "modifying a tracked file must produce added lines")
	assert.Greater(t, stats.Removed, 0, "modifying a tracked file must produce removed lines")
	assert.Contains(t, stats.Content, "README.md")
}

// TestGitWorktreeDiff_StaleBaseCommitSHA_ClearsCacheAndReturnsEmptyStats verifies that
// a cached baseCommitSHA which no longer resolves to a real commit object (e.g. after
// a rebase or gc rewrote history) is cleared so the next call re-resolves it, rather
// than surfacing as an error — the go-git equivalent of the old subprocess
// implementation's "unable to read" stderr match.
func TestGitWorktreeDiff_StaleBaseCommitSHA_ClearsCacheAndReturnsEmptyStats(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-stale-base")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	require.NotEmpty(t, wt.baseCommitSHA)
	// A syntactically valid but nonexistent commit hash — never written to the
	// object database by anything in this test.
	wt.baseCommitSHA = "0123456789abcdef0123456789abcdef01234567"

	stats := wt.Diff()
	require.NoError(t, stats.Error, "an unreadable cached base SHA must not surface as an error")
	assert.True(t, stats.IsEmpty())
	assert.Empty(t, wt.baseCommitSHA, "Diff() must clear the stale cached SHA so the next call re-resolves it")
}

// TestGitWorktreeDiff_NotAGitRepo_ReturnsEmptyStatsNoError verifies the common,
// expected case of a session directory with no .git at all: Diff() must return
// empty stats without an error (no log/error-spam for a routine non-git session).
func TestGitWorktreeDiff_NotAGitRepo_ReturnsEmptyStatsNoError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	wt := NewGitWorktreeFromStorage(dir, dir, "test-not-a-repo", "main", "")
	require.NotNil(t, wt)

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.True(t, stats.IsEmpty())
}

// TestGitWorktreeDiff_UnopenableGitDir_ReturnsEmptyStatsNoError covers the case where
// a ".git" entry exists (so the cheap os.Stat existence check passes) but doesn't open
// as a usable repository — treated the same as "not a git repository" rather than
// surfaced as an error, matching every "not a git repository" stderr-matching branch
// the old subprocess implementation had.
func TestGitWorktreeDiff_UnopenableGitDir_ReturnsEmptyStatsNoError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("not a real gitdir pointer"), 0644))

	wt := NewGitWorktreeFromStorage(dir, dir, "test-unopenable-git", "main", "deadbeef")
	require.NotNil(t, wt)

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.True(t, stats.IsEmpty())
}

// TestGitWorktreeDiff_SanitizesInvalidUTF8_When_DiffContainsNonUTF8Bytes is the regression
// test for the "[internal] marshal message: proto: field session.v1.DiffStats.content
// contains invalid UTF-8" crash: git diff output is raw bytes and can contain sequences
// that are not valid UTF-8 (e.g. a file written in a non-UTF-8 encoding). DiffStats.Content
// is later copied verbatim into a proto3 string field, which panics/errors at marshal time
// on invalid UTF-8, so Diff() must sanitize before storing.
func TestGitWorktreeDiff_SanitizesInvalidUTF8_When_DiffContainsNonUTF8Bytes(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-invalid-utf8-diff")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// Latin-1 encoded byte 0xE9 ("é") is not valid UTF-8 on its own.
	invalidUTF8 := []byte("some non-utf8 content: \xe9\n")
	require.NoError(t, os.WriteFile(filepath.Join(wt.worktreePath, "latin1.txt"), invalidUTF8, 0644))

	cmd := safeexec.CommandContext(context.Background(), "git", "-C", wt.worktreePath, "add", "latin1.txt")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add failed: %s", out)

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.True(t, utf8.ValidString(stats.Content), "Diff().Content must always be valid UTF-8")
	assert.NotEmpty(t, stats.Content)
}
