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
	// resyncing is true between BeginRepaint and FinishRepaint: a repaint capture is in flight, so
	// Broadcast holds frames here (at most cap(ch)-1) instead of queueing them. heldLost records
	// that one did not fit. Guarded by ClientFanout.mu.
	resyncing bool
	held      [][]byte
	heldLost  bool
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
		if sub.resyncing {
			if len(sub.held) < cap(sub.ch)-1 {
				sub.held = append(sub.held, data)
			} else {
				sub.heldLost = true
			}
			continue
		}
		select {
		case sub.ch <- data:
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

// BeginRepaint claims every subscriber that has lost a frame and returns their ids. Until
// FinishRepaint their new output is held, so everything still queued pre-dates the capture the
// caller takes next.
func (f *ClientFanout) BeginRepaint() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id, sub := range f.subscribers {
		if !sub.dropped {
			continue
		}
		sub.dropped = false
		sub.resyncing = true
		sub.held, sub.heldLost = nil, false
		ids = append(ids, id)
	}
	return ids
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

// FinishRepaint ends a BeginRepaint claim. With a repaint (the capture succeeded) each claimed
// subscriber's queued output, all of which pre-dates the capture, is replaced by it, and the
// output held during the capture follows. With none (the capture failed) the queue is kept and
// only the held output is appended. A subscriber that could not keep all held output, or had no
// repaint, is re-flagged for another pass. It returns how many subscribers were repainted.
func (f *ClientFanout) FinishRepaint(ids []string, repaint []byte) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, id := range ids {
		sub, ok := f.subscribers[id]
		if !ok {
			continue
		}
		held := sub.held
		complete := repaint != nil && !sub.heldLost
		sub.resyncing, sub.held, sub.heldLost = false, nil, false
		if repaint != nil {
			// Under f.mu nothing else sends on sub.ch, and a consumer only makes room.
			for drained := false; !drained; {
				select {
				case <-sub.ch:
				default:
					drained = true
				}
			}
			held = append([][]byte{repaint}, held...)
			n++
		}
		for _, d := range held {
			select {
			case sub.ch <- d:
			default:
				complete = false
			}
		}
		if !complete {
			sub.dropped = true
		}
	}
	return n
}

// count reports the current subscriber count — test-only introspection.
func (f *ClientFanout) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subscribers)
}
