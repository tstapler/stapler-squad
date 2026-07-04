package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsureDirectorySessionPath_CreatesAndGitInitsMissingDirectory verifies that
// a not-yet-existing path is created and git-initialized, matching the same
// behavior the inline SessionTypeDirectory/CreateIfMissing branch had before this
// logic was extracted into an exported, reusable function.
func TestEnsureDirectorySessionPath_CreatesAndGitInitsMissingDirectory(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "new-repo")

	err := EnsureDirectorySessionPath(target)
	require.NoError(t, err)

	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	require.True(t, info.IsDir())

	_, gitErr := os.Stat(filepath.Join(target, ".git"))
	require.NoError(t, gitErr, "expected target to be git-initialized")
}

// TestEnsureDirectorySessionPath_NoopWhenAlreadyExists verifies calling it on an
// already-existing directory (already a git repo, from a prior call) is a no-op
// that doesn't error — this is what makes it safe to call once before spawning a
// session and rely on the spawn's own CreateIfMissing check finding the directory
// already present.
func TestEnsureDirectorySessionPath_NoopWhenAlreadyExists(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "existing-repo")

	require.NoError(t, EnsureDirectorySessionPath(target))
	// Second call on the same, now-existing (and git-initialized) path must also succeed.
	require.NoError(t, EnsureDirectorySessionPath(target))

	_, gitErr := os.Stat(filepath.Join(target, ".git"))
	require.NoError(t, gitErr)
}

// TestEnsureDirectorySessionPath_NoopWhenPathIsAnExistingFile documents the actual
// (pre-existing, unchanged-by-extraction) behavior: the function only acts when
// os.Stat reports the path does not exist at all — an existing plain file is left
// untouched rather than triggering git.InitializeProjectDirectory's own
// file-collision error, because the exists-check short-circuits first. This
// matches the original inline SessionTypeDirectory/CreateIfMissing logic exactly.
func TestEnsureDirectorySessionPath_NoopWhenPathIsAnExistingFile(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "not-a-dir")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

	require.NoError(t, EnsureDirectorySessionPath(target))

	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	require.False(t, info.IsDir(), "the existing file must be left untouched, not replaced with a directory")
}
