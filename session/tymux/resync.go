package tymux

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// applySnapshotResync is the innermost shared step of the resync path
// (Task 2.5.2b): given a fresh *v1.PaneSnapshot from any source, it
// reseeds the cached liveness and broadcasts a full-screen SGR redraw to
// every ClientFanout subscriber — "one mechanism, not two" (pitfalls.md
// §5), used by both entry points below.
func (s *tymuxGRPCSession) applySnapshotResync(snap *v1.PaneSnapshot) error {
	s.mu.Lock()
	s.liveness = snap.GetLiveness()
	s.restartIsPending = false
	s.mu.Unlock()

	// RenderSnapshotRedraw upgrades CellsToSGR's bare "\n" row separators to "\r\n" itself:
	// this redraw goes to a live vt100 parser (xterm.js) over ClientFanout, where "\n" is Line
	// Feed only and rows would stagger — and it restores modes, title and cursor, not just cells.
	redraw, err := RenderSnapshotRedraw(snap)
	if err != nil {
		return fmt.Errorf("tymux: resync: RenderSnapshotRedraw: %w", err)
	}
	s.fanout.Broadcast([]byte(redraw))
	recordAttachEvent(attachEventSnapshotRedraw)
	return nil
}

const (
	// dropRepaintMinInterval spaces repaint captures, so a dead pane or a subscriber that keeps
	// overflowing cannot turn the daemon into a capture loop.
	dropRepaintMinInterval = 250 * time.Millisecond
	// dropRepaintMaxBackoff caps the wait after a failed capture.
	dropRepaintMaxBackoff = 4 * time.Second
	// dropRepaintGiveUpRetry is how long after a run gave up (the pane could not be captured)
	// before the still-flagged subscribers are tried again.
	dropRepaintGiveUpRetry = 30 * time.Second
	// dropRepaintMaxAttempts bounds one scheduled repaint run; the next dropped frame starts a new one.
	dropRepaintMaxAttempts = 5
)

// scheduleDropRepaint is the fanout's drop callback: a subscriber channel overflowed, so its
// byte stream has a hole the browser's terminal cannot detect. It repaints, in the background and
// coalesced, only the subscribers that lost output.
func (s *tymuxGRPCSession) scheduleDropRepaint() {
	if s.fanout.SeqValid() {
		s.requestStreamResync(resyncReasonDrop, ScopeDropped)
		return
	}
	if !s.dropRepaintPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		sid := s.GetSessionIdentifier()
		exhausted := repaintDroppedLoop(s.fanout, func() ([]byte, error) {
			ctx, cancel := s.interruptibleContext()
			defer cancel()
			snap, err := s.capturePaneNoRevive(ctx, 0)
			if err != nil {
				return nil, err
			}
			redraw, err := RenderSnapshotRedraw(snap)
			return []byte(redraw), err
		}, dropRepaintMinInterval, s.sleepUnlessAborted, func(n int) {
			recordAttachEvent(attachEventSnapshotRedraw)
			log.Warn("tymux: repainted subscribers after dropped output", "session_id", sid, "subscribers", n)
		}, func(err error) {
			log.Warn("tymux: repaint after dropped output failed", "session_id", sid, "error", err)
		})
		s.dropRepaintPending.Store(false)
		if s.fanout.HasDropped() {
			// A drop that landed after the loop's last look found the flag set; and a run that gave up
			// (dead pane) must not leave subscribers flagged with nothing to retry them.
			if exhausted {
				time.AfterFunc(dropRepaintGiveUpRetry, s.scheduleDropRepaint)
			} else {
				s.scheduleDropRepaint()
			}
		}
	}()
}

// sleepUnlessAborted waits d and reports false if the session was torn down meanwhile.
func (s *tymuxGRPCSession) sleepUnlessAborted(d time.Duration) bool {
	s.mu.RLock()
	abort := s.abortReconnect
	s.mu.RUnlock()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-abort:
		return false
	}
}

// repaintDroppedLoop repaints dropped subscribers until none is left or dropRepaintMaxAttempts
// passes ran, waiting minInterval between captures and backing off exponentially after a failed
// one. A sleep that returns false (session torn down) also ends it. It reports whether it stopped
// with subscribers still flagged; the caller must not retry immediately in that case. onFail runs for the first failure only, so a dead
// pane logs once per run, not once per pass.
func repaintDroppedLoop(f *ClientFanout, capture func() ([]byte, error), minInterval time.Duration,
	sleep func(time.Duration) bool, onRepainted func(n int), onFail func(error)) (exhausted bool) {
	backoff := minInterval
	failed := false
	for attempt := 0; attempt < dropRepaintMaxAttempts; attempt++ {
		ids := f.BeginRepaint()
		if len(ids) == 0 {
			return false
		}
		redraw, err := capture()
		if err != nil {
			f.FinishRepaint(ids, nil)
			if !failed {
				onFail(err)
				failed = true
			}
			if !sleep(backoff) {
				return f.HasDropped()
			}
			backoff = min(backoff*2, dropRepaintMaxBackoff)
			continue
		}
		if n := f.FinishRepaint(ids, redraw); n > 0 {
			onRepainted(n)
		}
		if !sleep(minInterval) {
			return f.HasDropped()
		}
	}
	return f.HasDropped()
}

// resyncViaCapturePane is the resync entry point for the OutputGap event
// handler (readAttachLoop, Task 2.5.2b): OutputGap carries no payload of
// its own (the proto's output_gap is a bare bool — "some output between
// the previous and next Output event was lost"), so an explicit
// CapturePane call is the only way to get a fresh full-screen snapshot to
// redraw from.
func (s *tymuxGRPCSession) resyncViaCapturePane(ctx context.Context) error {
	snap, err := s.capturePane(ctx, 0)
	if err != nil {
		return fmt.Errorf("tymux: resync: %w", err)
	}
	return s.applySnapshotResync(snap)
}

// resyncAfterReconnect is the resync entry point ReconnectLoop calls
// after a successful reattach (Task 2.5.2b), given the first AttachEvent
// dialAttach already received. ADR-003 guarantees a successful Attach's
// first message is always a priming Snapshot, taken atomically with the
// server subscribing this new stream to live output (subscribe-before-
// snapshot, with the amendment's sequence-based skip) — so that Snapshot
// IS already the correct fresh resync source.
//
// Deliberately does NOT issue a second, independent CapturePane call
// here the way resyncViaCapturePane does for OutputGap: a second RPC
// taken at a later instant than the Attach's own snapshot would open
// exactly the double-render race ADR-003's amendment fixed server-side
// for Attach itself — any output produced between the Attach's internal
// snapshot and a later separate CapturePane call would be reflected in
// that CapturePane's grid AND still sitting, un-consumed, in the
// stream's queued broadcast channel, about to be delivered again as an
// ordinary Output event on the very next Receive() call. Reusing the
// already-correctly-sequenced Attach snapshot avoids reintroducing that
// bug client-side (this is exactly what Task 2.5.2d's regression test —
// "ClientFanout's subscribers never receive the same byte range twice
// across the reconnect boundary" — checks for). Falls back to a fresh
// CapturePane only defensively, if the first event is ever not a
// Snapshot (never expected in practice given the ADR-003 guarantee).
func (s *tymuxGRPCSession) resyncAfterReconnect(ctx context.Context, first *v1.AttachEvent) error {
	if snap := first.GetSnapshot(); snap != nil {
		s.seedSequence(snap)
		if snap.Seq != nil {
			return s.applyPrimingSnapshot(snap)
		}
		return s.applySnapshotResync(snap)
	}
	// No priming snapshot to reseed from: a restarted daemon may number from lower seqs, which
	// BroadcastSeq would drop as duplicates.
	s.fanout.DisableSeq()
	return s.resyncViaCapturePane(ctx)
}

// seedSequence restarts sequenced tracking from a priming snapshot (the first event of every
// Attach): output after snap.Seq follows on this stream. A snapshot without a seq (older
// daemon) leaves the stream unsequenced.
func (s *tymuxGRPCSession) seedSequence(snap *v1.PaneSnapshot) {
	if snap.Seq == nil {
		s.fanout.DisableSeq()
		return
	}
	s.fanout.ResetSeq(snap.GetSeq())
}

// applyPrimingSnapshot is applySnapshotResync for a reattach's priming snapshot on a sequenced
// stream: subscribers' queues, which hold pre-disconnect output, are replaced by the redraw.
func (s *tymuxGRPCSession) applyPrimingSnapshot(snap *v1.PaneSnapshot) error {
	s.mu.Lock()
	s.liveness = snap.GetLiveness()
	s.restartIsPending = false
	s.mu.Unlock()
	redraw, err := RenderSnapshotRedraw(snap)
	if err != nil {
		return fmt.Errorf("tymux: resync: RenderSnapshotRedraw: %w", err)
	}
	if _, ok := s.fanout.ResyncAt(snap.GetSeq(), s.fanout.SeqGeneration(), []byte(redraw), ScopeAll); !ok {
		return fmt.Errorf("tymux: resync: sequenced stream not ready after reseed")
	}
	recordAttachEvent(attachEventSnapshotRedraw)
	return nil
}
