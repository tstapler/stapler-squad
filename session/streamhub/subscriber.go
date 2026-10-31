package streamhub

import (
	"sync"
	"sync/atomic"
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

	// awaitingFirstPaint is true from attach until FirstPaint runs, for a ManualFirstPaint
	// subscriber; its writer is not started meanwhile, so frames queue in outbound.
	awaitingFirstPaint atomic.Bool
	writerOnce         sync.Once

	// Guarded by mu. painting is true from beginFirstPaint until the writer has drained pending:
	// frames then land in pending (they may post-date the capture, so none may be lost) instead
	// of outbound, so the queue cannot fill while the paint and the replay are in flight.
	// overflow records that pending hit maxPaintBacklog.
	firstPaintTimer Timer
	painting        bool
	expired         bool
	pending         [][]byte
	pendingBytes    int
	overflow        bool
	// replaySent counts frames the writer has sent from a held batch. The slow-subscriber timer
	// compares it across the grace, so a client that is slow but moving through one long batch
	// is not mistaken for a stalled one (pending only shrinks at batch boundaries).
	replaySent atomic.Uint64
	// paintDone is set when the paint returned. From then on, pending growing past drainLimit (one
	// queue's worth beyond what the writer has yet to take) counts as the queue being full: frames
	// are still held, none is lost, but the slow-subscriber grace starts, so a transport that
	// stalls during replay is evicted instead of absorbing frames forever.
	paintDone  bool
	drainLimit int

	// mu guards closed and slowSince. closed is checked and set under the
	// same lock as every send attempt (trySend) so a send can never race
	// close's close(s.outbound) — sending on a channel concurrently with
	// closing it is a data race by Go's own channel semantics, so trySend
	// and close serialize through mu rather than relying on outbound's
	// buffering to make that safe.
	mu        sync.Mutex
	closed    bool
	slowSince time.Time // zero value means "not currently observed as slow"

	// resizeMu guards hasVoted/lastVote — this subscriber's last ResizeVote
	// (Story 1.3.1). Kept separate from mu (the send-path lock) so a
	// negotiation pass never contends with Broadcast's delivery path.
	resizeMu sync.Mutex
	hasVoted bool
	lastVote TerminalSize
}

// newSubscriber allocates a subscriber with a bufferSize-deep outbound queue.
// It does not start the writer goroutine — call startWriter for that once the
// subscriber is registered with the hub, so onSendError always has a live
// registry entry to evict.
func newSubscriber(id SubscriberID, transport Transport, capability SubscriberCapability, bufferSize int) *subscriber {
	s := &subscriber{
		id:         id,
		transport:  transport,
		capability: capability,
		outbound:   make(chan []byte, bufferSize),
		done:       make(chan struct{}),
	}
	s.awaitingFirstPaint.Store(capability.ManualFirstPaint)
	return s
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
	s.writerOnce.Do(func() {
		go func() {
			defer close(s.done)
			if !s.drainPending(onSendError) {
				return
			}
			for data := range s.outbound {
				if err := s.transport.Send(data); err != nil {
					onSendError(s.id, err)
					return
				}
			}
		}()
	})
}

// trySend attempts a non-blocking enqueue of data onto the subscriber's
// outbound queue. It reports delivered=false both when the queue is full and
// when the subscriber has already been closed — the caller (StreamHub.deliver)
// treats both identically: this frame is dropped for this subscriber.
//
// stalled is true when the frame was accepted into the replay backlog but the backlog is past
// drainLimit, i.e. the writer is not keeping up; the caller then arms the slow-subscriber timer.
func (s *subscriber) trySend(data []byte) (delivered, stalled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, false
	}
	// Before FirstPaint begins, its capture supersedes anything sent, and no writer drains
	// outbound, so queueing would only fill it and trigger a slow-subscriber eviction.
	if s.awaitingFirstPaint.Load() {
		return true, false
	}
	if s.painting {
		stalled = s.paintDone && len(s.pending) >= s.drainLimit
		if len(s.pending) >= maxPaintBacklog || s.pendingBytes >= maxPaintBacklogBytes {
			s.overflow = true
		} else {
			s.pending = append(s.pending, data)
			s.pendingBytes += len(data)
		}
		return true, stalled
	}
	select {
	case s.outbound <- data:
		s.slowSince = time.Time{}
		return true, false
	default:
		return false, false
	}
}

// markSlow records the first moment this subscriber's outbound queue was
// observed full. It reports true exactly once per stall — the caller arms a
// single grace-period eviction timer on that signal, rather than one timer
// per dropped frame while the stall continues.
func (s *subscriber) markSlow(now time.Time) (firstObservation bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.slowSince.IsZero() {
		s.slowSince = now
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
	if s.painting && s.paintDone {
		return len(s.pending) >= s.drainLimit
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
		if s.firstPaintTimer != nil {
			s.firstPaintTimer.Stop()
		}
		s.mu.Unlock()

		_ = s.transport.Close()
		close(s.outbound)
	})
}

// recordResizeVote stores size as this subscriber's most recent ResizeVote
// (Task 1.3.1b). Callers are responsible for the SubscriberCapability.CanResize
// gate — recordResizeVote itself has no opinion on capability.
func (s *subscriber) recordResizeVote(size TerminalSize) {
	s.resizeMu.Lock()
	defer s.resizeMu.Unlock()
	s.hasVoted = true
	s.lastVote = size
}

// currentResizeVote returns this subscriber's last recorded ResizeVote and
// whether it has ever voted. A subscriber that has never called
// RequestResize reports voted=false — negotiation treats that as "no
// constraint from this subscriber" rather than defaulting it to a hardcoded
// size (GoTTY-bug avoidance, Task 1.3.1e).
func (s *subscriber) currentResizeVote() (size TerminalSize, voted bool) {
	s.resizeMu.Lock()
	defer s.resizeMu.Unlock()
	return s.lastVote, s.hasVoted
}

// maxPaintBacklog bounds the frames held while a first paint is in flight; past it the paint
// fails visibly (errPaintBacklog) rather than silently leaving a gap.
const maxPaintBacklog = 4096

// maxPaintBacklogBytes bounds the same list by size: a batch unit is not capped (a flush
// carries every byte read in the 20 ms window, so a bulk `cat` makes megabyte units), and 4096
// of those would hold gigabytes per subscriber. Checked before appending, so one oversized
// frame is still held and the next one overflows, failing the paint like the count bound does.
const maxPaintBacklogBytes = 16 << 20

// armFirstPaintTimeout evicts (via onExpire) a subscriber still awaiting its FirstPaint after d.
// The check and beginFirstPaint share mu, so it can never evict a subscriber mid-paint.
func (s *subscriber) armFirstPaintTimeout(clock Clock, d time.Duration, onExpire func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.firstPaintTimer = clock.AfterFunc(d, func() {
		s.mu.Lock()
		expire := s.awaitingFirstPaint.Load() && !s.closed
		if expire {
			s.expired = true
		}
		s.mu.Unlock()
		if expire {
			onExpire()
		}
	})
}

// beginFirstPaint leaves the awaiting state and starts holding frames. It reports false if the
// subscriber already expired or closed.
func (s *subscriber) beginFirstPaint() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expired || s.closed {
		return false
	}
	if s.firstPaintTimer != nil {
		s.firstPaintTimer.Stop()
	}
	if s.awaitingFirstPaint.CompareAndSwap(true, false) {
		s.painting = true
	}
	return true
}

// endFirstPaint reports whether any frame was lost to maxPaintBacklog during the paint, and
// starts the replay stall bound. Frames keep going to pending until the writer's drainPending has caught up.
func (s *subscriber) endFirstPaint() (overflowed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paintDone = true
	s.drainLimit = len(s.pending) + cap(s.outbound)
	return s.overflow
}

// drainPending sends the frames held during a first paint, in order, and switches the
// subscriber to the live outbound queue once none are left, so no frame can fall between the two.
// It reports false if the writer should stop (send error, or the backlog overflowed).
func (s *subscriber) drainPending(onSendError func(id SubscriberID, err error)) bool {
	for {
		s.mu.Lock()
		if s.overflow {
			s.mu.Unlock()
			onSendError(s.id, errPaintBacklog)
			return false
		}
		batch := s.pending
		s.pending = nil
		s.pendingBytes = 0
		s.drainLimit = cap(s.outbound)
		if len(batch) == 0 {
			s.painting = false
			s.mu.Unlock()
			return true
		}
		s.mu.Unlock()
		for _, data := range batch {
			if err := s.transport.Send(data); err != nil {
				onSendError(s.id, err)
				return false
			}
			s.replaySent.Add(1)
		}
	}
}
