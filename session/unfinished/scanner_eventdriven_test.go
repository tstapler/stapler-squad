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
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// ---- nil-safe fallback (fsnotify unavailable) -----------------------------

func TestScanner_AddRepoRemoveRepo_should_notPanic_When_FsWatcherNil(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	w, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	s := &Scanner{scanQueue: make(chan scanTask, 50), fsWatcher: w}
	repoPath := newRepoWithGitDir(t)

	s.watchRepo(repoPath)

	assert.Contains(t, w.WatchList(), filepath.Join(repoPath, ".git"))
}

func TestScanner_unwatchRepo_should_removeGitDirFromWatcher_When_Called(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// --- BUG-034: repo removal never fires on session/worktree cleanup ---------

// TestScanner_forgetSessionRepo_should_removeRepo_When_NoOtherSessionReferencesIt
// is the direct regression test for BUG-034's EventSessionDeleted handling: a
// repo auto-spidered from a session must stop being watched once that
// session is deleted and nothing else still points at the same repo root.
func TestScanner_forgetSessionRepo_should_removeRepo_When_NoOtherSessionReferencesIt(t *testing.T) {
	t.Parallel()
	s := &Scanner{scanQueue: make(chan scanTask, 50)}
	s.sessionRepos.Store("session-uuid-1", "/repo/a")
	s.repoSet.Store("/repo/a", true)

	s.forgetSessionRepo("session-uuid-1")

	_, stillTracked := s.repoSet.Load("/repo/a")
	assert.False(t, stillTracked, "repo must be removed once its only owning session is gone")
	_, stillMapped := s.sessionRepos.Load("session-uuid-1")
	assert.False(t, stillMapped, "the session->repo mapping itself must be forgotten")
}

// TestScanner_forgetSessionRepo_should_keepRepo_When_AnotherSessionStillReferencesIt
// verifies two sessions sharing one repo root (e.g. two non-worktree sessions
// in the same project) don't have scanning cut out from under the surviving
// one when only one of them is deleted.
func TestScanner_forgetSessionRepo_should_keepRepo_When_AnotherSessionStillReferencesIt(t *testing.T) {
	t.Parallel()
	s := &Scanner{scanQueue: make(chan scanTask, 50)}
	s.sessionRepos.Store("session-uuid-1", "/repo/shared")
	s.sessionRepos.Store("session-uuid-2", "/repo/shared")
	s.repoSet.Store("/repo/shared", true)

	s.forgetSessionRepo("session-uuid-1")

	_, stillTracked := s.repoSet.Load("/repo/shared")
	assert.True(t, stillTracked, "repo must stay watched while another session still references it")
	_, session2StillMapped := s.sessionRepos.Load("session-uuid-2")
	assert.True(t, session2StillMapped)
}

// TestScanner_forgetSessionRepo_should_beNoOp_When_SessionNeverTracked verifies
// a delete event for a session that was never auto-spidered (e.g. auto-spider
// was disabled, or it's a pinned repo with no session at all) doesn't panic
// or otherwise misbehave.
func TestScanner_forgetSessionRepo_should_beNoOp_When_SessionNeverTracked(t *testing.T) {
	t.Parallel()
	s := &Scanner{scanQueue: make(chan scanTask, 50)}
	assert.NotPanics(t, func() {
		s.forgetSessionRepo("never-seen-uuid")
		s.forgetSessionRepo("")
	})
}

// TestScanner_subscribeToSessionEvents_should_removeRepo_When_SessionDeletedEventReceived
// is the end-to-end version: a real EventSessionDeleted published on the
// event bus (the shape server/services/session_service.go's DeleteSession
// actually publishes — Session nil, SessionID set) must reach
// subscribeToSessionEvents and remove the repo, exactly mirroring how
// EventSessionCreated already adds one.
func TestScanner_subscribeToSessionEvents_should_removeRepo_When_SessionDeletedEventReceived(t *testing.T) {
	t.Parallel()
	bus := pkgevents.NewEventBus(4)
	s := &Scanner{scanQueue: make(chan scanTask, 50), eventBus: bus}
	s.autoSpiderEnabled.Store(true)

	repoPath := newRepoWithGitDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.subscribeToSessionEvents(ctx)

	// Wait for the subscription to actually register before publishing —
	// otherwise the first Publish can race the goroutine's Subscribe call and
	// be missed entirely (this is a live pub/sub, not a durable queue).
	err := wait.WaitForCondition(func() bool {
		return bus.SubscriberCount() > 0
	}, wait.FastWaitConfig())
	require.NoError(t, err, "expected subscribeToSessionEvents to register a subscriber")

	bus.Publish(&pkgevents.Event{
		Type:    pkgevents.EventSessionCreated,
		Session: &session.Instance{UUID: "session-uuid-1", Title: "test-session", Path: repoPath},
	})
	err = wait.WaitForCondition(func() bool {
		_, ok := s.repoSet.Load(repoPath)
		return ok
	}, wait.FastWaitConfig())
	require.NoError(t, err, "expected repo to be added after SessionCreated")

	bus.Publish(&pkgevents.Event{
		Type:      pkgevents.EventSessionDeleted,
		SessionID: "session-uuid-1",
	})
	err = wait.WaitForCondition(func() bool {
		_, ok := s.repoSet.Load(repoPath)
		return !ok
	}, wait.FastWaitConfig())
	assert.NoError(t, err, "expected repo to be removed after SessionDeleted")
}

// --- BUG-034: pruneMissingRepos self-pruning backstop -----------------------

// TestScanner_pruneMissingRepos_should_removeRepo_When_PathNoLongerExists is
// the backstop's own regression test: any repo whose path has been deleted
// from disk (by any cleanup path, present or future — not just the explicit
// RemoveRepo call sites) must eventually be dropped from the watch set.
func TestScanner_pruneMissingRepos_should_removeRepo_When_PathNoLongerExists(t *testing.T) {
	orig := pruneRepoInterval
	pruneRepoInterval = 10 * time.Millisecond
	t.Cleanup(func() { pruneRepoInterval = orig })

	dir := t.TempDir()
	goneRepo := filepath.Join(dir, "gone")
	require.NoError(t, os.Mkdir(goneRepo, 0o755))

	s := &Scanner{scanQueue: make(chan scanTask, 50)}
	s.repoSet.Store(goneRepo, true)

	require.NoError(t, os.RemoveAll(goneRepo))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.pruneMissingRepos(ctx)

	err := wait.WaitForCondition(func() bool {
		_, ok := s.repoSet.Load(goneRepo)
		return !ok
	}, wait.FastWaitConfig())
	assert.NoError(t, err, "expected the missing repo to be pruned")
}

// TestScanner_pruneMissingRepos_should_keepRepo_When_PathStillExists verifies
// the backstop doesn't over-trigger — a repo whose directory is still present
// must not be removed just because a prune tick fired.
func TestScanner_pruneMissingRepos_should_keepRepo_When_PathStillExists(t *testing.T) {
	orig := pruneRepoInterval
	pruneRepoInterval = 10 * time.Millisecond
	t.Cleanup(func() { pruneRepoInterval = orig })

	repoPath := newRepoWithGitDir(t)

	s := &Scanner{scanQueue: make(chan scanTask, 50)}
	s.repoSet.Store(repoPath, true)

	ctx, cancel := context.WithCancel(context.Background())
	go s.pruneMissingRepos(ctx)

	time.Sleep(50 * time.Millisecond) // let a few ticks fire
	cancel()

	_, stillTracked := s.repoSet.Load(repoPath)
	assert.True(t, stillTracked, "an existing repo must survive prune ticks")
}
