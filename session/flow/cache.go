package flow

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// ErrBusy is returned when a caller's context expires while it waits for the
// one-slot limiter that serializes uncached computes.
var ErrBusy = errors.New("flow: another uncached compute is in progress")

const (
	defaultCacheTTL     = 30 * time.Second
	defaultCacheEntries = 64
	maxCoalesceAttempts = 3
)

// CacheOptions tunes a Cache; zero fields take the plan defaults (30s TTL, 64 entries).
type CacheOptions struct {
	TTL        time.Duration
	MaxEntries int
	// DefaultConfig identifies the cacheable Config; queries with any other
	// non-zero Config bypass the cache.
	DefaultConfig Config
	// slot, when set, is the limiter shared with other caches so every kind of
	// uncached compute serializes through one slot.
	slot chan struct{}
}

// Cache decorates an aggregation compute with a bounded TTL/LRU cache,
// singleflight coalescing and a one-slot limiter for uncached computes. It is
// generic over the result type V so it does not depend on the aggregator's
// result type; a hit returns the stored V unchanged, so its QueryMs and AsOf
// are those of the original compute. Safe for concurrent use.
type Cache[V any] struct {
	clock Clock
	opts  CacheOptions
	group singleflight.Group
	slot  chan struct{}

	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List // most recently used at the front
}

type cacheEntry[V any] struct {
	key     string
	value   V
	expires time.Time
}

// NewCache builds a Cache that reads time only from clock.
func NewCache[V any](clock Clock, opts CacheOptions) *Cache[V] {
	if opts.TTL <= 0 {
		opts.TTL = defaultCacheTTL
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = defaultCacheEntries
	}
	if opts.slot == nil {
		opts.slot = make(chan struct{}, 1)
	}
	return &Cache[V]{
		clock:   clock,
		opts:    opts,
		slot:    opts.slot,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
	}
}

// Len reports the number of cached entries (expired entries count until touched).
func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// Do returns the cached value for q, or runs compute once for all concurrent
// callers of the same query. Cacheable queries (default or unset Config, no
// ItemIDs filter) are keyed by WindowSpec and Cohort and stored; others are
// only coalesced. Misses run one at a time; a caller whose ctx expires while
// waiting (for the slot or a shared compute) gets ErrBusy wrapping ctx.Err(). Errors are never cached.
func (c *Cache[V]) Do(ctx context.Context, q FlowQuery, compute func(context.Context) (V, error)) (V, error) {
	var zero V
	cacheable := c.cacheable(q)
	key := c.cacheKey(q)
	if cacheable {
		if v, ok := c.get(key); ok {
			return v, nil
		}
	}
	flightKey := key
	if !cacheable {
		flightKey = c.flightKey(q)
	}

	var err error
	for attempt := 0; attempt < maxCoalesceAttempts; attempt++ {
		ch := c.group.DoChan(flightKey, func() (any, error) {
			storeKey := ""
			if cacheable {
				storeKey = key
			}
			return c.fill(ctx, storeKey, compute)
		})
		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("%w: %w", ErrBusy, ctx.Err())
		case res := <-ch:
			if res.Err == nil {
				return res.Val.(V), nil
			}
			err = res.Err
		}
		// The leader's own context can fail the shared call; a follower whose
		// context is still live retries rather than inheriting that failure.
		if ctx.Err() != nil || (!errors.Is(err, ErrBusy) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			break
		}
	}
	return zero, err
}

// fill runs as the singleflight leader: re-check the cache, take the slot,
// compute, and store under storeKey (empty means do not store).
func (c *Cache[V]) fill(ctx context.Context, storeKey string, compute func(context.Context) (V, error)) (any, error) {
	if storeKey != "" {
		if v, ok := c.get(storeKey); ok {
			return v, nil
		}
	}
	// select picks randomly when both cases are ready, so check ctx first: an
	// abandoned leader must not start a compute once its caller has given up.
	if ctx.Err() != nil {
		return nil, ErrBusy
	}
	select {
	case c.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrBusy
	}
	defer func() { <-c.slot }()

	v, err := compute(ctx)
	if err != nil {
		return nil, err
	}
	if storeKey != "" {
		c.put(storeKey, v)
	}
	return v, nil
}

func (c *Cache[V]) cacheable(q FlowQuery) bool {
	return len(q.ItemIDs) == 0 && (q.Config == (Config{}) || q.Config == c.opts.DefaultConfig)
}

func (c *Cache[V]) cacheKey(q FlowQuery) string {
	return fmt.Sprintf("%s|%s|%v|%t|%d", q.Window.CacheKey(), q.Cohort, q.Sections, q.Compact, q.ListLimit)
}

// flightKey widens the key with the filters that make a query uncacheable so
// distinct bypass queries are not coalesced into one another.
func (c *Cache[V]) flightKey(q FlowQuery) string {
	return fmt.Sprintf("%s|%+v|%v", c.cacheKey(q), q.Config, q.ItemIDs)
}

func (c *Cache[V]) get(key string) (V, bool) {
	var zero V
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return zero, false
	}
	e := el.Value.(*cacheEntry[V])
	if !c.clock.Now().Before(e.expires) {
		c.removeLocked(el)
		return zero, false
	}
	c.lru.MoveToFront(el)
	return e.value, true
}

func (c *Cache[V]) put(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	expires := c.clock.Now().Add(c.opts.TTL)
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*cacheEntry[V])
		e.value, e.expires = v, expires
		c.lru.MoveToFront(el)
		return
	}
	c.entries[key] = c.lru.PushFront(&cacheEntry[V]{key: key, value: v, expires: expires})
	for c.lru.Len() > c.opts.MaxEntries {
		c.removeLocked(c.lru.Back())
	}
}

func (c *Cache[V]) removeLocked(el *list.Element) {
	delete(c.entries, el.Value.(*cacheEntry[V]).key)
	c.lru.Remove(el)
}
