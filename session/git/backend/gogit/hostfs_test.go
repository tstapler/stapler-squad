package gogit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func TestHostFSFollowsSymlinksOutOfItsRootButNotLexically(t *testing.T) {
	root := real(t, t.TempDir())
	outside := real(t, t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(outside, "f"), []byte("x"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "dir")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "f"), filepath.Join(root, "file")))
	fsys := hostFS{root: root}

	for _, name := range []string{"dir/f", "file", "/dir/f"} {
		f, err := fsys.Open(name)
		require.NoError(t, err, name)
		require.NoError(t, f.Close())
	}
	entries, err := fsys.ReadDir("dir")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	sub, err := fsys.Chroot("dir")
	require.NoError(t, err)
	_, err = sub.Stat("f")
	require.NoError(t, err)

	for _, name := range []string{"..", "../x", "dir/../../x"} {
		_, err := fsys.Stat(name)
		assert.ErrorIs(t, err, billy.ErrCrossedBoundary, name)
	}
}

func TestHostFSNeverWrites(t *testing.T) {
	fsys := hostFS{root: real(t, t.TempDir())}
	_, err := fsys.Create("a")
	assert.ErrorIs(t, err, billy.ErrReadOnly)
	_, err = fsys.OpenFile("a", os.O_RDWR|os.O_CREATE, 0o600)
	assert.ErrorIs(t, err, billy.ErrReadOnly)
	assert.ErrorIs(t, fsys.MkdirAll("a", 0o700), billy.ErrReadOnly)
	assert.ErrorIs(t, fsys.Remove("a"), billy.ErrReadOnly)
	assert.ErrorIs(t, fsys.Rename("a", "b"), billy.ErrReadOnly)
	assert.ErrorIs(t, fsys.Symlink("a", "b"), billy.ErrReadOnly)
	_, err = fsys.TempFile("", "x")
	assert.ErrorIs(t, err, billy.ErrReadOnly)
	assert.NoFileExists(t, filepath.Join(fsys.Root(), "a"))
}

// A .git symlinked to a git directory elsewhere is served in-process, with git's answer.
func TestSymlinkedDotGitIsServed(t *testing.T) {
	dir, _, head := initRepo(t, 3)
	store := filepath.Join(real(t, t.TempDir()), "store.git")
	require.NoError(t, os.Rename(filepath.Join(dir, ".git"), store))
	require.NoError(t, os.Symlink(store, filepath.Join(dir, ".git")))
	b := newBackend(t)
	ctx := context.Background()

	got, err := b.ResolveRef(ctx, local(dir), "HEAD~1")
	require.NoError(t, err)
	assert.NotEqual(t, backend.CommitSHA(head.String()), got)
	gotHead, err := b.ResolveRef(ctx, local(dir), "HEAD")
	require.NoError(t, err)
	assert.Equal(t, backend.CommitSHA(head.String()), gotHead)
	gitDir, err := b.GitDir(ctx, local(dir))
	require.NoError(t, err)
	assert.Equal(t, backend.GitDir(store), gitDir)
}

// A refs directory that links back into itself would recurse forever; the reader gives up to
// the CLI rather than guess.
func TestRefDirectorySymlinkLoopFallsBack(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	require.NoError(t, os.Symlink(filepath.Join(dir, ".git", "refs", "heads"), filepath.Join(dir, ".git", "refs", "heads", "loop")))
	_, err := newBackend(t).ListRefs(context.Background(), local(dir), backend.ListRefsRequest{})
	require.Error(t, err)
	assert.True(t, isFallback(err), "%v", err)
}
