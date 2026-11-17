package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// masterOriginClone builds an origin whose only branch is "master" (like kibitzer)
// and returns a clone of it.
func masterOriginClone(t *testing.T) (clone string) {
	t.Helper()
	origin := t.TempDir()
	runGit(t, origin, "init", "-b", "master")
	runGit(t, origin, "config", "user.email", "t@example.com")
	runGit(t, origin, "config", "user.name", "T")
	require.NoError(t, os.WriteFile(filepath.Join(origin, "a.txt"), []byte("a\n"), 0o644))
	runGit(t, origin, "add", "a.txt")
	runGit(t, origin, "commit", "-m", "init")
	clone = filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(clone), "clone", origin, clone)
	return clone
}

func TestResolveDefaultBranchName_should_ReturnMaster_When_RepoDefaultIsMaster(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "master", ResolveDefaultBranchName(masterOriginClone(t)))
}

func TestResolveDefaultBranchName_should_ReturnMain_When_RepoDefaultIsMain(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	assert.Equal(t, "main", ResolveDefaultBranchName(cloneTestRepo(t, origin)))
}

func TestResolveDefaultBranchName_should_FallBackToMain_When_PathIsNotARepo(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "main", ResolveDefaultBranchName(t.TempDir()))
}

// Regression for the wrong-branch failure: against a master-default repo the
// hard-coded "main" cannot resolve a ref; the resolved name verifies cleanly.
func TestIsCommitOnMain_should_Succeed_When_ResolvedDefaultBranchIsMaster(t *testing.T) {
	t.Parallel()
	clone := masterOriginClone(t)
	sha := strings.TrimSpace(runGit(t, clone, "rev-parse", "HEAD"))

	_, err := IsCommitOnMain(clone, "main", sha)
	require.Error(t, err, "hard-coded main has no ref in a master-default repo")

	onMain, err := IsCommitOnMain(clone, ResolveDefaultBranchName(clone), sha)
	require.NoError(t, err)
	assert.True(t, onMain)
}
