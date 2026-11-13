package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitwt"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend (gitprovider), so none of them is parallel.

func TestGetMainRepoPath_RoutesCommonDirThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnCommonDir = func(backend.RepoLocation) (backend.CommonDir, error) { return "/repos/app/.git", nil }

	got, err := GetMainRepoPath("/repos/app-wt")

	require.NoError(t, err)
	assert.Equal(t, "/repos/app", got)
	rig.RequireOnlyGoGit(t, backend.OpCommonDir)
}

func TestGetMainRepoPath_WrapsBackendError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	boom := errors.New("boom")
	fake.OnCommonDir = func(backend.RepoLocation) (backend.CommonDir, error) { return "", boom }

	_, err := GetMainRepoPath("/nope")

	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "failed to get git common dir")
}

func TestDetectWorktree_ReadsRemoteURLThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnGetConfig = func(_ backend.RepoLocation, key backend.ConfigKey) (string, error) {
		assert.Equal(t, backend.ConfigKey("remote.origin.url"), key)
		return "https://github.com/acme/widgets.git", nil
	}
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))

	info, err := detectWorktreeUncached(repo)

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme/widgets.git", info.RemoteURL)
	assert.Equal(t, "acme", info.GitHubOwner)
	assert.Equal(t, "widgets", info.GitHubRepo)
	rig.RequireOnlyGoGit(t, backend.OpGetConfig)
}

func TestDetectWorktree_UnsetRemoteIsNotAnError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnGetConfig = func(_ backend.RepoLocation, key backend.ConfigKey) (string, error) {
		return "", backend.ErrConfigUnset
	}
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))

	info, err := detectWorktreeUncached(repo)

	require.NoError(t, err)
	assert.Empty(t, info.RemoteURL)
}

func TestGitWorktreeManager_GetCurrentCommitSHA_RoutesHeadThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnResolveRef = func(_ backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
		if ref == "HEAD" {
			return sessiontest.RefsTestSHA, nil
		}
		return "", backend.ErrRefNotFound
	}
	gm := &gitwt.Manager{}
	gm.SetWorktree(git.NewGitWorktreeFromStorage("/repo", "/repo-wt", "s", "b", ""))

	got, err := gm.GetCurrentCommitSHA()

	require.NoError(t, err)
	assert.Equal(t, sessiontest.RefsTestSHA, got)
	rig.RequireOnlyGoGit(t, backend.OpResolveRef)
	assert.Equal(t, backend.Local{Root: "/repo-wt"}, rig.GoGit.Calls()[0].Loc)
}

func TestGitWorktreeManager_GetCurrentCommitSHA_UnbornIsEmptyNotError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrUnborn
	}
	gm := &gitwt.Manager{}
	gm.SetWorktree(git.NewGitWorktreeFromStorage("/repo", "/repo-wt", "s", "b", ""))

	got, err := gm.GetCurrentCommitSHA()

	require.NoError(t, err)
	assert.Empty(t, got)
}

// The backend's CommonDir is a realpath; a symlinked caller path must still get its own
// spelling back (the old relative `--git-common-dir` output preserved it).
func TestGetMainRepoPath_KeepsSymlinkedSpelling(t *testing.T) {
	real := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(real, "sub"), 0o755))
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))
	realResolved, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnCommonDir = func(backend.RepoLocation) (backend.CommonDir, error) {
		return backend.CommonDir(filepath.Join(realResolved, ".git")), nil
	}

	got, err := GetMainRepoPath(link)
	require.NoError(t, err)
	assert.Equal(t, link, got)

	got, err = GetMainRepoPath(filepath.Join(link, "sub"))
	require.NoError(t, err)
	assert.Equal(t, link, got, "a subdirectory maps back to the symlinked root")
}
