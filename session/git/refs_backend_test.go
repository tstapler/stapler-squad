package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// fakeRemoteRunner is a remote tmux.CommandRunner that fails the test if git is run on it
// directly: ref reads must reach it only through the backend.
type fakeRemoteRunner struct{ t *testing.T }

func (r fakeRemoteRunner) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	r.t.Helper()
	return nil, fmt.Errorf("fakeRemoteRunner: unexpected direct run of %s %v", name, args)
}

func (fakeRemoteRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return nil, nil, nil, errors.New("not supported")
}

func (fakeRemoteRunner) IsRemote() bool { return true }

const sha1Hex = "0123456789abcdef0123456789abcdef01234567"

func TestMainRepoPathForWorktree_RoutesCommonDirThroughBackend(t *testing.T) {
	t.Parallel()
	rig := gitfake.NewRig(t)
	rig.GoGit.OnCommonDir = func(backend.RepoLocation) (backend.CommonDir, error) { return "/repos/app/.git", nil }

	got, err := mainRepoPathForWorktree(rig.Router, "/repos/app-wt")

	require.NoError(t, err)
	assert.Equal(t, "/repos/app", got)
	rig.RequireOnlyGoGit(t, backend.OpCommonDir)
}

func TestMainRepoPathForWorktree_WrapsBackendError(t *testing.T) {
	t.Parallel()
	b := gitfake.New("backend")
	boom := errors.New("boom")
	b.OnCommonDir = func(backend.RepoLocation) (backend.CommonDir, error) { return "", boom }

	_, err := mainRepoPathForWorktree(b, "/wt")

	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "failed to resolve git-common-dir for worktree /wt")
}

func TestRepoHasAnyRefViaBackend_RoutesListRefsThroughBackendWithLimitOne(t *testing.T) {
	t.Parallel()
	rig := gitfake.NewRig(t)
	rig.GoGit.OnListRefs = func(_ backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error) {
		assert.Equal(t, 1, req.Limit)
		return []backend.RefName{"refs/heads/main"}, nil
	}

	has, err := repoHasAnyRefViaBackend(rig.Router, "/repo")

	require.NoError(t, err)
	assert.True(t, has)
	rig.RequireOnlyGoGit(t, backend.OpListRefs)
}

func TestRepoHasAnyRefViaBackend_FalseWhenNoRefs(t *testing.T) {
	t.Parallel()
	b := gitfake.New("backend")
	b.OnListRefs = func(backend.RepoLocation, backend.ListRefsRequest) ([]backend.RefName, error) { return nil, nil }

	has, err := repoHasAnyRefViaBackend(b, "/repo")

	require.NoError(t, err)
	assert.False(t, has)
}

func TestHeadMergeBase_LocalReachesGoGitRemoteNeverDoes(t *testing.T) {
	t.Parallel()
	t.Run("local", func(t *testing.T) {
		rig := gitfake.NewRig(t)
		rig.GoGit.OnMergeBase = func(_ backend.RepoLocation, req backend.MergeBaseRequest) (backend.CommitSHA, error) {
			assert.Equal(t, backend.MergeBaseRequest{Left: "HEAD", Right: "main"}, req)
			return sha1Hex, nil
		}
		g := &GitWorktree{worktreePath: "/wt", refBackend: rig.Router}

		assert.Equal(t, sha1Hex, g.headMergeBase("main"))
		rig.RequireOnlyGoGit(t, backend.OpMergeBase)
		assert.Equal(t, backend.Local{Root: "/wt"}, rig.GoGit.Calls()[0].Loc, "HEAD must resolve in the worktree, not the shared repo")
	})
	t.Run("remote", func(t *testing.T) {
		rig := gitfake.NewRig(t)
		rig.CLI.OnMergeBase = func(backend.RepoLocation, backend.MergeBaseRequest) (backend.CommitSHA, error) { return sha1Hex, nil }
		runner := fakeRemoteRunner{t}
		g := &GitWorktree{worktreePath: "/wt", runner: runner, refBackend: rig.Router}

		assert.Equal(t, sha1Hex, g.headMergeBase("main"))
		rig.RequireOnlyCLIRemote(t, backend.OpMergeBase)
		assert.Equal(t, runner, rig.CLI.Calls()[0].Loc.(backend.Remote).Runner, "the worktree's own runner must reach the backend")
	})
}

func TestHeadMergeBase_EmptyWhenBackendErrors(t *testing.T) {
	t.Parallel()
	b := gitfake.New("backend")
	b.OnMergeBase = func(backend.RepoLocation, backend.MergeBaseRequest) (backend.CommitSHA, error) {
		return "", backend.ErrNoMergeBase
	}
	g := &GitWorktree{worktreePath: "/wt", refBackend: b}

	assert.Empty(t, g.headMergeBase("main"))
	assert.Empty(t, g.resolveBaseCommitSHA())
	assert.Len(t, b.CallsTo(backend.OpMergeBase), 1+len(CandidateDefaultBranches), "every default-branch candidate is tried")
}

func TestResolveRemoteWorktreeBaseCommit_ReadsRefsOnRemoteNeverGoGit(t *testing.T) {
	t.Parallel()
	rig := gitfake.NewRig(t)
	// fetch runs on the runner directly (network cohort, not migrated here); let it succeed.
	runner := &fetchOKRunner{fakeRemoteRunner: fakeRemoteRunner{t}}
	rig.CLI.OnResolveRef = func(_ backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
		if ref == "origin/main" {
			return sha1Hex, nil
		}
		return "", backend.ErrRefNotFound
	}

	branch, sha, err := resolveRemoteWorktreeBaseCommit(context.Background(), rig.Router, cli.New(nil), runner, "/remote/repo")

	require.NoError(t, err)
	assert.Equal(t, "main", branch)
	assert.Equal(t, sha1Hex, sha)
	rig.RequireOnlyCLIRemote(t, backend.OpResolveRef)
}

type fetchOKRunner struct{ fakeRemoteRunner }

func (fetchOKRunner) Run(context.Context, string, string, ...string) ([]byte, error) { return nil, nil }

func TestResolveRemoteWorktreeBaseCommit_UnbornVersusFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		headErr error
		wantErr bool
	}{
		{"unborn falls back to ambient HEAD", fmt.Errorf("x: %w", backend.ErrUnborn), false},
		{"any other failure is a hard error", errors.New("connection reset"), true},
		{"detached or resolvable HEAD is a hard error", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := gitfake.New("backend")
			b.OnResolveRef = func(_ backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
				if ref == "HEAD" && tc.headErr == nil {
					return sha1Hex, nil
				}
				if ref == "HEAD" {
					return "", tc.headErr
				}
				return "", backend.ErrRefNotFound
			}
			branch, sha, err := resolveRemoteWorktreeBaseCommit(context.Background(), b, cli.New(nil), &fetchOKRunner{}, "/r")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Empty(t, branch)
			assert.Empty(t, sha)
		})
	}
}

func TestRemoteAmbientHEADDivergesFromBase(t *testing.T) {
	t.Parallel()
	t.Run("routes through the backend as Remote and reports the branch", func(t *testing.T) {
		rig := gitfake.NewRig(t)
		rig.CLI.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return sha1Hex, nil }
		rig.CLI.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "feat/x", nil }

		diverged, br := remoteAmbientHEADDivergesFromBase(context.Background(), rig.Router, fakeRemoteRunner{t}, "/r", "otherbase")

		assert.True(t, diverged)
		assert.Equal(t, "feat/x", br)
		rig.RequireOnlyCLIRemote(t, backend.OpResolveRef)
		assert.Len(t, rig.CLI.CallsTo(backend.OpCurrentBranch), 1)
	})
	t.Run("detached HEAD diverges with no branch name", func(t *testing.T) {
		b := gitfake.New("backend")
		b.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return sha1Hex, nil }
		b.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "", backend.ErrDetachedHead }

		diverged, br := remoteAmbientHEADDivergesFromBase(context.Background(), b, fakeRemoteRunner{t}, "/r", "other")

		assert.True(t, diverged)
		assert.Empty(t, br)
	})
	t.Run("HEAD equal to base or unreadable is not divergence", func(t *testing.T) {
		b := gitfake.New("backend")
		b.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return sha1Hex, nil }
		diverged, _ := remoteAmbientHEADDivergesFromBase(context.Background(), b, fakeRemoteRunner{t}, "/r", sha1Hex)
		assert.False(t, diverged)
		assert.Empty(t, b.CallsTo(backend.OpCurrentBranch))

		b2 := gitfake.New("backend2")
		b2.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return "", errors.New("down") }
		diverged, _ = remoteAmbientHEADDivergesFromBase(context.Background(), b2, fakeRemoteRunner{t}, "/r", "x")
		assert.False(t, diverged)
	})
}
