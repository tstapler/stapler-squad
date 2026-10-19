package repostate

import (
	"runtime"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

// CLOCK byte-budget eviction (ADR-009 item 7, Story 2.3.1). No mutex anywhere
// on the hit, insert or eviction path:
//
//   - the index is an xsync.Map from key to *clockEntry;
//   - an append-only ring of chunked slots holds each live entry exactly once;
//     slot indices only grow within a core, so a slot is never reused (no ABA)
//     and reclamation is the GC's (a reader holding an evicted entry still
//     reads valid memory);
//   - the hand is one atomic index claimed by CAS, and what it decides is
//     decided on the entry's own state word, never on a slot;
//   - an entry's bytes are subtracted exactly once, by whoever wins the
//     compare-and-swap that sets Evicted, and added exactly once at insert
//     before the entry is published.
//
// A second chance re-appends the entry at the tail (a fresh slot) instead of
// moving the hand backwards. Clear swaps in a whole new core by one atomic
// store, so racing readers finish against the old one.

const (
	entryReferenced uint32 = 1 << iota // set by a hit, cleared by the hand
	entryEvicted                       // terminal: set once, by the CAS winner

	clockChunkSlots = 256
	// maxStallSpins bounds the hand's wait for a producer that claimed a slot
	// but has not stored its entry yet (a few instructions in practice).
	maxStallSpins = 64
	// maxIdleSweeps bounds how long a sweeper waits for entries other
	// sweepers hold; it only matters if one of them stalls.
	maxIdleSweeps = 1024
)

type clockEntry[K comparable, V any] struct {
	key     K
	val     V
	size    int64
	expires int64 // unix nanoseconds; zero means no TTL
	state   atomic.Uint32
}

type clockChunk[K comparable, V any] struct {
	base  uint64
	slots [clockChunkSlots]atomic.Pointer[clockEntry[K, V]]
	next  atomic.Pointer[clockChunk[K, V]]
}

// clockRing is the append-only slot log the hand sweeps.
type clockRing[K comparable, V any] struct {
	head atomic.Pointer[clockChunk[K, V]] // chunk holding the hand, or older
	tail padded[atomic.Uint64]            // next slot index to claim
	hand padded[atomic.Uint64]            // next slot index to process
}

func newClockRing[K comparable, V any]() *clockRing[K, V] {
	r := &clockRing[K, V]{}
	r.head.Store(&clockChunk[K, V]{})
	return r
}

// chunkFor returns the chunk holding slot idx, creating missing chunks. It
// starts at head, which never passes an unconsumed slot.
func (r *clockRing[K, V]) chunkFor(idx uint64) *clockChunk[K, V] {
	c := r.head.Load()
	for idx >= c.base+clockChunkSlots {
		n := c.next.Load()
		if n == nil {
			fresh := &clockChunk[K, V]{base: c.base + clockChunkSlots}
			if !c.next.CompareAndSwap(nil, fresh) {
				continue
			}
			n = fresh
		}
		c = n
	}
	return c
}

func (r *clockRing[K, V]) push(e *clockEntry[K, V]) {
	idx := r.tail.V.Add(1) - 1
	c := r.chunkFor(idx)
	c.slots[idx-c.base].Store(e)
}

func (r *clockRing[K, V]) length() uint64 { return r.tail.V.Load() - r.hand.V.Load() }

// pop claims the oldest slot's entry. ok is false when the ring is empty or a
// producer that claimed the next slot never stored its entry within the spin
// bound; the entry stays in the ring for a later sweep.
func (r *clockRing[K, V]) pop(casRetries *padded[atomic.Uint64], yield func()) (e *clockEntry[K, V], ok bool) {
	for spins := 0; ; {
		i := r.hand.V.Load()
		if i >= r.tail.V.Load() {
			return nil, false
		}
		c := r.chunkFor(i)
		if i < c.base {
			continue // i is stale: other poppers consumed it and the head moved on
		}
		slot := &c.slots[i-c.base]
		e = slot.Load()
		if e == nil {
			if r.hand.V.Load() != i {
				continue // another popper took and cleared it: not a stalled producer
			}
			if spins++; spins > maxStallSpins {
				return nil, false
			}
			runtime.Gosched()
			continue
		}
		yield()
		if !r.hand.V.CompareAndSwap(i, i+1) {
			casRetries.V.Add(1)
			continue
		}
		slot.Store(nil) // the claim made this slot ours alone
		r.advanceHead()
		return e, true
	}
}

// advanceHead drops fully consumed chunks from the front so the GC can take them.
func (r *clockRing[K, V]) advanceHead() {
	for {
		h := r.head.Load()
		if r.hand.V.Load() < h.base+clockChunkSlots {
			return
		}
		n := h.next.Load()
		if n == nil || !r.head.CompareAndSwap(h, n) {
			return
		}
	}
}

// clockCore is one epoch of a cache: index, ring and byte count.
type clockCore[K comparable, V any] struct {
	index *xsync.Map[K, *clockEntry[K, V]]
	ring  *clockRing[K, V]
	bytes atomic.Int64
	// held counts goroutines that popped or are about to pop an entry: an entry
	// in their hands is in neither the ring nor evictable, so an empty ring
	// with held > 0 is transient, not "nothing left to evict".
	held atomic.Int64
}

func newClockCore[K comparable, V any]() *clockCore[K, V] {
	return &clockCore[K, V]{index: xsync.NewMap[K, *clockEntry[K, V]](), ring: newClockRing[K, V]()}
}

// clockCounters are the cache-level counters shared by every core.
type clockCounters struct {
	evictions     atomic.Int64
	casRetries    padded[atomic.Uint64]
	secondChances atomic.Uint64
	handSteps     atomic.Uint64
	// beforeUnindex is a test seam run between an eviction's winning CAS and
	// its index removal, the window compare-and-delete exists for.
	beforeUnindex func()
}

// retire marks e Evicted. Exactly one caller wins and subtracts e's bytes.
func (k *clockCore[K, V]) retire(e *clockEntry[K, V], yield func(), n *clockCounters) (won bool) {
	for {
		st := e.state.Load()
		if st&entryEvicted != 0 {
			return false
		}
		yield()
		if e.state.CompareAndSwap(st, st|entryEvicted) {
			k.bytes.Add(-e.size)
			return true
		}
		n.casRetries.V.Add(1)
	}
}

// unindex removes e from the index only if e is still the stored entry, so a
// fresh entry re-inserted under the same key is never deleted.
func (k *clockCore[K, V]) unindex(e *clockEntry[K, V]) {
	k.index.Compute(e.key, func(cur *clockEntry[K, V], loaded bool) (*clockEntry[K, V], xsync.ComputeOp) {
		if loaded && cur == e {
			return nil, xsync.DeleteOp
		}
		return cur, xsync.CancelOp
	})
}

// sweep advances the hand until bytes fit the budget. A referenced entry gets
// a second chance (bit cleared, re-appended); an unreferenced one is evicted if
// this goroutine wins its state CAS; a dead one is dropped from the ring. The
// step bound keeps a hot, constantly re-referenced set from livelocking it.
// protect is the entry whose insert triggered the sweep.
func (k *clockCore[K, V]) sweep(budget int64, protect *clockEntry[K, V], yield func(), n *clockCounters) {
	maxSteps := 3*k.ring.length() + 16
	idle := 0
	for steps := uint64(0); steps < maxSteps && k.bytes.Load() > budget; steps++ {
		k.held.Add(1)
		e, ok := k.ring.pop(&n.casRetries, yield)
		if !ok {
			k.held.Add(-1)
			// Bailing here would leave the overshoot to nobody: entries other
			// sweepers hold will be re-pushed or evicted, so wait for them.
			// With none held, the bytes over budget are unpushed in-flight
			// inserts whose owners sweep themselves.
			if idle++; idle > maxIdleSweeps || (k.held.Load() == 0 && k.ring.length() == 0) {
				return
			}
			runtime.Gosched()
			steps--
			continue
		}
		n.handSteps.Add(1)
		if e == protect && e.state.Load()&entryEvicted == 0 {
			k.ring.push(e) // the entry being inserted is not its own victim
		} else {
			k.judge(e, yield, n)
		}
		k.held.Add(-1)
	}
}

// judge applies the CLOCK decision to one entry the hand just claimed.
func (k *clockCore[K, V]) judge(e *clockEntry[K, V], yield func(), n *clockCounters) {
	for {
		st := e.state.Load()
		switch {
		case st&entryEvicted != 0:
			return // already retired by a Put, Delete or expiry: leaves the ring
		case st&entryReferenced != 0:
			yield()
			if e.state.CompareAndSwap(st, st&^entryReferenced) {
				n.secondChances.Add(1)
				k.ring.push(e)
				return
			}
		default:
			yield()
			if e.state.CompareAndSwap(st, st|entryEvicted) {
				k.bytes.Add(-e.size)
				if n.beforeUnindex != nil {
					n.beforeUnindex()
				}
				k.unindex(e)
				n.evictions.Add(1)
				return
			}
		}
		n.casRetries.V.Add(1)
	}
}

// compact bounds the ring when entries die by replacement or deletion rather
// than eviction: dead slots are dropped and live entries are re-appended with
// their reference bits untouched, so nothing is evicted. It runs only when the
// ring holds well over twice the live entries.
func (k *clockCore[K, V]) compact(yield func(), n *clockCounters) {
	live := uint64(k.index.Size())
	if k.ring.length() <= 2*live+clockChunkSlots {
		return
	}
	for steps := k.ring.length(); steps > 0 && k.ring.length() > live+clockChunkSlots; steps-- {
		k.held.Add(1)
		e, ok := k.ring.pop(&n.casRetries, yield)
		if !ok {
			k.held.Add(-1)
			return
		}
		if e.state.Load()&entryEvicted == 0 {
			k.ring.push(e)
		}
		k.held.Add(-1)
	}
}
