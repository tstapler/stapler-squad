package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pointHeadAtMissingObject rewrites the current branch ref to a SHA with no backing
// object, so go-git's HEAD read yields a "not a real commit object" result on every
// retry while `git rev-parse HEAD` still reports the ref's value — the production
// failure getHeadCommitSHA's CLI fallback exists for.
func pointHeadAtMissingObject(t *testing.T, repo string) string {
	t.Helper()
	const missing = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	branch := strings.TrimSpace(runGit(t, repo, "symbolic-ref", "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", filepath.FromSlash(branch)), []byte(missing+"\n"), 0o644))
	return missing
}

func TestGetHeadCommitSHA_should_FallBackToCLI_When_GoGitResolvesMissingObject(t *testing.T) {
	t.Parallel()
	repo := setupTestRepo(t)
	missing := pointHeadAtMissingObject(t, repo)

	got, err := getHeadCommitSHA(repo)

	require.NoError(t, err)
	assert.Equal(t, missing, got, "the CLI fallback must return the ref's value, which go-git rejects")
	assert.Equal(t, strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD")), got)
}

func TestNativeMergeDeps_should_UseCLIFallback_When_GoGitHeadReadFails(t *testing.T) {
	t.Parallel()
	repo := setupTestRepo(t)
	missing := pointHeadAtMissingObject(t, repo)

	got, err := nativeMergeDeps.HeadSHA(repo)

	require.NoError(t, err)
	assert.Equal(t, missing, got)
}
