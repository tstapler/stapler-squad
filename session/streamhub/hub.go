package streamhub

import (
	"errors"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// errSlowSubscriberEvicted is the eviction reason logged when a subscriber's
// outbound queue stays full past slowSubscriberGrace (Task 1.2.1e), as
// opposed to a Transport.Send error (Task 1.2.1g).
var errSlowSubscriberEvicted = errors.New("streamhub: subscriber outbound queue full past grace period")

const (
	// defaultSubscriberBufferSize bounds each subscriber's outbound queue
	// depth, mirroring the buffered channel size
	// TmuxSession.SubscribeToControlModeUpdates uses today
	// (session/tmux/control_mode.go).
	defaultSubscriberBufferSize = 100

	// defaultSlowSubscriberGrace mirrors controlModeSlowSubscriberGrace
	// (session/tmux/control_mode.go:51): how long a subscriber's outbound
	// queue may stay observed-full before it is evicted as unable to keep
	// up, rather than being evicted on the very first full send (which
	// would disconnect healthy-but-bursty consumers, not just stuck ones).
	defaultSlowSubscriberGrace = 250 * time.Millisecond

	// DefaultHubTeardownGrace is how long a StreamHub with zero subscribers
	// waits before tearing down control mode, so a brief reconnect doesn't
	// kill it (Story 1.2.2).
	DefaultHubTeardownGrace = 5 * time.Second
)

// sessionStopper is a minimal placeholder scoped to exactly what Epic 1.2's
// teardown path needs from the underlying tmux session. The full
// SessionController interface (SetWindowSize/ResizePTY/CapturePaneContent/
// StopControlMode/Subscribe-UnsubscribeControlModeUpdates) is defined in
// Epic 1.3 (Story 1.3.2a, plan.md's SessionController glossary entry);
// StreamHub is expected to depend on that interface once it lands, in place
// of this placeholder. Kept unexported and single-method now so Epic 1.2
// doesn't invent a speculative, over-broad interface ahead of its consumer
// (interface-pollution-checklist.md smell #1).
type sessionStopper interface {
	StopControlMode() error
}

// HubOption configures a StreamHub at construction time. Tests use these to
// shrink buffer sizes and grace periods so lifecycle/eviction assertions run
// in milliseconds instead of seconds; production callers can rely on the
// zero-value defaults below.
type HubOption func(*StreamHub)

// WithSubscriberBufferSize overrides the outbound queue depth for every
// subscriber attached after this option is applied.
func WithSubscriberBufferSize(n int) HubOption {
	return func(h *StreamHub) { h.subscriberBufferSize = n }
}

// WithSlowSubscriberGrace overrides how long a full outbound queue is
// tolerated before the subscriber is evicted.
func WithSlowSubscriberGrace(d time.Duration) HubOption {
	return func(h *StreamHub) { h.slowSubscriberGrace = d }
}

// WithTeardownGrace overrides how long the hub waits after its last
// subscriber detaches before tearing down.
func WithTeardownGrace(d time.Duration) HubOption {
	return func(h *StreamHub) { h.teardownGrace = d }
}

// StreamHub is the single-owner runtime object for one tmux session's output
// stream: it fans output out to every attached subscriber over a
// Transport-agnostic interface. Epic 1.2 implements the subscriber registry,
// fan-out, and lifecycle/teardown; resize/quiescence/capture ownership
// (Epic 1.3) builds on top of this type.
type StreamHub struct {
	sessionName string
	controller  sessionStopper

	subscriberBufferSize int
	slowSubscriberGrace  time.Duration
	teardownGrace        time.Duration

	mu            sync.Mutex
	state         HubLifecycleState
	subscribers   map[SubscriberID]*subscriber
	teardownTimer *time.Timer
}

// NewStreamHub constructs a StreamHub for one tmux session, starting in
// HubStarting state with zero subscribers. controller is used only by
// ForceTeardown; it may be nil in tests that never exercise teardown.
func NewStreamHub(sessionName string, controller sessionStopper, opts ...HubOption) *StreamHub {
	h := &StreamHub{
		sessionName:          sessionName,
		controller:           controller,
		subscriberBufferSize: defaultSubscriberBufferSize,
		slowSubscriberGrace:  defaultSlowSubscriberGrace,
		teardownGrace:        DefaultHubTeardownGrace,
		state:                HubStarting,
		subscribers:          make(map[SubscriberID]*subscriber),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// State returns the hub's current HubLifecycleState.
func (h *StreamHub) State() HubLifecycleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// SubscriberCount returns the number of currently attached subscribers.
func (h *StreamHub) SubscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

// AttachSubscriber registers a new subscriber backed by transport, starts its
// writer goroutine, and returns its SubscriberID. Attaching during HubDraining
// cancels the pending teardown (Task 1.2.2a) and returns the hub to
// HubActive; attaching to a HubTornDown hub also reactivates it, since
// streamhub itself has no opinion on whether that's a legitimate reattach —
// that policy belongs to HubRegistry (Epic 3).
func (h *StreamHub) AttachSubscriber(transport Transport, capability SubscriberCapability) SubscriberID {
	id := NewSubscriberID()
	sub := newSubscriber(id, transport, capability, h.subscriberBufferSize)

	h.mu.Lock()
	h.subscribers[id] = sub
	h.cancelPendingTeardownLocked()
	switch h.state {
	case HubStarting, HubActive, HubDraining, HubTornDown:
		h.state = HubActive
	default:
		panic("unhandled HubLifecycleState")
	}
	count := len(h.subscribers)
	h.mu.Unlock()

	sub.startWriter(h.handleSubscriberSendError)

	log.Info("streamhub subscriber attached",
		"session", h.sessionName, "subscriber_id", string(id), "subscriber_count", count)
	return id
}

// DetachSubscriber removes the subscriber and stops its writer goroutine
// without leaking it. If this was the last subscriber, the hub schedules
// teardown after its grace period rather than tearing down immediately
// (Story 1.2.2). Detaching an unknown SubscriberID is a no-op.
func (h *StreamHub) DetachSubscriber(id SubscriberID) {
	h.removeSubscriber(id, nil)
}

// handleSubscriberSendError is the writer goroutine's callback for a failed
// Transport.Send (Task 1.2.1g). It evicts the subscriber via the exact same
// path as DetachSubscriber/slow-subscriber eviction, so the failure is
// reported and removed exactly once — never once per attempted frame, since
// the writer goroutine returns immediately after calling this.
func (h *StreamHub) handleSubscriberSendError(id SubscriberID, err error) {
	h.removeSubscriber(id, err)
}

// removeSubscriber is the single removal path shared by graceful detach,
// send-error eviction, and slow-subscriber eviction. reason is nil for a
// graceful detach and non-nil for an eviction, which only changes the log
// line — the registry/teardown-scheduling/close behavior is identical.
func (h *StreamHub) removeSubscriber(id SubscriberID, reason error) {
	h.mu.Lock()
	sub, ok := h.subscribers[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.subscribers, id)
	remaining := len(h.subscribers)
	if remaining == 0 && h.state != HubTornDown {
		h.scheduleTeardownLocked()
	}
	h.mu.Unlock()

	sub.close()

	if reason != nil {
		log.Warn("streamhub subscriber evicted",
			"session", h.sessionName, "subscriber_id", string(id), "subscriber_count", remaining, "error", reason)
		return
	}
	log.Info("streamhub subscriber detached",
		"session", h.sessionName, "subscriber_id", string(id), "subscriber_count", remaining)
}

// Broadcast fans data out to every attached subscriber via a non-blocking
// send. A subscriber whose outbound queue is observed full is never blocked
// on: the first observation arms a one-shot grace-period timer, independent
// of whether further frames are broadcast, and eviction (Task 1.2.1e) fires
// only if the queue is still full when that timer expires — a subscriber
// that merely had a momentary burst and drained in time is never evicted or
// re-armed for the same stall.
func (h *StreamHub) Broadcast(data []byte) {
	h.mu.Lock()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		subs = append(subs, sub)
	}
	grace := h.slowSubscriberGrace
	h.mu.Unlock()

	for _, sub := range subs {
		h.deliver(sub, data, grace)
	}
}

func (h *StreamHub) deliver(sub *subscriber, data []byte, grace time.Duration) {
	if sub.trySend(data) {
		return
	}

	if !sub.markSlow() {
		// Already stalled and a timer is pending for it; this frame is
		// simply dropped, same as any frame that finds the queue full.
		return
	}

	time.AfterFunc(grace, func() {
		if sub.isStillFull() {
			h.removeSubscriber(sub.id, errSlowSubscriberEvicted)
			return
		}
		sub.clearSlow()
	})
}

// cancelPendingTeardownLocked stops any in-flight teardown timer. Callers
// must hold h.mu.
func (h *StreamHub) cancelPendingTeardownLocked() {
	if h.teardownTimer != nil {
		h.teardownTimer.Stop()
		h.teardownTimer = nil
	}
}

// scheduleTeardownLocked transitions the hub to HubDraining and arms a timer
// that calls ForceTeardown after teardownGrace, unless a subscriber
// reattaches first and cancels it (Task 1.2.2a). Callers must hold h.mu.
func (h *StreamHub) scheduleTeardownLocked() {
	h.state = HubDraining
	h.cancelPendingTeardownLocked()
	h.teardownTimer = time.AfterFunc(h.teardownGrace, h.onTeardownGraceExpired)
}

// onTeardownGraceExpired fires on the timer goroutine once teardownGrace has
// elapsed with no reattach. It re-checks state under the lock before tearing
// down, so a reattach that raced the timer (already stopped, but whose
// firing was already in flight) can't cause a spurious teardown of an active
// hub (Task 1.2.2c: single teardown path, no duplicate logic).
func (h *StreamHub) onTeardownGraceExpired() {
	h.mu.Lock()
	stillDraining := h.state == HubDraining && len(h.subscribers) == 0
	h.mu.Unlock()
	if !stillDraining {
		return
	}
	_ = h.ForceTeardown()
}

// ForceTeardown tears the hub down unconditionally: every remaining
// subscriber is closed, the hub transitions to HubTornDown, and
// SessionController.StopControlMode() is invoked exactly once. It is the
// single teardown code path reached both by grace-period expiry
// (onTeardownGraceExpired) and by an external trigger such as the
// flag-flipped-back-to-legacy case Story 3.1.2 wires up. Calling it more than
// once (including concurrently) is safe — StopControlMode fires only for the
// caller that wins the HubTornDown transition.
func (h *StreamHub) ForceTeardown() error {
	h.mu.Lock()
	if h.state == HubTornDown {
		h.mu.Unlock()
		return nil
	}
	h.cancelPendingTeardownLocked()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		subs = append(subs, sub)
	}
	h.subscribers = make(map[SubscriberID]*subscriber)
	h.state = HubTornDown
	h.mu.Unlock()

	for _, sub := range subs {
		sub.close()
	}

	if h.controller == nil {
		log.Info("streamhub torn down", "session", h.sessionName)
		return nil
	}

	if err := h.controller.StopControlMode(); err != nil {
		log.Warn("streamhub force teardown: StopControlMode failed", "session", h.sessionName, "error", err)
		return err
	}
	log.Info("streamhub torn down", "session", h.sessionName)
	return nil
}
