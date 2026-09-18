package tymux

import (
	"context"
	"fmt"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// resyncClearAndHomeSeq resets a terminal to a blank state before a resync
// redraw (ANSI "clear screen" + "cursor home"), so ClientFanout
// subscribers — each running their own vt100-style parser (e.g. xterm.js)
// over the raw bytes this package forwards — don't compose the redrawn
// snapshot on top of stale content that arrived before the gap/reconnect.
const resyncClearAndHomeSeq = "\x1b[2J\x1b[H"

// applySnapshotResync is the innermost shared step of the resync path
// (Task 2.5.2b): given a fresh *v1.PaneSnapshot from any source, it
// reseeds the cached liveness and broadcasts a full-screen SGR redraw to
// every ClientFanout subscriber — "one mechanism, not two" (pitfalls.md
// §5), used by both entry points below.
func (s *tymuxGRPCSession) applySnapshotResync(snap *v1.PaneSnapshot) error {
	s.mu.Lock()
	s.liveness = snap.GetLiveness()
	s.mu.Unlock()

	sgr, err := CellsToSGR(snap.GetGrid())
	if err != nil {
		return fmt.Errorf("tymux: resync: CellsToSGR: %w", err)
	}
	s.fanout.Broadcast([]byte(resyncClearAndHomeSeq + sgr))
	return nil
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
		return s.applySnapshotResync(snap)
	}
	return s.resyncViaCapturePane(ctx)
}
