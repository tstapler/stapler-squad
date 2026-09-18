package session

// git_worktree_watcher_test.go covers Epic 2.1 (WorktreeChangeDetector):
//  - Story 2.1.1: construct/start/stop, .git-watch-unavailable fallback,
//    edge-triggered firing, panic isolation between OnChange callbacks.
//  - Story 2.1.2: activeFunc gating of the periodic tick's real work.
//
// Per this repo's deterministic-fast-tests convention, every test uses the
// injectable statWalkInterval (Task 2.1.1g) plus channel-signaled callbacks
// with a bounded select/time.After backstop -- never a real 15s wait and
// never a time.Sleep poll loop.

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// testStatWalkInterval is the short interval injected into every detector
// under test via setStatWalkInterval, so tests run in milliseconds instead
// of waiting out the real 15s changeDetectionStatWalkInterval.
const testStatWalkInterval = 15 * time.Millisecond

// waitForSignal blocks until ch receives a value or bound elapses, returning
// whether a value was received. A single bounded channel wait (not a
// time.Sleep poll loop) -- the accepted deterministic-fast-tests shape for
// "assert an async event does/doesn't happen".
func waitForSignal(t *testing.T, ch <-chan struct{}, bound time.Duration) bool {
	t.Helper()
	select {
	case <-ch:
		return true
	case <-time.After(bound):
		return false
	}
}

// mustMkGitDir creates a `.git` subdirectory under root so
// fsnotify.Watcher.Add() succeeds against it, letting a test exercise the
// real .git-watch-active path rather than always falling back to
// periodic-only.
func mustMkGitDir(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
}

// TestWorktreeChangeDetector_FiresOnDirtyFlip covers Story 2.1.1's
// edge-triggered-fire acceptance criterion: a fake fingerprint starts at
// (false,"sha1") and, once flipped to (true,"sha1"), fires OnChange exactly
// once -- not before the flip, and not repeatedly after it.
func TestWorktreeChangeDetector_FiresOnDirtyFlip(t *testing.T) {
	t.Parallel()

	var dirty atomic.Bool
	fp := func() (bool, string, error) { return dirty.Load(), "sha1", nil }

	d := NewWorktreeChangeDetector(t.TempDir(), fp, nil)
	d.setStatWalkInterval(testStatWalkInterval)

	fireCh := make(chan struct{}, 8)
	d.OnChange(func() { fireCh <- struct{}{} })
	d.Start()
	defer d.Stop()

	require.False(t, waitForSignal(t, fireCh, 5*testStatWalkInterval),
		"OnChange must not fire before the dirty flag flips")

	dirty.Store(true)
	require.True(t, waitForSignal(t, fireCh, 2*time.Second),
		"OnChange must fire once the dirty flag flips")

	require.False(t, waitForSignal(t, fireCh, 5*testStatWalkInterval),
		"OnChange must fire exactly once per flip, not repeatedly on later ticks")
}

// TestWorktreeChangeDetector_NoFireWhenFingerprintUnchanged covers Story
// 2.1.1's "Start() takes a baseline synchronously, no spurious fire"
// acceptance criterion: a fingerprint that never changes must never fire
// OnChange across several ticks.
func TestWorktreeChangeDetector_NoFireWhenFingerprintUnchanged(t *testing.T) {
	t.Parallel()

	fp := func() (bool, string, error) { return false, "sha1", nil }

	d := NewWorktreeChangeDetector(t.TempDir(), fp, nil)
	d.setStatWalkInterval(testStatWalkInterval)

	fireCh := make(chan struct{}, 8)
	d.OnChange(func() { fireCh <- struct{}{} })
	d.Start()
	defer d.Stop()

	require.False(t, waitForSignal(t, fireCh, 8*testStatWalkInterval),
		"OnChange must never fire when the fingerprint never changes across several ticks")
}

// TestWorktreeChangeDetector_GitWatchUnavailableStillRunsPeriodicLoop covers
// Story 2.1.1's ".git watch failure degrades to periodic-only" acceptance
// criterion: worktreePath has no .git subdirectory, so watcher.Add() fails
// (ENOENT) -- GitWatchActive() must report false, but the periodic loop must
// still fire on a fingerprint change.
func TestWorktreeChangeDetector_GitWatchUnavailableStillRunsPeriodicLoop(t *testing.T) {
	t.Parallel()

	worktreePath := t.TempDir() // deliberately no .git subdirectory

	var dirty atomic.Bool
	fp := func() (bool, string, error) { return dirty.Load(), "sha1", nil }

	d := NewWorktreeChangeDetector(worktreePath, fp, nil)
	d.setStatWalkInterval(testStatWalkInterval)

	fireCh := make(chan struct{}, 8)
	d.OnChange(func() { fireCh <- struct{}{} })
	d.Start()
	defer d.Stop()

	require.False(t, d.GitWatchActive(), "GitWatchActive must be false when the .git Add() fails")

	dirty.Store(true)
	require.True(t, waitForSignal(t, fireCh, 2*time.Second),
		"the periodic loop must still fire on a fingerprint change when the .git watch is unavailable")
}

// TestWorktreeChangeDetector_StopReleasesGoroutines covers Story 2.1.1's
// "Stop() releases both goroutines, no leak" acceptance criterion via
// goleak, mirroring actor_test.go's IgnoreCurrent()-baseline pattern (this
// test runs inside the large `session` package's shared test binary, so a
// bare process-wide goleak.VerifyNone would false-positive on unrelated
// long-lived goroutines from other tests). Constructs the worktree with a
// real .git subdirectory so both the periodic-loop and the git-watch-loop
// goroutines are started and must both be joined by Stop().
func TestWorktreeChangeDetector_StopReleasesGoroutines(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	worktreePath := t.TempDir()
	mustMkGitDir(t, worktreePath)

	fp := func() (bool, string, error) { return false, "sha1", nil }
	d := NewWorktreeChangeDetector(worktreePath, fp, nil)
	d.setStatWalkInterval(testStatWalkInterval)
	d.Start()
	require.True(t, d.GitWatchActive(), "expected the .git watch to start against a real .git directory")
	d.Stop()
}

// TestWorktreeChangeDetector_PanickingCallbackDoesNotStopOtherCallbacksOrDetector
// covers Story 2.1.1's panic-isolation acceptance criterion: two OnChange
// callbacks are registered, the first panics unconditionally on every fire,
// the second increments a counter. Two successive fingerprint flips must
// both still run the second callback, proving the detector's own goroutine
// (not just one fire() call) survives the panic.
func TestWorktreeChangeDetector_PanickingCallbackDoesNotStopOtherCallbacksOrDetector(t *testing.T) {
	t.Parallel()

	var state atomic.Int32 // toggled by the test to flip the fingerprint
	fp := func() (bool, string, error) { return state.Load()%2 == 1, "sha1", nil }

	d := NewWorktreeChangeDetector(t.TempDir(), fp, nil)
	d.setStatWalkInterval(testStatWalkInterval)

	var counter int32
	signalCh := make(chan struct{}, 8)
	d.OnChange(func() { panic("boom") })
	d.OnChange(func() {
		atomic.AddInt32(&counter, 1)
		signalCh <- struct{}{}
	})
	d.Start()
	defer d.Stop()

	state.Store(1)
	require.True(t, waitForSignal(t, signalCh, 2*time.Second),
		"the second callback must run despite the first callback panicking")
	require.EqualValues(t, 1, atomic.LoadInt32(&counter))

	state.Store(0)
	require.True(t, waitForSignal(t, signalCh, 2*time.Second),
		"the second callback must run again on a second flip, proving the detector's goroutine survived the first panic")
	require.EqualValues(t, 2, atomic.LoadInt32(&counter))
}

// TestWorktreeChangeDetector_ActiveFuncGatesPeriodicFingerprinting covers
// Story 2.1.2's activeFunc gate from both sides: a permanently-cold worktree
// (activeFunc==false) must suppress fingerprint() on every periodic tick
// after Start()'s unconditional baseline read and never fire OnChange
// (core acceptance criterion); a permanently-warm one (activeFunc==true)
// must keep calling fingerprint() every tick, same as before Story 2.1.2
// existed (Task 2.1.2a's regression guard).
func TestWorktreeChangeDetector_ActiveFuncGatesPeriodicFingerprinting(t *testing.T) {
	tests := []struct {
		name       string
		active     bool
		wantTicked bool // whether fingerprint() should be called beyond the Start()-time baseline
	}{
		{name: "cold worktree suppresses tick fingerprinting", active: false, wantTicked: false},
		{name: "warm worktree keeps fingerprinting every tick", active: true, wantTicked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertActiveFuncGatesTicking(t, tt.active, tt.wantTicked)
		})
	}
}

// assertActiveFuncGatesTicking starts a detector whose fingerprint always
// reports "not dirty" (so OnChange must never fire regardless of active) and
// asserts whether the periodic loop keeps calling fingerprint() beyond
// Start()'s synchronous baseline read, per wantTicked.
func assertActiveFuncGatesTicking(t *testing.T, active, wantTicked bool) {
	t.Helper()

	var calls int32
	tickCh := make(chan struct{}, 32)
	fp := func() (bool, string, error) {
		n := atomic.AddInt32(&calls, 1)
		if n > 1 { // skip Start()'s synchronous baseline read
			select {
			case tickCh <- struct{}{}:
			default:
			}
		}
		return false, "sha1", nil
	}
	activeFunc := func() bool { return active }

	d := NewWorktreeChangeDetector(t.TempDir(), fp, activeFunc)
	d.setStatWalkInterval(testStatWalkInterval)

	fireCh := make(chan struct{}, 8)
	d.OnChange(func() { fireCh <- struct{}{} })
	d.Start()
	defer d.Stop()

	require.False(t, waitForSignal(t, fireCh, 8*testStatWalkInterval),
		"fingerprint never reports dirty, so OnChange must never fire")

	ticked := waitForSignal(t, tickCh, 2*time.Second)
	require.Equal(t, wantTicked, ticked,
		"activeFunc()==%v must gate whether periodic ticks call fingerprint() beyond the baseline read", active)
	if wantTicked {
		require.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(2))
	} else {
		require.EqualValues(t, 1, atomic.LoadInt32(&calls),
			"activeFunc()==false must suppress every tick's fingerprint() call after the baseline read")
	}
}
