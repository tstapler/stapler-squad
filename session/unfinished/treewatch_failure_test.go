package unfinished

import (
	"slices"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/testutil/wait"

	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingTreeWatcher struct{ fakeTreeWatcher }

func (f *failingTreeWatcher) Add(r string) error {
	f.added = append(f.added, r)
	return errors.New("inotify limit")
}

func armedAfterWatch(t *testing.T, tw treeWatcher) bool {
	t.Helper()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n"})
	g, _ := dirtyGenReader()
	s, _ := newSeamScanner(true)
	s.reader, s.treeWatch = g, tw
	s.watchRepo(dir)
	return g.dirtyGenFor(dir).armed.Load()
}

func TestWatchRepo_ShouldLeaveDirtyGenUnarmed_WhenTreeWatchAddFails(t *testing.T) {
	t.Parallel()
	assert.True(t, armedAfterWatch(t, &fakeTreeWatcher{}), "control: a working tree watch arms DirtyGen")
	assert.False(t, armedAfterWatch(t, &failingTreeWatcher{}), "no tree events: caches must not be trusted")
}

func TestInstallTreeWatcher_ShouldClearTreeWatch_WhenContextCancelled(t *testing.T) {
	t.Parallel()
	s := &Scanner{}
	ctx, cancel := context.WithCancel(context.Background())
	s.installTreeWatcher(ctx, &fakeTreeWatcher{})
	require.NotNil(t, s.treeWatch)

	cancel()
	s.wg.Wait()

	s.mu.RLock()
	defer s.mu.RUnlock()
	assert.Nil(t, s.treeWatch, "a re-run Start must not reuse the closed watcher")
}

func TestWatchRepo_TierA_ShouldOnlyWatchAdminDirUnderCommonWorktrees_WhenPointerFileIsHostile(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, fw := newSeamScanner(true)
	s.watchRepo(f.linked1)
	require.Equal(t, []string{f.adminDir1}, fw.addedPaths(), "control: the real admin dir is watched")

	for name, target := range map[string]string{
		"absolute elsewhere": t.TempDir(),
		"parent escape":      "..",
	} {
		repo := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+target+"\n"), 0o644))
		s, fw := newSeamScanner(true)

		s.watchRepo(repo)

		assert.Empty(t, fw.addedPaths(), name)
		_, ok := s.worktreeTargetFor(filepath.Clean(filepath.Join(repo, target)))
		assert.False(t, ok, name)
	}
}

// .git/worktrees created after the first watch is picked up from the Create
// event on <repo>/.git alone (default classification is all-worktrees, which
// re-runs watchRepo under Tier A).
func TestFsnotifyLoop_TierA_ShouldWatchWorktreesDir_WhenCreatedAfterFirstWatch(t *testing.T) {
	t.Parallel()
	repo, mainDir := testrepo.NewRepo(t)
	testrepo.Linear(t, repo, 2)
	s, fw := newSeamScanner(true)
	s.watchRepo(mainDir)
	worktreesDir := filepath.Join(mainDir, ".git", "worktrees")
	require.NotContains(t, fw.addedPaths(), worktreesDir, "precondition: no worktrees dir yet")

	testrepo.NewLinkedWorktree(t, mainDir, "wt1", "feature-1")
	runLoop(t, s)
	send(t, fw, fsnotify.Event{Name: worktreesDir, Op: fsnotify.Create})

	require.NoError(t, wait.WaitForCondition(func() bool {
		added := fw.addedPaths()
		return slices.Contains(added, worktreesDir) && slices.Contains(added, filepath.Join(worktreesDir, "wt1"))
	}, wait.FastWaitConfig()))
}
