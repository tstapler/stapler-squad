package vc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// Detached HEAD is reported from the backend's ResolveRef, not a direct `git rev-parse HEAD`.
// Not parallel: it swaps the process-wide git backend.
func TestGitProviderGetBranch_DetachedHeadResolvesThroughBackend(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-b", "main")
	configureGitUser(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644))
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "init")
	gitIn(t, dir, "checkout", "--detach")
	const sha = "0123456789abcdef0123456789abcdef01234567"
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "", backend.ErrDetachedHead }
	rig.GoGit.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return sha, nil }
	provider, err := NewGitProvider(dir)
	require.NoError(t, err)

	branch, err := provider.GetBranch()

	require.NoError(t, err)
	assert.Equal(t, "(detached: "+truncateHash(sha)+")", branch)
	assert.True(t, strings.HasPrefix(branch, "(detached: "))
	rig.RequireOnlyGoGit(t, backend.OpResolveRef)
	call := rig.GoGit.CallsTo(backend.OpResolveRef)[0]
	assert.Equal(t, "HEAD", call.Arg)
	assert.Equal(t, backend.Local{Root: backend.RepoRoot(provider.repoRoot)}, call.Loc)
}
