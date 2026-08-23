package tymux

import (
	"context"
	"fmt"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// openStandingStream opens the one Attach stream that backs a
// tymuxGRPCSession for its whole lifetime (Story 2.3.1) — called once from
// cacheFromSession, the common tail of Start/RestoreWithWorkDir, never
// per-SendKeys-call. Tears down any previously open stream first, so a
// second Start/RestoreWithWorkDir call (e.g. a reconnect) never leaks the
// prior stream's reader goroutine.
func (s *tymuxGRPCSession) openStandingStream(paneID string) error {
	s.teardownStandingStream()

	ctx, cancel := context.WithCancel(context.Background())
	stream := s.transport.Attach(ctx)
	if stream == nil {
		cancel()
		return fmt.Errorf("tymux: Attach: transport returned a nil stream")
	}
	if err := stream.Send(&v1.AttachRequest{
		Payload: &v1.AttachRequest_PaneId{PaneId: paneID},
	}); err != nil {
		cancel()
		return classifyRPCError("Attach", err)
	}

	done := make(chan struct{})
	s.mu.Lock()
	s.stream = stream
	s.cancelAttach = cancel
	s.streamDone = done
	s.mu.Unlock()

	go s.readAttachLoop(stream, done)
	return nil
}

// teardownStandingStream cancels the currently open standing stream (if
// any) and waits for its reader goroutine to exit. Safe to call when no
// stream is open (no-op).
func (s *tymuxGRPCSession) teardownStandingStream() {
	s.mu.Lock()
	cancel := s.cancelAttach
	done := s.streamDone
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// readAttachLoop is the standing stream's reader goroutine (Task 2.3.1b):
// on Snapshot (Epic 1.3's priming event), seed local liveness state; on
// Output, forward to ClientFanout; on Exited, update cached liveness and
// fire the registered exit callback (Epic 2.4 wires SetOnExitCallback's
// fire-once/re-registration semantics; this just calls whatever is
// currently registered); on OutputGap, this is the resync trigger point —
// Epic 2.5 implements the shared resync path, so today this only logs
// nothing and lets normal streaming resume, matching the proto's own
// "not a fatal condition" contract. Closes done on any terminal error
// (including context cancellation from DetachSafely/teardownStandingStream)
// so Attach()'s returned channel and any WaitGroup-style caller unblocks.
func (s *tymuxGRPCSession) readAttachLoop(stream attachStream, done chan struct{}) {
	defer close(done)
	for {
		event, err := stream.Receive()
		if err != nil {
			return
		}
		switch p := event.GetPayload().(type) {
		case *v1.AttachEvent_Snapshot:
			s.mu.Lock()
			s.liveness = p.Snapshot.GetLiveness()
			s.mu.Unlock()
		case *v1.AttachEvent_Output:
			s.fanout.Broadcast(p.Output)
		case *v1.AttachEvent_Exited:
			s.mu.Lock()
			s.liveness = v1.Liveness_LIVENESS_DEAD
			cb := s.exitCallback
			s.mu.Unlock()
			if cb != nil {
				cb(exitReason(p.Exited))
			}
		case *v1.AttachEvent_OutputGap:
			// Epic 2.5 implements the shared resync path triggered here.
		}
	}
}

// exitReason renders an ExitStatus (ADR-001: an absent code is a
// first-class "exited, but no code known" state, never fabricated as 0)
// into the reason string SetOnExitCallback's callers expect, matching
// TmuxSession.SetOnExitCallback's func(reason string) shape.
func exitReason(status *v1.ExitStatus) string {
	if status == nil || status.Code == nil {
		return "exited: code=unknown"
	}
	return fmt.Sprintf("exited: code=%d", status.GetCode())
}

// sendOnStream writes req to the standing stream's input side. Every
// input method (SendKeys/TapEnter/SendPromptWithEnter/
// SendInputViaControlMode, Story 2.3.3) and SetWindowSize/SetDetachedSize
// (Epic 2.4) funnel through here rather than opening a new RPC.
func (s *tymuxGRPCSession) sendOnStream(req *v1.AttachRequest) error {
	s.mu.RLock()
	stream := s.stream
	s.mu.RUnlock()
	if stream == nil {
		return errSessionNotStarted
	}
	if err := stream.Send(req); err != nil {
		return classifyRPCError("Attach.Send", err)
	}
	return nil
}
