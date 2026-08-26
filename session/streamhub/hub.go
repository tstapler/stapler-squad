package streamhub

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

// WithBatchMaxWindow overrides the hub's BatchWindow ceiling (default:
// MaxBatchWindow, 20ms) — tests use this to shrink it so ceiling-flush
// assertions run in milliseconds.
func WithBatchMaxWindow(d time.Duration) HubOption {
	return func(h *StreamHub) { h.batchMaxWindow = d }
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
	batchMaxWindow        time.Duration

	mu sync.Mutex
	// state only transitions to HubTornDown once ForceTeardown's close/
	// StopControlMode work has actually finished (see its doc comment) —
	// teardownInFlight is a separate exactly-once claim so a second,
	// concurrent ForceTeardown call is rejected immediately without
	// waiting for that work, instead of racing to call StopControlMode
	// twice.
	state            HubLifecycleState
	teardownInFlight bool
	// reactivatedDuringTeardown is set by AttachSubscriber when it observes
	// teardownInFlight, so ForceTeardown's final write can tell "genuinely
	// idle throughout" apart from "attached and detached again mid-
	// teardown" — len(subscribers)==0 alone can't distinguish those two
	// cases, and the latter can otherwise silently clobber a legitimate
	// HubDraining state (and its freshly-armed teardownTimer) that
	// removeSubscriber re-armed in response to the second detach.
	reactivatedDuringTeardown bool
	subscribers               map[SubscriberID]*subscriber
	teardownTimer             *time.Timer

	// batchWindow is the hub's single opportunistic-with-ceiling
	// accumulation buffer and timer (Epic 2.1) — OnRawOutput feeds it
	// instead of broadcasting directly, so N attached subscribers share one
	// coalesce/flush pass per burst instead of each paying their own.
	batchWindow *BatchWindow

	// resizeMu guards negotiatedSize/resizing — kept separate from mu (the
	// subscriber-registry lock) so a slow SessionController call (a real
	// tmux ioctl/capture-pane) never blocks AttachSubscriber/
	// DetachSubscriber/Broadcast on unrelated subscribers.
	resizeMu       sync.Mutex
	negotiatedSize TerminalSize
	resizing       bool

	// resizeApplyMu serializes RequestResize's full negotiate-then-apply
	// sequence end to end, across every subscriber attached to this hub.
	// It is a distinct lock from resizeMu (which only guards brief
	// reads/writes of negotiatedSize/resizing for OnRawOutput/
	// NegotiatedSize) because resizeApplyMu is held for the entire
	// duration of applyNegotiatedSize's SetWindowSize ->
	// quiescence-wait -> CapturePaneContent pipeline, which can run for
	// hundreds of milliseconds. Without it, two subscribers voting for a
	// resize near-simultaneously could each independently observe
	// changed == true (under resizeMu, released before applyNegotiatedSize
	// runs) and both call applyNegotiatedSize concurrently — violating its
	// doc comment's "exactly once per call" contract and this package's
	// single-owner concurrency guarantee, which is the entire point of the
	// stream-hub redesign.
	resizeApplyMu sync.Mutex

	// slowSubscriberDropsTotal counts slow-subscriber evictions (never
	// individual dropped frames — Story 1.4.2's AC is explicit that this must
	// increment once per eviction, not once per dropped frame). This is a
	// minimal internal counter standing in for the
	// streamhub_slow_subscriber_drops_total metric named in plan.md's
	// Observability Plan; wiring it to real Atlas/OTel metrics is Epic 3.2's
	// scope ("Registry consolidation & observability"), not this epic's.
	slowSubscriberDropsTotal atomic.Int64

	// snapshotMu guards lastSnapshot — kept separate from mu (the
	// subscriber-registry lock) for the same reason resizeMu is separate:
	// AttachSubscriber reads it, and applyNegotiatedSize writes it, and
	// neither should contend with unrelated subscriber-registry operations.
	snapshotMu sync.Mutex

	// lastSnapshot is the most recent pane content captured by
	// applyNegotiatedSize's post-quiescence CapturePaneContent call, if any.
	// AttachSubscriber's CatchUpSnapshot (Story 1.2.1's AC) prefers this
	// cached value over issuing a fresh CapturePaneContent call, per this
	// project's root-cause concern about redundant capture-pane calls
	// (research/pitfalls.md) — nil until the first successful resize
	// quiescence cycle completes.
	lastSnapshot []byte

	// pumpActive tracks whether a production raw-output pump
	// (pumpControlModeOutputIntoHub, server/services) is currently running
	// for this hub. Guarded by mu, not a separate lock: TryStartPump/
	// MarkPumpExited are rare, non-blocking bookkeeping calls, not part of
	// any hot path this hub's other locks were split out to protect.
	// See TryStartPump's doc comment for why this exists.
	pumpActive bool
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
		batchMaxWindow:        MaxBatchWindow,
		state:                 HubStarting,
		subscribers:           make(map[SubscriberID]*subscriber),
	}
	for _, opt := range opts {
		opt(h)
	}
	h.batchWindow = NewBatchWindow(h.onBatchFlush, WithMaxBatchWindow(h.batchMaxWindow))

	incActiveHubs()
	log.Info("streamhub hub created", "session", sessionName, "resolved_path", "hub_owned")
	return h
}

// onBatchFlush is the hub's BatchWindow.onFlush callback: it broadcasts the
// flushed unit's bytes to every attached subscriber, after logging and
// recording the flush event (Story 3.2.2) — frames coalesced, byte count,
// and trigger reason, the same information an operator would otherwise only
// be able to reconstruct from `BatchWindow.FlushCount`/`TimersArmed`'s
// test-only instrumentation. Stamping unit.Seq/unit.Reason into the actual
// wire envelope a subscriber receives is Epic 2.2's WebSocketTransport
// scope, not this epic's.
func (h *StreamHub) onBatchFlush(unit BroadcastUnit) {
	// Debug, not Info: this fires on every coalesced flush (up to ~1/batch
	// window, i.e. many times per second per active session during
	// continuous output) — Story 3.2.2's flush telemetry is better read from
	// recordBatchFlushFramesCoalesced's metric than from a log line per flush.
	log.Debug("streamhub batch flushed",
		"session", h.sessionName,
		"frames_coalesced", unit.FramesCoalesced,
		"bytes", len(unit.Data),
		"reason", unit.Reason.String())
	recordBatchFlushFramesCoalesced(unit.FramesCoalesced)
	h.Broadcast(unit.Data)
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

// TryStartPump atomically claims the right to run this hub's raw-output
// pump, returning true for exactly one caller. AttachSubscriber's doc
// comment notes that reactivating a HubTornDown hub is HubRegistry's policy
// to own — this is that policy's missing half: a hub that has fully torn
// down (0 subscribers, grace period expired) has no pump goroutine left
// feeding it (MarkPumpExited already flipped this back to false), and
// nothing else restarts one on reattach, silently losing live output for
// the rest of the process's life. Callers (HubRegistry.GetOrCreate) should
// call this unconditionally after every LoadOrCompute — a healthy hub with
// pumpActive already true simply loses the CAS and spawns nothing, so this
// is safe to call on every reconnect, not just a detected cache-hit case.
func (h *StreamHub) TryStartPump() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pumpActive {
		return false
	}
	h.pumpActive = true
	return true
}

// MarkPumpExited records that this hub's raw-output pump has stopped
// running — called by pumpControlModeOutputIntoHub immediately before each
// of its return points, so TryStartPump's next caller can correctly restart
// one. See TryStartPump's doc comment.
func (h *StreamHub) MarkPumpExited() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pumpActive = false
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
	if h.teardownInFlight {
		// ForceTeardown is mid-StopControlMode-call for this exact attach —
		// tell its final state write to skip claiming HubTornDown even if
		// this subscriber detaches again before that call returns (see
		// ForceTeardown's doc comment).
		h.reactivatedDuringTeardown = true
	}
	switch h.state {
	case HubStarting, HubActive, HubDraining, HubTornDown:
		h.state = HubActive
	default:
		panic("unhandled HubLifecycleState")
	}
	count := len(h.subscribers)
	h.mu.Unlock()

	// Story 1.2.1's AC: a newly-attached subscriber receives a CatchUpSnapshot
	// within this same call, not just the browser WebSocket path via its own
	// ad-hoc workaround (server/services/connectrpc_websocket.go) — every
	// Transport (MuxTransport, any future one) now gets the current pane
	// immediately instead of a blank pane until the next broadcast/resize.
	// This runs before startWriter so it is guaranteed to be the very first
	// Send call this transport ever receives, with no writer goroutine yet
	// running to race it.
	h.sendCatchUpSnapshot(sub)

	sub.startWriter(h.handleSubscriberSendError)

	log.Info("streamhub subscriber attached",
		"session", h.sessionName, "subscriber_id", string(id), "subscriber_count", count,
		"transport_type", fmt.Sprintf("%T", transport),
		"can_resize", capability.CanResize, "can_write", capability.CanWrite)
	recordSubscribersPerHub(count)
	return id
}

// sendCatchUpSnapshot delivers the hub's current pane content directly to a
// newly-attached subscriber's Transport.Send (Story 1.2.1's AC), bypassing
// the subscriber's outbound queue/writer goroutine entirely — it runs before
// startWriter is ever called for sub, so there is no writer yet to race.
//
// A missing snapshot (h.controller is nil, as in most unit tests; or
// CapturePaneContent errors, most commonly because this is the very first
// subscriber of a brand-new hub and nothing has ever been captured yet) is
// graceful degradation, not a failure of AttachSubscriber: it is logged and
// skipped rather than treated as fatal.
func (h *StreamHub) sendCatchUpSnapshot(sub *subscriber) {
	content, ok := h.currentSnapshot()
	if !ok {
		return
	}
	if err := sub.transport.Send(content); err != nil {
		log.Warn("streamhub: CatchUpSnapshot send failed for newly-attached subscriber",
			"session", h.sessionName, "subscriber_id", string(sub.id), "error", err)
	}
}

// currentSnapshot returns the hub's best-known current pane content: the
// cached result of the most recent successful post-quiescence
// CapturePaneContent call (applyNegotiatedSize) if one exists, or a fresh
// SessionController capture otherwise — never both, so an attach never pays
// for a redundant capture-pane call when a recent one is already known-good
// (this project's own root-cause concern about redundant captures,
// research/pitfalls.md). A successful on-demand capture is itself cached, so
// only the first subscriber of a hub that has never resized ever triggers a
// real capture-pane call; every later attach before the next resize reuses
// it.
//
// ok is false when no snapshot is available at all: h.controller is nil
// (tests that never exercise the controller surface), CapturePaneContent
// errors, or it succeeds with empty content (nothing has ever been rendered
// to this pane yet).
func (h *StreamHub) currentSnapshot() (content []byte, ok bool) {
	h.snapshotMu.Lock()
	cached := h.lastSnapshot
	h.snapshotMu.Unlock()
	if cached != nil {
		return cached, true
	}

	if h.controller == nil {
		return nil, false
	}
	captured, err := h.controller.CapturePaneContentRaw()
	if err != nil {
		log.Info("streamhub: no CatchUpSnapshot available for newly-attached subscriber",
			"session", h.sessionName, "error", err)
		return nil, false
	}
	if captured == "" {
		return nil, false
	}

	prepared := ansiSnapshotPrefix + prepareSnapshotContent(captured)
	capturedBytes := []byte(withCursorSync(prepared, h.controller))
	h.snapshotMu.Lock()
	h.lastSnapshot = capturedBytes
	h.snapshotMu.Unlock()
	return capturedBytes, true
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
			h.slowSubscriberDropsTotal.Add(1)
			recordSlowSubscriberDrop()
			h.removeSubscriber(sub.id, errSlowSubscriberEvicted)
			return
		}
		sub.clearSlow()
	})
}

// SlowSubscriberDropsTotal returns the number of times a subscriber has been
// evicted for staying slow past its grace period — incremented exactly once
// per eviction, never once per dropped frame. See the field doc comment on
// StreamHub for scoping notes (Epic 3.2 owns wiring this to a real metrics
// backend).
func (h *StreamHub) SlowSubscriberDropsTotal() int64 {
	return h.slowSubscriberDropsTotal.Load()
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
// subscriber is closed, SessionController.StopControlMode() is invoked
// exactly once, and only then does the hub transition to HubTornDown —
// callers polling State() must never observe HubTornDown before
// StopControlMode has actually returned (see
// TestStreamHub_should_NotReportHubTornDown_While_StopControlModeInFlight).
// It is the single teardown code path reached both by grace-period expiry
// (onTeardownGraceExpired) and an external trigger, e.g. Story 3.1.2's
// flag-flip-to-legacy case. Safe to call concurrently or more than once:
// teardownInFlight, not h.state, rejects a second caller, since h.state
// can't serve as that guard until the transition completes.
func (h *StreamHub) ForceTeardown() error {
	h.mu.Lock()
	if h.state == HubTornDown || h.teardownInFlight {
		h.mu.Unlock()
		return nil
	}
	h.teardownInFlight = true
	h.reactivatedDuringTeardown = false
	h.cancelPendingTeardownLocked()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		subs = append(subs, sub)
	}
	subscriberCountAtTeardown := len(h.subscribers)
	h.subscribers = make(map[SubscriberID]*subscriber)
	// HubDraining, not left stale at whatever h.state was before this call
	// (e.g. HubActive) — every real subscriber is already committed to
	// closing at this point, so leaving HubActive's "at least one
	// subscriber is attached" invariant (types.go) visibly false for the
	// entire StopControlMode call below would be its own bug.
	h.state = HubDraining
	h.mu.Unlock()

	decActiveHubs()

	for _, sub := range subs {
		sub.close()
	}

	var stopErr error
	if h.controller != nil {
		stopErr = h.controller.StopControlMode()
	}

	// Only claim HubTornDown if nothing reattached-then-detached while
	// StopControlMode was running. len(h.subscribers)==0 alone can't tell
	// "genuinely idle throughout" apart from "attached and detached again
	// mid-teardown" (removeSubscriber re-arms a fresh HubDraining +
	// teardownTimer for that second detach, via scheduleTeardownLocked) —
	// reactivatedDuringTeardown is the explicit signal that distinguishes
	// them, so a legitimate re-armed grace period isn't silently clobbered
	// back to HubTornDown.
	h.mu.Lock()
	h.teardownInFlight = false
	reactivated := h.reactivatedDuringTeardown
	h.reactivatedDuringTeardown = false
	if !reactivated && len(h.subscribers) == 0 {
		h.state = HubTornDown
	}
	h.mu.Unlock()

	if stopErr != nil {
		log.Warn("streamhub force teardown: StopControlMode failed",
			"session", h.sessionName, "subscriber_count", subscriberCountAtTeardown, "error", stopErr)
		return stopErr
	}
	log.Info("streamhub torn down", "session", h.sessionName, "subscriber_count", subscriberCountAtTeardown)
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
// to bring every subscriber back in sync instead. Frames that pass the
// resize gate are fed into the hub's single BatchWindow (Epic 2.1) rather
// than broadcast directly, so N attached subscribers share one
// accumulation/coalesce pass per burst instead of each running their own.
func (h *StreamHub) OnRawOutput(data []byte) {
	h.resizeMu.Lock()
	resizing := h.resizing
	h.resizeMu.Unlock()
	if resizing {
		return
	}
	h.batchWindow.Add(data)
}

// TryFlush flushes the hub's BatchWindow immediately if anything is pending,
// with FlushOpportunistic as the reason. This is the hub-level hook a raw
// output feed (pumpControlModeOutputIntoHub) calls once it has drained every
// frame immediately available on its subscription channel — mirroring the
// `default: break coalesce` point in
// server/services/connectrpc_websocket.go's legacy per-connection coalesce
// loop, so a hub-owned burst is not forced to always pay MaxBatchWindow's
// full ceiling latency when the feed momentarily has nothing left to drain.
// A no-op if nothing is buffered (e.g. a resize is in progress and
// OnRawOutput has been suppressing every frame).
func (h *StreamHub) TryFlush() {
	h.batchWindow.TryFlush()
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
		if errors.Is(err, ErrSessionNotStarted) {
			log.Info("streamhub: session not started yet, skipping this resize (will retry on the next negotiated size)", "session", h.sessionName)
			return
		}
		log.Error("streamhub: SetWindowSize failed", "session", h.sessionName, "cols", size.cols, "rows", size.rows, "error", err)
		h.handleControllerError(err)
		return
	}

	subID, updates := h.controller.SubscribeControlModeUpdates()
	waitForQuiescence(updates, h.quiescenceTimeout, h.quiescenceQuietPeriod, h.sessionName)
	h.controller.UnsubscribeControlModeUpdates(subID)

	content, err := h.controller.CapturePaneContentRaw()
	if err != nil {
		if errors.Is(err, ErrSessionNotStarted) {
			log.Info("streamhub: session not started yet, skipping this resize's capture (will retry on the next negotiated size)", "session", h.sessionName)
			return
		}
		log.Error("streamhub: CapturePaneContentRaw failed after resize", "session", h.sessionName, "error", err)
		h.handleControllerError(err)
		return
	}

	// Raw capture-pane output must go through the same
	// erase+home/CRLF-normalize/cursor-sync pipeline as any other
	// full-screen snapshot (snapshot_prepare.go) before subscribers ever
	// see it — sending it unprepared, as this used to, staircased every
	// resize's new content diagonally across whatever the client was
	// already displaying (2026-08-25 reflow bug).
	prepared := []byte(withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(content), h.controller))

	h.snapshotMu.Lock()
	h.lastSnapshot = prepared
	h.snapshotMu.Unlock()

	// The post-resize CatchUpSnapshot bypasses the BatchWindow entirely
	// (Story 2.1.2, research/pitfalls.md §2c/§2d): it must never wait behind
	// whatever raw output happens to be mid-accumulation, since it is itself
	// the authoritative resync point every subscriber is waiting on.
	h.batchWindow.Bypass(prepared)
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
	// The stream-ended sentinel is the same class of urgent control signal as
	// ResizeQuiescence/CatchUpSnapshot (Story 2.1.2): it must reach every
	// subscriber immediately, not wait behind a pending batch that a
	// now-dead controller will never help flush cleanly anyway.
	h.batchWindow.Bypass(streamEndedSentinel)
	_ = h.ForceTeardown()
}
