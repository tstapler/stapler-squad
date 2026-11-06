package vcs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend, so none of them is parallel.

func TestGitAvailable_RoutesGitDirThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnGitDir = func(backend.RepoLocation) (backend.GitDir, error) { return "/r/.git", nil }

	assert.True(t, gitAvailable("/r"))
	rig.RequireOnlyGoGit(t, backend.OpGitDir)
	assert.Equal(t, backend.Local{Root: "/r"}, rig.GoGit.Calls()[0].Loc)
}

func TestGitAvailable_FalseOutsideARepository(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnGitDir = func(backend.RepoLocation) (backend.GitDir, error) { return "", backend.ErrNotARepo }

	assert.False(t, gitAvailable("/not-a-repo"))
}

func TestGitClient_GetCurrentBookmark(t *testing.T) {
	for _, tc := range []struct {
		name    string
		branch  backend.BranchName
		err     error
		want    string
		wantErr bool
	}{
		{"on a branch", "feat/x", nil, "feat/x", false},
		{"detached HEAD is an empty bookmark, not an error", "", backend.ErrDetachedHead, "", false},
		{"unborn HEAD stays an error", "", backend.ErrUnborn, "", true},
		{"other failure stays an error", "", errors.New("boom"), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := gitfake.New("backend")
			gitfake.Use(t, fake)
			fake.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return tc.branch, tc.err }

			got, err := NewGitClient("/r").GetCurrentBookmark()

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantErr, err != nil)
		})
	}
}

func TestGitClient_GetCurrentBookmark_RefsGoGitReachesGoGit(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnCurrentBranch = func(backend.RepoLocation) (backend.BranchName, error) { return "main", nil }

	got, err := NewGitClient("/r").GetCurrentBookmark()

	require.NoError(t, err)
	assert.Equal(t, "main", got)
	rig.RequireOnlyGoGit(t, backend.OpCurrentBranch)
}

func TestGitClient_BranchExists(t *testing.T) {
	for _, tc := range []struct {
		name   string
		exists bool
		err    error
		want   bool
	}{
		{"present", true, nil, true},
		{"absent", false, nil, false},
		{"lookup failure is not existence", false, errors.New("boom"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := gitfake.NewRig(t)
			rig.Install(t)
			rig.GoGit.OnRefExists = func(_ backend.RepoLocation, ref backend.RefName) (bool, error) {
				assert.Equal(t, backend.RefName("feat"), ref)
				return tc.exists, tc.err
			}

			assert.Equal(t, tc.want, NewGitClient("/r").branchExists("feat"))
			assert.NotEmpty(t, rig.GoGit.CallsTo(backend.OpRefExists))
		})
	}
}

// Behaviour preserved against the real CLI backend.
func TestGitClient_RealGit_BookmarkAndBranchExists(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@e.com")
	run("config", "user.name", "t")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644))
	run("add", ".")
	run("commit", "-m", "init")
	run("branch", "other")
	c := NewGitClient(dir)

	got, err := c.GetCurrentBookmark()
	require.NoError(t, err)
	assert.Equal(t, "main", got)
	assert.True(t, c.branchExists("other"))
	assert.False(t, c.branchExists("nope"))

	run("checkout", "--detach")
	got, err = c.GetCurrentBookmark()
	require.NoError(t, err)
	assert.Empty(t, got, "detached HEAD has no bookmark")
}
