package streamhub

import (
	"github.com/tstapler/stapler-squad/internal/proflabel"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// noProgressCheck is a droppedAtSent sentinel: the writer has already shown progress, so the
// next repaint need not wait for more.
const noProgressCheck = ^uint64(0)

// latencyLabeler is implemented by a Transport that knows which backend and listener its
// observations belong to; others are recorded under "unknown".
type latencyLabeler interface{ LatencyLabels() termlat.Labels }

// queuedFrame stamps each enqueue so the writer can record how long the frame waited.
type queuedFrame struct {
	data []byte
	at   time.Time
}

// subscriber is the hub-side handle for one attached Transport: a bounded
// outbound queue drained by a dedicated writer goroutine, plus the
// SubscriberCapability and slow-eviction bookkeeping the hub needs to manage
// it. It is unexported — nothing outside this package touches a subscriber
// directly; callers only ever see the SubscriberID AttachSubscriber returns.
type subscriber struct {
	id         SubscriberID
	transport  Transport
	capability SubscriberCapability
	// upgraded, when set, supersedes capability (see StreamHub.UpgradeSubscriber).
	upgraded atomic.Pointer[SubscriberCapability]

	outbound  chan queuedFrame
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
	// sentFrames counts frames the writer has delivered to the transport. The slow-subscriber timer
	// compares it across the grace, so a client that is slow but moving (e.g. through one long
	// replay batch, where pending only shrinks at batch boundaries) is not mistaken for a stalled
	// one; it also gates repaints, which a writer that has not drained since the drop cannot use.
	sentFrames atomic.Uint64
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
	// dropped is set when a frame was discarded because outbound was full: the client's byte
	// stream now has a hole, so the hub repaints it (see StreamHub.runResync).
	dropped bool
	// droppedAtSent is sentFrames when dropped was first set (or noProgressCheck).
	droppedAtSent uint64
	// resyncing is true from beginResync until finishResync: a repaint capture is in flight, so
	// frames are held (bounded by cap(outbound)-1) instead of queued, and queue only what the
	// capture does not contain once the repaint is placed ahead of them.
	resyncing bool
	held      [][]byte
	heldLost  bool // a frame arrived while held was full

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
		outbound:   make(chan queuedFrame, bufferSize),
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
		go proflabel.Do("streamhub_writer", func() {
			defer close(s.done)
			if !s.drainPending(onSendError) {
				return
			}
			labels := s.latencyLabels()
			for f := range s.outbound {
				sendStart := time.Now()
				termlat.Default.Record(termlat.StageOutputQueueWait, labels, sendStart.Sub(f.at))
				err := s.transport.Send(f.data)
				termlat.Default.Record(termlat.StageOutputSend, labels, time.Since(sendStart))
				if err != nil {
					onSendError(s.id, err)
					return
				}
				s.sentFrames.Add(1)
			}
		})
	})
}

func (s *subscriber) latencyLabels() termlat.Labels {
	if l, ok := s.transport.(latencyLabeler); ok {
		return l.LatencyLabels()
	}
	return termlat.Labels{}
}

// trySend attempts a non-blocking enqueue of data onto the subscriber's
// outbound queue. It reports delivered=false both when the queue is full and
// when the subscriber has already been closed — the caller (StreamHub.deliver)
// treats both identically: this frame is dropped for this subscriber.
//
// stalled is true when the frame was accepted into the replay backlog but the backlog is past
// drainLimit, i.e. the writer is not keeping up; the caller then arms the slow-subscriber timer.
func (s *subscriber) trySend(data []byte) (delivered, stalled bool) {
	delivered, stalled, queueDepth, fullDrop := s.enqueueLocked(data)
	if fullDrop {
		termlat.Default.CountDrop(termlat.DropHubQueueFull, s.latencyLabels().Backend)
	}
	if queueDepth >= 0 {
		// Recorded after s.mu is released: telemetry must not extend the hub's per-frame lock hold.
		termlat.Default.RecordValue(termlat.StageOutputQueue, s.latencyLabels(), float64(queueDepth))
	}
	return delivered, stalled
}

// enqueueLocked does trySend's work under s.mu; queueDepth is -1 unless a frame went onto outbound,
// and fullDrop reports that the frame was dropped on a full queue (for the caller to count unlocked).
func (s *subscriber) enqueueLocked(data []byte) (delivered, stalled bool, queueDepth int, fullDrop bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, false, -1, false
	}
	// Before FirstPaint begins, its capture supersedes anything sent, and no writer drains
	// outbound, so queueing would only fill it and trigger a slow-subscriber eviction.
	if s.awaitingFirstPaint.Load() {
		return true, false, -1, false
	}
	if s.painting {
		stalled = s.paintDone && len(s.pending) >= s.drainLimit
		if len(s.pending) >= maxPaintBacklog || s.pendingBytes >= maxPaintBacklogBytes {
			s.overflow = true
		} else {
			s.pending = append(s.pending, data)
			s.pendingBytes += len(data)
		}
		return true, stalled, -1, false
	}
	if s.resyncing {
		if len(s.held) < cap(s.outbound)-1 {
			s.held = append(s.held, data)
		} else {
			s.heldLost = true
		}
		return true, false, -1, false
	}
	select {
	case s.outbound <- queuedFrame{data: data, at: time.Now()}:
		s.slowSince = time.Time{}
		return true, false, len(s.outbound), false
	default:
		if !s.dropped {
			s.droppedAtSent = s.sentFrames.Load()
		}
		s.dropped = true
		return false, false, -1, true
	}
}

// beginResync claims this subscriber's pending repaint and starts holding new frames until
// finishResync, so every frame left in the queue pre-dates the capture the caller is about to take.
//
// ok is false when nothing was dropped, or when the writer has not delivered a single frame since
// the drop (a repaint would only queue behind a stalled transport and hide the stall from the
// slow-subscriber eviction). The flag is then left set for a later pass.
func (s *subscriber) beginResync() (ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.resyncing || !s.dropped || s.sentFrames.Load() == s.droppedAtSent {
		return false
	}
	s.dropped = false
	s.resyncing = true
	s.held, s.heldLost = nil, false
	return true
}

// beginRepaint claims a repaint regardless of dropped frames (the post-resize settle repaints
// everyone) and holds new frames until finishResync. It returns false when the subscriber is
// closed, or another repaint already holds it (that one will repaint it).
func (s *subscriber) beginRepaint() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.resyncing {
		return false
	}
	s.dropped = false
	s.resyncing = true
	s.held, s.heldLost = nil, false
	return true
}

// redropLocked re-flags the subscriber for another repaint pass.
func (s *subscriber) redropLocked() {
	s.dropped = true
	s.droppedAtSent = noProgressCheck
}

// hasDropped reports whether a dropped frame is waiting to be repainted.
func (s *subscriber) hasDropped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

// finishResync ends a claim from beginResync. With a repaint (the capture succeeded) it replaces
// the queued frames, all of which pre-date the capture, and queues the repaint followed by the
// frames held meanwhile. With none (the capture failed) the queue is kept and only the held
// frames are appended. Either way, if held frames could not all be kept, or there was no repaint,
// the subscriber is re-flagged for another pass.
//
// A subscriber still waiting for its first paint or replaying one keeps its own ordering rules.
func (s *subscriber) finishResync(repaint []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.held
	s.resyncing, s.held = false, nil
	if s.closed {
		return
	}
	if s.painting {
		s.pending = append(s.pending, held...)
		return
	}
	if s.awaitingFirstPaint.Load() {
		return
	}
	complete := repaint != nil && !s.heldLost
	if repaint != nil {
	drain:
		for {
			select {
			case <-s.outbound:
			default:
				break drain
			}
		}
		held = append([][]byte{repaint}, held...)
	}
	for _, data := range held {
		select {
		case s.outbound <- queuedFrame{data: data, at: time.Now()}:
		default:
			complete = false
		}
	}
	if !complete {
		s.redropLocked()
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
			s.sentFrames.Add(1)
		}
	}
}

// currentCapability is the attach-time capability unless UpgradeSubscriber replaced it.
func (s *subscriber) currentCapability() SubscriberCapability {
	if up := s.upgraded.Load(); up != nil {
		return *up
	}
	return s.capability
}
