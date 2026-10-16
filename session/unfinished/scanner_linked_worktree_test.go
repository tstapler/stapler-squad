package unfinished

// Characterization tests (Story 0.1.4b) for a registered repo that is a
// linked worktree, whose `.git` is a "gitdir: ..." pointer FILE. They pin
// today's watch and ListWorktrees behaviour, including its gaps.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// linkedFixture is a main repo with two linked worktrees ("wt1", "wt2").
type linkedFixture struct {
	mainDir, linked1, linked2 string
	adminDir1                 string // <main>/.git/worktrees/wt1, the linked worktree's real gitdir
}

func newLinkedFixture(t *testing.T) linkedFixture {
	t.Helper()
	repo, mainDir := testrepo.NewRepo(t)
	testrepo.Linear(t, repo, 2)
	return linkedFixture{
		mainDir:   mainDir,
		linked1:   testrepo.NewLinkedWorktree(t, mainDir, "wt1", "feature-1"),
		linked2:   testrepo.NewLinkedWorktree(t, mainDir, "wt2", "feature-2"),
		adminDir1: filepath.Join(mainDir, ".git", "worktrees", "wt1"),
	}
}

func newWatchingScanner(t *testing.T) (*Scanner, *fsnotify.Watcher) {
	t.Helper()
	w, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return &Scanner{scanQueue: make(chan scanTask, 50), fsWatcher: w}, w
}

// watchRepo registers exactly <repo>/.git. For a linked worktree that is the
// pointer file: the real gitdir (<main>/.git/worktrees/<name>) and the main
// .git are not watched. VERIFIED, replacing the plan's INFERRED.
func TestLinkedWorktree_WatchRepo_ShouldWatchOnlyThePointerFile(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, w := newWatchingScanner(t)

	pointer := filepath.Join(f.linked1, ".git")
	fi, err := os.Stat(pointer)
	require.NoError(t, err)
	require.False(t, fi.IsDir(), "precondition: linked .git is a pointer file")

	s.watchRepo(f.linked1)

	assert.Equal(t, []string{pointer}, w.WatchList(), "only the pointer file is watched")
	assert.NotContains(t, w.WatchList(), f.adminDir1)
	assert.NotContains(t, w.WatchList(), filepath.Join(f.mainDir, ".git"))
}

// Observable consequence of the above: a write to the linked worktree's real
// gitdir (what `git commit` in that worktree touches) delivers no event, while
// rewriting the pointer file does.
func TestLinkedWorktree_FsnotifyLoop_ShouldIgnoreRealGitdirWrites_AndReactToPointerRewrite(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, _ := newWatchingScanner(t)
	s.watchRepo(f.linked1)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.fsnotifyLoop(ctx)

	// Real gitdir HEAD change first: unwatched, so it must not enqueue.
	headPath := filepath.Join(f.adminDir1, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte("ref: refs/heads/feature-1\n"), 0o644))

	// Then the pointer file, same content: the one write the watcher can see.
	pointer := filepath.Join(f.linked1, ".git")
	body, err := os.ReadFile(pointer)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pointer, body, 0o644))

	var got []string
	err = wait.WaitForCondition(func() bool {
		select {
		case task := <-s.scanQueue:
			got = append(got, task.repoPath)
			return true
		default:
			return false
		}
	}, wait.FastWaitConfig())
	require.NoError(t, err, "pointer-file rewrite should enqueue a rescan")
	assert.Equal(t, []string{f.linked1}, got, "event maps to the linked worktree path as the repo root")
	assert.Empty(t, s.scanQueue, "the real-gitdir write produced no task")
}

// From a linked path, ListWorktrees labels the linked path itself as the
// first ("main") entry, then lists every linked worktree from the common
// dir, so the linked path appears twice and the real main worktree is absent.
func TestListWorktrees_FromLinkedPath_ShouldListSelfThenAllLinked_NotMain(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)

	g := &GoGitVCSReader{}
	got, err := g.ListWorktrees(f.linked1)
	require.NoError(t, err)

	paths := make([]string, len(got))
	for i, wt := range got {
		paths[i] = wt.Path
	}
	assert.Equal(t, f.linked1, paths[0], "entry 0 is the queried path, whatever it is")
	assert.Equal(t, "feature-1", got[0].Branch)
	assert.ElementsMatch(t, []string{f.linked1, f.linked1, f.linked2}, paths,
		"linked path listed twice; main worktree never listed")
	assert.NotContains(t, paths, f.mainDir)
}

// Contrast: from the main path the result is main plus both linked entries.
func TestListWorktrees_FromMainPath_ShouldListMainThenLinked(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)

	g := &GoGitVCSReader{}
	got, err := g.ListWorktrees(f.mainDir)
	require.NoError(t, err)

	paths := make([]string, len(got))
	for i, wt := range got {
		paths[i] = wt.Path
	}
	assert.Equal(t, f.mainDir, paths[0])
	assert.ElementsMatch(t, []string{f.mainDir, f.linked1, f.linked2}, paths)
}
