package tmux

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// newRefcountTestSession builds a TmuxSession that looks like it has a running control mode
// process (controlModeCmd set, channels allocated) so we can test refcount logic without
// actually forking tmux. The cmd is a real already-exited process so Kill() is a no-op.
func newRefcountTestSession(t *testing.T) *TmuxSession {
	t.Helper()
	doneCh := make(chan struct{})
	// Use a real already-exited process so Kill() on it is a harmless no-op.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fakeCmd := exec.CommandContext(ctx, "true") //nolint:norawexec // process is immediately Wait()ed; Kill() on the already-dead process is a harmless no-op
	if err := fakeCmd.Start(); err != nil {
		t.Skipf("cannot start 'true': %v", err)
	}
	_ = fakeCmd.Wait() // already dead; Kill() on dead process is a no-op
	sess := &TmuxSession{
		sanitizedName:          "refcount_test",
		controlModeCmd:         fakeCmd,
		controlModeDone:        doneCh,
		controlModeRefCount:    1,
		controlModeSubscribers: make(map[string]chan []byte),
		highPriSendCh:          make(chan cmSendReq, 64),
		normPriSendCh:          make(chan cmSendReq, 256),
		cmSenderExited:         make(chan struct{}),
	}
	// Close cmSenderExited immediately so StopControlMode won't block waiting for the sender.
	close(sess.cmSenderExited)
	t.Cleanup(func() {
		select {
		case <-doneCh:
		default:
			close(doneCh)
		}
	})
	return sess
}

// TestRefcount_SecondStartIncrementsCount verifies that calling StartControlMode on a session
// that already has a running control mode process increments the refcount and returns nil
// without starting another process.
func TestRefcount_SecondStartIncrementsCount(t *testing.T) {
	sess := newRefcountTestSession(t)

	// Refcount is 1 (set in newRefcountTestSession to simulate a running process).
	originalCmd := sess.controlModeCmd

	// Second Start should increment refcount and return nil.
	if err := sess.StartControlMode(); err != nil {
		t.Fatalf("second StartControlMode returned error: %v", err)
	}

	sess.controlModeSubMu.RLock()
	count := sess.controlModeRefCount
	cmd := sess.controlModeCmd
	sess.controlModeSubMu.RUnlock()

	if count != 2 {
		t.Errorf("refcount after second Start = %d, want 2", count)
	}
	if cmd != originalCmd {
		t.Error("second StartControlMode replaced the running process; expected same cmd pointer")
	}
}

// TestRefcount_StopWithActiveClientsDoesNotKillProcess verifies that StopControlMode with
// remaining subscribers (refcount > 1) decrements the refcount but does NOT close
// controlModeDone or nil controlModeCmd.
func TestRefcount_StopWithActiveClientsDoesNotKillProcess(t *testing.T) {
	sess := newRefcountTestSession(t)

	// Simulate a second client: manually bump refcount to 2.
	sess.controlModeSubMu.Lock()
	sess.controlModeRefCount = 2
	sess.controlModeSubMu.Unlock()

	doneCh := sess.controlModeDone

	// First Stop: should decrement to 1, leave process running.
	if err := sess.StopControlMode(); err != nil {
		t.Fatalf("first StopControlMode returned error: %v", err)
	}

	sess.controlModeSubMu.RLock()
	count := sess.controlModeRefCount
	cmd := sess.controlModeCmd
	sess.controlModeSubMu.RUnlock()

	if count != 1 {
		t.Errorf("refcount after first Stop = %d, want 1", count)
	}
	if cmd == nil {
		t.Error("first StopControlMode nilled controlModeCmd; process should still be running")
	}
	select {
	case <-doneCh:
		t.Error("first StopControlMode closed controlModeDone; process should still be running")
	default:
	}
}

// TestRefcount_LastStopActuallyTearsDown verifies that when refcount reaches 0,
// StopControlMode closes controlModeDone, nils controlModeCmd, and zeroes the refcount.
func TestRefcount_LastStopActuallyTearsDown(t *testing.T) {
	sess := newRefcountTestSession(t)
	doneCh := sess.controlModeDone

	if err := sess.StopControlMode(); err != nil {
		t.Fatalf("final StopControlMode returned error: %v", err)
	}

	sess.controlModeSubMu.RLock()
	count := sess.controlModeRefCount
	cmd := sess.controlModeCmd
	sess.controlModeSubMu.RUnlock()

	if count != 0 {
		t.Errorf("refcount after final Stop = %d, want 0", count)
	}
	if cmd != nil {
		t.Error("final StopControlMode did not nil controlModeCmd")
	}
	select {
	case <-doneCh:
	case <-time.After(100 * time.Millisecond):
		t.Error("controlModeDone not closed after final Stop")
	}
}

// TestRefcount_UnderflowIsSafe verifies that calling StopControlMode when refcount is
// already 0 does not panic or underflow to a negative number.
func TestRefcount_UnderflowIsSafe(t *testing.T) {
	sess := newRefcountTestSession(t)
	// Manually set refcount=0 and cmd=nil to exercise the actual underflow guard
	// (not the early-return path that fires when controlModeCmd is nil).
	sess.controlModeSubMu.Lock()
	sess.controlModeRefCount = 0
	sess.controlModeCmd = nil
	sess.controlModeSubMu.Unlock()

	if err := sess.StopControlMode(); err != nil {
		t.Fatalf("StopControlMode with refcount=0 returned error: %v", err)
	}

	sess.controlModeSubMu.RLock()
	count := sess.controlModeRefCount
	sess.controlModeSubMu.RUnlock()

	// Count must not go negative — the guard should prevent the decrement.
	if count != 0 {
		t.Errorf("refcount after underflow Stop = %d, want exactly 0 (guard should prevent decrement)", count)
	}
}

// TestRefcount_ConcurrentStartsAreIdempotent verifies that concurrent StartControlMode
// calls on a session that already has a running process all return nil and only bump the
// refcount without forking additional processes.
func TestRefcount_ConcurrentStartsAreIdempotent(t *testing.T) {
	sess := newRefcountTestSession(t)
	originalCmd := sess.controlModeCmd

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = sess.StartControlMode()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d StartControlMode error: %v", i, err)
		}
	}

	sess.controlModeSubMu.RLock()
	count := sess.controlModeRefCount
	cmd := sess.controlModeCmd
	sess.controlModeSubMu.RUnlock()

	// 1 (initial) + 10 concurrent = 11
	if count != 11 {
		t.Errorf("refcount after 10 concurrent Starts = %d, want 11", count)
	}
	if cmd != originalCmd {
		t.Error("concurrent StartControlMode replaced the running process")
	}
}

// TestRefcount_ConcurrentStartStopAtBoundary races a Stop against a Start at the 1→0→1
// boundary and verifies the refcount never goes negative.
func TestRefcount_ConcurrentStartStopAtBoundary(t *testing.T) {
	sess := newRefcountTestSession(t)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = sess.StopControlMode()
	}()
	go func() {
		defer wg.Done()
		_ = sess.StartControlMode()
	}()
	wg.Wait()

	sess.controlModeSubMu.RLock()
	count := sess.controlModeRefCount
	sess.controlModeSubMu.RUnlock()
	if count < 0 {
		t.Errorf("refcount went negative: %d", count)
	}
}

// TestBroadcast_NoSendOnClosedPanic verifies that broadcastControlModeUpdate does not
// panic when UnsubscribeFromControlModeUpdates closes a channel concurrently.
// This is a regression test for the RACE #7 (send-on-closed-channel) fix.
func TestBroadcast_NoSendOnClosedPanic(t *testing.T) {
	sess := newRefcountTestSession(t)

	// Subscribe two clients.
	id1, ch1 := sess.SubscribeToControlModeUpdates()
	id2, _ := sess.SubscribeToControlModeUpdates()

	// Drain ch1 in background so the channel never fills up.
	go func() {
		for range ch1 {
		}
	}()

	// Concurrently: broadcast many messages while unsubscribing client 2.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// This should not panic even when ch2 is concurrently closed.
			sess.broadcastControlModeUpdate([]byte("hello"))
		}
	}()
	go func() {
		defer wg.Done()
		sess.UnsubscribeFromControlModeUpdates(id2)
	}()
	wg.Wait()

	// Post-condition: subscriber 1 should still be registered; subscriber 2 should be gone.
	sess.controlModeSubMu.RLock()
	_, id1Exists := sess.controlModeSubscribers[id1]
	_, id2Exists := sess.controlModeSubscribers[id2]
	sess.controlModeSubMu.RUnlock()

	if !id1Exists {
		t.Error("subscriber 1 was incorrectly removed during concurrent broadcast")
	}
	if id2Exists {
		t.Error("subscriber 2 was not removed by UnsubscribeFromControlModeUpdates")
	}

	// Clean up client 1.
	sess.UnsubscribeFromControlModeUpdates(id1)
}
