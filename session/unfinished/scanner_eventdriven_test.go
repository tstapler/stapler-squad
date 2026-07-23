package unfinished

// Tests for the event-driven (fsnotify) scanning added to Scanner, and for
// the memory-pressure graceful-degradation skip added to EnqueueRepo.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// ---- nil-safe fallback (fsnotify unavailable) -----------------------------

func TestScanner_AddRepoRemoveRepo_should_notPanic_When_FsWatcherNil(t *testing.T) {
	s := &Scanner{scanQueue: make(chan scanTask, 50)}
	require.Nil(t, s.fsWatcher)

	assert.NotPanics(t, func() {
		s.AddRepo("/tmp/some-repo")
		s.RemoveRepo("/tmp/some-repo")
	})
}

// ---- fsnotify registration wiring ------------------------------------------

func newRepoWithGitDir(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	return repoPath
}

func TestScanner_watchRepo_should_registerGitDirWithWatcher_When_FsWatcherPresent(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	s := &Scanner{scanQueue: make(chan scanTask, 50), fsWatcher: w}
	repoPath := newRepoWithGitDir(t)

	s.watchRepo(repoPath)

	assert.Contains(t, w.WatchList(), filepath.Join(repoPath, ".git"))
}

func TestScanner_unwatchRepo_should_removeGitDirFromWatcher_When_Called(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	s := &Scanner{scanQueue: make(chan scanTask, 50), fsWatcher: w}
	repoPath := newRepoWithGitDir(t)

	s.watchRepo(repoPath)
	require.Contains(t, w.WatchList(), filepath.Join(repoPath, ".git"))

	s.unwatchRepo(repoPath)
	assert.NotContains(t, w.WatchList(), filepath.Join(repoPath, ".git"))
}

func TestScanner_AddRepo_should_registerFsnotifyWatch_When_FsWatcherPresent(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	s := &Scanner{scanQueue: make(chan scanTask, 50), fsWatcher: w}
	repoPath := newRepoWithGitDir(t)

	s.AddRepo(repoPath)

	assert.Contains(t, w.WatchList(), filepath.Join(repoPath, ".git"))
}

// ---- end-to-end: a real .git write triggers a targeted rescan -------------

func TestScanner_fsnotifyLoop_should_enqueueTargetedRescan_When_GitDirFileWritten(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	require.NoError(t, err)

	s := &Scanner{
		scanQueue: make(chan scanTask, 50),
		fsWatcher: w,
	}
	repoPath := newRepoWithGitDir(t)
	s.watchRepo(repoPath)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.fsnotifyLoop(ctx)

	// Simulate a real git operation touching .git/HEAD.
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644))

	err = wait.WaitForCondition(func() bool {
		select {
		case task := <-s.scanQueue:
			return task.repoPath == repoPath
		default:
			return false
		}
	}, wait.FastWaitConfig())
	assert.NoError(t, err, "expected a scanTask for %s to be enqueued after a .git write", repoPath)
}

// ---- memory-pressure graceful degradation in EnqueueRepo -------------------

func TestEnqueueRepo_should_skipScan_When_UnderSeverePressure(t *testing.T) {
	withHeapInUse(t, severeMemoryPressureThreshold+1)

	s := &Scanner{
		scanQueue: make(chan scanTask, 50),
		reader:    &GoGitVCSReader{},
	}

	s.EnqueueRepo("/tmp/some-repo")

	select {
	case task := <-s.scanQueue:
		t.Fatalf("expected no scan to be enqueued under severe pressure, got %+v", task)
	default:
	}
	assert.True(t, s.severePressureWarned.Load(), "expected the pressure warning flag to be set")
}

func TestEnqueueRepo_should_scanNormally_When_NoPressure(t *testing.T) {
	withHeapInUse(t, 1*1024*1024*1024) // well below highMemoryPressureThreshold

	s := &Scanner{
		scanQueue: make(chan scanTask, 50),
		reader:    &GoGitVCSReader{},
	}

	s.EnqueueRepo("/tmp/some-repo")

	select {
	case task := <-s.scanQueue:
		assert.Equal(t, "/tmp/some-repo", task.repoPath)
	case <-time.After(time.Second):
		t.Fatal("expected a scan task to be enqueued when not under pressure")
	}
	assert.False(t, s.severePressureWarned.Load())
}

func TestEnqueueRepo_should_clearWarningFlag_When_PressureSubsides(t *testing.T) {
	s := &Scanner{
		scanQueue: make(chan scanTask, 50),
		reader:    &GoGitVCSReader{},
	}

	withHeapInUse(t, severeMemoryPressureThreshold+1)
	s.EnqueueRepo("/tmp/repo-a")
	require.True(t, s.severePressureWarned.Load())

	withHeapInUse(t, 1*1024*1024*1024)
	s.EnqueueRepo("/tmp/repo-b")
	assert.False(t, s.severePressureWarned.Load(), "expected the warning flag to clear once pressure subsides")
}
