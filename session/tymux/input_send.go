package tymux

import (
	"errors"
	"fmt"
	"time"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// Write path for the standing Attach stream.
//
// connect-go documents that BidiStreamForClient.Send is not safe for concurrent use, and
// concurrent calls on a real stream silently lose almost all input (8 writers delivered 1 of 1600
// bytes against a live tymuxd, with no error). Everything that writes to the stream — keystrokes,
// pastes, resizes, the PTY adapter — funnels through sendOnStream, which enforces two guarantees:
//
//  1. Frames never overlap: one Send is in flight per stream at a time (frame lock).
//  2. One logical input payload (a SendKeys/paste) is contiguous in the pane's input byte
//     stream: no other input is interleaved into it (input lock). A payload up to maxInputChunk
//     bytes is one frame, so a bracketed-paste unit or escape sequence that fits is never split.
//     A larger payload is cut into maxInputChunk frames in order; resize frames (a control message,
//     not in-band bytes) may interleave between them, other input may not.
//
// The locks belong to one stream generation (streamSendGate). A Send stuck on a dead transport
// therefore cannot block writes to the stream that replaced it after a reconnect.
// A single Send cannot be interrupted short of cancelling the stream; waiting for either lock and
// the total time of a multi-chunk payload are bounded so callers get an error instead of hanging.

const (
	// maxInputChunk is the largest single Input frame. 32 KiB keeps each Send short enough that
	// the frame lock is held briefly and a stalled pane shows up as slow chunks, not one
	// multi-megabyte write that blocks with nothing delivered.
	maxInputChunk = 32 * 1024

	defaultSendLockWait        = 30 * time.Second
	defaultInputPayloadTimeout = 2 * time.Minute
)

var (
	errSendBusy      = errors.New("tymux: input stream busy: timed out waiting to send")
	errInputBusy     = errors.New("tymux: input stream busy: timed out waiting for an earlier input to finish")
	errInputTimedOut = errors.New("tymux: input payload timed out")
)

// timedLock is a mutex whose acquisition can time out.
type timedLock chan struct{}

func newTimedLock() timedLock { return make(timedLock, 1) }

func (l timedLock) acquire(timeout time.Duration) bool {
	select {
	case l <- struct{}{}:
		return true
	default:
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case l <- struct{}{}:
		return true
	case <-t.C:
		return false
	}
}

func (l timedLock) release() { <-l }

// streamSendGate holds the write locks for one stream generation.
type streamSendGate struct {
	frame timedLock
	input timedLock
}

func newStreamSendGate() *streamSendGate {
	return &streamSendGate{frame: newTimedLock(), input: newTimedLock()}
}

// setStreamLocked installs stream and a fresh send gate. The caller holds s.mu.
func (s *tymuxGRPCSession) setStreamLocked(stream attachStream) {
	s.stream = stream
	s.sendGate = newStreamSendGate()
}

// sendOnStream writes req to the standing stream's input side. Every input method
// (SendKeys/TapEnter/SendPromptWithEnter/SendInputViaControlMode), the PTY adapter and
// SetWindowSize/SetDetachedSize funnel through here rather than opening a new RPC.
func (s *tymuxGRPCSession) sendOnStream(req *v1.AttachRequest) error {
	s.mu.RLock()
	stream, gate := s.stream, s.sendGate
	frameWait, payloadTimeout := s.sendLockWait, s.inputPayloadTimeout
	s.mu.RUnlock()
	if stream == nil {
		return errSessionNotStarted
	}
	if frameWait <= 0 {
		frameWait = defaultSendLockWait
	}
	if payloadTimeout <= 0 {
		payloadTimeout = defaultInputPayloadTimeout
	}
	if _, isInput := req.GetPayload().(*v1.AttachRequest_Input); isInput {
		return sendInputPayload(stream, gate, req.GetInput(), frameWait, payloadTimeout)
	}
	return sendFrame(stream, gate, req, frameWait)
}

func sendFrame(stream attachStream, gate *streamSendGate, req *v1.AttachRequest, wait time.Duration) error {
	if !gate.frame.acquire(wait) {
		return errSendBusy
	}
	defer gate.frame.release()
	if err := stream.Send(req); err != nil {
		return classifyRPCError("Attach.Send", err)
	}
	return nil
}

// sendInputPayload sends data as one contiguous logical input (see the guarantees above).
func sendInputPayload(stream attachStream, gate *streamSendGate, data []byte, wait, payloadTimeout time.Duration) error {
	if !gate.input.acquire(wait) {
		return errInputBusy
	}
	defer gate.input.release()

	deadline := time.Now().Add(payloadTimeout)
	sent := 0
	for {
		end := min(sent+maxInputChunk, len(data))
		if err := sendFrame(stream, gate, &v1.AttachRequest{
			Payload: &v1.AttachRequest_Input{Input: data[sent:end]},
		}, wait); err != nil {
			if sent > 0 {
				return fmt.Errorf("%w (after %d of %d bytes)", err, sent, len(data))
			}
			return err
		}
		sent = end
		if sent >= len(data) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: sent %d of %d bytes in %s", errInputTimedOut, sent, len(data), payloadTimeout)
		}
	}
}
