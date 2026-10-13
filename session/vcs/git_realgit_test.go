package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// newRealRepo returns a repository with one commit on main and a GitClient for it.
func newRealRepo(t *testing.T) (string, func(args ...string), *GitClient) {
	t.Helper()
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
	run("config", "user.name", "Ada")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644))
	run("add", ".")
	run("commit", "-m", "init")
	return dir, run, NewGitClient(dir)
}

func TestGitClient_RealGit_StatusAndUncommittedChanges(t *testing.T) {
	dir, run, c := newRealRepo(t)

	clean, err := c.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, clean)
	st, err := c.GetStatus()
	require.NoError(t, err)
	assert.False(t, st.HasChanges)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("changed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new"), []byte("n"), 0o644))
	run("add", "new")

	dirty, err := c.HasUncommittedChanges()
	require.NoError(t, err)
	assert.True(t, dirty)
	st, err = c.GetStatus()
	require.NoError(t, err)
	assert.Equal(t, 1, st.ModifiedFiles)
	assert.Equal(t, 1, st.AddedFiles)
	assert.True(t, st.HasChanges)
}

func TestGitClient_RealGit_RevisionsAndBookmarks(t *testing.T) {
	_, run, c := newRealRepo(t)
	run("branch", "other")

	rev, err := c.GetCurrentRevision()
	require.NoError(t, err)
	assert.Len(t, rev.ID, 40)
	assert.NotEmpty(t, rev.ShortID)
	assert.Equal(t, "init", rev.Description)
	assert.Equal(t, "Ada", rev.Author)
	assert.False(t, rev.Timestamp.IsZero())
	assert.True(t, rev.IsCurrent)
	assert.ElementsMatch(t, []string{"main", "other"}, rev.Bookmarks)

	revs, err := c.ListRecentRevisions(10)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	assert.Equal(t, rev.ID, revs[0].ID)
	none, err := c.ListRecentRevisions(0)
	require.NoError(t, err)
	assert.Empty(t, none)

	marks, err := c.ListBookmarks()
	require.NoError(t, err)
	names := make([]string, 0, len(marks))
	for _, m := range marks {
		names = append(names, m.Name)
		assert.False(t, m.IsRemote)
	}
	assert.ElementsMatch(t, []string{"main", "other"}, names)
}

func TestGitClient_RealGit_SwitchTo(t *testing.T) {
	dir, run, c := newRealRepo(t)
	run("branch", "other")

	// Dirty tree + BringAlong: stash, switch, pop.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("carried"), 0o644))
	require.NoError(t, c.SwitchTo("other", SwitchOptions{ChangeStrategy: BringAlong}))
	got, err := c.GetCurrentBookmark()
	require.NoError(t, err)
	assert.Equal(t, "other", got)
	body, err := os.ReadFile(filepath.Join(dir, "f"))
	require.NoError(t, err)
	assert.Equal(t, "carried", string(body))

	// Missing target is created on request, an unknown one is an error.
	require.NoError(t, c.SwitchTo("fresh", SwitchOptions{ChangeStrategy: BringAlong, CreateIfMissing: true}))
	got, _ = c.GetCurrentBookmark()
	assert.Equal(t, "fresh", got)
	assert.Error(t, c.SwitchTo("nope", SwitchOptions{}))

	// A commit-ish that is not a branch detaches, as `git checkout <sha>` did.
	rev, err := c.GetCurrentRevision()
	require.NoError(t, err)
	require.NoError(t, c.SwitchTo(rev.ID, SwitchOptions{ChangeStrategy: Abandon}))
	got, _ = c.GetCurrentBookmark()
	assert.Empty(t, got)
}

func TestGitClient_RealGit_AbandonChangesAndCreateBookmark(t *testing.T) {
	dir, _, c := newRealRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("dirty"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "junk"), []byte("j"), 0o644))

	require.NoError(t, c.AbandonChanges())

	dirty, err := c.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, dirty)

	require.NoError(t, c.CreateBookmark("topic", ""))
	assert.True(t, c.branchExists("topic"))
}

func TestGitClient_RealGit_Worktrees(t *testing.T) {
	dir, _, c := newRealRepo(t)
	wtPath := filepath.Join(t.TempDir(), "wt")

	require.NoError(t, c.CreateWorktree(wtPath, "wt-branch"))

	wts, err := c.ListWorktrees()
	require.NoError(t, err)
	require.Len(t, wts, 2)
	byName := map[string]Worktree{}
	for _, w := range wts {
		byName[w.Name] = w
	}
	assert.Equal(t, "wt-branch", byName["wt-branch"].Bookmark)
	assert.Len(t, byName["wt-branch"].RevisionID, 40)
	main := byName["main"]
	evalDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	evalMain, err := filepath.EvalSymlinks(main.Path)
	require.NoError(t, err)
	assert.Equal(t, evalDir, evalMain)
}
