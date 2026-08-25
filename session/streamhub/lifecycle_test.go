package streamhub_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// fakeSessionController is a call-counting SessionController test double,
// with no import of package session. Story 1.2.2's tests only exercise
// StopControlMode; Story 1.3.2's tests (hub_test.go) exercise the full
// resize/quiescence/capture surface via the same type.
type fakeSessionController struct {
	stopCalls atomic.Int32
	stopErr   error
	// stopFn, when set, replaces StopControlMode's body entirely (stopCalls
	// is still incremented) — lets a test block ForceTeardown mid-call to
	// assert on state visible only during that window.
	stopFn func() error

	setWindowSizeCalls atomic.Int32
	setWindowSizeErr   error

	resizePTYCalls atomic.Int32

	captureCalls   atomic.Int32
	captureContent string
	captureErr     error

	mu         sync.Mutex
	lastResize [2]int // cols, rows of the most recent SetWindowSize call
	resizeLog  [][2]int
	updates    chan []byte
	subscribed bool
}

func newFakeSessionController() *fakeSessionController {
	return &fakeSessionController{updates: make(chan []byte, 16)}
}

func (f *fakeSessionController) StopControlMode() error {
	f.stopCalls.Add(1)
	if f.stopFn != nil {
		return f.stopFn()
	}
	return f.stopErr
}

func (f *fakeSessionController) SetWindowSize(cols, rows int) error {
	f.setWindowSizeCalls.Add(1)
	f.mu.Lock()
	f.lastResize = [2]int{cols, rows}
	f.resizeLog = append(f.resizeLog, [2]int{cols, rows})
	f.mu.Unlock()
	return f.setWindowSizeErr
}

func (f *fakeSessionController) ResizePTY(_, _ int) error {
	f.resizePTYCalls.Add(1)
	return nil
}

func (f *fakeSessionController) CapturePaneContent() (string, error) {
	f.captureCalls.Add(1)
	return f.captureContent, f.captureErr
}

// SubscribeControlModeUpdates returns the fake's single shared updates
// channel. Only one live subscription is modeled — sufficient for
// StreamHub's own usage (subscribe/unsubscribe scoped to a single
// in-flight resize at a time).
func (f *fakeSessionController) SubscribeControlModeUpdates() (string, <-chan []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribed = true
	return "fake-subscriber", f.updates
}

// UnsubscribeControlModeUpdates closes the updates channel so a range loop
// reading from it (or a blocked waitForQuiescence select) can exit without
// leaking, mirroring the real *session.Instance contract.
func (f *fakeSessionController) UnsubscribeControlModeUpdates(_ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribed {
		f.subscribed = false
		close(f.updates)
		f.updates = make(chan []byte, 16)
	}
}

// resizeCallCount reports how many times SetWindowSize was called with
// exactly (cols, rows).
func (f *fakeSessionController) resizeCallCount(cols, rows int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, r := range f.resizeLog {
		if r == [2]int{cols, rows} {
			count++
		}
	}
	return count
}

func TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(30*time.Millisecond))

	id := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})
	if got := hub.State(); got != streamhub.HubActive {
		t.Fatalf("expected HubActive after attach, got %v", got)
	}

	hub.DetachSubscriber(id)

	if got := hub.State(); got != streamhub.HubDraining {
		t.Fatalf("expected HubDraining immediately after last detach, got %v", got)
	}
	if calls := controller.stopCalls.Load(); calls != 0 {
		t.Fatalf("expected StopControlMode not yet called, got %d calls", calls)
	}

	// 5s, not 1s: under a full `-race` suite run, CPU contention from other
	// packages' tests can delay this hub's 30ms teardown timer well past 1s
	// (observed flake in CI: "expected StopControlMode called exactly once, got 0 calls").
	if !waitFor(t, 5*time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
		t.Fatalf("expected HubTornDown after grace period elapses, got %v", hub.State())
	}
	if calls := controller.stopCalls.Load(); calls != 1 {
		t.Fatalf("expected StopControlMode called exactly once, got %d calls", calls)
	}
}

func TestStreamHub_should_CancelPendingTeardown_When_SubscriberReattachesDuringGracePeriod(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(150*time.Millisecond))

	firstID := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})
	hub.DetachSubscriber(firstID)

	if got := hub.State(); got != streamhub.HubDraining {
		t.Fatalf("expected HubDraining after last detach, got %v", got)
	}

	// Reattach well within the grace period.
	time.Sleep(20 * time.Millisecond)
	secondTransport := newMemoryTransport()
	secondID := hub.AttachSubscriber(secondTransport, streamhub.SubscriberCapability{})

	if got := hub.State(); got != streamhub.HubActive {
		t.Fatalf("expected HubActive immediately after reattach, got %v", got)
	}

	// Wait past the original grace deadline; the pending teardown must never fire.
	time.Sleep(200 * time.Millisecond)

	if got := hub.State(); got != streamhub.HubActive {
		t.Fatalf("expected HubActive to persist past the original grace deadline, got %v", got)
	}
	if calls := controller.stopCalls.Load(); calls != 0 {
		t.Fatalf("expected StopControlMode to never fire, got %d calls", calls)
	}

	hub.DetachSubscriber(secondID)
}

func TestStreamHub_should_CallStopControlModeExactlyOnce_When_ForceTeardownIsCalled(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(time.Hour))

	hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})
	hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})

	if err := hub.ForceTeardown(); err != nil {
		t.Fatalf("expected ForceTeardown to succeed, got error: %v", err)
	}

	if got := hub.State(); got != streamhub.HubTornDown {
		t.Fatalf("expected HubTornDown after ForceTeardown, got %v", got)
	}
	if calls := controller.stopCalls.Load(); calls != 1 {
		t.Fatalf("expected StopControlMode called exactly once, got %d calls", calls)
	}

	// A second call (e.g. both grace-period expiry and an external flag-flip
	// trigger racing each other, per Story 3.1.2) must not call it again.
	if err := hub.ForceTeardown(); err != nil {
		t.Fatalf("expected second ForceTeardown call to be a safe no-op, got error: %v", err)
	}
	if calls := controller.stopCalls.Load(); calls != 1 {
		t.Fatalf("expected StopControlMode still called exactly once after a second ForceTeardown, got %d calls", calls)
	}
}

func TestStreamHub_should_ReturnStopControlModeError_When_ForceTeardownControllerFails(t *testing.T) {
	defer goleak.VerifyNone(t)

	wantErr := errors.New("stop failed")
	controller := newFakeSessionController()
	controller.stopErr = wantErr
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(time.Hour))

	hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})

	err := hub.ForceTeardown()
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected ForceTeardown to surface the controller's error, got %v", err)
	}
	if got := hub.State(); got != streamhub.HubTornDown {
		t.Fatalf("expected HubTornDown even when StopControlMode errors, got %v", got)
	}
}

func TestStreamHub_should_NotReportHubTornDown_While_StopControlModeInFlight(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	inStopControlMode := make(chan struct{})
	release := make(chan struct{})
	controller.stopFn = func() error {
		close(inStopControlMode)
		<-release
		return nil
	}
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(time.Hour))
	hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})

	done := make(chan error, 1)
	go func() { done <- hub.ForceTeardown() }()

	<-inStopControlMode
	if got := hub.State(); got == streamhub.HubTornDown {
		t.Fatalf("State() reported HubTornDown while StopControlMode was still running")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("ForceTeardown: %v", err)
	}
	if got := hub.State(); got != streamhub.HubTornDown {
		t.Fatalf("expected HubTornDown after StopControlMode returns, got %v", got)
	}
}

func TestStreamHub_should_CallStopControlModeExactlyOnce_When_ForceTeardownRacesConcurrently(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	release := make(chan struct{})
	controller.stopFn = func() error {
		<-release
		return nil
	}
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(time.Hour))
	hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = hub.ForceTeardown()
		}(i)
	}
	// Give both goroutines a chance to reach ForceTeardown's guard before
	// releasing StopControlMode — without this, the second call could run
	// entirely before the first even starts, which wouldn't exercise the
	// teardownInFlight branch at all.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("ForceTeardown call %d: %v", i, err)
		}
	}
	if calls := controller.stopCalls.Load(); calls != 1 {
		t.Fatalf("expected StopControlMode called exactly once under concurrent ForceTeardown, got %d calls", calls)
	}
	if got := hub.State(); got != streamhub.HubTornDown {
		t.Fatalf("expected HubTornDown after both ForceTeardown calls settle, got %v", got)
	}
}

func TestStreamHub_should_NotClobberReArmedHubDraining_When_SubscriberReattachesAndDetachesDuringTeardown(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := newFakeSessionController()
	inStopControlMode := make(chan struct{})
	release := make(chan struct{})
	controller.stopFn = func() error {
		close(inStopControlMode)
		<-release
		return nil
	}
	hub := streamhub.NewStreamHub("test-session", controller, streamhub.WithTeardownGrace(time.Hour))
	hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})

	done := make(chan error, 1)
	go func() { done <- hub.ForceTeardown() }()
	<-inStopControlMode

	// Reattach then detach again while the original ForceTeardown's
	// StopControlMode call is still in flight — len(subscribers) is back to
	// 0 by the time StopControlMode returns, but a real subscriber lifecycle
	// happened in between and removeSubscriber re-armed a fresh HubDraining
	// grace period for it (scheduleTeardownLocked). ForceTeardown's stale
	// call must not clobber that back to HubTornDown.
	id := hub.AttachSubscriber(newMemoryTransport(), streamhub.SubscriberCapability{})
	hub.DetachSubscriber(id)

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("ForceTeardown: %v", err)
	}
	if got := hub.State(); got != streamhub.HubDraining {
		t.Fatalf("expected the reattach+redetach's own HubDraining to survive the original ForceTeardown call, got %v", got)
	}
}
