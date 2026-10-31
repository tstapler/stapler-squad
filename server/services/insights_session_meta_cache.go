package services

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// watchSessionMetaTTL bounds how stale a WatchInsights event's backlog
// attribution can be. The meta comes from a full item_sessions scan, and the
// stream fires once per parsed transcript file, so an uncached read issued
// hundreds of back-to-back SELECTs per stream during a rescan.
const watchSessionMetaTTL = 5 * time.Second

// sessionMetaCache is a short-TTL cache plus singleflight over
// sessionMetaForSessions. The zero value is ready to use.
type sessionMetaCache struct {
	mu      sync.Mutex
	meta    map[string]SessionMeta
	expires time.Time
	flight  singleflight.Group

	now func() time.Time // test seam; nil means time.Now
}

func (c *sessionMetaCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// get returns the cached meta, loading it through load at most once per TTL
// (concurrent misses share one load). The returned map is read-only.
func (c *sessionMetaCache) get(ctx context.Context, load func(context.Context) map[string]SessionMeta) map[string]SessionMeta {
	c.mu.Lock()
	if c.meta != nil && c.clock().Before(c.expires) {
		m := c.meta
		c.mu.Unlock()
		return m
	}
	c.mu.Unlock()

	v, _, _ := c.flight.Do("meta", func() (any, error) {
		m := load(ctx)
		if m != nil {
			c.mu.Lock()
			c.meta, c.expires = m, c.clock().Add(watchSessionMetaTTL)
			c.mu.Unlock()
		}
		return m, nil
	})
	m, _ := v.(map[string]SessionMeta)
	return m
}
