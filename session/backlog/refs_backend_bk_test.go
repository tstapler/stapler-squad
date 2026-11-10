package backlog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

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
	rig.GoGit.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return sessiontest.RefsTestSHA, nil
	}

	got, err := ResolveBranchTip(context.Background(), "/repo", "backlog/x")

	require.NoError(t, err)
	assert.Equal(t, sessiontest.RefsTestSHA, got)
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
			return sessiontest.RefsTestSHA, nil
		}
		return "", backend.ErrNoMergeBase
	}

	got, err := RecoverBaseCommitSHA(context.Background(), "/repo", "feature")

	require.NoError(t, err)
	assert.Equal(t, sessiontest.RefsTestSHA, got)
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
