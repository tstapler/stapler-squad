package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend (gitprovider), so none of them is parallel.

const refsTestSHA = "0123456789abcdef0123456789abcdef01234567"

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
			return refsTestSHA, nil
		}
		return "", backend.ErrRefNotFound
	}
	gm := &GitWorktreeManager{}
	gm.SetWorktree(git.NewGitWorktreeFromStorage("/repo", "/repo-wt", "s", "b", ""))

	got, err := gm.GetCurrentCommitSHA()

	require.NoError(t, err)
	assert.Equal(t, refsTestSHA, got)
	rig.RequireOnlyGoGit(t, backend.OpResolveRef)
	assert.Equal(t, backend.Local{Root: "/repo-wt"}, rig.GoGit.Calls()[0].Loc)
}

func TestGitWorktreeManager_GetCurrentCommitSHA_UnbornIsEmptyNotError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrUnborn
	}
	gm := &GitWorktreeManager{}
	gm.SetWorktree(git.NewGitWorktreeFromStorage("/repo", "/repo-wt", "s", "b", ""))

	got, err := gm.GetCurrentCommitSHA()

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAddWorktreeExcludes_ResolvesCommonDirThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	common := t.TempDir()
	rig.GoGit.OnCommonDir = func(backend.RepoLocation) (backend.CommonDir, error) { return backend.CommonDir(common), nil }

	addWorktreeExcludes("/some/worktree")

	rig.RequireOnlyGoGit(t, backend.OpCommonDir)
	data, err := os.ReadFile(filepath.Join(common, "info", "exclude"))
	require.NoError(t, err)
	assert.True(t, strings.TrimSpace(string(data)) != "", "exclude file must be written under the common dir the backend returned")
}

func TestResolveBranchTip_RoutesResolveRefThroughBackendInRepoPath(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return refsTestSHA, nil }

	got, err := ResolveBranchTip(context.Background(), "/repo", "backlog/x")

	require.NoError(t, err)
	assert.Equal(t, refsTestSHA, got)
	rig.RequireOnlyGoGit(t, backend.OpResolveRef)
	call := rig.GoGit.Calls()[0]
	assert.Equal(t, "backlog/x", call.Arg)
	assert.Equal(t, backend.Local{Root: "/repo"}, call.Loc, "the lookup must name the repo explicitly, not inherit the process CWD")
}

func TestResolveBranchTip_AbsentBranchIsAnError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrRefNotFound
	}

	_, err := ResolveBranchTip(context.Background(), "/repo", "gone")

	require.ErrorIs(t, err, backend.ErrRefNotFound)
}

func TestRecoverBaseCommitSHA_RoutesMergeBaseThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnMergeBase = func(_ backend.RepoLocation, req backend.MergeBaseRequest) (backend.CommitSHA, error) {
		if req.Right == "master" {
			assert.Equal(t, backend.RefName("feature"), req.Left, "explicit branch ref, never ambient HEAD")
			return refsTestSHA, nil
		}
		return "", backend.ErrNoMergeBase
	}

	got, err := RecoverBaseCommitSHA(context.Background(), "/repo", "feature")

	require.NoError(t, err)
	assert.Equal(t, refsTestSHA, got)
	rig.RequireOnlyGoGit(t, backend.OpMergeBase)
	assert.Len(t, rig.GoGit.CallsTo(backend.OpMergeBase), 2, "main fails, master succeeds")
}

func TestRecoverBaseCommitSHA_ErrorsWhenNoCandidateResolves(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnMergeBase = func(backend.RepoLocation, backend.MergeBaseRequest) (backend.CommitSHA, error) {
		return "", backend.ErrNoMergeBase
	}

	_, err := RecoverBaseCommitSHA(context.Background(), "/repo", "feature")

	require.ErrorIs(t, err, backend.ErrNoMergeBase)
	assert.Contains(t, err.Error(), "no merge-base found for feature")
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
