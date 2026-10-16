package unfinished

// Story 4.3.1 tests: Tier A linked-worktree gitdir watching, driven through
// the gitWatcher seam with a fake, so no real fs events and no sleeps.

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGitWatcher struct {
	mu      sync.Mutex
	added   []string
	removed []string
	events  chan fsnotify.Event
	errs    chan error
}

func newFakeGitWatcher() *fakeGitWatcher {
	return &fakeGitWatcher{events: make(chan fsnotify.Event), errs: make(chan error)}
}

func (f *fakeGitWatcher) Add(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, name)
	return nil
}

func (f *fakeGitWatcher) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, name)
	return nil
}

func (f *fakeGitWatcher) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeGitWatcher) Errors() <-chan error          { return f.errs }
func (f *fakeGitWatcher) Close() error                  { return nil }

func (f *fakeGitWatcher) addedPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.added...)
}

func newSeamScanner(tierA bool) (*Scanner, *fakeGitWatcher) {
	fw := newFakeGitWatcher()
	return &Scanner{
		scanQueue: make(chan scanTask, 50),
		gitwatch:  gitwatchState{override: fw, watchWorktreeGitdirs: tierA},
	}, fw
}

func runLoop(t *testing.T, s *Scanner) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.fsnotifyLoop(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
}

// nextTask blocks for the next queued scan; the timeout is only a failsafe.
func nextTask(t *testing.T, s *Scanner) scanTask {
	t.Helper()
	select {
	case task := <-s.scanQueue:
		return task
	case <-time.After(10 * time.Second):
		t.Fatal("no scan task enqueued")
		return scanTask{}
	}
}

func send(t *testing.T, fw *fakeGitWatcher, ev fsnotify.Event) {
	t.Helper()
	select {
	case fw.events <- ev:
	case <-time.After(10 * time.Second):
		t.Fatal("event loop not receiving")
	}
}

func TestWatchRepo_ShouldWatchLinkedWorktreeGitdir_WhenTierAEnabled(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, fw := newSeamScanner(true)
	admin2 := filepath.Join(f.mainDir, ".git", "worktrees", "wt2")

	s.watchRepo(f.mainDir)

	assert.ElementsMatch(t, []string{filepath.Join(f.mainDir, ".git"), f.adminDir1, admin2}, fw.addedPaths())

	// Seed fresh caches so invalidation scope is observable.
	c1, c2 := &worktreeCache{ttl: time.Hour}, &worktreeCache{ttl: time.Hour}
	c1.Set(ScanResult{})
	c2.Set(ScanResult{})
	s.cacheStore.Store(f.linked1, c1)
	s.cacheStore.Store(f.linked2, c2)

	runLoop(t, s)
	send(t, fw, fsnotify.Event{Name: filepath.Join(f.adminDir1, "HEAD"), Op: fsnotify.Write})
	task := nextTask(t, s)

	assert.Equal(t, f.mainDir, task.repoPath)
	_, fresh1 := c1.Get()
	_, fresh2 := c2.Get()
	assert.False(t, fresh1, "wt1's cache is invalidated")
	assert.True(t, fresh2, "wt2's cache is untouched: the burst is scoped to wt1")

	// index events are relevant too, and lock files are not.
	s.inFlight.Delete(f.mainDir)
	send(t, fw, fsnotify.Event{Name: filepath.Join(f.adminDir1, "index.lock"), Op: fsnotify.Create})
	send(t, fw, fsnotify.Event{Name: filepath.Join(f.adminDir1, "index"), Op: fsnotify.Write})
	assert.Equal(t, f.mainDir, nextTask(t, s).repoPath)
	assert.Empty(t, s.scanQueue, "the lock-file event enqueued nothing extra")
}

func TestWatchRepo_ShouldWatchResolvedGitdir_WhenDotGitIsPointerFile(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, fw := newSeamScanner(true)

	s.watchRepo(f.linked1)

	assert.Equal(t, []string{f.adminDir1}, fw.addedPaths(), "resolved gitdir, not the pointer file")

	runLoop(t, s)
	send(t, fw, fsnotify.Event{Name: filepath.Join(f.adminDir1, "HEAD"), Op: fsnotify.Write})
	assert.Equal(t, f.linked1, nextTask(t, s).repoPath)
}

func TestUnwatchRepo_ShouldRemoveLinkedGitdirs_WhenTierAEnabled(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, fw := newSeamScanner(true)
	s.watchRepo(f.mainDir)

	s.unwatchRepo(f.mainDir)

	assert.Contains(t, fw.removed, f.adminDir1)
	assert.Contains(t, fw.removed, filepath.Join(f.mainDir, ".git"))
	assert.Empty(t, s.gitwatch.targets)
}

func TestClassifyGitEvent_ShouldTreatPackedRefsRenameLikeCreate_WhenMacLockfilePattern(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, fw := newSeamScanner(true)
	s.watchRepo(f.mainDir)
	runLoop(t, s)

	send(t, fw, fsnotify.Event{Name: filepath.Join(f.mainDir, ".git", "packed-refs"), Op: fsnotify.Rename})

	assert.Equal(t, f.mainDir, nextTask(t, s).repoPath)

	// Contrast: with Tier A off the Rename is dropped as before; the sentinel
	// Create behind it proves the loop processed it.
	off, offFw := newSeamScanner(false)
	runLoop(t, off)
	send(t, offFw, fsnotify.Event{Name: filepath.Join(f.mainDir, ".git", "packed-refs"), Op: fsnotify.Rename})
	send(t, offFw, fsnotify.Event{Name: filepath.Join(f.linked2, ".git", "HEAD"), Op: fsnotify.Create})
	assert.Equal(t, f.linked2, nextTask(t, off).repoPath, "first task is the sentinel's, not the Rename's")
}

func TestWatcher_ShouldFullWalkEveryWorktree_WhenFakeWatcherInjectsErrEventOverflow(t *testing.T) {
	t.Parallel()
	s, fw := newSeamScanner(true)
	s.repoSet.Store("/repo/a", true)
	s.repoSet.Store("/repo/b", true)
	runLoop(t, s)

	select {
	case fw.errs <- fsnotify.ErrEventOverflow:
	case <-time.After(10 * time.Second):
		t.Fatal("event loop not receiving")
	}
	got := []scanTask{nextTask(t, s), nextTask(t, s)}

	assert.ElementsMatch(t, []string{"/repo/a", "/repo/b"}, []string{got[0].repoPath, got[1].repoPath})
	assert.True(t, got[0].force && got[1].force, "overflow forces a rescan past the freshness TTL")
	assert.EqualValues(t, 1, s.gitwatch.overflowRescans.Load())
}

func TestWatcher_ShouldOnlyLogOverflow_WhenTierADisabled(t *testing.T) {
	t.Parallel()
	s, fw := newSeamScanner(false)
	s.repoSet.Store("/repo/a", true)
	runLoop(t, s)

	fw.errs <- fsnotify.ErrEventOverflow
	// The events loop is a single goroutine, so once this sentinel is
	// consumed the error before it has been handled.
	send(t, fw, fsnotify.Event{Name: "/repo/z/.git/HEAD", Op: fsnotify.Write})

	assert.Equal(t, "/repo/z", nextTask(t, s).repoPath)
	assert.Empty(t, s.scanQueue)
	assert.Zero(t, s.gitwatch.overflowRescans.Load())
}

func TestWatchRepo_ShouldWatchOnlyDotGit_WhenTierADisabled(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	s, fw := newSeamScanner(false)

	s.watchRepo(f.mainDir)
	s.watchRepo(f.linked1)

	assert.Equal(t, []string{filepath.Join(f.mainDir, ".git"), filepath.Join(f.linked1, ".git")}, fw.addedPaths())
	require.Empty(t, s.gitwatch.targets)
}
