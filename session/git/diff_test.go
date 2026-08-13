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

// TestGitWorktreeDiff_SanitizesInvalidUTF8_When_DiffContainsNonUTF8Bytes is the regression
// test for the "[internal] marshal message: proto: field session.v1.DiffStats.content
// contains invalid UTF-8" crash: git diff output is raw bytes and can contain sequences
// that are not valid UTF-8 (e.g. a file written in a non-UTF-8 encoding). DiffStats.Content
// is later copied verbatim into a proto3 string field, which panics/errors at marshal time
// on invalid UTF-8, so Diff() must sanitize before storing.
func TestGitWorktreeDiff_SanitizesInvalidUTF8_When_DiffContainsNonUTF8Bytes(t *testing.T) {
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
