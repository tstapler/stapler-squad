package tmux

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

// newDispatchTestSession creates a TmuxSession wired to an in-memory pipe so that
// sendCMCommand can write to it and tests can feed fabricated CM response lines to
// processControlModeLine directly.
func newDispatchTestSession(t *testing.T) (*TmuxSession, io.WriteCloser) {
	t.Helper()
	pr, pw := io.Pipe()
	doneCh := make(chan struct{})
	sess := &TmuxSession{
		sanitizedName:    "test_session",
		controlModeStdin: pw,
		highPriSendCh:    make(chan cmSendReq, 64),
		normPriSendCh:    make(chan cmSendReq, 256),
		cmSenderExited:   make(chan struct{}),
	}
	// Drain the read side so writes don't block.
	go func() {
		io.Copy(io.Discard, pr)
	}()
	go sess.runCMSender(doneCh, pw)
	t.Cleanup(func() {
		close(doneCh)
		pw.Close()
	})
	return sess, pw
}

// enqueueChannels directly inserts result channels into pendingCmds in a known order,
// bypassing goroutine scheduling to test the state machine deterministically.
func enqueueChannels(sess *TmuxSession, n int) []chan cmdResult {
	channels := make([]chan cmdResult, n)
	for i := range channels {
		channels[i] = make(chan cmdResult, 1)
	}
	sess.controlModeSubMu.Lock()
	sess.pendingCmds = append(sess.pendingCmds, channels...)
	sess.controlModeSubMu.Unlock()
	return channels
}

func TestCMDispatch_SingleCommand(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		body, err := sess.sendCMCommand(ctx, "display-message", "-p", "test")
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- body
	}()

	// Wait for the goroutine to enqueue its command.
	if err := wait.WaitForCondition(func() bool {
		sess.controlModeSubMu.Lock()
		defer sess.controlModeSubMu.Unlock()
		return len(sess.pendingCmds) > 0
	}, wait.WaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond, Description: "pending command enqueued"}); err != nil {
		t.Fatalf("goroutine did not enqueue command: %v", err)
	}

	sess.processControlModeLine("%begin 1234 1 0")
	sess.processControlModeLine("output-line")
	sess.processControlModeLine("%end 1234 1 0")

	select {
	case body := <-resultCh:
		if body != "output-line" {
			t.Fatalf("expected 'output-line', got %q", body)
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for CM response")
	}
}

func TestCMDispatch_ResponseParsedFromBeginEnd(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	channels := enqueueChannels(sess, 1)

	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("line1")
	sess.processControlModeLine("line2")
	sess.processControlModeLine("line3")
	sess.processControlModeLine("%end 1 1 0")

	r := <-channels[0]
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	want := "line1\nline2\nline3"
	if r.body != want {
		t.Fatalf("expected %q, got %q", want, r.body)
	}
}

// TestCMDispatch_TwoCommandsQueuedInOrder verifies that the FIFO state machine delivers
// the first response to the first-queued channel and the second to the second-queued channel.
func TestCMDispatch_TwoCommandsQueuedInOrder(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	// Pre-populate in known order.
	channels := enqueueChannels(sess, 2)
	chA, chB := channels[0], channels[1]

	// Feed two responses in FIFO order.
	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("resp-A")
	sess.processControlModeLine("%end 1 1 0")

	sess.processControlModeLine("%begin 1 2 0")
	sess.processControlModeLine("resp-B")
	sess.processControlModeLine("%end 1 2 0")

	rA := <-chA
	if rA.err != nil || rA.body != "resp-A" {
		t.Fatalf("A: want body 'resp-A', got %q err %v", rA.body, rA.err)
	}
	rB := <-chB
	if rB.err != nil || rB.body != "resp-B" {
		t.Fatalf("B: want body 'resp-B', got %q err %v", rB.body, rB.err)
	}
}

// TestCMDispatch_ConcurrentCommandsArriveFIFO verifies FIFO ordering under concurrent
// goroutines by pre-seeding pendingCmds in known order and checking each channel gets
// the correspondingly ordered response.
func TestCMDispatch_ConcurrentCommandsArriveFIFO(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	const n = 10
	channels := enqueueChannels(sess, n)

	// Feed n response blocks in order.
	for j := 0; j < n; j++ {
		sess.processControlModeLine(fmt.Sprintf("%%begin 1 %d 0", j+1))
		sess.processControlModeLine(fmt.Sprintf("resp-%d", j))
		sess.processControlModeLine(fmt.Sprintf("%%end 1 %d 0", j+1))
	}

	// Each channel should receive the response at the same FIFO position.
	for i, ch := range channels {
		r := <-ch
		if r.err != nil {
			t.Errorf("channel %d: unexpected error %v", i, r.err)
		}
		want := fmt.Sprintf("resp-%d", i)
		if r.body != want {
			t.Errorf("channel %d: want %q, got %q", i, want, r.body)
		}
	}
}

// TestCMDispatch_ConcurrentSendCMCommand verifies that concurrent sendCMCommand callers
// all receive a non-error response under real goroutine concurrency.
func TestCMDispatch_ConcurrentSendCMCommand(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	const n = 8
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type res struct {
		body string
		err  error
	}
	results := make([]chan res, n)
	for i := range results {
		results[i] = make(chan res, 1)
	}

	var startWg sync.WaitGroup
	startWg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			startWg.Done()
			body, err := sess.sendCMCommand(ctx, fmt.Sprintf("cmd-%d", i))
			results[i] <- res{body, err}
		}()
	}
	startWg.Wait()
	// Wait for all n goroutines to enqueue their commands.
	if err := wait.WaitForCondition(func() bool {
		sess.controlModeSubMu.Lock()
		defer sess.controlModeSubMu.Unlock()
		return len(sess.pendingCmds) >= n
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 5 * time.Millisecond, Description: fmt.Sprintf("all %d commands enqueued", n)}); err != nil {
		t.Fatalf("goroutines did not enqueue %d commands: %v", n, err)
	}

	// Feed n responses — each goroutine gets one, FIFO.
	for j := 0; j < n; j++ {
		sess.processControlModeLine(fmt.Sprintf("%%begin 1 %d 0", j+1))
		sess.processControlModeLine(fmt.Sprintf("body-%d", j))
		sess.processControlModeLine(fmt.Sprintf("%%end 1 %d 0", j+1))
	}

	bodies := make(map[string]bool)
	for i := 0; i < n; i++ {
		select {
		case r := <-results[i]:
			if r.err != nil {
				t.Errorf("goroutine %d: unexpected error %v", i, r.err)
			}
			if r.body == "" {
				t.Errorf("goroutine %d: empty body", i)
			}
			if bodies[r.body] {
				t.Errorf("goroutine %d: duplicate body %q", i, r.body)
			}
			bodies[r.body] = true
		case <-ctx.Done():
			t.Fatalf("timed out waiting for goroutine %d", i)
		}
	}
}

func TestCMDispatch_ErrorResponsePropagated(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	channels := enqueueChannels(sess, 1)

	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("error description here")
	sess.processControlModeLine("%error 1 1 0")

	r := <-channels[0]
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(r.err.Error(), "error description") {
		t.Fatalf("expected error to contain description, got %q", r.err.Error())
	}
}

func TestCMDispatch_OutputNotificationDuringCommandDoesNotCorruptQueue(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	// Set up a subscriber to receive broadcast output.
	subCh := make(chan []byte, 4)
	sess.controlModeSubMu.Lock()
	sess.controlModeSubscribers = map[string]chan []byte{"test-sub": subCh}
	sess.controlModeSubMu.Unlock()

	channels := enqueueChannels(sess, 1)

	sess.processControlModeLine("%begin 1 1 0")
	// %output arriving between %begin and %end must be broadcast, not added to body.
	sess.processControlModeLine("%output %0 \\150\\145\\154\\154\\157") // octal "hello"
	sess.processControlModeLine("body-line")
	sess.processControlModeLine("%end 1 1 0")

	r := <-channels[0]
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if r.body != "body-line" {
		t.Fatalf("expected 'body-line', got %q", r.body)
	}

	select {
	case data := <-subCh:
		if !strings.Contains(string(data), "hello") {
			t.Fatalf("expected broadcast to contain 'hello', got %q", string(data))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestCMDispatch_FallbackWhenControlModeNil(t *testing.T) {
	sess := &TmuxSession{
		sanitizedName:    "test",
		controlModeStdin: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := sess.sendCMCommand(ctx, "display-message", "-p", "test")
	if err != ErrControlModeNotRunning {
		t.Fatalf("expected ErrControlModeNotRunning, got %v", err)
	}
}

func TestCMDispatch_StopDrainsInflightCommands(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	channels := enqueueChannels(sess, 1)
	ch := channels[0]

	// Simulate EOF cleanup: drain pendingCmds with ErrControlModeStopped.
	sess.controlModeSubMu.Lock()
	for _, c := range sess.pendingCmds {
		select {
		case c <- cmdResult{err: ErrControlModeStopped}:
		default:
		}
	}
	sess.pendingCmds = nil
	sess.controlModeSubMu.Unlock()

	select {
	case r := <-ch:
		if r.err != ErrControlModeStopped {
			t.Fatalf("expected ErrControlModeStopped, got %v", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not drained")
	}
}

func TestCMDispatch_DoubleBeginResetsState(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	// Pre-populate two channels in known order.
	channels := enqueueChannels(sess, 2)
	chA, chB := channels[0], channels[1]

	// First %begin claims chA.
	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("partial")
	// Second %begin before %end — chA should get error; chB becomes current.
	sess.processControlModeLine("%begin 1 2 0")
	sess.processControlModeLine("resp-B")
	sess.processControlModeLine("%end 1 2 0")

	rA := <-chA
	if rA.err == nil {
		t.Error("expected error for A after double begin, got nil")
	}
	rB := <-chB
	if rB.err != nil || rB.body != "resp-B" {
		t.Fatalf("B: want 'resp-B', got %q err %v", rB.body, rB.err)
	}
}

func TestCMFeatureFlag_OffUsesSubprocess(t *testing.T) {
	// Save and restore flag.
	prev := cmCommandsEnabled.Load()
	cmCommandsEnabled.Store(false)
	defer cmCommandsEnabled.Store(prev)

	callCount := 0
	mock := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			callCount++
			return []byte("80 24\n"), nil
		},
	}
	sess := &TmuxSession{
		sanitizedName:    "test",
		cmdExec:          mock,
		controlModeStdin: fakeWriteCloser{}, // non-nil, but flag is off
	}

	w, h, err := sess.GetPaneDimensions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 80 || h != 24 {
		t.Fatalf("expected 80x24, got %dx%d", w, h)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 subprocess call, got %d", callCount)
	}
}

func TestCMFeatureFlag_OnUsesCMPath(t *testing.T) {
	// Save and restore flag.
	prev := cmCommandsEnabled.Load()
	cmCommandsEnabled.Store(true)
	defer cmCommandsEnabled.Store(prev)

	subprocessCalled := false
	mock := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			subprocessCalled = true
			return []byte("80 24\n"), nil
		},
	}

	pr, pw := io.Pipe()
	doneCh := make(chan struct{})
	sess := &TmuxSession{
		sanitizedName:    "test",
		cmdExec:          mock,
		controlModeStdin: pw,
		highPriSendCh:    make(chan cmSendReq, 64),
		normPriSendCh:    make(chan cmSendReq, 256),
		cmSenderExited:   make(chan struct{}),
	}
	defer func() { close(doneCh); pw.Close() }()
	go sess.runCMSender(doneCh, pw)

	// Capture what sendCMCommand writes.
	written := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := pr.Read(buf)
		written <- string(buf[:n])
		pr.Close()
	}()

	// Simulate CM response: wait until sendCMCommand has enqueued itself, then reply.
	go func() {
		_ = wait.WaitForCondition(func() bool {
			sess.controlModeSubMu.Lock()
			defer sess.controlModeSubMu.Unlock()
			return len(sess.pendingCmds) > 0
		}, wait.WaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond, Description: "CM command enqueued"})
		sess.processControlModeLine("%begin 1 1 0")
		sess.processControlModeLine("220 50")
		sess.processControlModeLine("%end 1 1 0")
	}()

	w, h, err := sess.GetPaneDimensions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 220 || h != 50 {
		t.Fatalf("expected 220x50, got %dx%d", w, h)
	}
	if subprocessCalled {
		t.Fatal("subprocess was called when CM path was used")
	}

	select {
	case cmd := <-written:
		if !strings.Contains(cmd, "display-message") {
			t.Fatalf("expected display-message in CM stdin, got %q", cmd)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for CM stdin write")
	}
}

// fakeWriteCloser is a non-nil io.WriteCloser that discards all writes.
type fakeWriteCloser struct{}

func (fakeWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (fakeWriteCloser) Close() error                { return nil }

// TestCMDispatch_ExitDrainsPendingCmdsImmediately verifies that %exit drains
// in-flight pendingCmds with ErrControlModeStopped without waiting for scanner EOF.
// This is the fix for the race where capture-pane/resize commands block for 3s.
func TestCMDispatch_ExitDrainsPendingCmdsImmediately(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	channels := enqueueChannels(sess, 3)

	// Fire %exit — all pending commands must be drained immediately.
	sess.processControlModeLine("%exit")

	deadline := time.After(100 * time.Millisecond)
	for i, ch := range channels {
		select {
		case r := <-ch:
			if r.err != ErrControlModeStopped {
				t.Errorf("channel %d: expected ErrControlModeStopped, got %v", i, r.err)
			}
		case <-deadline:
			t.Fatalf("channel %d was not drained within 100ms of %%exit (would be 3s timeout without fix)", i)
		}
	}
}

// TestCMDispatch_ExitSetsControlModeExitedFlag verifies that controlModeExited is
// set synchronously when %exit is processed, so SubscribeToControlModeUpdates
// immediately returns a pre-closed channel for late subscribers.
func TestCMDispatch_ExitSetsControlModeExitedFlag(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	sess.processControlModeLine("%exit")

	sess.controlModeSubMu.Lock()
	exited := sess.controlModeExited
	sess.controlModeSubMu.Unlock()

	if !exited {
		t.Fatal("controlModeExited should be true immediately after exit is processed")
	}

	// SubscribeToControlModeUpdates must return a pre-closed channel.
	_, ch := sess.SubscribeToControlModeUpdates()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected pre-closed channel (ok=false), got open channel")
		}
	default:
		t.Fatal("channel was not closed — late subscriber would block forever")
	}
}

// TestCMDispatch_ProcessAfterExitReturnsStopped verifies that commands enqueued via
// runCMSender AFTER %exit is processed get ErrControlModeStopped immediately, not after
// a 3-second context timeout. This is the race: runCMSender can still be alive when
// %exit fires, and previously it would append to pendingCmds after the drain had run.
func TestCMDispatch_ProcessAfterExitReturnsStopped(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	// Simulate %exit having already been processed.
	sess.processControlModeLine("%exit")

	// Now enqueue a command AFTER exit — simulates the race where runCMSender
	// picks up a resize/capture-pane request after the CM process has died.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := sess.sendCMCommand(ctx, "capture-pane", "-p", "-e", "-t", "test")
	if err == nil {
		t.Fatal("expected error after exit, got nil")
	}
	// Must return quickly — not after the 3-second cmCtx timeout.
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("sendCMCommand timed out; command should have returned ErrControlModeStopped immediately (3s race)")
	}
	if err != ErrControlModeStopped && !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("expected ErrControlModeStopped, got %v", err)
	}
}

// TestCMDispatch_ExitDrainsInFlightCmdResp verifies that an in-flight command
// (between %begin and %exit) receives ErrControlModeStopped, not a silent hang.
func TestCMDispatch_ExitDrainsInFlightCmdResp(t *testing.T) {
	sess, _ := newDispatchTestSession(t)

	channels := enqueueChannels(sess, 1)
	ch := channels[0]

	// %begin claims the channel; %exit arrives before %end.
	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("partial body")
	sess.processControlModeLine("%exit")

	select {
	case r := <-ch:
		if r.err != ErrControlModeStopped {
			t.Fatalf("expected ErrControlModeStopped for in-flight cmd, got err=%v body=%q", r.err, r.body)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("in-flight channel was not drained after exit")
	}
}
