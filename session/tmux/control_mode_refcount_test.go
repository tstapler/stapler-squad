package tmux

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor"
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
		// StartControlMode's checkControlModeVersionMatchOnce needs a real
		// cmdExec — left unset (nil interface), it panics the first time a
		// test in this file reaches the uncached branch (real bug found via
		// PR #739 CI: nil cmdExec, not a flake — reproduces deterministically
		// once versionCheckedSockets hasn't already memoized this test's
		// empty-string socket from an earlier test in the same run).
		cmdExec: executor.MakeExecutor(),
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// waitForSlowSendResolved blocks until subscriberID's drainSlowSubscriber goroutine (if any)
// has finished, bounded by timeout.
func waitForSlowSendResolved(t *testing.T, sess *TmuxSession, subscriberID string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		sess.controlModeSubMu.RLock()
		inFlight := sess.slowSendInFlight[subscriberID]
		sess.controlModeSubMu.RUnlock()
		if !inFlight {
			return
		}
		select {
		case <-deadline:
			t.Fatal("drainSlowSubscriber never finished")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestBroadcastControlModeUpdate_ClosesSlowSubscriber_When_ChannelFull is the regression
// test for the bug this fix addresses: a subscriber whose channel is full used to have its
// update silently dropped, which can strip partial escape sequences (cursor positioning,
// erase-line, SGR resets) from the exact byte stream rendered in the browser terminal,
// producing corrupted/overlapping rendering. The correct behavior — mirroring
// NativeProcessManager.fanOut in session/native_process_manager.go — is to close and remove
// the slow subscriber instead, so the consumer observes end-of-stream rather than silently
// missing bytes mid-stream.
func TestBroadcastControlModeUpdate_ClosesSlowSubscriber_When_ChannelFull(t *testing.T) {
	t.Parallel()
	sess := newRefcountTestSession(t)

	// Subscribe one client and never drain it, so its buffered channel fills up.
	slowID, slowCh := sess.SubscribeToControlModeUpdates()
	// Subscribe a second, healthy client that we drain in lockstep with the send loop
	// below. A background goroutine racing the tight send loop isn't guaranteed to keep
	// up — if it loses that race, healthyCh fills too and gets closed just like slowCh,
	// making the test flaky. Draining synchronously here guarantees healthyCh never holds
	// more than one buffered message, regardless of scheduling.
	healthyID, healthyCh := sess.SubscribeToControlModeUpdates()

	// Fill the slow subscriber's buffer (capacity 100) and then push past it so a
	// subsequent broadcast observes a full channel, while keeping the healthy
	// subscriber's channel drained.
	for i := 0; i < 101; i++ {
		sess.broadcastControlModeUpdate([]byte("x"))
		<-healthyCh
	}

	// The slow subscriber must be closed and removed, not left silently missing bytes.
	// The close happens on a background goroutine (drainSlowSubscriber) after its own
	// bounded wait, not inline within broadcastControlModeUpdate anymore — see that
	// function's doc comment for why. Wait for that goroutine to finish (bounded, well
	// past its grace period) WITHOUT reading from slowCh first: draining it here would
	// itself free room for the goroutine's held send to succeed, masking the exact
	// "consumer genuinely never drains" case this test exists to cover.
	waitForSlowSendResolved(t, sess, slowID, time.Second)

	// Now that the goroutine has resolved, its decision (close, since nothing drained
	// the channel above) is already locked in — safe to drain the 100 buffered items
	// still sitting ahead of the close signal without risking the earlier "reading here
	// masks a should-have-timed-out send" race, since there's nothing left in flight to
	// influence. Every receive is individually bounded so a regression (channel left
	// open) fails fast instead of hanging the test run.
	drainDeadline := time.After(time.Second)
drain:
	for {
		select {
		case _, ok := <-slowCh:
			if !ok {
				break drain
			}
		case <-drainDeadline:
			t.Fatal("slow subscriber's channel was never closed")
		}
	}

	sess.controlModeSubMu.RLock()
	_, slowExists := sess.controlModeSubscribers[slowID]
	_, healthyExists := sess.controlModeSubscribers[healthyID]
	sess.controlModeSubMu.RUnlock()

	if slowExists {
		t.Error("slow subscriber should have been removed after its channel filled up")
	}
	if !healthyExists {
		t.Error("healthy subscriber should not have been removed")
	}

	sess.UnsubscribeFromControlModeUpdates(healthyID)
}

// TestBroadcastControlModeUpdate_KeepsBurstySubscriberOpen_When_ConsumerCatchesUpWithinGracePeriod
// is the regression test for the bug fixed on top of the close-slow-subscriber change above:
// fast typing (readline/prompt redraws emit several %output events per keystroke) can fill the
// 100-slot buffer in a single burst while the consumer is mid-write on a coalesced WebSocket
// frame — that consumer is healthy and about to drain, not stuck. Closing on the very first
// instantaneously-full send disconnected the terminal for exactly this case. A subscriber that
// drains within the bounded grace period must NOT be closed.
func TestBroadcastControlModeUpdate_KeepsBurstySubscriberOpen_When_ConsumerCatchesUpWithinGracePeriod(t *testing.T) {
	t.Parallel()
	sess := newRefcountTestSession(t)

	id, ch := sess.SubscribeToControlModeUpdates()

	// Fill the buffer completely without draining, simulating a fast-typing burst that
	// produces more control-mode output than the 100-slot buffer holds before the consumer
	// gets its next scheduler turn.
	for i := 0; i < 100; i++ {
		sess.broadcastControlModeUpdate([]byte("x"))
	}

	// Simulate the consumer catching up concurrently rather than being fully drained
	// beforehand, so this exercises the actual race: room must open up mid-wait for the send
	// below. No artificial delay before draining — whenever the goroutine actually gets
	// scheduled, broadcastControlModeUpdate's 250ms grace period only needs it to land one
	// drain within that window, so this holds regardless of scheduler contention. (An earlier
	// version slept 10ms before draining; under heavy CPU contention across a full -race test
	// run, that fixed delay left far less margin than draining immediately does, and caused a
	// rare flake.)
	go func() {
		for i := 0; i < 100; i++ {
			<-ch
		}
	}()

	// This call observes the channel full and hands the grace-period wait off to a
	// background goroutine (drainSlowSubscriber) rather than blocking here — see
	// broadcastControlModeUpdate's doc comment for why. So this returns immediately;
	// wait for that goroutine to finish (bounded, well past the 250ms grace period)
	// before asserting on the outcome.
	sess.broadcastControlModeUpdate([]byte("y"))

	waitForSlowSendResolved(t, sess, id, time.Second)

	sess.controlModeSubMu.RLock()
	_, exists := sess.controlModeSubscribers[id]
	sess.controlModeSubMu.RUnlock()

	if !exists {
		t.Error("subscriber was closed on a transient burst that cleared within the grace period; a bursty-but-healthy consumer must not be disconnected")
	}

	sess.UnsubscribeFromControlModeUpdates(id)
}

// TestBroadcastControlModeUpdate_DoesNotBlockCallerOnSlowSubscriber is the regression test
// for the actual bug this file's async rework fixes: broadcastControlModeUpdate is called
// synchronously, in-line, from the tmux control-mode read loop for every line tmux writes
// (readControlModeOutput's scanner.Scan() loop). Blocking that call for up to
// controlModeSlowSubscriberGrace (250ms) per stalled subscriber — the pre-fix behavior —
// stopped the read loop from calling scanner.Scan() again, which stopped it draining tmux's
// stdout. A tmux control-mode process blocked mid-write() (because nobody's reading its
// stdout) can't simultaneously notice its stdin was closed, which was forcing
// StopControlMode's 2-second wait-then-kill path on every single teardown of a busy session
// (confirmed empirically against a real tmux process: a plain `tmux -C attach-session` with
// no output backlog exits within ~1s of stdin EOF). The fix must make the caller return
// promptly regardless of how slow (or entirely absent) the subscriber's consumer is.
func TestBroadcastControlModeUpdate_DoesNotBlockCallerOnSlowSubscriber(t *testing.T) {
	t.Parallel()
	sess := newRefcountTestSession(t)

	// Subscribe a client and never drain it — the worst case, a completely stuck consumer.
	id, _ := sess.SubscribeToControlModeUpdates()

	// Fill its buffer.
	for i := 0; i < 100; i++ {
		sess.broadcastControlModeUpdate([]byte("x"))
	}

	// One more call now observes the channel full. Pre-fix, this blocked synchronously for
	// up to controlModeSlowSubscriberGrace (250ms) waiting for room that will never come.
	// Post-fix, it must return in negligible time regardless — the wait moved to a
	// background goroutine.
	start := time.Now()
	sess.broadcastControlModeUpdate([]byte("y"))
	elapsed := time.Since(start)

	const mustReturnWithin = 50 * time.Millisecond // well under controlModeSlowSubscriberGrace's 250ms
	if elapsed > mustReturnWithin {
		t.Errorf("broadcastControlModeUpdate blocked for %v on a stuck subscriber; must return within %v regardless of subscriber drain state", elapsed, mustReturnWithin)
	}

	sess.UnsubscribeFromControlModeUpdates(id)
}

// TestBroadcastControlModeUpdate_NeverSendsWhileOlderFrameStillDraining is the regression
// test for the CRITICAL frame-reordering fix: broadcastControlModeUpdate must check
// slowSendInFlight for a subscriber BEFORE ever attempting the fast-path send, so a newer
// frame can never win a freed buffer slot ahead of an older frame's still-in-flight
// drainSlowSubscriber goroutine. Deterministically simulates the race window (frame A's
// drain already marked in flight, one buffer slot freed, as if a consumer had drained one
// message) rather than relying on real goroutine scheduling timing, which would make the
// test itself flaky.
func TestBroadcastControlModeUpdate_NeverSendsWhileOlderFrameStillDraining(t *testing.T) {
	t.Parallel()
	sess := newRefcountTestSession(t)

	id, ch := sess.SubscribeToControlModeUpdates()

	// Fill to capacity, then free exactly one slot -- simulating a consumer having drained
	// one message while frame A's drainSlowSubscriber goroutine hasn't yet retried its send.
	for i := 0; i < 100; i++ {
		ch <- []byte("filler")
	}
	<-ch

	sess.controlModeSubMu.Lock()
	if sess.slowSendInFlight == nil {
		sess.slowSendInFlight = make(map[string]bool)
	}
	sess.slowSendInFlight[id] = true // simulates frame A's drain still in flight
	sess.controlModeSubMu.Unlock()

	// Frame B arrives while frame A is still draining. The freed slot exists -- pre-fix,
	// the fast-path select would grab it and deliver B ahead of A.
	sess.broadcastControlModeUpdate([]byte("B"))

	sess.controlModeSubMu.RLock()
	_, stillSubscribed := sess.controlModeSubscribers[id]
	sess.controlModeSubMu.RUnlock()

	if stillSubscribed {
		t.Fatal("subscriber with an in-flight drain should have been closed, not sent a new frame")
	}

	var received [][]byte
readLoop:
	for {
		select {
		case data := <-ch:
			received = append(received, data)
		case <-time.After(50 * time.Millisecond):
			break readLoop
		}
	}

	for _, data := range received {
		if string(data) == "B" {
			t.Fatal("frame B was delivered while an older frame was still draining -- reordering bug")
		}
	}
	if len(received) != 99 {
		t.Fatalf("expected exactly 99 buffered filler messages remaining, got %d", len(received))
	}
}

// TestBroadcastControlModeUpdate_UnsubscribeDuringSlowDrainDefersClose is the regression test
// for pendingCloseAfterDrain: Unsubscribe calling closeSubscriberLocked while a
// drainSlowSubscriber goroutine is still parked on `ch <- data` must defer the close to that
// goroutine, not close ch directly (which would race the blocked send and panic).
func TestBroadcastControlModeUpdate_UnsubscribeDuringSlowDrainDefersClose(t *testing.T) {
	t.Parallel()
	sess := newRefcountTestSession(t)

	id, ch := sess.SubscribeToControlModeUpdates()
	for i := 0; i < 100; i++ {
		sess.broadcastControlModeUpdate([]byte("x"))
	}
	sess.broadcastControlModeUpdate([]byte("y")) // spawns drainSlowSubscriber, returns immediately

	sess.controlModeSubMu.RLock()
	inFlight := sess.slowSendInFlight[id]
	sess.controlModeSubMu.RUnlock()
	if !inFlight {
		t.Fatal("expected drainSlowSubscriber still in flight right after broadcast returned")
	}

	sess.UnsubscribeFromControlModeUpdates(id) // must defer, not close ch here

	sess.controlModeSubMu.RLock()
	_, stillSubscribed := sess.controlModeSubscribers[id]
	_, deferred := sess.pendingCloseAfterDrain[id]
	sess.controlModeSubMu.RUnlock()
	if stillSubscribed {
		t.Error("Unsubscribe did not remove the subscriber immediately")
	}
	if !deferred {
		t.Error("Unsubscribe closed ch directly instead of deferring to the in-flight drainSlowSubscriber goroutine")
	}

	// drainSlowSubscriber must perform exactly one close(ch); a double-close would panic.
	deadline := time.After(time.Second)
drain:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drain
			}
		case <-deadline:
			t.Fatal("deferred close was never performed")
		}
	}
}
