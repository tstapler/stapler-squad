package repostate

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"golang.org/x/sync/singleflight"
)

// Lock-free (ADR-009 item 7): the index is an xsync.Map, eviction is the
// CLOCK hand of cache_clock.go, and a whole-cache Clear is one atomic core
// swap. The only locks left are xsync bucket locks around non-blocking compute
// functions and singleflight's own, none held across a storer read.

// CacheStats is a point-in-time snapshot of one cache's counters.
type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	// Coalesced counts GetOrCompute callers that shared another caller's computation.
	Coalesced int64
	Entries   int
	Bytes     int64
	// CASRetries counts lost compare-and-swaps in the eviction path (CLOCK hand
	// claims and entry state changes); zero for caches without one.
	CASRetries int64
}

// RepoCache is a byte-budgeted CLOCK cache keyed by a comparable struct key,
// with singleflight on misses. A Get hit allocates nothing and takes no lock.
// Errors are never stored. Safe for concurrent use. A hit sets the entry's
// reference bit; when an insert pushes the cache over budget the hand gives
// referenced entries a second chance and evicts the first unreferenced one.
type RepoCache[K comparable, V any] struct {
	opts   Options
	budget int64
	ttl    time.Duration // zero means entries never expire
	sizeOf func(V) int
	yield  func() // test seam: called before every CAS and publishing store

	core atomic.Pointer[clockCore[K, V]]

	sf        singleflight.Group //nolint:exhaustruct
	hits      atomic.Int64
	misses    atomic.Int64
	counters  clockCounters
	coalesced atomic.Int64
	joined    atomic.Int64 // callers registered in a flight; lets tests wait without sleeping
}

// NewRepoCache returns a cache holding at most budgetBytes (as measured by
// sizeOf). A positive ttl expires entries lazily on read, using opts.Clock.
func NewRepoCache[K comparable, V any](budgetBytes int64, ttl time.Duration, sizeOf func(V) int, opts Options) *RepoCache[K, V] {
	c := &RepoCache[K, V]{opts: opts, budget: budgetBytes, ttl: ttl, sizeOf: sizeOf, yield: func() {}}
	c.core.Store(newClockCore[K, V]())
	return c
}

// Get returns the cached value for key and marks it referenced.
func (c *RepoCache[K, V]) Get(key K) (V, bool) {
	core := c.core.Load()
	e, ok := core.index.Load(key)
	if ok {
		// An entry retired after this Load (replaced, or claimed by the hand
		// but not yet unindexed) was current when Get read it, so its value is
		// still a valid answer; only expiry turns a loaded entry into a miss.
		switch {
		case e.expires != 0 && c.opts.now().UnixNano() >= e.expires:
			if core.retire(e, c.yield, &c.counters) {
				core.unindex(e)
			}
			ok = false
		case e.state.Load()&entryReferenced == 0:
			e.state.Or(entryReferenced)
		}
	}
	if !ok {
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	c.hits.Add(1)
	return e.val, true
}

// Put stores val under key, evicting with the CLOCK hand to stay within
// budget. A value larger than the whole budget, and any value stored while the
// pressure function reports pressure, is not retained. Bytes are added before
// the entry is published and subtracted exactly once by whoever retires it.
func (c *RepoCache[K, V]) Put(key K, val V) {
	if c.opts.underPressure() {
		return
	}
	size := int64(c.sizeOf(val))
	if size > c.budget {
		return
	}
	e := &clockEntry[K, V]{key: key, val: val, size: size}
	if c.ttl > 0 {
		e.expires = c.opts.now().Add(c.ttl).UnixNano()
	}
	core := c.core.Load()
	core.bytes.Add(size)
	var old *clockEntry[K, V]
	c.yield()
	core.index.Compute(key, func(cur *clockEntry[K, V], loaded bool) (*clockEntry[K, V], xsync.ComputeOp) {
		if loaded {
			old = cur
		}
		return e, xsync.UpdateOp
	})
	if old != nil {
		core.retire(old, c.yield, &c.counters)
	}
	core.ring.push(e)
	core.sweep(c.budget, e, c.yield, &c.counters)
	core.compact(c.yield, &c.counters)
}

// Delete removes key if present.
func (c *RepoCache[K, V]) Delete(key K) {
	core := c.core.Load()
	var old *clockEntry[K, V]
	core.index.Compute(key, func(cur *clockEntry[K, V], loaded bool) (*clockEntry[K, V], xsync.ComputeOp) {
		if !loaded {
			return cur, xsync.CancelOp
		}
		old = cur
		return nil, xsync.DeleteOp
	})
	if old != nil {
		core.retire(old, c.yield, &c.counters)
	}
}

// Bytes returns the bytes currently held, lock-free (Extension callback contract).
func (c *RepoCache[K, V]) Bytes() int64 { return c.core.Load().bytes.Load() }

// Drop discards every entry (the Product eviction hook).
func (c *RepoCache[K, V]) Drop() { c.Clear() }

// Clear drops every entry by swapping in an empty core with one atomic store;
// callers already holding the old core finish against it and the GC reclaims it.
func (c *RepoCache[K, V]) Clear() {
	c.yield()
	c.core.Store(newClockCore[K, V]())
}

// GetOrCompute returns the cached value or runs fn once for all concurrent
// callers of the same key. fn runs outside
// every cache lock. Errors, including ErrNotFound, are returned and not stored.
func (c *RepoCache[K, V]) GetOrCompute(key K, fn func() (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	return c.computeMiss(key, fn)
}

// computeMiss is GetOrCompute for a caller that already saw a miss from Get
// (so the miss is not counted twice and the hit path stays closure-free).
func (c *RepoCache[K, V]) computeMiss(key K, fn func() (V, error)) (V, error) {
	var led bool // written by the flight goroutine, read after <-ch
	ch := c.sf.DoChan(fmt.Sprint(key), func() (out any, doErr error) {
		defer func() {
			if r := recover(); r != nil {
				doErr = fmt.Errorf("repostate compute panic: %v", r)
			}
		}()
		led = true
		v, fnErr := fn()
		if fnErr != nil {
			return nil, fnErr
		}
		c.Put(key, v)
		return v, nil
	})
	// DoChan has registered this caller in the flight by now.
	c.joined.Add(1)
	r := <-ch
	if !led {
		c.coalesced.Add(1)
	}
	if r.Err != nil {
		var zero V
		return zero, r.Err
	}
	return r.Val.(V), nil
}

// Stats returns a snapshot of the counters.
func (c *RepoCache[K, V]) Stats() CacheStats {
	core := c.core.Load()
	return CacheStats{
		Hits:       c.hits.Load(),
		Misses:     c.misses.Load(),
		Evictions:  c.counters.evictions.Load(),
		Coalesced:  c.coalesced.Load(),
		Entries:    core.index.Size(),
		Bytes:      core.bytes.Load(),
		CASRetries: int64(c.counters.casRetries.V.Load()),
	}
}

// ClockStats are the CLOCK-specific counters (Observability Plan: cas_retries_total).
type ClockStats struct {
	CASRetries    uint64 // lost CASes on hand claims and entry state
	SecondChances uint64 // reference bits cleared by the hand
	HandSteps     uint64 // entries the hand examined
}

// ClockStats snapshots the eviction counters.
func (c *RepoCache[K, V]) ClockStats() ClockStats {
	return ClockStats{
		CASRetries:    c.counters.casRetries.V.Load(),
		SecondChances: c.counters.secondChances.Load(),
		HandSteps:     c.counters.handSteps.Load(),
	}
}

// ErrNotFound marks a negative result; callers return it from a compute
// function to signal "absent", and the cache never stores it.
var ErrNotFound = errors.New("repostate: not found")
