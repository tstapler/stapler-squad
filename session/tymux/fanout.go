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
	enq     uint64 // frames successfully queued on ch; guarded by ClientFanout.mu
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
	dropped := false
	for _, sub := range f.subscribers {
		select {
		case sub.ch <- data:
			sub.enq++
		default:
			sub.dropped = true
			dropped = true
		}
	}
	onDrop := f.onDrop
	f.mu.Unlock()
	if dropped && onDrop != nil {
		onDrop()
	}
}

// BeginRepaint claims every subscriber that has lost a frame and returns, per subscriber id, how
// many frames it had been sent when claimed. Call it before capturing the screen: a frame counted
// in the mark is contained in that capture, any later one is not.
func (f *ClientFanout) BeginRepaint() map[string]uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var marks map[string]uint64
	for id, sub := range f.subscribers {
		if !sub.dropped {
			continue
		}
		sub.dropped = false
		if marks == nil {
			marks = make(map[string]uint64)
		}
		marks[id] = sub.enq
	}
	return marks
}

// Redrop re-flags the subscribers of an abandoned BeginRepaint claim.
func (f *ClientFanout) Redrop(marks map[string]uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id := range marks {
		if sub, ok := f.subscribers[id]; ok {
			sub.dropped = true
		}
	}
}

// HasDropped reports whether any subscriber is waiting for a repaint.
func (f *ClientFanout) HasDropped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, sub := range f.subscribers {
		if sub.dropped {
			return true
		}
	}
	return false
}

// RepaintDropped replaces, for each claimed subscriber, the queued output that pre-dates the
// capture (everything within its mark) with repaint, a full-screen redraw that already contains
// it. Output queued after the capture stays behind the repaint, so no byte after it is lost. It
// returns how many subscribers it repainted.
//
// A subscriber whose consumer has already taken output newer than the capture, or whose queue
// cannot hold the repaint beside the newer output, is re-flagged instead: repainting it would
// roll its screen back, so a later pass retries with a fresher capture.
func (f *ClientFanout) RepaintDropped(repaint []byte, marks map[string]uint64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for id, mark := range marks {
		sub, ok := f.subscribers[id]
		if !ok {
			continue
		}
		if f.repaintLocked(sub, repaint, mark) {
			n++
		} else {
			sub.dropped = true
		}
	}
	return n
}

// repaintLocked is RepaintDropped for one subscriber; the caller holds f.mu, so nothing else
// sends on sub.ch while the consumer may still be receiving from it.
func (f *ClientFanout) repaintLocked(sub *fanoutSubscriber, repaint []byte, mark uint64) bool {
	post := int(sub.enq - mark)
	qlen := len(sub.ch)
	if post >= cap(sub.ch) || qlen < post {
		return false
	}
	for i := 0; i < qlen-post; i++ {
		select {
		case <-sub.ch:
		default:
		}
	}
	rest := make([][]byte, 0, post)
	for drained := false; !drained; {
		select {
		case d := <-sub.ch:
			rest = append(rest, d)
		default:
			drained = true
		}
	}
	complete := len(rest) == post
	if complete {
		rest = append([][]byte{repaint}, rest...)
	}
	for _, d := range rest {
		sub.ch <- d // sub.ch was emptied under f.mu and rest fits in it, so this cannot block
	}
	return complete
}

// count reports the current subscriber count — test-only introspection.
func (f *ClientFanout) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subscribers)
}
