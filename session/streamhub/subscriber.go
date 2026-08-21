package streamhub

import (
	"sync"
	"time"
)

// subscriber is the hub-side handle for one attached Transport: a bounded
// outbound queue drained by a dedicated writer goroutine, plus the
// SubscriberCapability and slow-eviction bookkeeping the hub needs to manage
// it. It is unexported — nothing outside this package touches a subscriber
// directly; callers only ever see the SubscriberID AttachSubscriber returns.
type subscriber struct {
	id         SubscriberID
	transport  Transport
	capability SubscriberCapability

	outbound  chan []byte
	done      chan struct{}
	closeOnce sync.Once

	// mu guards closed and slowSince. closed is checked and set under the
	// same lock as every send attempt (trySend) so a send can never race
	// close's close(s.outbound) — sending on a channel concurrently with
	// closing it is a data race by Go's own channel semantics, so trySend
	// and close serialize through mu rather than relying on outbound's
	// buffering to make that safe.
	mu        sync.Mutex
	closed    bool
	slowSince time.Time // zero value means "not currently observed as slow"
}

// newSubscriber allocates a subscriber with a bufferSize-deep outbound queue.
// It does not start the writer goroutine — call startWriter for that once the
// subscriber is registered with the hub, so onSendError always has a live
// registry entry to evict.
func newSubscriber(id SubscriberID, transport Transport, capability SubscriberCapability, bufferSize int) *subscriber {
	return &subscriber{
		id:         id,
		transport:  transport,
		capability: capability,
		outbound:   make(chan []byte, bufferSize),
		done:       make(chan struct{}),
	}
}

// startWriter launches the subscriber's writer goroutine. It drains outbound
// and forwards each frame to transport.Send in order. A non-nil Send error is
// treated identically to the slow-subscriber-eviction path (Task 1.2.1g): it
// is reported to onSendError exactly once and the goroutine exits — Send is
// never retried and the failure is never hub-fatal. Ranging over outbound is
// safe concurrently with close's close(s.outbound) (only concurrent sends
// are unsafe), so the writer needs no coordination with trySend/close beyond
// the channel itself.
func (s *subscriber) startWriter(onSendError func(id SubscriberID, err error)) {
	go func() {
		defer close(s.done)
		for data := range s.outbound {
			if err := s.transport.Send(data); err != nil {
				onSendError(s.id, err)
				return
			}
		}
	}()
}

// trySend attempts a non-blocking enqueue of data onto the subscriber's
// outbound queue. It reports delivered=false both when the queue is full and
// when the subscriber has already been closed — the caller (StreamHub.deliver)
// treats both identically: this frame is dropped for this subscriber.
func (s *subscriber) trySend(data []byte) (delivered bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}
	select {
	case s.outbound <- data:
		s.slowSince = time.Time{}
		return true
	default:
		return false
	}
}

// markSlow records the first moment this subscriber's outbound queue was
// observed full. It reports true exactly once per stall — the caller arms a
// single grace-period eviction timer on that signal, rather than one timer
// per dropped frame while the stall continues.
func (s *subscriber) markSlow() (firstObservation bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.slowSince.IsZero() {
		s.slowSince = time.Now()
		return true
	}
	return false
}

// clearSlow resets slow-eviction bookkeeping, e.g. once a pending
// eviction-timer check finds the queue has since drained.
func (s *subscriber) clearSlow() {
	s.mu.Lock()
	s.slowSince = time.Time{}
	s.mu.Unlock()
}

// isStillFull reports whether the outbound queue remains at capacity, i.e.
// nothing has drained it since the stall was first observed. Used by the
// grace-period eviction timer to distinguish a subscriber that recovered
// from one that is genuinely stuck. A closed subscriber is reported as not
// still full — it's already being torn down via another path, so the timer
// has nothing left to evict.
func (s *subscriber) isStillFull() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	return len(s.outbound) == cap(s.outbound)
}

// close releases the subscriber's transport and outbound queue exactly once.
// Setting closed under mu before closing the channel guarantees trySend can
// never attempt to send on outbound after (or concurrently with) this close
// call. Closing the transport first lets a well-behaved Transport unblock a
// Send call that is stalled indefinitely (e.g. on a stuck network write), so
// the writer goroutine started by startWriter is guaranteed to observe
// either a Send error or a drained+closed outbound channel and exit — no
// leaked goroutine, regardless of which eviction path triggered the close.
func (s *subscriber) close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()

		_ = s.transport.Close()
		close(s.outbound)
	})
}
