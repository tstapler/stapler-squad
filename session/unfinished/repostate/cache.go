package repostate

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// Lock order (ADR-001, ADR-009): the cache mutex is a leaf. It guards only the
// index and the eviction order, and is never held across a compute function or
// a storer read. Singleflight internals are likewise leaves. Story 2.3.1 may
// replace the mutex-guarded LRU with a lock-free CLOCK behind the evictor
// interface without touching callers.

// CacheStats is a point-in-time snapshot of one cache's counters.
type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	// Coalesced counts GetOrCompute callers that shared another caller's computation.
	Coalesced int64
	Entries   int
	Bytes     int64
}

// evictor decides eviction order. Methods are called with the cache mutex held.
type evictor[K comparable] interface {
	Insert(k K)
	Touch(k K)
	Remove(k K)
	Victim() (K, bool)
}

// lruEvictor evicts the least recently used key.
type lruEvictor[K comparable] struct {
	order *list.List // most recently used at the front
	elems map[K]*list.Element
}

var _ evictor[string] = (*lruEvictor[string])(nil)

func newLRUEvictor[K comparable]() *lruEvictor[K] {
	return &lruEvictor[K]{order: list.New(), elems: make(map[K]*list.Element)}
}

func (l *lruEvictor[K]) Insert(k K) { l.elems[k] = l.order.PushFront(k) }

func (l *lruEvictor[K]) Touch(k K) {
	if el, ok := l.elems[k]; ok {
		l.order.MoveToFront(el)
	}
}

func (l *lruEvictor[K]) Remove(k K) {
	if el, ok := l.elems[k]; ok {
		l.order.Remove(el)
		delete(l.elems, k)
	}
}

func (l *lruEvictor[K]) Victim() (K, bool) {
	back := l.order.Back()
	if back == nil {
		var zero K
		return zero, false
	}
	return back.Value.(K), true
}

type cacheEntry[V any] struct {
	val     V
	size    int64
	expires time.Time // zero means no TTL
}

// RepoCache is a byte-budgeted LRU keyed by a comparable struct key, with
// singleflight on misses. A Get hit allocates nothing. Errors are never stored.
// Safe for concurrent use.
type RepoCache[K comparable, V any] struct {
	opts   Options
	budget int64
	ttl    time.Duration // zero means entries never expire
	sizeOf func(V) int

	mu      sync.Mutex
	entries map[K]cacheEntry[V]
	order   evictor[K]
	bytes   atomic.Int64 // written under mu; atomic so Bytes() is lock-free (Extension callback contract)

	sf        singleflight.Group //nolint:exhaustruct
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
	coalesced atomic.Int64
	joined    atomic.Int64 // callers registered in a flight; lets tests wait without sleeping
}

// NewRepoCache returns a cache holding at most budgetBytes (as measured by
// sizeOf). A positive ttl expires entries lazily on read, using opts.Clock.
func NewRepoCache[K comparable, V any](budgetBytes int64, ttl time.Duration, sizeOf func(V) int, opts Options) *RepoCache[K, V] {
	return &RepoCache[K, V]{
		opts:    opts,
		budget:  budgetBytes,
		ttl:     ttl,
		sizeOf:  sizeOf,
		entries: make(map[K]cacheEntry[V]),
		order:   newLRUEvictor[K](),
	}
}

// Get returns the cached value for key, marking it recently used.
func (c *RepoCache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	e, ok := c.entries[key]
	if ok && !e.expires.IsZero() && !c.opts.now().Before(e.expires) {
		c.removeLocked(key, e)
		ok = false
	}
	if ok {
		c.order.Touch(key)
	}
	c.mu.Unlock()
	if !ok {
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	c.hits.Add(1)
	return e.val, true
}

// Put stores val under key, evicting least recently used entries to stay
// within budget. A value larger than the whole budget, and any value stored
// while the pressure function reports pressure, is not retained.
func (c *RepoCache[K, V]) Put(key K, val V) {
	if c.opts.underPressure() {
		return
	}
	size := int64(c.sizeOf(val))
	if size > c.budget {
		return
	}
	e := cacheEntry[V]{val: val, size: size}
	if c.ttl > 0 {
		e.expires = c.opts.now().Add(c.ttl)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.entries[key]; ok {
		c.removeLocked(key, old)
	}
	c.entries[key] = e
	c.order.Insert(key)
	c.bytes.Add(size)
	for c.bytes.Load() > c.budget {
		victim, ok := c.order.Victim()
		if !ok {
			break
		}
		c.removeLocked(victim, c.entries[victim])
		c.evictions.Add(1)
	}
}

// Delete removes key if present.
func (c *RepoCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.removeLocked(key, e)
	}
}

// Bytes returns the bytes currently held, without taking the cache mutex.
func (c *RepoCache[K, V]) Bytes() int64 { return c.bytes.Load() }

// Drop discards every entry (the Product eviction hook).
func (c *RepoCache[K, V]) Drop() { c.Clear() }

// Clear drops every entry.
func (c *RepoCache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[K]cacheEntry[V])
	c.order = newLRUEvictor[K]()
	c.bytes.Store(0)
}

func (c *RepoCache[K, V]) removeLocked(key K, e cacheEntry[V]) {
	delete(c.entries, key)
	c.order.Remove(key)
	c.bytes.Add(-e.size)
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
	c.mu.Lock()
	entries := len(c.entries)
	c.mu.Unlock()
	return CacheStats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
		Coalesced: c.coalesced.Load(),
		Entries:   entries,
		Bytes:     c.bytes.Load(),
	}
}

// ErrNotFound marks a negative result; callers return it from a compute
// function to signal "absent", and the cache never stores it.
var ErrNotFound = errors.New("repostate: not found")
