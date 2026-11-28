package workspacerpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

func listBranchesVia(t *testing.T, includeRemote bool) (*connect.Response[sessionv1.ListBranchesResponse], error) {
	t.Helper()
	home := t.TempDir()
	svctest.WithFakeHomePath(t, home)
	repo := filepath.Join(home, "repo")
	require.NoError(t, os.Mkdir(repo, 0o755))
	return NewWorkspaceService(nil, nil).ListBranches(context.Background(),
		connect.NewRequest(&sessionv1.ListBranchesRequest{RepoPath: repo, IncludeRemote: includeRemote}))
}

func TestWorkspaceService_ListBranches_RoutesListRefsThroughBackendWithShortNames(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnListRefs = func(_ backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error) {
		assert.Equal(t, backend.RefPattern("refs/"), req.Pattern)
		return []backend.RefName{
			"refs/heads/main", "refs/heads/feat/x", "refs/remotes/origin/main", "refs/remotes/origin/HEAD", "refs/tags/v1",
		}, nil
	}

	resp, err := listBranchesVia(t, true)

	require.NoError(t, err)
	assert.Equal(t, []string{"main", "feat/x", "origin/main", "origin", "v1"}, resp.Msg.Branches)
	assert.False(t, resp.Msg.Truncated)
	rig.RequireOnlyGoGit(t, backend.OpListRefs)
}

func TestWorkspaceService_ListBranches_LocalOnlyUsesHeadsPattern(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnListRefs = func(_ backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error) {
		assert.Equal(t, backend.RefPattern("refs/heads"), req.Pattern)
		return []backend.RefName{"refs/heads/main"}, nil
	}

	resp, err := listBranchesVia(t, false)

	require.NoError(t, err)
	assert.Equal(t, []string{"main"}, resp.Msg.Branches)
}

func TestWorkspaceService_ListBranches_GitFailureIsAnEmptyListNotAnError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnListRefs = func(backend.RepoLocation, backend.ListRefsRequest) ([]backend.RefName, error) {
		return nil, backend.ErrNotARepo
	}

	resp, err := listBranchesVia(t, false)

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Branches)
	assert.False(t, resp.Msg.Truncated)
}

func TestShortRefName_MatchesGitForTheKindsABranchPickerLists(t *testing.T) {
	for full, want := range map[string]string{
		"refs/heads/main":          "main",
		"refs/heads/a/b":           "a/b",
		"refs/remotes/origin/main": "origin/main",
		"refs/remotes/origin/HEAD": "origin",
		"refs/tags/v1":             "v1",
		"refs/stash":               "stash",
	} {
		assert.Equal(t, want, shortRefName(full), full)
	}
}
