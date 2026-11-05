package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// runGitOrFail runs a git command in dir and fails the test on error, for building
// the small real repo fixtures below.
func runGitOrFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
}

func runGitOutputOrFail(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}

// cloneWithOrigin clones originDir into a fresh temp directory with a real "origin"
// remote pointing back at originDir, so git.EnsureBranchSyncedWithMain's fetch/merge/
// push calls have something real to talk to (unlike this file's other fixtures, which
// use a single standalone repo with no remote at all — fine for GetGitDiff, but not
// for exercising the BUG-044 drift precondition, which fetches before every check).
func cloneWithOrigin(t *testing.T, originDir string) string {
	t.Helper()
	parent := t.TempDir()
	cloneDir := filepath.Join(parent, "clone")
	runGitOrFail(t, parent, "clone", originDir, cloneDir)
	runGitOrFail(t, cloneDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, cloneDir, "config", "user.name", "Test")
	return cloneDir
}

func TestBackfillLockFilePath_SkipsInMemoryDSN(t *testing.T) {
	require.Empty(t, (&EntRepository{dbPath: ":memory:"}).backfillLockFilePath("x.lock"))
	require.Equal(t, filepath.Join("/data", "x.lock"), (&EntRepository{dbPath: "/data/sessions.db"}).backfillLockFilePath("x.lock"))
}
