package backlog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

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
