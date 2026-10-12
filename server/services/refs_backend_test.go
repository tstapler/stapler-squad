package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend (gitprovider), so none of them is parallel.

const refsSvcSHA = "0123456789abcdef0123456789abcdef01234567"

func TestSearchService_CachedBranch_RoutesCurrentBranchThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "feat/x", nil }

	got := NewSearchService(nil, nil, 0).cachedBranch(context.Background(), "/proj")

	assert.Equal(t, "feat/x", got)
	rig.RequireOnlyGoGit(t, backend.OpCurrentBranch)
	assert.Equal(t, backend.Local{Root: "/proj"}, rig.GoGit.Calls()[0].Loc)
}

func TestSearchService_CachedBranch_DetachedUnbornAndErrorsShowNoBranch(t *testing.T) {
	for name, err := range map[string]error{
		"detached": backend.ErrDetachedHead,
		"unborn":   backend.ErrUnborn,
		"failure":  errors.New("not a repo"),
	} {
		t.Run(name, func(t *testing.T) {
			fake := gitfake.New("backend")
			gitfake.Use(t, fake)
			fake.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "", err }

			assert.Empty(t, NewSearchService(nil, nil, 0).cachedBranch(context.Background(), "/proj"))
		})
	}
}

func TestCheckBranchDivergence_RoutesCountCommitsThroughBackend(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		err   error
		want  bool
	}{
		{"ahead", 3, nil, true},
		{"level", 0, nil, false},
		{"no origin/HEAD", 0, backend.ErrRefNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := gitfake.NewRig(t)
			rig.Install(t)
			rig.GoGit.OnCountCommits = func(_ backend.RepoLocation, rng backend.RangeSpec) (int, error) {
				assert.Equal(t, backend.RangeSpec{Exclude: "origin/HEAD", Include: "HEAD"}, rng)
				return tc.count, tc.err
			}

			assert.Equal(t, tc.want, checkBranchDivergence("/work"))
			rig.RequireOnlyGoGit(t, backend.OpCountCommits)
		})
	}
}

func TestWorktreeHeadSHA_RoutesResolveRefThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return refsSvcSHA, nil }

	assert.Equal(t, refsSvcSHA, worktreeHeadSHA(context.Background(), "/wt"))
	rig.RequireOnlyGoGit(t, backend.OpResolveRef)
	assert.Equal(t, "HEAD", rig.GoGit.Calls()[0].Arg)
}

func TestWorktreeHeadSHA_EmptyOnError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return "", backend.ErrUnborn }

	assert.Empty(t, worktreeHeadSHA(context.Background(), "/wt"))
}

func TestGitLogSummary_RoutesLogThroughBackendAndFormatsOneline(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnLog = func(_ backend.RepoLocation, req backend.LogRequest) ([]backend.LogEntry, error) {
		assert.Equal(t, gitLogOneLineCommits, req.Limit)
		assert.Equal(t, backend.RangeSpec{Exclude: "base123", Include: "feature"}, req.Range)
		return []backend.LogEntry{{SHA: refsSvcSHA, Subject: "second"}, {SHA: "fedcba9876543210fedcba9876543210fedcba98", Subject: "first"}}, nil
	}

	got := gitLogSummary(context.Background(), "/wt", "base123", "feature")

	assert.Equal(t, "0123456 second\nfedcba9 first", got)
	rig.RequireOnlyGoGit(t, backend.OpLog)
}

func TestGitLogSummary_EmptyOnErrorOrMissingInput(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnLog = func(backend.RepoLocation, backend.LogRequest) ([]backend.LogEntry, error) {
		return nil, errors.New("bad range")
	}

	assert.Empty(t, gitLogSummary(context.Background(), "/wt", "base", "head"))
	assert.Empty(t, gitLogSummary(context.Background(), "", "base", "head"))
	assert.Empty(t, fake.Calls()[1:], "missing input must not reach git")
}

func listBranchesVia(t *testing.T, includeRemote bool) (*connect.Response[sessionv1.ListBranchesResponse], error) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
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
