package streamhub_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// fakeSessionController is a minimal SessionController-shaped test double
// scoped to what Story 1.2.2's teardown path needs (StopControlMode). The
// full SessionController interface belongs to Epic 1.3 (Story 1.3.2a); this
// double satisfies whatever narrower shape StreamHub currently depends on
// structurally, with no import of package session.
type fakeSessionController struct {
	stopCalls atomic.Int32
	stopErr   error
}

func (f *fakeSessionController) StopControlMode() error {
	f.stopCalls.Add(1)
	return f.stopErr
}

func TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := &fakeSessionController{}
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

	if !waitFor(t, time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
		t.Fatalf("expected HubTornDown after grace period elapses, got %v", hub.State())
	}
	if calls := controller.stopCalls.Load(); calls != 1 {
		t.Fatalf("expected StopControlMode called exactly once, got %d calls", calls)
	}
}

func TestStreamHub_should_CancelPendingTeardown_When_SubscriberReattachesDuringGracePeriod(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := &fakeSessionController{}
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

	controller := &fakeSessionController{}
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
	controller := &fakeSessionController{stopErr: wantErr}
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
