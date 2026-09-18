package streamhub

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// waitForBatch polls cond until it returns true or timeout elapses, the same
// pattern subscriber_test.go's waitFor uses in the external test package —
// redefined here because this file lives in the internal streamhub package
// (needed for direct access to StreamHub.batchWindow) and an internal test
// package can't see helpers declared in the streamhub_test package.
func waitForBatch(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}

// recordingTransport is a Transport that appends every delivered frame to a
// slice under a mutex — used where a test needs to assert exact delivered
// byte content, not just a count.
type recordingTransport struct {
	mu     sync.Mutex
	frames [][]byte
}

func (r *recordingTransport) Send(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Copy: BatchWindow/StreamHub hand off a buffer that must not be
	// retained past the Send call without copying (see batchScratchPool's
	// doc comment) — mirror that contract here rather than aliasing data.
	frame := append([]byte(nil), data...)
	r.frames = append(r.frames, frame)
	return nil
}

func (r *recordingTransport) Close() error { return nil }

func (r *recordingTransport) receivedFrames() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.frames))
	copy(out, r.frames)
	return out
}

func (r *recordingTransport) receivedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.frames)
}

// TestBatchWindow_should_ConcatenateBytesInExactOrder_When_MultipleAddCallsPrecedeFlush
// is the concatenation-integrity AC: BatchWindow must treat every Add call as
// an opaque byte range, concatenated verbatim in call order — never
// reordered, never truncated mid-event, never content-sniffed (Story 2.1.1's
// AC; Task 2.1.1d).
func TestBatchWindow_should_ConcatenateBytesInExactOrder_When_MultipleAddCallsPrecedeFlush(t *testing.T) {
	var mu sync.Mutex
	var got []BroadcastUnit
	bw := NewBatchWindow(func(unit BroadcastUnit) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, unit)
	}, WithMaxBatchWindow(time.Hour))

	// Includes a byte range that looks like a split ANSI CSI sequence
	// (ESC [ 3 1 m) fed across two separate Add calls — BatchWindow must
	// never inspect or reassemble escape sequences, only concatenate the
	// raw bytes in order.
	events := [][]byte{
		[]byte("hello "),
		[]byte("\x1b[31m"),
		[]byte("red text"),
		[]byte("\x1b[0m"),
		[]byte(" done"),
	}
	for _, e := range events {
		bw.Add(e)
	}
	bw.TryFlush()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 flushed unit, got %d", len(got))
	}
	want := bytes.Join(events, nil)
	if !bytes.Equal(got[0].Data, want) {
		t.Fatalf("flushed data = %q, want exact in-order concatenation %q", got[0].Data, want)
	}
}

// TestBatchWindow_should_ArmCeilingTimerExactlyOncePerWindow_When_MultipleAddCallsOccurBeforeFlush
// is the single-timer-per-hub AC: an arbitrarily long burst of Add calls
// within one still-pending window must arm the ceiling timer exactly once,
// not once per Add call — the invariant that keeps N subscribers from each
// paying independent timer drift (Task 2.1.1d).
func TestBatchWindow_should_ArmCeilingTimerExactlyOncePerWindow_When_MultipleAddCallsOccurBeforeFlush(t *testing.T) {
	bw := NewBatchWindow(func(BroadcastUnit) {}, WithMaxBatchWindow(time.Hour))

	for i := 0; i < 50; i++ {
		bw.Add([]byte("x"))
	}
	if got := bw.TimersArmed(); got != 1 {
		t.Fatalf("expected exactly 1 timer armed for 50 Add calls within one window, got %d", got)
	}

	// Flushing closes the window; the next Add starts a fresh one and must
	// arm a second, independent timer.
	bw.TryFlush()
	bw.Add([]byte("y"))
	if got := bw.TimersArmed(); got != 2 {
		t.Fatalf("expected a second timer armed for the next window, got %d", got)
	}
}

// TestBatchWindow_should_FlushWithOpportunisticReason_When_TryFlushCalledBeforeCeilingElapses
// covers the opportunistic side of Story 2.1.1's flush-reason AC.
func TestBatchWindow_should_FlushWithOpportunisticReason_When_TryFlushCalledBeforeCeilingElapses(t *testing.T) {
	var mu sync.Mutex
	var got []BroadcastUnit
	bw := NewBatchWindow(func(unit BroadcastUnit) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, unit)
	}, WithMaxBatchWindow(time.Hour)) // ceiling far away — only TryFlush can trigger this

	bw.Add([]byte("data"))
	bw.TryFlush()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 flushed unit, got %d", len(got))
	}
	if got[0].Reason != FlushOpportunistic {
		t.Fatalf("expected FlushOpportunistic, got %v", got[0].Reason)
	}
}

// TestBatchWindow_should_FlushWithCeilingReason_When_MaxWindowElapsesWithoutTryFlush
// covers the ceiling side of Story 2.1.1's flush-reason AC.
func TestBatchWindow_should_FlushWithCeilingReason_When_MaxWindowElapsesWithoutTryFlush(t *testing.T) {
	flushed := make(chan BroadcastUnit, 1)
	bw := NewBatchWindow(func(unit BroadcastUnit) {
		flushed <- unit
	}, WithMaxBatchWindow(5*time.Millisecond))

	bw.Add([]byte("data"))

	select {
	case unit := <-flushed:
		if unit.Reason != FlushCeiling {
			t.Fatalf("expected FlushCeiling, got %v", unit.Reason)
		}
		if !bytes.Equal(unit.Data, []byte("data")) {
			t.Fatalf("flushed data = %q, want %q", unit.Data, "data")
		}
	case <-time.After(time.Second):
		t.Fatal("expected a ceiling flush within 1s, got none")
	}
}

// TestBatchWindow_should_NotFlush_When_NothingIsBuffered is a guard against a
// spurious empty flush racing the ceiling timer against an opportunistic
// TryFlush that already drained the buffer (onCeilingExpired's re-check).
func TestBatchWindow_should_NotFlush_When_NothingIsBuffered(t *testing.T) {
	flushCount := 0
	bw := NewBatchWindow(func(BroadcastUnit) { flushCount++ }, WithMaxBatchWindow(time.Hour))

	bw.TryFlush() // nothing buffered yet — must be a no-op
	if flushCount != 0 {
		t.Fatalf("expected 0 flushes for an empty buffer, got %d", flushCount)
	}
}

// TestBatchWindow_should_DeliverBypassImmediately_When_BatchIsPending is the
// bypass-not-blocked-by-pending-batch AC (Story 2.1.2): a control/quiescence
// message must reach onFlush synchronously via Bypass, without waiting for
// whatever raw output happens to be mid-accumulation in the same window.
func TestBatchWindow_should_DeliverBypassImmediately_When_BatchIsPending(t *testing.T) {
	var mu sync.Mutex
	var got []BroadcastUnit
	bw := NewBatchWindow(func(unit BroadcastUnit) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, unit)
	}, WithMaxBatchWindow(time.Hour)) // ceiling never fires during this test

	bw.Add([]byte("pending raw output"))

	bw.Bypass([]byte("control message"))

	mu.Lock()
	if len(got) != 1 {
		mu.Unlock()
		t.Fatalf("expected the bypass message to flush immediately, got %d flushed units", len(got))
	}
	if got[0].Reason != FlushBypass {
		mu.Unlock()
		t.Fatalf("expected FlushBypass, got %v", got[0].Reason)
	}
	if !bytes.Equal(got[0].Data, []byte("control message")) {
		mu.Unlock()
		t.Fatalf("bypass data = %q, want %q (must not include the pending batch)", got[0].Data, "control message")
	}
	mu.Unlock()

	// The pending batch is untouched by the bypass and still flushes later
	// on its own schedule.
	bw.TryFlush()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected the pending batch to still flush after the bypass, got %d flushed units total", len(got))
	}
	if !bytes.Equal(got[1].Data, []byte("pending raw output")) {
		t.Fatalf("second flushed unit = %q, want the untouched pending batch %q", got[1].Data, "pending raw output")
	}
}

// TestBatchWindow_should_StampSequenceAtBroadcastTime_Not_AtBufferTime is
// Story 2.1.2's core ordering AC: a unit that started accumulating earlier
// must not receive a lower HubSequenceNumber than one that bypassed it
// later — the number reflects when a unit is actually handed to onFlush
// (broadcast time), not when its bytes first arrived.
func TestBatchWindow_should_StampSequenceAtBroadcastTime_Not_AtBufferTime(t *testing.T) {
	var mu sync.Mutex
	var got []BroadcastUnit
	bw := NewBatchWindow(func(unit BroadcastUnit) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, unit)
	}, WithMaxBatchWindow(time.Hour))

	// This batch starts accumulating first...
	bw.Add([]byte("older raw output, buffered first"))
	// ...but the bypass message is broadcast before the batch ever flushes.
	bw.Bypass([]byte("urgent control message"))
	// The batch is only flushed afterward.
	bw.TryFlush()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 flushed units, got %d", len(got))
	}
	bypassUnit, batchUnit := got[0], got[1]
	if bypassUnit.Reason != FlushBypass {
		t.Fatalf("expected first unit to be the bypass, got reason %v", bypassUnit.Reason)
	}
	if batchUnit.Reason == FlushBypass {
		t.Fatalf("expected second unit to be the batch flush, got reason %v", batchUnit.Reason)
	}
	if bypassUnit.Seq >= batchUnit.Seq {
		t.Fatalf("expected the earlier-broadcast bypass (Seq=%d) to receive a lower sequence number than the later-flushed batch (Seq=%d), despite the batch buffering first",
			bypassUnit.Seq, batchUnit.Seq)
	}
}

// TestBatchWindow_should_AssignStrictlyIncreasingSequenceNumbers_When_ManyUnitsFlushSequentially
// is a broader monotonicity check across a mix of opportunistic flushes and
// bypasses — no gaps, no duplicates, no regressions, regardless of which
// path produced each unit.
func TestBatchWindow_should_AssignStrictlyIncreasingSequenceNumbers_When_ManyUnitsFlushSequentially(t *testing.T) {
	var mu sync.Mutex
	var seqs []HubSequenceNumber
	bw := NewBatchWindow(func(unit BroadcastUnit) {
		mu.Lock()
		defer mu.Unlock()
		seqs = append(seqs, unit.Seq)
	}, WithMaxBatchWindow(time.Hour))

	for i := 0; i < 20; i++ {
		if i%2 == 0 {
			bw.Add([]byte("data"))
			bw.TryFlush()
		} else {
			bw.Bypass([]byte("control"))
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seqs) != 20 {
		t.Fatalf("expected 20 flushed units, got %d", len(seqs))
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("expected strictly increasing sequence numbers, got %d after %d at index %d", seqs[i], seqs[i-1], i)
		}
	}
}

// TestStreamHub_should_RunCoalesceStepExactlyOnce_When_ThreeSubscribersReceiveASharedBurst
// is Task 2.1.1e's call-counting AC: for a burst of many raw output events
// delivered to N>=3 attached subscribers within one MaxBatchWindow tick, the
// hub's accumulation/coalesce step (BatchWindow.flushLocked) must run
// exactly once for the whole burst — an Nx reduction versus today's
// per-connection design (server/services/connectrpc_websocket.go:790-845),
// where each of N subscribers runs its own coalesce loop against the same
// burst. This is about eliminating duplicated coalescing computation, not
// wire-message count (plan.md's corrected Story 2.1.1 success metric).
func TestStreamHub_should_RunCoalesceStepExactlyOnce_When_ThreeSubscribersReceiveASharedBurst(t *testing.T) {
	hub := NewStreamHub("test-session", nil,
		WithTeardownGrace(time.Hour),
		WithBatchMaxWindow(time.Hour), // ceiling never fires during the burst
	)
	defer hub.ForceTeardown()

	transports := []*recordingTransport{{}, {}, {}}
	for _, tr := range transports {
		hub.AttachSubscriber(tr, SubscriberCapability{})
	}

	const eventCount = 50
	var want []byte
	for i := 0; i < eventCount; i++ {
		event := []byte{byte('a' + i%26)}
		want = append(want, event...)
		hub.OnRawOutput(event)
	}

	// Nothing has been broadcast yet — the whole burst is still sitting in
	// the hub's single BatchWindow.
	if got := hub.batchWindow.FlushCount(); got != 0 {
		t.Fatalf("expected 0 flushes before any TryFlush/ceiling fires, got %d", got)
	}

	hub.batchWindow.TryFlush()

	if got := hub.batchWindow.FlushCount(); got != 1 {
		t.Fatalf("expected exactly 1 coalesce/flush step for a %d-event burst regardless of subscriber count, got %d", eventCount, got)
	}

	for i, tr := range transports {
		if !waitForBatch(t, time.Second, func() bool { return tr.receivedCount() == 1 }) {
			t.Fatalf("subscriber %d: expected exactly 1 delivered frame, got %d", i, tr.receivedCount())
		}
		frames := tr.receivedFrames()
		if !bytes.Equal(frames[0], want) {
			t.Fatalf("subscriber %d: delivered frame = %q, want the exact %d-event concatenation %q", i, frames[0], eventCount, want)
		}
	}
}
