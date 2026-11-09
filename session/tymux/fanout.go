package tymux

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/internal/termlat"
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
	// skipThrough discards sequenced frames with seq <= it: a resync already delivered them
	// inside its redraw (see fanout_seq.go).
	skipThrough uint64
	// stamps holds the enqueue time of each queued frame (same order) for a timed subscriber, so
	// Dequeued can report the queue wait without changing the frame channel's type. nil if untimed.
	stamps chan time.Time
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
	// seqOn mirrors seq.valid so DisableSeq, run per legacy frame, can skip the lock.
	seqOn atomic.Bool
	rec   *termlat.Recorder
	// stamps maps a timed subscriber's id to its stamp queue so Dequeued, called once per frame,
	// never takes f.mu.
	stamps sync.Map
}

// sendLocked queues data without blocking and reports whether it fit. A timed subscriber also
// stamps the enqueue time (see Dequeued). Only the broadcasting paths send, all under ClientFanout.mu.
func (s *fanoutSubscriber) sendLocked(data []byte) bool {
	if len(s.ch) == cap(s.ch) {
		return false
	}
	if s.stamps != nil {
		select {
		case s.stamps <- time.Now():
		default: // a consumer that never calls Dequeued: lose the sample, never block the reader
		}
	}
	s.ch <- data
	return true
}

// drainLocked empties the frame queue and its stamps. Only call under ClientFanout.mu (nothing
// else sends; a consumer only makes room).
func (s *fanoutSubscriber) drainLocked() {
	for drained := false; !drained; {
		select {
		case <-s.ch:
		default:
			drained = true
		}
	}
	if s.stamps != nil {
		for drained := false; !drained; {
			select {
			case <-s.stamps:
			default:
				drained = true
			}
		}
	}
}

var tymuxLabels = termlat.Labels{Backend: "tymux"}

// NewClientFanout constructs an empty ClientFanout.
func NewClientFanout() *ClientFanout { return newClientFanoutRecording(termlat.Default) }

func newClientFanoutRecording(rec *termlat.Recorder) *ClientFanout {
	return &ClientFanout{subscribers: make(map[string]*fanoutSubscriber), rec: rec}
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
func (f *ClientFanout) Subscribe() (string, chan []byte) { return f.subscribe(false) }

// SubscribeTimed is Subscribe for a consumer that calls Dequeued after every receive, which
// records how long each frame waited in the queue (termlat.StageFanoutWait).
func (f *ClientFanout) SubscribeTimed() (string, chan []byte) { return f.subscribe(true) }

func (f *ClientFanout) subscribe(timed bool) (string, chan []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := strconv.FormatUint(f.nextID, 10)
	sub := &fanoutSubscriber{ch: make(chan []byte, subscriberBufferSize)}
	if timed {
		// A consumer is between receive and Dequeued for at most one frame, so the stamps can
		// outnumber the queue by one; the slack keeps Broadcast from ever blocking on a stamp.
		sub.stamps = make(chan time.Time, subscriberBufferSize+2)
		f.stamps.Store(id, sub.stamps)
	}
	f.subscribers[id] = sub
	return id, sub.ch
}

// Dequeued records the queue wait of the oldest frame a timed subscriber has not yet accounted
// for. Call it once per frame received from the channel; extra calls record nothing.
func (f *ClientFanout) Dequeued(id string) {
	v, ok := f.stamps.Load(id)
	if !ok {
		return
	}
	select {
	case at := <-v.(chan time.Time):
		f.rec.Record(termlat.StageFanoutWait, tymuxLabels, time.Since(at))
	default:
	}
}

// Unsubscribe removes and closes the subscriber channel for id. A no-op if
// id is unknown or was already unsubscribed.
func (f *ClientFanout) Unsubscribe(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if sub, ok := f.subscribers[id]; ok {
		delete(f.subscribers, id)
		f.stamps.Delete(id)
		close(sub.ch)
	}
}

// Broadcast sends data to every current subscriber's channel with a
// non-blocking send — a subscriber whose buffer is full drops this frame
// rather than blocking the caller (the standing stream's reader
// goroutine).
func (f *ClientFanout) Broadcast(data []byte) {
	var tel fanoutTelemetry
	f.mu.Lock()
	dropped := f.deliverLocked(data, 0, &tel)
	onDrop := f.onDrop
	f.mu.Unlock()
	f.recordTelemetry(&tel)
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
		if !sub.resyncing {
			continue // a sequenced ResyncAt already replaced this claim
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
			if sub.stamps != nil {
				for drained := false; !drained; {
					select {
					case <-sub.stamps:
					default:
						drained = true
					}
				}
			}
			held = append([][]byte{repaint}, held...)
			n++
		}
		for _, d := range held {
			if !sub.sendLocked(d) {
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
