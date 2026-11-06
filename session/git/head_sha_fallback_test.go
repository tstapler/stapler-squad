package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

const missingSHA = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

// pointHeadAtMissingObject rewrites the current branch ref to a SHA with no backing
// object, so go-git's HEAD read yields a "not a real commit object" result on every
// retry — the production failure getHeadCommitSHA's backend fallback exists for.
func pointHeadAtMissingObject(t *testing.T, repo string) {
	t.Helper()
	branch := strings.TrimSpace(runGit(t, repo, "symbolic-ref", "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", filepath.FromSlash(branch)), []byte(missingSHA+"\n"), 0o644))
}

// The fallback now asks the backend for a verified commit (ResolveRef), where it used to take
// whatever text `git rev-parse HEAD` printed, so a ref naming an absent object is an error
// rather than a returned phantom SHA.
func TestGetHeadCommitSHA_should_ReturnTheBackendsSHA_When_GoGitResolvesMissingObject(t *testing.T) {
	t.Parallel()
	repo := setupTestRepo(t)
	pointHeadAtMissingObject(t, repo)
	const want = "0123456789abcdef0123456789abcdef01234567"
	b := gitfake.New("backend")
	b.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return want, nil }

	got, err := headCommitSHA(b, repo)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	calls := b.CallsTo(backend.OpResolveRef)
	require.Len(t, calls, 1)
	assert.Equal(t, "HEAD", calls[0].Arg)
	assert.Equal(t, backend.Local{Root: backend.RepoRoot(repo)}, calls[0].Loc)
}

func TestGetHeadCommitSHA_should_Error_When_GoGitAndBackendBothRejectTheHeadObject(t *testing.T) {
	t.Parallel()
	repo := setupTestRepo(t)
	pointHeadAtMissingObject(t, repo)

	_, err := getHeadCommitSHA(repo)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read HEAD at "+repo)
}

func TestNativeMergeDeps_should_UseBackendFallback_When_GoGitHeadReadFails(t *testing.T) {
	t.Parallel()
	repo := setupTestRepo(t)
	pointHeadAtMissingObject(t, repo)

	_, err := nativeMergeDeps.HeadSHA(repo)

	require.Error(t, err, "HeadSHA is getHeadCommitSHA: the backend fallback verifies the commit exists")
}
