package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"go.uber.org/goleak"
)

// UT-21: TestMarkSessionNeedsAttention_NilReviewQueue — nil queue must not panic.
func TestMarkSessionNeedsAttention_NilReviewQueue(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "test-nil-queue",
		UUID:        "test-uuid-1",
		reviewQueue: nil,
	}
	// Should not panic.
	markSessionNeedsAttention(inst, "test reason")
}

// TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak
// is the regression test for the BLOCKER fix: StopSessionDriver must not return
// while a handleDriverFailure-spawned retry continuation is still running.
//
// Before the fix, StopSessionDriver waited on a per-run sessionDriverStopper.done
// channel that was only closed by the *original* run's goroutine (via
// StartSessionDriver's defer). handleDriverFailure spawns a second, untracked-by-
// `done` goroutine to continue the run after a restart and returns immediately
// (see handleDriverFailure's doc comment: the caller "must return immediately"),
// so the original goroutine's defer fired and closed `done` while the retry
// goroutine was still alive — StopSessionDriver returned early, reintroducing the
// exact "goroutine outlives Destroy()" bug this package exists to prevent.
//
// This test reproduces the exact interleaving handleDriverFailure produces
// (Add(1) for the retry BEFORE the original goroutine's Done() fires) without
// depending on real tmux/session restart plumbing: the fix is in StopSessionDriver
// and inst.driverWG's bookkeeping, not in handleDriverFailure's business logic.
// simulateHandleDriverFailureRetryInterleaving reproduces the exact
// interleaving handleDriverFailure produces: Add(1) and spawn a retry
// continuation goroutine BEFORE the simulated original goroutine's own
// Done() fires — the sequencing that broke the old done-channel-based
// StopSessionDriver. The returned channels let the caller observe the retry
// goroutine starting and release it when done.
func simulateHandleDriverFailureRetryInterleaving(inst *Instance, stopper *sessionDriverStopper) (retryStarted, retryFinish chan struct{}) {
	retryStarted = make(chan struct{})
	retryFinish = make(chan struct{})

	inst.driverWG.Add(1)
	go func() {
		defer inst.driverWG.Done()
		<-stopper.stop

		inst.driverWG.Add(1)
		go func() {
			defer inst.driverWG.Done()
			close(retryStarted)
			<-retryFinish
		}()
	}()
	return retryStarted, retryFinish
}

func TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(sessiontest.KnownBackgroundGoroutines, baseline)...)

	inst := &Instance{Title: "test-stop-waits-for-retry"}
	stopper := &sessionDriverStopper{stop: make(chan struct{})}
	inst.driverStopper.Store(stopper)

	retryStarted, retryFinish := simulateHandleDriverFailureRetryInterleaving(inst, stopper)

	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		StopSessionDriver(inst)
	}()

	select {
	case <-retryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("simulated retry goroutine never started")
	}

	select {
	case <-stopDone:
		t.Fatal("StopSessionDriver returned before the handleDriverFailure-style retry goroutine finished")
	case <-time.After(200 * time.Millisecond):
	}

	close(retryFinish)

	select {
	case <-stopDone:
	case <-time.After(driverStopTimeout + 2*time.Second):
		t.Fatal("StopSessionDriver did not return after the retry goroutine finished")
	}
}
