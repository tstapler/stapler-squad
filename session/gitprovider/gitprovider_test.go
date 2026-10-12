package gitprovider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

func TestBackend_UnregisteredRejectsLocalLocations(t *testing.T) {
	defer gitprovider.Register(nil)()

	_, err := gitprovider.Backend().RepoRoot(context.Background(), backend.Local{Root: t.TempDir()})

	require.ErrorIs(t, err, backend.ErrNoLocalRunner)
}

func TestRegister_RestoreReinstatesThePreviousProvider(t *testing.T) {
	first, second := gitfake.New("first"), gitfake.New("second")
	restoreFirst := gitprovider.Register(func() backend.Backend { return first })
	defer restoreFirst()
	restoreSecond := gitprovider.Register(func() backend.Backend { return second })
	assert.Same(t, second, gitprovider.Backend())
	restoreSecond()
	assert.Same(t, first, gitprovider.Backend())
}

func TestRefReader_AdaptsPlainPathsToLocalLocations(t *testing.T) {
	fake := gitfake.New("fake")
	fake.OnRepoRoot = func(backend.RepoLocation) (backend.RepoRoot, error) { return "/repo", nil }
	fake.OnGitDir = func(backend.RepoLocation) (backend.GitDir, error) { return "/repo/.git/worktrees/wt", nil }
	defer gitprovider.Register(func() backend.Backend { return fake })()

	var r gitprovider.RefReader
	top, err := r.ShowToplevel(context.Background(), "/repo/sub")
	require.NoError(t, err)
	assert.Equal(t, "/repo", top)
	gd, err := r.GitDir(context.Background(), "/repo/sub")
	require.NoError(t, err)
	assert.Equal(t, "/repo/.git/worktrees/wt", gd)

	assert.Equal(t, backend.Local{Root: "/repo/sub"}, fake.CallsTo(backend.OpRepoRoot)[0].Loc)
	assert.Equal(t, backend.Local{Root: "/repo/sub"}, fake.CallsTo(backend.OpGitDir)[0].Loc)
}
