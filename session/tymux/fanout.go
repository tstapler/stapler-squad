package tymux

import (
	"strconv"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// subscriberBufferSize is each subscriber channel's buffer depth — enough
// to absorb a short burst without an immediate drop, while still bounding
// memory if a subscriber stops reading entirely.
const subscriberBufferSize = 64

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
type ClientFanout struct {
	mu          sync.Mutex
	subscribers map[string]*fanoutSub
	nextID      uint64
	rec         *termlat.Recorder
}

// fanoutSub is one subscriber's queue. A timed subscriber also keeps the enqueue time of each
// queued frame in stamps (same capacity, same order), so Dequeued can report the queue wait
// without changing the frame channel's type.
type fanoutSub struct {
	ch     chan []byte
	stamps chan time.Time // nil for an untimed subscriber
}

var tymuxLabels = termlat.Labels{Backend: "tymux"}

// NewClientFanout constructs an empty ClientFanout.
func NewClientFanout() *ClientFanout { return newClientFanoutRecording(termlat.Default) }

func newClientFanoutRecording(rec *termlat.Recorder) *ClientFanout {
	return &ClientFanout{subscribers: make(map[string]*fanoutSub), rec: rec}
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
	sub := &fanoutSub{ch: make(chan []byte, subscriberBufferSize)}
	if timed {
		// A consumer is between receive and Dequeued for at most one frame, so the stamps can
		// outnumber the queue by one; the slack keeps Broadcast from ever blocking on a stamp.
		sub.stamps = make(chan time.Time, subscriberBufferSize+2)
	}
	f.subscribers[id] = sub
	return id, sub.ch
}

// Dequeued records the queue wait of the oldest frame a timed subscriber has not yet accounted
// for. Call it once per frame received from the channel; extra calls record nothing.
func (f *ClientFanout) Dequeued(id string) {
	f.mu.Lock()
	sub := f.subscribers[id]
	f.mu.Unlock()
	if sub == nil || sub.stamps == nil {
		return
	}
	select {
	case at := <-sub.stamps:
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
		close(sub.ch)
	}
}

// Broadcast sends data to every current subscriber's channel with a
// non-blocking send — a subscriber whose buffer is full drops this frame
// rather than blocking the caller (the standing stream's reader
// goroutine).
func (f *ClientFanout) Broadcast(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, sub := range f.subscribers {
		f.rec.RecordValue(termlat.StageFanoutDepth, tymuxLabels, float64(len(sub.ch)))
		if len(sub.ch) == cap(sub.ch) {
			f.rec.CountDrop(termlat.DropFanoutFull, tymuxLabels.Backend)
			continue
		}
		// Broadcast is the only sender and holds f.mu, so the room just checked cannot vanish.
		if sub.stamps != nil {
			select {
			case sub.stamps <- time.Now():
			default: // a consumer that never calls Dequeued: lose the sample, never block the reader
			}
		}
		sub.ch <- data
	}
}

// count reports the current subscriber count — test-only introspection.
func (f *ClientFanout) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subscribers)
}
