package tymux

import (
	"bytes"
	"context"
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
// Every wait also honors the caller's context, so a 2s browser-keystroke context is not held for
// the full lock wait behind a long paste.
//
// Small keystrokes deliberately do not bypass the input lock to interleave between a paste's
// chunks: that would splice typed bytes into the middle of the paste's byte stream. A busy input
// lock is instead reported (errInputBusy / the caller's context error) and the PTY adapter retries.

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
	errLockTimeout   = errors.New("tymux: lock wait timed out")

	bracketedPasteStart = []byte("\x1b[200~")
	bracketedPasteEnd   = []byte("\x1b[201~")
)

// timedLock is a mutex whose acquisition can time out.
type timedLock chan struct{}

func newTimedLock() timedLock { return make(timedLock, 1) }

// acquire waits for the lock for at most timeout or until ctx is done, whichever comes first.
// An already-cancelled ctx fails even when the lock is free.
func (l timedLock) acquire(ctx context.Context, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case l <- struct{}{}:
		return nil
	default:
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case l <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return errLockTimeout
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
	_, err := s.sendOnStreamCtx(context.Background(), req)
	return err
}

// sendOnStreamCtx is sendOnStream bounded by ctx. For an Input frame it also returns how many
// payload bytes were written, so a caller can tell a partial multi-chunk failure from a clean one.
func (s *tymuxGRPCSession) sendOnStreamCtx(ctx context.Context, req *v1.AttachRequest) (int, error) {
	s.mu.RLock()
	stream, gate := s.stream, s.sendGate
	frameWait, payloadTimeout := s.sendLockWait, s.inputPayloadTimeout
	s.mu.RUnlock()
	if stream == nil {
		return 0, errSessionNotStarted
	}
	if frameWait <= 0 {
		frameWait = defaultSendLockWait
	}
	if payloadTimeout <= 0 {
		payloadTimeout = defaultInputPayloadTimeout
	}
	if _, isInput := req.GetPayload().(*v1.AttachRequest_Input); isInput {
		return sendInputPayload(ctx, stream, gate, req.GetInput(), frameWait, payloadTimeout)
	}
	return 0, sendFrame(ctx, stream, gate, req, frameWait)
}

// lockErr maps a failed acquire to the package's busy error, leaving ctx errors untouched.
func lockErr(err, busy error) error {
	if errors.Is(err, errLockTimeout) {
		return busy
	}
	return err
}

func sendFrame(ctx context.Context, stream attachStream, gate *streamSendGate, req *v1.AttachRequest, wait time.Duration) error {
	if err := gate.frame.acquire(ctx, wait); err != nil {
		return lockErr(err, errSendBusy)
	}
	defer gate.frame.release()
	if err := stream.Send(req); err != nil {
		return classifyRPCError("Attach.Send", err)
	}
	return nil
}

// sendInputPayload sends data as one contiguous logical input (see the guarantees above) and
// returns the number of bytes written, which is less than len(data) only alongside an error.
func sendInputPayload(ctx context.Context, stream attachStream, gate *streamSendGate, data []byte, wait, payloadTimeout time.Duration) (int, error) {
	if err := gate.input.acquire(ctx, wait); err != nil {
		return 0, lockErr(err, errInputBusy)
	}
	defer gate.input.release()

	deadline := time.Now().Add(payloadTimeout)
	sent := 0
	for {
		end := min(sent+maxInputChunk, len(data))
		if err := sendFrame(ctx, stream, gate, &v1.AttachRequest{
			Payload: &v1.AttachRequest_Input{Input: data[sent:end]},
		}, wait); err != nil {
			return sent, abortPayload(stream, gate, data, sent, wait, err)
		}
		sent = end
		if sent >= len(data) {
			return sent, nil
		}
		if time.Now().After(deadline) {
			err := fmt.Errorf("%w: sent %d of %d bytes in %s", errInputTimedOut, sent, len(data), payloadTimeout)
			return sent, abortPayload(stream, gate, data, sent, wait, err)
		}
	}
}

// abortPayload annotates a mid-payload failure with how far it got and, if the bytes already
// delivered left the app inside a bracketed paste, best-effort closes it so the app does not keep
// swallowing keystrokes as paste text. Runs on a fresh context: the caller's is often why we failed.
func abortPayload(stream attachStream, gate *streamSendGate, data []byte, sent int, wait time.Duration, err error) error {
	if sent == 0 {
		return err
	}
	if inBracketedPaste(data[:sent]) {
		ctx, cancel := context.WithTimeout(context.Background(), min(wait, time.Second))
		defer cancel()
		_ = sendFrame(ctx, stream, gate, &v1.AttachRequest{
			Payload: &v1.AttachRequest_Input{Input: bracketedPasteEnd},
		}, wait)
	}
	return fmt.Errorf("%w (after %d of %d bytes)", err, sent, len(data))
}

func inBracketedPaste(delivered []byte) bool {
	return bytes.LastIndex(delivered, bracketedPasteStart) > bytes.LastIndex(delivered, bracketedPasteEnd)
}
