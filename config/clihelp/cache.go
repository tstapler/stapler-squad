package clihelp

import (
	"strconv"
	"sync"
	"time"
)

const (
	cacheMaxEntries = 128
	cacheTTL        = 10 * time.Minute
	timeoutTTL      = 60 * time.Second
)

// cacheKey identifies one version of one binary: shims that swap content
// change mtime or size, so stale results are never served across an upgrade.
type cacheKey struct {
	RealPath   string
	MtimeNanos int64
	Size       int64
}

func (k cacheKey) flightKey() string {
	return k.RealPath + "\x00" + strconv.FormatInt(k.MtimeNanos, 10) + "\x00" + strconv.FormatInt(k.Size, 10)
}

type cacheEntry struct {
	res     ProbeResult
	expires time.Time
}

// probeCache is a bounded, TTL'd map that evicts the oldest insertion.
type probeCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[cacheKey]cacheEntry
	order   []cacheKey
}

func newProbeCache(now func() time.Time) *probeCache {
	return &probeCache{now: now, entries: map[cacheKey]cacheEntry{}}
}

func (c *probeCache) get(k cacheKey) (ProbeResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok || !c.now().Before(e.expires) {
		return ProbeResult{}, false
	}
	return e.res, true
}

// put stores res when its status is cacheable; NOT_FOUND, ERROR, BUSY and
// NEEDS_CONFIRM describe the moment, not the binary, and are never stored.
func (c *probeCache) put(k cacheKey, res ProbeResult) {
	var ttl time.Duration
	switch res.Status {
	case ProbeStatusFoundParsed, ProbeStatusFoundNoFlags:
		ttl = cacheTTL
	case ProbeStatusTimeout:
		ttl = timeoutTTL
	case ProbeStatusUnspecified, ProbeStatusNotFound, ProbeStatusError, ProbeStatusBusy, ProbeStatusNeedsConfirm:
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[k]; !exists {
		c.order = appendBounded(c.order, k, cacheMaxEntries, func(old cacheKey) { delete(c.entries, old) })
	}
	c.entries[k] = cacheEntry{res: res, expires: c.now().Add(ttl)}
}

// keySet is a bounded, process-lifetime set of confirmed targets.
type keySet struct {
	mu    sync.Mutex
	set   map[cacheKey]struct{}
	order []cacheKey
}

func newKeySet() *keySet { return &keySet{set: map[cacheKey]struct{}{}} }

func (s *keySet) has(k cacheKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.set[k]
	return ok
}

func (s *keySet) add(k cacheKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.set[k]; ok {
		return
	}
	s.order = appendBounded(s.order, k, cacheMaxEntries, func(old cacheKey) { delete(s.set, old) })
	s.set[k] = struct{}{}
}

// appendBounded appends k, first evicting the oldest key when order is full.
func appendBounded(order []cacheKey, k cacheKey, limit int, evict func(cacheKey)) []cacheKey {
	if len(order) >= limit {
		evict(order[0])
		order = order[1:]
	}
	return append(order, k)
}
