package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

// fakeScrollForwardProcessManager is a minimal ProcessManager test double
// for exercising ForwardScroll's send/capture path without a real tmux
// backend. Embeds a nil ProcessManager, mirroring
// fakeSendKeysProcessManager's precedent (autonomous_driver_test.go), so any
// unexpected method call panics loudly.
type fakeScrollForwardProcessManager struct {
	ProcessManager

	mu sync.Mutex

	sent [][]byte

	// sendErrOnCall, if non-zero, forces SendInputViaControlMode's call at
	// this 1-based index to return sendErr.
	sendErrOnCall int
	sendErr       error

	// captureSeq scripts CapturePaneContent's successive return values,
	// repeating the last entry once exhausted (mirrors fakePanePreviewer's
	// pop-with-repeat pattern).
	captureSeq []string
	captureIdx int
	captureErr error

	scrollbackContent string
	scrollbackErr     error
	scrollbackCalls   []string // "startLine,endLine" pairs
}

func (f *fakeScrollForwardProcessManager) HasSession() bool { return true }

func (f *fakeScrollForwardProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	callNum := len(f.sent) + 1
	f.sent = append(f.sent, append([]byte(nil), data...))
	if f.sendErrOnCall != 0 && callNum == f.sendErrOnCall {
		return f.sendErr
	}
	return nil
}

func (f *fakeScrollForwardProcessManager) sentSequences() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeScrollForwardProcessManager) CapturePaneContent() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.captureErr != nil {
		return "", f.captureErr
	}
	if len(f.captureSeq) == 0 {
		return "", nil
	}
	idx := f.captureIdx
	if idx >= len(f.captureSeq) {
		idx = len(f.captureSeq) - 1
	}
	f.captureIdx++
	return f.captureSeq[idx], nil
}

func (f *fakeScrollForwardProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scrollbackCalls = append(f.scrollbackCalls, startLine+","+endLine)
	if f.scrollbackErr != nil {
		return "", f.scrollbackErr
	}
	return f.scrollbackContent, nil
}

// newForwardScrollTestInstance builds a passing-gate *Instance
// (program=claude, alt-screen active, DetectedStatus idle) wired to a fake
// ProcessManager, mirroring newScrollGateTestInstance (scroll_gate_test.go)
// plus the started/processManager wiring fakeSendKeysProcessManager's
// callers use (autonomous_driver_test.go).
func newForwardScrollTestInstance(t *testing.T, fakePM *fakeScrollForwardProcessManager) *Instance {
	t.Helper()
	inst := &Instance{Title: t.Name(), Program: "claude", AltScreenActive: true}
	inst.processManager = fakePM
	inst.started.Store(true)

	src := NewPiStatusSource(inst.Title, nil)
	src.status.Store(int32(detection.StatusIdle))
	mgr := NewInstanceStatusManager()
	mgr.RegisterPiStatusSource(inst.Title, src)
	inst.SetStatusManager(mgr)

	return inst
}

func TestForwardScroll_should_SendKeySequencesInOrderAndReturnDelivered_When_GateAndLeasePass(t *testing.T) {
	fakePM := &fakeScrollForwardProcessManager{captureSeq: []string{"settled", "settled"}}
	inst := newForwardScrollTestInstance(t, fakePM)

	outcome, blockedReason, content, err := inst.ForwardScroll(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("ForwardScroll() returned unexpected error: %v", err)
	}
	if outcome != sessionv1.ScrollForwardOutcome_DELIVERED {
		t.Fatalf("outcome = %v, want DELIVERED", outcome)
	}
	if blockedReason != sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED {
		t.Fatalf("blockedReason = %v, want unspecified", blockedReason)
	}
	if string(content) != "settled" {
		t.Fatalf("content = %q, want %q", content, "settled")
	}

	sent := fakePM.sentSequences()
	if len(sent) != 1 {
		t.Fatalf("expected exactly one SendInputViaControlMode call (ClaudeScrollAdapter's single-element KeySequences), got %d", len(sent))
	}
	if string(sent[0]) != string(pageUpBytes) {
		t.Fatalf("sent[0] = %x, want PageUp bytes %x", sent[0], pageUpBytes)
	}
}

func TestForwardScroll_should_ReturnBlockedWithoutPTYWrite_When_AppScrollGateFailsMultipleViewers(t *testing.T) {
	fakePM := &fakeScrollForwardProcessManager{}
	inst := newForwardScrollTestInstance(t, fakePM)

	outcome, blockedReason, content, err := inst.ForwardScroll(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("ForwardScroll() returned unexpected error: %v", err)
	}
	if outcome != sessionv1.ScrollForwardOutcome_BLOCKED {
		t.Fatalf("outcome = %v, want BLOCKED", outcome)
	}
	if blockedReason != sessionv1.ScrollBlockedReason_MULTIPLE_VIEWERS {
		t.Fatalf("blockedReason = %v, want MULTIPLE_VIEWERS", blockedReason)
	}
	if content != nil {
		t.Fatalf("content = %v, want nil", content)
	}
	if len(fakePM.sentSequences()) != 0 {
		t.Fatalf("SendInputViaControlMode was called despite AppScrollGate failing")
	}
}

// TestForwardScroll_should_ReturnDistinctBlockedReason_When_SubscriberCountIsNegativeVsGreaterThanOne
// closes the adversarial-review BLOCKER: the -1 PathLegacyPerConnection
// sentinel and a genuine 2+-subscriber block must never collapse into the
// same client-visible ScrollBlockedReason (plan.md's Risk Control, "Concrete
// resolution"). AppScrollGate (scroll_gate.go, Epic 1.1) emits the identical
// "multiple viewers connected" reason string for both subscriberCount<0 and
// subscriberCount>1 -- ForwardScroll's mapGateReason disambiguates using the
// subscriberCount it already has, rather than widening that shared string.
func TestForwardScroll_should_ReturnDistinctBlockedReason_When_SubscriberCountIsNegativeVsGreaterThanOne(t *testing.T) {
	legacySentinelInst := newForwardScrollTestInstance(t, &fakeScrollForwardProcessManager{})
	_, legacyReason, _, err := legacySentinelInst.ForwardScroll(context.Background(), -1, nil)
	if err != nil {
		t.Fatalf("ForwardScroll(-1) returned unexpected error: %v", err)
	}
	if legacyReason != sessionv1.ScrollBlockedReason_UNSUPPORTED_STREAMING_PATH {
		t.Fatalf("subscriberCount=-1: blockedReason = %v, want UNSUPPORTED_STREAMING_PATH", legacyReason)
	}

	multiViewerInst := newForwardScrollTestInstance(t, &fakeScrollForwardProcessManager{})
	_, multiReason, _, err := multiViewerInst.ForwardScroll(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("ForwardScroll(2) returned unexpected error: %v", err)
	}
	if multiReason != sessionv1.ScrollBlockedReason_MULTIPLE_VIEWERS {
		t.Fatalf("subscriberCount=2: blockedReason = %v, want MULTIPLE_VIEWERS", multiReason)
	}

	if legacyReason == multiReason {
		t.Fatalf("subscriberCount=-1 and subscriberCount=2 must map to different ScrollBlockedReason values, both got %v", legacyReason)
	}
}

func TestForwardScroll_should_ReturnBlockedLeaseContention_When_CalledConcurrentlyForSameInstance(t *testing.T) {
	// A blocking capture holds the lease open long enough for a second,
	// concurrent ForwardScroll call to observe contention deterministically.
	captureStarted := make(chan struct{})
	releaseCapture := make(chan struct{})
	inst := newForwardScrollTestInstance(t, &fakeScrollForwardProcessManager{})
	inst.processManager = &blockingCaptureProcessManager{
		fakeScrollForwardProcessManager: inst.processManager.(*fakeScrollForwardProcessManager),
		started:                         captureStarted,
		release:                         releaseCapture,
	}

	first := startBlockedFirstForwardScroll(t, inst, captureStarted)

	_, blockedReason, content, err := inst.ForwardScroll(context.Background(), 1, nil)
	close(releaseCapture)
	first.wg.Wait()

	if err != nil {
		t.Fatalf("second ForwardScroll() returned unexpected error: %v", err)
	}
	if blockedReason != sessionv1.ScrollBlockedReason_LEASE_CONTENTION {
		t.Fatalf("second call blockedReason = %v, want LEASE_CONTENTION", blockedReason)
	}
	if content != nil {
		t.Fatalf("second call content = %v, want nil", content)
	}
	if first.err != nil {
		t.Fatalf("first ForwardScroll() returned unexpected error: %v", first.err)
	}
	if first.outcome != sessionv1.ScrollForwardOutcome_AT_TOP && first.outcome != sessionv1.ScrollForwardOutcome_DELIVERED {
		t.Fatalf("first call outcome = %v, want the one call that actually proceeded to complete", first.outcome)
	}
}

// forwardScrollCallResult carries one background ForwardScroll call's
// outcome/error back to the test goroutine.
type forwardScrollCallResult struct {
	wg      *sync.WaitGroup
	outcome sessionv1.ScrollForwardOutcome
	err     error
}

// startBlockedFirstForwardScroll starts inst.ForwardScroll in a goroutine and
// waits for it to reach its blocking capture point (captureStarted), so the
// caller can then fire a genuinely concurrent second call instead of relying
// on a sleep-based race.
func startBlockedFirstForwardScroll(t *testing.T, inst *Instance, captureStarted <-chan struct{}) *forwardScrollCallResult {
	t.Helper()
	result := &forwardScrollCallResult{wg: &sync.WaitGroup{}}
	result.wg.Add(1)
	go func() {
		defer result.wg.Done()
		result.outcome, _, _, result.err = inst.ForwardScroll(context.Background(), 1, nil)
	}()

	select {
	case <-captureStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("first ForwardScroll call never reached its capture wait")
	}
	return result
}

// blockingCaptureProcessManager wraps fakeScrollForwardProcessManager,
// making its first CapturePaneContent call block until release closes --
// giving a concurrent ForwardScroll call a deterministic window to observe
// lease contention instead of relying on a sleep-based race.
type blockingCaptureProcessManager struct {
	*fakeScrollForwardProcessManager
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingCaptureProcessManager) CapturePaneContent() (string, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return "settled", nil
}

func TestForwardScroll_should_ReleaseLeaseAndAttachBarrier_When_SendInputViaControlModeErrorsMidForward(t *testing.T) {
	sendErr := errors.New("fake: SendInputViaControlMode failed")
	fakePM := &fakeScrollForwardProcessManager{sendErrOnCall: 1, sendErr: sendErr}
	inst := newForwardScrollTestInstance(t, fakePM)

	// nil controller: currentSnapshot degrades gracefully (see its doc
	// comment -- "h.controller is nil, as in most unit tests"), and neither
	// error path below ever resizes or captures through the hub itself.
	hub := streamhub.NewStreamHub(t.Name(), nil, streamhub.WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()

	outcome, blockedReason, content, err := inst.ForwardScroll(context.Background(), 1, hub)
	if err == nil {
		t.Fatalf("ForwardScroll() returned nil error, want the forced send error")
	}
	if !errors.Is(err, sendErr) {
		t.Fatalf("ForwardScroll() error = %v, want it to wrap %v", err, sendErr)
	}
	if outcome != 0 || blockedReason != 0 || content != nil {
		t.Fatalf("ForwardScroll() on error path = (%v, %v, %v), want all zero values", outcome, blockedReason, content)
	}

	assertLeaseAndBarrierReleased(t, inst, hub)
}

func TestForwardScroll_should_ReleaseLeaseAndAttachBarrier_When_WaitForRedrawQuiescencePollErrorsMidForward(t *testing.T) {
	captureErr := errors.New("fake: pane content poll failed")
	fakePM := &fakeScrollForwardProcessManager{captureErr: captureErr}
	inst := newForwardScrollTestInstance(t, fakePM)

	// nil controller: currentSnapshot degrades gracefully (see its doc
	// comment -- "h.controller is nil, as in most unit tests"), and neither
	// error path below ever resizes or captures through the hub itself.
	hub := streamhub.NewStreamHub(t.Name(), nil, streamhub.WithTeardownGrace(time.Hour))
	defer hub.ForceTeardown()

	outcome, blockedReason, content, err := inst.ForwardScroll(context.Background(), 1, hub)
	if err == nil {
		t.Fatalf("ForwardScroll() returned nil error, want the forced capture error")
	}
	if !errors.Is(err, captureErr) {
		t.Fatalf("ForwardScroll() error = %v, want it to wrap %v", err, captureErr)
	}
	if outcome != 0 || blockedReason != 0 || content != nil {
		t.Fatalf("ForwardScroll() on error path = (%v, %v, %v), want all zero values", outcome, blockedReason, content)
	}

	assertLeaseAndBarrierReleased(t, inst, hub)
}

// assertLeaseAndBarrierReleased is the pre-mortem P1 #3 regression guard
// shared by both mid-forward-error tests above: a subsequent ForwardScroll
// call must not see LeaseContention, and a subsequent AttachSubscriber call
// on the same hub must not block -- forcing the actual defer chain to run
// under an error, not just reading it as correct.
func assertLeaseAndBarrierReleased(t *testing.T, inst *Instance, hub *streamhub.StreamHub) {
	t.Helper()

	if !inst.scrollLease.tryAcquire() {
		t.Fatalf("scrollLease still held after ForwardScroll returned an error")
	}
	inst.scrollLease.release()

	attachReturned := make(chan streamhub.SubscriberID, 1)
	go func() {
		id := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{})
		attachReturned <- id
	}()
	select {
	case <-attachReturned:
	case <-time.After(time.Second):
		t.Fatalf("hub.AttachSubscriber blocked after ForwardScroll returned an error -- ScrollForwardAttachBarrier was not released")
	}
}

func TestWaitForRedrawQuiescence_should_ReturnSettledContent_When_TwoConsecutivePollsMatch(t *testing.T) {
	seq := []string{"partial-a", "partial-b", "settled", "settled"}
	idx := 0
	paneContent := func() (string, error) {
		v := seq[idx]
		if idx < len(seq)-1 {
			idx++
		}
		return v, nil
	}

	start := time.Now()
	content, deadlineExceeded, err := waitForRedrawQuiescence(context.Background(), paneContent, 10*time.Millisecond, 2*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("waitForRedrawQuiescence() returned unexpected error: %v", err)
	}
	if deadlineExceeded {
		t.Fatalf("deadlineExceeded = true, want false")
	}
	if content != "settled" {
		t.Fatalf("content = %q, want %q", content, "settled")
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("waitForRedrawQuiescence waited out the full deadline (%v) instead of returning once polls matched", elapsed)
	}
}

func TestWaitForRedrawQuiescence_should_ReturnErrorImmediately_When_PaneContentPollFails(t *testing.T) {
	pollErr := errors.New("fake: pane content poll failed")
	calls := 0
	paneContent := func() (string, error) {
		calls++
		return "", pollErr
	}

	_, deadlineExceeded, err := waitForRedrawQuiescence(context.Background(), paneContent, 10*time.Millisecond, time.Second)
	if err == nil {
		t.Fatalf("waitForRedrawQuiescence() returned nil error, want the forced poll error")
	}
	if !errors.Is(err, pollErr) {
		t.Fatalf("waitForRedrawQuiescence() error = %v, want it to wrap %v", err, pollErr)
	}
	if deadlineExceeded {
		t.Fatalf("deadlineExceeded = true, want false (a poll error is not a deadline-exceeded case)")
	}
	if calls != 1 {
		t.Fatalf("paneContent was called %d times, want exactly 1 (no retry on error)", calls)
	}
}

func TestWaitForRedrawQuiescence_should_ReturnLastSeenContentWithDeadlineExceeded_When_ContentNeverStabilizes(t *testing.T) {
	call := 0
	paneContent := func() (string, error) {
		call++
		return "content-" + time.Now().String() + "-" + string(rune('a'+call%26)), nil
	}

	content, deadlineExceeded, err := waitForRedrawQuiescence(context.Background(), paneContent, 20*time.Millisecond, 150*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForRedrawQuiescence() returned unexpected error: %v", err)
	}
	if !deadlineExceeded {
		t.Fatalf("deadlineExceeded = false, want true")
	}
	if content == "" {
		t.Fatalf("content = %q, want the last-seen (non-empty) content, not an empty capture", content)
	}
}

func TestForwardScroll_should_CaptureViaNativeScrollbackDelegate_When_AdapterDeclaresThatMode(t *testing.T) {
	fakePM := &fakeScrollForwardProcessManager{scrollbackContent: "native scrollback dump"}
	inst := newForwardScrollTestInstance(t, fakePM)

	outcome, blockedReason, content, err := inst.captureViaNativeScrollbackDelegate()
	if err != nil {
		t.Fatalf("captureViaNativeScrollbackDelegate() returned unexpected error: %v", err)
	}
	if outcome != sessionv1.ScrollForwardOutcome_DELIVERED {
		t.Fatalf("outcome = %v, want DELIVERED", outcome)
	}
	if blockedReason != sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED {
		t.Fatalf("blockedReason = %v, want unspecified", blockedReason)
	}
	if string(content) != "native scrollback dump" {
		t.Fatalf("content = %q, want %q", content, "native scrollback dump")
	}
	if len(fakePM.scrollbackCalls) != 1 {
		t.Fatalf("expected exactly one CapturePaneContentWithOptions call, got %d", len(fakePM.scrollbackCalls))
	}
}

// fakeResizeCapableController is a minimal streamhub.SessionController
// double used only to drive StreamHub.RequestResize deterministically and
// quickly in TestForwardScroll_should_DiscardOutcomeWithError_When_ResizeRacesCaptureWindow
// (Fix 5) -- SubscribeControlModeUpdates never sends, so RequestResize's
// quiescence wait always resolves via the hub's short, test-configured
// quiescenceTimeout rather than a real update.
type fakeResizeCapableController struct{}

func (fakeResizeCapableController) SetWindowSizeContext(context.Context, int, int) error {
	return nil
}

func (fakeResizeCapableController) ResizePTY(int, int) error { return nil }

func (fakeResizeCapableController) CapturePaneContentRawContext(context.Context) (streamhub.RawPaneContent, error) {
	return streamhub.RawPaneContent("resized pane"), nil
}

func (fakeResizeCapableController) GetPaneCursorPosition() (int, int, error) { return 0, 0, nil }

func (fakeResizeCapableController) StartControlMode() error { return nil }

func (fakeResizeCapableController) StopControlMode() error { return nil }

func (fakeResizeCapableController) SubscribeControlModeUpdates() (string, <-chan []byte) {
	return "fake-sub", make(chan []byte)
}

func (fakeResizeCapableController) UnsubscribeControlModeUpdates(string) {}

// TestForwardScroll_should_DiscardOutcomeWithError_When_ResizeRacesCaptureWindow
// is the Fix 5 regression test: neither StreamHub.RequestResize nor
// applyNegotiatedSize acquires ScrollForwardAttachBarrier's scrollForwardMu
// (unlike AttachSubscriber, Fix 4), so a resize can run fully concurrently
// with an in-flight ForwardScroll's own independent pane-capture window.
// Forces a resize to complete while ForwardScroll's waitForRedrawQuiescence
// is still blocked mid-capture, then asserts ForwardScroll surfaces this as
// an error (which the caller, scrollbackResultForRequest, already treats the
// same as any other gate miss -- falling back to tmux-native scrollback --
// see errScrollForwardResizeRaced's doc comment) instead of silently
// trusting a possibly pane-reflowed DELIVERED/AT_TOP diff.
func TestForwardScroll_should_DiscardOutcomeWithError_When_ResizeRacesCaptureWindow(t *testing.T) {
	captureStarted := make(chan struct{})
	releaseCapture := make(chan struct{})
	fakePM := &fakeScrollForwardProcessManager{}
	inst := newForwardScrollTestInstance(t, fakePM)
	inst.processManager = &blockingCaptureProcessManager{
		fakeScrollForwardProcessManager: fakePM,
		started:                         captureStarted,
		release:                         releaseCapture,
	}

	hub := streamhub.NewStreamHub(t.Name(), fakeResizeCapableController{},
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(20*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)
	defer hub.ForceTeardown()

	resizeSubID := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})

	forwardDone := make(chan struct{})
	var outcome sessionv1.ScrollForwardOutcome
	var blockedReason sessionv1.ScrollBlockedReason
	var content []byte
	var err error
	go func() {
		defer close(forwardDone)
		outcome, blockedReason, content, err = inst.ForwardScroll(context.Background(), 1, hub)
	}()

	select {
	case <-captureStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("ForwardScroll never reached its capture wait")
	}

	// Drive a real resize to completion (incrementing StreamHub's
	// resizeGeneration counter) while ForwardScroll's own capture is still
	// blocked -- RequestResize is not gated by scrollForwardMu, so this must
	// succeed rather than deadlock, which is exactly the gap Fix 5 closes.
	size, sizeErr := streamhub.NewTerminalSize(100, 30)
	if sizeErr != nil {
		t.Fatalf("NewTerminalSize() returned unexpected error: %v", sizeErr)
	}
	hub.RequestResize(context.Background(), resizeSubID, size)

	close(releaseCapture)

	select {
	case <-forwardDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("ForwardScroll never returned after its capture was released")
	}

	if err == nil {
		t.Fatalf("ForwardScroll() returned nil error, want an error discarding the resize-raced outcome")
	}
	if !errors.Is(err, errScrollForwardResizeRaced) {
		t.Fatalf("ForwardScroll() error = %v, want it to wrap errScrollForwardResizeRaced", err)
	}
	if outcome != 0 || blockedReason != 0 || content != nil {
		t.Fatalf("ForwardScroll() on resize-raced path = (%v, %v, %v), want all zero values", outcome, blockedReason, content)
	}

	assertLeaseAndBarrierReleased(t, inst, hub)
}
