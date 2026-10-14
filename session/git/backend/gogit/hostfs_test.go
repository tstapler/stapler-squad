package gogit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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

// Objects that live only in an alternate store (a `git clone --shared`) are found through the
// absolute alternates path.
func TestAlternateObjectStoreIsRead(t *testing.T) {
	srcDir, _, head := initRepo(t, 2)
	dir, _, _ := initRepo(t, 0)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects", "info"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "objects", "info", "alternates"), []byte(filepath.Join(srcDir, ".git", "objects")+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "refs", "heads"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "refs", "heads", "main"), []byte(head.String()+"\n"), 0o600))

	got, err := newBackend(t).MergeBase(context.Background(), local(dir), backend.MergeBaseRequest{Left: "HEAD", Right: "HEAD~1"})
	require.NoError(t, err)
	assert.NotEmpty(t, got)
}

// packedDeltaRepo builds a repo whose history is aggressively repacked into deltas and returns
// its dir plus a git runner. go-git reopens a pack by fs.Open(file.Name()) for objects it will
// not inflate eagerly (packfile.FSObject); a hostFile whose Name() was the host path got joined
// onto the root a second time: "open <gitdir>/<gitdir>/objects/pack/x.pack: no such file".
func packedDeltaRepo(t *testing.T) (string, func(dir string, args ...string) string) {
	dir := real(t, t.TempDir())
	git := func(dir string, args ...string) string {
		cmd := safeexec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(hermetic(t), "PATH="+os.Getenv("PATH"), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	git(dir, "init", "-q", "-b", "main")
	body := strings.Repeat("a long, repetitive commit message body that deltifies well\n", 400)
	for i := range 6 {
		git(dir, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("c%d\n\n%s", i, body))
	}
	git(dir, "branch", "side", "HEAD~2")
	git(dir, "gc", "-q", "--aggressive")
	require.Contains(t, git(dir, "count-objects", "-v"), "count: 0\n", "history must be packed for this test to mean anything")
	return dir, git
}

func requireMergeBaseMatchesGit(t *testing.T, at string, git func(string, ...string) string) {
	t.Helper()
	got, err := newBackend(t).MergeBase(context.Background(), local(at), backend.MergeBaseRequest{Left: "main", Right: "side"})
	require.NoError(t, err)
	assert.Equal(t, git(at, "merge-base", "main", "side"), string(got))
}

func TestMergeBaseReopensPacksInMainAndLinkedWorktrees(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprintf("linked=%v", linked), func(t *testing.T) {
			dir, git := packedDeltaRepo(t)
			at := dir
			if linked {
				at = filepath.Join(real(t, t.TempDir()), "wt")
				git(dir, "worktree", "add", "-q", "--detach", at, "HEAD")
			}
			requireMergeBaseMatchesGit(t, at, git)
		})
	}
}

// The pack is reopened through the alternates chroot, and through a symlinked .git.
func TestMergeBaseReopensPacksViaAlternatesAndSymlinkedDotGit(t *testing.T) {
	t.Run("alternates", func(t *testing.T) {
		src, git := packedDeltaRepo(t)
		clone := filepath.Join(real(t, t.TempDir()), "clone")
		git(src, "clone", "-q", "--shared", src, clone)
		git(clone, "fetch", "-q", "origin", "side:side")
		requireMergeBaseMatchesGit(t, clone, git)
	})
	t.Run("symlinked .git", func(t *testing.T) {
		dir, git := packedDeltaRepo(t)
		store := filepath.Join(real(t, t.TempDir()), "store.git")
		require.NoError(t, os.Rename(filepath.Join(dir, ".git"), store))
		require.NoError(t, os.Symlink(store, filepath.Join(dir, ".git")))
		requireMergeBaseMatchesGit(t, dir, git)
	})
}
