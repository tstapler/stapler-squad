package streamhub

import (
	"sync"
	"sync/atomic"
	"time"
)

// MaxBatchWindow bounds how long a BatchWindow accumulates raw output bytes
// before flushing automatically with FlushCeiling — Story 2.1.1's AC. It is
// deliberately close to the latency ceiling
// server/services/connectrpc_websocket.go's existing per-connection coalesce
// loop already tolerates today (a 32-frame batch cap at typical control-mode
// output rates), so batching never regresses today's perceived latency.
const MaxBatchWindow = 20 * time.Millisecond

// FlushReason records why a BatchWindow flushed a batch, distinguishing an
// opportunistic flush (the caller found a subscriber-write opportunity
// before the ceiling elapsed) from one forced by MaxBatchWindow, and from a
// Bypass call that never went through accumulation at all. Every switch over
// FlushReason must include a default: panic("unhandled FlushReason") case,
// matching HubLifecycleState/StreamPath in types.go (enforced by the
// `exhaustive` linter).
type FlushReason int

const (
	// FlushOpportunistic fires when the caller signals a subscriber-write
	// opportunity (TryFlush) while data is pending.
	FlushOpportunistic FlushReason = iota
	// FlushCeiling fires when MaxBatchWindow elapses since the first byte
	// buffered in the current window, with no opportunistic flush beating it.
	FlushCeiling
	// FlushBypass marks a unit that never touched the accumulation buffer at
	// all (Story 2.1.2) — a control/quiescence message sent via Bypass.
	FlushBypass
)

// String renders FlushReason for logging.
func (r FlushReason) String() string {
	switch r {
	case FlushOpportunistic:
		return "opportunistic"
	case FlushCeiling:
		return "ceiling"
	case FlushBypass:
		return "bypass"
	default:
		panic("unhandled FlushReason")
	}
}

// BroadcastUnit is one flushed batch or bypassed control message, stamped
// with the hub's HubSequenceNumber at the moment it is actually handed to
// onFlush (broadcast time) — never at Add/Bypass call time, so a unit that
// spent longer accumulating never receives a lower sequence number than one
// that bypassed it later (Story 2.1.2's AC).
type BroadcastUnit struct {
	Seq    HubSequenceNumber
	Data   []byte
	Reason FlushReason
}

// batchScratchPool holds reusable accumulation buffers, adapting
// server/services/connectrpc_websocket.go:49's coalesceBufPool pattern (Task
// 2.1.1a) to BatchWindow's accumulation step. Unlike that call site — safe
// to reuse immediately because its downstream marshals/copies the payload
// before returning (connectrpc_websocket.go:805-808's comment) —
// StreamHub.Broadcast hands a flushed frame's bytes directly to each
// subscriber's outbound queue without copying (subscriber.trySend). So a
// scratch buffer here is only ever returned to this pool *after*
// flushLocked has copied its contents into a fresh, independent slice for
// the onFlush handoff — the scratch buffer itself is never handed to
// onFlush and reused afterward, which would race with (or silently corrupt)
// whatever a subscriber's writer goroutine reads from it later.
var batchScratchPool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

// BatchWindowOption configures a BatchWindow at construction time.
type BatchWindowOption func(*BatchWindow)

// WithMaxBatchWindow overrides MaxBatchWindow — tests use this to shrink the
// ceiling so ceiling-flush assertions run in milliseconds.
func WithMaxBatchWindow(d time.Duration) BatchWindowOption {
	return func(b *BatchWindow) { b.maxWindow = d }
}

// BatchWindow is StreamHub's single, hub-owned opportunistic-with-ceiling
// accumulation buffer and timer (Story 2.1.1): every raw output event for
// one hub is appended here as an opaque byte range — concatenated verbatim,
// never truncated, reordered, or content-sniffed — and the accumulated
// bytes are handed to onFlush exactly once per flush regardless of how many
// subscribers are attached, either at the caller's next TryFlush
// (opportunistic) or after maxWindow elapses since the first buffered byte
// (ceiling), whichever comes first. Exactly one BatchWindow exists per hub,
// so exactly one accumulation buffer and one *time.Timer exist per hub too,
// never one per subscriber.
type BatchWindow struct {
	onFlush   func(BroadcastUnit)
	maxWindow time.Duration
	seq       sequenceCounter

	mu    sync.Mutex
	buf   *[]byte
	timer *time.Timer

	// flushCount and timersArmed are test-only instrumentation for Task
	// 2.1.1e's call-counting AC (the hub's coalesce step must run once per
	// flush, not once per subscriber) — never read by production code.
	flushCount  atomic.Int64
	timersArmed atomic.Int64
}

// NewBatchWindow constructs a BatchWindow that hands each flushed
// BroadcastUnit to onFlush. onFlush must not block indefinitely — it runs
// synchronously on whichever goroutine triggered the flush (an Add/TryFlush
// caller, or the ceiling timer's own goroutine).
func NewBatchWindow(onFlush func(BroadcastUnit), opts ...BatchWindowOption) *BatchWindow {
	buf := batchScratchPool.Get().(*[]byte)
	*buf = (*buf)[:0]
	b := &BatchWindow{
		onFlush:   onFlush,
		maxWindow: MaxBatchWindow,
		buf:       buf,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Add appends data to the pending accumulation buffer as an opaque byte
// range. Arms the ceiling timer exactly once per window, the moment the
// buffer transitions from empty to non-empty; every subsequent Add call
// within the same still-pending window never re-arms it — the invariant
// that keeps "exactly one timer per hub" true across an arbitrarily long
// burst of events.
func (b *BatchWindow) Add(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	first := len(*b.buf) == 0
	*b.buf = append(*b.buf, data...)
	if first {
		b.armCeilingLocked()
	}
}

// armCeilingLocked starts the ceiling timer. Callers must hold b.mu; only
// called from Add's empty-to-non-empty transition, so at most one
// outstanding timer exists per BatchWindow at any time.
func (b *BatchWindow) armCeilingLocked() {
	b.timersArmed.Add(1)
	b.timer = time.AfterFunc(b.maxWindow, b.onCeilingExpired)
}

// onCeilingExpired runs on the timer goroutine once maxWindow has elapsed
// with no opportunistic flush beating it. It re-checks the buffer under the
// lock, so a flush that raced the timer (TryFlush already drained the
// buffer, but the timer's firing was already in flight) can't cause a
// spurious empty flush.
func (b *BatchWindow) onCeilingExpired() {
	b.mu.Lock()
	if len(*b.buf) == 0 {
		b.mu.Unlock()
		return
	}
	b.flushLocked(FlushCeiling)
}

// TryFlush flushes any pending accumulated bytes immediately, with reason
// FlushOpportunistic. This is the hook for "the next subscriber-write
// opportunity" (Story 2.1.1's AC): a caller feeding raw output into Add
// calls TryFlush once it has drained every immediately-available event,
// mirroring the `default: break coalesce` point in
// server/services/connectrpc_websocket.go's existing per-connection coalesce
// loop. A no-op if nothing is buffered.
func (b *BatchWindow) TryFlush() {
	b.mu.Lock()
	if len(*b.buf) == 0 {
		b.mu.Unlock()
		return
	}
	b.flushLocked(FlushOpportunistic)
}

// flushLocked hands the pending buffer's contents to onFlush and resets the
// buffer for the next window. Callers must hold b.mu; flushLocked releases
// it before invoking onFlush so a concurrent Add for the *next* window is
// never blocked behind a slow subscriber fan-out.
func (b *BatchWindow) flushLocked(reason FlushReason) {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}

	// Copy out before handoff — see batchScratchPool's doc comment for why
	// this copy (unlike coalesceBufPool's) is load-bearing, not incidental.
	data := append([]byte(nil), (*b.buf)...)
	scratch := b.buf
	*scratch = (*scratch)[:0]
	fresh := batchScratchPool.Get().(*[]byte)
	*fresh = (*fresh)[:0]
	b.buf = fresh
	b.mu.Unlock()

	batchScratchPool.Put(scratch)
	b.flushCount.Add(1)
	b.onFlush(BroadcastUnit{Seq: b.seq.next(), Data: data, Reason: reason})
}

// Bypass immediately delivers data as its own BroadcastUnit, stamped with
// the next HubSequenceNumber, without touching the pending accumulation
// buffer at all (Story 2.1.2) — ResizeQuiescence signals and the post-resize
// CatchUpSnapshot must never wait behind a pending batch's
// opportunistic-or-ceiling flush (research/pitfalls.md §2c/§2d). Any
// already-pending batch still flushes later on its own normal schedule;
// Bypass never cancels or folds into it.
func (b *BatchWindow) Bypass(data []byte) {
	b.onFlush(BroadcastUnit{Seq: b.seq.next(), Data: data, Reason: FlushBypass})
}

// FlushCount reports how many times the accumulation/coalesce step
// (flushLocked) has executed — test-only instrumentation for Task 2.1.1e's
// call-counting AC (must be 1 for an N-subscriber burst, not N).
func (b *BatchWindow) FlushCount() int64 { return b.flushCount.Load() }

// TimersArmed reports how many times the ceiling timer has been armed —
// test-only instrumentation proving at most one timer exists per window
// regardless of how many Add calls or subscribers are involved.
func (b *BatchWindow) TimersArmed() int64 { return b.timersArmed.Load() }
