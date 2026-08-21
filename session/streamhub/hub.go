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

// WithQuiescenceTimeout overrides the hard deadline a resize's
// quiescence-wait gives up at (default: 500ms, matching the pre-hub
// per-connection behavior). Tests use this to shrink the deadline so
// timeout-path assertions run in milliseconds.
func WithQuiescenceTimeout(d time.Duration) HubOption {
	return func(h *StreamHub) { h.quiescenceTimeout = d }
}

// WithQuiescenceQuietPeriod overrides how long a resize's quiescence-wait
// must see no update before declaring the reflow settled (default: 200ms).
func WithQuiescenceQuietPeriod(d time.Duration) HubOption {
	return func(h *StreamHub) { h.quiescenceQuietPeriod = d }
}

// StreamHub is the single-owner runtime object for one tmux session's output
// stream: it fans output out to every attached subscriber over a
// Transport-agnostic interface, and is the sole caller of that session's
// resize/quiescence/capture-pane surface (Epic 1.3) via SessionController.
// Epic 1.2 implements the subscriber registry, fan-out, and
// lifecycle/teardown that this builds on.
type StreamHub struct {
	sessionName string
	controller  SessionController

	subscriberBufferSize  int
	slowSubscriberGrace   time.Duration
	teardownGrace         time.Duration
	quiescenceTimeout     time.Duration
	quiescenceQuietPeriod time.Duration

	mu            sync.Mutex
	state         HubLifecycleState
	subscribers   map[SubscriberID]*subscriber
	teardownTimer *time.Timer

	// resizeMu guards negotiatedSize/resizing — kept separate from mu (the
	// subscriber-registry lock) so a slow SessionController call (a real
	// tmux ioctl/capture-pane) never blocks AttachSubscriber/
	// DetachSubscriber/Broadcast on unrelated subscribers.
	resizeMu       sync.Mutex
	negotiatedSize TerminalSize
	resizing       bool
}

// NewStreamHub constructs a StreamHub for one tmux session, starting in
// HubStarting state with zero subscribers. controller is used by
// ForceTeardown and the resize/quiescence/capture pipeline; it may be nil in
// tests that never exercise either.
func NewStreamHub(sessionName string, controller SessionController, opts ...HubOption) *StreamHub {
	h := &StreamHub{
		sessionName:           sessionName,
		controller:            controller,
		subscriberBufferSize:  defaultSubscriberBufferSize,
		slowSubscriberGrace:   defaultSlowSubscriberGrace,
		teardownGrace:         DefaultHubTeardownGrace,
		quiescenceTimeout:     defaultQuiescenceTimeout,
		quiescenceQuietPeriod: defaultQuiescenceQuietPeriod,
		state:                 HubStarting,
		subscribers:           make(map[SubscriberID]*subscriber),
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

// streamEndedSentinel is broadcast to every subscriber when the hub's
// SessionController surfaces an error it cannot recover from inline (a
// SetWindowSize/ResizePTY/CapturePaneContent failure) — the same signal
// class as HubCrashed/TmuxProcessDied (research/architecture.md's
// EventStorming table), so a subscriber can tell "the stream ended for a
// real reason" apart from a mere disconnect.
var streamEndedSentinel = []byte("\x00STREAM_ENDED\x00")

// OnRawOutput is StreamHub's gate for raw tmux output once a caller (the
// production wiring that connects a StreamHub to its tmux session, outside
// Epic 1.3's scope) drives it with SessionController.SubscribeControlModeUpdates
// frames. It always counts the arrival toward quiescence via the resizing
// flag's readers, but suppresses broadcasting the frame itself while a
// resize is in progress (Task 1.3.2d) — mirroring
// server/services/connectrpc_websocket.go's existing resizeSettling-gated
// forwarding loop, which also drops rather than replays suppressed frames,
// relying on the post-quiescence capture-pane snapshot (applyNegotiatedSize)
// to bring every subscriber back in sync instead.
func (h *StreamHub) OnRawOutput(data []byte) {
	h.resizeMu.Lock()
	resizing := h.resizing
	h.resizeMu.Unlock()
	if resizing {
		return
	}
	h.Broadcast(data)
}

// applyNegotiatedSize is the hub's single call site for
// SessionController.SetWindowSize (Task 1.3.2c) — the only place any
// negotiated resize is actually applied to the underlying tmux session,
// regardless of how many subscriber votes triggered the change. It runs the
// full resize -> quiescence-wait -> capture -> broadcast pipeline exactly
// once per call: raw output arriving mid-pipeline is suppressed via the
// resizing flag (OnRawOutput, Task 1.3.2d), and once quiescence is reached
// the hub captures the pane exactly once and broadcasts the result to every
// attached subscriber (Task 1.3.2e), not just the one whose vote triggered
// the resize.
func (h *StreamHub) applyNegotiatedSize(size TerminalSize) {
	if h.controller == nil {
		return
	}

	h.resizeMu.Lock()
	h.resizing = true
	h.resizeMu.Unlock()
	defer func() {
		h.resizeMu.Lock()
		h.resizing = false
		h.resizeMu.Unlock()
	}()

	if err := h.controller.SetWindowSize(size.cols, size.rows); err != nil {
		log.Error("streamhub: SetWindowSize failed", "session", h.sessionName, "cols", size.cols, "rows", size.rows, "error", err)
		h.handleControllerError(err)
		return
	}

	subID, updates := h.controller.SubscribeControlModeUpdates()
	waitForQuiescence(updates, h.quiescenceTimeout, h.quiescenceQuietPeriod, h.sessionName)
	h.controller.UnsubscribeControlModeUpdates(subID)

	content, err := h.controller.CapturePaneContent()
	if err != nil {
		log.Error("streamhub: CapturePaneContent failed after resize", "session", h.sessionName, "error", err)
		h.handleControllerError(err)
		return
	}

	h.Broadcast([]byte(content))
}

// handleControllerError is reached when SetWindowSize or CapturePaneContent
// errors while the hub is otherwise alive (Task 1.3.2g): every attached
// subscriber receives the same stream-ended sentinel used for
// HubCrashed/TmuxProcessDied, and the hub tears itself down so no pending
// ResizeVote/NegotiatedSize is left referencing a controller that can no
// longer act on it. Epic 1.3 does not implement session *restart* — that
// machinery (recreating a SessionController, re-attaching subscribers to a
// fresh instance) belongs to Epic 1.4's failure-mode suite and HubRegistry
// work. Until it lands, "attempt a restart, or clean teardown if restart is
// not possible" (Story 1.3.2's AC) always resolves to the teardown branch,
// which is the correct behavior today, not a partial implementation left
// unfinished.
func (h *StreamHub) handleControllerError(err error) {
	log.Error("streamhub: SessionController call failed, ending stream",
		"session", h.sessionName, "error", err)
	h.Broadcast(streamEndedSentinel)
	_ = h.ForceTeardown()
}
