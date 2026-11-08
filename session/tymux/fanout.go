package tymux

import (
	"strconv"
	"sync"
)

// subscriberBufferSize is each subscriber channel's buffer depth — enough
// to absorb a short burst without an immediate drop, while still bounding
// memory if a subscriber stops reading entirely.
const subscriberBufferSize = 64

// fanoutSubscriber is one subscriber's channel plus whether Broadcast has dropped a frame for it
// since its last repaint.
type fanoutSubscriber struct {
	ch      chan []byte
	dropped bool
	// skipThrough discards sequenced frames with seq <= it: a resync already delivered them
	// inside its redraw (see fanout_seq.go).
	skipThrough uint64
}

// ClientFanout is a local, in-process multi-subscriber broadcast for one
// standing Attach stream's output events (Story 2.3.2): the GoF Observer
// pattern per Pattern Decisions, satisfying SubscribeToControlModeUpdates'
// multi-subscriber contract from a single upstream stream instead of
// opening a second Attach call per subscriber.
//
// Broadcast is non-blocking (drop-if-full) so one slow subscriber can never
// stall the standing stream's reader goroutine — matching the existing
// lossy-broadcast precedent tymuxd's own output_gap semantics already
// establish server-side (ADR-003-adjacent design, see Attach's proto doc).
// A drop leaves a hole in a byte stream the consumer parses statefully, so
// it is reported through SetOnDrop and healed with RepaintDropped.
type ClientFanout struct {
	mu          sync.Mutex
	subscribers map[string]*fanoutSubscriber
	nextID      uint64
	onDrop      func()
	seq         seqState // guarded by mu; see fanout_seq.go
}

// NewClientFanout constructs an empty ClientFanout.
func NewClientFanout() *ClientFanout {
	return &ClientFanout{subscribers: make(map[string]*fanoutSubscriber)}
}

// SetOnDrop registers fn to run (outside the fanout's lock, on the broadcasting goroutine)
// whenever a Broadcast drops a frame for a full subscriber. fn must not block.
func (f *ClientFanout) SetOnDrop(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onDrop = fn
}

// Subscribe registers a new subscriber and returns its id (for
// Unsubscribe) and a channel that receives every subsequent Broadcast
// call's data. The channel is closed by Unsubscribe, never by Broadcast.
func (f *ClientFanout) Subscribe() (string, chan []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := strconv.FormatUint(f.nextID, 10)
	ch := make(chan []byte, subscriberBufferSize)
	f.subscribers[id] = &fanoutSubscriber{ch: ch}
	return id, ch
}

// Unsubscribe removes and closes the subscriber channel for id. A no-op if
// id is unknown or was already unsubscribed.
func (f *ClientFanout) Unsubscribe(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sub, ok := f.subscribers[id]; ok {
		delete(f.subscribers, id)
		close(sub.ch)
	}
}

// Broadcast sends data to every current subscriber's channel with a
// non-blocking send — a subscriber whose buffer is full drops this frame
// rather than blocking the caller (the standing stream's reader
// goroutine).
func (f *ClientFanout) Broadcast(data []byte) {
	f.mu.Lock()
	dropped := f.deliverLocked(data, 0)
	onDrop := f.onDrop
	f.mu.Unlock()
	if dropped && onDrop != nil {
		onDrop()
	}
}

// RepaintDropped replaces the queued, stale output of every subscriber that has lost a frame
// with repaint (a full-screen redraw that already contains it), clears their dropped flag, and
// returns how many it repainted. Subscribers that lost nothing are untouched.
func (f *ClientFanout) RepaintDropped(repaint []byte) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, sub := range f.subscribers {
		if !sub.dropped {
			continue
		}
		sub.dropped = false
		for drained := false; !drained; {
			select {
			case <-sub.ch:
			default:
				drained = true
			}
		}
		sub.ch <- repaint // the channel was just emptied under f.mu, so this cannot block
		n++
	}
	return n
}

// count reports the current subscriber count — test-only introspection.
func (f *ClientFanout) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subscribers)
}
